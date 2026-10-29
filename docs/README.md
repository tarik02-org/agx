# agx documentation

New to agx? Read [Getting started](getting-started.md) first.

## Using agx

- [Getting started](getting-started.md): install agx, add your first
  instructions, skill, and MCP server, and launch a harness.
- [Configuration](configuration.md): where the configuration lives, how files
  and scopes fit together, and the commands that edit it for you.
- [Harnesses](harnesses.md): launching Codex, Claude Code, and Pi; your own
  harness definitions; shims.
- [MCP servers](mcp.md): adding servers, logging in with OAuth, and testing them.
- [Skills, instructions, and packages](skills-and-packages.md): where they come
  from, installing, locking, and updating.
- [Secrets](secrets.md): API keys, tokens, `!secret`/`!env`/`!file`, and
  sops-encrypted files.
- [Launcher and sessions](sessions.md): the interactive launcher and resuming
  earlier conversations.
- [Integrations](integrations.md): running agents through agx from other
  tools, such as T3 Code.
- [Web UI](web-ui.md): `agx ui`, the runtime in a browser.
- [The daemon](daemon.md): what runs in the background, and how to start, stop,
  and restart it.
- [Troubleshooting](troubleshooting.md): `agx doctor` and common problems.
- [Security model](security.md): what agx protects, and what it does not.

## Reference

- [CLI reference](cli-reference.md): every command and flag, exit codes, and
  JSON output.
- JSON Schemas for [configuration files](../schema/agx.schema.json) and
  [package manifests](../schema/agx.package.schema.json).
- [Glossary](../GLOSSARY.md) and [changelog](../CHANGELOG.md).

## Working on agx

[Development notes](development/README.md), the management API, and the
[architecture decision records](adr/) live separately from these usage docs.
