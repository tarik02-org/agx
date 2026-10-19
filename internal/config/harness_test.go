package config_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/tarik02-org/agx/internal/config"
)

func literals(values []config.Value) []string {
	out := []string{}
	for _, v := range values {
		out = append(out, v.String())
	}
	return out
}

func TestHarnessMerge(t *testing.T) {
	dir := t.TempDir()
	codex := filepath.Join(dir, "bin", "codex")
	write(t, filepath.Join(dir, "more.yaml"), `harnesses:
  codex-cpa:
    args:
      - [--from-more]
    environment:
      MORE: "yes"
`)
	root := write(t, filepath.Join(dir, "agx.yaml"), `imports: [{path: more.yaml}]
harnesses:
  codex:
    executable: '`+filepath.ToSlash(codex)+`'
  codex-cpa:
    base: codex
    args:
      - [-c, 'model_provider="cpa"']
      - values: [--profile, cpa]
        modes: [interactive, exec, resume]
      - values: [--pt-flag]
        modes: [passthrough]
    environment:
      CPA_KEY: literal
  pi-flat:
    base: pi
    args: [--flat-a, --flat-b]
`)
	c := read(t, root)
	h, err := c.Harness("codex-cpa")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.Chain, []string{"codex", "codex-cpa"}) || h.Builtin != "codex" {
		t.Fatalf("chain = %v, builtin %s", h.Chain, h.Builtin)
	}
	if h.Executable != codex {
		t.Errorf("executable = %q, want %q inherited from codex", h.Executable, codex)
	}
	for _, tt := range []struct {
		mode string
		want []string
	}{
		{"exec", []string{"--from-more", "-c", `model_provider="cpa"`, "--profile", "cpa"}},
		{"app-server", []string{"--from-more", "-c", `model_provider="cpa"`}},
		{"passthrough", []string{"--pt-flag"}},
	} {
		if got := literals(h.Arguments(tt.mode)); !slices.Equal(got, tt.want) {
			t.Errorf("Arguments(%s) = %q, want %q", tt.mode, got, tt.want)
		}
	}
	if h.Environment["MORE"].Literal != "yes" || h.Environment["CPA_KEY"].Literal != "literal" {
		t.Errorf("env = %v", h.Environment)
	}

	flat, err := c.Harness("pi-flat")
	if err != nil {
		t.Fatal(err)
	}
	if got := literals(flat.Arguments("print")); !slices.Equal(got, []string{"--flat-a", "--flat-b"}) {
		t.Errorf("flat args in print = %q", got)
	}
	if got := flat.Arguments("passthrough"); len(got) != 0 {
		t.Errorf("flat args apply to passthrough: %v", got)
	}

	if got := c.HarnessNames(); !slices.Equal(got, []string{"claude", "codex", "codex-cpa", "pi", "pi-flat"}) {
		t.Errorf("harness names = %v", got)
	}
}

func TestHarnessBaseChain(t *testing.T) {
	dir := t.TempDir()
	c2 := filepath.Join(dir, "c2")
	root := write(t, filepath.Join(dir, "agx.yaml"), `harnesses:
  c2:
    base: claude
    executable: '`+filepath.ToSlash(c2)+`'
    args: [--two]
    environment: {A: two, B: two}
  c3:
    base: c2
    args: [--three]
    environment: {B: three}
`)
	h, err := read(t, root).Harness("c3")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.Chain, []string{"claude", "c2", "c3"}) {
		t.Fatalf("chain = %v", h.Chain)
	}
	if h.Executable != c2 {
		t.Errorf("executable = %q", h.Executable)
	}
	if got := literals(h.Arguments("interactive")); !slices.Equal(got, []string{"--two", "--three"}) {
		t.Errorf("args = %q, want base groups first", got)
	}
	if h.Environment["A"].Literal != "two" || h.Environment["B"].Literal != "three" {
		t.Errorf("env = %v, want extensions to replace per key", h.Environment)
	}
}

func TestHarnessHidden(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "more.yaml"), "harnesses:\n  claude: {hidden: false}\n")
	root := write(t, filepath.Join(dir, "agx.yaml"), `imports: [{path: more.yaml}]
harnesses:
  codex: {hidden: true}
  codex-cpa: {base: codex}
  codex-old: {base: codex, hidden: true}
`)
	c := read(t, root)
	for name, want := range map[string]bool{
		"codex":     true,
		"codex-cpa": false, // hiding a base does not hide definitions based on it
		"codex-old": true,
		"claude":    false,
		"pi":        false,
	} {
		h, err := c.Harness(name)
		if err != nil {
			t.Fatal(err)
		}
		if h.Hidden != want {
			t.Errorf("%s hidden = %v, want %v", name, h.Hidden, want)
		}
	}
	if got := c.HarnessNames(); !slices.Contains(got, "codex") {
		t.Errorf("hidden harness left the harness names: %v", got)
	}
}

