package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/adapters/builtin"
	"github.com/tarik02-org/agx/internal/adapters/codex"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/shims"
)

func TestHarnessCheck(t *testing.T) {
	notFound := errors.New("native codex executable not found")
	for _, test := range []struct {
		name, configured, output string
		defined                  bool
		findErr, runErr          error
		status                   checkStatus
		detail                   string
	}{
		{"codex", "", "codex-cli 0.162.1\n", false, nil, nil, checkPass, "codex 0.162.1"},
		{"codex", "", "codex-cli 0.159.9\n", false, nil, nil, checkWarn, "older than the supported 0.160.0"},
		{"claude", "", "2.1.300 (Claude Code)\n", false, nil, nil, checkPass, "claude 2.1.300"},
		{"claude", "", "2.1.30 (Claude Code)\n", false, nil, nil, checkWarn, "older than the supported 2.1.290"},
		{"pi", "", "0.99.0\n", false, nil, nil, checkPass, "pi 0.99.0"},
		{"pi", "", "0.87.1\n", false, nil, nil, checkWarn, "pi 0.87.1 is older than the supported 0.99.0"},
		{"pi", "", "0.73.1\n", true, nil, nil, checkFail, "pi 0.73.1 is older than the supported 0.99.0"},
		{"pi", "", "v1.2.0\n", false, nil, nil, checkPass, "pi 1.2.0"},
		{"pi", "", "", false, notFound, nil, checkWarn, "not found on PATH"},
		{"pi", "/opt/pi", "", false, notFound, nil, checkFail, "not found"},
		{"pi", "", "", false, nil, errors.New("exit status 1"), checkWarn, "--version failed"},
		{"pi", "", "pi dev build\n", false, nil, nil, checkWarn, "version not recognized"},
	} {
		got := harnessCheck(
			test.name,
			test.configured,
			test.defined,
			"/bin/"+test.name,
			test.findErr,
			test.output,
			test.runErr,
		)
		if got.Status != test.status || !strings.Contains(got.Detail, test.detail) {
			t.Errorf("%s %q: got %s %q, want %s containing %q", test.name, test.output, got.Status, got.Detail,
				test.status, test.detail)
		}
		if got.Status != checkPass && got.Hint == "" && test.runErr == nil && !strings.Contains(test.output, "dev") {
			t.Errorf("%s %q: %s without a hint", test.name, test.output, got.Status)
		}
	}
	if compareVersions("0.160.0", "0.99.0") != 1 || compareVersions("2.1.29", "2.1.290") != -1 ||
		compareVersions("1.2.0", "1.2.0") != 0 {
		t.Error("compareVersions misorders versions")
	}
}

func TestPackageChecks(t *testing.T) {
	read := &config.Config{Root: "/c/agx.yaml"}
	loaded := &config.Config{Root: "/c/agx.yaml", Packages: map[config.ID]config.Package{"team": {}}}
	_, ok, rows := packageChecks(read, loaded, nil, "1 file")
	if !ok || len(rows) != 2 || rows[1].Status != checkPass ||
		rows[1].Detail != "1 package installed; lockfiles current" {
		t.Errorf("loaded = %v %+v", ok, rows)
	}
	_, ok, rows = packageChecks(read, nil, errors.New("package team is not installed; run agx install"), "1 file")
	if ok || len(rows) != 2 || rows[0].Status != checkPass || rows[1].Status != checkFail ||
		rows[1].Hint != "run agx install" {
		t.Errorf("missing package = %v %+v", ok, rows)
	}
	_, ok, rows = packageChecks(read, nil, errors.New("/c/agx.yaml:3: unknown skill x"), "1 file")
	if ok || len(rows) != 1 || rows[0].Status != checkFail || !strings.Contains(rows[0].Detail, "unknown skill") {
		t.Errorf("invalid = %v %+v", ok, rows)
	}
}

