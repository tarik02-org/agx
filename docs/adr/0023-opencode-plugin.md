# Support OpenCode through one plugin that asks the daemon per location

Status: deferred. Not implemented.

OpenCode 2 runs one server for many directories: the shared background service
that every `opencode` attaches to, a private `--standalone` server, or
`opencode serve` started by a host such as T3 Code. A per-process launch, as agx
gives Codex, Claude Code, and Pi, cannot scope resources there: the process
serves every directory, and most of them are not started by agx at all.

OpenCode loads server plugins once per **location** (a directory, plus an
optional workspace), and plugins can edit that location's MCP servers and skills
and the system prompt of its model requests. agx therefore supports OpenCode
with one plugin, installed into OpenCode's configuration, that asks the daemon
what its location gets and registers it.

## Decision

`agx install` writes the agx plugin into OpenCode's global configuration. Each
plugin instance prepares a **location launch** for `ctx.location.directory`,
receives the resolved snapshot's MCP endpoints with their grant, skills, and
instructions, and registers them:

| Resource | OpenCode mechanism |
| --- | --- |
| MCP servers | `ctx.mcp.transform`: `editor.set("agx-<id>", {type: "remote", url, headers: {Authorization: "Bearer <grant>"}, oauth: false})` |
| Skills | `ctx.skill.transform`: `editor.add({id, name, description, path, content})`, `path` being the `SKILL.md` in agx's compiled artifacts |
| Instructions | `ctx.session.hook("context", …)`: `event.system.push({type: "text", text})` before every agent-loop request |

The same plugin serves plain `opencode`, the shared service, `agx opencode`,
and any `opencode serve`, with no wrapper in between.

- MCP servers are named `agx-<id>` as in other harnesses. `oauth: false` is
  required: remote servers default to OAuth discovery, and the daemon handles
  upstream credentials itself. `codemode` stays unset, so OpenCode's default
  (tools reached through its Code Mode `execute` tool) applies; an MCP server
  definition may later carry an OpenCode override. On-demand servers
  ([ADR 0022](0022-on-demand-mcp-calls.md)) are left out, as elsewhere.
- Skills keep their names, as in Pi. OpenCode derives the skill's base
  directory from `path`, so supporting files resolve inside agx's artifact
  directory. `content` is the body without frontmatter. A skill ID OpenCode
  already has (from `.opencode/skills`, `~/.agents/skills`, and so on) is
  skipped with a warning rather than replaced.
- Instructions are appended after OpenCode's own system prompt and the
  project's `AGENTS.md`. They are added per request and never stored in the
  session, so they follow the session's current directory, also after
  `session.move`. Title and compaction requests do not get them.

## Lifecycle

A location loads when OpenCode first needs its services (the first prompt in
it, not session creation or listing) and loads the plugin once for all sessions
there. A subdirectory is its own location.

1. `setup` registers the three transforms over empty state, then prepares the
   location launch inline, so a running daemon's resources are in place before
   the first prompt, and calls `ctx.mcp.reload()` and `ctx.skill.reload()`.
2. A background loop renews the lease (default every 20 seconds of a 60-second
   lease, as for other launches). When the daemon is unreachable, setup still
   succeeds with no resources and the loop keeps retrying; when it answers, the
   plugin fills the state and reloads.
3. A renewal that finds the launch unknown (a daemon replaced) or its snapshot outdated (configuration or packages changed)
   prepares a new location launch and reloads. Unlike other harnesses, OpenCode
   picks up new MCP endpoints, skills, and instructions without a restart.
4. The cleanup function finishes the launch. OpenCode runs it when the plugin
   reloads (any change to the configuration or to the plugin file reloads every
   loaded location), when a location is evicted after 60 minutes without
   session activity, and on a graceful server stop. A killed server leaves the
   lease to expire.

The plugin must never throw: OpenCode retries a failing `setup` once and then
deactivates the plugin for that location until the next reload, and a transform
that fails schema validation disables the whole plugin. It validates the
daemon's answer before touching an editor.

## Daemon

`LaunchService` gains `PrepareLocationLaunch(directory, harness, plugin_version,
environment_inputs)`, and `RenewLaunch`/`FinishLaunch` accept the location
launch's lifecycle token. A location launch is a launch without a wrapper: mode
`plugin`, harness `opencode` (or the definition named by `AGX_HARNESS`, below),
its own grant and lifecycle token, shown by `agx launch list`. Unlike other
launches ([ADR 0016](0016-launches-survive-daemon-restarts.md)), it is not
attached to a restarted daemon: the plugin prepares again and reloads.

Environment inputs (`!env` values) come from the OpenCode server's own
environment: the daemon answers a first request with the names it needs, as it
does for the wrapper, and the plugin sends only those. Under `agx opencode`
that is the launching shell's environment; under the shared service, whatever
environment the service started with.

