package codex

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestInstalledPlugins(t *testing.T) {
	home := t.TempDir()
	if names, err := installedPlugins(home); err != nil || len(names) != 0 {
		t.Fatalf("without config.toml: %q, %v", names, err)
	}
	config := `[plugins."review-1@agx-managed"]
enabled = true

[plugins."lint-2@agx-managed"]
enabled = false

[plugins."removed-3@agx-managed"]
enabled = true

[plugins."other@elsewhere"]
enabled = true
`
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{
		"agx-managed/review-1/1.0.0",
		"agx-managed/lint-2/1.0.0",
		// Removed: its cache holds no version.
		"agx-managed/removed-3",
		// Cached but not configured.
		"agx-managed/orphan-4/1.0.0",
		"elsewhere/other/1.0.0",
	} {
		if err := os.MkdirAll(filepath.Join(home, "plugins", "cache", dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	names, err := installedPlugins(home)
	if err != nil || !slices.Equal(names, []string{"lint-2", "review-1"}) {
		t.Errorf("installedPlugins = %q, %v", names, err)
	}
}

func TestStale(t *testing.T) {
	native := t.TempDir()
	now := time.Now()
	for _, name := range []string{"old", "current"} {
		writeFile(t, filepath.Join(native, "superseded", name), "")
		old := now.Add(-supersededGrace - time.Hour)
		if err := os.Chtimes(filepath.Join(native, "superseded", name), old, old); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(native, "superseded", "recent"), "")
	// "current" is configured again: its mark is no reason to remove it. "unknown" is marked now.
	got, err := stale(native, []string{"current", "old", "recent", "unknown"}, []string{"current"}, now)
	if err != nil || !slices.Equal(got, []string{"old"}) {
		t.Errorf("stale = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(native, "superseded", "unknown")); err != nil {
		t.Errorf("unknown not marked: %v", err)
	}
}
