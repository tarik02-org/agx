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
      - values: [--from-more]
    environment:
      MORE: "yes"
`)
	root := fragment(t, dir, `harnesses:
  codex:
    executable: '`+filepath.ToSlash(codex)+`'
  codex-cpa:
    base: codex
    args:
      - values: [-c, 'model_provider="cpa"']
      - values: [--profile, cpa]
        modes: [interactive, exec, resume]
      - values: [--pt-flag]
        modes: [passthrough]
    environment:
      CPA_KEY: literal
  pi-flat:
    base: pi
    args: [--flat-a, --flat-b]
`, "more.yaml")
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

	if got := c.HarnessNames(); !slices.Equal(
		got,
		[]string{"claude", "codex", "codex-cpa", "opencode", "pi", "pi-flat"},
	) {
		t.Errorf("harness names = %v", got)
	}
}

func TestHarnessBaseChain(t *testing.T) {
	dir := t.TempDir()
	c2 := filepath.Join(dir, "c2")
	root := fragment(t, dir, `harnesses:
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
	root := fragment(t, dir, `harnesses:
  codex: {hidden: true}
  codex-cpa: {base: codex}
  codex-old: {base: codex, hidden: true}
`, "more.yaml")
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

func TestHarnessDisplayNameAndEnabled(t *testing.T) {
	root := fragment(t, t.TempDir(), `harnesses:
  codex: {enabled: false, display_name: OpenAI Codex}
  codex-cpa: {base: codex, display_name: Codex CPA}
  codex-old: {base: codex-cpa, enabled: false}
  claude: {enabled: true}
`)
	c := read(t, root)
	for name, want := range map[string]struct {
		display string
		enabled bool
	}{
		"codex": {"OpenAI Codex", false},
		// Neither field comes from the base: a disabled or renamed base leaves its variants as they are.
		"codex-cpa": {"Codex CPA", true},
		"codex-old": {"codex-old", false},
		"claude":    {"claude", true},
		"pi":        {"pi", true},
	} {
		h, err := c.Harness(name)
		if err != nil {
			t.Fatal(err)
		}
		if h.DisplayName != want.display || h.Enabled != want.enabled {
			t.Errorf(
				"%s = %q enabled %v, want %q enabled %v",
				name,
				h.DisplayName,
				h.Enabled,
				want.display,
				want.enabled,
			)
		}
	}
	if got := c.HarnessNames(); !slices.Contains(got, "codex") {
		t.Errorf("disabled harness left the harness names: %v", got)
	}
}

