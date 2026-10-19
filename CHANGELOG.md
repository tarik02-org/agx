# Changelog

All notable changes are listed here. Versions follow [Semantic Versioning](https://semver.org).

## Unreleased

### Added

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

### Changed

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

### Fixed

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
