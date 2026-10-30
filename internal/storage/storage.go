// Package storage opens agx's durable state and rebuildable cache databases.
package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/tarik02-org/agx/internal/securefs"
)

//go:generate go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate

//go:embed state/0*.sql cache/0*.sql
var migrations embed.FS

func State(dir string) string { return filepath.Join(dir, "state.db") }
func Cache(dir string) string { return filepath.Join(dir, "cache.db") }

func Open(ctx context.Context, path string) (*sql.DB, error) { return open(ctx, path, "state", "") }
func OpenCache(ctx context.Context, path string) (*sql.DB, error) {
	return open(ctx, path, "cache", "")
}

// OpenExisting leaves an absent database absent, and upgrades an existing one before use.
func OpenExisting(ctx context.Context, path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	if err := securefs.MakePrivate(filepath.Dir(path)); err != nil {
		return nil, err
	}
	if err := securefs.MakePrivate(path); err != nil {
		return nil, err
	}
	return open(ctx, path, "state", "ro")
}

func open(ctx context.Context, path, kind, mode string) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if mode == "" {
		if err := securefs.EnsureDir(filepath.Dir(path)); err != nil {
			return nil, err
		}
		if err := securefs.CreatePrivate(path); err != nil {
			return nil, err
		}
	}
	schema, err := fs.Sub(migrations, kind)
	if err != nil {
		return nil, err
	}
	db, err := connect(path, mode)
	if err != nil {
		return nil, err
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, schema)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	var version int64
	err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version)
	sources := provider.ListSources()
	latest := sources[len(sources)-1].Version
	if err == nil && version > latest {
		err = fmt.Errorf("database schema %d is newer than supported schema %d", version, latest)
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if version == latest {
		return db, nil
	}
	_ = db.Close()
	if err := migrate(ctx, path, schema); err != nil {
		return nil, err
	}
	db, err = connect(path, mode)
	if err != nil {
		return nil, err
	}
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err == nil && version != latest {
		err = fmt.Errorf("database schema %d does not match supported schema %d", version, latest)
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(ctx context.Context, path string, schema fs.FS) error {
	// Goose reads schema versions outside its migration transactions. Serialize
	// upgrades across processes; ordinary reads and writes need no application lock.
	lock := flock.New(path + ".migrate.lock")
	if _, err := lock.TryLockContext(ctx, 10*time.Millisecond); err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	db, err := connect(path, "")
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, schema)
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}

func connect(path, mode string) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	// A Windows drive letter belongs to the URI path, not its hostname.
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
	}
	q := url.Values{"_pragma": {"busy_timeout(5000)", "foreign_keys(ON)"}, "_txlock": {"immediate"}}
	if mode != "" {
		q.Set("mode", mode)
		q.Set("_txlock", "deferred")
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}
