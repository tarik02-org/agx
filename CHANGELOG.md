# Changelog

All notable changes are listed here. Versions follow [Semantic Versioning](https://semver.org).

## Unreleased

### Added

- `agx secret rotate <secret-id>` and `SecretService.RotateSecret` renew an OAuth secret's
  access token with its refresh token now, whether or not it expired, and report when the new
  one expires. A refused refresh token fails with `needs_login`, an authorization server that
  cannot be reached with `network` (the new `REASON_NETWORK`).
- Definitions carry an icon and color: a skill's from the `interface.icon_small` and
  `interface.brand_color` its `agents/openai.yaml` declares, and an MCP server's from the
  icons it declares when it starts (`McpTools.icon`), which the daemon keeps in
  `mcp-icons.json` in its cache for while the server is not running.
- `PackageService.PreviewSource` fetches a Git source (`owner/repo[@ref][/path]` or a URL)
  into the cache and lists its skills with their SKILL.md and files, without configuring
  anything; `ReadPreviewFile` reads one of their files for 30 minutes.
- `PackageService.ListSourceRefs` lists a Git source's branches and tags, with their commits,
  without fetching it; `InspectSource` checks one out at its pins for the commit, the content
  hash a pinned package records, and its directories.
- `ConfigurationService.PlanChanges` and `ApplyChanges`: the edits the CLI makes (define,
  undefine, enable, disable, a local skill's or instructions' file, and a Git package) as a
  plan with a diff and SHA-256 per file, and an apply that writes all of it or nothing, and
  fails when a file changed since the plan.
- `ConfigurationService.GetConfiguration` (harnesses, scope directories, and files with
  whether each is writable), `CompleteDirectory`, `ListDefinitions` (every definition with the
  YAML that defines it), `GetSkill`, and `ReadFile` (configuration fragments and files of
  configured skills and instructions only), and harness settings in `ResolveResponse`.
- `agx mcp stop <pid>` and `RuntimeService.StopMcpProcess` stop a local MCP server process;
  its sessions reconnect and the next one starts it again.
- Harness definitions take `settings:` in the harness's own configuration schema. Codex gets
  one `-c` override per setting, encoded as TOML; Claude Code gets a per-launch file passed
  with `--settings`; Pi definitions reject them. Settings merge along the base chain with
  `null` removing inherited keys, allow `!file`, and reject `!secret` and `!env`.
  `agx explain --harness` and `agx harness list --json` show them, and `agx doctor` runs
  Codex with each definition's settings to report what it rejects.

- A Home Manager module, `homeManagerModules.default` (`programs.agx`): installs agx,
  writes `settings` to the drop-in `agx.d/10-home-manager.yaml` with `!secret`, `!env`,
  and `!file` values from `config.lib.agx`, validates it at build time, runs
  `agx shims install` at activation with the shims directory on `PATH`, and links
  built-in harnesses to agx with their native executable pinned.
- `agx mcp tools [<server>] [--schema] [--all]` lists the MCP servers you can call, with
  their state, load mode, description, and tools, or one server's tools as compact
  TypeScript-like signatures. `agx mcp call <server>.<tool> [key=value ...]` calls a tool
  through the daemon: values are converted by the tool's schema, `key=@file` reads a file,
  `--args` takes a JSON object, binary content is written to files (`--save-dir`), and
  `--json` prints `{mcp_id, tool, is_error, content, structured_content}`. Inside a launch
  they reach that launch's servers with its grant; elsewhere the current directory's, or
  every server with `--all`. Client sessions are pooled in the daemon.
- `agx mcp types [<server> ...] [--out <dir>]` generates TypeScript declarations and a
  dependency-free ES module client, so node, bun, or deno scripts can chain tool calls.
- MCP servers take `description` and `load: native | on_demand`. On-demand servers stay out
  of every harness's tool list and are reached with `agx mcp call`; `mcp list`, `resolve`,
  `explain`, and `mcp tools` show the load mode, and `agx mcp add` takes `--description`
  and `--load`.
- Local MCP servers take `share: none | directory | repo | global`: harness sessions
  and `agx mcp call` sessions join one process per launch directory, per repository root,
  or for every launch, instead of starting a process each. The daemon multiplexes the
  sessions onto the server, answers its `roots/list` with the shared root, and refuses
  sampling and elicitation; the process stops five minutes after its last session, when a
  secret it uses changes, or when it crashes; `share: {type, idle}` sets how long it
  stays idle. `agx mcp add` takes `--share`; `mcp list`, `resolve`, and `explain` show it.
- `agx mcp ps [--cwd <dir>] [--under <dir>]` and `RuntimeService.ListMcpProcesses` list the running local
  MCP server processes, shared or not, with their share mode, root, sessions, launches,
  and when an idle shared one stops.
- Harnesses get `AGX_LAUNCH_ID` next to `AGX_MCP_GRANT`; Codex app-server threads get
  their own in their shell environment.
- `RuntimeService.ListTools` and `CallTool`, and the JSON error code `mcp_failed`
  (`REASON_MCP_FAILED`) for failures inside an MCP server.
- `agx daemon restart [--force] [--timeout <duration>] [--json]` stops the daemon,
  waits for it to exit, starts one of this build with the same root, and reports
  how many launches it restored (`{stopped, started, version, restored}`).
- `--json` on every command except harness entry points: edits, `config validate`,
  `install`, `update`, `mcp test`, `use`, `unuse`, `remove`, `shims`, `daemon start|stop`,
  `launch revoke`, and `secret set|logout` print one versioned document; `secret login` and
  `mcp login` stream `started` and `finished` events, one per line. Errors become
  `{"version":1,"error":{…}}` documents with stable codes, usage errors included, and
  `--json` implies `--yes` and never prompts. `TestMcpServer` returns tool descriptions.
- `agx doctor [directory] [--all] [--json]` checks configuration, packages,
  harness versions, Codex plugins, the daemon, MCP servers, secrets, sops, and
  shims, with a fix for each problem.
- `sops` secrets take an optional `age_key` (`!secret`, `!env`, or `!file`) passed
  to sops as `SOPS_AGE_KEY`, so the age identity can live in the OS keychain.
- `agx man` prints a man page of every command.
- Harnesses take `hidden: true` to stay out of the launcher's new-session list, built-ins
  included; they still run by name, and definitions based on them stay listed.
- `agx doctor` passes the shims check for definitions that `PATH` already runs as agx
  through links outside the shim directory, such as ones a package manager installs.
- `kind: file` secrets read a file at each use, without trailing newlines, such as one
  sops-nix or agenix decrypts under `/run`; unlike `!file` they work as an MCP server's
  `auth` and show in `agx secret status`.
- A link to agx named after a built-in harness, such as `codex`, runs it like `agx codex`,
  as shims do for definitions; the native executable is the pinned `executable` or the
  next one on `PATH` that is not agx.

### Changed

- Keychain secrets and OAuth credentials live in `secrets.json` in the state directory,
  encrypted with AES-256-GCM under one key that the OS keychain holds, instead of one keychain
  item each. Values have no size limit, so long OAuth credentials fit on Windows, the keychain
  is asked once per process, and processes sharing the store see each other's refreshed
  tokens. Secrets stored in the keychain before need `agx secret set` or `login` again. See
  ADR 0026.
- Every `master` commit that passes CI publishes its own nightly prerelease, tagged
  `nightly-<version>.<commits>-<commit>`, instead of replacing one rolling `nightly` release.
- A failed package installation the daemon runs reports git's error, with credentials in
  URLs redacted, instead of asking to retry `agx install` locally.
- A package that is not installed, or changed since it was installed, no longer fails
  launches: they go on without its skills, instructions, and MCP servers, and warn once per
  package to run `agx install`; the daemon logs the same warning. `agx config validate`,
  `resolve`, `explain`, and `doctor` warn about it too. A malformed lockfile, a reference to
  an undeclared package, and a broken cache entry are still errors.
- `agx install` says when `git` is not on `PATH`.
- The skill picker of `agx skill add` filters with `/`, toggles with `x` or Space and all with Ctrl-A; prompts are drawn with `huh`.
- `agx skill add owner/skills` names the package after the owner (`anthropics`, `mattpocock`) instead of `skills`, so such repositories no longer collide.

- `agx daemon status` succeeds without a running daemon: it prints that none runs
  (`{"running":false,…}` with `--json`) and says why none can start, such as saved
  launches of a newer agx.
- `agx daemon start` succeeds when a daemon of this version already runs
  (`"started": false` with `--json`).
- A command using another root configuration replaces a daemon that has no
  active launches, as one of another agx build is replaced, instead of refusing
  to launch; `agx daemon start` does the same.
- The daemon shuts down gracefully on `daemon stop`, `daemon restart`, replacement,
  and SIGTERM or SIGINT (Windows console close events included): it refuses new
  launches and MCP sessions with a retryable error, lets MCP requests and tool
  calls in flight finish for `daemon.shutdown_grace_seconds` (default 10;
  `--timeout` overrides it), saves its launches, and lets stdio MCP servers exit
  before killing them. A second signal stops it at once; a detached daemon
  ignores SIGHUP. Wrappers retry refused launches and find the next daemon
  within a fraction of a second.
- Running launches survive daemon restarts and upgrades. The daemon keeps active
  launches in `daemon-state.json` in the state directory (mode 0600, token
  verifiers and metadata only); the next daemon restores them on the same port,
  and wrappers reconnect to it and resend their environment inputs, so harnesses
  keep their MCP grants, `codex app-server` threads included. MCP sessions start
  again: clients get 404 for the old session ID. Plain `daemon stop` now succeeds
  with active launches and keeps them; `--force` revokes them and clears the
  file. A command finding a busy daemon of another build replaces it instead of
  failing. `daemon stop --json` adds `kept_launches`, and `doctor` checks the
  file's permissions.
- Codex launches install a missing skill plugin themselves instead of failing
  with "run agx install".
- `agx skill remove <package-id>` also removes the rule `add` entries that enable
  the package's skills, as `skill add` writes them, so it no longer needs
  `--force` right after `skill add`. It lists those skills under `disabled` with
  `--json`. Other references, such as bundles, still need `--force`.
- Help is laid out and styled with fang in the terminal's ANSI colors. It follows
  `--color` and `NO_COLOR` like other output and is plain text when piped.

### Added

- The Nix package installs bash, fish, and zsh completions.

### Fixed

- An OAuth refresh finishes and is stored even when the request that needed it is cancelled.
  Authorization servers that rotate refresh tokens, such as T3 Code's, invalidated the old one
  as soon as they answered, so a cancelled refresh left only a dead token and asked for
  `agx secret login`. A refresh token the server refuses is retried once with the one another
  daemon stored meanwhile.
- Launches survive a daemon that dies without saving them: a wrapper whose daemon no longer
  holds its launch, or let its lease run out, re-attaches it with its ID and grant
  (`RestoreLaunch` with `launch` and `grant`), and a wrapper whose daemon was killed starts the
  next one itself, on the same port. Revoked launches stay revoked across restarts. Wrappers no
  longer give up while their harness runs; they report when MCP access pauses and comes back.
- On Windows the daemon starts outside the job object of the process that started it, so the
  exit of an editor or agent harness that ran an agx command no longer kills it; on Unix it
  starts in a session of its own.
- A hard-link or copy shim left with an older agx build by an in-place update runs the current
  agx instead, so launches through it no longer replace the daemon with the old build. The
  daemon runs from agx itself rather than from a shim.
- The daemon log records daemon starts and stops, launches it does not restore and why, launches
  that expire or are revoked, and re-attached launches; discovery carries the daemon's PID.
  Temporary files a killed process left in the state directory are removed at startup.
- An HTTP MCP server that rejects an OAuth access token with 401 before its recorded expiry,
  or one issued without `expires_in`, gets the request again with a refreshed token instead of
  asking for `agx secret login`.
- A failed OAuth refresh says why: the token endpoint's HTTP status and OAuth error code, or
  that it could not be reached, and the daemon logs it. A refresh token the authorization
  server refuses shows the secret as `needs_login` and is not sent again; an unreachable
  token endpoint fails the request without asking for a login.
- `agx secret status` marks an expired OAuth token that renews on its next use, and
  `SecretStatus.refreshable` reports it.
- Without a root configuration file, harness launches and other commands that
  start the daemon work again with an empty configuration (or only `agx.d`
  fragments); the daemon no longer receives the path as a `--config` that must
  exist.
- `agx doctor` reports a missing configuration file as such, fails on a daemon
  state file of a newer agx (and warns on one that does not decode), and spells
  every path with symbolic links resolved.
- Human error messages no longer include RPC code names, the daemon log's
  `agx:` prefix, or `agx --help` for subcommand usage errors; a launch in a
  missing directory, a Git source that is missing or private, a harness not on
  `PATH`, and an unknown configuration key (with a suggestion) are explained.
- `agx launch revoke` confirms the revocation; `--scope` must name an existing
  directory when enabling resources.
- `go install` builds report their module version instead of `dev`.

- `agx codex app-server` gives each thread the resources of its own directory.
  Hosts such as T3 Code start one app server and create every thread through
  it; agx now relays its JSON-RPC and prepares a thread launch per started,
  forked, or resumed thread, with its own MCP servers and grant, instructions,
  and skill plugins, ended when the thread closes. `launch list` shows thread
  launches under their process, and `sessions` resumes their threads. Other
  app-server transports keep one launch per process.
- Claude Code driven by the Agent SDK (`--input-format stream-json`, as T3 Code
  does) keeps agx instructions: the SDK's `initialize` request carries its own
  `appendSystemPrompt`, which replaced `--append-system-prompt-file`, so the
  wrapper now prepends the instructions to it. Such launches are `print` mode.
- With Codex plugins disabled (`features.plugins = false`), `install` and Codex
  launches say so and name the file; launches go on without skills.
- Codex skill plugins follow the `CODEX_HOME` each launch runs Codex with: the
  definition's `environment`, else the caller's (now forwarded to the daemon), instead of
  the daemon's. `install`, launch auto-installation, `doctor` (per harness
  definition), and the plugin commands agx runs all use it, and installed
  plugins are recorded per home.
