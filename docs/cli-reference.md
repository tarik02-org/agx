# CLI reference

Every command also accepts `-h`/`--help`, which is the authoritative list of
flags for your version. `agx man` prints a man page of every command.

## Global options

Put these before the command.

| Option | Meaning |
| --- | --- |
| `--config <file>` | root configuration file (default: `agx/agx.yaml` in your config directory; `AGX_CONFIG`); must exist, except for `add` commands, which create it |
| `--cache <dir>` | package and artifact cache (default: `agx` in your cache directory; `AGX_CACHE`) |
| `--color auto\|always\|never` | style output; `auto` styles terminals unless `NO_COLOR` is set |
| `-v`, `--version` | print the version |

### Environment variables

| Variable | Meaning |
| --- | --- |
| `AGX_CONFIG` | root configuration file, absolute |
| `AGX_CACHE` | cache directory, absolute |
| `AGX_STATE` | state directory, absolute; defaults to `<cache>/state` when the cache is not the default |
| `AGX_NO_AUTOSTART=1` | never start the daemon automatically |
| `AGX_LAUNCH_ID`, `AGX_MCP_GRANT` | set by agx in a harness's environment: the launch and its grant, which `agx mcp tools`, `agx mcp call`, and generated clients use to reach the launch's MCP servers |
| `NO_COLOR`, `CLICOLOR_FORCE`, `FORCE_COLOR` | turn styling off, or force it on |

