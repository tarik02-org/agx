package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// skillRepository commits a repository holding the skills alpha and beta.
func skillRepository(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "kit.git")
	for _, name := range []string{"alpha", "beta"} {
		path := filepath.Join(dir, "skills", name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		content := "---\nname: " + name + "\ndescription: " + name + " skill\n---\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "."},
		{"-c", "user.name=agx", "-c", "user.email=agx@example.com", "commit", "-q", "-m", "skills"},
	} {
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestRemoveSkillPackage(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	path := isolate(t, "{}\n")
	t.Chdir(t.TempDir())
	repo := skillRepository(t)

	var added editDocument
	okJSON(t, &added, "skill", "add", "file://"+filepath.ToSlash(repo), "--skill", "alpha")
	lockfile := filepath.Join(filepath.Dir(path), "agx.lock.json")
	if lock, err := os.ReadFile(lockfile); err != nil || !strings.Contains(string(lock), `"kit"`) {
		t.Fatalf("lockfile after skill add: %v\n%s", err, lock)
	}

	d := failJSON(t, 1, codeReadOnly, "skill", "remove", "kit/alpha", "--json")
	if !strings.Contains(d.Error.Message, "agx skill remove kit") {
		t.Fatalf("removing a package skill: %s", d.Error.Message)
	}
	d = failJSON(t, 1, codeConflict, "skill", "remove", "kit", "--json")
	if !strings.Contains(d.Error.Message, "still referenced") {
		t.Fatalf("removing a referenced package: %s", d.Error.Message)
	}

	var removed editDocument
	okJSON(t, &removed, "skill", "remove", "kit", "--force")
	if len(removed.Removed) != 1 || removed.Removed[0] != (definitionJSON{"packages", "kit"}) {
		t.Fatalf("skill remove = %+v", removed)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "kit") {
		t.Fatalf("configuration still names the package:\n%s", content)
	}
	if lock, err := os.ReadFile(lockfile); err != nil || strings.Contains(string(lock), `"kit"`) {
		t.Fatalf("lockfile after removal: %v\n%s", err, lock)
	}
	okJSON(t, nil, "config", "validate")
}
