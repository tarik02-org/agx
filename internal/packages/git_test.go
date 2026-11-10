package packages

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitMissingFromPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := git(t.Context(), "https", false, "version")
	if !errors.Is(err, ErrFetch) || !strings.Contains(err.Error(), "git is not on PATH") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckoutGitFetchesOneRevision(t *testing.T) {
	repo := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(cmd.Environ(), "GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@a", "GIT_COMMITTER_NAME=a",
			"GIT_COMMITTER_EMAIL=a@a", "GIT_CONFIG_GLOBAL="+os.DevNull)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q", "-b", "main")
	run("commit", "-q", "--allow-empty", "-m", "first")
	first := run("rev-parse", "HEAD")
	run("tag", "-a", "v1", "-m", "v1")
	run("branch", "old")
	run("commit", "-q", "--allow-empty", "-m", "second")
	second := run("rev-parse", "HEAD")
	// A daemon started from a Git hook keeps the hook's repository in its environment.
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "elsewhere"))
	t.Setenv("GIT_WORK_TREE", t.TempDir())
	url := "file://" + filepath.ToSlash(repo)
	for ref, want := range map[string]string{"HEAD": second, "main": second, "old": first, "v1": first, first: first} {
		dir := filepath.Join(t.TempDir(), "git")
		got, err := checkoutGit(t.Context(), dir, url, "file", ref)
		if err != nil || got != want {
			t.Errorf("checkoutGit(%s) = %s, %v; want %s", ref, got, err, want)
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
			t.Errorf("checkoutGit(%s) used another repository: %v", ref, err)
		}
	}
	if _, err := checkoutGit(t.Context(), filepath.Join(t.TempDir(), "git"), url, "file", "missing"); !errors.Is(
		err,
		errUnresolved,
	) {
		t.Errorf("missing ref: %v", err)
	}
}
