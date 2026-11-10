package claude_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
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

func TestMergeNativeSettings(t *testing.T) {
	dir := t.TempDir()
	local := []byte(`{"model": "sonnet", "cleanupPeriodDays": 30}`)
	if err := os.WriteFile(filepath.Join(dir, "local.json"), local, 0o600); err != nil {
		t.Fatal(err)
	}
	definition := map[string]any{
		"model":       "opus",
		"permissions": map[string]any{"allow": []any{"Bash(npm test:*)"}, "deny": []any{"Read(./.env)"}},
	}
	settings, rest, err := claude.Adapter{}.MergeNativeSettings(definition, []string{
		"--settings", `{"showThinkingSummaries":true,"permissions":{"allow":["Bash(npm test:*)","Read"]}}`,
		"-p", "hi",
		"--settings=local.json",
		"--", "--settings", "{}",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(settings)
	want := `{"cleanupPeriodDays":30,"model":"sonnet","permissions":{"allow":["Bash(npm test:*)","Read"],` +
		`"deny":["Read(./.env)"]},"showThinkingSummaries":true}`
	if string(got) != want {
		t.Fatalf("settings = %s", got)
	}
	if !slices.Equal(rest, []string{"-p", "hi", "--", "--settings", "{}"}) {
		t.Fatalf("rest = %q", rest)
	}
	if allow := definition["permissions"].(map[string]any)["allow"].([]any); len(allow) != 1 {
		t.Fatalf("definition settings changed: %v", definition)
	}
	missing := []string{"--settings", "missing.json"}
	if _, _, err := (claude.Adapter{}).MergeNativeSettings(definition, missing, dir); err == nil {
		t.Fatal("a missing settings file merged")
	}
}
