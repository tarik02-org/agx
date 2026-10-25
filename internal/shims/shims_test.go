package shims

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// target is a stand-in for the agx executable.
func target(t *testing.T) string {
	t.Helper()
	name := "agx"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("agx binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func actions(changes []Change) map[string]Action {
	out := map[string]Action{}
	for _, c := range changes {
		out[c.Name] = c.Action
	}
	return out
}

func states(t *testing.T, dir, agx string, names []string) map[string]State {
	t.Helper()
	entries, err := List(dir, agx, names)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]State{}
	for _, e := range entries {
		out[e.Name] = e.State
	}
	return out
}

func install(t *testing.T, dir, agx string, names ...string) map[string]Action {
	t.Helper()
	changes, err := Install(dir, agx, names)
	if err != nil {
		t.Fatal(err)
	}
	return actions(changes)
}

func TestInstallSyncsDefinitions(t *testing.T) {
	agx := target(t)
	dir := filepath.Join(t.TempDir(), "shims")

	got := install(t, dir, agx, "codex-cpa", "claude-x")
	if got["codex-cpa"] != Created || got["claude-x"] != Created {
		t.Fatalf("first install = %v", got)
	}
	file := filepath.Join(dir, fileName("codex-cpa"))
	if runtime.GOOS == "windows" {
		if b, err := os.ReadFile(file); err != nil || string(b) != "agx binary" || recorded(dir) != agx {
			t.Fatalf("copy = %q, %v; recorded %q", b, err, recorded(dir))
		}
	} else if link, err := os.Readlink(file); err != nil || link != agx {
		t.Fatalf("symlink = %q, %v", link, err)
	}
	if got := install(
		t,
		dir,
		agx,
		"codex-cpa",
		"claude-x",
	); got["codex-cpa"] != Unchanged ||
		got["claude-x"] != Unchanged {
		t.Fatalf("second install = %v, want idempotent", got)
	}

	got = install(t, dir, agx, "codex-cpa", "pi-r")
	if got["claude-x"] != Removed || got["pi-r"] != Created || got["codex-cpa"] != Unchanged {
		t.Fatalf("sync = %v", got)
	}
	if _, err := os.Lstat(filepath.Join(dir, fileName("claude-x"))); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("shim of a removed definition remains: %v", err)
	}
	if s := states(t, dir, agx, []string{"codex-cpa", "pi-r", "new"}); s["codex-cpa"] != Current ||
		s["pi-r"] != Current || s["new"] != Missing {
		t.Fatalf("states = %v", s)
	}

	changes, err := Remove(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := actions(changes); got["codex-cpa"] != Removed || got["pi-r"] != Removed {
		t.Fatalf("remove = %v", got)
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("directory remains after remove: %v", err)
	}
}

// agx owns the shim directory: installing replaces and removes anything else there.
func TestInstallOwnsTheDirectory(t *testing.T) {
	agx := target(t)
	dir := filepath.Join(t.TempDir(), "shims")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{fileName("pi-r"), "stray"} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte("other"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if s := states(t, dir, agx, []string{"pi-r"}); s["pi-r"] != Stale || s["stray"] != Stale {
		t.Fatalf("states = %v", s)
	}
	got := install(t, dir, agx, "pi-r")
	if runtime.GOOS != "windows" && got["pi-r"] != Updated || got["stray"] != Removed {
		t.Fatalf("install = %v", got)
	}
	if s := states(t, dir, agx, []string{"pi-r"}); s["pi-r"] != Current || len(s) != 1 {
		t.Fatalf("states = %v", s)
	}
}

func TestStaleTarget(t *testing.T) {
	agx := target(t)
	dir := filepath.Join(t.TempDir(), "shims")
	install(t, dir, agx, "codex-cpa")
	other := target(t)
	if s := states(t, dir, other, []string{"codex-cpa"}); s["codex-cpa"] != Stale {
		t.Fatalf("state = %v, want stale for another agx path", s)
	}
	if got := install(t, dir, other, "codex-cpa"); got["codex-cpa"] != Updated {
		t.Fatalf("install = %v, want updated", got)
	}
	if s := states(t, dir, other, []string{"codex-cpa"}); s["codex-cpa"] != Current {
		t.Fatalf("state = %v", s)
	}
}

func TestCollisions(t *testing.T) {
	agx := target(t)
	shimDir := t.TempDir()
	other := t.TempDir()
	name := "codex-cpa"
	if runtime.GOOS == "windows" {
		t.Setenv("PATHEXT", ".EXE")
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(other, name), []byte("other"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", strings.Join([]string{other, shimDir}, string(os.PathListSeparator)))
	got := Collisions(shimDir, agx, []string{"codex-cpa", "pi-r"})
	if len(got) != 1 || got[0].Name != "codex-cpa" || !got[0].First {
		t.Fatalf("collisions = %+v", got)
	}
	t.Setenv("PATH", strings.Join([]string{shimDir, other}, string(os.PathListSeparator)))
	if got := Collisions(shimDir, agx, []string{"codex-cpa"}); len(got) != 1 || got[0].First {
		t.Fatalf("collisions behind the shim directory = %+v", got)
	}
	if !OnPath(shimDir) || OnPath(t.TempDir()) {
		t.Fatal("OnPath is wrong")
	}
}

func TestDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		local := t.TempDir()
		t.Setenv("LOCALAPPDATA", local)
		if dir, err := Dir(); err != nil || dir != filepath.Join(local, "agx", "shims") {
			t.Fatalf("Dir = %q, %v", dir, err)
		}
		return
	}
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	if dir, err := Dir(); err != nil || dir != filepath.Join(data, "agx", "shims") {
		t.Fatalf("Dir = %q, %v", dir, err)
	}
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", home)
	if dir, err := Dir(); err != nil || dir != filepath.Join(home, ".local", "share", "agx", "shims") {
		t.Fatalf("Dir = %q, %v", dir, err)
	}
}

// A shim runs the agx it was made for, whatever build the shim's own file holds.
func TestOrigin(t *testing.T) {
	agx := target(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	install(t, dir, agx, "claude-cpa")
	if origin, ok := Origin(filepath.Join(dir, fileName("claude-cpa"))); !ok || origin != agx {
		t.Fatalf("Origin = %q, %v", origin, ok)
	}
	if _, ok := Origin(agx); ok {
		t.Fatal("a file outside the shim directory has an origin")
	}
}