func TestHarnessOverride(t *testing.T) {
	dir := t.TempDir()
	write(
		t,
		filepath.Join(dir, "one.yaml"),
		absolute(dir, "harnesses:\n  codex:\n    executable: ABS/a\n    args: [--a]\n"),
	)
	root := fragment(t, dir, absolute(dir, `harnesses:
  codex:
    override: true
    executable: ABS/b
`), "one.yaml")
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
				"one.yaml":  "harnesses:\n  codex:\n    executable: ABS/a\n",
				"main.yaml": "harnesses:\n  codex:\n    executable: ABS/b\n",
			},
			want: []string{"main.yaml:3", "harness codex: executable already set at", "one.yaml:3"},
		},
		{
			name: "environment key set in two fragments",
			files: map[string]string{
				"one.yaml":  "harnesses:\n  codex:\n    environment: {A: '1'}\n",
				"main.yaml": "harnesses:\n  codex:\n    environment: {A: '2'}\n",
			},
			want: []string{"environment.A already set at", "one.yaml"},
		},
		{
			name: "hidden set in two fragments",
			files: map[string]string{
				"one.yaml":  "harnesses:\n  codex: {hidden: true}\n",
				"main.yaml": "harnesses:\n  codex: {hidden: false}\n",
			},
			want: []string{"harness codex: hidden already set at", "one.yaml"},
		},
		{
			name: "display name set in two fragments",
			files: map[string]string{
				"one.yaml":  "harnesses:\n  codex: {display_name: One}\n",
				"main.yaml": "harnesses:\n  codex: {display_name: Two}\n",
			},
			want: []string{"harness codex: display_name already set at", "one.yaml"},
		},
		{
			name: "enabled set in two fragments",
			files: map[string]string{
				"one.yaml":  "harnesses:\n  codex: {enabled: false}\n",
				"main.yaml": "harnesses:\n  codex: {enabled: true}\n",
			},
			want: []string{"harness codex: enabled already set at", "one.yaml"},
		},
		{
			name:  "display name over several lines",
			files: map[string]string{"main.yaml": "harnesses:\n  codex: {display_name: \"Codex\\nCPA\"}\n"},
			want:  []string{"main.yaml:2", "harness codex: display_name must be one line"},
		},
		{"base cycle", map[string]string{"main.yaml": "harnesses:\n  a: {base: b}\n  b: {base: a}\n"}, []string{"harness base cycle: a -> b -> a"}},
		{"unknown base", map[string]string{"main.yaml": "harnesses:\n  a: {base: nope}\n"}, []string{`harness a: unknown base "nope"`}},
		{"missing base", map[string]string{"main.yaml": "harnesses:\n  a: {environment: {X: '1'}}\n"}, []string{"harness a needs a base"}},
		{"shadowed command", map[string]string{"main.yaml": "harnesses:\n  install: {base: codex}\n"}, []string{"harness install would shadow the agx install command"}},
		{"built-in with a base", map[string]string{"main.yaml": "harnesses:\n  claude: {base: codex}\n"}, []string{"built-in harness claude cannot set base"}},
		{
			name:  "mode of another built-in",
			files: map[string]string{"main.yaml": "harnesses:\n  c2: {base: claude}\n  c3:\n    base: c2\n    args: [{values: [-x], modes: [exec]}]\n"},
			want:  []string{`claude has no mode "exec"`},
		},
		{"empty modes", map[string]string{"main.yaml": "harnesses:\n  codex:\n    args: [{values: [-x], modes: []}]\n"}, []string{"modes: must not be empty"}},
		{"mixed args", map[string]string{"main.yaml": "harnesses:\n  codex:\n    args: [--a, {values: [--b]}]\n"}, []string{"args[0]: expected a mapping, not a string"}},
		{"relative executable", map[string]string{"main.yaml": "harnesses:\n  codex: {executable: bin/codex}\n"}, []string{"executable must be absolute"}},
		{"reserved environment", map[string]string{"main.yaml": "harnesses:\n  codex:\n    environment: {AGX_MCP_GRANT: x}\n"}, []string{`environment: key "AGX_MCP_GRANT" is not allowed`}},
		{"non-string environment value", map[string]string{"main.yaml": "harnesses:\n  codex:\n    environment: {X: 1}\n"}, []string{"environment.X: expected a string, not a number"}},
		{"unknown scope filter", map[string]string{"main.yaml": "scopes:\n  - harnesses: [nope]\n"}, []string{`scope: unknown harness "nope"`}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			before := []string{}
			for name, content := range tt.files {
				write(t, filepath.Join(dir, name), absolute(dir, content))
				if name != "main.yaml" {
					before = append(before, name)
				}
			}
			root := fragment(t, dir, absolute(dir, tt.files["main.yaml"]), before...)
			_, err := config.Load(root, config.Options{Builtins: builtins})
			wantError(t, err, tt.want...)
		})
	}

	t.Run("unknown resource in a harness selection", func(t *testing.T) {
		root := fragment(t, t.TempDir(), "harnesses:\n  codex:\n    add: {skills: [nope]}\n")
		_, err := config.Load(root, config.Options{Builtins: builtins, Cache: t.TempDir()})
		wantError(t, err, `harness codex: unknown skill "nope"`)
	})
}
