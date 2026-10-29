package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/daemon"
)

const scriptingConfig = `
instructions:
  conventions:
    text: Use pnpm.
mcp:
  fs:
    command: [npx, server-filesystem]
secrets:
  token:
    kind: keychain
harnesses:
  codex-work:
    base: codex
`

// document is any line agx writes to stdout in JSON mode.
type document struct {
	Version int             `json:"version"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Source  *config.Source `json:"source"`
	} `json:"error"`
}

// runJSON runs agx with args and checks that stdout holds JSON documents only, each versioned
// and carrying either data or an error.
func runJSON(t *testing.T, args ...string) (int, []document) {
	t.Helper()
	var out bytes.Buffer
	code := run(append([]string{"agx"}, args...), &out)
	if out.Len() == 0 {
		t.Fatalf("agx %q exited %d without output", args, code)
	}
	docs := []document{}
	for line := range strings.Lines(out.String()) {
		var d document
		decoder := json.NewDecoder(strings.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&d); err != nil {
			t.Fatalf("agx %q wrote a line that is not a document: %v\n%s", args, err, line)
		}
		if d.Version != 1 || (d.Data != nil) == (d.Error != nil) {
			t.Fatalf("agx %q wrote a malformed document: %s", args, line)
		}
		docs = append(docs, d)
	}
	return code, docs
}

// okJSON runs agx in JSON mode and decodes its single data document into into.
func okJSON(t *testing.T, into any, args ...string) {
	t.Helper()
	code, docs := runJSON(t, append([]string{"--json"}, args...)...)
	if code != 0 || len(docs) != 1 || docs[0].Data == nil {
		t.Fatalf("agx %q exited %d: %+v", args, code, docs)
	}
	if into != nil {
		if err := json.Unmarshal(docs[0].Data, into); err != nil {
			t.Fatal(err)
		}
	}
}

// failJSON runs agx in JSON mode and checks its error document and exit status.
func failJSON(t *testing.T, status int, code string, args ...string) document {
	t.Helper()
	got, docs := runJSON(t, args...)
	if got != status || len(docs) != 1 || docs[0].Error == nil || docs[0].Error.Code != code {
		b, _ := json.Marshal(docs)
		t.Fatalf("agx %q exited %d with %s, want exit %d and error %s", args, got, b, status, code)
	}
	if docs[0].Error.Message == "" {
		t.Fatalf("agx %q: error without a message", args)
	}
	return docs[0]
}

type editDocument = edits

func TestJSONEdits(t *testing.T) {
	path := isolate(t, scriptingConfig)
	work := t.TempDir()
	t.Chdir(work)

	var added editDocument
	okJSON(t, &added, "mcp", "add", "docs", "https://docs.example/mcp", "--auth", "none")
	if len(added.Files) != 1 || added.Files[0].Path == "" ||
		!strings.Contains(added.Files[0].Diff, "+  docs:") {
		t.Fatalf("files = %+v", added.Files)
	}
	if len(added.Defined) != 1 || added.Defined[0] != (definitionID{"mcp", "docs"}) ||
		len(added.Enabled) != 1 || added.Scope == nil || added.Scope.Directory != "" ||
		added.NeedsLogin == nil || *added.NeedsLogin {
		t.Fatalf("mcp add = %+v", added)
	}

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(server.Close)
	var registered editDocument
	okJSON(t, &registered, "mcp", "add", "gh", server.URL+"/mcp", "--client-id", "fixture-client", "--define-only")
	if len(registered.Files) != 1 || !strings.Contains(registered.Files[0].Diff, "+      client_id: fixture-client") {
		t.Fatalf("mcp add --client-id = %+v", registered.Files)
	}

	var stdio editDocument
	okJSON(t, &stdio, "mcp", "add", "local", "--define-only", "--", "server", "--flag")
	if len(stdio.Enabled) != 0 || stdio.Scope != nil || len(stdio.Defined) != 1 {
		t.Fatalf("mcp add --define-only = %+v", stdio)
	}

	var used editDocument
	okJSON(t, &used, "use", "local", "--scope", "here", "--harness", "codex")
	if used.Scope == nil || used.Scope.Directory == "" || len(used.Scope.Harnesses) != 1 || len(used.Files) != 1 {
		t.Fatalf("use = %+v", used)
	}
	var again editDocument
	okJSON(t, &again, "use", "local", "--scope", "here", "--harness", "codex")
	if len(again.Files) != 0 || len(again.Enabled) != 1 {
		t.Fatalf("use without a change = %+v", again)
	}
	var unused editDocument
	okJSON(t, &unused, "unuse", "docs", "--scope", "here")
	if len(unused.Disabled) != 1 || len(unused.Files) != 1 {
		t.Fatalf("unuse = %+v", unused)
	}

	skills := filepath.Join(work, "skills", "deploy")
	if err := os.MkdirAll(skills, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(skills, "SKILL.md"),
		[]byte("---\nname: deploy\n---\nDeploy.\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	var skill editDocument
	okJSON(t, &skill, "skill", "add", "./skills/deploy")
	if len(skill.Defined) != 1 || skill.Defined[0] != (definitionID{"skills", "deploy"}) {
		t.Fatalf("skill add = %+v", skill)
	}
	var instructions editDocument
	okJSON(t, &instructions, "instructions", "add", "--id", "style", "--text", "Be brief.")
	if len(instructions.Defined) != 1 {
		t.Fatalf("instructions add = %+v", instructions)
	}
	var harness editDocument
	okJSON(t, &harness, "harness", "add", "claude-work", "--base", "claude", "--", "--verbose")
	if len(harness.Defined) != 1 || harness.Defined[0] != (definitionID{"harnesses", "claude-work"}) {
		t.Fatalf("harness add = %+v", harness)
	}

	for _, kind := range []string{"mcp", "skill", "instructions", "harness"} {
		okJSON(t, nil, kind, "list")
	}

	failJSON(t, 1, codeConflict, "mcp", "remove", "local", "--json")
	var removed editDocument
	okJSON(t, &removed, "mcp", "remove", "local", "--force")
	if len(removed.Removed) != 1 || len(removed.Files) != 1 {
		t.Fatalf("mcp remove = %+v", removed)
	}
	okJSON(t, nil, "skill", "remove", "deploy", "--force")
	okJSON(t, nil, "instructions", "remove", "style", "--force")
	okJSON(t, nil, "harness", "remove", "claude-work")

	failJSON(t, 1, codeNotFound, "mcp", "remove", "missing", "--json")
	failJSON(t, 1, codeConflict, "mcp", "add", "docs", "https://other.example/mcp", "--json")
	failJSON(t, 1, codeNotFound, "use", "missing", "--json")
	failJSON(t, 1, codeConflict, "harness", "remove", "codex", "--json")
	failJSON(t, 1, codeReadOnly, "harness", "remove", "pi", "--json")
	// --json implies --yes and never prompts: a missing argument is a usage error.
	failJSON(t, 2, codeUsage, "mcp", "remove", "--json")

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "docs:") || strings.Contains(string(b), "local:") {
		t.Fatalf("configuration after the edits:\n%s", b)
	}
}

func TestJSONValidate(t *testing.T) {
	path := isolate(t, scriptingConfig)
	root := filepath.Join(filepath.Dir(filepath.Dir(path)), "agx.yaml")
	var valid struct {
		Root  string   `json:"root"`
		Files []string `json:"files"`
	}
	okJSON(t, &valid, "config", "validate")
	if code, docs := runJSON(t, "--config", root, "config", "validate", "--json"); code != 0 || len(docs) != 1 {
		t.Fatalf("trailing --json validate exited %d: %+v", code, docs)
	}
	if valid.Root == "" || len(valid.Files) != 2 {
		t.Fatalf("validate = %+v", valid)
	}

	if err := os.WriteFile(
		path,
		[]byte("mcp:\n  fs:\n    url: https://x.example\n    command: [x]\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	d := failJSON(t, 1, codeConfigInvalid, "config", "validate", "--json")
	if d.Error.Source == nil || d.Error.Source.Line != 4 || !strings.HasSuffix(d.Error.Source.Path, "main.yaml") {
		t.Fatalf("invalid validate = %+v", d.Error)
	}
	// --json before the command works as after it.
	failJSON(t, 1, codeConfigInvalid, "--json", "explain")

	d = failJSON(t, 1, codeConfigInvalid, "explain", "--json")
	if d.Error.Source == nil || d.Error.Source.Line != 4 {
		t.Fatalf("config error without its source: %+v", d.Error)
	}
	failJSON(t, 1, codeConfigInvalid, "mcp", "add", "x", "--json", "--", "y")
}

func TestMissingPaths(t *testing.T) {
	isolate(t, scriptingConfig)
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.yaml")
	d := failJSON(t, 1, codeConfigInvalid, "--config", missing, "config", "validate", "--json")
	if !strings.Contains(d.Error.Message, missing+" does not exist") {
		t.Fatalf("missing --config: %s", d.Error.Message)
	}
	// Adding a definition creates the named root, importing the fragment it writes.
	created := filepath.Join(dir, "created.yaml")
	okJSON(t, nil, "--config", created, "instructions", "add", "--id", "x", "--text", "hi")
	if b, err := os.ReadFile(created); err != nil || string(b) != config.DefaultConfig {
		t.Fatalf("created root: %v\n%s", err, b)
	}
	if b, err := os.ReadFile(
		filepath.Join(dir, "agx.d", "main.yaml"),
	); err != nil ||
		!strings.Contains(string(b), "x:") {
		t.Fatalf("created fragment: %v\n%s", err, b)
	}
	d = failJSON(t, 1, codeNotFound, "explain", filepath.Join(dir, "nope"), "--json")
	if !strings.Contains(d.Error.Message, "does not exist") {
		t.Fatalf("missing directory: %s", d.Error.Message)
	}
}

// With several writable fragments, an adding command needs --file, relative to the root.
func TestChooseFile(t *testing.T) {
	first := isolate(t, "{}\n")
	second := filepath.Join(filepath.Dir(first), "other.yaml")
	if err := os.WriteFile(second, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := failJSON(t, 1, "choose_file", "instructions", "add", "--id", "x", "--text", "hi", "--json")
	if !strings.Contains(d.Error.Message, "main.yaml") || !strings.Contains(d.Error.Message, "other.yaml") {
		t.Fatalf("choose_file error: %s", d.Error.Message)
	}
	t.Chdir(t.TempDir())
	var added editDocument
	okJSON(t, &added, "instructions", "add", "--id", "x", "--text", "hi", "--file", "agx.d/other.yaml")
	if len(added.Files) != 1 || !strings.HasSuffix(added.Files[0].Path, "other.yaml") {
		t.Fatalf("instructions add --file = %+v", added.Files)
	}
	failJSON(
		t,
		1,
		codeInvalidRequest,
		"instructions",
		"add",
		"--id",
		"y",
		"--text",
		"hi",
		"--file",
		"agx.yaml",
		"--json",
	)
}

func TestConfigSchema(t *testing.T) {
	isolate(t, "")
	// The schema goes to standard output as it is, not through the document writer.
	stdout, err := os.Create(filepath.Join(t.TempDir(), "schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = stdout
	code := run([]string{"agx", "config", "schema", "--root"}, io.Discard)
	os.Stdout = saved
	if err := stdout.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(stdout.Name())
	if err != nil || code != 0 || !strings.Contains(string(b), `"title": "agx root configuration"`) {
		t.Fatalf("config schema --root exited %d: %v\n%s", code, err, b)
	}
	failJSON(t, 2, codeUsage, "config", "schema", "--root", "--package", "--json")
}

func TestJSONCommands(t *testing.T) {
	isolate(t, scriptingConfig)
	t.Setenv("AGX_NO_AUTOSTART", "1")
	t.Chdir(t.TempDir())
	// secret set reads stdin before it reaches the daemon.
	stdin := os.Stdin
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = devNull
	t.Cleanup(func() {
		os.Stdin = stdin
		_ = devNull.Close()
	})

	okJSON(t, nil, "explain", "--harness", "codex-work")
	failJSON(t, 1, codeNotFound, "explain", "--harness", "missing", "--json")

	okJSON(t, nil, "install", "--frozen")
	okJSON(t, nil, "update")
	failJSON(t, 1, codeNotFound, "update", "missing", "--json")

	var shims struct {
		Directory string `json:"directory"`
		Changes   []struct {
			Name   string `json:"name"`
			Action string `json:"action"`
		} `json:"changes"`
	}
	okJSON(t, &shims, "shims", "install")
	if len(shims.Changes) != 1 || shims.Changes[0].Name != "codex-work" || shims.Changes[0].Action != "created" {
		t.Fatalf("shims install = %+v", shims)
	}
	okJSON(t, nil, "shims", "list")
	okJSON(t, &shims, "shims", "remove")
	if len(shims.Changes) != 1 || shims.Changes[0].Action != "removed" {
		t.Fatalf("shims remove = %+v", shims)
	}

	// Without a daemon, and with autostart off, daemon-backed commands report it as JSON.
	for _, args := range [][]string{
		{"launch", "list"},
		{"launch", "show", "x"},
		{"launch", "revoke", "x"},
		{"secret", "status"},
		{"secret", "set", "token"},
		{"secret", "logout", "token"},
		{"secret", "login", "token"},
		{"mcp", "tools", "fs"},
	} {
		failJSON(t, 1, codeDaemonUnavailable, append(args, "--json")...)
	}
	var status any = "unset"
	okJSON(t, &status, "daemon", "status")
	if status != nil {
		t.Fatalf("daemon status without a daemon = %+v", status)
	}
	for _, args := range [][]string{{"daemon", "stop"}, {"daemon", "stop", "--force"}} {
		var stopped struct {
			Running *bool `json:"running"`
		}
		okJSON(t, &stopped, args...)
		if stopped.Running == nil || *stopped.Running {
			t.Fatalf("%q without a daemon = %+v", args, stopped)
		}
	}
}

func TestJSONUsageErrors(t *testing.T) {
	isolate(t, scriptingConfig)
	for _, args := range [][]string{
		{"mcp", "list", "--json", "--bogus"},
		{"launch", "show", "--json"},
		{"nonsense", "--json"},
		{"config", "validate", "extra", "--json"},
		{"--color", "bogus", "mcp", "list", "--json"},
		{"--json", "--color", "bogus", "mcp", "list"},
		{"--json", "codex", "exec"},
		{"--json"},
	} {
		failJSON(t, 2, codeUsage, args...)
	}
	// A harness's native --json is not agx's.
	if !wantsJSON([]string{"mcp", "list", "--json"}) || wantsJSON([]string{"x", "--", "--json"}) {
		t.Fatal("wantsJSON misreads the command line")
	}
}

func TestErrorCodes(t *testing.T) {
	for _, test := range []struct {
		err  error
		want string
	}{
		{&usageError{errors.New("x")}, codeUsage},
		{coded(codeConflict, &usageError{errors.New("x")}), codeUsage},
		{coded(codeConflict, coded(codeNotFound, errors.New("x"))), codeConflict},
		{daemon.ErrVersionMismatch, codeVersionMismatch},
		{daemon.ErrUnavailable, codeDaemonUnavailable},
		{reasonError(t, connect.CodeNotFound, v1.ErrorDetail_REASON_NOT_FOUND), codeNotFound},
		{reasonError(t, connect.CodeFailedPrecondition, v1.ErrorDetail_REASON_NEEDS_LOGIN), codeNeedsLogin},
		{reasonError(t, connect.CodeFailedPrecondition, v1.ErrorDetail_REASON_CONFIG_INVALID), codeConfigInvalid},
		{reasonError(t, connect.CodeUnavailable, v1.ErrorDetail_REASON_CONFIG_INVALID), codeConfigInvalid},
		{reasonError(t, connect.CodeUnauthenticated, v1.ErrorDetail_REASON_UNAUTHENTICATED), codeDaemonUnavailable},
		// The reason decides, never the message.
		{connect.NewError(connect.CodeFailedPrecondition, errors.New("needs-auth: x")), codeInternal},
		{reasonError(t, connect.CodeInternal, v1.ErrorDetail_Reason(999)), codeInternal},
		{reasonError(t, connect.CodeNotFound, v1.ErrorDetail_REASON_UNSPECIFIED), codeInternal},
		// Errors without a reason come from the transport.
		{connect.NewError(connect.CodeUnavailable, errors.New("x")), codeDaemonUnavailable},
		{connect.NewError(connect.CodeDeadlineExceeded, errors.New("x")), codeTimeout},
		{connect.NewError(connect.CodeNotFound, errors.New("x")), codeInternal},
		{os.ErrNotExist, codeNotFound},
		{errors.New("unclassified"), codeInternal},
	} {
		if got := errorCode(test.err); got != test.want {
			t.Errorf("errorCode(%v) = %s, want %s", test.err, got, test.want)
		}
	}
	located := &config.SourceError{Path: "/etc/agx.yaml", Line: 12, Err: errors.New("bad")}
	for err, want := range map[error]*config.Source{
		located: {Path: "/etc/agx.yaml", Line: 12},
		fmt.Errorf("configuration is invalid: %w", &config.SourceError{Path: "/etc/agx.yaml", Err: errors.New("bad")}): {
			Path: "/etc/agx.yaml",
		},
		errors.New("package x is not installed; run agx install"): nil,
	} {
		if got := errorSource(err); !reflect.DeepEqual(got, want) {
			t.Errorf("errorSource(%v) = %+v, want %+v", err, got, want)
		}
	}
}

// reasonError is an error as the daemon reports it, with its reason in the details.
func reasonError(t *testing.T, code connect.Code, reason v1.ErrorDetail_Reason) error {
	t.Helper()
	err := connect.NewError(code, errors.New("x"))
	detail, detailErr := connect.NewErrorDetail(&v1.ErrorDetail{Reason: reason})
	if detailErr != nil {
		t.Fatal(detailErr)
	}
	err.AddDetail(detail)
	return err
}

func TestDefinitionLinkWithBrokenConfiguration(t *testing.T) {
	isolate(t, "harnesses: [broken\n")
	var out bytes.Buffer
	// Falling through to the CLI would print agx's version for the harness's --version.
	if code := run([]string{filepath.Join("bin", "codex-work"), "--version"}, &out); code != 1 || out.Len() != 0 {
		t.Fatalf("definition link exited %d with %q", code, out.String())
	}
	if code := run([]string{filepath.Join("bin", "agx-dev"), "--version"}, &out); code != 0 ||
		!strings.Contains(out.String(), "agx") {
		t.Fatalf("renamed agx exited %d with %q", code, out.String())
	}
}
