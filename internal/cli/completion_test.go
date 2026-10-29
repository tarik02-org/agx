package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const completionConfig = `
instructions:
  conventions:
    text: Use pnpm.
mcp:
  github:
    url: https://api.githubcopilot.com/mcp
  fs:
    command: [npx, server-filesystem]
  docs:
    url: https://docs.example/mcp
    auth: none
secrets:
  token:
    kind: keychain
bundles:
  base:
    mcp: [fs]
harnesses:
  codex-work:
    base: codex
`

// isolate points every agx path and home directory into a temporary directory, with the
// configuration given in a fragment, and returns the fragment's path.
func isolate(t *testing.T, configuration string) string {
	t.Helper()
	dir := t.TempDir()
	root, fragment := writeConfig(t, filepath.Join(dir, "config"), configuration)
	for name, value := range map[string]string{
		"AGX_CONFIG":      root,
		"AGX_CACHE":       filepath.Join(dir, "cache"),
		"AGX_STATE":       filepath.Join(dir, "state"),
		"LOCALAPPDATA":    filepath.Join(dir, "home", "localappdata"),
		"XDG_DATA_HOME":   filepath.Join(dir, "home", "data"),
		"HOME":            filepath.Join(dir, "home"),
		"XDG_CONFIG_HOME": filepath.Join(dir, "home", ".config"),
		"XDG_CACHE_HOME":  filepath.Join(dir, "home", ".cache"),
		"XDG_STATE_HOME":  filepath.Join(dir, "home", ".local", "state"),
	} {
		t.Setenv(name, value)
	}
	return fragment
}

