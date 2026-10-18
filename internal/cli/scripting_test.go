package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
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
	Event   string          `json:"event"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Source  *struct {
			Path string `json:"path"`
			Line int    `json:"line"`
		} `json:"source"`
	} `json:"error"`
}

// runJSON runs agx with args and checks that stdout holds JSON documents only, each versioned
// and carrying exactly one of data, event, or error.
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
		parts := 0
		for _, present := range []bool{d.Data != nil, d.Event != "", d.Error != nil} {
			if present {
				parts++
			}
		}
		if d.Version != 1 || parts != 1 {
			t.Fatalf("agx %q wrote a malformed document: %s", args, line)
		}
		docs = append(docs, d)
	}
	return code, docs
}

// okJSON runs agx in JSON mode and decodes its single data document into into.
func okJSON(t *testing.T, into any, args ...string) {
	t.Helper()
	// --json goes before any "--", after which arguments are not agx's.
	at := slices.Index(args, "--")
	if at < 0 {
		at = len(args)
	}
	code, docs := runJSON(t, slices.Insert(slices.Clone(args), at, "--json")...)
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

type editDocument struct {
	Files []struct {
		Path    string `json:"path"`
		Diff    string `json:"diff"`
		Written bool   `json:"written"`
	} `json:"files"`
	Defined  []definitionJSON `json:"defined"`
	Enabled  []definitionJSON `json:"enabled"`
	Disabled []definitionJSON `json:"disabled"`
	Removed  []definitionJSON `json:"removed"`
	Scope    *struct {
		Directory *string  `json:"directory"`
		Harnesses []string `json:"harnesses"`
	} `json:"scope"`
	NeedsLogin *bool `json:"needs_login"`
}

func TestJSONEdits(t *testing.T) {
	path := isolate(t, scriptingConfig)
	work := t.TempDir()
	t.Chdir(work)

	var added editDocument
	okJSON(t, &added, "mcp", "add", "docs", "https://docs.example/mcp", "--auth", "none")
	if len(added.Files) != 1 || added.Files[0].Path == "" || !added.Files[0].Written ||
		!strings.Contains(added.Files[0].Diff, "+  docs:") {
		t.Fatalf("files = %+v", added.Files)
	}
	if len(added.Defined) != 1 || added.Defined[0] != (definitionJSON{"mcp", "docs"}) ||
		len(added.Enabled) != 1 || added.Scope == nil || added.Scope.Directory != nil ||
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
	if used.Scope == nil || used.Scope.Directory == nil || len(used.Scope.Harnesses) != 1 || len(used.Files) != 1 {
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
	if len(skill.Defined) != 1 || skill.Defined[0] != (definitionJSON{"skills", "deploy"}) {
		t.Fatalf("skill add = %+v", skill)
	}
	var instructions editDocument
	okJSON(t, &instructions, "instructions", "add", "--id", "style", "--text", "Be brief.")
	if len(instructions.Defined) != 1 {
		t.Fatalf("instructions add = %+v", instructions)
	}
	var harness editDocument
	okJSON(t, &harness, "harness", "add", "claude-work", "--base", "claude", "--", "--verbose")
	if len(harness.Defined) != 1 || harness.Defined[0] != (definitionJSON{"harnesses", "claude-work"}) {
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
	var valid struct {
		Valid       bool              `json:"valid"`
		Root        string            `json:"root"`
		Files       []string          `json:"files"`
		Counts      *definitionCounts `json:"counts"`
		Diagnostics []json.RawMessage `json:"diagnostics"`
	}
	okJSON(t, &valid, "config", "validate")
	if code, docs := runJSON(t, "--json", "--config", path, "config", "validate"); code != 0 || len(docs) != 1 {
		t.Fatalf("leading --json validate exited %d: %+v", code, docs)
	}
	if !valid.Valid || valid.Root == "" || len(valid.Files) != 1 || valid.Counts == nil ||
		valid.Counts.MCP != 1 || valid.Counts.Secrets != 1 || valid.Counts.Harnesses != 1 ||
		len(valid.Diagnostics) != 0 {
		t.Fatalf("validate = %+v", valid)
	}

	if err := os.WriteFile(
		path,
		[]byte("mcp:\n  fs:\n    url: https://x.example\n    command: [x]\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	code, docs := runJSON(t, "config", "validate", "--json")
	var invalid struct {
		Valid       bool `json:"valid"`
		Diagnostics []struct {
			Severity string     `json:"severity"`
			Message  string     `json:"message"`
			Source   sourceJSON `json:"source"`
		} `json:"diagnostics"`
	}
	if code != 1 || len(docs) != 1 || docs[0].Data == nil {
		t.Fatalf("invalid validate exited %d: %+v", code, docs)
	}
	if err := json.Unmarshal(docs[0].Data, &invalid); err != nil {
		t.Fatal(err)
	}
	if invalid.Valid || len(invalid.Diagnostics) != 1 || invalid.Diagnostics[0].Severity != "error" ||
		invalid.Diagnostics[0].Source.Line == nil || *invalid.Diagnostics[0].Source.Line != 2 || !strings.HasSuffix(invalid.Diagnostics[0].Source.Path, "agx.yaml") {
		t.Fatalf("invalid validate = %+v", invalid)
	}

	// --json before the command works as after it.
	failJSON(t, 1, codeConfigInvalid, "--json", "resolve")
	if code, docs := runJSON(t, "--json", "config", "validate"); code != 1 || len(docs) != 1 || docs[0].Data == nil {
		t.Fatalf("leading --json validate exited %d: %+v", code, docs)
	}

	d := failJSON(t, 1, codeConfigInvalid, "resolve", "--json")
	if d.Error.Source == nil || d.Error.Source.Line != 2 {
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
	// Adding a definition creates the named file.
	created := filepath.Join(dir, "created.yaml")
	okJSON(t, nil, "--config", created, "instructions", "add", "--id", "x", "--text", "hi")
	if _, err := os.Stat(created); err != nil {
		t.Fatal(err)
	}
	d = failJSON(t, 1, codeNotFound, "resolve", filepath.Join(dir, "nope"), "--json")
	if !strings.Contains(d.Error.Message, "does not exist") {
		t.Fatalf("missing directory: %s", d.Error.Message)
	}
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

	okJSON(t, nil, "resolve")
	okJSON(t, nil, "explain", "--harness", "codex-work")
	failJSON(t, 1, codeNotFound, "resolve", "--harness", "missing", "--json")

	var installed struct {
		Packages  []string `json:"packages"`
		Lockfiles []string `json:"lockfiles"`
		Frozen    bool     `json:"frozen"`
		Updated   bool     `json:"updated"`
	}
	okJSON(t, &installed, "install", "--frozen")
	if installed.Packages == nil || installed.Lockfiles == nil || !installed.Frozen || installed.Updated {
		t.Fatalf("install = %+v", installed)
	}
	okJSON(t, &installed, "update")
	if !installed.Updated {
		t.Fatalf("update = %+v", installed)
	}
	failJSON(t, 1, codeNotFound, "update", "missing", "--json")

	dir := filepath.Join(t.TempDir(), "shims")
	var shims struct {
		Directory string `json:"directory"`
		Changes   []struct {
			Name   string `json:"name"`
			Action string `json:"action"`
		} `json:"changes"`
	}
	okJSON(t, &shims, "shims", "install", "--dir", dir)
	if len(shims.Changes) != 1 || shims.Changes[0].Name != "codex-work" || shims.Changes[0].Action != "created" {
		t.Fatalf("shims install = %+v", shims)
	}
	okJSON(t, nil, "shims", "list", "--dir", dir)
	okJSON(t, &shims, "shims", "remove", "--dir", dir)
	if len(shims.Changes) != 1 || shims.Changes[0].Action != "removed" {
		t.Fatalf("shims remove = %+v", shims)
	}

	// Without a daemon, and with autostart off, daemon-backed commands report it as JSON.
	for _, args := range [][]string{
		{"launch", "list"},
		{"launch", "show", "x"},
		{"launch", "revoke", "x"},
		{"secret", "status"},
		{"secret", "set", "token", "--stdin"},
		{"secret", "logout", "token"},
		{"secret", "login", "token"},
		{"mcp", "test", "fs"},
	} {
		failJSON(t, 1, codeDaemonUnavailable, append(args, "--json")...)
	}
	var status struct {
		Running *bool   `json:"running"`
		Version *string `json:"version"`
	}
	okJSON(t, &status, "daemon", "status", "--json")
	if status.Running == nil || *status.Running || status.Version != nil {
		t.Fatalf("daemon status without a daemon = %+v", status)
	}
	for _, args := range [][]string{{"daemon", "stop"}, {"daemon", "stop", "--force"}} {
		var stopped struct {
			Running  *bool `json:"running"`
			Stopping *bool `json:"stopping"`
		}
		okJSON(t, &stopped, append(args, "--json")...)
		if stopped.Running == nil || *stopped.Running || stopped.Stopping == nil || *stopped.Stopping {
			t.Fatalf("%q without a daemon = %+v", args, stopped)
		}
	}
	failJSON(t, 1, codeFailedPrecondition, "mcp", "login", "fs", "--json")
	failJSON(t, 1, codeNotFound, "mcp", "test", "missing", "--json")
	failJSON(t, 1, codeInvalidRequest, "secret", "set", "token", "--json")
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
	if !wantsJSON(nil, []string{"mcp", "list", "--json"}) || wantsJSON(nil, []string{"x", "--", "--json"}) ||
		wantsJSON(nil, []string{"mcp", "list", "--json=false"}) {
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
	// A drive-letter path on Windows exercises the colon after the drive.
	path := "/etc/agx.yaml"
	if runtime.GOOS == "windows" {
		path = `C:\agx\agx.yaml`
	}
	for message, want := range map[string]*sourceJSON{
		path + ":12: MCP server x: bad":                       {Path: path, Line: jsonInt(12)},
		"configuration is invalid; fix it: " + path + ": bad": {Path: path},
		path + ": yaml: construct errors: line 4: field x":    {Path: path, Line: jsonInt(4)},
		path + ": load error in parser at L5.C1: no content":  {Path: path, Line: jsonInt(5)},
		"package x is not installed; run agx install":         nil,
	} {
		got := errorSource(message)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("errorSource(%q) = %+v, want %+v", message, got, want)
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

func TestLoginEvents(t *testing.T) {
	var out bytes.Buffer
	scripting.enabled, scripting.stdout = true, &out
	t.Cleanup(func() { scripting.enabled, scripting.stdout = false, os.Stdout })
	expires := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := printLoginStarted("github", &v1.LoginStarted{
		AuthorizationUrl: "https://example.com/authorize",
		ExpiresAt:        timestamppb.New(expires),
		McpId:            "github",
	}); err != nil {
		t.Fatal(err)
	}
	if err := loginOutcome("github", &v1.LoginFinished{Outcome: v1.LoginFinished_OUTCOME_SUCCEEDED}); err != nil {
		t.Fatal(err)
	}
	var exit *ExitError
	err := loginOutcome("github", &v1.LoginFinished{Outcome: v1.LoginFinished_OUTCOME_EXPIRED})
	if !errors.As(err, &exit) || exit.Code != 1 {
		t.Fatalf("expired login returned %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	want := []string{
		`{"version":1,"event":"started","secret_id":"github","mcp_id":"github",` +
			`"url":"https://example.com/authorize","expires_at":"2026-01-02T03:04:05Z"}`,
		`{"version":1,"event":"finished","secret_id":"github","outcome":"success","message":null}`,
		`{"version":1,"event":"finished","secret_id":"github","outcome":"expired","message":null}`,
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("events:\n%s\nwant:\n%s", out.String(), strings.Join(want, "\n"))
	}
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
