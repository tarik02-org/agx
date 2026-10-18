package config_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/config"
)

// The resolved values below must never appear in snapshots or their JSON.
const (
	secretValue = "secret-VALUE-1111"
	envValue    = "env-VALUE-2222"
	fileValue   = "file-VALUE-3333"
)

func tagFixture(t *testing.T) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	work := mkdir(t, filepath.Join(dir, "work"))
	write(t, filepath.Join(dir, "conf", "prompt.txt"), fileValue+"\n\n")
	write(t, filepath.Join(dir, "conf", "harness.yaml"), `harnesses:
  codex-t:
    base: codex
    args:
      - [--prompt, !file prompt.txt]
      - values: [--system, !file prompt.txt]
        modes: [exec]
    environment:
      TOKEN: !env WORK_TOKEN
      KEY: !secret key
      PLAIN: plain
`)
	root := write(t, filepath.Join(dir, "agx.yaml"), `imports: [{path: conf/harness.yaml}]
secrets:
  key: {kind: environment, variable: KEY_SOURCE}
mcp:
  up:
    command: [up]
    environment:
      UP_SECRET: !secret key
      UP_ENV: !env MCP_ONLY
  remote:
    url: 'https://remote.example/mcp'
    headers:
      X-Key: !secret key
scopes:
  - add: {mcp: [up, remote]}
`)
	t.Setenv("KEY_SOURCE", secretValue)
	t.Setenv("WORK_TOKEN", envValue)
	t.Setenv("MCP_ONLY", envValue)
	return load(t, root), work
}

func TestTagsResolveAtLaunchOnly(t *testing.T) {
	c, work := tagFixture(t)
	s, err := c.Resolve(work, "codex-t")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"KEY_SOURCE", "MCP_ONLY", "WORK_TOKEN"}; !slices.Equal(s.RequiredEnvironment, want) {
		t.Errorf("required environment = %v, want %v", s.RequiredEnvironment, want)
	}
	if want := []string{"KEY_SOURCE", "MCP_ONLY"}; !slices.Equal(s.MCPEnvironment(), want) {
		t.Errorf("MCP-only environment = %v, want %v", s.MCPEnvironment(), want)
	}
	injections := map[string]string{}
	for _, i := range s.Injections {
		injections[i.Target] = i.Text
	}
	for target, text := range map[string]string{
		"harness codex-t environment KEY":   "!secret key",
		"harness codex-t environment TOKEN": "!env WORK_TOKEN",
		"mcp up environment UP_SECRET":      "!secret key",
		"mcp remote header X-Key":           "!secret key",
		"mcp remote auth":                   "oauth (on demand)",
		"mcp up environment UP_ENV":         "!env MCP_ONLY",
	} {
		if injections[target] != text {
			t.Errorf("injection %q = %q, want %q", target, injections[target], text)
		}
	}

	if _, ok := injections["harness codex-t environment PLAIN"]; ok {
		t.Error("a literal value is listed as an injection")
	}

	for name, v := range map[string]any{"snapshot": s, "config": s.Config} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range []string{secretValue, envValue, fileValue} {
			if strings.Contains(string(b), value) {
				t.Errorf("%s JSON contains the resolved value %q", name, value)
			}
		}
	}

	h, err := c.Harness("codex-t")
	if err != nil {
		t.Fatal(err)
	}
	secret := func(id config.ID) (string, error) {
		if id != "key" {
			return "", fmt.Errorf("unexpected secret %s", id)
		}
		return secretValue, nil
	}
	args, env, err := h.Resolve("exec", os.LookupEnv, secret)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"--prompt", fileValue, "--system", fileValue}; !slices.Equal(args, want) {
		t.Errorf("exec args = %q, want %q (trailing newlines dropped)", args, want)
	}
	if env["TOKEN"] != envValue || env["KEY"] != secretValue || env["PLAIN"] != "plain" {
		t.Errorf("env = %v", env)
	}
	args, _, err = h.Resolve("interactive", os.LookupEnv, secret)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"--prompt", fileValue}; !slices.Equal(args, want) {
		t.Errorf("interactive args = %q, want %q", args, want)
	}
}

