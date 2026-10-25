# Configuration

agx reads one YAML configuration that says which instructions, skills, and MCP
servers each directory gets. You can build it entirely with commands such as
`agx mcp add`, write it by hand, or mix both: the commands keep your comments
and layout.

## Where files live

| What | Default location | Override |
| --- | --- | --- |
| Configuration | `agx/agx.yaml` in your config directory | `AGX_CONFIG` or `--config` |
| Cache: installed packages, generated files, daemon log | `agx` in your cache directory | `AGX_CACHE` or `--cache` |
| State: launch records, the daemon's saved launches | `agx` in your state directory | `AGX_STATE` |

The base directories follow the XDG variables (`XDG_CONFIG_HOME`,
`XDG_CACHE_HOME`, `XDG_STATE_HOME`) and otherwise default to:

| | Linux | macOS | Windows |
| --- | --- | --- | --- |
| Config | `~/.config` | `~/Library/Application Support` | `%LOCALAPPDATA%` |
| Cache | `~/.cache` | `~/Library/Caches` | `%LOCALAPPDATA%\cache` |
| State | `~/.local/state` | `~/Library/Application Support` | `%LOCALAPPDATA%` |

`AGX_CONFIG`, `AGX_CACHE`, and `AGX_STATE` must be absolute paths. When you set
only `AGX_CACHE` (or `--cache`), state moves to `<cache>/state`, so one variable
is enough for a fully separate setup, for example to experiment:

```sh
export AGX_CONFIG=/tmp/agx-test/agx.yaml AGX_CACHE=/tmp/agx-test/cache
```

`--config` and `--cache` go before the command: `agx --config ./agx.yaml explain`.
A `--config` file that does not exist is an error, so a typo cannot silently
give you an empty configuration; only the `add` commands create it. The default
location and `AGX_CONFIG` may point at a file that does not exist yet: agx then
runs with an empty configuration, and the first `add` creates it.
Use the same configuration and cache for every agx command in a session; the
daemon serves one configuration at a time (see [The daemon](daemon.md)).

agx does not look for configuration files inside projects. A directory gets its
resources from rules in your configuration that name it.

## A first configuration

```yaml
instructions:
  pnpm:
    text: Use pnpm, not npm.

mcp:
  docs:
    url: https://docs.example.com/mcp

scopes:
  - add:                        # no directory: everywhere
      instructions: [pnpm]
  - directory: ~/work
    add:
      mcp: [docs]
      skills:
        - ~/skills/review       # a skill folder, defined right here
  - directory: ~/work/legacy
    remove:
      instructions: [pnpm]
```

- Top-level sections (`instructions`, `skills`, `mcp`, `secrets`, `bundles`,
  `packages`, `harnesses`) define things by ID.
- `scopes` decide where they are used. Each rule adds or removes resources for
  a directory and everything below it; a rule without `directory` applies
  everywhere.
