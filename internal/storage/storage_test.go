package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gofrs/flock"
)

func TestConcurrentInitialization(t *testing.T) {
	if path := os.Getenv("AGX_TEST_DATABASE"); path != "" {
		db, err := Open(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		if _, err := db.ExecContext(
			t.Context(),
			"INSERT INTO secrets(key, ciphertext) VALUES (?, ?)",
			os.Getpid(),
			"encrypted",
		); err != nil {
			t.Fatal(err)
		}
		return
	}
	path := State(t.TempDir())
	var group sync.WaitGroup
	for range 4 {
		group.Go(func() {
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestConcurrentInitialization$")
			cmd.Env = append(os.Environ(), "AGX_TEST_DATABASE="+path)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("initializing process: %v\n%s", err, output)
			}
		})
	}
	group.Wait()
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM secrets").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("retained %d of 4 concurrent writes", count)
	}
}

func TestMigrationsPreserveStateAndRollbackFailure(t *testing.T) {
	path := State(t.TempDir())
	initial, err := fs.ReadFile(migrations, "state/001_state.sql")
	if err != nil {
		t.Fatal(err)
	}
	base := fstest.MapFS{"001_state.sql": &fstest.MapFile{Data: initial}}
	if err := migrate(t.Context(), path, base); err != nil {
		t.Fatal(err)
	}
	db, err := connect(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "INSERT INTO secrets VALUES ('credential','ciphertext')"); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	schema := fstest.MapFS{
		"001_state.sql": &fstest.MapFile{Data: initial},
		"002_upgrade.sql": &fstest.MapFile{
			Data: []byte(
				"-- +goose Up\nALTER TABLE secrets ADD COLUMN label TEXT NOT NULL DEFAULT '';\nPRAGMA user_version = 2;\n",
			),
		},
	}
	if err := migrate(t.Context(), path, schema); err != nil {
		t.Fatal(err)
	}
	db, err = connect(path, "")
	if err != nil {
		t.Fatal(err)
	}
	var value, label string
	if err := db.QueryRowContext(t.Context(), "SELECT ciphertext, label FROM secrets WHERE key = 'credential'").
		Scan(&value, &label); err != nil {
		t.Fatal(err)
	}
	if value != "ciphertext" || label != "" {
		t.Fatalf("credential after upgrade: %q, %q", value, label)
	}
	_ = db.Close()
	schema["003_broken.sql"] = &fstest.MapFile{
		Data: []byte(
			"-- +goose Up\nDELETE FROM secrets;\nINSERT INTO missing_table VALUES (1);\nPRAGMA user_version = 3;\n",
		),
	}
	if err := migrate(t.Context(), path, schema); err == nil {
		t.Fatal("broken migration succeeded")
	}
	db, err = connect(path, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var count, version int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM secrets").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if count != 1 || version != 2 {
		t.Fatalf("failed migration lost state: count=%d version=%d", count, version)
	}
	if _, err := db.ExecContext(t.Context(), "PRAGMA user_version = 99999"); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(t.Context(), path); err == nil {
		t.Fatal("an older client accepted a newer database")
	}
}

func TestLaunchAssociationIntegrity(t *testing.T) {
	db, err := Open(t.Context(), State(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	insert := "INSERT INTO launch_sessions(builtin,store,session,launch) VALUES('codex','store','s','l')"
	if _, err := db.ExecContext(t.Context(), insert); err == nil {
		t.Fatal("association accepted a missing launch")
	}
	if _, err := db.ExecContext(
		t.Context(),
		"INSERT INTO launches VALUES('l','codex-work','codex','store','directory',1,NULL,1)",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), insert); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "DELETE FROM launches WHERE id='l'"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM launch_sessions").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("deleted launch left an orphan association")
	}
}

func TestCancelledMigrationWaitKeepsState(t *testing.T) {
	path := State(t.TempDir())
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	lock := flock.New(path + ".migrate.lock")
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Unlock() }()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	schema, err := fs.Sub(migrations, "state")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, path, schema); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled migration: %v", err)
	}
}

func TestOpenRestoresPrivatePermissions(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		t.Run(fmt.Sprint(readOnly), func(t *testing.T) {
			testDatabasePermissions(t, readOnly)
		})
	}
}

func testDatabasePermissions(t *testing.T, readOnly bool) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	path := State(t.TempDir())
	db, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if readOnly {
		db, err = OpenExisting(t.Context(), path)
	} else {
		db, err = Open(t.Context(), path)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("database permissions = %v", info.Mode())
	}
}

func TestReadExistingLeavesMissingDatabaseAbsent(t *testing.T) {
	path := State(t.TempDir())
	if _, err := OpenExisting(t.Context(), path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absent read = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read created database: %v", err)
	}
	initial, err := fs.ReadFile(migrations, "state/001_state.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := fstest.MapFS{"001_state.sql": &fstest.MapFile{Data: initial}}
	if err := migrate(t.Context(), path, schema); err != nil {
		t.Fatal(err)
	}
	db, err := connect(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "INSERT INTO secrets VALUES ('credential','encrypted')"); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	db, err = OpenExisting(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var ciphertext string
	if err := db.QueryRowContext(t.Context(), "SELECT ciphertext FROM secrets WHERE key='credential'").
		Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if ciphertext != "encrypted" {
		t.Fatalf("read changed credential: %q", ciphertext)
	}
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM configurations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("read invented a local configuration")
	}
	if _, err := db.ExecContext(t.Context(), "DELETE FROM secrets"); err == nil {
		t.Fatal("read connection permits writes")
	}
}
