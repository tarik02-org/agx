# Verification and current support

Implementation and verification performed on macOS arm64, 2026-10-16.
Local verification scripts and transcripts are working artifacts under `.audit/`;
they are excluded from version control. No CI or permanent test suite was added.

## Exercised behavior

Configuration/package verification covers ordered imports, optional missing
imports, import and bundle cycles, explicit overrides, nested package namespaces,
parent inheritance, leaf removals, deeper re-additions, component-based matching,
symlink canonicalization, provenance, and content-sensitive snapshot IDs. Local
and Git packages install into content-addressed entries. Frozen installs preserve
locks and reject changed local sources; ordinary Git installs retain the locked
commit, and explicit updates advance it. Escaping symlinks fail without changing
existing locks or leaving a partial installation.

Inline definitions are checked in every form: path, generated, and Git skills,
instruction files and text, and inline MCP servers, with ID collisions reported
at both locations. Generated skills are checked for their `SKILL.md` front matter,
executable scripts, and copied files and directories, and for rejected `files`
keys and names. Pinned sources write no lock entries: a `hash` mismatch fails
installation, `/nix/store` and rev-pinned sources need no lockfile, and frozen
installs pass without one; unpinned Git skills lock and update like packages.
Instruction concatenation follows `order` and resolution order. `~` expands in
scope directories, imports, lockfiles, package and resource paths, `!file`,
harness executables, and stdio MCP commands and working directories; `~user`,
`version`, and `exclude` are rejected.

Isolated loopback fixtures exercise authenticated Connect management and separate
stdio/HTTP MCP endpoints. Checks cover native schemas and annotations, structured
and image results, resources, prompts, progress notifications, cancellation,
MCP server/session isolation, lifecycle/MCP authority separation, idempotency and
conflicting keys, redacted inspection, invalid configuration reloads, revocation,
lease renewal/expiry, wrapper exit status, and interrupt forwarding. Runtime and
package-operation watches expose replacement/current state without credentials.

The installed native harnesses were also exercised using temporary account homes:

- Codex 0.162.1: app-server discovers a selected plugin skill, reads always-on
  developer instructions, discovers and executes a native MCP tool, and removes
  the skill for a child directory. A generated skill is discovered with its
  script, copied file, and copied directory. No model request is required for these RPCs.
- Claude Code 2.1.300: a local Anthropic-compatible model fixture observes the
  selected skill and appended instructions, requests a native MCP tool, receives
  its result, and completes the turn.
- Pi 1.2.0: a local OpenAI-compatible model fixture observes the selected skill
  and appended instructions, requests a session-registered native MCP tool,
  receives its result, and completes the turn. RPC skill discovery was also checked.

OAuth verification uses an isolated issuer/callback and the keyring library's
in-memory provider. It checks discovery, pending-login reuse, PKCE S256, invalid
state, callback replay rejection, concurrent refresh, persisted token rotation,
server-origin binding, logout, and failure without plaintext fallback; for
on-demand OAuth, discovery from a `401` challenge's `resource_metadata` with RFC
9728 resource validation, cancellation freeing the pending attempt, expiry, and
separate keychain items for inline and top-level secrets. It does not access the
user's system keychain or authorize an external account.

A loopback end-to-end check runs the daemon against a fake protected MCP server
and authorization server: an HTTP entry without `auth` reports `needs-auth` after
the `401`, `agx secret login --no-browser` blocks until the printed URL is
followed and exits 0, MCP calls then carry the bearer token, `auth: none` keeps
the `401` an ordinary failure, SIGINT exits 130 and leaves no pending attempt, and
a daemon stopping mid-login fails the command. This check uses the system
keychain and logs out afterwards.

## Build checks

`go vet ./...`, `buf lint`, and `buf generate` pass. Go/Connect and TypeScript
bindings are generated from the same schema. Race-enabled local verification
covers the runtime/auth paths. Binaries cross-compile for Linux amd64 and Windows
amd64 with `CGO_ENABLED=0`.

## Limits

Native Linux/Windows runtime behavior and real OS keychain/issuer integration
still need verification on those systems. Windows code uses protected ACLs and
Job Objects, assigning stdio children to their jobs before resuming them. Native
Windows harness executables must be `.exe` files; PATH lookup skips `.cmd`/`.bat`
wrappers and reports an actionable error when nothing else matches. Git
checkouts without symlink support store links as files, so their package hashes
can differ from other platforms.

Endpoints currently use the stateful MCP initialize handshake. The stdio bridge
accepts protocol versions through 2025-11-25; the newer stateless discovery
protocol is not implemented. HTTP forwarding preserves upstream responses and
SSE streams without rewriting tool catalogs or retrying calls.

OAuth currently supports public clients, with optional dynamic registration.
Live registration and confidential-client methods have not been exercised.
OAuth secrets apply to HTTP MCP servers' `auth`; injected credentials can use
keychain, environment, or command secrets. Logout applies to keychain and OAuth
secrets; other kinds reject it. A daemon restart invalidates active grants.

The daemon reloads configuration on demand (resolution, launches, status) and
skips the load while every file, glob, lockfile, and package cache entry it read
keeps its modification time and size; it publishes only valid configurations.
An invalid reload blocks new preparations while existing launches retain their
snapshots. Ended launches and package operations are pruned after an hour;
idle MCP sessions after 30 minutes. Package caches and generated artifacts are retained; automatic garbage
collection, upstream pooling, registries, a UI, and automatic native binary updates
are deferred. Native resume arguments pass through; exhaustive resumption and
interactive terminal/job-control scenarios have not been exercised.

Managed instructions use Codex's `developer_instructions` override, Claude's
`--append-system-prompt-file`, and Pi's `--append-system-prompt`. The Codex override
replaces that native configuration field for the launch and is passed on the
command line, so very large instructions can exceed Windows' command-line limit; native project instruction
file discovery continues. Existing unmanaged global skills are outside exclusions.