- The `features.plugins = false` check follows Codex's layering: the selected
  profile, `-c features.plugins=…` and `--enable`/`--disable plugins` in the
  definition's or caller's arguments override `config.toml`. Launches whose
  arguments enable plugins install and enable their skills again. Definitions
  with their own `executable` try the installation and report plugins disabled
  only when Codex refuses it.
- Codex launches check that Codex still has their skill plugins (its
  `config.toml` entry and plugin cache) instead of trusting agx's record, so a
  plugin removed with `codex plugin remove` is installed again.
- Codex installation adds only the skills a rule selects, in the versions the
  configuration has, instead of every version ever published, and removes agx's plugins the configuration
  no longer has from that `CODEX_HOME` once superseded for 7 days.
- `agx use` writes a selection list that `unuse` emptied back in its usual place
  (`skills`, `instructions`, `mcp`, `bundles`), so use after unuse restores the
  text exactly.
- `agx doctor` reads a harness version printed to stderr, as Pi 0.73 does, and
  reports "pi 0.73.1 is older than the supported 0.99.0": a warning, or a
  failure when the configuration defines a harness based on it.
- `agx daemon stop` (with or without `--force`) exits 0 with "Daemon is not
  running." when no daemon runs. Its JSON document has one shape either way,
  `{running, stopping, forced, version}`, with `running: false`, `stopping: false`,
  `forced: false`, and `version: ""` when no daemon runs.
