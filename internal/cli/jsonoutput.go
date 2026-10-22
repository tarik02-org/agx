package cli

import (
	"encoding/json"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
)

// The documents --json prints: the scripting contract (docs/cli-reference.md, JSON output). Documents are built from
// these types only, never by marshaling configuration, runtime, or protobuf types, so the
// contract changes only here. Every type follows the same conventions:
//
//   - Field names are snake_case. An object's own identifier is id (name for harnesses); a
//     reference to another object is <kind>_id.
//   - Enumerations are lowercase snake_case strings.
//   - Times are UTC RFC 3339 strings with second precision, in fields named *_at.
//   - Every field is always present: a value that does not apply is null, an empty list [],
//     and an empty map {}.
//   - A list command wraps its list in an object, under the configuration section's name (mcp,
//     skills, instructions, harnesses) or the plural of the thing (launches, secrets), so the
//     document can gain fields without breaking its readers.

// sourceJSON locates a definition or error in a configuration file. line is null when unknown.
type sourceJSON struct {
	Path string `json:"path"`
	Line *int   `json:"line"`
}

func configSource(s config.Source) sourceJSON {
	return sourceJSON{Path: s.Path, Line: jsonInt(s.Line)}
}

// diagnosticJSON is a problem a document reports without failing: severity is warning or error;
// code classifies it, such as mcp_failed or secret_unavailable, or is null.
type diagnosticJSON struct {
	Severity string      `json:"severity"`
	Code     *string     `json:"code"`
	Message  string      `json:"message"`
	Source   *sourceJSON `json:"source"`
}

var severities = map[v1.Diagnostic_Severity]string{
	v1.Diagnostic_SEVERITY_WARNING: "warning",
	v1.Diagnostic_SEVERITY_ERROR:   "error",
}

func diagnosticsJSON(diagnostics []*v1.Diagnostic) []diagnosticJSON {
	out := []diagnosticJSON{}
	for _, d := range diagnostics {
		item := diagnosticJSON{
			Severity: enumJSON(severities, d.Severity),
			Code:     jsonString(d.Code),
			Message:  d.Summary,
		}
		if d.Source != nil && d.Source.Path != "" {
			item.Source = &sourceJSON{Path: d.Source.Path, Line: jsonInt(int(d.Source.Line))}
		}
		out = append(out, item)
	}
	return out
}

// errorJSON is the error of a failed command; source locates a configuration error when known.
type errorJSON struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Source  *sourceJSON `json:"source"`
}

// config validate

type validationJSON struct {
	Valid bool     `json:"valid"`
	Root  string   `json:"root"`
	Files []string `json:"files"`
	// Counts is null for a configuration that does not load.
	Counts      *definitionCounts `json:"counts"`
	Diagnostics []diagnosticJSON  `json:"diagnostics"`
}

// definitionCounts counts what a configuration defines; on-demand OAuth secrets are implied, not
// defined.
type definitionCounts struct {
	Skills       int `json:"skills"`
	Instructions int `json:"instructions"`
	MCP          int `json:"mcp"`
	Secrets      int `json:"secrets"`
	Harnesses    int `json:"harnesses"`
	Scopes       int `json:"scopes"`
	Packages     int `json:"packages"`
}

// resolve and explain

// resourceJSON names a selected definition: kind is skill, instructions, or mcp. load is an MCP
// server's load mode, native or on_demand, and null for other kinds.
type resourceJSON struct {
	Kind string  `json:"kind"`
	ID   string  `json:"id"`
	Load *string `json:"load"`
	// Share is none, directory, repository, or global for MCP servers, null for other kinds.
	Share *string `json:"share"`
}

func resourceDocument(c *config.Config, r config.Resource) resourceJSON {
	out := resourceJSON{Kind: r.Kind, ID: string(r.ID)}
	if r.Kind == "mcp" {
		out.Load = jsonString(c.MCP[r.ID].Load)
		out.Share = jsonString(c.MCP[r.ID].ShareMode())
	}
	return out
}

type resolveJSON struct {
	ID        string `json:"id"`
	Directory string `json:"directory"`
	// Harness is null when resolved without one.
	Harness             *string        `json:"harness"`
	Resources           []resourceJSON `json:"resources"`
	RequiredEnvironment []string       `json:"required_environment"`
}