func TestCodexChecks(t *testing.T) {
	rows := codexChecks(codex.PluginState{Disabled: "Codex plugins are disabled (features.plugins = false in x)"}, nil)
	if len(rows) != 1 || rows[0].Status != checkWarn || !strings.Contains(rows[0].Detail, "features.plugins = false") {
		t.Errorf("disabled = %+v", rows)
	}
	rows = codexChecks(codex.PluginState{Registered: true, Installed: []string{"review"}}, nil)
	if len(rows) != 3 || rows[1].Status != checkPass || rows[2].Status != checkPass {
		t.Errorf("installed = %+v", rows)
	}
	rows = codexChecks(codex.PluginState{Missing: []string{"review"}}, nil)
	if rows[1].Status != checkWarn || rows[2].Status != checkWarn || rows[2].Detail != "not installed: review" {
		t.Errorf("missing = %+v", rows)
	}
}

// Each Codex-based definition is checked with its own CODEX_HOME and arguments.
func TestCodexDefinitionChecks(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "work-codex")
	for path, content := range map[string]string{
		filepath.Join(home, "config.toml"):                 "[features]\nplugins = false\n",
		filepath.Join(dir, "skills", "review", "SKILL.md"): "---\nname: review\ndescription: r\n---\nR.\n",
		filepath.Join(dir, "codex"):                        "not run",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	executable := "'" + filepath.ToSlash(filepath.Join(dir, "codex")) + "'"
	workHome := "'" + filepath.ToSlash(home) + "'"
	if err := os.WriteFile(filepath.Join(dir, "agx.yaml"), []byte(`skills:
  review: {path: ./skills/review}
scopes:
  - add: {skills: [review]}
harnesses:
  codex: {executable: `+executable+`}
  codex-work:
    base: codex
    environment: {CODEX_HOME: `+workHome+`}
  codex-on:
    base: codex-work
    args: [-c, features.plugins=true]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := builtin.Registry()
	c, err := config.Load(filepath.Join(dir, "agx.yaml"), filepath.Join(dir, "cache"), registry.Builtins(nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", filepath.Join(dir, "caller-codex"))
	statuses := map[string]checkStatus{}
	details := map[string]string{}
	for _, row := range codexDefinitionChecks(t.Context(), c, registry, filepath.Join(dir, "cache")) {
		statuses[row.Name], details[row.Name] = row.Status, row.Detail
	}
	if statuses["plugins feature"] != checkPass || !strings.Contains(details["plugins feature"], "caller-codex") {
		t.Errorf("codex = %v %v", statuses, details)
	}
	if statuses["codex-work plugins feature"] != checkWarn ||
		!strings.Contains(details["codex-work plugins feature"], filepath.Join(home, "config.toml")) {
		t.Errorf("codex-work = %v %v", statuses, details)
	}
	if statuses["codex-on plugins feature"] != checkPass || statuses["codex-on skill plugins"] != checkWarn {
		t.Errorf("codex-on = %v %v", statuses, details)
	}
}

func TestConfigHintIsQuoted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX quoting")
	}
	row := daemonCheck(daemonFacts{config: "/c", wantConfig: "/home/me/my config/it's.yaml"})
	if want := `agx --config '/home/me/my config/it'\''s.yaml' daemon start`; !strings.Contains(row.Hint, want) {
		t.Errorf("hint = %s, want %s", row.Hint, want)
	}
	if got := shellQuote("/plain/agx.yaml"); got != "/plain/agx.yaml" {
		t.Errorf("plain path quoted as %s", got)
	}
}

func TestDaemonCheck(t *testing.T) {
	base := daemonFacts{version: "1", config: "/c", wantConfig: "/c", instance: "a", previous: "a"}
	if got := daemonCheck(base); got.Status != checkPass || !strings.Contains(got.Detail, "running") {
		t.Errorf("running = %+v", got)
	}
	started := base
	started.mayHaveStarted, started.previous = true, ""
	if got := daemonCheck(started); got.Status != checkPass || !strings.HasPrefix(got.Detail, "started") {
		t.Errorf("started = %+v", got)
	}
	replaced := base
	replaced.mayHaveStarted, replaced.previous, replaced.otherBuild = true, "old", true
	if got := daemonCheck(replaced); !strings.HasPrefix(got.Detail, "replaced") {
		t.Errorf("replaced = %+v", got)
	}
	other := base
	other.config = "/other"
	if got := daemonCheck(other); got.Status != checkFail || !strings.Contains(got.Hint, "daemon stop") {
		t.Errorf("other configuration = %+v", got)
	}
}

func TestMCPCheck(t *testing.T) {
	server := config.MCPServer{Auth: config.MCPAuth{Secret: "github"}}
	got := mcpCheck("docs", server, &v1.TestMcpServerResponse{Tools: []string{"a", "b"}, ServerName: "docs"}, nil)
	if got.Status != checkPass || got.Detail != "2 tools from docs" {
		t.Errorf("ready = %+v", got)
	}
	needsAuth := reasonError(t, connect.CodeFailedPrecondition, v1.ErrorDetail_REASON_NEEDS_LOGIN)
	got = mcpCheck("github", server, nil, needsAuth)
	if got.Status != checkWarn || got.Hint != "run agx secret login github" || got.Detail != "needs authorization: x" {
		t.Errorf("needs auth = %+v", got)
	}
	got = mcpCheck("slow", config.MCPServer{}, nil, connect.NewError(connect.CodeDeadlineExceeded, errors.New("late")))
	if got.Status != checkFail || !strings.Contains(got.Detail, "no answer within") {
		t.Errorf("timeout = %+v", got)
	}
	failed := connect.NewError(connect.CodeFailedPrecondition, errors.New("upstream process exited during startup"))
	got = mcpCheck("broken", config.MCPServer{}, nil, failed)
	if got.Status != checkFail || got.Detail != "upstream process exited during startup" {
		t.Errorf("failed = %+v", got)
	}
}

func TestSecretCheck(t *testing.T) {
	env := func(values map[string]string) func(string) (string, bool) {
		return func(name string) (string, bool) {
			value, ok := values[name]
			return value, ok
		}
	}
	none := env(nil)
	for _, test := range []struct {
		status *v1.SecretStatus
		secret config.Secret
		lookup func(string) (string, bool)
		want   checkStatus
		hint   string
	}{
		{&v1.SecretStatus{SecretId: "a", Kind: "keychain", State: v1.SecretStatus_STATE_READY}, config.Secret{}, none, checkPass, ""},
		{
			&v1.SecretStatus{SecretId: "gh", Kind: "oauth", State: v1.SecretStatus_STATE_NEEDS_LOGIN},
			config.Secret{},
			none, checkWarn, "run agx secret login gh",
		},
		{
			&v1.SecretStatus{SecretId: "key", Kind: "keychain", State: v1.SecretStatus_STATE_NEEDS_LOGIN},
			config.Secret{},
			none, checkWarn, "run agx secret set key --stdin",
		},
		{
			&v1.SecretStatus{
				SecretId: "s", Kind: "sops", State: v1.SecretStatus_STATE_UNAVAILABLE,
				Diagnostics: []*v1.Diagnostic{{Summary: "sops cannot decrypt"}},
			},
			config.Secret{},
			none, checkFail, "",
		},
		{
			&v1.SecretStatus{SecretId: "t", Kind: "environment", State: v1.SecretStatus_STATE_UNCONFIGURED},
			config.Secret{Kind: "environment", Variable: "TOKEN"},
			env(map[string]string{"TOKEN": "x"}), checkPass, "",
		},
		{
			&v1.SecretStatus{SecretId: "t", Kind: "environment", State: v1.SecretStatus_STATE_UNCONFIGURED},
			config.Secret{Kind: "environment", Variable: "TOKEN"},
			none, checkWarn, "set TOKEN in the shell that launches harnesses",
		},
	} {
		got := secretCheck(test.status, test.secret, test.lookup)
		if got.Status != test.want || got.Hint != test.hint {
			t.Errorf(
				"%s (%s): got %+v, want %s with hint %q",
				test.status.SecretId,
				test.status.State,
				got,
				test.want,
				test.hint,
			)
		}
	}
}

func TestSopsChecks(t *testing.T) {
	c := &config.Config{Secrets: map[config.ID]config.Secret{"s": {Kind: "sops"}}}
	missing := func(string) (string, error) { return "", exec.ErrNotFound }
	if rows := sopsChecks(c, missing); len(rows) != 1 || rows[0].Status != checkFail {
		t.Errorf("missing sops = %+v", rows)
	}
	found := func(string) (string, error) { return "/bin/sops", nil }
	if rows := sopsChecks(c, found); len(rows) != 1 || rows[0].Status != checkPass {
		t.Errorf("found sops = %+v", rows)
	}
	if rows := sopsChecks(&config.Config{}, missing); len(rows) != 0 {
		t.Errorf("without sops secrets = %+v", rows)
	}
}

func TestLinkedOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("links on PATH are symlinks")
	}
	isolate(t, "{}\n")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(self, filepath.Join(dir, "codex-cpa")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "codex-other"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	linked, rest := linkedOnPath([]string{"codex-cpa", "codex-other", "codex-none"})
	if !slices.Equal(linked, []string{"codex-cpa"}) || !slices.Equal(rest, []string{"codex-other", "codex-none"}) {
		t.Errorf("linked = %v, rest = %v", linked, rest)
	}
}