### Authorization

The plugin runs inside the harness, so it must not hold the management token
from `daemon.json`. `agx install` writes a **plugin token** file in the state
directory (mode 0600, a private ACL on Windows) that authorizes only
`PrepareLocationLaunch`, `RenewLaunch`, and `FinishLaunch` for harnesses based
on `opencode`. The plugin reads the daemon's URL from `daemon.json` and the
token from that file on every (re)connect, so a daemon restart or token
rotation needs no reinstall. Calls are Connect JSON over HTTP to `127.0.0.1`,
which the plugin makes with `fetch`.

This token lets any process of the user obtain grants for any directory, which
the management token already allows; the gain is that a harness never holds
the authority to change configuration, secrets, or other launches.

When discovery is missing, the plugin starts the daemon once with the
absolute agx path recorded at installation (`agx daemon start`, detached), then
retries as above.

## Installation

`agx install` and `agx opencode` (when the file is missing or outdated) write
the plugin as `agx.js` in the `plugins` directory of OpenCode's global
configuration (`$XDG_CONFIG_HOME/opencode/plugins`, or `OPENCODE_CONFIG_DIR`).
OpenCode loads files there automatically, so agx edits no OpenCode JSON. The
generated file embeds the agx executable path, agx's state directory, and its
own version; agx recognizes it by a header and content hash, like
[shims](0012-harness-adapters-and-shims.md#shims), and replaces or removes only
its own file.

Like Codex's `CODEX_HOME`, a definition whose `environment` sets
`XDG_CONFIG_HOME` or `OPENCODE_CONFIG_DIR` gets its own installation there.
`opencode plugin add` is not used: it accepts only npm or Git packages.

The plugin is a plain ES module with a `{id, setup}` default export and no
imports besides `node:` built-ins, embedded in the agx binary. It needs no
bundling or `node_modules`; OpenCode also provides `@opencode/plugin` and
`effect` to local plugins, but the Promise API is enough. A plugin whose
version the daemon does not accept registers nothing and asks for
`agx install`, which `agx doctor` also reports.

## `agx opencode`

Plain `opencode` already gets everything. `agx opencode` adds:

- The daemon started and the plugin installed before OpenCode runs.
- Harness definitions: `agx opencode-work` runs OpenCode with the definition's
  arguments and environment plus `AGX_HARNESS=opencode-work`, and adds
  `--standalone` unless `--server` or `--standalone` is given, since only a
  private server inherits that environment. Its plugin prepares launches for
  that definition, so its `add`/`remove` apply. Plain `agx opencode` adds
  nothing and attaches to the shared service.
- Modes: `interactive` (no subcommand), `print` (`run`), `serve`, `acp`, and
  `passthrough` for management commands (`auth`, `mcp`, `plugin`, `service`,
  `models`, `debug`, help, version).
- Sessions in the launcher, below.

## Sessions

`opencode session list --format json` prints
`[{id, title, updated, created, projectId, directory}]`, but only for the
current directory's project, through a server (the shared service or a private
one), and has no option for all projects. The HTTP API's `GET /api/session`
without `project` lists every project, but also needs a server.

The adapter reads OpenCode's SQLite database instead, read-only:
`$XDG_DATA_HOME/opencode/opencode.db` (or `OPENCODE_DB`), table `session_v2`
(`id`, `directory`, `title`, `parent_id`, `time_created`, `time_updated`,
`time_archived`), skipping child and archived sessions; the first user message
comes from `session_message`. Resuming runs `opencode --session <id>`, which
continues in the session's stored directory regardless of the current one.

## Limits

- **Scoping is per location, not per session.** The context event carries
  `sessionID`, `agent`, `model`, `system`, `messages`, `options`, and `tools`,
  and MCP `tools/call` requests carry `_meta["ai.opencode/sessionID"]`, but MCP
  connections, skills, and the grant belong to the location instance that all
  sessions in a directory share. Per-session differences are possible only for
  instructions and for hiding tools in the context hook. Since agx resolves per
  directory, this matches its model.
- **Definitions in the shared service.** The plugin cannot tell which agx
  definition, if any, opened a session on the shared service; there it always
  applies the `opencode` harness. Definitions take effect only on servers they
  start (`agx opencode-work`, which forces `--standalone`).
- **First prompt without MCP tools.** OpenCode 2.0.29 leaves MCP tools out of
  the first request after a location boots, for configured servers and plugin
  servers alike; the next request has them. This is upstream behavior, not the
  plugin's.
- Locations are keyed by the directory string a client sends: `/tmp/x` and
  `/private/tmp/x` are two instances. agx canonicalizes before resolving, so
  both get the same resources, with two launches.
- Instructions do not reach compaction or title requests; the `compaction` hook
  could add them later.

