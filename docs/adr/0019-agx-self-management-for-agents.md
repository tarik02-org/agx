# Let agents manage agx configuration through a built-in MCP server

Status: deferred. Not implemented.

Users ask agents to "install this MCP server" or "add that skill"; without help,
agents edit harness-native files (`~/.claude.json`, Codex `config.toml`) that agx
neither sees nor scopes. agx therefore offers its own management surface to
agents, opt-in and gated, built on the editing commands of
[ADR 0014](0014-configuration-editing-commands.md).

Agents can also run the `agx` CLI from their shell like any other command; the
modes and limits below govern the built-in server, which suits harnesses
without a shell and users who want writes staged for review.

## The built-in server

A built-in MCP server, `agx`, is served by the daemon like any other entry and
selected like one: off until a scope enables it.

```yaml
scopes:
  - add:
      mcp:
        - id: agx-self
          builtin: agx
          mode: propose              # read | propose | write
          files: [agx.d/50-agent.yaml]
```

Its tools reuse the CLI's JSON code paths:

| Kind | Tools |
| --- | --- |
| Read | `explain`, `list` (MCP servers, skills, instructions, harnesses), `doctor`, `sessions`, `secret_status` |
| Write | `mcp_add`, `skill_add` (path, Git, `owner/repo` with a skill list), `instructions_add`, `use`, `unuse`, `remove` |
| Auth | `login`, returning the authorization URL for the user to open |

Every write returns the diff the CLI would show and is validated before it
applies. Tools never read or set secret values; `secret_status` reports state
only.

## Modes

- `read`: write tools are not listed.
- `propose` (default): a write is staged, not applied. `agx changes` lists staged
  changes with their diffs, the session that proposed them, and the reason the
  agent gave; `agx apply <id>` or `agx discard <id>` decides, and the launcher shows
  pending changes. A staged change that no longer applies cleanly is rejected at
  apply time.
- `write`: writes apply immediately, except the always-gated changes below.

Always gated, staged even in `write` mode: a new stdio MCP command, a skill or
package from a remote source, and any change to harness definitions or
executables. Each runs code chosen by whoever controls the agent's input, so a
person confirms it.

## Limits

- **Files:** writes may touch only the fragments listed in `files`, created on
  first write and imported by the default `agx.d/*.yaml` glob or an explicit
  import. The main configuration and read-only fragments are never written, so
  `git diff` shows exactly what agents changed.
- **Scope:** an agent can only enable resources in directories at or below its
  launch directory, or in its own harness scope; `--scope user` requires `write`
  mode or approval.
- **Effect:** changes apply to new sessions and resumes, never to the running
  one, bounding what a hostile edit can reach to sessions started afterwards.
- **Audit:** applied and proposed changes are appended to `<state>/changes.log`
  with the launch, harness, and diff.

## Bundled skill

A built-in `agx` skill, enabled with the server (`builtin: agx` adds both), tells
agents to use these tools for MCP servers, skills, and instructions instead of
editing harness-native configuration, and that changes are proposed until the user
approves.

Rejected: applying writes by default; a separate approval UI outside the CLI
and launcher.
