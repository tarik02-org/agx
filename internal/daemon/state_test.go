package daemon

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/control"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/storage"
)

func stateServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	return &Server{
		paths: platform.Paths{
			Runtime: filepath.Join(dir, "runtime"),
			Cache:   dir,
			State:   filepath.Join(dir, "state"),
		},
		launches: map[string]*launch{},
		revoked:  map[string]time.Time{},
		port:     4242,
		changed:  &notifier{},
	}
}

func TestStateKeepsRecentRevocations(t *testing.T) {
	s := stateServer(t)
	now := time.Now()
	s.revoked["old"] = now.Add(-revokedRetention - time.Minute)
	s.revoked["new"] = now.Add(-time.Minute)
	s.saveState()
	state, err := loadState(s.paths, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state.Revoked["new"]; state.Port != 4242 || len(state.Revoked) != 1 || !ok {
		t.Fatalf("state = %+v", state)
	}
	if err := os.WriteFile(storage.State(s.paths.State), bytes.Repeat([]byte("invalid"), 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	if state, err := loadState(s.paths, now); err == nil || state.Revoked == nil {
		t.Fatalf("invalid state = %+v, %v", state, err)
	}
}

func TestAttachable(t *testing.T) {
	s := stateServer(t)
	held := func(id string, state v1.Launch_State) {
		s.launches[id] = &launch{
			view:      &v1.Launch{Id: id, State: state},
			grant:     verifier("grant"),
			lifecycle: verifier("lifecycle"),
		}
	}
	held("running", v1.Launch_STATE_RUNNING)
	held("expired", v1.Launch_STATE_EXPIRED)
	held("finished", v1.Launch_STATE_FINISHED)
	s.revoked["revoked"] = time.Now()
	if l, err := s.attachable("running", "grant", "lifecycle"); err != nil || l != s.launches["running"] {
		t.Errorf("held launch = %v, %v", l, err)
	}
	for _, id := range []string{"expired", "unknown"} {
		if l, err := s.attachable(id, "grant", "lifecycle"); err != nil || l != nil {
			t.Errorf("%s launch = %v, %v", id, l, err)
		}
	}
	if _, err := s.attachable("running", "other", "lifecycle"); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("foreign grant: %v", err)
	}
	for id, how := range map[string]string{"finished": "finished", "revoked": "revoked"} {
		var e *connect.Error
		if _, err := s.attachable(
			id,
			"grant",
			"lifecycle",
		); !errors.As(err, &e) ||
			e.Meta().Get(control.EndedHeader) != how {
			t.Errorf("%s launch: %v", id, err)
		}
	}
}

func TestSweepKeepsHeldAndNewLaunchFiles(t *testing.T) {
	s := stateServer(t)
	s.launches["held"] = &launch{view: &v1.Launch{Id: "held"}}
	old := time.Now().Add(-time.Hour)
	for _, id := range []string{"held", "gone", "new", "other"} {
		dir := filepath.Join(s.paths.Cache, "launches", id)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		owner := s.paths.Config
		if id == "other" {
			owner += "/other"
		}
		writeFile(t, filepath.Join(dir, "configuration"), owner)
		if id != "new" {
			if err := os.Chtimes(dir, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	s.sweep(time.Now().Add(-time.Minute))
	for id, kept := range map[string]bool{"held": true, "gone": false, "new": true, "other": true} {
		_, err := os.Stat(filepath.Join(s.paths.Cache, "launches", id))
		if (err == nil) != kept {
			t.Errorf("%s: kept %v, want %v", id, err == nil, kept)
		}
	}
}

func TestRemoveAbandonedTemporaries(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	files := map[string]time.Duration{
		".agx-123":            time.Hour,
		".agx-456":            time.Minute,
		".agx-shim-1":         time.Hour,
		"daemon-state.json":   time.Hour,
		"launch-records.json": time.Hour,
	}
	for name, age := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	removeAbandonedTemporaries(dir, now)
	for name := range files {
		_, err := os.Stat(filepath.Join(dir, name))
		// Only the old temporary is abandoned; a recent one may belong to a write in progress.
		if gone := errors.Is(err, fs.ErrNotExist); gone != (name == ".agx-123") {
			t.Errorf("%s: removed %v", name, gone)
		}
	}
}

func TestServeRejectsCorruptState(t *testing.T) {
	s := stateServer(t)
	s.paths.Config = filepath.Join(t.TempDir(), "agx.yaml")
	if err := os.MkdirAll(s.paths.State, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(storage.State(s.paths.State), bytes.Repeat([]byte("invalid"), 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	close(stop)
	err := Serve(t.Context(), stop, s.paths, adapters.NewRegistry(), config.Builtins{})
	if err == nil {
		t.Fatalf("corrupt state was accepted: %v", err)
	}
}
