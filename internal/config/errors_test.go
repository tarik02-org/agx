package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/config"
)

func TestReadableDecodeErrors(t *testing.T) {
	for _, tt := range []struct{ name, content, want string }{
		{"syntax", "skills: [a, b\nmcp: {}\n", "agx.yaml:2: invalid YAML: did not find expected ',' or ']' (while parsing a flow sequence started at line 1)"},
		{"tab", "\tbad: tab\n", "agx.yaml:1: invalid YAML: found character that cannot start any token; indent with spaces"},
		{"list expected", "mcp:\n  x:\n    command: hello\n", "agx.yaml:3: mcp.x.command: expected a list, not a string"},
		{"string expected", "skills:\n  a: {path: [x]}\n", "agx.yaml:2: skills.a.path: expected a string, not a list"},
		{"in a sequence", "scopes:\n  - add: {skills: x}\n", "agx.yaml:2: scopes[0].add.skills: expected a list, not a string"},
		{"unknown key", "mcp:\n  x:\n    url: 'https://a.example'\n    bogus: 1\n", "agx.yaml:4: mcp.x: unknown key bogus"},
		{"nested unknown key", "harnesses:\n  codex:\n    args:\n      - {values: [a], mode: [x]}\n", "agx.yaml:4: harnesses.codex.args[0]: unknown key mode"},
		{"number expected", "daemon:\n  lease_seconds: abc\n", "agx.yaml:2: daemon.lease_seconds: expected a number, not a string"},
		{"missing import", "imports: [{path: other.yaml}]\n", "agx.yaml:1: import other.yaml: "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := write(t, filepath.Join(t.TempDir(), "agx.yaml"), tt.content)
			_, err := config.Read(root, builtins)
			wantError(t, err, tt.want)
			for _, internal := range []string{"go-yaml", "construct", "config.", "!!"} {
				if strings.Contains(err.Error(), internal) {
					t.Errorf("error %q exposes %q", err, internal)
				}
			}
		})
	}
}