- A selected MCP server is in the harness's tool list, unless it sets
  `load: on_demand`: then agents reach it with `agx mcp call` instead, and its
  tools cost no context until used. See
  [On-demand servers](mcp.md#on-demand-servers).

Check what a directory ends up with:

```sh
agx explain ~/work/legacy
```

`explain` lists every selected and removed resource with the file and line that
decided it, and marks on-demand MCP servers. `agx config validate` checks the
whole configuration.

## Scopes

Rules apply from the shallowest directory to the deepest, so more specific
rules win:

1. Rules without `directory` come first, wherever they are written.
2. Then directory rules, parents before children. Rules for the same directory
   keep the order they are written in.
3. Within one rule, `add` happens before `remove`.

A deeper rule can remove what a parent added, and an even deeper rule can add
it back. Removing a skill removes it however many bundles supplied it, and
removing a bundle removes each of its resources. Directories match by whole path
components: `~/work/app` does not match `~/work/apple`. Symlinks are resolved
first.

Limit a rule to some harnesses with `harnesses`:

```yaml
scopes:
  - directory: ~/work
    harnesses: [claude]
    add:
      mcp: [browser]
```

The rule then applies only to Claude Code launches and to
[harness definitions](harnesses.md) based on it. `agx explain --harness claude`
shows the result for one harness; without `--harness`, `explain`
skips harness-specific rules.

## Selection lists

Every `add` and `remove` list (and every bundle and harness selection) accepts
three forms:

```yaml
add:
  skills:
    - review                          # an ID defined elsewhere
    - ./skills/deploy                 # a path: ID is the folder name
    - id: release-notes               # an inline definition
      description: Write release notes for the current tag.
      content: |
        Summarize the commits since the previous tag.
  instructions:
    - ./AGENTS.md                     # a path: ID is the file stem, "AGENTS"
    - id: tone
      text: Be brief.
  mcp:
    - id: tracker
      url: https://tracker.example.com/mcp
```

- A string starting with `.`, `/`, or `~` is a path. MCP servers have no path
  form.
- Any other string is an ID.
- A mapping is a full definition with an `id`.

An inline definition is the same as a top-level one: its ID must be unique, and
other rules refer to it by ID. Remove it with its ID, not by repeating the path.

## Bundles

A bundle names a group of resources so you can enable them together:

```yaml
bundles:
  web:
    skills: [review, frontend]
    instructions: [pnpm]
    mcp: [browser]

scopes:
  - directory: ~/work/site
    add:
      bundles: [web]
```

Removing a bundle removes everything in it, even items another bundle also
added.

## Splitting configuration into files

The root file can import others. Relative paths are relative to the file that
contains them:

```yaml
imports:
  - personal.yaml           # must exist
  - agx.d/*.yaml            # every match, in name order; may match nothing
```

An import containing `*`, `?`, or `[` is a glob; any other import is a path.

Imports load in order, before the file that imports them. Name drop-ins
`10-team.yaml`, `50-local.yaml` to control their order.

When there is no root file at all, agx behaves as if it contained just
`imports: [agx.d/*.yaml]`, so a directory of drop-ins works without one.
The editing commands create the root file with that import when they first
write.

Generated configuration belongs in a drop-in too, so the root file stays yours.
The Home Manager module (`programs.agx.settings`, see the README) writes
`agx.d/10-home-manager.yaml` and warns at activation when an existing root file
does not import `agx.d/*.yaml`.

Defining the same ID twice is an error, unless the later definition says
`override: true`. Harness definitions are the exception: they merge field by
field across files. Each `daemon` setting may be set in one file.

### Paths

- Relative paths in a file are relative to that file.
- `~/` expands to your home directory in directories, imports, resource paths,
  harness executables, MCP server commands, and `!file` values. `~user` and
  `$VAR` are not expanded.
- Keep machine-specific paths in a separate file, such as an ignored
  `agx.d/50-local.yaml`, so the rest can live in a Git repository.

### A configuration repository

Your configuration directory can be a Git repository you clone onto each
machine. Keep secrets out of it with [secrets](secrets.md): environment
variables, the OS keychain, or [sops](secrets.md#sops-encrypted-files) files
encrypted in the repository. Commit lockfiles so every machine installs the same
package versions. On a new machine:

1. Clone the repository as your configuration directory, for example
   `~/.config/agx`.
2. Provide whatever your secrets need, such as the sops age key.
3. Run `agx install --frozen` to install exactly the locked packages.
4. Run `agx secret login <id>` for each MCP server that uses OAuth, and `agx
   secret set <id>` for each keychain secret. Both stay in each
   machine's keychain.

## Editing with commands

These commands change the configuration for you:

```sh
agx mcp add docs https://docs.example.com/mcp
agx skill add anthropics/skills --skill pdf
agx instructions add ./AGENTS.md --scope ~/work
agx harness add codex-work --base codex -- --profile work
agx use docs --scope ~/work/site
agx unuse docs --scope ~/work/oss
agx mcp remove docs
```

`add` defines something and enables it. `use` enables something already
defined; `unuse` disables it. `list` shows what is defined, and `remove` deletes
a definition.

Options shared by the commands that write:

| Option | Meaning |
| --- | --- |
| `--scope user` | everywhere (the default) |
| `--scope here` | the current directory and below |
| `--scope <dir>` | that directory and below; it must exist, except for `unuse` |
| `--harness <name>` | only for launches of that harness; repeatable |
| `--file <path>` | write new definitions into this file instead of the root |
| `--define-only` | define without enabling (`add` commands) |
| `-y`, `--yes` | write without asking |

On a terminal, each command shows the change as a diff and asks before writing;
missing arguments are asked for. Without a terminal, it writes without asking,
and a missing argument is an error.

A few things worth knowing:

- Edits keep comments, blank lines, key order, and quoting. The result is
  validated before anything is written, so a command never leaves an invalid
  configuration behind.
- `unuse` removes the ID from the matching rule's `add` list, or, when a
  broader rule adds it, adds it to a `remove` list. `use` undoes that.
- `remove` refuses while other rules or bundles still refer to the definition
  and lists them; `--force` removes those references too.
- Files that are read-only, or inside `/nix/store`, are refused; use `--file`
  to write somewhere else.
- Definitions that come from packages cannot be removed one by one. `agx skill
  remove <package-id>` removes a whole package of skills, such as one `skill
  add` declared, with its lock entries and the rule `add` entries that enable
  its skills; other references to them still need `--force`.

Anything the commands do not cover, such as bundles or `order`, you write by
hand.

## Editor support

agx publishes JSON Schemas for completion and checking in editors that use
yaml-language-server (VS Code's YAML extension, Neovim, Helix, and others). Add
this line at the top of a configuration file:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/tarik02-org/agx/master/schema/agx.schema.json
```

Use `agx.package.schema.json` for package manifests. `agx config schema` (or
`agx config schema --package`) prints the schema matching your agx version. To
stop the editor complaining about agx's tags, declare them:

```json
"yaml.customTags": ["!env scalar", "!file scalar", "!secret scalar"]
```

The schema checks structure only; `agx config validate` also checks references
and paths.

## Daemon settings

```yaml
daemon:
  autostart: true               # start the daemon when a command needs it
  lease_seconds: 60             # how long a launch survives without its wrapper
  renew_seconds: 20             # how often wrappers renew (less than the lease)
```

All of these are optional. See [The daemon](daemon.md).

The daemon rereads configuration when a file it read changes. A configuration
that fails to load does not replace the last valid one: running launches keep
theirs, and new launches fail until it is fixed.

## Related pages

- [Harnesses](harnesses.md): the `harnesses` section.
- [MCP servers](mcp.md): the `mcp` section.
- [Skills, instructions, and packages](skills-and-packages.md): `skills`,
  `instructions`, `packages`, and lockfiles.
- [Secrets](secrets.md): the `secrets` section and the `!secret`, `!env`, and
  `!file` tags.
