//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	version    = "0.0.0-e2e"
	oldVersion = "0.0.0-e2e-old"
	token      = "e2e-secret-token"
	sealed     = "e2e-sops-value"
)

// binaries holds the executables TestMain builds; tools is the directory of those found on PATH.
var binaries struct{ agx, oldAgx, harness, mcp, tools string }

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "agx-e2e-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := func(name string) string {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		return filepath.Join(dir, name)
	}
	binaries.agx, binaries.oldAgx = exe("agx"), exe("agx-old")
	binaries.harness, binaries.mcp = exe("fakeharness"), exe("fakemcp")
	binaries.tools = filepath.Join(dir, "tools")
	sops := filepath.Join(binaries.tools, filepath.Base(exe("sops")))
	if err := os.MkdirAll(binaries.tools, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	const versionFlag = "-ldflags=-X github.com/tarik02-org/agx/internal/daemon.Version="
	for _, b := range [][]string{
		{binaries.agx, versionFlag + version, "github.com/tarik02-org/agx/cmd/agx"},
		{binaries.oldAgx, versionFlag + oldVersion, "github.com/tarik02-org/agx/cmd/agx"},
		{binaries.harness, "./testdata/fakeharness"},
		{binaries.mcp, "./testdata/fakemcp"},
		{sops, "../auth/testdata/fakesops"},
	} {
		cmd := exec.CommandContext(context.Background(), "go", append([]string{"build", "-o", b[0]}, b[1:]...)...)
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "building", b[0], err)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// env is an isolated environment: agx paths, home, and the harnesses' stores all live in dir.
type env struct {
	t    *testing.T
	dir  string
	work string
	vars []string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	e := &env{t: t, dir: dir, work: filepath.Join(dir, "work")}
	home := filepath.Join(dir, "home")
	overrides := map[string]string{
		"AGX_CONFIG":          filepath.Join(dir, "config", "agx.yaml"),
		"AGX_CACHE":           filepath.Join(dir, "cache"),
		"AGX_STATE":           filepath.Join(dir, "state"),
		"HOME":                home,
		"USERPROFILE":         home,
		"XDG_CONFIG_HOME":     filepath.Join(home, "config"),
		"XDG_CACHE_HOME":      filepath.Join(home, "cache"),
		"XDG_STATE_HOME":      filepath.Join(home, "state"),
		"XDG_DATA_HOME":       filepath.Join(home, "data"),
		"CODEX_HOME":          filepath.Join(home, "codex"),
		"CLAUDE_CONFIG_DIR":   filepath.Join(home, "claude"),
		"PI_CODING_AGENT_DIR": filepath.Join(home, "pi"),
		"E2E_TOKEN":           token,
		"FAKE_ENV":            "AGX_MCP_GRANT,DEFINED,E2E_TOKEN",
		"NO_COLOR":            "1",
		"PATH":                binaries.tools + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	for _, pair := range os.Environ() {
		name, _, _ := strings.Cut(pair, "=")
		if _, replaced := overrides[strings.ToUpper(name)]; replaced ||
			strings.HasPrefix(strings.ToUpper(name), "AGX_") {
			continue
		}
		e.vars = append(e.vars, pair)
	}
	for name, value := range overrides {
		e.vars = append(e.vars, name+"="+value)
	}
	for _, d := range []string{home, filepath.Join(e.work, "sub")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	e.write(
		"config/skills/review/SKILL.md",
		"---\nname: review\ndescription: Review changes.\n---\nReview carefully.\n",
	)
	// The fake sops reads "encrypted" files as plain JSON.
	e.write("config/sealed.json", `{"echo": {"extra": "`+sealed+`"}}`)
	e.write("config/agx.yaml", `skills:
  review: {path: ./skills/review}
instructions:
  rules: {text: E2E instructions.}
secrets:
  token: {kind: environment, variable: E2E_TOKEN}
  sealed: {kind: sops, file: ./sealed.json, key: echo.extra}
mcp:
  echo:
    command: ['`+filepath.ToSlash(binaries.mcp)+`']
    environment:
      ECHO_TOKEN: !secret token
      ECHO_EXTRA: !secret sealed
scopes:
  - directory: ../work
    add:
      skills: [review]
      instructions: [rules]
      mcp: [echo]
harnesses:
  claude:
    executable: '`+filepath.ToSlash(binaries.harness)+`'
  claude-e2e:
    base: claude
    args: [--from-definition]
    environment: {DEFINED: "yes"}
`)
	return e
}

func (e *env) write(rel, content string) {
	e.t.Helper()
	path := filepath.Join(e.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

type result struct {
	code           int
	stdout, stderr string
}

// agx runs the binary in the work directory with extra environment variables.
func (e *env) agx(extra []string, args ...string) result {
	e.t.Helper()
	r := e.run(binaries.agx, extra, args...)
	if r.code < 0 {
		e.t.Fatalf("agx %q: %s", args, r.stderr)
	}
	return r
}

// run runs an agx binary in the work directory with extra environment variables. It is safe
// for concurrent use.
func (e *env) run(binary string, extra []string, args ...string) result {
	return e.runInput(binary, extra, "", args...)
}

// runInput runs an agx binary like run, with input as its standard input.
func (e *env) runInput(binary string, extra []string, input string, args ...string) result {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Dir = e.work
	cmd.Env = append(slices.Clone(e.vars), extra...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	r := result{stdout: stdout.String(), stderr: stderr.String()}
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		r.code = exit.ExitCode()
	case err != nil:
		r.code = -1
		r.stderr += err.Error()
	}
	return r
}

// ok runs agx and fails the test unless it exits 0.
func (e *env) ok(args ...string) result {
	e.t.Helper()
	r := e.agx(nil, args...)
	if r.code != 0 {
		e.t.Fatalf("agx %q exited %d\nstdout: %s\nstderr: %s", args, r.code, r.stdout, r.stderr)
	}
	return r
}

type dump struct {
	Args         []string          `json:"args"`
	Directory    string            `json:"directory"`
	Env          map[string]string `json:"env"`
	Skills       map[string]string `json:"skills"`
	Instructions string            `json:"instructions"`
	Settings     string            `json:"settings"`
	Endpoints    map[string]string `json:"endpoints"`
	Tools        map[string]string `json:"tools"`
}

// harness runs a harness command through agx and returns what the fake harness recorded.
func (e *env) harness(name string, args ...string) (dump, result) {
	e.t.Helper()
	path := filepath.Join(e.dir, "dump.json")
	_ = os.Remove(path)
	r := e.agx([]string{"FAKE_DUMP=" + path}, append([]string{name}, args...)...)
	var d dump
	b, err := os.ReadFile(path)
	if err != nil {
		e.t.Fatalf("agx %s %q did not run the harness (exit %d): %s", name, args, r.code, r.stderr)
	}
	if err := json.Unmarshal(b, &d); err != nil {
		e.t.Fatal(err)
	}
	return d, r
}

// stopDaemon stops the daemon and waits until its process released the runtime directory.
func (e *env) stopDaemon(force bool) {
	e.t.Helper()
	args := []string{"daemon", "stop"}
	if force {
		args = append(args, "--force")
	}
	if r := e.agx(nil, args...); r.code != 0 {
		if force {
			return
		}
		e.t.Fatalf("daemon stop exited %d: %s", r.code, r.stderr)
	}
	runtimeDir := filepath.Join(e.dir, "cache", "runtime")
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		_, err := os.Stat(filepath.Join(runtimeDir, "daemon.json"))
		// Windows refuses to remove the lock file while the daemon holds it.
		if errors.Is(err, fs.ErrNotExist) {
			lock := filepath.Join(runtimeDir, "daemon.lock")
			if err := os.Remove(lock); err == nil || errors.Is(err, fs.ErrNotExist) {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	e.t.Fatal("daemon did not exit")
}

// daemonRunning reports whether a daemon published its discovery file.
func (e *env) daemonRunning() bool {
	_, err := os.Stat(filepath.Join(e.dir, "cache", "runtime", "daemon.json"))
	return err == nil
}

// daemonVersion returns the version daemon status reports.
func (e *env) daemonVersion() string {
	e.t.Helper()
	var status struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	r := e.ok("daemon", "status", "--json")
	if err := json.Unmarshal([]byte(r.stdout), &status); err != nil {
		e.t.Fatal(err)
	}
	return status.Data.Version
}

// envelope is one JSON document agx writes to stdout with --json.
type envelope struct {
	Version int             `json:"version"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// jsonRun runs agx with --json and checks that stdout is exactly one versioned document, with
// data when it exits 0 and an error otherwise.
func (e *env) jsonRun(args ...string) (int, envelope) {
	e.t.Helper()
	r := e.agx(nil, append(args, "--json")...)
	var doc envelope
	if strings.Count(r.stdout, "\n") != 1 || json.Unmarshal([]byte(r.stdout), &doc) != nil || doc.Version != 1 {
		e.t.Fatalf("agx %q --json printed %q (stderr %q)", args, r.stdout, r.stderr)
	}
	if (r.code == 0) != (doc.Data != nil) || (r.code == 0) == (doc.Error != nil) {
		e.t.Fatalf("agx %q --json exited %d with %s", args, r.code, r.stdout)
	}
	return r.code, doc
}

// jsonOK runs agx with --json, requires success, and decodes the document's data into into.
func (e *env) jsonOK(into any, args ...string) {
	e.t.Helper()
	if code, doc := e.jsonRun(args...); code != 0 {
		e.t.Fatalf("agx %q --json exited %d: %+v", args, code, doc.Error)
	} else if err := json.Unmarshal(doc.Data, into); err != nil {
		e.t.Fatal(err)
	}
}

// jsonFail runs agx with --json and requires exit status 1 with the error code given.
func (e *env) jsonFail(code string, args ...string) {
	e.t.Helper()
	if got, doc := e.jsonRun(args...); got != 1 || doc.Error.Code != code {
		e.t.Fatalf("agx %q --json exited %d with %+v, want error %s", args, got, doc.Error, code)
	}
}

func TestEndToEnd(t *testing.T) {
	e := newEnv(t)

	if r := e.ok("--version"); !strings.Contains(r.stdout, version) {
		t.Fatalf("--version printed %q", r.stdout)
	}
	e.ok("config", "validate")

	t.Run("passthrough without a daemon", func(t *testing.T) {
		d, r := e.harness("claude-e2e", "--version")
		if r.code != 0 || !strings.Contains(r.stdout, "fakeharness 1") {
			t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
		}
		if !slices.Equal(d.Args, []string{"--version"}) {
			t.Errorf("args = %q; definition groups do not apply to passthrough", d.Args)
		}
		if d.Env["DEFINED"] != "yes" {
			t.Errorf("definition env missing: %v", d.Env)
		}
		if _, ok := d.Env["AGX_MCP_GRANT"]; ok {
			t.Error("passthrough received a grant")
		}
	})

	t.Run("links named after harnesses", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("shims are hard links or copies on Windows")
		}
		links, native := filepath.Join(e.dir, "links"), filepath.Join(e.dir, "native")
		for _, d := range []string{links, native} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		for _, name := range []string{"claude", "claude-e2e", "codex"} {
			if err := os.Symlink(binaries.agx, filepath.Join(links, name)); err != nil {
				t.Fatal(err)
			}
		}
		// codex has no pinned executable: lookup skips the link to agx earlier on PATH.
		if err := os.Symlink(binaries.harness, filepath.Join(native, "codex")); err != nil {
			t.Fatal(err)
		}
		path := "PATH=" + links + string(os.PathListSeparator) + native + string(os.PathListSeparator) +
			binaries.tools + string(os.PathListSeparator) + os.Getenv("PATH")
		for _, name := range []string{"claude", "claude-e2e", "codex"} {
			dumped := filepath.Join(e.dir, "dump-"+name+".json")
			r := e.run(filepath.Join(links, name), []string{path, "FAKE_DUMP=" + dumped}, "--version")
			if r.code != 0 || !strings.Contains(r.stdout, "fakeharness 1") {
				t.Fatalf("%s --version: exit %d, stdout %q, stderr %q", name, r.code, r.stdout, r.stderr)
			}
			b, err := os.ReadFile(dumped)
			if err != nil {
				t.Fatalf("%s did not run the native harness: %v", name, err)
			}
			var d dump
			if err := json.Unmarshal(b, &d); err != nil {
				t.Fatal(err)
			}
			if want := name == "claude-e2e"; (d.Env["DEFINED"] == "yes") != want {
				t.Errorf("%s: definition environment applied = %v, want %v", name, !want, want)
			}
		}

		// A managed launch through a link starts the daemon as agx, not as the link's harness.
		r := e.run(filepath.Join(links, "claude-e2e"), []string{path}, "-p", "hi")
		if r.code != 0 || !e.daemonRunning() {
			t.Fatalf("managed launch through a link: exit %d, stderr %q", r.code, r.stderr)
		}
		e.stopDaemon(true)
	})

	if e.daemonRunning() {
		t.Fatal("a passthrough command started the daemon")
	}
	r := e.agx([]string{"AGX_NO_AUTOSTART=1"}, "claude", "-p", "hi")
	if r.code != 1 || !strings.Contains(r.stderr, "agx daemon start") || e.daemonRunning() {
		t.Fatalf("a managed launch with autostart disabled: exit %d, %q", r.code, r.stderr)
	}
	if r := e.agx(nil, "daemon", "status"); r.code != 0 || !strings.Contains(r.stdout, "not running") ||
		e.daemonRunning() {
		t.Fatalf("daemon status without a daemon: exit %d, %q", r.code, r.stderr)
	}

	e.ok("daemon", "start")
	t.Cleanup(func() { e.stopDaemon(true) })
	if r := e.ok("daemon", "status", "--json"); !strings.Contains(r.stdout, version) {
		t.Fatalf("daemon status = %s", r.stdout)
	}

	var launched dump
	t.Run("managed launch", func(t *testing.T) {
		d, r := e.harness("claude-e2e", "-p", "hello")
		if r.code != 0 {
			t.Fatalf("exit %d: %s", r.code, r.stderr)
		}
		launched = d
		if n := len(d.Args); n < 3 || !slices.Equal(d.Args[n-3:], []string{"--from-definition", "-p", "hello"}) {
			t.Errorf("args = %q, want the definition's group before the native arguments", d.Args)
		}
		work, err := filepath.EvalSymlinks(e.work)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := filepath.EvalSymlinks(d.Directory); !strings.EqualFold(got, work) {
			t.Errorf("directory = %q, want %q", d.Directory, work)
		}
		if d.Env["DEFINED"] != "yes" || d.Env["AGX_MCP_GRANT"] == "" {
			t.Errorf("env = %v", d.Env)
		}
		if _, ok := d.Env["E2E_TOKEN"]; ok {
			t.Error("a variable only the MCP server needs reached the harness")
		}
		if len(d.Skills) != 1 {
			t.Errorf("skills = %v", d.Skills)
		}
		for _, s := range d.Skills {
			if !strings.Contains(s, "Review carefully.") {
				t.Errorf("skill = %q", s)
			}
		}
		if !strings.Contains(d.Instructions, "E2E instructions.") {
			t.Errorf("instructions = %q", d.Instructions)
		}
		if got := d.Tools["agx-echo"]; got != "echo:hello:"+token+":"+sealed {
			t.Errorf("echo tool through the daemon = %q", got)
		}
	})

	t.Run("sops secret status", func(t *testing.T) {
		r := e.ok("secret", "status", "sealed", "--json")
		if !strings.Contains(r.stdout, `"kind":"sops"`) || !strings.Contains(r.stdout, `"state":"ready"`) {
			t.Fatalf("status = %s", r.stdout)
		}
		if strings.Contains(r.stdout, sealed) {
			t.Fatal("secret status reveals the value")
		}
		if r := e.agx(nil, "secret", "set", "sealed"); r.code == 0 ||
			!strings.Contains(r.stderr, "edit it with sops") {
			t.Fatalf("secret set of a sops secret: exit %d, %s", r.code, r.stderr)
		}
	})

	t.Run("finished launch", func(t *testing.T) {
		var view struct {
			Data struct {
				Launches []struct {
					Harness string `json:"harness"`
					Mode    string `json:"mode"`
					State   string `json:"state"`
				} `json:"launches"`
			} `json:"data"`
		}
		r := e.ok("launch", "list", "--json")
		if err := json.Unmarshal([]byte(r.stdout), &view); err != nil {
			t.Fatal(err)
		}
		if len(view.Data.Launches) != 1 || view.Data.Launches[0].State != "finished" ||
			view.Data.Launches[0].Harness != "claude-e2e" || view.Data.Launches[0].Mode != "print" {
			t.Fatalf("launches = %+v", view.Data.Launches)
		}
		if strings.Contains(r.stdout, launched.Env["AGX_MCP_GRANT"]) || strings.Contains(r.stdout, token) {
			t.Fatal("launch list reveals a credential")
		}
		endpoint := launched.Endpoints["agx-echo"]
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+launched.Env["AGX_MCP_GRANT"])
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("grant of a finished launch: HTTP %d", resp.StatusCode)
		}
	})

	t.Run("json", func(t *testing.T) {
		var view struct {
			Launches []struct {
				ID string `json:"id"`
			} `json:"launches"`
		}
		e.jsonOK(&view, "launch", "list")
		if len(view.Launches) != 1 {
			t.Fatalf("launches = %+v", view.Launches)
		}
		var revoked struct {
			LaunchID string `json:"launch_id"`
			Revoked  bool   `json:"revoked"`
		}
		e.jsonOK(&revoked, "launch", "revoke", view.Launches[0].ID)
		if revoked.LaunchID != view.Launches[0].ID || !revoked.Revoked {
			t.Fatalf("revoke = %+v", revoked)
		}
		e.jsonFail("not_found", "launch", "revoke", "missing")
		e.jsonFail("not_found", "launch", "show", "missing")

		var tested struct {
			Transport string `json:"transport"`
			Server    struct {
				Name string `json:"name"`
			} `json:"server"`
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
			State string `json:"state"`
		}
		e.jsonOK(&tested, "mcp", "test", "echo")
		if tested.Transport != "stdio" || tested.Server.Name != "fakemcp" || tested.State != "ready" ||
			len(tested.Tools) != 5 || tested.Tools[0].Name != "echo" || tested.Tools[0].Description != "Echo text." {
			t.Fatalf("mcp test = %+v", tested)
		}
		e.jsonFail("not_found", "mcp", "test", "missing")

		var secrets struct {
			Secrets []struct {
				ID    string `json:"id"`
				State string `json:"state"`
			} `json:"secrets"`
		}
		e.jsonOK(&secrets, "secret", "status")
		if len(secrets.Secrets) != 2 || secrets.Secrets[0].ID == "" || secrets.Secrets[0].State == "" {
			t.Fatalf("secret status = %+v", secrets)
		}
		e.jsonFail("failed_precondition", "secret", "set", "sealed")
	})

	t.Run("agent sdk initialize", func(t *testing.T) {
		stdin := filepath.Join(e.dir, "stdin.jsonl")
		dump := filepath.Join(e.dir, "dump.json")
		user := `{"type":"user","message":{"role":"user","content":"hi <there>"}}` + "\n"
		input := `{"type":"control_request","request_id":"r1","request":{"subtype":"initialize",` +
			`"appendSystemPrompt":"HOST","hooks":{}}}` + "\n" + user
		r := e.runInput(
			binaries.agx,
			[]string{"FAKE_STDIN=" + stdin, "FAKE_DUMP=" + dump},
			input,
			"claude", "--output-format", "stream-json", "--input-format", "stream-json", "--verbose",
		)
		if r.code != 0 {
			t.Fatalf("exit %d: %s", r.code, r.stderr)
		}
		b, err := os.ReadFile(stdin)
		if err != nil {
			t.Fatal(err)
		}
		first, rest, _ := strings.Cut(string(b), "\n")
		if rest != user {
			t.Errorf("later input changed: %q", rest)
		}
		var initialize struct {
			Request map[string]any `json:"request"`
		}
		if err := json.Unmarshal([]byte(first), &initialize); err != nil {
			t.Fatal(err)
		}
		if got := initialize.Request["appendSystemPrompt"]; got != "E2E instructions.\n\nHOST" {
			t.Errorf("appendSystemPrompt = %q", got)
		}
		if _, ok := initialize.Request["hooks"]; !ok {
			t.Errorf("initialize lost a field: %s", first)
		}
	})

	t.Run("native exit status", func(t *testing.T) {
		if _, r := e.harness("claude", "--exit=7"); r.code != 7 {
			t.Fatalf("exit %d: %s", r.code, r.stderr)
		}
	})

	t.Run("daemon stop", func(t *testing.T) {
		e.stopDaemon(false)
		if r := e.agx(nil, "daemon", "status"); r.code != 0 || !strings.Contains(r.stdout, "Daemon is not running.") {
			t.Fatalf("status after stop: exit %d: %s", r.code, r.stdout)
		}
	})
}

func TestDaemonJSON(t *testing.T) {
	e := newEnv(t)
	var status struct {
		Running *bool   `json:"running"`
		Version *string `json:"version"`
	}
	e.jsonOK(&status, "daemon", "status")
	if status.Running == nil || *status.Running || status.Version != nil {
		t.Fatalf("daemon status without a daemon = %+v", status)
	}
	for _, args := range [][]string{{"daemon", "stop"}, {"daemon", "stop", "--force"}} {
		var absent struct {
			Running  *bool `json:"running"`
			Stopping *bool `json:"stopping"`
		}
		e.jsonOK(&absent, args...)
		if absent.Running == nil || *absent.Running || absent.Stopping == nil || *absent.Stopping {
			t.Fatalf("%q without a daemon = %+v", args, absent)
		}
		if r := e.ok(args...); !strings.Contains(r.stdout, "Daemon is not running.") {
			t.Fatalf("%q without a daemon printed %q", args, r.stdout)
		}
	}
	var started struct {
		Started bool   `json:"started"`
		Version string `json:"version"`
		Log     string `json:"log"`
	}
	e.jsonOK(&started, "daemon", "start")
	t.Cleanup(func() { e.stopDaemon(true) })
	if !started.Started || started.Version != version || started.Log == "" {
		t.Fatalf("daemon start = %+v", started)
	}
	e.jsonOK(&started, "daemon", "start")
	if started.Started || started.Version != version || started.Log == "" {
		t.Fatalf("daemon start with one running = %+v", started)
	}
	var stopped struct {
		Stopping bool `json:"stopping"`
		Forced   bool `json:"forced"`
	}
	e.jsonOK(&stopped, "daemon", "stop", "--force")
	if !stopped.Stopping || !stopped.Forced {
		t.Fatalf("daemon stop = %+v", stopped)
	}
}

func TestAutostart(t *testing.T) {
	t.Run("launch starts the daemon", func(t *testing.T) {
		e := newEnv(t)
		t.Cleanup(func() { e.stopDaemon(true) })
		d, r := e.harness("claude", "-p", "hello")
		if r.code != 0 {
			t.Fatalf("exit %d: %s", r.code, r.stderr)
		}
		if d.Env["AGX_MCP_GRANT"] == "" || d.Tools["agx-echo"] != "echo:hello:"+token+":"+sealed {
			t.Fatalf("launch did not go through the daemon: env %v, tools %v", d.Env, d.Tools)
		}
		if r.stdout != "" {
			t.Errorf("autostart wrote to stdout: %q", r.stdout)
		}
		if got := e.daemonVersion(); got != version {
			t.Fatalf("daemon version = %q", got)
		}
	})

	t.Run("concurrent launches share one daemon", func(t *testing.T) {
		e := newEnv(t)
		t.Cleanup(func() { e.stopDaemon(true) })
		const n = 4
		results := make(chan error, n)
		endpoints := make([]string, n)
		for i := range n {
			go func() {
				path := filepath.Join(e.dir, fmt.Sprintf("dump-%d.json", i))
				r := e.run(binaries.agx, []string{"FAKE_DUMP=" + path}, "claude", "-p", "hello")
				if r.code != 0 {
					results <- fmt.Errorf("launch %d: exit %d: %s", i, r.code, r.stderr)
					return
				}
				var d dump
				b, err := os.ReadFile(path)
				if err == nil {
					err = json.Unmarshal(b, &d)
				}
				endpoints[i] = d.Endpoints["agx-echo"]
				results <- err
			}()
		}
		for range n {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		origin := func(endpoint string) string {
			before, _, _ := strings.Cut(strings.TrimPrefix(endpoint, "http://"), "/")
			return before
		}
		for _, endpoint := range endpoints {
			if endpoint == "" || origin(endpoint) != origin(endpoints[0]) {
				t.Fatalf("launches reached different daemons: %q", endpoints)
			}
		}
	})

	t.Run("idle daemon of another build is replaced", func(t *testing.T) {
		e := newEnv(t)
		t.Cleanup(func() { e.stopDaemon(true) })
		if r := e.run(binaries.oldAgx, nil, "daemon", "start"); r.code != 0 {
			t.Fatalf("old daemon start: exit %d: %s", r.code, r.stderr)
		}
		if r := e.agx(nil, "daemon", "status"); r.code != 0 || !strings.Contains(r.stderr, "runs agx "+oldVersion) {
			t.Fatalf("status of another build: exit %d, %q", r.code, r.stderr)
		}
		_, r := e.harness("claude", "-p", "hello")
		if r.code != 0 {
			t.Fatalf("exit %d: %s", r.code, r.stderr)
		}
		if !strings.Contains(r.stderr, "restarting daemon "+oldVersion) {
			t.Errorf("no restart note on stderr: %q", r.stderr)
		}
		if got := e.daemonVersion(); got != version {
			t.Fatalf("daemon version = %q, want %q", got, version)
		}
	})

	t.Run("daemon.autostart false", func(t *testing.T) {
		e := newEnv(t)
		b, err := os.ReadFile(filepath.Join(e.dir, "config", "agx.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		e.write("config/agx.yaml", string(b)+"daemon: {autostart: false}\n")
		if r := e.agx(nil, "secret", "status"); r.code != 1 || !strings.Contains(r.stderr, "agx daemon start") {
			t.Fatalf("secret status with autostart disabled: exit %d, %q", r.code, r.stderr)
		}
		if e.daemonRunning() {
			t.Fatal("daemon started although daemon.autostart is false")
		}
	})
}

func TestDoctor(t *testing.T) {
	e := newEnv(t)
	type report struct {
		Checks []struct {
			Group  string `json:"group"`
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
			Hint   string `json:"hint"`
		} `json:"checks"`
		Summary struct{ Pass, Warn, Fail int } `json:"summary"`
	}
	doctor := func(args ...string) (report, result) {
		t.Helper()
		r := e.agx(nil, append([]string{"doctor", "--json"}, args...)...)
		var doc struct {
			Data report `json:"data"`
		}
		if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil {
			t.Fatalf("doctor printed %q (exit %d): %v\n%s", r.stdout, r.code, err, r.stderr)
		}
		return doc.Data, r
	}
	status := func(rep report, group, name string) string {
		for _, c := range rep.Checks {
			if c.Group == group && c.Name == name {
				return c.Status + ": " + c.Detail
			}
		}
		return "missing"
	}

	rep, r := doctor()
	t.Cleanup(func() { e.stopDaemon(true) })
	if r.code != 0 || rep.Summary.Fail != 0 {
		t.Fatalf("doctor exited %d with %+v", r.code, rep)
	}
	for _, want := range [][3]string{
		{"config", "configuration", "pass"},
		{"config", "packages", "pass"},
		// The fake harness reports no version number.
		{"harness", "claude", "warn"},
		{"daemon", "daemon", "pass"},
		{"mcp", "echo", "pass"},
		{"secret", "token", "pass"},
		{"secret", "sealed", "pass"},
		{"secret", "sops", "pass"},
		{"shims", "shims", "warn"},
	} {
		if got := status(rep, want[0], want[1]); !strings.HasPrefix(got, want[2]+":") {
			t.Errorf("%s %s = %s, want %s", want[0], want[1], got, want[2])
		}
	}

	// A server outside the directory's selection is tested with --all, and its failure fails doctor.
	e.write(
		"config/broken.yaml",
		"mcp:\n  broken:\n    command: ['"+filepath.ToSlash(filepath.Join(e.dir, "missing"))+"']\n",
	)
	config, err := os.ReadFile(filepath.Join(e.dir, "config", "agx.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	e.write("config/agx.yaml", "imports: [broken.yaml]\n"+string(config))
	rep, r = doctor()
	if r.code != 0 || status(rep, "mcp", "broken") != "missing" {
		t.Fatalf("doctor without --all exited %d: %+v", r.code, rep)
	}
	rep, r = doctor("--all")
	if r.code != 1 || !strings.HasPrefix(status(rep, "mcp", "broken"), "fail:") || rep.Summary.Fail != 1 {
		t.Fatalf("doctor --all exited %d: %s", r.code, status(rep, "mcp", "broken"))
	}

	human := e.agx(nil, "doctor", "--all")
	if human.code != 1 || !strings.Contains(human.stdout, "CHECK") || !strings.Contains(human.stdout, "mcp broken") ||
		!strings.Contains(human.stdout, "agx mcp test broken") {
		t.Fatalf("doctor --all printed (exit %d):\n%s", human.code, human.stdout)
	}
}
