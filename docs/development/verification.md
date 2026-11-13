# Verification and current support

Behavior is verified by the Go test suite, which CI runs on every push and pull
request on Linux, macOS, and Windows (`.github/workflows/ci.yml`):

```sh
go test ./...                          # unit tests; -race on Linux in CI
go test -tags e2e ./internal/e2e/...   # end to end, against built client/daemon pairs
```

Tests run in temporary directories with `HOME`, `USERPROFILE`, the `AGX_*`
paths, the XDG directories, and the harness stores (`CODEX_HOME`,
`CLAUDE_CONFIG_DIR`, `PI_CODING_AGENT_DIR`) pointed inside them. They need no
network and never touch the user's keychain: OAuth and keychain tests use the
keyring library's in-memory provider. CI also lints for Linux and Windows,
checks that `buf generate` and `go generate` leave no diff, and builds the Nix
package and Home Manager integration on Linux and macOS. The end-to-end suite
checks sibling daemon lookup, installed shims (including Windows copies), missing
helper errors, daemon replacement and launch reattachment. A dependency test
keeps SQL storage and daemon implementation out of the client binary.

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
ACP agents from the ACP registry ran through `acp` definitions in a bubblewrap
sandbox (ACP version 1). From a scoped directory, claude-agent-acp 0.106.0 and
codex-acp 2.6.0 (HTTP MCP servers and a client system prompt) called a managed
MCP tool and followed the directory's instructions, which reached them only
through the system prompt; from an unscoped directory they did not follow them.
fx 0.0.16 (`fx acp`) refuses that system prompt, since it implements an earlier
draft; agx sent the session again without it, and fx called the tool and
followed the instructions from the first prompt.
Every agent in the ACP registry (46, on 2026-11-13) then ran through agx in the
same sandbox, once from a scoped directory, where agx added MCP servers and
instructions, and once from an unscoped one: each initialized and answered
`session/new` the same way in both (a session, or the same sign-in or setup
error), except fast-agent and minion-code, whose packages fail to start. Of the
agents that run a prompt without signing in, opencode and opencode-v2 called
the managed tool and followed the scoped instructions.
Pointed at an OpenAI-compatible endpoint, goose 1.59.0 and Qwen Code 0.27.3
called the tool and followed the scoped instructions. Mistral Vibe 3.1.1, which
takes no HTTP MCP servers, called it through `agx mcp bridge` and followed
them; deepagents 0.1.7 followed them but ignores every MCP server a session
names.
opencode 2.2.6 (`opencode acp`, HTTP MCP servers, no system prompt) called the
tool, and pi-acp 0.0.34 (neither) followed the instructions, both from the
first prompt. kilo 7.8.27 accepted sessions with agx resources; Gemini CLI
0.68.0 and Qwen Code 0.27.3 initialized but were not signed in.
These depend on installed harnesses, terminals, or keychains, so they are not
part of the automated suite.

## Limits

Real OS keychains on Linux and Windows and real authorization servers have not
been exercised. Windows code uses protected ACLs and Job Objects, assigning stdio
children to their jobs before resuming them; the end-to-end test runs a stdio
MCP server through that path in CI. Native Windows harness executables must be
`.exe` files; PATH lookup skips `.cmd`/`.bat` wrappers and reports an actionable
error when nothing else matches. Windows symlinks need Developer Mode, so shims
there are copies, and tests that need symlinks skip without them. Git
checkouts without symlink support store links as files, so their package hashes
can differ from other platforms.

Endpoints currently use the stateful MCP initialize handshake. The stdio bridge
accepts protocol versions through 2025-11-25; the newer stateless discovery
protocol is not implemented. HTTP forwarding preserves upstream responses and
SSE streams without rewriting tool catalogs or retrying calls.

OAuth currently supports public clients, with optional dynamic registration.
Confidential-client methods are not implemented. OAuth secrets apply to HTTP MCP
servers' `auth`; injected credentials can use keychain, environment, or command
secrets. Logout applies to keychain and OAuth secrets; other kinds reject it.
Launches survive a daemon restart without force, but their MCP sessions do not:
clients must start a new session on 404, as the MCP specification requires.
Claude Code recovers for tool calls in recent releases; Codex bundling rmcp
older than 1.2 does not re-initialize and needs a new thread or `thread/resume`;
Pi was not checked.

ACP version 2 follows the draft schema `2.0.0-alpha.15` and is tested only with
scripted messages, since no agent speaking it was available. Instructions reach
an ACP agent without client system prompt support only through a new session's
first prompt, so a session loaded or resumed from outside agx goes without them.

The daemon reloads configuration on demand (resolution, launches, status) and
skips the load while every file, glob, lockfile, and package cache entry it read
keeps its modification time and size; it publishes only valid configurations.
An invalid reload blocks new preparations while existing launches retain their
snapshots. Ended launches and package operations are pruned after an hour; idle
MCP sessions after 30 minutes. Package caches are retained, and generated
artifacts until no launch used them for 7 days; package garbage collection,
registries, a UI, and automatic native binary updates are deferred.

Managed instructions use Codex's `developer_instructions` override (a thread's
`developerInstructions` under `app-server`), Claude's
`--append-system-prompt-file` (merged into an Agent SDK `initialize` request),
and Pi's `--append-system-prompt`. The Codex
override replaces that native configuration field for the launch and is passed
on the command line, so very large instructions can exceed Windows' command-line
limit; native project instruction file discovery continues. Existing unmanaged
global skills are outside exclusions.
