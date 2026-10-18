# Runtime and configuration contracts

These contracts guide the implementation. See [configuration](configuration.md)
for the concrete schema and [verification](verification.md) for exercised behavior
and current support limits.

## Configuration resolution

Use explicit imports for separate personal, work, and local files. Relative file
paths belong to the declaring fragment. Missing imports fail unless explicitly
optional. Detect import/bundle cycles and conflicting definitions; replacing a
definition requires an explicit override. Do not use generic YAML map merging.

Match literal directory roots and their descendants initially. Canonicalize both
scope roots and launch paths, respect filesystem case behavior, and compare path
components: `/work/app` must not match `/work/apple`. Show canonical paths in
explanations. Machine-local fragments supply different root paths on each OS.

Apply scopes from shallowest to deepest, then import/rule order at equal depth.
Within one rule, expand additions first and removals second. Use sets of
qualified leaf IDs, not reference counts: removing a skill removes it even if
several bundles supplied it. A deeper addition can restore it. Bundle removals
expand to removals of their contents. Sort final output deterministically, except
that selected instructions keep their concatenation order: `order`, then the
resolution step that selected each; record every step for explanation.

Snapshot IDs fingerprint resolved configuration, lock/package content, relevant
platform inputs, canonical directory, and harness. Tokens, timestamps, and injected
values are not fingerprint inputs; secret references and the tag and argument of
each injected value are included.
Do not reread configuration while serving a launch; changes create new snapshots.
Snapshots freeze definitions, not credential refresh or administrative revocation.
Publish only fully validated config reloads. If a reload is invalid, retain the
last valid snapshot for active launches and reject new preparations until fixed.

## Launch preparation and credentials

The wrapper first resolves required environment variable names, then submits only
those allowlisted values with the expected snapshot ID, the harness name, and the
native arguments. The daemon takes the harness definition's arguments,
environment, and executable from its own snapshot, never from the wrapper. This supports credentials
from a development shell that the daemon did not inherit. Reject stale snapshots
before using forwarded values; never capture the wrapper's whole environment.
Injected values resolve at launch and remain in launch memory, not snapshots,
artifacts, or package storage. A missing input produces a preparation error naming
the variable and the configuration location, never a value. Secrets explicitly
using the daemon environment need no forwarding.

Use three separate authorities: private management credentials, wrapper-only
lifecycle credentials, and harness MCP grants. Never forward management or
lifecycle credentials into the harness environment. Store daemon discovery data
and management credentials with per-user filesystem permissions/Windows ACLs.
Authenticate every MCP request against both its launch and selected MCP server;
an MCP session ID or URL path is not authorization.

Take an exclusive per-user startup lock before binding a literal loopback address
on an available port. Publish discovery metadata atomically only after startup;
include an instance ID so clients can recognize restarts. Clear stale metadata
under the same lock. Process IDs alone are insufficient because they are reused.
Clients that start the daemon on demand spawn it detached and rely on this lock:
concurrent starters may each spawn one, the losers exit, and every starter
connects to the winner. A starter whose daemon exited fails only once the lock is
free, reporting the log's last line. A client replaces a daemon of another build
only when it serves the same root, and either keeps its launches for the next
daemon (discovery `restores_launches`) or reports none active; it asks the
daemon to stop without force and waits for the lock before starting its own.

Use opaque, unpredictable launch tokens with server-side validity. Renewal extends
the lease without changing the token already supplied to the harness. Defaults
are a 60-second lease, renewed every 20 seconds; `daemon.lease_seconds` and
`daemon.renew_seconds` override them.
Hash stored token verifiers and redact all sensitive request/response fields.
Repeat mutation requests with the same idempotency key after ambiguous failures.

Active launches survive a daemon restart that does not revoke them
([ADR 0016](adr/0016-launches-survive-daemon-restarts.md)). The daemon writes
`daemon-state.json` in the state directory atomically, mode 0600 (a private ACL on
Windows), whenever a launch or thread launch starts, ends, or changes state, and
for renewals at most every five seconds. It holds the format version, listener
port, configuration root, and per launch its ID, parent, session, snapshot ID,
directory, harness, mode, state, lease, creation and expiry times, and SHA-256
verifiers of its grant and lifecycle token: never tokens, environment inputs, or
native arguments. The next daemon drops expired launches and those of another
root, binds the saved port when it is free, and restores the rest; harnesses keep
their grants. Their wrappers find the new daemon through discovery and resend
environment inputs and native arguments with `RestoreLaunch`, under management
authorization plus the lifecycle token. Endpoints whose values need no inputs
serve before that; the others answer 503. A restored launch must resolve to its
saved snapshot ID: when configuration or packages changed meanwhile, its MCP
servers answer 403 until relaunch, since snapshots freeze definitions. Upstream
MCP sessions do not survive; the endpoint answers 404 for an unknown
`Mcp-Session-Id`, and clients start a new session. When the port is taken, the
daemon listens elsewhere: launches keep their lease and grant, but harnesses
holding the old endpoint URLs lose MCP access until relaunch (a Codex app-server
thread gets new URLs on `thread/resume`). A forced stop revokes all launches and
clears them from the file. Lease loss does not automatically kill a harness.

A stopping daemon shuts down gracefully (see
[graceful shutdown](cli.md#graceful-shutdown)). It refuses new launches, thread
launches, and MCP sessions with Connect `unavailable`, or HTTP 503, carrying
`Agx-Daemon-Stopping: 1`; callers retry those with the daemon discovery names
next. It lets requests in flight finish for `daemon.shutdown_grace_seconds`, or a
stop's `grace`, ends event streams, writes and freezes the state file, closes its
listener, ends upstream MCP sessions, and removes discovery last. A forced stop
drains at most 2 seconds. A second signal skips the rest of the grace period.
Account credentials survive daemon restart according to their configured storage.

## MCP runtime

Keep one downstream MCP session bound to one launch/MCP server and its upstream
session. Validate that session IDs cannot cross endpoints. Preserve native tool
schemas, annotations, structured and image results, resources, prompts, negotiated
capabilities, notifications, cancellation, and upstream errors. Unsupported
capabilities must be negotiated honestly. Do not retry tool calls automatically:
the upstream may already have performed a side effect.

Daemon-owned stdio upstreams receive only their required environment and cwd.
End a stdio session by closing the server's input, then sending SIGTERM to its
process group after 2 seconds, then killing it after 1 more; terminate the
remaining process tree using Unix process groups or Windows Job Objects (Windows
skips SIGTERM). A child cannot receive rotated environment values after startup;
servers needing that must support native refresh/reload or be restarted.

API keys and OAuth state stay daemon-owned, except credentials that upstream child
processes require. Bind cached OAuth credentials to the configured identity,
issuer/client/resource, and server origin. Serialize refresh per credential.
Do not forward agx authorization headers to upstreams. Keep callbacks local,
validate state/PKCE, and provide an explicit headless flow or credential helper.

## Artifacts and native integration

Write caches and generated artifacts atomically, keyed by content and adapter
version. Reject package paths escaping their installation root. Preserve native
skill directory layout and support platform-specific commands. Do not modify
repository files or native account state as a side effect of launching.

Installation may register native plugin marketplace entries. Launch selection
uses per-launch flags/configuration, never global enable/disable races. Keep
artifacts alive while any launch references them. Explicitly verify Codex plugin
discovery and activation for app-server usage; CLI plugin documentation alone is
not proof of that path. A host serving many threads from one process gets one
launch per thread, with resources resolved for the thread's directory and an MCP
grant that ends with the thread. Existing unmanaged global skills remain outside exclusions.