type explainJSON struct {
	resolveJSON
	Traces []traceJSON `json:"traces"`
	// Injections are tagged values as configured, never resolved.
	Injections []injectionJSON `json:"injections"`
}

type traceJSON struct {
	Resource resourceJSON `json:"resource"`
	Selected bool         `json:"selected"`
	// Order is a selected instructions definition's sort key; null otherwise.
	Order *int       `json:"order"`
	Steps []stepJSON `json:"steps"`
}

// stepJSON is one rule that added or removed a resource: change is add or remove.
type stepJSON struct {
	Change string     `json:"change"`
	Source sourceJSON `json:"source"`
	// ScopeDirectory is null for a rule without a directory, which applies everywhere.
	ScopeDirectory *string `json:"scope_directory"`
	// Harness names the definition whose rule it is; null for scopes.
	Harness     *string  `json:"harness"`
	BundleChain []string `json:"bundle_chain"`
}

type injectionJSON struct {
	Target string     `json:"target"`
	Value  string     `json:"value"`
	Source sourceJSON `json:"source"`
}

func resolveDocument(snapshot *config.Snapshot) resolveJSON {
	resources := []resourceJSON{}
	for _, r := range snapshot.Resources {
		resources = append(resources, resourceDocument(snapshot.Config, r))
	}
	return resolveJSON{
		ID:                  snapshot.ID,
		Directory:           snapshot.Directory,
		Harness:             jsonString(snapshot.Harness),
		Resources:           resources,
		RequiredEnvironment: jsonList(snapshot.RequiredEnvironment),
	}
}

func explainDocument(snapshot *config.Snapshot) explainJSON {
	traces := []traceJSON{}
	for _, t := range snapshot.Traces {
		steps := []stepJSON{}
		for _, s := range t.Steps {
			chain := []string{}
			for _, id := range s.BundleChain {
				chain = append(chain, string(id))
			}
			steps = append(steps, stepJSON{
				Change:         s.Change,
				Source:         configSource(s.Source),
				ScopeDirectory: jsonString(s.Directory),
				Harness:        jsonString(s.Harness),
				BundleChain:    chain,
			})
		}
		traces = append(traces, traceJSON{
			Resource: resourceDocument(snapshot.Config, t.Resource),
			Selected: t.Selected,
			Order:    jsonInt(t.Order),
			Steps:    steps,
		})
	}
	injections := []injectionJSON{}
	for _, i := range snapshot.Injections {
		injections = append(injections, injectionJSON{Target: i.Target, Value: i.Text, Source: configSource(i.Source)})
	}
	return explainJSON{resolveJSON: resolveDocument(snapshot), Traces: traces, Injections: injections}
}

// install and update

type installJSON struct {
	Packages  []string `json:"packages"`
	Lockfiles []string `json:"lockfiles"`
	Frozen    bool     `json:"frozen"`
	Updated   bool     `json:"updated"`
}

// sessions

type sessionsJSON struct {
	// Scope is here or all.
	Scope     string `json:"scope"`
	Directory string `json:"directory"`
	// Query is null when none was given.
	Query    *string          `json:"query"`
	Sessions []sessionJSON    `json:"sessions"`
	Files    sessionFilesJSON `json:"files"`
	// Errors holds why a harness's sessions could not be listed, by built-in harness name.
	Errors map[string]string `json:"errors"`
}

type sessionJSON struct {
	ID string `json:"id"`
	// Harness resumes the session: the recorded definition, or the built-in.
	Harness   string  `json:"harness"`
	Builtin   string  `json:"builtin"`
	Recorded  bool    `json:"recorded"`
	Directory string  `json:"directory"`
	Title     *string `json:"title"`
	Message   *string `json:"message"`
	StartedAt *string `json:"started_at"`
	UpdatedAt *string `json:"updated_at"`
}

type sessionFilesJSON struct {
	Parsed    int `json:"parsed"`
	Cached    int `json:"cached"`
	Malformed int `json:"malformed"`
}

// mcp, skill, instructions, and harness list

type mcpListJSON struct {
	MCP []mcpServerJSON `json:"mcp"`
}

