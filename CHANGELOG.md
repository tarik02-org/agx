# Changelog

All notable changes are listed here. Versions follow [Semantic Versioning](https://semver.org).

## Unreleased

agx has not been released yet. What it does today:

- Directory-scoped configuration: YAML fragments with imports, scope rules that add and
  remove skills, instructions, MCP servers, and bundles per directory or harness, and
  `agx explain` with provenance.
- Skills and instructions from directories, Git, or inline text; packages from local paths
  and Git with lockfiles, pinned sources, and content-addressed installation.
- Launches of Codex, Claude Code, and Pi with their skills, instructions, MCP servers, and
  harness settings; harness definitions based on a built-in, shims and links that put them
  on `PATH`, and a launcher and `agx sessions` that search and resume sessions.
- A per-user daemon that owns MCP servers over stdio and HTTP, shares local server
  processes per directory, repository, or globally, and exposes them to each launch through
  expiring grants. Launches survive daemon restarts, upgrades, and crashes.
- On-demand MCP servers reached with `agx mcp call`, `agx mcp tools`, and generated
  TypeScript clients (`agx mcp types`).
- `codex app-server` hosts get resources per thread.
- Injected values (`!env`, `!file`, `!secret`) that never reach snapshots, output, or logs;
  secrets from the environment, commands, files, sops, an encrypted store keyed by the OS
  keychain, or OAuth with `agx secret login` and automatic refresh.
- Editing commands (`agx mcp|skill|instructions|harness add|list|remove`, `agx use`,
  `agx unuse`) that keep comments and layout, and a Connect API to plan and apply the same
  edits.
- `agx doctor`, `--json` on every command, shell completions, `agx man`, JSON Schemas for
  configuration and package manifests, a Home Manager module, and a nightly prerelease per
  `master` build.

### Changed

- `imports` is a list of strings: a path that must exist, or a glob that may match nothing.
- Removed `rev`; a full commit hash in `ref` pins a Git source.
- Removed the `lockfile` key; unpinned packages lock in `agx.lock.json` beside their fragment.
- Removed the MCP `transport` key; `url` means HTTP and `command` means stdio.
- Removed `daemon.shutdown_grace_seconds` and `daemon.override`.
- Harness `args` groups are `{values, modes}` only; skill `files` take `{content}` or `{path}` only.
- Removed `agx mcp login` (use `agx secret login`) and `agx resolve` (use `agx explain`).
- `agx secret set` prompts without echo on a terminal and reads piped stdin otherwise.
- A daemon of another agx build is always replaced, keeping its launches.
- Wrappers attach their launches, `codex app-server` threads included, to a daemon that does not
  hold them; the daemon keeps only its port and revoked launches in `daemon-state.json`.
- `agx daemon restart --json` reports `kept_launches` instead of `restored`.
- Passthrough harness commands replace the agx process on Unix.
- Codex plugin checks ask `codex features list` instead of reading Codex configuration.