func TestHarnessOverride(t *testing.T) {
	dir := t.TempDir()
	write(
		t,
		filepath.Join(dir, "one.yaml"),
		absolute(dir, "harnesses:\n  codex:\n    executable: ABS/a\n    args: [--a]\n"),
	)
	root := write(t, filepath.Join(dir, "agx.yaml"), absolute(dir, `imports: [{path: one.yaml}]
harnesses:
  codex:
    override: true
    executable: ABS/b
`))
	h, err := read(t, root).Harness("codex")
	if err != nil {
		t.Fatal(err)
	}
	if h.Executable != filepath.Join(dir, "b") || len(h.Args) != 0 {
		t.Fatalf("override kept the earlier definition: %+v", h)
	}
}

func TestHarnessErrors(t *testing.T) {
	for _, tt := range []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name: "executable set in two fragments",
			files: map[string]string{
				"one.yaml": "harnesses:\n  codex:\n    executable: ABS/a\n",
				"agx.yaml": "imports: [{path: one.yaml}]\nharnesses:\n  codex:\n    executable: ABS/b\n",
			},
			want: []string{"agx.yaml:4", "harness codex: executable already set at", "one.yaml:3"},
		},
		{
			name: "environment key set in two fragments",
			files: map[string]string{
				"one.yaml": "harnesses:\n  codex:\n    environment: {A: '1'}\n",
				"agx.yaml": "imports: [{path: one.yaml}]\nharnesses:\n  codex:\n    environment: {A: '2'}\n",
			},
			want: []string{"environment.A already set at", "one.yaml"},
		},
		{
			name: "hidden set in two fragments",
			files: map[string]string{
				"one.yaml": "harnesses:\n  codex: {hidden: true}\n",
				"agx.yaml": "imports: [{path: one.yaml}]\nharnesses:\n  codex: {hidden: false}\n",
			},
			want: []string{"harness codex: hidden already set at", "one.yaml"},
		},
		{"base cycle", map[string]string{"agx.yaml": "harnesses:\n  a: {base: b}\n  b: {base: a}\n"}, []string{"harness base cycle: a -> b -> a"}},
		{"unknown base", map[string]string{"agx.yaml": "harnesses:\n  a: {base: nope}\n"}, []string{`harness a: unknown base "nope"`}},
		{"missing base", map[string]string{"agx.yaml": "harnesses:\n  a: {environment: {X: '1'}}\n"}, []string{"harness a needs a base"}},
		{"shadowed command", map[string]string{"agx.yaml": "harnesses:\n  install: {base: codex}\n"}, []string{"harness install would shadow the agx install command"}},
		{"built-in with a base", map[string]string{"agx.yaml": "harnesses:\n  claude: {base: codex}\n"}, []string{"built-in harness claude cannot set base"}},
		{
			name:  "mode of another built-in",
			files: map[string]string{"agx.yaml": "harnesses:\n  c2: {base: claude}\n  c3:\n    base: c2\n    args: [{values: [-x], modes: [exec]}]\n"},
			want:  []string{`claude has no mode "exec"`},
		},
		{"empty modes", map[string]string{"agx.yaml": "harnesses:\n  codex:\n    args: [{values: [-x], modes: []}]\n"}, []string{"modes must not be empty"}},
		{"mixed args", map[string]string{"agx.yaml": "harnesses:\n  codex:\n    args: [--a, [--b]]\n"}, []string{"args mixes strings with argument groups"}},
		{"relative executable", map[string]string{"agx.yaml": "harnesses:\n  codex: {executable: bin/codex}\n"}, []string{"executable must be absolute"}},
		{"reserved environment", map[string]string{"agx.yaml": "harnesses:\n  codex:\n    environment: {AGX_MCP_GRANT: x}\n"}, []string{`invalid environment name "AGX_MCP_GRANT"`}},
		{"non-string environment value", map[string]string{"agx.yaml": "harnesses:\n  codex:\n    environment: {X: 1}\n"}, []string{"value must be a string"}},
		{"unknown scope filter", map[string]string{"agx.yaml": "scopes:\n  - harnesses: [nope]\n"}, []string{`scope: unknown harness "nope"`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				write(t, filepath.Join(dir, name), absolute(dir, content))
			}
			_, err := config.Read(filepath.Join(dir, "agx.yaml"), builtins)
			wantError(t, err, tt.want...)
		})
	}

	t.Run("unknown resource in a harness selection", func(t *testing.T) {
		root := write(t, filepath.Join(t.TempDir(), "agx.yaml"), "harnesses:\n  codex:\n    add: {skills: [nope]}\n")
		_, err := config.Load(root, t.TempDir(), builtins)
		wantError(t, err, `harness codex: unknown skill "nope"`)
	})
}
