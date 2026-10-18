package codex

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	now := time.Now()
	superseded := map[string]time.Time{
		"old":    now.Add(-supersededGrace - time.Hour),
		"recent": now.Add(-time.Hour),
		// Configured again by the installation: its mark is no reason to remove it.
		"current": now.Add(-supersededGrace - time.Hour),
	}
	got := stale([]string{"current", "old", "recent", "unknown"}, []string{"current"}, superseded, now)
	if !slices.Equal(got, []string{"old"}) {
		t.Errorf("stale = %q", got)
	}
}

func TestCatalogFromNewerAgx(t *testing.T) {
	cache := t.TempDir()
	writeFile(t, filepath.Join(cache, "native", "catalog.json"), `{"version":2,"names":[]}`)
	if _, err := readCatalog(cache); err == nil || !strings.Contains(err.Error(), "uses format version 2") {
		t.Fatalf("readCatalog = %v, want the newer catalog rejected", err)
	}
}
