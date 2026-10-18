package securefs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteIsPrivateAndAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new", "nested", "state.json")
	if err := WriteJSON(path, map[string]string{"a": "b"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "{\n  \"a\": \"b\"\n}\n" {
		t.Fatalf("content = %q, %v", b, err)
	}
	if err := Write(path, []byte("replaced"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v, %v", entries, err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	for _, p := range []string{path, filepath.Dir(path), filepath.Join(dir, "new")} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0o600)
		if info.IsDir() {
			want = 0o700
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s mode = %v, want %v", p, info.Mode().Perm(), want)
		}
	}
}

func TestContained(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "f"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := Contained(
		root,
		filepath.Join("sub", "f"),
	); err != nil ||
		!strings.HasSuffix(got, filepath.Join("sub", "f")) {
		t.Fatalf("Contained = %q, %v", got, err)
	}
	for _, rel := range []string{"../x", filepath.Join("sub", "..", "..", "x")} {
		if _, err := Contained(root, rel); err == nil || !strings.Contains(err.Error(), "escapes") {
			t.Errorf("Contained(%q) = %v", rel, err)
		}
	}
	if _, err := Contained(root, filepath.Join(root, "sub")); err == nil {
		t.Error("absolute path accepted")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Contained(root, "link"); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Errorf("escaping symlink accepted: %v", err)
	}
}

func TestCopyTree(t *testing.T) {
	src := t.TempDir()
	files := map[string]string{"SKILL.md": "skill", filepath.Join("scripts", "run.sh"): "#!/bin/sh\n"}
	for name, content := range files {
		path := filepath.Join(src, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(src, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "copy")
	if err := CopyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if b, err := os.ReadFile(filepath.Join(dst, name)); err != nil || string(b) != content {
			t.Errorf("%s = %q, %v", name, b, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, ".git")); err == nil {
		t.Error(".git copied")
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(filepath.Join(dst, "scripts", "run.sh")); info.Mode().Perm()&0o100 == 0 {
			t.Error("executable bit lost")
		}
	}

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(src, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := CopyTree(
		src,
		filepath.Join(t.TempDir(), "copy"),
	); err == nil ||
		!strings.Contains(err.Error(), "escapes") {
		t.Fatalf("escaping symlink copied: %v", err)
	}
}

func TestWithin(t *testing.T) {
	root := filepath.Join(string(filepath.Separator)+"a", "b")
	for _, tt := range []struct {
		path string
		want bool
	}{
		{root, true},
		{filepath.Join(root, "c"), true},
		{filepath.Join(string(filepath.Separator)+"a", "bc"), false},
		{string(filepath.Separator) + "a", false},
	} {
		if got := Within(root, tt.path); got != tt.want {
			t.Errorf("Within(%q, %q) = %v", root, tt.path, got)
		}
	}
}
