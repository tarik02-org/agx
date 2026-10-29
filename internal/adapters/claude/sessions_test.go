package claude_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/adapters/claude"
	"github.com/tarik02-org/agx/internal/sessions"
)

func list(t *testing.T, home string) (map[string]adapters.Session, sessions.Stats) {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	index, err := sessions.OpenIndex(t.Context(), filepath.Join(t.TempDir(), "cache.db"), "claude")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	root, err := (claude.Adapter{}).SessionRoot(".", os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	found, err := (claude.Adapter{}).Sessions(context.Background(), index, root)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]adapters.Session{}
	for _, s := range found {
		byID[s.ID] = s
	}
	return byID, index.Stats()
}

func TestSessions(t *testing.T) {
	home, err := filepath.Abs(filepath.Join("testdata", "claude"))
	if err != nil {
		t.Fatal(err)
	}
	byID, stats := list(t, home)
	if len(byID) != 2 {
		t.Fatalf("sessions = %+v; sidechains, subagents, and transcripts without a directory are not listed", byID)
	}
	first := byID["22222222-0000-4000-8000-000000000001"]
	if first.Directory != "/work/project" || first.Title != "Uploader retries" ||
		first.Message != "Add retry to the uploader" {
		t.Errorf("first = %+v; injected caveats and commands are not typed messages", first)
	}
	if renamed := byID["22222222-0000-4000-8000-000000000002"]; renamed.Title != "Renamed by user" {
		t.Errorf("title = %q, want the custom title to outrank the generated one", renamed.Title)
	}
	if stats.Malformed != 1 {
		t.Errorf("stats = %+v, want one malformed transcript", stats)
	}
	if got := (claude.Adapter{}).Resume(first); strings.Join(got, " ") != "--resume "+first.ID {
		t.Errorf("Resume = %q", got)
	}
}

func TestTitleBeyondTheHead(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "projects", "-work", "44444444-0000-4000-8000-000000000001.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	line := func(v any) {
		encoded, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(encoded)
		b.WriteByte('\n')
	}
	message := map[string]any{"role": "user", "content": "Long investigation"}
	line(map[string]any{"type": "user", "cwd": "/work", "timestamp": "2026-10-04T08:00:00Z", "message": message})
	for b.Len() <= adapters.SessionHead+1024 {
		line(map[string]any{"type": "attachment", "attachment": strings.Repeat("x", 4000)})
	}
	line(map[string]any{"type": "custom-title", "customTitle": "Renamed late"})
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	byID, _ := list(t, home)
	if s := byID["44444444-0000-4000-8000-000000000001"]; s.Title != "Renamed late" ||
		s.Message != "Long investigation" {
		t.Fatalf("session = %+v", s)
	}
}
