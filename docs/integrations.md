# Integrations

Tools that start a coding agent for you (desktop apps, editors, orchestrators)
usually let you set the path of the agent's binary. Point that path at an agx
[shim](harnesses.md#shims) and every session the tool starts gets the skills,
MCP servers, and instructions of its working directory, exactly as `agx codex`
or `agx claude` would.

Built-in harnesses get no shim, so define a harness for the tool:

```yaml
harnesses:
  codex-app:
    base: codex
    hidden: true      # keep it out of the launcher
  claude-app:
    base: claude
    hidden: true
```

```sh
agx shims install
agx shims list        # prints the shims directory
```

A definition of its own also lets you give the tool something your terminal
sessions don't get, such as instructions about the tool's own MCP server (see
[T3 Code](#t3-code) below).

## T3 Code

[T3 Code](https://github.com/pingdotgg/t3code) runs Codex and Claude Code
sessions. In its settings, add a provider instance (or edit an existing one)
and set **Binary path** to the shim: `~/.local/share/agx/shims/codex-app` for a
Codex instance, `~/.local/share/agx/shims/claude-app` for a Claude instance.

The same settings in `~/.t3/userdata/settings.json`:

```json
{
  "providerInstances": {
    "codex_app": {
      "driver": "codex",
      "displayName": "Codex",
      "config": { "binaryPath": "/Users/me/.local/share/agx/shims/codex-app" }
    },
    "claude_app": {
      "driver": "claudeAgent",
      "displayName": "Claude",
      "config": { "binaryPath": "/Users/me/.local/share/agx/shims/claude-app" }
    }
  }
}
```

To have only agx-managed providers, disable T3 Code's built-in Codex and
Claude providers.

Each thread's working directory picks the scope, so a thread in
`~/work/acme-web` gets what `agx explain ~/work/acme-web` lists. agx handles the
modes T3 Code starts agents in, such as Codex's `app-server`.

To tell agents about T3 Code's own tools only when they run inside it, add the
instructions to the definitions above:

```yaml
instructions:
  t3code:
    text: |
      Running in T3 Code means the `t3-code` MCP tools are available.
      Use them for threads, delegation, and terminals.

harnesses:
  codex-app:
    base: codex
    hidden: true
    add:
      instructions: [t3code]
  claude-app:
    base: claude
    hidden: true
    add:
      instructions: [t3code]
```

Changes to agx configuration apply to the next session T3 Code starts.