- `agx use` after `agx unuse` in the same rule deletes the `remove` entry
  instead of also adding the ID to `add`, so the fragment returns to its earlier text.

- Stdio MCP servers that send notifications while the harness holds no event
  stream no longer lose their session; undeliverable server requests get an
  error reply instead of waiting forever.
- `daemon stop` with active launches reports error code `conflict`, and its
  message, like the other-build hint, names `agx daemon stop --force`.
- A shim or link named after a definition no longer runs the agx CLI when the
  configuration fails to load; it reports the load error and exits 1.
- Shell completion no longer writes cobra's `Completion ended with directive`
  line to stderr, which PowerShell could show under the prompt.
- `--json` before the command (`agx --json config validate`) works as after it.
- Skills appear under their native names (`agx-managed:review` instead of
  `agx-managed:resource-…`; Codex plugins are `review-<hash>`).

### Fixed

- A harness started through a link or shim named after it no longer starts the daemon under that name, which on macOS relaunched the harness in a loop and grew `daemon.log` without bound. The daemon log is now moved aside to `daemon.log.old` past 1 MiB.

## 0.1.0 - 2026-10-18

First release.

### Added

- Directory-scoped configuration: YAML fragments with ordered, optional, and glob
  imports; scope rules that add and remove skills, instructions, MCP servers, and
  bundles per directory, everywhere, or per harness; `explain` with provenance.
