# Changelog

All notable changes are listed here. Versions follow [Semantic Versioning](https://semver.org).

## Unreleased

agx has not been released yet. What the first release contains:

- Directory-scoped configuration: scope rules that add and remove skills, instructions, MCP
  servers, and bundles per directory or harness, and `agx explain` with provenance.
- Two places to define things. Local configuration, saved by commands in the daemon's SQLite
  state (`agx mcp|skill|instructions|harness add|list|remove`, `agx use`, `agx unuse`,
  `agx config export`), and declarative YAML overlays: a root `agx.yaml` listing fragments and
  holding daemon settings. An overlay's definition replaces a local one of the same name.
- Skills and instructions from directories, Git, or inline text; packages from local paths
  and Git with lockfiles, pinned sources, and content-addressed installation.
- Launches of Codex, Claude Code, and Pi with their skills, instructions, MCP servers, and
  harness settings; harness definitions based on a built-in, shims and links that put them
  on `PATH`, and a launcher and `agx sessions` that search and resume sessions.
- OpenCode 2 through a native plugin that gives its server directory-scoped resources and
  lists its sessions in the launcher.
- Two binaries: `agx`, the client, and `agxd`, a per-user daemon that owns the database, the
  secrets, and MCP servers over stdio and HTTP. It shares local server processes per
  directory, repository, or globally, and exposes them to each launch through expiring
  grants. Launches survive daemon restarts, upgrades, and crashes. A service manager can run
  the daemon instead (`daemon.managed`).
- On-demand MCP servers reached with `agx mcp call`, `agx mcp tools`, and generated
  TypeScript clients (`agx mcp types`).
- `codex app-server` hosts get resources per thread.
- Any agent that speaks the Agent Client Protocol (versions 1 and 2), through harness
  definitions on the `acp` base: each session gets its directory's MCP servers and
  instructions in the forms the agent advertises; agents without HTTP MCP servers reach
  them over stdio through `agx mcp bridge`.
- Injected values (`!env`, `!file`, `!secret`) that never reach snapshots, output, or logs;
  secrets from commands, sops, an encrypted store keyed by the OS keychain, or OAuth with
  `agx secret login` and automatic refresh.
- A Connect API for everything the commands do, with Go and TypeScript clients.
- `agx doctor`, `--json` on every command, shell completions, `agx man`, JSON Schemas for
  the root file, fragments, and package manifests, a Home Manager module, and a nightly
  prerelease per `master` build.
