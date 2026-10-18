# Verification and current support

Behavior is verified by the Go test suite, which CI runs on every push and pull
request on Linux, macOS, and Windows (`.github/workflows/ci.yml`):

```sh
go test ./...                          # unit tests; -race on Linux in CI
go test -tags e2e ./internal/e2e/...   # end to end, against the built binary
```

Tests run in temporary directories with `HOME`, `USERPROFILE`, the `AGX_*`
paths, the XDG directories, and the harness stores (`CODEX_HOME`,
`CLAUDE_CONFIG_DIR`, `PI_CODING_AGENT_DIR`) pointed inside them. They need no
network and never touch the user's keychain: OAuth and keychain tests use the
keyring library's in-memory provider. CI also lints for Linux and Windows,
checks that `buf generate` and `go generate` leave no diff, and builds the Nix
package on Linux and macOS.

## What the tests cover

- **Configuration** (`internal/config`): import order, optional imports, glob
  drop-ins and the default root, import cycles and errors inside imports;
  override precedence between fragments and between configuration and packages;
  package manifest rules and namespaces; inline definitions in every form and ID
  collisions reported at both locations; generated skill validation; pinned
  sources (`rev`, `hash`, `/nix/store`) and lock requirements; `~` expansion in
  every place it applies, with `~user` rejected; scope resolution, removals,
  re-additions, harness filters, symlink canonicalization, and traces;
  instruction order; snapshot identity (content-sensitive, independent of line
  numbers and `!file` content).
- **Harness definitions**: field-by-field merging across fragments, set-once
  conflicts, `override`, base chains, argument groups per mode, and validation
  (cycles, unknown bases, shadowed commands, modes of another built-in).
- **Injected values**: tag placement, resolution per mode, errors naming the
  location but not the value, and that resolved values never appear in snapshot
  or configuration JSON.
- **Editing** (`internal/config/edit`): exact text after edits, with comments,
  blank lines, quoting, flow style, and tags kept; reference removal and pruning;
  writability checks and mode-preserving writes.
- **Adapters**: mode classification of native arguments for each harness,
  Codex `-C`/`--cd` rewriting, and the exact argv, environment, unset variables,
  and artifacts `adapters.Build` produces for Codex, Claude Code, and Pi; Codex
  thread plans; the Agent SDK `initialize` merge; and the app-server relay's
  request, response, and notification rewrites against a fake host.
- **Sessions**: parsing of small crafted Codex, Claude Code, and Pi stores in
  `testdata/` (titles, first messages, subagents, malformed files, titles beyond
  the read head), the parse cache, launch record matching and pruning, search.
- **Shims**: install, sync, stale targets, scripts, and that files agx did not
  create or that changed since are never replaced or removed.
- **OAuth** (`internal/auth`): PKCE S256, state and callback replay rejection,
  pending-login sharing, serialized refresh with rotation, origin binding,
  logout, on-demand OAuth from a `401` with RFC 9728 resource validation,
  cancellation, expiry, keychain item separation, and no plaintext fallback.
- **End to end** (`internal/e2e`): builds `agx`, a fake harness, and a fake stdio
  MCP server (Go programs, so they run on every OS), then checks `--version`,
  passthrough without a daemon, a managed launch through a harness definition
  with its skill and instructions artifacts, an MCP tool call through the daemon
  with the launch grant, the finished launch revoking that grant, the Agent SDK
  `initialize` merge on the harness's input, native exit status, and daemon
  stop. The fake harness also serves a minimal Codex app server: two threads in
  different scopes get their own MCP servers, grants, instructions, and disabled
  plugins, and closing a thread revokes its grant only.

## Exercised manually

The real native harnesses were exercised on macOS arm64 with temporary account
homes: Codex 0.162.1 (app-server discovers a selected plugin skill, reads
always-on developer instructions, and calls a managed MCP tool), Claude Code
2.1.300 and Pi 1.2.0 (a local model fixture observes the skill and instructions,
requests a managed MCP tool, and receives its result). Interactive terminal
behavior (the launcher, prompts, signal forwarding through a terminal) and an
OAuth login against the system keychain were also checked manually on macOS.
Through agx, one Codex 0.162.1 app server with a fake model endpoint served two
threads in different scopes, each listing only its own MCP server and tools,
receiving only its instructions and skill, and reporting the other directory's
skill disabled in `skills/list`; resuming and forking a thread by ID alone kept
its directory's resources. Claude Code 2.1.300 driven by the Agent SDK that T3
Code ships, with its own system prompt append, received agx instructions and
the SDK's append once each, also on resume.
These depend on installed harnesses, terminals, or keychains, so they are not
part of the automated suite.

## Limits

Real OS keychains on Linux and Windows and real authorization servers have not
been exercised. Windows code uses protected ACLs and Job Objects, assigning stdio
children to their jobs before resuming them; the end-to-end test runs a stdio
MCP server through that path in CI. Native Windows harness executables must be
`.exe` files; PATH lookup skips `.cmd`/`.bat` wrappers and reports an actionable
error when nothing else matches. Windows symlinks need Developer Mode, so shims
there are hard links, and tests that need symlinks skip without them. Git
checkouts without symlink support store links as files, so their package hashes
can differ from other platforms.

Endpoints currently use the stateful MCP initialize handshake. The stdio bridge
accepts protocol versions through 2025-11-25; the newer stateless discovery
protocol is not implemented. HTTP forwarding preserves upstream responses and
SSE streams without rewriting tool catalogs or retrying calls.

OAuth currently supports public clients, with optional dynamic registration.
Confidential-client methods are not implemented. OAuth secrets apply to HTTP MCP
servers' `auth`; injected credentials can use keychain, environment, or command
secrets. Logout applies to keychain and OAuth secrets; other kinds reject it. A
daemon restart invalidates active grants.

The daemon reloads configuration on demand (resolution, launches, status) and
skips the load while every file, glob, lockfile, and package cache entry it read
keeps its modification time and size; it publishes only valid configurations.
An invalid reload blocks new preparations while existing launches retain their
snapshots. Ended launches and package operations are pruned after an hour; idle
MCP sessions after 30 minutes. Package caches and generated artifacts are
retained; automatic garbage collection, upstream pooling, registries, a UI, and
automatic native binary updates are deferred.

Managed instructions use Codex's `developer_instructions` override (a thread's
`developerInstructions` under `app-server`), Claude's
`--append-system-prompt-file` (merged into an Agent SDK `initialize` request),
and Pi's `--append-system-prompt`. The Codex
override replaces that native configuration field for the launch and is passed
on the command line, so very large instructions can exceed Windows' command-line
limit; native project instruction file discovery continues. Existing unmanaged
global skills are outside exclusions.
