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
Within one rule, expand additions first and exclusions second. Use sets of
qualified leaf IDs, not reference counts: excluding a skill removes it even if
several bundles supplied it. A deeper addition can restore it. Bundle exclusions
expand to exclusions of their contents. Sort final output deterministically;
record every step for explanation.

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

Use opaque, unpredictable launch tokens with server-side validity. Renewal extends
the lease without changing the token already supplied to the harness. Defaults
are a 60-second lease, renewed every 20 seconds; `daemon.lease_seconds` and
`daemon.renew_seconds` override them.
Hash stored token verifiers and redact all sensitive request/response fields.
Repeat mutation requests with the same idempotency key after ambiguous failures.

A daemon restart invalidates active grants in v1. Existing harnesses can keep doing
non-MCP work, but need relaunching for managed MCP access. Lease loss does not
automatically kill a harness. Account credentials survive daemon restart according
to their configured storage; launches and tokens do not.

## MCP runtime

Keep one downstream MCP session bound to one launch/MCP server and its upstream
session. Validate that session IDs cannot cross endpoints. Preserve native tool
schemas, annotations, structured and image results, resources, prompts, negotiated
capabilities, notifications, cancellation, and upstream errors. Unsupported
capabilities must be negotiated honestly. Do not retry tool calls automatically:
the upstream may already have performed a side effect.

Daemon-owned stdio upstreams receive only their required environment and cwd.
Terminate their process trees on shutdown using Unix process groups or Windows
Job Objects. A child cannot receive rotated environment values after startup;
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
not proof of that path. Existing unmanaged global skills remain outside exclusions.