// mcpServerJSON is an MCP server as configured: transport is stdio or http; url is null for
// stdio, command null for http.
type mcpServerJSON struct {
	ID        string   `json:"id"`
	Transport string   `json:"transport"`
	URL       *string  `json:"url"`
	Command   []string `json:"command"`
	// Auth is null for stdio servers, which take credentials through their environment.
	Auth *mcpAuthJSON `json:"auth"`
	// Load is native or on_demand.
	Load string `json:"load"`
	// Share is none, directory, repository, or global.
	Share       string     `json:"share"`
	Description *string    `json:"description"`
	Inline      bool       `json:"inline"`
	Source      sourceJSON `json:"source"`
}

// mcpAuthJSON is how an HTTP server authenticates: kind is none or the kind of its secret
// (environment, daemon_environment, command, keychain, oauth, or sops). An on-demand secret is
// the OAuth login a server without auth uses once it asks for one.
type mcpAuthJSON struct {
	Kind     string  `json:"kind"`
	SecretID *string `json:"secret_id"`
	OnDemand bool    `json:"on_demand"`
}

type skillListJSON struct {
	Skills []skillJSON `json:"skills"`
}

// skillJSON is a skill as configured: kind is path, generated, git, or package; from is its
// path, Git URL, or package, and null for a generated skill.
type skillJSON struct {
	ID     string     `json:"id"`
	Kind   string     `json:"kind"`
	From   *string    `json:"from"`
	Inline bool       `json:"inline"`
	Source sourceJSON `json:"source"`
}

type instructionsListJSON struct {
	Instructions []instructionsJSON `json:"instructions"`
}

// instructionsJSON is an instructions definition: kind is file, text, git, or package; path is
// null for text; order is null when unset.
type instructionsJSON struct {
	ID     string     `json:"id"`
	Kind   string     `json:"kind"`
	Path   *string    `json:"path"`
	Order  *int       `json:"order"`
	Inline bool       `json:"inline"`
	Source sourceJSON `json:"source"`
}

type harnessListJSON struct {
	Harnesses []harnessJSON `json:"harnesses"`
}

// harnessJSON is a harness: a built-in, with null base and source, or a definition.
type harnessJSON struct {
	Name       string      `json:"name"`
	Base       *string     `json:"base"`
	Builtin    string      `json:"builtin"`
	Executable *string     `json:"executable"`
	Source     *sourceJSON `json:"source"`
}

// editJSON is the document of a command that edits configuration.
type editJSON struct {
	Files    []fileChangeJSON `json:"files"`
	Defined  []definitionJSON `json:"defined"`
	Enabled  []definitionJSON `json:"enabled"`
	Disabled []definitionJSON `json:"disabled"`
	Removed  []definitionJSON `json:"removed"`
	// Scope is where resources were enabled or disabled; null when the command changed no scope.
	Scope *scopeJSON `json:"scope"`
	// NeedsLogin says whether a server mcp add defined asks for an OAuth login; null for other
	// commands.
	NeedsLogin *bool `json:"needs_login"`
}

type fileChangeJSON struct {
	Path    string `json:"path"`
	Diff    string `json:"diff"`
	Written bool   `json:"written"`
}

// definitionJSON names a definition by its configuration section, such as mcp or harnesses.
type definitionJSON struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type scopeJSON struct {
	// Directory is null for everywhere.
	Directory *string  `json:"directory"`
	Harnesses []string `json:"harnesses"`
}

// mcp test

// mcpTestJSON is the outcome of mcp test: state is ready, needs_auth, or failed. A server that
// does not answer has a null server and a message.
type mcpTestJSON struct {
	ID        string             `json:"id"`
	Transport string             `json:"transport"`
	Server    *mcpTestServerJSON `json:"server"`
	Tools     []mcpToolJSON      `json:"tools"`
	State     string             `json:"state"`
	Message   *string            `json:"message"`
}

type mcpTestServerJSON struct {
	Name    *string `json:"name"`
	Version *string `json:"version"`
}

type mcpToolJSON struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

// mcp tools

type mcpToolsListJSON struct {
	MCP []mcpToolsJSON `json:"mcp"`
}

// mcpToolsJSON is an MCP server a call reaches: load is native or on_demand; state is ready,
// needs_auth, or failed, with message saying why; server is null when it did not answer.
type mcpToolsJSON struct {
	ID          string              `json:"id"`
	Description *string             `json:"description"`
	Load        string              `json:"load"`
	State       string              `json:"state"`
	Message     *string             `json:"message"`
	Server      *mcpTestServerJSON  `json:"server"`
	Tools       []mcpToolDetailJSON `json:"tools"`
}

