//go:build e2e

package e2e

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestCodexPlugins installs skill plugins into the CODEX_HOME each launch runs Codex with, and
// enables them when the launch's arguments override a configuration that disables plugins.
func TestCodexPlugins(t *testing.T) {
	e := newEnv(t)
	home := func(name string) string { return filepath.Join(e.dir, name) }
	e.write("disabled-codex/config.toml", "[features]\nplugins = false\n")
	e.write("override-codex/config.toml", "[features]\nplugins = false\n")
	quoted := func(path string) string { return "'" + filepath.ToSlash(path) + "'" }
	e.write("config/agx.yaml", `skills:
  review: {path: ./skills/review}
scopes:
  - directory: ../work
    add:
      skills: [review]
harnesses:
  codex:
    executable: `+quoted(binaries.harness)+`
  codex-defined:
    base: codex
    env: {CODEX_HOME: `+quoted(home("defined-codex"))+`}
  codex-override:
    base: codex
    env: {CODEX_HOME: `+quoted(home("override-codex"))+`}
    args: [-c, features.plugins=true]
  codex-disabled:
    base: codex
    env: {CODEX_HOME: `+quoted(home("disabled-codex"))+`}
`)
	// The daemon keeps the CODEX_HOME of the environment that starts it.
	e.ok("daemon", "start")
	t.Cleanup(func() { e.stopDaemon(true) })

	launch := func(harness string, extra ...string) (dump, result) {
		t.Helper()
		path := filepath.Join(e.dir, "dump.json")
		_ = os.Remove(path)
		extra = append(extra, "FAKE_DUMP="+path, "FAKE_ENV=CODEX_HOME")
		r := e.agx(extra, harness, "exec", "hi")
		var d dump
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("agx %s did not run the harness (exit %d): %s", harness, r.code, r.stderr)
		}
		if err := json.Unmarshal(b, &d); err != nil {
			t.Fatal(err)
		}
		return d, r
	}
	// installs returns the arguments of every plugin add the fake Codex ran in a home.
	installs := func(home string) [][]string {
		t.Helper()
		out := [][]string{}
		f, err := os.Open(filepath.Join(home, "fake-plugin-log.jsonl"))
		if err != nil {
			return out
		}
		defer func() { _ = f.Close() }()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var args []string
			if err := json.Unmarshal(scanner.Bytes(), &args); err != nil {
				t.Fatal(err)
			}
			if i := slices.Index(args, "add"); i > 0 && args[i-1] == "plugin" {
				out = append(out, args)
			}
		}
		return out
	}
	enabled := func(d dump) bool {
		for _, arg := range d.Args {
			if strings.HasPrefix(arg, "plugins.review-") && strings.HasSuffix(arg, "@agx-managed.enabled=true") {
				return true
			}
		}
		return false
	}

	for _, test := range []struct {
		name, harness, home string
		extra               []string
	}{
		{"caller", "codex", home("caller-codex"), []string{"CODEX_HOME=" + home("caller-codex")}},
		{"definition", "codex-defined", home("defined-codex"), []string{"CODEX_HOME=" + home("caller-codex")}},
		{"override", "codex-override", home("override-codex"), nil},
	} {
		d, r := launch(test.harness, test.extra...)
		if r.code != 0 || strings.Contains(r.stderr, "disabled") {
			t.Errorf("%s: exit %d: %s", test.name, r.code, r.stderr)
		}
		added := installs(test.home)
		if len(added) != 1 || !slices.Contains(added[0], "--enable") {
			t.Errorf("%s: plugin installations in %s = %q, want one with plugins enabled", test.name, test.home, added)
		}
		if !samePath(d.Env["CODEX_HOME"], test.home) || !enabled(d) {
			t.Errorf("%s: harness ran in %q with %q", test.name, d.Env["CODEX_HOME"], d.Args)
		}
		// The installation is recorded for that home: the next launch needs none.
		if _, r := launch(test.harness, test.extra...); r.code != 0 || len(installs(test.home)) != 1 {
			t.Errorf("%s: second launch installed again: %q (%s)", test.name, installs(test.home), r.stderr)
		}
	}
	// The daemon's own CODEX_HOME saw no launch.
	if added := installs(filepath.Join(e.dir, "home", "codex")); len(added) != 0 {
		t.Errorf("installed in the daemon's CODEX_HOME: %q", added)
	}

	d, r := launch("codex-disabled")
	want := "Codex plugins are disabled (features.plugins = false in " + filepath.Join(
		home("disabled-codex"),
		"config.toml",
	)
	if r.code != 0 || !strings.Contains(r.stderr, want) || !strings.Contains(r.stderr, "launching without skills") {
		t.Errorf("disabled: exit %d: %s", r.code, r.stderr)
	}
	if enabled(d) {
		t.Errorf("disabled: skills enabled: %q", d.Args)
	}
}

// samePath compares directories by identity: Windows may spell one with short names or forward slashes.
func samePath(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	return err == nil && os.SameFile(ai, bi)
}