## Spike results

Run against OpenCode 2.0.29 in a sandbox (separate `HOME`, `XDG_*`, `TMPDIR`),
with a private `opencode serve` and `--standalone` runs, a fake
OpenAI-compatible provider recording every request, a stub MCP server
requiring `Authorization: Bearer token-<name>`, and a stub daemon.

| Spike | Result | Evidence |
| --- | --- | --- |
| Plugin loading | works | A plain `{id, setup}` module with no imports loads from a configured directory (`{"package": "/abs/dir", "options": {…}}`), from `<project>/.opencode/plugins/`, and from the global `plugins` directory. An Effect plugin importing `@opencode/plugin/effect` and `effect` loads from `.opencode/plugins/` with no `node_modules` anywhere. `ctx.location` = `{directory, project: {id, directory, canonical}}`; `ctx.options` arrive. |
| Per-directory instances | works | One `serve`, sessions created with `location.directory` dirA and dirB: plugin loaded once per directory (same PID) on each first prompt, with that directory; a subdirectory got its own instance. A config edit unloaded and reloaded every location; SIGTERM ran every cleanup. |
| MCP registration | works | The stub logged `initialize`, `tools/list`, and `tools/call` with `Bearer token-alpha` from dirA's instance and `Bearer token-beta` from dirB's, never crossed. With `codemode: false` the request listed `agx-alpha_whoami_alpha`; the fake model called it and the result (`I am alpha`) came back. With Code Mode on, it appeared in the Code Mode catalog. |
| Skills | works | The skill appeared in `<available_skills>` of the system prompt and in `GET /api/skill`; the `skill` tool returned its content with `Base directory for this skill: <dir of path>`. A skill without `description` failed schema validation and disabled the plugin. |
| Instructions | works | The pushed text was in the system messages of dirA's requests and dirB's text in dirB's; after moving a session to dirB, its next request carried dirB's text only. Context event keys: `sessionID, model, system, messages, options, agent, tools`. |
| Daemon mechanics | works | The plugin read a discovery file and called the stub over `fetch` to `127.0.0.1` with a bearer token; renewals ran from a timer. With the stub down, setup succeeded empty, and a later connect plus `reload()` applied MCP, skill, and instructions to the next request. A throwing `setup` was retried once, then deactivated. Environment variables of `agx`'s shell reached the plugin under `--standalone`. |
| Sessions | partially | `session list --format json` shape as above, project-scoped, needs a server. `GET /api/session` and `session_v2` hold all projects. `run --session <id>` from another directory continued in the session's directory. |
| Installation | works | `opencode plugin add` takes only npm/Git packages; local plugins load from config paths and `plugins/` directories, and OpenCode watches both and the plugin file itself, reloading on change. |

Two of seven `opencode run --standalone` invocations hung after the private
server started and before any location booted, so before the plugin loaded;
worth watching, but not caused by it.

## Effort

About a week:

1. Plugin (about 250 lines) and its install, update, and removal: 1.5 days.
2. Location launches, the plugin token, and renewal with snapshot staleness in
   the daemon: 2 days.
3. The `opencode` adapter (modes, definitions, `--standalone` rule, session
   reader, version check in `agx doctor`): 1.5 days.
4. End-to-end tests against a fake provider and docs: 1 day.

## Open questions

- Reading `opencode.db` needs a SQLite driver in agx, a new dependency
  (`modernc.org/sqlite` or a WASM build). The alternative is the HTTP API of
  the shared service, which requires starting it.
- Whether `agx opencode-work` should force `--standalone`, or rather pass the
  definition through session metadata (`session.create` accepts `metadata`),
  which the TUI does not let agx set today.
- Whether a live reload that only changes an MCP server's headers (a new grant)
  reconnects it; the spike verified empty-to-populated reloads and full plugin
  reloads.
- Whether subagent sessions reach the `context` hook with their own session ID,
  and whether instructions should apply to them.
- Starting the daemon from inside the shared service, whose environment may
  lack the user's shell setup.

Rejected:

- `OPENCODE_CONFIG_CONTENT` with `--standalone` per launch: configuration is
  process-wide, so one `opencode serve` cannot give directories different
  resources, and plain `opencode`, the shared service, and hosts' servers would
  get nothing.
- An HTTP reverse proxy in front of `opencode serve`: no request carries MCP
  servers, skills, or system prompt to rewrite; it would track a large,
  changing API; and the TUI talks to the shared service directly.
- ACP through `opencode acp` and the [ACP proxy](0017-acp-proxy.md): it covers
  only ACP clients, cannot deliver skills, and leaves OpenCode's own TUI and
  service out. It remains available for ACP clients.
- Writing `.opencode/` files into projects: agx does not modify repository
  files as a side effect of launching.