// writeConfig writes a root configuration in dir importing agx.d/*.yaml, and the definitions
// into agx.d/main.yaml unless they are empty. It returns both paths.
func writeConfig(t *testing.T, dir, definitions string) (string, string) {
	t.Helper()
	root, fragment := filepath.Join(dir, "agx.yaml"), filepath.Join(dir, "agx.d", "main.yaml")
	if err := os.MkdirAll(filepath.Dir(fragment), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root, []byte("fragments: [agx.d/*.yaml]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if definitions != "" {
		if err := os.WriteFile(fragment, []byte(definitions), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root, fragment
}

// complete runs agx __complete with args and returns the candidates and the directive.
func complete(t *testing.T, args ...string) ([]string, cobra.ShellCompDirective) {
	t.Helper()
	var out bytes.Buffer
	if code := run(append([]string{"agx", cobra.ShellCompRequestCmd}, args...), &out); code != 0 {
		t.Fatalf("agx __complete %v exited %d", args, code)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	last := lines[len(lines)-1]
	directive, err := strconv.Atoi(strings.TrimPrefix(last, ":"))
	if err != nil || !strings.HasPrefix(last, ":") {
		t.Fatalf("no directive in %q", out.String())
	}
	return lines[:len(lines)-1], cobra.ShellCompDirective(directive)
}

func TestCompletion(t *testing.T) {
	isolate(t, completionConfig)
	for _, test := range []struct {
		args      []string
		want      []string
		directive cobra.ShellCompDirective
	}{
		{
			[]string{"mcp", "remove", ""},
			[]string{"docs\thttps://docs.example/mcp", "fs\tnpx server-filesystem", "github\thttps://api.githubcopilot.com/mcp"},
			cobra.ShellCompDirectiveNoFileComp,
		},
		{[]string{"mcp", "remove", "fs", ""}, []string{}, cobra.ShellCompDirectiveNoFileComp},
		{
			[]string{"use", "--kind", "mcp", ""},
			[]string{"docs\tMCP server", "fs\tMCP server", "github\tMCP server"},
			cobra.ShellCompDirectiveNoFileComp,
		},
		{[]string{"unuse", "b"}, []string{"base\tbundle"}, cobra.ShellCompDirectiveNoFileComp},
		{[]string{"instructions", "remove", ""}, []string{"conventions\ttext"}, cobra.ShellCompDirectiveNoFileComp},
		{[]string{"secret", "set", ""}, []string{"token\tkeychain"}, cobra.ShellCompDirectiveNoFileComp},
		{[]string{"secret", "login", ""}, []string{"github\toauth, on demand"}, cobra.ShellCompDirectiveNoFileComp},
		{[]string{"harness", "remove", ""}, []string{"codex-work\tbase codex"}, cobra.ShellCompDirectiveNoFileComp},
		{[]string{"run", "codex-"}, []string{"codex-work\tbase codex"}, cobra.ShellCompDirectiveNoFileComp},
		{[]string{"run", "codex-work", ""}, []string{}, cobra.ShellCompDirectiveDefault},
		{[]string{"codex-work", ""}, []string{}, cobra.ShellCompDirectiveDefault},
		{
			[]string{"use", "fs", "--harness", "codex,codex-"},
			[]string{"codex,codex-work\tbase codex"},
			cobra.ShellCompDirectiveNoFileComp,
		},
		{[]string{"explain", "--harness", "codex-"}, []string{"codex-work\tbase codex"}, cobra.ShellCompDirectiveNoFileComp},
		{
			[]string{"use", "fs", "--scope", ""},
			[]string{"user\teverywhere", "here\tthis directory"},
			cobra.ShellCompDirectiveNoFileComp,
		},
		{[]string{"use", "fs", "--scope", "./"}, []string{}, cobra.ShellCompDirectiveFilterDirs},
		{[]string{"use", "fs", "--file", ""}, []string{"yaml", "yml"}, cobra.ShellCompDirectiveFilterFileExt},
		{[]string{"skill", "add", ""}, []string{}, cobra.ShellCompDirectiveDefault},
		{[]string{"sessions", ""}, []string{}, cobra.ShellCompDirectiveNoFileComp},
		{[]string{"daemon", "re"}, []string{"restart\tStop the daemon and start one of this build"}, cobra.ShellCompDirectiveNoFileComp},
		{[]string{"daemon", "restart", "--timeout", "1"}, []string{"10s", "1m"}, cobra.ShellCompDirectiveNoFileComp},
		// No daemon runs, and completion must not start one.
		{[]string{"launch", "show", ""}, []string{}, cobra.ShellCompDirectiveNoFileComp},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			got, directive := complete(t, test.args...)
			if !slices.Equal(got, test.want) {
				t.Errorf("candidates = %q, want %q", got, test.want)
			}
			if directive != test.directive {
				t.Errorf("directive = %d, want %d", directive, test.directive)
			}
		})
	}
}

func TestCompletionRootFlags(t *testing.T) {
	isolate(t, "")
	other, _ := writeConfig(t, t.TempDir(), completionConfig)
	for _, args := range [][]string{
		{"--config", other, "secret", "login", ""},
		{"--config=" + other, "secret", "login", ""},
		{"secret", "login", "--config", other, ""},
	} {
		if got, _ := complete(t, args...); !slices.Equal(got, []string{"github\toauth, on demand"}) {
			t.Errorf("%q: candidates = %q", args, got)
		}
	}
	// The harness command exists only in the configuration --config names.
	if _, directive := complete(t, "--config", other, "codex-work", ""); directive != cobra.ShellCompDirectiveDefault {
		t.Errorf("harness command from --config: directive = %d", directive)
	}
	if got, directive := complete(
		t,
		"--config",
		"",
	); len(got) != 2 ||
		directive != cobra.ShellCompDirectiveFilterFileExt {
		t.Errorf("--config value: %q, %d", got, directive)
	}
}

func TestCompletionBrokenConfig(t *testing.T) {
	isolate(t, "mcp: [unclosed\n")
	for _, args := range [][]string{
		{"mcp", "remove", ""},
		{"use", ""},
		{"run", ""},
		{"secret", "login", ""},
	} {
		got, directive := complete(t, args...)
		if len(got) != 0 || directive != cobra.ShellCompDirectiveNoFileComp {
			t.Errorf("%q: candidates = %q, directive = %d", args, got, directive)
		}
	}
}

func TestCompletionKeepsStderrQuiet(t *testing.T) {
	isolate(t, "")
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = stderr
	complete(t, "daemon", "")
	os.Stderr = saved
	// Windows cannot remove the temporary directory while the file is open.
	if err := stderr.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(stderr.Name())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "Completion ended with directive") {
		t.Fatalf("completion wrote its directive to stderr: %q", b)
	}
}