See [Where files live](configuration.md#where-files-live) for the defaults.

## Harnesses

| Command | Description |
| --- | --- |
| `agx` | open the [launcher](sessions.md) on a terminal; print help otherwise |
| `agx codex [args...]` | launch Codex with this directory's resources |
| `agx claude [args...]` | launch Claude Code |
| `agx pi [args...]` | launch Pi |
| `agx <definition> [args...]` | launch one of your [harness definitions](harnesses.md#harness-definitions) |
| `agx run <harness> -- [args...]` | launch a harness by name |
| `agx sessions [query] [--all] [--json]` | list sessions to resume, here or (`--all`) everywhere |
| `agx shims install\|list\|remove [--dir <dir>] [--json]` | manage [shims](harnesses.md#shims) |

Arguments after a harness name go to the harness unchanged; agx's own options
go before it.

## Configuration

| Command | Description |
| --- | --- |
| `agx config validate [--json]` | load and check the whole configuration |
| `agx config schema [--package]` | print the JSON Schema for configuration files or package manifests |
| `agx resolve [dir] [--harness <name>] [--json]` | show the resources selected for a directory |
| `agx explain [dir] [--harness <name>] [--verbose] [--json]` | show why each resource is selected or removed |
| `agx install [--frozen] [--json]` | install packages and write lockfiles; register Codex skill plugins |
| `agx update [package-id...] [--json]` | move packages to their newest commit (all when none given) |

### Editing

| Command | Description |
| --- | --- |
| `agx mcp add <id> <url>` | define a remote MCP server and enable it |
| `agx mcp add <id> -- <command> [args...]` | define a local MCP server and enable it |
| `agx mcp list` | list MCP servers |
| `agx mcp remove <id>` | remove an MCP server definition |
| `agx mcp login <id> [--no-browser]` | log in to an MCP server with OAuth, as `agx secret login <id>` |
| `agx mcp test <id>` | connect to an MCP server and list its tools |
| `agx skill add <source>` | add skills from a folder, Git URL, or `owner/repo[@ref][/path]` |
| `agx skill list` | list skills |
| `agx skill remove <id \| package-id>` | remove a skill definition, or a Git package `skill add` declared with the rule entries enabling its skills |
| `agx instructions add <path>` | add instructions from a file |
| `agx instructions add --id <id> --text <text>` | add instructions from text |
| `agx instructions list` | list instructions |
| `agx instructions remove <id>` | remove instructions |
| `agx harness add <name> --base <harness> [-- args...]` | define a harness |
| `agx harness list` | list harnesses, built-ins included |
| `agx harness remove <name>` | remove a harness definition |
| `agx use <id>` | enable a defined resource in a scope |
| `agx unuse <id>` | disable a resource in a scope |

All of them take `--json`. Commands that write take:

| Flag | Meaning |
| --- | --- |
| `--scope user\|here\|<dir>` | where to enable: everywhere (default), here and below, or a directory |
| `--harness <name>` | only for these harnesses; repeatable |
| `--file <path>` | fragment to write new definitions to (default: the root configuration) |
| `--define-only` | define without enabling (`add` only) |
| `-y`, `--yes` | write without confirming; implied without a terminal |
| `--force` | `remove`: also remove the entries that refer to the definition (a package's enabling rule entries go without it) |
| `--kind skills\|instructions\|mcp\|bundles` | `use`/`unuse`: which kind, when an ID names several |

Command-specific flags:

| Command | Flags |
| --- | --- |
| `mcp add` | `--header KEY=VALUE` (remote), `--env KEY=VALUE` (local), `--auth none\|oauth\|keychain\|'!secret <id>'`, `--client-id <id>` (OAuth public client), `--description <text>`, `--load native\|on_demand`, `--share none\|directory\|repository\|global` (local), `--no-browser` |
| `skill add` | `--skill <name>` (repeatable), `--all`, `--ref <ref>`, `--path <dir>`, `--package <id>` |
| `instructions add` | `--id <id>`, `--text <text>` |
| `harness add` | `--base <harness>`, `--executable <path>`, `--env KEY=VALUE` (repeatable) |

In `--header` and `--env`, a value written `!secret <id>`, `!env NAME`, or
`!file PATH` becomes that [tag](secrets.md#using-values-from-elsewhere).

## Runtime

| Command | Description |
| --- | --- |
| `agx doctor [dir] [--all] [--json]` | check configuration, harnesses, daemon, MCP servers, secrets, and shims |
| `agx daemon start [--foreground] [--json]` | start the daemon |
| `agx daemon status [--json]` | show whether it runs, its version, and its launches |
| `agx daemon stop [--force] [--timeout <duration>] [--json]` | stop it, keeping running launches unless `--force` |
| `agx daemon restart [--force] [--timeout <duration>] [--json]` | stop it and start this version |
| `agx launch list [--json]` | list launches |
| `agx launch show <launch-id> [--json]` | show one launch and its MCP servers |
| `agx launch revoke <launch-id> [--json]` | cut off a launch's MCP access |
| `agx secret status [secret-id] [--json]` | show secret states, never values |
| `agx secret set <secret-id> --stdin [--json]` | store a keychain secret read from stdin |
| `agx secret login <secret-id> [--no-browser] [--json]` | log in to an OAuth secret |
| `agx secret logout <secret-id> [--json]` | delete a stored secret or OAuth login |

See [The daemon](daemon.md) and [Secrets](secrets.md).

## Calling MCP servers

| Command | Description |
| --- | --- |
| `agx mcp tools [<server>] [--schema] [--all] [--json]` | list the servers you can call with their state, load mode, description, and tools; with a server, its tool signatures |
| `agx mcp call <server>.<tool> [key=value ...]` | call a tool and print its result |
| `agx mcp types [<server> ...] [--out <dir>] [--all] [--json]` | print TypeScript declarations for the tools; with `--out`, write them with an ES module client |
| `agx mcp ps [--cwd <dir>] [--under <dir>] [--json]` | list the running local MCP server processes, shared or not; `--cwd` keeps those working in exactly that directory, `--under` those in it or below it |
| `agx mcp stop <pid> [--json]` | stop a running local MCP server process; its sessions reconnect and the next one starts it again |

`mcp call` flags:

| Flag | Meaning |
| --- | --- |
| `key=value` | an argument, converted by the tool's schema; `key=@file` reads a file, `@@` escapes a leading `@` |
| `--args <json\|->` | the arguments as a JSON object, or `-` to read it from standard input; `key=value` pairs override its members |
| `--raw-strings` | pass `key=value` values as strings, without conversion |
| `--save-dir <dir>` | where binary content goes (default: a new temporary directory per call) |
| `--timeout <duration>` | how long to wait for the result (default `60s`) |
| `--all` | reach every configured server, not only those of this directory or launch |

Inside a launch, these commands reach that launch's servers; elsewhere, the
servers selected for the current directory. `call` exits 1 when the tool
reports an error, after printing the result. See
[Calling servers from the shell](mcp.md#calling-servers-from-the-shell).

## Shell completion

```sh
source <(agx completion zsh)            # zsh: add to ~/.zshrc, after compinit
source <(agx completion bash)           # bash: ~/.bashrc; needs bash-completion
agx completion fish | source            # fish: ~/.config/fish/config.fish
agx completion powershell | Out-String | Invoke-Expression   # PowerShell: $PROFILE
```

Completion offers the IDs in your configuration (MCP servers, skills,
instructions, bundles, secrets, packages, harnesses) wherever a command takes
one, and launch IDs from a running daemon. It never starts the daemon.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | success |
| 1 | the command failed |
| 2 | invalid arguments or flags |
| 128 + n | stopped by signal n (130 for Ctrl-C) |

Harness commands exit with the harness's own status.

## JSON output

Every command that does something takes `--json`, except harness launches, the
launcher, `config schema`, `completion`, and `help`. With `--json`:

- stdout carries only JSON, one document per line; messages and warnings stay
  on stderr;
- nothing is asked: `--json` implies `--yes`, and a missing argument is an
  error.

A successful command prints a `data` document:

```json
{"version":1,"data":{"secret_id":"docs-key","logged_out":true}}
```

and a failed one an `error` document, with the same exit code as without
`--json`:

```json
{"version":1,"error":{"code":"not_found","message":"unknown MCP server docs"}}
```

Conventions:

- Field names are `snake_case`; enum values are lowercase strings.
- Times are UTC, in RFC 3339 format (`2026-10-19T12:05:00Z`).
- Every field is always present: a value that doesn't apply is `null`, an
  empty list is `[]`, and an empty map is `{}`.
- An object's own identifier is `id` (`name` for harnesses); a reference to
  another object is `<kind>_id`, such as `mcp_id` or `secret_id`.
- List commands wrap the list in an object, such as `{"mcp":[…]}` or
  `{"launches":[…]}`.
- An enum value this build of agx doesn't know is printed as `"unknown"`.
- `error.code` is a stable, machine-readable string; branch on it rather than
  on `message`. It is one of:

  | Code | Meaning |
  | --- | --- |
  | `usage` | invalid arguments or flags |
  | `config_invalid` | the configuration doesn't load |
  | `not_found` | the named thing doesn't exist |
  | `conflict` | the change clashes with existing state |
  | `read_only` | the target can't be edited |
  | `invalid_request` | the request was rejected as invalid |
  | `failed_precondition` | the command can't run in the current state |
  | `needs_login` | a secret needs `agx secret login` first |
  | `mcp_failed` | an MCP server failed the request, or could not be reached or started |
  | `daemon_unavailable` | the daemon isn't running and couldn't start |
  | `version_mismatch` | the CLI and the daemon are different versions |
  | `network` | a network request failed |
  | `timeout` | the command timed out |
  | `interrupted` | the command was interrupted |
  | `internal` | anything else; see `message` |
- `version` changes only when a document changes incompatibly; new fields may
  appear at any time.

The fields of each command's `data` are what `--json` prints; try a command to
see its shape.

`agx secret login` and `agx mcp login` stream events instead, one per line, as
the login progresses:

```json
{"version":1,"event":"started","secret_id":"github","mcp_id":"github","url":"https://github.com/login/oauth/authorize?…","expires_at":"2026-10-19T12:05:00Z"}
{"version":1,"event":"finished","secret_id":"github","outcome":"success","message":null}
```

With `--json`, the browser opens only when stdout is a terminal, so a script can
open the URL itself.

Examples:

```sh
agx config validate --json | jq .data.valid
agx mcp list --json | jq -r '.data.mcp[].id'
agx doctor --json | jq '.data.checks[] | select(.status != "pass")'
```
