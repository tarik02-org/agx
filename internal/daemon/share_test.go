package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tarik02-org/agx/internal/config"
)

func TestMCPDefinitionSharesPerRepositoryRoot(t *testing.T) {
	repository := t.TempDir()
	if err := os.Mkdir(filepath.Join(repository, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A worktree's .git is a file, and the worktree is a repository of its own.
	worktree := filepath.Join(repository, "worktrees", "feature")
	nested := filepath.Join(worktree, "packages", "api")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	server := config.MCPServer{Transport: "stdio", Command: []string{"x"}, Share: config.ShareRepository}
	for directory, want := range map[string]string{
		repository: repository,
		nested:     worktree,
		outside:    outside,
	} {
		definition, root := mcpDefinition(server, directory)
		if root != want || definition.Cwd != want {
			t.Errorf("%s: root %s, cwd %s; want %s", directory, root, definition.Cwd, want)
		}
	}
	server.Share = config.ShareDirectory
	if definition, root := mcpDefinition(server, nested); root != nested || definition.Cwd != nested {
		t.Errorf("directory share: root %s, cwd %s", root, definition.Cwd)
	}
	server.Share, server.Cwd = config.ShareGlobal, ""
	home, _ := os.UserHomeDir()
	if definition, root := mcpDefinition(server, nested); root != "" || definition.Cwd != home {
		t.Errorf("global share: root %q, cwd %s", root, definition.Cwd)
	}
}