// mcpToolDetailJSON is a tool: signature is TypeScript-like; the schemas are null unless
// --schema asks for them, and output_schema when the tool declares none.
type mcpToolDetailJSON struct {
	Name         string          `json:"name"`
	Title        *string         `json:"title"`
	Description  *string         `json:"description"`
	Signature    string          `json:"signature"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema"`
}

// mcp call

// mcpCallJSON is a tool's result. Content items are textContentJSON, fileContentJSON,
// linkContentJSON, resourceContentJSON, or otherContentJSON, told apart by type;
// structured_content is the tool's structured result, or null.
type mcpCallJSON struct {
	MCPID             string          `json:"mcp_id"`
	Tool              string          `json:"tool"`
	IsError           bool            `json:"is_error"`
	Content           []any           `json:"content"`
	StructuredContent json.RawMessage `json:"structured_content"`
}

type textContentJSON struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// fileContentJSON is image or audio content, written to path; size is in bytes.
type fileContentJSON struct {
	Type     string `json:"type"`
	MIMEType string `json:"mime_type"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
}

type linkContentJSON struct {
	Type        string  `json:"type"`
	URI         string  `json:"uri"`
	Name        string  `json:"name"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
	MIMEType    *string `json:"mime_type"`
	Size        *int64  `json:"size"`
}

// resourceContentJSON is an embedded resource: text for a text resource, otherwise path and
// size of the file its binary content was written to.
type resourceContentJSON struct {
	Type     string  `json:"type"`
	URI      string  `json:"uri"`
	MIMEType *string `json:"mime_type"`
	Text     *string `json:"text"`
	Path     *string `json:"path"`
	Size     *int64  `json:"size"`
}

// otherContentJSON is content of a type this build does not know, as the server sent it.
type otherContentJSON struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

// mcp types

// mcpTypesJSON names the servers mcp types covered and the files it wrote.
type mcpTypesJSON struct {
	MCP   []string `json:"mcp"`
	Files []string `json:"files"`
}

// shims

type shimListJSON struct {
	Directory string     `json:"directory"`
	OnPath    bool       `json:"on_path"`
	Target    string     `json:"target"`
	Shims     []shimJSON `json:"shims"`
}

// shimJSON is a shim: kind is symlink, hardlink, copy, or script, and null for a missing shim;
// state is current, stale, missing, modified, or conflict.
type shimJSON struct {
	Name   string  `json:"name"`
	File   string  `json:"file"`
	Kind   *string `json:"kind"`
	Target *string `json:"target"`
	State  string  `json:"state"`
}

type shimChangesJSON struct {
	Directory string `json:"directory"`
	OnPath    bool   `json:"on_path"`
	// Target is null for shims remove.
	Target  *string          `json:"target"`
	Changes []shimChangeJSON `json:"changes"`
}

// shimChangeJSON is what install or remove did to a shim: action is created, updated, removed,
// unchanged, or kept; state explains a kept shim and is null otherwise.
type shimChangeJSON struct {
	Name   string  `json:"name"`
	File   string  `json:"file"`
	Kind   *string `json:"kind"`
	Target *string `json:"target"`
	Action string  `json:"action"`
	State  *string `json:"state"`
}

// doctor

type doctorJSON struct {
	Directory string            `json:"directory"`
	Root      string            `json:"root"`
	Files     []string          `json:"files"`
	Checks    []checkJSON       `json:"checks"`
	Summary   doctorSummaryJSON `json:"summary"`
}

// checkJSON is one check: status is pass, warn, or fail; hint is null when there is nothing to do.
type checkJSON struct {
	Group  string  `json:"group"`
	Name   string  `json:"name"`
	Status string  `json:"status"`
	Detail string  `json:"detail"`
	Hint   *string `json:"hint"`
}

type doctorSummaryJSON struct {
	Pass int `json:"pass"`
	Warn int `json:"warn"`
	Fail int `json:"fail"`
}

func doctorDocument(report doctorReport) doctorJSON {
	checks := []checkJSON{}
	for _, c := range report.Checks {
		checks = append(checks, checkJSON{
			Group:  c.Group,
			Name:   c.Name,
			Status: string(c.Status),
			Detail: c.Detail,
			Hint:   jsonString(c.Hint),
		})
	}
	return doctorJSON{
		Directory: report.Directory,
		Root:      report.Root,
		Files:     jsonList(report.Files),
		Checks:    checks,
		Summary:   doctorSummaryJSON(report.Summary),
	}
}