- Skills from directories, Git, or generated from configuration; instructions
  from files, Git, or text with an explicit `order`; inline definitions in any
  selection list.
- Packages from local paths and Git with JSON lockfiles, content-addressed
  installation, frozen installs, explicit updates, and sources pinned by commit,
  content hash, or `/nix/store` path.
- Native integration for Codex, Claude Code, and Pi: launches classified into
  modes, harness definitions extending a built-in with an executable, argument
  groups per mode, environment, and their own selection; passthrough for help,
  version, and native management commands.
- A daemon that owns MCP upstreams over stdio and HTTP and exposes them to each
  launch through expiring grants, with leases, revocation, and a Connect
  management API.
- Injected values (`!env`, `!file`, `!secret`) resolved at launch and never
  written to snapshots, output, or logs; secrets from the environment, commands,
  the OS keychain, sops-encrypted files, or OAuth, including on-demand OAuth after
  a `401` and `agx secret login`. sops secrets let a configuration directory be a
  Git repository restored on another machine.
- Configuration editing commands (`agx mcp|skill|instructions|harness
  add|list|remove`, `agx use`, `agx unuse`) that keep comments and layout and
  validate before writing.
- Shims that put harness definitions on PATH, and a launcher and `agx sessions`
  that search and resume Codex, Claude Code, and Pi sessions.
- JSON Schemas for configuration fragments and package manifests.
- Test suite with end-to-end tests, CI on Linux, macOS, and Windows, release
  archives for six platforms, and a rolling nightly prerelease.
