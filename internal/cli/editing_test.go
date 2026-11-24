package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tarik02-org/agx/internal/adapters/builtin"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/configstore"
)

func TestUseUndoesUnuse(t *testing.T) {
	path := isolate(t, "mcp:\n  docs:\n    command: [docs]\nscopes:\n  - add:\n      mcp: [docs]\n")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	t.Chdir(work)
	okJSON(t, nil, "unuse", "docs", "--scope", "here")
	okJSON(t, nil, "use", "docs", "--scope", "here")
	c, err := configstore.Load(
		os.Getenv("AGX_CONFIG"), os.Getenv("AGX_STATE"),
		config.Options{
			Builtins: builtin.Registry().Builtins(nil),

			Cache: os.Getenv("AGX_CACHE"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Resolve(filepath.Clean(work), "")
	if err != nil || len(result.Resources) != 1 {
		t.Fatalf("selection: %+v %v", result, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("changed YAML: %s %v", after, err)
	}
}