// daemon

// daemonStartJSON reports daemon start; started is false when a daemon of this version already ran.
type daemonStartJSON struct {
	Started bool   `json:"started"`
	Version string `json:"version"`
	Log     string `json:"log"`
}

// daemonStatusJSON describes the daemon: without a running one, running is false, version and
// instance_id are null, and diagnostics say when none can start.
type daemonStatusJSON struct {
	Running            bool             `json:"running"`
	Version            *string          `json:"version"`
	InstanceID         *string          `json:"instance_id"`
	LaunchCount        int              `json:"launch_count"`
	RunningLaunchCount int              `json:"running_launch_count"`
	MCPServerCount     int              `json:"mcp_server_count"`
	Diagnostics        []diagnosticJSON `json:"diagnostics"`
}

func daemonStatusDocument(view *v1.RuntimeView) daemonStatusJSON {
	running := 0
	for _, l := range view.Launches {
		if l.State == v1.Launch_STATE_RUNNING {
			running++
		}
	}
	return daemonStatusJSON{
		Running:            true,
		Version:            jsonString(view.Version),
		InstanceID:         jsonString(view.DaemonInstanceId),
		LaunchCount:        len(view.Launches),
		RunningLaunchCount: running,
		MCPServerCount:     len(view.McpServers),
		Diagnostics:        diagnosticsJSON(view.Diagnostics),
	}
}

// daemonStopJSON reports daemon stop: running says whether a daemon was running; with none,
// stopping and forced are false and version is null.
type daemonStopJSON struct {
	Running  bool    `json:"running"`
	Stopping bool    `json:"stopping"`
	Forced   bool    `json:"forced"`
	Version  *string `json:"version"`
	// KeptLaunches counts the active launches the next daemon restores.
	KeptLaunches int `json:"kept_launches"`
}

// daemonRestartJSON reports daemon restart: stopped says whether a daemon was running; version is
// the new daemon's; restored counts the active launches it restored.
type daemonRestartJSON struct {
	Stopped  bool   `json:"stopped"`
	Started  bool   `json:"started"`
	Version  string `json:"version"`
	Restored int    `json:"restored"`
}

// launch

type launchListJSON struct {
	Launches []launchJSON `json:"launches"`
}

// launchJSON is a launch: state is prepared, running, finished, revoked, or expired. A thread
// launch has the launch of the harness process serving it as its parent.
type launchJSON struct {
	ID             string          `json:"id"`
	Harness        string          `json:"harness"`
	Mode           string          `json:"mode"`
	State          string          `json:"state"`
	Directory      string          `json:"directory"`
	SnapshotID     string          `json:"snapshot_id"`
	ExpiresAt      *string         `json:"expires_at"`
	ParentLaunchID *string         `json:"parent_launch_id"`
	SessionID      *string         `json:"session_id"`
	MCPServers     []launchMCPJSON `json:"mcp_servers"`
}

// launchMCPJSON is an MCP server of a launch: state is connecting, ready, needs_auth, or failed.
type launchMCPJSON struct {
	MCPID       string           `json:"mcp_id"`
	State       string           `json:"state"`
	Diagnostics []diagnosticJSON `json:"diagnostics"`
}

var launchStates = map[v1.Launch_State]string{
	v1.Launch_STATE_PREPARED: "prepared",
	v1.Launch_STATE_RUNNING:  "running",
	v1.Launch_STATE_FINISHED: "finished",
	v1.Launch_STATE_REVOKED:  "revoked",
	v1.Launch_STATE_EXPIRED:  "expired",
}

var mcpStates = map[v1.McpRuntime_State]string{
	v1.McpRuntime_STATE_CONNECTING: "connecting",
	v1.McpRuntime_STATE_READY:      "ready",
	v1.McpRuntime_STATE_NEEDS_AUTH: "needs_auth",
	v1.McpRuntime_STATE_FAILED:     "failed",
}

