# Changelog

All notable changes are listed here. Versions follow [Semantic Versioning](https://semver.org).

## Unreleased

agx has not been released yet. What it does today:

- Directory-scoped configuration: a root `agx.yaml` listing fragments and holding daemon settings, YAML
  fragments with the definitions, scope rules that add and
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
  secrets from commands, sops, an encrypted store keyed by the OS keychain, or OAuth with `agx secret login` and automatic refresh.
- Editing commands (`agx mcp|skill|instructions|harness add|list|remove`, `agx use`,
  `agx unuse`) that keep comments and layout, and a Connect API to plan and apply the same
  edits.
- `agx doctor`, `--json` on every command, shell completions, `agx man`, JSON Schemas for
  the root file, fragments, and package manifests, a Home Manager module, and a nightly
  prerelease per `master` build.

### Changed

- The root `agx.yaml` holds only `fragments` and `daemon`; `imports` is gone. `fragments` is a
  list of strings, a path that must exist or a glob that may match nothing, and fragments hold
  the definitions and cannot list others.
- Editing commands write new definitions and rules to `--file`, the only writable fragment, or
  one picked on a terminal, creating `agx.d/main.yaml` when there is none; `PlanChangesRequest.file`
  replaces `DefineChange.file`.
- Removed `rev`; a full commit hash in `ref` pins a Git source.
- Removed the `lockfile` key; unpinned packages lock in `agx.lock.json` beside their fragment.
- Removed the MCP `transport` key; `url` means HTTP and `command` means stdio.
- Removed `daemon.shutdown_grace_seconds` and `daemon.override`.
- Harness `args` groups are `{values, modes}` only; skill `files` take `{content}` or `{path}` only.
- Removed `agx mcp login` (use `agx secret login`) and `agx resolve` (use `agx explain`).
- `agx secret set` prompts without echo on a terminal and reads piped stdin otherwise.
- Only a newer agx replaces a running daemon, keeping its launches; an agx uses a daemon of
  another version that speaks its protocol, and fails without replacing one it cannot use.
- Wrappers attach their launches, `codex app-server` threads included, to a daemon that does not
  hold them; the daemon keeps only its port and revoked launches in `daemon-state.json`.
- `agx daemon restart --json` reports `kept_launches` instead of `restored`.
- `--json` is a global flag; daemon-backed commands print the API's messages, and list and
  explain commands the loaded definitions, instead of separate JSON shapes.
- Passthrough harness commands replace the agx process on Unix.
- Codex plugin checks ask `codex features list` instead of reading Codex configuration.
- Fragments and package manifests are checked against the published JSON Schema when they load.
- Daemon errors about configuration carry the file and line in `ErrorDetail.source`.
- Git packages fetch only the requested commit instead of cloning the repository.
- MCP `auth` takes `!env` and `!file`, and `age_key` a literal key; removed the `environment`,
  `file`, and `daemon_environment` secret kinds.
- agx owns its shim directory: removed `agx shims --dir`, and `install` removes other files there.
  Windows shims are copies that run the agx recorded beside them.
- New Claude launches pass `--session-id`, so `agx sessions` resumes exactly the session a
  launch started with its definition.
- Only the runtime and state directories are private; caches get default permissions, and agx
  no longer changes the permissions of the configuration directory or writes lockfiles private.
- Launches share one cached copy of their skills and instructions per distinct content, instead
  of one per directory and configuration; Codex launches copy no skills. The daemon removes
  copies no launch used for 7 days, and what interrupted writes and installations left in the
  cache.
- Launch records are an append-only `launch-records.jsonl` that a listing folds and prunes, so a
  launch appends two lines instead of rewriting every record; an existing `launch-records.json`
  is folded in and removed. Session listings open only session files that changed.
