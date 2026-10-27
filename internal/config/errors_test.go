package config_test

import (
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/config"
)

func TestReadableDecodeErrors(t *testing.T) {
	for _, tt := range []struct{ name, content, want string }{
		{"syntax", "skills: [a, b\nmcp: {}\n", "main.yaml:2: invalid YAML: did not find expected ',' or ']' (while parsing a flow sequence started at line 1)"},
		{"tab", "\tbad: tab\n", "main.yaml:1: invalid YAML: found character that cannot start any token; indent with spaces"},
		{"list expected", "mcp:\n  x:\n    command: hello\n", "main.yaml:3: mcp.x.command: expected a list, not a string"},
		{"string expected", "skills:\n  a: {path: [x]}\n", "main.yaml:2: skills.a.path: expected a string, not a list"},
		{"in a sequence", "scopes:\n  - add: {skills: x}\n", "main.yaml:2: scopes[0].add.skills: expected a list, not a string"},
		{"unknown key", "mcp:\n  x:\n    url: 'https://a.example'\n    bogus: 1\n", "main.yaml:4: mcp.x: unknown key bogus"},
		{"misspelled key", "harnesses:\n  codex:\n    env: {A: b}\n", "main.yaml:3: harnesses.codex: unknown key env (did you mean environment?)"},
		{"nested unknown key", "harnesses:\n  codex:\n    args:\n      - {values: [a], mode: [x]}\n", "main.yaml:4: harnesses.codex.args[0]: unknown key mode"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := fragment(t, t.TempDir(), tt.content)
			_, err := config.Load(root, config.Options{Builtins: builtins})
			wantError(t, err, tt.want)
			for _, internal := range []string{"go-yaml", "construct", "config.", "!!"} {
				if strings.Contains(err.Error(), internal) {
					t.Errorf("error %q exposes %q", err, internal)
				}
			}
		})
	}
}
