# Subagents as a selectable resource

Status: accepted.

Claude Code, Codex and OpenCode each let a session delegate work to a named agent
with its own prompt, and each reads those agents from its own directories in its
own format. A subagent defined for one harness is invisible to the others, and
none of them can limit a user-level subagent to some directories. AGX already
solves both problems for skills, so subagents become a resource of the same kind.

A subagent is defined under `agents` with a `description` and `content`, the
prompt it runs with. A `path` or `git` source is a Markdown file whose front
matter holds the description and whose body is the prompt; AGX reads no other
front matter field, since those belong to the harness the file was written for.
Scopes, bundles, packages and harness definitions select subagents under
`add.agents` and `remove.agents`, and a snapshot lists them like any other
resource, with their content in its identity. The resource is called a subagent
because the glossary already uses "agent" for the harness itself; the
configuration key stays `agents`, the name all three harnesses use.

Only the name, description and prompt mean the same thing everywhere. Models,
effort levels, tool lists and permissions exist in each harness but with native
values, so a definition carries them in a `harnesses` map keyed by built-in
harness, passed through unchecked like harness `settings`:

```yaml
agents:
  reviewer:
    description: Reviews a diff for correctness bugs.
    content: |
      You review changes for ...
    harnesses:
      claude: { model: opus, tools: [Read, Grep] }
      codex: { model_reasoning_effort: high }
      opencode: { steps: 20 }
```

AGX does not translate these values between harnesses. A portable model or tool
vocabulary would need a mapping that goes stale with every harness release.

The native name defaults to the last segment of the ID. Selected subagents that
share a name each get a suffix from their ID, as skills do.

Delivery is per launch, so selection follows the directory without writing into
the harness's own directories:

- Claude Code takes the selected subagents as one inline `--agents` JSON object.
  The file form of that flag works only with `--print`. Subagents keep their
  bare names; the session plugin directory would prefix them `agx-managed:`.
  Claude Code keeps only the last `--agents`, so one given at launch replaces
  AGX's, and the launch warns rather than merging two sources of agents.
- Codex takes `-c agents.<name>.config_file=<path>` for a TOML file in the launch
  directory holding `name`, `description`, `developer_instructions` and the
  native keys. Codex names the role after the file's `name`, not the key. This
  key is absent from Codex's documentation, which describes agent files only in
  its own agents directories; those cannot follow the launch directory. Codex
  ignores keys it does not know, so nothing detects the key's removal short of
  spawning a subagent. App-server threads receive the same key in their thread
  configuration.
- OpenCode's plugin registers them through `ctx.agent.transform`, where
  `editor.update` creates an agent that does not exist yet, with mode `subagent`
  unless the native map says otherwise. The native map is decoded as an OpenCode
  agent configuration entry.
- Pi has no subagents. A Pi launch that selects any reports them as skipped.

A subagent whose name matches a built-in or a native file replaces or loses to
it by the harness's own rules; AGX does not detect the clash.

Subagents are declarative: configuration files and package manifests define and
select them. Local configuration does not store them, because its tables list
the resource kinds in constraints and adding one rebuilds three tables; the
editing commands follow once that migration exists. Skills and MCP servers of a
subagent are the session's. Narrowing them per subagent, and running a session
as a subagent (`claude --agent`, OpenCode primary agents), are separate work.

## Verification

Checked on 2026-11-10 with Claude Code 2.1.322, Codex 0.167.0 and OpenCode
2.2.5. Through a built AGX, Claude Code in print mode listed the selected
subagents under their bare names, and Codex listed them as roles in its spawn
tool with a native `model_reasoning_effort` applied. With the same override
given by hand, a spawned Codex thread followed the file's instructions. The
OpenCode plugin's end-to-end test finds the subagent, with a native `steps`, in
the selected directory only. Codex app-server threads are covered by the
adapter's plan alone.