func TestTagResolutionErrors(t *testing.T) {
	c, _ := tagFixture(t)
	h, err := c.Harness("codex-t")
	if err != nil {
		t.Fatal(err)
	}
	noSecret := func(config.ID) (string, error) { return secretValue, nil }
	unset := func(string) (string, bool) { return "", false }
	_, _, err = h.Resolve("interactive", unset, noSecret)
	wantError(t, err, "harness.yaml:", "harness codex-t environment TOKEN: environment variable WORK_TOKEN is not set")

	failing := func(config.ID) (string, error) { return "", fmt.Errorf("helper failed") }
	_, _, err = h.Resolve("interactive", os.LookupEnv, failing)
	wantError(t, err, "harness codex-t environment KEY: helper failed")
	if strings.Contains(err.Error(), envValue) {
		t.Errorf("error %q contains a resolved value", err)
	}
}

func TestTagPlacement(t *testing.T) {
	for _, tt := range []struct{ name, content, want string }{
		{"unknown tag", "harnesses:\n  codex:\n    environment:\n      A: !foo x\n", "agx.yaml:4: unknown tag !foo; use !env, !file, or !secret"},
		{"secret in harness args", "harnesses:\n  codex:\n    args: [--key, !secret k]\n", "agx.yaml:3: !secret cannot be used in harness args, which other processes can read (ps); set it in the harness environment instead"},
		{"env in an argument group", "harnesses:\n  codex:\n    args:\n      - values: [!env TOKEN]\n", "agx.yaml:4: !env cannot be used in harness args"},
		{"tag in an MCP command", "mcp:\n  c:\n    command: [!env X]\n", "agx.yaml:3: !env is allowed only in"},
		{"tag in an executable", "harnesses:\n  codex:\n    executable: !file x\n", "agx.yaml:3: !file is allowed only in"},
		{"tag in a secret definition", "secrets:\n  s:\n    kind: environment\n    variable: !env X\n", "!env is allowed only in"},
		{"tagged key", "harnesses:\n  codex:\n    environment:\n      !env A: b\n", "!env is allowed only in"},
		{"tagged mapping", "harnesses:\n  codex:\n    environment:\n      A: !secret {x: y}\n", "!secret needs a scalar argument"},
		{"reserved variable", "harnesses:\n  codex:\n    environment:\n      A: !env AGX_X\n", "AGX_ variables are reserved"},
		{"invalid variable", "harnesses:\n  codex:\n    environment:\n      A: !env 1X\n", "!env needs a variable name"},
		{"non-secret tag in auth", "mcp:\n  c:\n    url: 'https://example.com/mcp'\n    auth: !env TOKEN\n", "auth must be a secret kind"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := write(t, filepath.Join(t.TempDir(), "agx.yaml"), tt.content)
			_, err := config.Read(root, builtins)
			wantError(t, err, tt.want)
		})
	}

	for _, tt := range []struct{ name, content, want string }{
		{"unknown secret", "harnesses:\n  codex:\n    environment:\n      A: !secret nope\n", "harness codex environment A: unknown secret nope"},
		{
			name:    "OAuth secret injected",
			content: "secrets:\n  o: {kind: oauth}\nmcp:\n  c:\n    command: [x]\n    environment:\n      A: !secret o\n",
			want:    "OAuth secret o can only be an MCP server's auth",
		},
		{
			name:    "inline auth referenced as a secret",
			content: "mcp:\n  c:\n    url: 'https://example.com/mcp'\n    auth: keychain\n  d:\n    command: [x]\n    environment:\n      A: !secret c\n",
			want:    "secret c is the inline auth of MCP server c",
		},
		{
			name:    "bare secret ID as auth",
			content: "secrets:\n  s: {kind: keychain}\nmcp:\n  c:\n    url: 'https://example.com/mcp'\n    auth: s\n",
			want:    `unknown kind "s"; reference a secret with auth: !secret s`,
		},
		{
			name:    "managed header",
			content: "secrets:\n  s: {kind: keychain}\nmcp:\n  c:\n    url: 'https://example.com/mcp'\n    headers:\n      Authorization: !secret s\n",
			want:    "header Authorization is managed by agx",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := write(t, filepath.Join(t.TempDir(), "agx.yaml"), tt.content)
			_, err := config.Load(root, t.TempDir(), builtins)
			wantError(t, err, tt.want)
		})
	}
}
