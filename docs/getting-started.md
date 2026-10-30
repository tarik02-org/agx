# Getting started

This walk-through installs agx, gives your agents some instructions, a skill,
and an MCP server, and launches a harness with them. It takes about five
minutes.

You need at least one harness installed on its own: [Codex](https://github.com/openai/codex),
[Claude Code](https://docs.anthropic.com/en/docs/claude-code), or
[Pi](https://github.com/badlogic/pi-mono). agx launches them; it does not
install or update them. Git is needed for skills and packages from Git.

## 1. Install agx

Pick one of the methods in the [README](../README.md#install), then check it:

```sh
agx --version
```

## 2. Try the bundled example (optional)

Release archives and the source tree contain `examples/agx.yaml`, a small
configuration whose fragment, `examples/main.yaml`, adds one set of instructions
for the directory it sits in.
From the unpacked archive or a checkout:

```sh
agx --config examples/agx.yaml config validate
agx --config examples/agx.yaml explain .
agx --config examples/agx.yaml codex       # or claude, or pi
```

`explain` shows that the `repository` instructions are selected here, and
the harness starts with them. `--config` points agx at that file for one command
without touching your own configuration; the file must exist. The harness
command also starts the agx daemon for that file; once the harness exits, the
next command that uses your own configuration replaces that daemon on its own.

## 3. Add instructions

Instructions are text every agent session receives, without having to invoke
anything:

```sh
agx instructions add --id pnpm --text 'Use pnpm, not npm.'
```

agx asks before saving local configuration (on a terminal), and
writes your configuration file, creating it if needed. Without `--scope`, the
instructions apply everywhere.

## 4. Add a skill

Skills are folders with a `SKILL.md` that agents can invoke. Add some from a
GitHub repository and pick which ones:

```sh
agx skill add anthropics/skills --skill pdf --skill docx
```

Without `--skill`, agx lists the repository's skills and lets you choose. A
local folder works too: `agx skill add ./skills/review`.

## 5. Add MCP servers

```sh
cd ~/work/my-project
agx mcp add fs --scope here -- npx -y @modelcontextprotocol/server-filesystem .
```

`--scope here` enables it in this directory and below, nowhere else. Anything
after `--` is the command that starts the server.

A remote server takes a URL instead. GitHub's MCP server, for example, works
with a [personal access token](https://github.com/settings/personal-access-tokens)
that agx keeps encrypted under an OS-keychain master key:

```sh
agx mcp add github https://api.githubcopilot.com/mcp/ --auth keychain
agx secret set github < github-token.txt
```

Reading the token from stdin keeps it out of your shell history; delete the file
afterwards. Servers that log in with OAuth instead are covered in
[MCP servers](mcp.md#logging-in).

## 6. See what a directory gets

```sh
agx explain
```

`explain` lists the instructions, skills, and MCP servers selected for the
current directory, and which line of which file selected each one. Run it in
another directory to compare.

## 7. Launch

```sh
agx codex          # or: agx claude, agx pi
```

Everything after the harness name goes to the harness unchanged, so
`agx claude --print 'Review the current change'` or `agx codex exec '…'` work
as usual. The agent sees your instructions, the skills, and the MCP servers as
`agx-fs` and `agx-github`.

Or run `agx` on its own for the [launcher](sessions.md): pick a harness, or
search and resume an earlier session.

## 8. Check the setup

```sh
agx doctor
```

`doctor` checks the configuration, installed packages, harness versions, the
daemon, MCP servers, and secrets, and prints a fix for each problem.

## Where things went

- Your configuration is `agx/agx.yaml` in your configuration directory
  (`~/.config` on Linux, `~/Library/Application Support` on macOS), which
  lists `agx.d/main.yaml`, where the commands above wrote your definitions.
  They save local definitions and rules; YAML overlays remain declarative. See
  [Configuration](configuration.md).
- The first command that needed it started the agx [daemon](daemon.md) in the
  background. It runs your MCP servers and holds their credentials.

## Next steps

- [Harnesses](harnesses.md): make `codex-work` or `claude-fast` variants with
  their own arguments, environment, and resources.
- [MCP servers](mcp.md): remote servers and OAuth logins.
- [Secrets](secrets.md): give MCP servers and harnesses API keys without
  writing them into files.
- [Shell completion](cli-reference.md#shell-completion) for configured IDs.
