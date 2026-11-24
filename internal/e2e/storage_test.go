//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tarik02-org/agx/internal/storage"
)

func TestOnlyDaemonNeedsStateDirectory(t *testing.T) {
	e := newEnv(t)
	e.ok("daemon", "start")
	t.Cleanup(func() { e.stopDaemon(true) })
	blocked := filepath.Join(e.dir, "client-state")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	extra := []string{"AGX_STATE=" + blocked}
	for _, args := range [][]string{
		{"instructions", "add", "--id", "remote", "--text", "Daemon-owned configuration."},
		{"config", "validate"},
		{"config", "export"},
		{"install", "--frozen"},
		{"sessions", "--all", "--json"},
		{"claude-e2e", "--help"},
		{"claude", "-p", "hello"},
		{"instructions", "remove", "remote", "--force"},
	} {
		r := e.agx(extra, args...)
		if r.code != 0 {
			t.Fatalf("%v accessed client state or failed: %s\n%s", args, r.stderr, r.stdout)
		}
	}
	db, err := storage.Open(t.Context(), storage.State(filepath.Join(e.dir, "state")))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var finished int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM launches WHERE finished IS NOT NULL").
		Scan(&finished); err != nil ||
		finished != 1 {
		t.Fatalf("daemon launch history = %d, %v", finished, err)
	}
	data, err := os.ReadFile(blocked)
	if err != nil || string(data) != "not a directory" {
		t.Fatalf("client state changed: %q, %v", data, err)
	}
}

func TestCompletionAfterDaemonStopsWithBrokenConfiguration(t *testing.T) {
	e := restartEnv(t)
	t.Cleanup(func() { e.stopDaemon(true) })
	h := e.hold(binaries.agx)
	e.stopDaemon(false)
	e.write("config/main.yaml", "harnesses: [broken\n")
	before := time.Now()
	h.exit()
	if h.cmd.ProcessState == nil || !h.cmd.ProcessState.Success() {
		t.Fatalf("wrapper failed: %s", h.stderr.String())
	}
	db, err := storage.Open(t.Context(), storage.State(filepath.Join(e.dir, "state")))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var end int64
	if err := db.QueryRowContext(t.Context(), "SELECT finished FROM launches WHERE id = ?", launchID(h.endpoint())).
		Scan(&end); err != nil {
		t.Fatal(err)
	}
	if finished := time.Unix(0, end); finished.Before(before) || finished.After(time.Now()) {
		t.Fatalf("invalid finish time: %v", finished)
	}
	if strings.Contains(h.stderr.String(), "cannot record the launch") {
		t.Fatalf("completion was lost: %s", h.stderr.String())
	}
}
