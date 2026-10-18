package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/adapters"
	"github.com/tarik02-org/agx/internal/config"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestHome(t *testing.T) {
	user := t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("USERPROFILE", user)
	t.Setenv("CODEX_HOME", filepath.Join(user, "daemon"))
	caller := func(value string) func(string) (string, bool) {
		return func(name string) (string, bool) {
			if name != "CODEX_HOME" {
				return "", false
			}
			return value, value != ""
		}
	}
	for _, test := range []struct {
		name   string
		target adapters.Target
		want   string
	}{
		{"daemon", adapters.Target{}, filepath.Join(user, "daemon")},
		{"caller", adapters.Target{Lookup: caller(filepath.Join(user, "caller"))}, filepath.Join(user, "caller")},
		// A caller without CODEX_HOME runs Codex in its default home, whatever the daemon has.
		{"caller unset", adapters.Target{Lookup: caller("")}, filepath.Join(user, ".codex")},
		{
			"definition",
			adapters.Target{
				Env:    map[string]string{"CODEX_HOME": filepath.Join(user, "definition")},
				Lookup: caller(filepath.Join(user, "caller")),
			},
			filepath.Join(user, "definition"),
		},
	} {
		got, err := home(test.target)
		if err != nil || got != test.want {
			t.Errorf("%s: home = %q, %v; want %q", test.name, got, err, test.want)
		}
	}
}

func TestPluginsDisabled(t *testing.T) {
	for _, test := range []struct {
		name     string
		config   string
		profiles map[string]string
		args     []string
		// cause is in the explanation when plugins are disabled, "" when they are enabled.
		cause string
	}{
		{name: "no configuration"},
		{name: "config.toml", config: "[features]\nplugins = false\n", cause: "features.plugins = false in "},
		{name: "config.toml enables", config: "[features]\nplugins = true\n"},
		{
			name:   "definition override",
			config: "[features]\nplugins = false\n",
			args:   []string{"-c", "features.plugins=true"},
		},
		{
			name:   "user override after the subcommand",
			config: "[features]\nplugins = false\n",
			args:   []string{"--definition", "exec", "-c", "features.plugins = true", "hi"},
		},
		{name: "--config=", config: "[features]\nplugins = false\n", args: []string{"--config=features.plugins=true"}},
		{name: "attached -c", config: "[features]\nplugins = false\n", args: []string{"-cfeatures.plugins=true"}},
		{name: "inline table", config: "[features]\nplugins = false\n", args: []string{"-c", "features={plugins=true}"}},
		{
			name:  "override disables",
			args:  []string{"-c", "features.plugins=false"},
			cause: "features.plugins = false from -c features.plugins=false",
		},
		{name: "last override wins", args: []string{"-c", "features.plugins=false", "-c", "features.plugins=true"}},
		{
			name:  "last override disables",
			args:  []string{"-c", "features.plugins=true", "-c", "features.plugins=false"},
			cause: "from -c",
		},
		{name: "--enable", config: "[features]\nplugins = false\n", args: []string{"--enable", "plugins"}},
		{name: "--enable over -c", args: []string{"--enable=plugins", "-c", "features.plugins=false"}},
		{name: "--disable", args: []string{"--disable", "plugins"}, cause: "--disable plugins"},
		{name: "--disable wins", args: []string{"--disable", "plugins", "--enable", "plugins"}, cause: "--disable plugins"},
		{name: "other features", args: []string{"--disable", "web_search", "-c", "plugins.x.enabled=false"}},
		{name: "not TOML", args: []string{"-c", "features.plugins=off"}},
		{
			name:     "profile file",
			config:   "[features]\nplugins = false\n",
			profiles: map[string]string{"work": "[features]\nplugins = true\n"},
			args:     []string{"-p", "work"},
		},
		{
			name:     "profile file disables",
			profiles: map[string]string{"work": "[features]\nplugins = false\n"},
			args:     []string{"--profile=work"},
			cause:    "work.config.toml",
		},
		{
			name:     "override over profile",
			profiles: map[string]string{"work": "[features]\nplugins = false\n"},
			args:     []string{"--profile", "work", "-c", "features.plugins=true"},
		},
		{
			name:   "legacy profile table",
			config: "[features]\nplugins = false\n[profiles.work.features]\nplugins = true\n",
			args:   []string{"-p", "work"},
		},
		{
			name:   "legacy default profile",
			config: "profile = \"work\"\n[features]\nplugins = true\n[profiles.work.features]\nplugins = false\n",
			cause:  "in profile work",
		},
		{
			name:   "arguments after --",
			config: "[features]\nplugins = false\n",
			args:   []string{"exec", "--", "-c", "features.plugins=true"},
			cause:  "features.plugins = false in ",
		},
		{
			name:   "flag value",
			config: "[features]\nplugins = false\n",
			args:   []string{"-m", "-c", "features.plugins=true"},
			cause:  "features.plugins = false in ",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if test.config != "" {
				writeFile(t, filepath.Join(dir, "config.toml"), test.config)
			}
			for name, content := range test.profiles {
				writeFile(t, filepath.Join(dir, name+".config.toml"), content)
			}
			got, err := pluginsDisabled(dir, test.args)
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case test.cause == "" && got != "":
				t.Errorf("plugins disabled: %s", got)
			case test.cause != "" && !strings.Contains(got, test.cause):
				t.Errorf("explanation %q lacks %q", got, test.cause)
			}
		})
	}
}

// Diagnostics read the home and arguments of the definition they check.
func TestPluginsForTarget(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "codex", "config.toml"), "[features]\nplugins = false\n")
	writeFile(t, filepath.Join(dir, "skills", "review", "SKILL.md"), "---\nname: review\ndescription: r\n---\nR.\n")
	c := &config.Config{
		Skills: map[config.ID]config.Skill{
			"review": {Path: filepath.Join(dir, "skills", "review")},
			"unused": {Path: filepath.Join(dir, "skills", "review")},
		},
		Scopes: []config.Scope{{Add: config.Selection{Skills: []config.ID{"review"}}}},
	}
	t.Setenv("CODEX_HOME", t.TempDir())
	target := adapters.Target{Harness: "codex-work", Env: map[string]string{"CODEX_HOME": filepath.Join(dir, "codex")}}
	missing := filepath.Join(dir, "missing-codex")
	state, err := Plugins(t.Context(), missing, c, filepath.Join(dir, "cache"), target)
	if err != nil || state.Home != filepath.Join(dir, "codex") || !strings.Contains(state.Disabled, state.Home) {
		t.Fatalf("disabled = %+v, %v", state, err)
	}
	target.Arguments = []string{"-c", "features.plugins=true"}
	state, err = Plugins(t.Context(), missing, c, filepath.Join(dir, "cache"), target)
	if err != nil || state.Disabled != "" || len(state.Missing) != 1 || state.Registered {
		t.Fatalf("enabled = %+v, %v", state, err)
	}
}
