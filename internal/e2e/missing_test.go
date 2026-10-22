//go:build e2e

package e2e

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLaunchWithoutInstalledPackage launches with a package that was never installed: the
// launch runs with everything else and warns once about the package, and the daemon logs it.
func TestLaunchWithoutInstalledPackage(t *testing.T) {
	e := newEnv(t)
	e.write("tools/SKILL.md", "---\nname: tools\ndescription: d\n---\n")
	e.write("config/agx.yaml", `packages:
  team: {path: ../team}
skills:
  tools: {path: ../tools}
scopes:
  - add:
      skills: [team/review, team/lint, tools]
harnesses:
  claude:
    executable: '`+filepath.ToSlash(binaries.harness)+`'
`)
	validated := e.ok("config", "validate")
	if strings.Count(validated.stderr, "package team is not installed; run agx install") != 1 {
		t.Fatalf("config validate stderr = %q", validated.stderr)
	}
	e.ok("daemon", "start")
	t.Cleanup(func() { e.stopDaemon(true) })
	d, r := e.harness("claude", "-p", "hi")
	if r.code != 0 {
		t.Fatalf("launch exited %d: %s", r.code, r.stderr)
	}
	if _, ok := d.Skills["tools"]; !ok || len(d.Skills) != 1 {
		t.Fatalf("skills = %v", d.Skills)
	}
	if strings.Count(r.stderr, "package team is not installed; run agx install") != 1 {
		t.Fatalf("launch stderr = %q", r.stderr)
	}
	logged := ""
	_ = filepath.WalkDir(e.dir, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.Name() == "daemon.log" {
			b, _ := os.ReadFile(path)
			logged += string(b)
		}
		return nil
	})
	if !strings.Contains(logged, "agx daemon: warning: package team is not installed; run agx install") {
		t.Fatalf("daemon log = %q", logged)
	}
}
