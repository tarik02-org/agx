//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestHarnessSettings launches definitions with native settings: Codex gets one -c per leaf
// before agx's own overrides and the definition's args, and Claude Code a --settings file that
// lives as long as the launch.
func TestHarnessSettings(t *testing.T) {
	e := newEnv(t)
	harness := "'" + filepath.ToSlash(binaries.harness) + "'"
	e.write("config/main.yaml", `harnesses:
  codex:
    executable: `+harness+`
    settings:
      model: o3
  codex-cpa:
    base: codex
    args: [-c, model="from-args"]
    settings:
      model_provider: cpa
      model_providers:
        cpa: {name: CPA, base_url: "http://127.0.0.1:8317/v1"}
  claude:
    executable: `+harness+`
  claude-work:
    base: claude
    settings:
      model: opus
      permissions: {allow: ["Bash(npm test:*)"]}
`)
	e.ok("daemon", "start")
	t.Cleanup(func() { e.stopDaemon(true) })

	d, r := e.harness("codex-cpa", "exec", "hi")
	if r.code != 0 {
		t.Fatalf("codex-cpa exited %d: %s", r.code, r.stderr)
	}
	want := []string{
		"-c", "model='o3'",
		"-c", "model_provider='cpa'",
		"-c", "model_providers.cpa.base_url='http://127.0.0.1:8317/v1'",
		"-c", "model_providers.cpa.name='CPA'",
	}
	if !slices.Equal(d.Args[:len(want)], want) {
		t.Fatalf("codex args = %q", d.Args)
	}
	// The definition's args follow, so they override a setting of the same key.
	if i := slices.Index(d.Args, `model="from-args"`); i < len(want) || d.Args[len(d.Args)-2] != "exec" {
		t.Fatalf("codex args = %q", d.Args)
	}

	claude, r := e.harness("claude-work", "-p", "hi")
	if r.code != 0 {
		t.Fatalf("claude-work exited %d: %s", r.code, r.stderr)
	}
	if len(claude.Args) < 2 || claude.Args[0] != "--settings" || !strings.HasSuffix(claude.Args[1], "settings.json") {
		t.Fatalf("claude args = %q", claude.Args)
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(claude.Settings), &settings); err != nil || settings["model"] != "opus" {
		t.Fatalf("settings file = %q", claude.Settings)
	}
	// The daemon removes a launch's files once it ends.
	deadline := time.Now().Add(10 * time.Second)
	for _, err := os.Stat(claude.Args[1]); !os.IsNotExist(err); _, err = os.Stat(claude.Args[1]) {
		if time.Now().After(deadline) {
			t.Fatalf("the settings file outlived its launch: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