func launchDocument(l *v1.Launch, servers []*v1.McpRuntime) launchJSON {
	mcp := []launchMCPJSON{}
	for _, s := range servers {
		if s.LaunchId != l.Id {
			continue
		}
		mcp = append(mcp, launchMCPJSON{
			MCPID:       s.McpId,
			State:       enumJSON(mcpStates, s.State),
			Diagnostics: diagnosticsJSON(s.Diagnostics),
		})
	}
	return launchJSON{
		ID:             l.Id,
		Harness:        l.Harness,
		Mode:           l.Mode,
		State:          enumJSON(launchStates, l.State),
		Directory:      l.Directory,
		SnapshotID:     l.SnapshotId,
		ExpiresAt:      jsonTimestamp(l.ExpiresAt),
		ParentLaunchID: jsonString(l.ParentLaunchId),
		SessionID:      jsonString(l.SessionId),
		MCPServers:     mcp,
	}
}

func launchListDocument(view *v1.RuntimeView) launchListJSON {
	launches := []launchJSON{}
	for _, l := range view.Launches {
		launches = append(launches, launchDocument(l, view.McpServers))
	}
	return launchListJSON{Launches: launches}
}

type launchRevokeJSON struct {
	LaunchID string `json:"launch_id"`
	Revoked  bool   `json:"revoked"`
}

// secret

type secretStatusJSON struct {
	Secrets []secretJSON `json:"secrets"`
}

// secretJSON is a secret's state: unconfigured, pending, ready, needs_login, or unavailable. kind
// is environment, daemon_environment, command, keychain, oauth, or sops. mcp_id names the MCP
// server whose auth declares the secret inline; on_demand marks the OAuth login of an HTTP
// server without auth.
type secretJSON struct {
	ID          string           `json:"id"`
	Kind        string           `json:"kind"`
	State       string           `json:"state"`
	ExpiresAt   *string          `json:"expires_at"`
	MCPID       *string          `json:"mcp_id"`
	OnDemand    bool             `json:"on_demand"`
	Diagnostics []diagnosticJSON `json:"diagnostics"`
}

var secretStates = map[v1.SecretStatus_State]string{
	v1.SecretStatus_STATE_UNCONFIGURED: "unconfigured",
	v1.SecretStatus_STATE_PENDING:      "pending",
	v1.SecretStatus_STATE_READY:        "ready",
	v1.SecretStatus_STATE_NEEDS_LOGIN:  "needs_login",
	v1.SecretStatus_STATE_UNAVAILABLE:  "unavailable",
}

func secretStatusDocument(statuses []*v1.SecretStatus) secretStatusJSON {
	secrets := []secretJSON{}
	for _, s := range statuses {
		secrets = append(secrets, secretJSON{
			ID:          s.SecretId,
			Kind:        s.Kind,
			State:       enumJSON(secretStates, s.State),
			ExpiresAt:   jsonTimestamp(s.ExpiresAt),
			MCPID:       jsonString(s.McpId),
			OnDemand:    s.OnDemand,
			Diagnostics: diagnosticsJSON(s.Diagnostics),
		})
	}
	return secretStatusJSON{Secrets: secrets}
}

type secretSetJSON struct {
	SecretID string `json:"secret_id"`
	Stored   bool   `json:"stored"`
}

type secretLogoutJSON struct {
	SecretID  string `json:"secret_id"`
	LoggedOut bool   `json:"logged_out"`
}

// Login events, one per line: started, then finished.

type loginStartedJSON struct {
	event
	SecretID string `json:"secret_id"`
	// MCPID names the MCP server whose URL the login authorizes, or is null.
	MCPID     *string `json:"mcp_id"`
	URL       string  `json:"url"`
	ExpiresAt *string `json:"expires_at"`
}

// loginFinishedJSON ends a login: outcome is success, failure, or expired.
type loginFinishedJSON struct {
	event
	SecretID string  `json:"secret_id"`
	Outcome  string  `json:"outcome"`
	Message  *string `json:"message"`
}

// Values

// enumJSON names a protobuf enumeration value; one this build does not know, or an unset one, is
// unknown.
func enumJSON[E comparable](names map[E]string, value E) string {
	if name, ok := names[value]; ok {
		return name
	}
	return "unknown"
}

func jsonString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// jsonInt is null for zero, which marks an unset line or order.
func jsonInt(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

func jsonTime(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

func jsonTimestamp(t *timestamppb.Timestamp) *string {
	if t == nil {
		return nil
	}
	return jsonTime(t.AsTime())
}

func jsonList[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
