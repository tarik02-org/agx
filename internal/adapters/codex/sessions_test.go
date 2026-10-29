package codex_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/adapters/codex"
	"github.com/tarik02-org/agx/internal/sessions"
)

func TestSessions(t *testing.T) {
	home, err := filepath.Abs(filepath.Join("testdata", "codex"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", home)
	index := sessions.OpenIndex(filepath.Join(t.TempDir(), "index.json"))
	root, err := (codex.Adapter{}).SessionRoot(".", os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	found, err := (codex.Adapter{}).Sessions(context.Background(), index, root)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]adapters.Session{}
	for _, s := range found {
		byID[s.ID] = s
	}
	if len(byID) != 2 {
		t.Fatalf("sessions = %+v, want two (the subagent thread and malformed rollout skipped)", found)
	}
	for _, tt := range []struct {
		id, directory, title, message string
		started                       time.Time
	}{
		{
			// The renamed title comes from the last index entry; the message skips AGENTS.md.
			"11111111-0000-7000-8000-000000000001", "/work/project", "Parser refactor", "Refactor the parser module",
			time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			"11111111-0000-7000-8000-000000000002", "/work/other", "", "Write release notes",
			time.Date(2026, 10, 2, 9, 30, 0, 0, time.UTC),
		},
	} {
		s := byID[tt.id]
		if s.Directory != tt.directory || s.Title != tt.title || s.Message != tt.message ||
			!s.Started.Equal(tt.started) {
			t.Errorf("%s = %+v", tt.id, s)
		}
	}
	if stats := index.Stats(); stats.Malformed != 1 || stats.Parsed != 4 {
		t.Errorf("stats = %+v, want 4 parsed and 1 malformed", stats)
	}
	if got := (codex.Adapter{}).Resume(byID["11111111-0000-7000-8000-000000000002"]); len(got) != 2 ||
		got[0] != "resume" || got[1] != "11111111-0000-7000-8000-000000000002" {
		t.Errorf("Resume = %q", got)
	}
}

func TestSessionsWithoutStore(t *testing.T) {
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "missing"))
	root, err := (codex.Adapter{}).SessionRoot(".", os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	found, err := (codex.Adapter{}).Sessions(
		context.Background(),
		sessions.OpenIndex(filepath.Join(t.TempDir(), "index.json")),
		root,
	)
	if err != nil || len(found) != 0 {
		t.Fatalf("missing store: %v, %v", found, err)
	}
}
