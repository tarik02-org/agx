# Management API

The contract is [agx.v1](../../api/agx/v1/agx.proto), exposed through Connect over
authenticated loopback HTTP. Generated Go/Connect and TypeScript bindings are included; `buf generate`
regenerates them. The MCP
endpoints are separate native MCP interfaces, not methods on these services.
The Go module path is `github.com/tarik02-org/agx`.

## Services

- `ConfigurationService`: validate the daemon's configured root and resolve a
  directory for an optional harness, including provenance and required
  environment names. `GetLocalConfiguration` supplies managed definitions and typed
  package locks for CLI previews, without exposing a database connection. Its web-UI APIs browse configuration, definitions, skill
  files and directories. `UpdateConfiguration` validates and commits semantic
  local edits at an expected revision; `WriteResourceFile` separately replaces
  a configured resource file. YAML configuration editing is removed.
- `PackageService`: web-UI APIs to install packages, streaming progress and then
  the result, and to preview and inspect Git sources, read preview files, and
  list source refs. CLI install/update use the same RPC, including selected updates
  and update-all. `packages_only` lets the CLI perform native plugin installation
  with its own PATH and harness environment after the daemon saves package locks.
- `LaunchService`: prepare, renew, finish, and administratively revoke a launch;
  resolve and prepare thread launches of a process serving many threads; attach
  a launch, with its thread launches, to a daemon that does not hold it.
  `RecordLaunch` saves idempotent start/completion history independently of leases
  and configuration loading, so a replacement can record a finished wrapper.
- `RuntimeService`: inspect redacted daemon, launch, and MCP server state, with
  `WatchRuntime` supplying live views for the web UI;
  `ListSessions` supplying cached and refreshed session history for the caller's
  explicit store roots;
  `ListTools` and `CallTool`, which list and call the
  tools of the MCP servers in a call scope (`agx mcp tools`, `agx mcp call`, and
  the client of `agx mcp types`); list and stop MCP server processes; and `Stop`,
  which stops the daemon.
- `SecretService`: log in with OAuth, set a keychain secret, inspect secret
  state, rotate a credential, and log out.

`ResolvePassthrough` accepts a harness, native arguments classified as passthrough,
and only the environment inputs that definition references. It returns configured
arguments and environment to the wrapper; no launch, lease, or MCP endpoints are
created. This response is sensitive, like `PrepareLaunch`'s spawn environment.
OAuth secrets remain restricted to HTTP MCP authentication.

There is deliberately no client-facing `GetCredential` RPC. Upstream
MCP credentials and tool execution remain inside the daemon: `CallTool` returns only
the tool's result ([ADR 0022](../adr/0022-on-demand-mcp-calls.md)).

## Tool calls

`ListTools` and `CallTool` take an `McpCallScope`. With `launch_id`, the request
carries that launch's grant (`AGX_MCP_GRANT`) or management authorization and
reaches the servers of the launch's snapshot, native and on-demand, through the
endpoints the launch holds, with its own values and environment inputs. Without
it, the request carries management authorization and reaches the servers
selected for `directory`, or every configured server with `all`; only the
environment inputs those servers' values read are accepted. A grant for an
ended, expired, or unknown launch is `Unauthenticated`.

`ListTools` with `mcp_id` describes one server and fails with its error;
without, it describes every server in scope, each with its state and a message
instead of failing. Tools carry their input and output JSON Schemas as JSON
text. `CallTool` returns the result's content items as a JSON array, binary data
in base64, and its structured content as JSON. An unknown server is `NotFound`,
one outside the scope `NotFound` naming the scope; an unknown tool, or arguments
the server rejects as invalid params, `InvalidArgument` with the closest tool
names; a server needing a login `Unauthenticated` with the `agx secret login`
command; and other failures of the server `FailedPrecondition`. A request without
a deadline gets one: 45 seconds for listing, 10 minutes for a call.

The daemon keeps one client session per launch, or per CLI scope, and server
(`clientPool`), opened over in-memory connections to the endpoint so nothing
else can reach it. Sessions close after five idle minutes, when their launch
ends, and when the daemon stops; a call whose session broke, as after a login
reset the endpoint, opens a new one once.

## Local configuration

`GetConfiguration` returns YAML overlay paths and the local configuration
revision. `GetLocalConfiguration` returns the editable local document, revision,
and typed package locks, including data hidden by YAML overlays. SQL access and
migrations stay in daemon processes; CLI previews combine this data with YAML
without a state directory, and commits use `UpdateConfiguration`.
`ListDefinitions` marks YAML/package definitions read-only; local
ones include their editable `google.protobuf.Value`, with `source.local` set.
Injected-value fields use `{"$env":"NAME"}`, `{"$file":"PATH"}`, or
`{"$secret":"ID"}`; literals remain strings. Definitions may use supported
scalar shorthand as well as objects. Native settings support only the `$file`
marker; skill frontmatter stays literal, including marker-shaped objects.

`UpdateConfiguration` accepts an ordered batch and `expected_revision`.
Validation or a stale revision leaves every local change unwritten. A successful
response reports the new revision. There are no YAML strings, fragment targets,
file plans, SHA comparisons, or diff RPCs. Adding a package can populate the
content cache before commit; cache contents are not part of the configuration
transaction. Overlay package locks must already be current (`agx install`).

`ReadFile` and `WriteResourceFile` only expose configured skill/instructions
contents. Package/cache contents are readable but read-only; YAML configuration
files cannot be read or written through these methods. `Logout` with `all`
clears stored credentials, including orphans, without deleting local config.

