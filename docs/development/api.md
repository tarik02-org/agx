# Management API

The contract is [agx.v1](../../api/agx/v1/agx.proto), exposed through Connect over
authenticated loopback HTTP. Generated Go/Connect and TypeScript bindings are included; `buf generate`
regenerates them. The MCP
endpoints are separate native MCP interfaces, not methods on these services.
The Go module path is `github.com/tarik02-org/agx`.

## Services

- `ConfigurationService`: validate the daemon's configured root and resolve a
  directory for an optional harness, including provenance and required
  environment names.
- `PackageService`: start install/update operations and watch their progress.
  The CLI installs in-process instead; both paths share the installer lock.
- `LaunchService`: prepare, renew, finish, and administratively revoke a launch;
  resolve and prepare thread launches of a process serving many threads; restore
  a launch a restarted daemon brought back.
- `RuntimeService`: inspect/watch redacted daemon, launch, and MCP server state,
  and `TestMcpServer`, which connects to one MCP server outside any launch and
  lists its tools with their descriptions (`agx mcp test`). It accepts only the environment variables the
  server's values name and reports failures with the endpoint's diagnostics.
- `SecretService`: log in with OAuth, set a keychain secret, inspect secret
  state, and log out.

There is deliberately no client-facing `GetCredential` or `CallTool` RPC.
Upstream credentials and tool execution remain inside the daemon.

## Launch flow

Resolve the working directory locally or through the daemon. `PrepareLaunch`
supplies the expected snapshot, harness name, original native arguments, and only
declared environment inputs, plus the variables the harness's adapter forwards
(Codex: `CODEX_HOME`, empty when unset), which locate the harness's own
configuration. The daemon looks up the harness definition, classifies
the mode, rejects passthrough invocations, and resolves injected values; a missing
input fails preparation with its configuration location. It returns a complete executable/argv/cwd plan,
environment changes containing the harness grant, a separate lifecycle token,
and the renewal interval. For Claude Code reading stream-json input, the plan's
`initialize_system_prompt` holds the instructions the wrapper prepends to the
`appendSystemPrompt` of the first `initialize` control request, which Claude
Code otherwise uses in place of `--append-system-prompt-file`.
Treat the response as sensitive; never expose it through ordinary JSON inspection.

`PrepareLaunch` creates a leased, prepared launch. The wrapper spawns the returned
executable and immediately renews with the harness PID, transitioning it to
running. PIDs are diagnostic only. Renew/finish authenticate using the lifecycle
token in request metadata. Prepared launches also expire, covering a wrapper that
dies before spawning. Spawn failure is an explicit finish outcome.

A harness process serving many threads (stdio `codex app-server`) has a launch
without snapshot resources. For each thread, the wrapper calls `ResolveThread`
with the parent launch and directory, which returns the snapshot, its required
environment names, and the native plugins the thread keeps disabled, then
`PrepareThreadLaunch` with the snapshot ID and declared inputs. That returns a
thread launch with its own MCP endpoints and grant, and a `ThreadPlan`: native
configuration overrides as JSON (sensitive, since the grant travels in MCP
headers), instructions, and disabled plugins. A thread launch shares its
parent's lease and lifecycle token. Renewing it with `session_id` reports its
thread and marks it running; it finishes with outcome `thread_closed`, and ends
with its parent however the parent ends.

A daemon stopped without force keeps its active launches for the next one
([ADR 0016](../adr/0016-launches-survive-daemon-restarts.md)). When renewal fails,
the wrapper rereads discovery and renews with the daemon it names, whatever its
build, until its lease runs out. `RenewLaunchResponse.restore_launch_ids` lists
the launch and its thread launches that the new daemon restored and that wait for
`RestoreLaunch`: management authorization, the lifecycle token in the message, and
the environment inputs (plus, for the launch itself, the native arguments) it was
prepared with, which the state file never holds. Until then a restored process
serving threads answers `Unavailable` to thread preparation.

Use management authorization for other RPCs. Preparation and package mutations
require an idempotency key: retries within the same daemon instance return the
same operation/launch instead of duplicating effects. Renew and finish are
idempotent; a terminal launch cannot be revived. Revoke releases its MCP runtime
without signaling the harness process.
Reject reuse of an idempotency key with different parameters.

## Watches and failures

Runtime watches start with a complete redacted view and send replacement views
when state changes. Clients reconnect by requesting a fresh view; v1 does not
promise durable event replay. Slow observers must not block MCP traffic.
Package-operation watches likewise emit current state through a terminal outcome.
Losing an observer does not cancel package installation. Daemon restart may abort
operations; atomic cache/lock writes allow a safe retry.

`Login` is server-streaming and takes a secret ID: a top-level OAuth secret or an
MCP server with inline or on-demand OAuth. Its first message, `started`, carries
the authorization URL, its expiry, and the MCP server whose URL is authorized;
its last, `finished`, reports success, failure with a redacted reason, or expiry.
Clients open the URL and keep the stream open; the daemon receives the callback.
Concurrent logins for the same secret share one attempt. Ending the stream early
withdraws the caller, and the daemon cancels the attempt once no caller waits,
freeing its state; a later callback gets an error page and changes nothing. A
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
