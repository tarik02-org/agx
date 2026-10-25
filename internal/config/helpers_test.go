package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/adapters/builtin"
	"github.com/tarik02-org/agx/internal/config"
)

var builtins = builtin.Registry().Builtins([]string{"agx", "help", "install", "run"})

// write creates path with content, and its parent directories.
func write(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// absolute replaces ABS in YAML with dir, written so that it is absolute on every platform.
func absolute(dir, content string) string {
	return strings.ReplaceAll(content, "ABS", filepath.ToSlash(dir))
}

func mkdir(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func skill(t *testing.T, dir, name string) string {
	t.Helper()
	return write(t, filepath.Join(dir, "SKILL.md"), "---\nname: "+name+"\ndescription: "+name+"\n---\n"+name+"\n")
}

func canonical(t *testing.T, path string) string {
	t.Helper()
	c, err := config.Canonical(path)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func read(t *testing.T, root string) *config.Config {
	t.Helper()
	c, err := config.Load(root, config.Options{Builtins: builtins})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// load reads and validates the configuration at root with an empty cache.
func load(t *testing.T, root string) *config.Config {
	t.Helper()
	c, err := config.Load(root, config.Options{Builtins: builtins, Cache: filepath.Join(t.TempDir(), "cache")})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// wantError fails unless err is set and contains every fragment.
func wantError(t *testing.T, err error, fragments ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("got no error, want one containing %q", fragments)
	}
	for _, f := range fragments {
		if !strings.Contains(err.Error(), f) {
			t.Fatalf("error %q does not contain %q", err, f)
		}
	}
}

// isolateHome points the home directory at a fresh directory on every platform.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := mkdir(t, filepath.Join(t.TempDir(), "home"))
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func ids(resources []config.Resource, kind string) []string {
	out := []string{}
	for _, r := range resources {
		if r.Kind == kind {
			out = append(out, string(r.ID))
		}
	}
	return out
}