## Launch flow

Resolve the working directory locally or through the daemon. The wrapper
resolves the native executable on its own `PATH`, or takes the configured absolute
path. `PrepareLaunch` supplies the expected snapshot, harness name, original native
arguments, that executable, and only declared environment inputs, plus the
variables the harness's adapter forwards (Codex: `CODEX_HOME`, empty when unset),
which locate the harness's own configuration. The daemon looks up the harness definition, classifies
the mode, rejects passthrough invocations, and resolves injected values; a missing
input fails preparation with its configuration location. It returns an argv/cwd plan,
environment changes containing the harness grant, a separate lifecycle token,
and the renewal interval. For Claude Code reading stream-json input, the plan's
`initialize_system_prompt` holds the instructions the wrapper prepends to the
`appendSystemPrompt` of the first `initialize` control request, which Claude
Code otherwise uses in place of `--append-system-prompt-file`.
Treat the response as sensitive; never expose it through ordinary JSON inspection.

`PrepareLaunch` creates a leased, prepared launch. The wrapper spawns the
executable it resolved with the returned arguments and immediately renews with
the harness PID, transitioning it to running. PIDs are diagnostic only. Renew/finish authenticate using the lifecycle
token in request metadata. Prepared launches also expire, covering a wrapper that
dies before spawning. Spawn failure is an explicit finish outcome.

A harness process serving many threads (stdio `codex app-server`) has a launch
without snapshot resources. For each thread, the wrapper calls `ResolveThread`
with the parent launch and directory, which returns the snapshot, its required
environment names, and the native plugins the thread keeps disabled
(`snapshot_only` leaves those out), then
`PrepareThreadLaunch` with the snapshot ID and declared inputs. That returns a
thread launch with its own MCP endpoints and grant, and a `ThreadPlan`: native
configuration overrides as JSON (sensitive, since the grant travels in MCP
headers), instructions, and disabled plugins. A thread launch shares its
parent's lease and lifecycle token. Renewing it with `session_id` reports its
thread and marks it running; it finishes with outcome `thread_closed`, and ends
with its parent however the parent ends.

Launches outlive the daemon that prepared them
([ADR 0016](../adr/0016-launches-survive-daemon-restarts.md)). When renewal fails,
the wrapper rereads discovery and renews with the daemon it names, whatever its
version. When that daemon does not hold the launch, or let it expire, the wrapper
sends `AttachLaunch` with management authorization: the launch, its grant and
lifecycle token, the environment inputs, native arguments, and executable it was
prepared with, and its active thread launches with their grants
(`PrepareThreadLaunchResponse.grant`) and inputs. Attaching is idempotent; a
revoked launch is refused. Until a launch is attached, its MCP endpoints answer
503 with `Retry-After`.

Use management authorization for other RPCs. Preparation is not idempotent: a
retried preparation prepares another launch. Renew and finish are idempotent; a
terminal launch cannot be revived. Revoke releases its MCP runtime without
signaling the harness process.

## Watches and failures

`ListSessions` requires management authorization and accepts absolute store roots
keyed by built-in harness name. The caller resolves its environment and Pi
settings before sending them; the daemon never substitutes its own store paths.
The stream sends an existing cached listing, when available, followed by the
result of a new scan marked `fresh`, then ends. `fresh_only` omits the interim
cached response, as used by `agx sessions`. Failed stores are named in `errors`
and retain their last known entries. Cancelling a caller stops its wait without
cancelling a refresh shared with other callers. Nothing is indexed at daemon
startup or periodically while the picker is closed.

`GetRuntime` with `probe_only` answers reachability checks with an empty response
without constructing a runtime view.

Runtime watches start with a complete redacted view and send replacement views
when state changes, at most one per 250 ms. A renewal that only extends a lease
sends none, so a launch's `expires_at` in a watched view may lag; `GetRuntime`
reports it current. Clients reconnect by requesting a fresh view; v1 does not
promise durable event replay. Slow observers must not block MCP traffic.
`Install` streams progress through a terminal outcome. Losing the caller does not
cancel the installation. Daemon restart may abort it; atomic cache/lock writes
allow a safe retry.

`Login` is server-streaming and takes a secret ID: a top-level OAuth secret or an
MCP server with inline or on-demand OAuth. Its first message, `started`, carries
the authorization URL, its expiry, and the MCP server whose URL is authorized;
its last, `finished`, reports success, failure with a redacted reason, or expiry.
Clients open the URL and keep the stream open; the daemon receives the callback.
A new login for the same secret cancels the previous attempt. Ending the stream
early cancels its attempt, freeing its state; a later callback gets an error
page and changes nothing. A
stopping daemon ends the stream with `Unavailable`. Like `WatchRuntime`, it
requires management authorization. Redact authorization URLs from logs.

Use standard Connect codes: `InvalidArgument` for malformed inputs,
`Unauthenticated` for missing/expired authorization, `PermissionDenied` for an
out-of-scope action, `NotFound` for unknown IDs, `Aborted` for stale snapshots,
`FailedPrecondition` for missing packages/configuration incompatibility, and
`Unavailable` for a stopped runtime. Return redacted diagnostics with source
locations. Reserve `Internal` for defects, not unclassified expected errors.

Generated messages are boundary data. Decode them into validated internal IDs,
paths, and variants; reject unspecified enums and unset `oneof` fields. Status
and explanation messages must never include secret values or raw native prompts.
MCP clients receive native MCP/HTTP errors rather than Connect envelopes.
