//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalConfiguration(t *testing.T) {
	e := newEnv(t)
	e.write("config/agx.yaml", "fragments: []\n")
	e.ok("daemon", "start")
	t.Cleanup(func() { e.stopDaemon(true) })
	e.ok("instructions", "add", "--id", "local", "--text", "SQLite instructions.", "--scope", "here")
	e.ok("skill", "add", filepath.Join(e.dir, "config", "skills", "review"), "--scope", "here")
	e.ok(
		"harness",
		"add",
		"sqlite-work",
		"--base",
		"claude",
		"--executable",
		binaries.harness,
		"--env",
		"DEFINED=!env E2E_TOKEN",
	)
	for _, restart := range []bool{false, true} {
		if restart {
			e.ok("daemon", "restart")
		}
		d, r := e.harness("sqlite-work", "-p", "hello")
		if r.code != 0 || d.Env["DEFINED"] != token || !strings.Contains(d.Instructions, "SQLite instructions.") ||
			len(d.Skills) != 1 {
			t.Fatalf("restart=%t: %+v %s", restart, d, r.stderr)
		}
	}
	data, err := os.ReadFile(filepath.Join(e.dir, "config", "agx.yaml"))
	if err != nil || string(data) != "fragments: []\n" {
		t.Fatalf("modified root: %s %v", data, err)
	}
	exported := e.ok("config", "export").stdout
	if !strings.Contains(exported, "!env E2E_TOKEN") || !strings.Contains(exported, "SQLite instructions.") {
		t.Fatalf("export: %s", exported)
	}
	e.ok("unuse", "local", "--scope", "here")
	d, r := e.harness("sqlite-work", "-p", "hello")
	if r.code != 0 || strings.Contains(d.Instructions, "SQLite instructions.") {
		t.Fatalf("unuse: %+v %s", d, r.stderr)
	}
	e.ok("instructions", "remove", "local", "--force")
	e.ok("config", "validate")
}
