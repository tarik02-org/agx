# Harnesses

A harness is a coding agent agx launches. agx knows three: **Codex**
(`codex`), **Claude Code** (`claude`), and **Pi** (`pi`). You can define your
own variants of them with extra arguments, environment, and resources.

## Launching

```sh
agx codex
agx claude --print 'Review the current change'
agx pi --mode rpc
agx codex-work                   # a harness definition of yours
agx run codex-work -- --help     # the same, by name
```

Everything after the harness name goes to the harness unchanged, as do standard
input and output, exit codes, and Ctrl-C. agx options such as `--config` go
before the harness name. agx's own messages go to stderr, so protocol modes
(`codex app-server`, `pi --mode rpc`, `claude --print --output-format
stream-json`) stay clean.

Each launch gets the resources selected for the current directory:

- **Instructions** are added to the system prompt (Codex
  `developer_instructions`, Claude `--append-system-prompt-file`, Pi
  `--append-system-prompt`). Your project's own `AGENTS.md` or `CLAUDE.md`
  still load as usual.
- **Skills** appear under their names: `agx-managed:<name>` in Claude Code,
  `<name>` in Pi, and as a plugin per skill in Codex.
- **MCP servers** appear as `agx-<id>`, for example `agx-github`. The harness
  talks to them through the agx daemon, which holds their credentials. Every
  managed launch also gets the built-in [`agx` server](mcp.md#the-built-in-agx-server)
  for resource discovery, configuration edits and temporary MCP attachments.

Codex's `-C`/`--cd` changes the directory agx resolves resources for.

### Commands without managed resources

Help, version, login, and other management commands of the harness (for
example `agx codex --version`, `agx claude mcp list`, `agx codex login`) run
the harness directly without managed resources. The daemon resolves the harness
definition's `environment` and its arguments for the `passthrough` mode,
so the environment's `!env` and `!secret` values must resolve for these
commands too.

### Which executable runs

agx finds the harness on the `PATH` of the shell you launch from, skipping agx
itself and its shims. That means per-project tool versions from mise, direnv,
or a Nix shell are respected: `agx codex` runs the `codex` that `codex` would.

To pin a specific binary, set `executable` to an absolute path (`~/` allowed):

```yaml
harnesses:
  codex:
    executable: ~/.local/bin/codex
```

## Harness definitions

Configuring a built-in name customizes it; any other name defines a new harness
based on one:

```yaml
harnesses:
  claude:
    add:
      skills: [review]            # every Claude launch, in every directory

  codex-work:
    base: codex
    args: [--profile, work]
    environment:
      CODEX_HOME: /Users/me/.codex-work
      OPENAI_API_KEY: !secret work-openai
    add:
      mcp: [jira]
    remove:
      skills: [personal-notes]
```

Each definition becomes a command: `agx codex-work`. To run it as plain
`codex-work` from any shell or tool, install [shims](#shims). Create one from
the command line with:

```sh
agx harness add codex-work --base codex --env 'OPENAI_API_KEY=!env WORK_OPENAI_KEY' -- --profile work
```

| Field | Meaning |
| --- | --- |
| `base` | the harness this one extends: a built-in or another definition |
| `executable` | absolute path of the binary; default: found on `PATH` |
| `args` | arguments placed before the ones you type |
| `environment` | environment variables for the harness |
| `settings` | the harness's own settings, see [Settings](#settings) |
| `add`, `remove` | resources for every launch of this harness, applied after directory rules |
| `hidden` | `true` leaves the harness out of the [launcher](sessions.md)'s new-session list |

A definition inherits from its base: arguments add up (base first),
`environment` and `settings` merge key by key, and `executable` is replaced; `hidden` is not
inherited. Names cannot clash with agx commands such as `install`.

### Arguments per mode

agx sorts every launch into a mode, so arguments can apply only where they make
sense:

| Harness | Modes |
| --- | --- |
| codex | `interactive`, `exec`, `resume`, `app-server`, `passthrough` |
| claude | `interactive`, `print`, `resume`, `passthrough` |
| pi | `interactive`, `print`, `rpc`, `resume`, `passthrough` |

For example, `codex exec …` is `exec`, `claude -p …` is `print`, and `claude
--resume …` is `resume`; help, version, and login commands are `passthrough`.

`args` is either a flat list, used for every mode except `passthrough`, or a
list of `{values, modes}` groups:

```yaml
harnesses:
  codex-cpa:
    base: codex
    args:
      - values: [--search]                       # every mode except passthrough
      - values: [--profile, cpa]
        modes: [interactive, exec, resume]       # only these modes
```

### Settings

`settings` holds the harness's own configuration, written in YAML with the keys
of Codex's `config.toml` or Claude Code's `settings.json`. agx passes it to
every launch of the definition without touching your files:

```yaml
harnesses:
  codex-cpa:
    base: codex
    environment:
      CPA_API_KEY: !secret cpa-key
    settings:
      model_provider: cpa
      model_reasoning_effort: high
      model_providers:
        cpa:
          name: CPA
          base_url: http://127.0.0.1:8317/v1
          env_key: CPA_API_KEY          # Codex reads the key from this variable
          wire_api: responses

  claude-work:
    base: claude
    settings:
      model: opus
      permissions:
        allow: ["Bash(npm test:*)"]
        deny: ["Read(./.env)"]
```

- **Codex** gets one `-c key.path=value` per setting, before agx's own
  overrides and the definition's `args`. A setting merges with the table your
  `config.toml` has at that path, so `model_providers.cpa` above keeps any
  other keys you set there. Your own `-c` and the definition's `args` win
  over a setting of the same key. The native MCP server names `agx` and
  `agx-*`, and skill plugins under `@agx-managed`, are reserved; definitions
  cannot put settings at those paths.
- **Claude Code** gets the settings as a file passed with `--settings`, which
  applies above your user and project settings for that session only. Lists
  such as `permissions.allow` add to the ones in your files. Use `settings`
  or `--settings` in `args`, not both.
- **Pi** has no way to take settings per launch; use flags such as
  `--provider`, `--model`, and `--thinking` in `args`.

agx does not check the keys; the harness does when it starts. `agx doctor`
runs Codex once with each definition's settings to report what Codex rejects,
such as a model provider it cannot find. `agx explain --harness <name>` shows
the settings a definition ends up with and the line that set each one.

A definition's settings merge over its base's key by key; `null` removes an
inherited key, and a list replaces the inherited one. `!file` reads a value
from a file. `!secret` and `!env` are not allowed, because Codex gets settings
as process arguments that other processes can read: put the secret in
`environment` and name the variable in settings, as `env_key` does above.

### Environment and secrets

`environment` values are literal strings, or tags resolved at each launch:

```yaml
environment:
  REGION: eu
  API_KEY: !secret cpa-key       # a configured secret
  TOKEN: !env WORK_TOKEN         # from the shell you launch from
  PROMPT: !file ./prompt.txt     # a file's contents
```

`args` accept literal strings and `!file`. Put secrets and environment
variables in `environment`: `!secret` and `!env` are rejected in `args`, since
command lines are visible to other processes.

agx does not expand `~` or `$VAR` in `args` or `environment` values; the
harness gets them exactly as written. Names starting with `AGX_` are reserved.
See [Secrets](secrets.md) for the tags and secret kinds.

### Hiding harnesses from the launcher

When you only use a harness through definitions, such as Codex through a
gateway, hide the built-in so the launcher offers just the definitions:

```yaml
harnesses:
  codex: {hidden: true}
  codex-cpa:
    base: codex
    settings: {model_provider: cpa}
```

A hidden harness still runs by name (`agx codex`, or a `codex` link), and its
sessions still appear in the launcher and resume with it.

## Shims

A shim is a link named after a harness definition that runs agx as that
definition, so `codex-work` works in any shell, editor, or tool that expects a
plain executable.

```sh
agx shims install       # make the shim directory match your definitions
agx shims list          # show the shims and their state
agx shims remove        # delete the shim directory
```

Shims go to `~/.local/share/agx/shims` (`%LOCALAPPDATA%\agx\shims` on Windows).
Add that directory to your `PATH`; `install` and `list` warn when it is missing
or when another program with the same name comes first. Run `agx shims install`
again after adding or removing definitions. Built-in harnesses get no shim. The
directory is agx's: `install` removes any other file in it.

Any link to agx named after a harness works the same way, built-ins included:
a `codex` link to agx, earlier on `PATH` than Codex itself, runs `agx codex`.
agx then finds the native executable by skipping every path to agx itself, or
uses `harnesses.codex.executable` when set. Package managers such as Nix can
install these links themselves instead of running `agx shims install`;
`agx doctor` accepts definitions that `PATH` already runs as agx this way.

On Unix shims are symlinks; on Windows they are copies of agx named
`<name>.exe` that run the agx recorded beside them, so they keep working after
agx updates.

## Harness-specific notes

### Codex

- Codex loads skills only as plugins, so agx registers a local plugin
  marketplace and installs each selected skill as a plugin. `agx install` does
  this, and a Codex launch that finds a plugin missing installs it first.
- Only skills that some rule or harness definition selects become plugins.
  Codex also enables installed plugins when you run it without agx, so a
  package's other skills stay uninstalled until you select them.
- A definition can set `CODEX_HOME` in `environment` to give its launches their
  own Codex configuration; agx installs plugins there.
- When Codex plugins are turned off (`features.plugins = false`), launches warn
  and start without skills; instructions and MCP servers still work.
  `-c features.plugins=true` in a definition's `args` turns them back on for it.
- `agx codex app-server` over stdio (as T3 Code uses it) gives each thread the
  resources of that thread's own directory.

### Claude Code

Claude Code driven by the Agent SDK (`--input-format stream-json`) still gets
agx's instructions, combined with the host's own system prompt additions.

### Supported versions

| Harness | Minimum | Tested with |
| --- | --- | --- |
| Codex | 0.160 | 0.162.1 |
| Claude Code | 2.1.29x | 2.1.300 |
| Pi | 0.99.0 | 1.2.0 |

Older versions may start but lose skills, instructions, or MCP servers.
`agx doctor` checks the installed versions.

On Windows, harness executables must be `.exe` files.
