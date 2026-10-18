# Changelog

All notable changes are listed here. Versions follow [Semantic Versioning](https://semver.org).

## Unreleased

First release: agx 0.1.0.

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
  the OS keychain, or OAuth, including on-demand OAuth after a `401` and
  `agx secret login`.
- Configuration editing commands (`agx mcp|skill|instructions|harness
  add|list|remove`, `agx use`, `agx unuse`) that keep comments and layout and
  validate before writing.
- Shims that put harness definitions on PATH, and a launcher and `agx sessions`
  that search and resume Codex, Claude Code, and Pi sessions.
- JSON Schemas for configuration fragments and package manifests.
- Test suite with end-to-end tests, CI on Linux, macOS, and Windows, release
  archives for six platforms, and a rolling nightly prerelease.
