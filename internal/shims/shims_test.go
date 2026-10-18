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
	kind := kinds()[0]
	file := filepath.Join(dir, fileName("codex-cpa", kind))
	switch kind {
	case Symlink:
		if link, err := os.Readlink(file); err != nil || link != agx {
			t.Fatalf("symlink = %q, %v", link, err)
		}
	case Hardlink:
		a, errA := os.Stat(file)
		b, errB := os.Stat(agx)
		if errA != nil || errB != nil || !os.SameFile(a, b) {
			t.Fatalf("hard link does not share agx's file: %v %v", errA, errB)
		}
	default:
		t.Fatalf("unexpected first kind %s", kind)
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
	if _, err := os.Lstat(filepath.Join(dir, fileName("claude-x", kind))); !errors.Is(err, fs.ErrNotExist) {
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
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("directory not empty after remove: %v", entries)
	}
}

func TestForeignFilesAreLeftAlone(t *testing.T) {
	agx := target(t)
	dir := filepath.Join(t.TempDir(), "shims")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(dir, fileName("foreign-x", kinds()[0]))
	if err := os.WriteFile(foreign, []byte("mine"), 0o755); err != nil {
		t.Fatal(err)
	}
	changes, err := Install(dir, agx, []string{"foreign-x", "pi-r"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		if c.Name == "foreign-x" && (c.Action != Kept || c.State != Conflict) {
			t.Fatalf("foreign file change = %+v", c)
		}
	}
	if b, _ := os.ReadFile(foreign); string(b) != "mine" {
		t.Fatalf("foreign file overwritten: %q", b)
	}
	if s := states(t, dir, agx, []string{"foreign-x"}); s["foreign-x"] != Conflict {
		t.Fatalf("state = %v", s)
	}

	// A shim the user replaced is no longer agx's.
	shim := filepath.Join(dir, fileName("pi-r", kinds()[0]))
	if err := os.Remove(shim); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shim, []byte("user"), 0o755); err != nil {
		t.Fatal(err)
	}
	if s := states(t, dir, agx, []string{"pi-r"}); s["pi-r"] != Modified {
		t.Fatalf("state = %v, want modified", s)
	}
	changes, err = Remove(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := actions(changes); got["pi-r"] != Kept {
		t.Fatalf("remove = %v", got)
	}
	for _, path := range []string{foreign, shim} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s removed: %v", path, err)
		}
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

func TestManifestCannotEscape(t *testing.T) {
	dir := t.TempDir()
	manifest := `{"version":1,"shims":[{"name":"x","file":"../victim","kind":"symlink","target":"/agx"}]}`
	if err := os.WriteFile(filepath.Join(dir, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Remove(dir); err == nil || !strings.Contains(err.Error(), "invalid entry") {
		t.Fatalf("Remove = %v, want the manifest rejected", err)
	}
}

func TestScript(t *testing.T) {
	agx := target(t)
	dir := t.TempDir()
	s, err := write(dir, "claude-x", agx, Script)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, s.File))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if s.File != "claude-x.cmd" || !strings.Contains(string(b), `"`+agx+`" claude-x %*`) {
			t.Fatalf("script %s = %q", s.File, b)
		}
	} else if want := "#!/bin/sh\nexec '" + agx + "' 'claude-x' \"$@\"\n"; string(b) != want {
		t.Fatalf("script = %q, want %q", b, want)
	}
	if ok, err := current(dir, s, agx); err != nil || !ok {
		t.Fatalf("current = %v, %v", ok, err)
	}
	if got := script("/it's/agx", "x"); runtime.GOOS != "windows" && !strings.Contains(string(got), `'/it'\''s/agx'`) {
		t.Fatalf("quoting = %q", got)
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

func TestDefaultDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		local := t.TempDir()
		t.Setenv("LOCALAPPDATA", local)
		if dir, err := DefaultDir(); err != nil || dir != filepath.Join(local, "agx", "shims") {
			t.Fatalf("DefaultDir = %q, %v", dir, err)
		}
		return
	}
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	if dir, err := DefaultDir(); err != nil || dir != filepath.Join(data, "agx", "shims") {
		t.Fatalf("DefaultDir = %q, %v", dir, err)
	}
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", home)
	if dir, err := DefaultDir(); err != nil || dir != filepath.Join(home, ".local", "share", "agx", "shims") {
		t.Fatalf("DefaultDir = %q, %v", dir, err)
	}
}

func TestManifestFromNewerAgx(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, manifestName),
		[]byte(`{"version":2,"shims":[],"extra":1}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	_, err := Remove(dir)
	if err == nil || !strings.Contains(err.Error(), "uses format version 2, but this agx reads only version 1") {
		t.Fatalf("Remove = %v, want the newer manifest rejected", err)
	}
}
