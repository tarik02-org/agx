// Package sessionprovider indexes summaries reported by native integrations.
// It does not read native stores or call native services.
package sessionprovider

import (
	"context"
	"errors"
	"sync"
	"time"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/sessions"
	"github.com/tarik02-org/agx/internal/storage"
	"github.com/tarik02-org/agx/internal/storage/cachedb"
)

var (
	ErrOffline = errors.New("session provider is offline")
	ErrStale   = errors.New("session provider instance has been replaced")
)

type sourceKey struct{ source, scope string }

// Connection tracks the lifetime of one publisher instance.
type Connection struct {
	Source *v1.SessionSource
	Done   chan struct{}
}

type Catalog struct {
	path        string
	mu          sync.Mutex
	connections map[sourceKey]*Connection
	retired     map[sourceKey]map[string]bool
	latest      map[sourceKey]string
}

func New(path string) *Catalog {
	return &Catalog{
		path:        path,
		connections: map[sourceKey]*Connection{},
		retired:     map[sourceKey]map[string]bool{},
		latest:      map[sourceKey]string{},
	}
}

func key(source *v1.SessionSource) sourceKey { return sourceKey{source.SourceId, source.ScopeId} }

func (c *Catalog) Connect(source *v1.SessionSource) (*Connection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := key(source)
	if c.retired[k][source.Epoch] {
		return nil, ErrStale
	}
	if previous := c.latest[k]; previous != "" && previous != source.Epoch {
		if c.retired[k] == nil {
			c.retired[k] = map[string]bool{}
		}
		c.retired[k][previous] = true
	}
	c.latest[k] = source.Epoch
	if old := c.connections[k]; old != nil {
		if old.Source.Epoch != source.Epoch {
			if c.retired[k] == nil {
				c.retired[k] = map[string]bool{}
			}
			c.retired[k][old.Source.Epoch] = true
		}
		close(old.Done)
	}
	connection := &Connection{Source: source, Done: make(chan struct{})}
	c.connections[k] = connection
	return connection, nil
}

func (c *Catalog) Disconnect(connection *Connection) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.connections[key(connection.Source)] == connection {
		delete(c.connections, key(connection.Source))
		close(connection.Done)
	}
}

func (c *Catalog) current(source *v1.SessionSource) (*Connection, error) {
	connection := c.connections[key(source)]
	if connection == nil {
		return nil, ErrOffline
	}
	if connection.Source.Epoch != source.Epoch || connection.Source.Builtin != source.Builtin ||
		connection.Source.Harness != source.Harness {
		return nil, ErrStale
	}
	return connection, nil
}

func (c *Catalog) Publish(ctx context.Context, request *v1.PublishSessionsRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.current(request.Source); err != nil {
		return err
	}
	db, err := storage.OpenCache(ctx, c.path)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	q := cachedb.New(tx)
	if request.Replace {
		rows, err := q.ListProviderSessions(ctx)
		if err != nil {
			return err
		}
		present := make(map[string]bool, len(request.Sessions))
		for _, session := range request.Sessions {
			present[session.Id] = true
		}
		// Preserve incoming rows so their timestamps still fence older snapshots. A session
		// moved to another scope is no longer owned by this scope's reconciliation.
		for _, row := range rows {
			if row.SourceID != request.Source.SourceId || row.ScopeID != request.Source.ScopeId ||
				present[row.SessionID] {
				continue
			}
			if err := q.DeleteProviderSession(
				ctx,
				cachedb.DeleteProviderSessionParams{
					SourceID:  row.SourceID,
					ScopeID:   row.ScopeID,
					SessionID: row.SessionID,
				},
			); err != nil {
				return err
			}
		}
	}
	for _, id := range request.DeletedIds {
		if err := q.DeleteProviderSession(
			ctx,
			cachedb.DeleteProviderSessionParams{
				SourceID:  request.Source.SourceId,
				ScopeID:   request.Source.ScopeId,
				SessionID: id,
			},
		); err != nil {
			return err
		}
	}
	for _, session := range request.Sessions {
		if err := q.PutProviderSession(ctx, cachedb.PutProviderSessionParams{
			SourceID: request.Source.SourceId, ScopeID: request.Source.ScopeId,
			Builtin: request.Source.Builtin, Harness: request.Source.Harness,
			SessionID: session.Id, Directory: session.Directory,
			Title: adapters.Excerpt(session.Title, 120), Message: adapters.Excerpt(session.Message, 200),
			Started: session.StartedAt.AsTime().UnixNano(), Updated: session.UpdatedAt.AsTime().UnixNano(),
		}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (c *Catalog) List(ctx context.Context) ([]sessions.Entry, error) {
	db, err := storage.OpenCache(ctx, c.path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	rows, err := cachedb.New(db).ListProviderSessions(ctx)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entries := make([]sessions.Entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, sessions.Entry{
			Session: adapters.Session{
				ID:        row.SessionID,
				Directory: row.Directory,
				Title:     row.Title,
				Message:   row.Message,
				Started:   time.Unix(0, row.Started),
				Updated:   time.Unix(0, row.Updated),
			},
			Builtin:  row.Builtin,
			Harness:  row.Harness,
			SourceID: row.SourceID,
			ScopeID:  row.ScopeID,
			Online:   c.connections[sourceKey{row.SourceID, row.ScopeID}] != nil,
		})
	}
	return entries, nil
}
