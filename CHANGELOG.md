# Changelog

All notable changes are listed here. Versions follow [Semantic Versioning](https://semver.org).

## Unreleased

### Added

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

- Codex launches install a missing skill plugin themselves instead of failing
  with "run agx install".

### Fixed

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
