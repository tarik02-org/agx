package packages

import (
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseRemoteRefs(t *testing.T) {
	out := "ref: refs/heads/main\tHEAD\n" +
		"aaa\tHEAD\n" +
		"bbb\trefs/heads/feature\n" +
		"aaa\trefs/heads/main\n" +
		"ccc\trefs/tags/v1.9.0\n" +
		"ddd\trefs/tags/v1.10.0\n" +
		"ccc1\trefs/tags/v1.10.0^{}\n" +
		"eee\trefs/tags/v1.2.0\n"
	got := parseRemoteRefs(out)
	want := []Ref{
		{Name: "main", Commit: "aaa", Default: true},
		{Name: "feature", Commit: "bbb"},
		{Name: "v1.10.0", Tag: true, Commit: "ccc1"},
		{Name: "v1.9.0", Tag: true, Commit: "ccc"},
		{Name: "v1.2.0", Tag: true, Commit: "eee"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("parseRemoteRefs =\n%+v\nwant\n%+v", got, want)
	}
}

func TestRemoteRefs(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	repo := filepath.Join(t.TempDir(), "repo")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = repo
		cmd.Env = append(cmd.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := exec.CommandContext(t.Context(), "git", "init", "-q", "-b", "trunk", repo).Run(); err != nil {
		t.Fatal(err)
	}
	run("commit", "-q", "--allow-empty", "-m", "first")
	run("tag", "-a", "v1.0.0", "-m", "release")
	run("branch", "next")
	refs, err := RemoteRefs(t.Context(), "file://"+filepath.ToSlash(repo))
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 3 || refs[0].Name != "trunk" || !refs[0].Default || refs[1].Name != "next" ||
		refs[2].Name != "v1.0.0" || !refs[2].Tag || refs[2].Commit != refs[0].Commit {
		t.Fatalf("RemoteRefs = %+v", refs)
	}
}
