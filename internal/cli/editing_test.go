package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/config"
)

// Undoing unuse with use restores the configuration text exactly, whether unuse wrote a remove
// entry into an existing rule or a rule of its own.
func TestUseUndoesUnuse(t *testing.T) {
	for name, configuration := range map[string]string{
		"new rule": `mcp:
  docs:
    url: https://example.com/mcp # the docs server
scopes:
  # Everywhere.
  - add:
      mcp: [docs]
`,
		"existing rule": `mcp:
  docs:
    url: https://example.com/mcp
  fs:
    command: [npx, server-filesystem]
scopes:
  - add:
      mcp: [docs]
  - directory: HERE
    add:
      mcp: [fs]
`,
	} {
		t.Run(name, func(t *testing.T) {
			path := isolate(t, "")
			here, err := os.Getwd()
			if err == nil {
				here, err = config.Canonical(here)
			}
			if err != nil {
				t.Fatal(err)
			}
			configuration := strings.ReplaceAll(configuration, "HERE", filepath.ToSlash(here))
			if err := os.WriteFile(path, []byte(configuration), 0o600); err != nil {
				t.Fatal(err)
			}
			var unused, used editDocument
			okJSON(t, &unused, "unuse", "docs", "--scope", "here")
			if len(unused.Files) != 1 {
				t.Fatalf("unuse = %+v", unused)
			}
			okJSON(t, &used, "use", "docs", "--scope", "here")
			if len(used.Files) != 1 {
				t.Fatalf("use = %+v", used)
			}
			b, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != configuration {
				t.Fatalf("use after unuse left\n%s\nwant\n%s", b, configuration)
			}
		})
	}
}
