package codex

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

// fakeCodex is a Codex that logs its arguments to $CODEX_HOME/log and lists plugins disabled
// when config.toml or its last plugins override disables them.
func fakeCodex(t *testing.T) string {
	if runtime.GOOS == "windows" {
		t.Skip("the fake Codex is a shell script")
	}
	path := filepath.Join(t.TempDir(), "codex")
	writeFile(t, path, `#!/bin/sh
echo "$@" >> "$CODEX_HOME/log"
state=true
grep -q 'plugins = false' "$CODEX_HOME/config.toml" 2>/dev/null && state=false
for arg; do
	case $arg in
	--config=features.plugins=*) state=${arg#--config=features.plugins=} ;;
	--disable=plugins) state=false ;;
	esac
done
printf 'apps     stable  true\nplugins  stable  %s\n' "$state"
`)
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// Codex decides from the configuration options of the arguments, once per configuration.
func TestPluginsDisabled(t *testing.T) {
	codex, dir := fakeCodex(t), t.TempDir()
	target := adapters.Target{Env: map[string]string{"CODEX_HOME": dir}}
	ask := func(args ...string) string {
		target.Arguments = args
		return pluginsDisabled(t.Context(), codex, dir, target)
	}
	if got := ask("-m", "-c", "exec", "-cfeatures.plugins=false", "--", "-c", "x"); !strings.Contains(got, "disabled") {
		t.Errorf("override: %q", got)
	}
	writeFile(t, filepath.Join(dir, "work.config.toml"), "[features]\nplugins = false\n")
	if got := ask("--profile=work", "--enable", "plugins"); got == "" {
		t.Error("profile: enabled")
	}
	if got := ask("-p", "work", "--config", "features.plugins=true"); got != "" {
		t.Errorf("override over profile: %q", got)
	}
	ask("-p", "work", "--config", "features.plugins=true")
	log, _ := os.ReadFile(filepath.Join(dir, "log"))
	want := "--config=features.plugins=false features list\n" +
		"--config=features.plugins=false --enable=plugins features list\n" +
		"--config=features.plugins=false --config=features.plugins=true features list\n"
	if string(log) != want {
		t.Errorf("Codex ran with\n%s\nwant\n%s", log, want)
	}
	// A changed profile asks again.
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "work.config.toml"), later, later); err != nil {
		t.Fatal(err)
	}
	ask("-p", "work", "--config", "features.plugins=true")
	if again, _ := os.ReadFile(filepath.Join(dir, "log")); strings.Count(string(again), "\n") != 4 {
		t.Errorf("after the profile changed, Codex ran with\n%s", again)
	}
	// Without Codex, plugins count as enabled.
	if got := pluginsDisabled(t.Context(), filepath.Join(dir, "missing"), dir, target); got != "" {
		t.Errorf("missing Codex: %q", got)
	}
}

// Diagnostics read the home and arguments of the definition they check.
func TestPluginsForTarget(t *testing.T) {
	dir, codex := t.TempDir(), fakeCodex(t)
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
	state, err := Plugins(t.Context(), codex, c, filepath.Join(dir, "cache"), target)
	if err != nil || state.Home != filepath.Join(dir, "codex") || !strings.Contains(state.Disabled, state.Home) {
		t.Fatalf("disabled = %+v, %v", state, err)
	}
	target.Arguments = []string{"-c", "features.plugins=true"}
	state, err = Plugins(t.Context(), codex, c, filepath.Join(dir, "cache"), target)
	if err != nil || state.Disabled != "" || len(state.Missing) != 1 || state.Registered {
		t.Fatalf("enabled = %+v, %v", state, err)
	}
}
