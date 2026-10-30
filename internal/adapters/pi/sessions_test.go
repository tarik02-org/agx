package pi_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/adapters/pi"
	"github.com/tarik02-org/agx/internal/sessions"
	"github.com/tarik02-org/agx/internal/sessionstore"
)

func list(t *testing.T) (map[string]adapters.Session, sessions.Stats) {
	t.Helper()
	index, err := sessionstore.OpenIndex(t.Context(), filepath.Join(t.TempDir(), "cache.db"), "pi")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	root, err := (pi.Adapter{}).SessionRoot(".", os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	found, err := (pi.Adapter{}).Sessions(context.Background(), index, root)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]adapters.Session{}
	for _, s := range found {
		byID[s.ID] = s
	}
	return byID, index.Stats()
}

func fixtures(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata", "pi"))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSessions(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("PI_CODING_AGENT_DIR", fixtures(t))
	byID, stats := list(t)
	if len(byID) != 2 {
		t.Fatalf("sessions = %+v", byID)
	}
	if s := byID["33333333-0000-7000-8000-000000000001"]; s.Directory != "/work/project" ||
		s.Title != "Build profiling" || s.Message != "Profile the build" {
		t.Errorf("named session = %+v", s)
	}
	if s := byID["33333333-0000-7000-8000-000000000002"]; s.Directory != "/work/other" || s.Title != "" ||
		s.Message != "Sketch the cache design" {
		t.Errorf("plain session = %+v", s)
	}
	if stats.Malformed != 1 {
		t.Errorf("stats = %+v, want the headerless file malformed", stats)
	}
	if got := (pi.Adapter{}).Resume(byID["33333333-0000-7000-8000-000000000002"]); strings.Join(got, " ") !=
		"--session 33333333-0000-7000-8000-000000000002" {
		t.Errorf("Resume = %q", got)
	}
}

func TestSessionDirectory(t *testing.T) {
	sessionsDir := filepath.Join(fixtures(t), "sessions")

	t.Run("settings sessionDir", func(t *testing.T) {
		agent := t.TempDir()
		settings, err := json.Marshal(map[string]string{"sessionDir": sessionsDir})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(agent, "settings.json"), settings, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
		t.Setenv("PI_CODING_AGENT_DIR", agent)
		if byID, _ := list(t); len(byID) != 2 {
			t.Fatalf("sessions = %+v", byID)
		}
	})
	t.Run("environment override", func(t *testing.T) {
		t.Setenv("PI_CODING_AGENT_DIR", t.TempDir())
		t.Setenv("PI_CODING_AGENT_SESSION_DIR", sessionsDir)
		if byID, _ := list(t); len(byID) != 2 {
			t.Fatalf("sessions = %+v", byID)
		}
	})
}
