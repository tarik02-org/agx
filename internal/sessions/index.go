// Package sessions lists the harnesses' own sessions for the launcher and agx sessions: it
// caches what adapters parse from their stores, and remembers which harness definition started
// each session.
package sessions

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"maps"
	"os"
	"sync"
	"time"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/storage"
	"github.com/tarik02-org/agx/internal/storage/cachedb"
)

// indexVersion changes whenever parsing does, so a newer agx parses every file again.
const indexVersion = 1

// Index caches parsed session files keyed by path, valid while a file's size and modification
// time are unchanged. It never holds more than a listing shows.
type Index struct {
	db       *sql.DB
	store    string
	mu       sync.Mutex
	entries  map[string]indexEntry
	original map[string]indexEntry
	seen     map[string]bool
	changed  map[string]bool
	stats    Stats
	err      error
}

// Stats counts what one listing did with session files.
type Stats struct {
	Parsed    int `json:"parsed"`
	Cached    int `json:"cached"`
	Malformed int `json:"malformed"`
}

type indexEntry struct {
	Version   int
	Size      int64             `json:"size"`
	Modified  int64             `json:"modified"`
	Session   *adapters.Session `json:"session,omitempty"`
	Malformed bool              `json:"malformed,omitempty"`
}

func OpenIndex(ctx context.Context, path, store string) (*Index, error) {
	db, err := storage.OpenCache(ctx, path)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = db.Close()
		}
	}()
	x := &Index{
		db:      db,
		store:   store,
		entries: map[string]indexEntry{},
		seen:    map[string]bool{},
		changed: map[string]bool{},
	}
	rows, err := cachedb.New(db).ListSessionFiles(ctx, store)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		entry := indexEntry{Version: int(row.Version), Size: row.Size, Modified: row.Modified, Malformed: row.Malformed}
		if row.Listed && entry.Version == indexVersion {
			entry.Session = &adapters.Session{
				ID:        row.SessionID,
				Directory: row.Directory,
				Title:     row.Title,
				Message:   row.Message,
			}
			if row.Started.Valid {
				entry.Session.Started = time.Unix(0, row.Started.Int64)
			}
			if row.Updated.Valid {
				entry.Session.Updated = time.Unix(0, row.Updated.Int64)
			}
		}
		x.entries[row.Path] = entry
	}

	x.original = maps.Clone(x.entries)
	success = true
	return x, nil
}

func (x *Index) Close() error { return x.db.Close() }

func (x *Index) Parse(path string, parse adapters.ParseFunc) (adapters.Session, bool) {
	// A cached file is recognized by its metadata alone: opening thousands of large stores is what
	// makes a listing slow.
	info, err := os.Stat(path)
	if err != nil {
		x.unreadable(err)
		return adapters.Session{}, false
	}
	if session, ok, hit := x.cached(path, info); hit {
		return session, ok
	}
	f, err := os.Open(path)
	if err != nil {
		x.unreadable(err)
		return adapters.Session{}, false
	}
	defer func() { _ = f.Close() }()
	// The file may have changed since it was looked up; what is parsed is what the cache keeps.
	if info, err = f.Stat(); err != nil {
		x.unreadable(err)
		return adapters.Session{}, false
	}
	session, listed, err := parse(f, info)
	e := indexEntry{
		Version:   indexVersion,
		Size:      info.Size(),
		Modified:  info.ModTime().UnixNano(),
		Malformed: err != nil,
	}
	if err == nil && listed {
		e.Session = &session
	}
	x.mu.Lock()
	x.entries[path] = e
	x.changed[path] = true
	x.stats.Parsed++
	if err != nil {
		x.stats.Malformed++
	}
	x.mu.Unlock()
	return session, err == nil && listed
}

// unreadable counts a file that could not be read; one removed while listing is simply gone.
func (x *Index) unreadable(err error) {
	if !errors.Is(err, fs.ErrNotExist) {
		x.mu.Lock()
		defer x.mu.Unlock()
		x.stats.Malformed++
		if x.err == nil {
			x.err = err
		}
	}
}

func (x *Index) Err() error {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.err
}

// cached returns the session cached for path while info matches it; hit is false otherwise.
func (x *Index) cached(path string, info fs.FileInfo) (session adapters.Session, ok, hit bool) {
	x.mu.Lock()
	x.seen[path] = true
	cached, found := x.entries[path]
	x.mu.Unlock()
	if !found || cached.Version != indexVersion || cached.Size != info.Size() ||
		cached.Modified != info.ModTime().UnixNano() {
		return adapters.Session{}, false, false
	}
	x.count(func(s *Stats) {
		s.Cached++
		if cached.Malformed {
			s.Malformed++
		}
	})
	if cached.Session == nil {
		return adapters.Session{}, false, true
	}
	return *cached.Session, true, true
}

func (x *Index) count(update func(*Stats)) {
	x.mu.Lock()
	defer x.mu.Unlock()
	update(&x.stats)
}

func (x *Index) Stats() Stats {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.stats
}

// Save publishes changed files and prunes unseen files only after a complete scan.
func (x *Index) Save(ctx context.Context, prune bool) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	tx, err := x.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	q := cachedb.New(tx)
	saved := map[string]indexEntry{}
	pruned := []string{}
	for path, entry := range x.entries {
		if prune && !x.seen[path] {
			if err := q.DeleteSessionFile(ctx, cachedb.DeleteSessionFileParams{
				Store: x.store, Path: path, Modified: entry.Modified, Size: entry.Size, Version: int64(entry.Version),
			}); err != nil {
				return err
			}
			pruned = append(pruned, path)
			continue
		}
		if !x.changed[path] {
			continue
		}
		var session adapters.Session
		if entry.Session != nil {
			session = *entry.Session
		}
		var started, updated sql.NullInt64
		if !session.Started.IsZero() {
			started = sql.NullInt64{Int64: session.Started.UnixNano(), Valid: true}
		}
		if !session.Updated.IsZero() {
			updated = sql.NullInt64{Int64: session.Updated.UnixNano(), Valid: true}
		}
		var err error
		var affected int64
		if original, exists := x.original[path]; exists {
			affected, err = q.UpdateSessionFile(ctx, cachedb.UpdateSessionFileParams{
				Store:            x.store,
				Path:             path,
				Size:             entry.Size,
				Modified:         entry.Modified,
				Version:          int64(indexVersion),
				Malformed:        entry.Malformed,
				Listed:           entry.Session != nil,
				SessionID:        session.ID,
				Directory:        session.Directory,
				Title:            session.Title,
				Message:          session.Message,
				Started:          started,
				Updated:          updated,
				OriginalSize:     original.Size,
				OriginalModified: original.Modified,
				OriginalVersion:  int64(original.Version),
			})
		} else {
			affected, err = q.InsertSessionFile(ctx, cachedb.InsertSessionFileParams{
				Store:     x.store,
				Path:      path,
				Size:      entry.Size,
				Modified:  entry.Modified,
				Version:   int64(indexVersion),
				Malformed: entry.Malformed,
				Listed:    entry.Session != nil,
				SessionID: session.ID,
				Directory: session.Directory,
				Title:     session.Title,
				Message:   session.Message,
				Started:   started,
				Updated:   updated,
			})
		}
		if err != nil {
			return err
		}
		if affected != 0 {
			saved[path] = entry
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for path, entry := range saved {
		x.original[path] = entry
	}
	for _, path := range pruned {
		delete(x.original, path)
		delete(x.entries, path)
	}
	clear(x.changed)
	return nil
}
