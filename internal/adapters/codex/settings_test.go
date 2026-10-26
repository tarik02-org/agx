package codex_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/tarik02-org/agx/internal/adapters/codex"
)

func TestSettingsArguments(t *testing.T) {
	args, err := codex.Adapter{}.SettingsArguments(map[string]any{
		"model_provider": "cpa",
		"notify":         []any{"say", "it's \"done\""},
		"model_providers": map[string]any{
			"cpa": map[string]any{"name": "CPA", "max_retries": int64(3), "stream": true},
		},
		"projects":    map[string]any{"/src/a.b": map[string]any{"trust_level": "trusted"}},
		"empty":       map[string]any{},
		"hooks":       []any{map[string]any{"command": "x"}},
		"temperature": 0.5,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"-c", "empty={}",
		"-c", `hooks=[{command = 'x'}]`,
		"-c", "model_provider='cpa'",
		"-c", "model_providers.cpa.max_retries=3",
		"-c", "model_providers.cpa.name='CPA'",
		"-c", "model_providers.cpa.stream=true",
		"-c", `notify=['say', "it's \"done\""]`,
		"-c", `projects={'/src/a.b' = {trust_level = 'trusted'}}`,
		"-c", "temperature=0.5",
	}
	if !slices.Equal(args, want) {
		t.Fatalf("args =\n%s", strings.Join(args, "\n"))
	}
	if _, err := (codex.Adapter{}).SettingsArguments(map[string]any{"a.b": "x"}, ""); err == nil {
		t.Fatal("a dotted top-level key was accepted")
	}
}

// Inline tables, which also carry the launch's MCP servers, must escape control characters the
// TOML way: Go escapes such as \a and \x01 are not TOML.
func TestSettingsArgumentsEscapeInlineTables(t *testing.T) {
	value := "bell\a, \x01 and \"quote\""
	args, err := codex.Adapter{}.SettingsArguments(map[string]any{"t": map[string]any{"a.b": value}}, "")
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]map[string]string
	if len(args) != 2 || toml.Unmarshal([]byte(strings.Replace(args[1], "=", " = ", 1)), &decoded) != nil ||
		decoded["t"]["a.b"] != value {
		t.Fatalf("args = %q", args)
	}
}
