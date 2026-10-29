// Package sessions lists the harnesses' own sessions for the launcher and agx sessions: it
// caches what adapters parse from their stores, and remembers which harness definition started
// each session.
package sessions

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"sync"
	"time"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/securefs"
)

// indexVersion changes whenever parsing does, so a newer agx parses every file again.
const indexVersion = 1

// Index caches parsed session files keyed by path, valid while a file's size and modification
// time are unchanged. It never holds more than a listing shows.
type Index struct {
	path    string
	mu      sync.Mutex
	entries map[string]indexEntry
	seen    map[string]bool
	changed bool
	stats   Stats
}

// Stats counts what one listing did with session files.
type Stats struct {
	Parsed    int `json:"parsed"`
	Cached    int `json:"cached"`
	Malformed int `json:"malformed"`
}

type indexEntry struct {
	Size      int64             `json:"size"`
	Modified  time.Time         `json:"modified"`
	Session   *adapters.Session `json:"session,omitempty"`
	Malformed bool              `json:"malformed,omitempty"`
}

type indexFile struct {
	Version int                   `json:"version"`
	Files   map[string]indexEntry `json:"files"`
}

// OpenIndex loads the cache at path; a missing, damaged, or outdated cache starts empty.
func OpenIndex(path string) *Index {
	x := &Index{path: path, entries: map[string]indexEntry{}, seen: map[string]bool{}}
	b, err := os.ReadFile(path)
	if err != nil {
		return x
	}
	var file indexFile
	if json.Unmarshal(b, &file) == nil && file.Version == indexVersion && file.Files != nil {
		x.entries = file.Files
	}
	return x
}

func (x *Index) Parse(path string, parse adapters.ParseFunc) (adapters.Session, bool) {
	f, err := os.Open(path)
	if err != nil {
		// A file removed while listing is simply gone; others count as unreadable.
		if !errors.Is(err, fs.ErrNotExist) {
			x.count(func(s *Stats) { s.Malformed++ })
		}
		return adapters.Session{}, false
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		x.count(func(s *Stats) { s.Malformed++ })
		return adapters.Session{}, false
	}
	x.mu.Lock()
	x.seen[path] = true
	cached, ok := x.entries[path]
	x.mu.Unlock()
	if ok && cached.Size == info.Size() && cached.Modified.Equal(info.ModTime()) {
		x.count(func(s *Stats) {
			s.Cached++
			if cached.Malformed {
				s.Malformed++
			}
		})
		if cached.Session == nil {
			return adapters.Session{}, false
		}
		return *cached.Session, true
	}
	session, listed, err := parse(f, info)
	e := indexEntry{Size: info.Size(), Modified: info.ModTime(), Malformed: err != nil}
	if err == nil && listed {
		e.Session = &session
	}
	x.mu.Lock()
	x.entries[path] = e
	x.changed = true
	x.stats.Parsed++
	if err != nil {
		x.stats.Malformed++
	}
	x.mu.Unlock()
	return session, err == nil && listed
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

// Save writes the cache when it changed. With prune, it keeps only files seen since it was opened;
// a listing where a store failed must not prune, or that store's entries would be lost.
func (x *Index) Save(prune bool) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	for path := range x.entries {
		if prune && !x.seen[path] {
			delete(x.entries, path)
			x.changed = true
		}
	}
	if !x.changed {
		return nil
	}
	return securefs.WriteCacheJSON(x.path, indexFile{Version: indexVersion, Files: x.entries})
}