func TestShimRows(t *testing.T) {
	entry := func(name string, state shims.State) shims.Entry {
		return shims.Entry{Shim: shims.Shim{Name: name}, State: state}
	}
	rows := shimRows("/s", []shims.Entry{entry("work", shims.Missing)}, false, nil)
	if len(rows) != 1 || rows[0].Status != checkWarn || !strings.Contains(rows[0].Hint, "agx shims install") {
		t.Errorf("none installed = %+v", rows)
	}
	rows = shimRows("/s", []shims.Entry{entry("work", shims.Current)}, true, nil)
	if len(rows) != 2 || rows[0].Status != checkPass || rows[1].Status != checkPass {
		t.Errorf("current = %+v", rows)
	}
	rows = shimRows(
		"/s",
		[]shims.Entry{entry("work", shims.Current), entry("old", shims.Stale)},
		false,
		[]shims.Collision{{Name: "work", Path: "/usr/bin/work", First: true}},
	)
	if len(rows) != 3 || rows[0].Status != checkWarn || rows[1].Status != checkWarn ||
		rows[2].Detail != "runs /usr/bin/work, not the agx shim" {
		t.Errorf("stale, off PATH, shadowed = %+v", rows)
	}
}

func TestDoctorJSONFailsWithoutConfiguration(t *testing.T) {
	isolate(t, "harnesses: [broken\n")
	t.Setenv("AGX_NO_AUTOSTART", "1")
	t.Setenv("PATH", t.TempDir())
	code, docs := runJSON(t, "doctor", "--json", t.TempDir())
	if code != 1 || len(docs) != 1 || docs[0].Data == nil {
		t.Fatalf("doctor exited %d: %+v", code, docs)
	}
	text := string(docs[0].Data)
	for _, want := range []string{
		`"name":"configuration","status":"fail"`,
		`"name":"daemon","status":"warn"`,
		`"fail":1`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("doctor document lacks %s:\n%s", want, text)
		}
	}
}

// A version printed only to standard error, as some Pi releases do, is still read.
func TestHarnessVersionStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a shell script")
	}
	path := filepath.Join(t.TempDir(), "pi")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho 0.73.1 >&2\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	output, err := harnessVersion(t.Context(), path)
	if err != nil || strings.TrimSpace(output) != "0.73.1" {
		t.Errorf("harnessVersion = %q, %v", output, err)
	}
}

func TestStateCheck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon-state.json")
	if got := stateCheck(path); got.Status != checkPass || got.Detail != "no saved launches" {
		t.Fatalf("missing state = %+v", got)
	}
	for _, test := range []struct {
		data   string
		status checkStatus
	}{
		{`{"version":1}`, checkPass},
		{`{"version":99}`, checkFail},
		{`{"version":`, checkWarn},
	} {
		if err := os.WriteFile(path, []byte(test.data), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := stateCheck(path); got.Status != test.status {
			t.Fatalf("state %s = %+v", test.data, got)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := stateCheck(path); got.Status != checkFail || !strings.Contains(got.Hint, "chmod 600") {
		t.Fatalf("readable state = %+v", got)
	}
}
