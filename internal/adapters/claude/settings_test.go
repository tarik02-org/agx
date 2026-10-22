package claude_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tarik02-org/agx/internal/adapters/claude"
)

func TestSettingsArguments(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "launch")
	args, err := claude.Adapter{}.SettingsArguments(map[string]any{
		"model":       "opus",
		"permissions": map[string]any{"allow": []any{"Bash(npm test:*)"}},
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	if len(args) != 2 || args[0] != "--settings" || args[1] != path {
		t.Fatalf("args = %q", args)
	}
	b, err := os.ReadFile(path)
	want := "{\n  \"model\": \"opus\",\n  \"permissions\": {\n    \"allow\": [\n      \"Bash(npm test:*)\"\n    ]\n  }\n}\n"
	if err != nil || string(b) != want {
		t.Fatalf("settings.json = %q, %v", b, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 && os.PathSeparator == '/' {
		t.Fatalf("mode = %v, %v", info.Mode(), err)
	}
	if args, err := (claude.Adapter{}).SettingsArguments(
		map[string]any{"model": "opus"},
		"",
	); err != nil ||
		args != nil {
		t.Fatalf("installation args = %q, %v", args, err)
	}
}
