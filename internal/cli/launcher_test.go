package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tarik02-org/agx/internal/adapters/builtin"
	"github.com/tarik02-org/agx/internal/config"
)

func TestLauncherHarnessesSkipHidden(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "agx.yaml")
	if err := os.WriteFile(root, []byte(`harnesses:
  claude: {hidden: true}
  codex: {hidden: true}
  codex-cpa: {base: codex}
  claude-cpa: {base: claude}
  claude-old: {base: claude, hidden: true}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := config.Read(root, builtin.Registry().Builtins(nil))
	if err != nil {
		t.Fatal(err)
	}
	harnesses, err := launcherHarnesses(c)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, h := range harnesses {
		names = append(names, h.Name)
	}
	if want := []string{"pi", "claude-cpa", "codex-cpa"}; !slices.Equal(names, want) {
		t.Fatalf("launcher harnesses = %v, want %v", names, want)
	}
}
