package config_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/tarik02-org/agx/internal/config"
)

func TestSopsSecrets(t *testing.T) {
	t.Run("files resolve like !file and keys parse", func(t *testing.T) {
		home := isolateHome(t)
		dir := t.TempDir()
		local := write(t, filepath.Join(dir, "secrets", "app.yaml"), "x")
		inHome := write(t, filepath.Join(home, "env.sops"), "x")
		root := fragment(t, dir, `secrets:
  dotted: {kind: sops, file: ./secrets/app.yaml, key: github.token}
  listed: {kind: sops, file: secrets/app.yaml, key: [a.b, c]}
  whole: {kind: sops, file: ~/env.sops}
mcp:
  remote:
    url: https://example.com/mcp
    auth: {kind: sops, file: ./secrets/app.yaml, key: remote}
`)
		c := load(t, root)
		for id, want := range map[config.ID]struct {
			file string
			key  config.SecretKey
		}{
			"dotted": {local, config.SecretKey{"github", "token"}},
			"listed": {local, config.SecretKey{"a.b", "c"}},
			"whole":  {inHome, nil},
			"remote": {local, config.SecretKey{"remote"}},
		} {
			got := c.Secrets[id]
			if canonical(t, got.File) != canonical(t, want.file) || !slices.Equal(got.Key, want.key) {
				t.Errorf("%s = %s %q, want %s %q", id, got.File, got.Key, want.file, want.key)
			}
		}
		if got := c.Secrets["listed"].Key.Extract(); got != `["a.b"]["c"]` {
			t.Errorf("extract path = %s", got)
		}
	})

	t.Run("package files stay inside the package", func(t *testing.T) {
		root := fragment(t, t.TempDir(), "{}\n")
		dir := mkdir(t, filepath.Join(t.TempDir(), "pkg"))
		write(t, filepath.Join(dir, "agx.package.yaml"), "secrets:\n  s: {kind: sops, file: ../secrets.yaml}\n")
		_, err := read(t, root).AddPackage("team", dir)
		wantError(t, err, "secret team/s: file ../secrets.yaml", "escapes package")
	})

	t.Run("age keys are injected values", func(t *testing.T) {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "secrets.yaml"), "x")
		root := fragment(t, dir, `secrets:
  age: {kind: sops, file: ./secrets.yaml, age_key: !env AGE_IDENTITY}
  github: {kind: sops, file: ./secrets.yaml, key: github.token, age_key: !secret age}
  nested: {kind: sops, file: ./secrets.yaml, age_key: !secret github}
  local: {kind: sops, file: ./secrets.yaml, age_key: !file ./age.txt}
mcp:
  remote:
    url: https://example.com/mcp
    auth: {kind: sops, file: ./secrets.yaml, age_key: !env REMOTE_AGE}
`)
		c := load(t, root)
		if got := c.Secrets["github"].AgeKey; got.Secret != "age" {
			t.Errorf("github age_key = %v", got)
		}
		if got := c.Secrets["local"].AgeKey.File; canonical(t, got) != canonical(t, filepath.Join(dir, "age.txt")) {
			t.Errorf("!file age_key = %s, want it anchored to the fragment", got)
		}
		for v, want := range map[config.Value]string{
			{Secret: "github"}: "AGE_IDENTITY",
			{Secret: "nested"}: "AGE_IDENTITY",
			{Secret: "remote"}: "REMOTE_AGE",
			{Secret: "local"}:  "",
		} {
			if got := c.Variable(v); got != want {
				t.Errorf("variable of %s = %q, want %q", v, got, want)
			}
		}
	})

	for _, tt := range []struct{ name, content, want string }{
		{"age key on another kind", "secrets:\n  s: {kind: keychain, age_key: !env AGE}\n", "secrets.s.age_key: not allowed here"},
		{"unknown age key secret", "secrets:\n  s: {kind: sops, file: ./agx.yaml, age_key: !secret nope}\n", "secret s age_key: unknown secret nope"},
		{
			"oauth age key",
			"secrets:\n  o: {kind: oauth}\n  s: {kind: sops, file: ./agx.yaml, age_key: !secret o}\n",
			"OAuth secret o can only be an MCP server's auth",
		},
		{"self age key", "secrets:\n  s: {kind: sops, file: ./agx.yaml, age_key: !secret s}\n", "age_key forms a cycle: s -> s"},
		{
			"age key cycle",
			"secrets:\n  a: {kind: sops, file: ./agx.yaml, age_key: !secret b}\n  b: {kind: sops, file: ./agx.yaml, age_key: !secret a}\n",
			"secret a age_key: age_key forms a cycle: a -> b -> a",
		},
		{"tag elsewhere in a secret", "secrets:\n  s: {kind: sops, file: !file ./agx.yaml}\n", "!file is allowed only in"},

		{"missing file", "secrets:\n  s: {kind: sops, file: ./nope.yaml}\n", "secret s: cannot read sops file"},
		{"no file", "secrets:\n  s: {kind: sops}\n", "secrets.s: missing file"},
		{"inline without file", "mcp:\n  c:\n    url: https://example.com/mcp\n    auth: {kind: sops}\n", "mcp.c.auth: missing file"},
		{"key on another kind", "secrets:\n  s: {kind: keychain, key: a}\n", "secrets.s.key: not allowed here"},
		{"file on another kind", "secrets:\n  s: {kind: keychain, file: ./agx.yaml}\n", "secrets.s.file: not allowed here"},
		{"empty segment", "secrets:\n  s: {kind: sops, file: ./agx.yaml, key: a..b}\n", "secrets.s.key: \"a..b\" is not valid here"},
		{"number segment", "secrets:\n  s: {kind: sops, file: ./agx.yaml, key: [a, 0]}\n", "secrets.s.key[1]: expected a string, not a number"},
		{"unsplittable segment", "secrets:\n  s: {kind: sops, file: ./agx.yaml, key: ['a][b']}\n", "cannot contain ]["},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := fragment(t, t.TempDir(), tt.content)
			_, err := config.Load(root, config.Options{Builtins: builtins, Cache: t.TempDir()})
			wantError(t, err, tt.want)
		})
	}
}
