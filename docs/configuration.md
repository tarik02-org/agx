# Configuration

agx combines local configuration managed by its CLI and web UI with declarative
YAML overlays. Local definitions and scope rules live in `state.db`, keyed by
the canonical root configuration path. The root YAML file lists overlays and
holds daemon settings; it may be absent.

YAML overlays take precedence: a same-name definition replaces the entire local
definition, including harness definitions. Overlay rules follow local rules at
equal directory specificity. Agx commands change local configuration and refuse
to edit YAML-owned or package-owned definitions, naming their source.

## Where files live

| What | Default location | Override |
| --- | --- | --- |
| Configuration: the root file, with fragments beside it in `agx.d/` | `agx/agx.yaml` in your config directory | `AGX_CONFIG` or `--config` |
| Cache: installed packages, generated files, daemon log | `agx` in your cache directory | `AGX_CACHE` or `--cache` |
| State: local configuration, launch records, encrypted secrets (`state.db`) | `agx` in your state directory | `AGX_STATE` |

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
An explicit `--config` must name an existing root file or a root with saved local
configuration; adding a definition also permits a new path. The default location
and `AGX_CONFIG` may name an absent file, read as `fragments: [agx.d/*.yaml]`.
Commands save local data without creating YAML. Each canonical root has its own
daemon (see [The daemon](daemon.md)).

agx does not look for configuration files inside projects. A directory gets its
resources from rules in your configuration that name it.

## A first configuration

For a declarative overlay, write definitions and rules in a fragment such as
`agx.d/main.yaml`:

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

`explain` uses one compact row per selected or removed resource, with shortened
paths and the file and line that
decided it, and marks on-demand MCP servers. `agx config validate` checks the
whole configuration. For declarative builds, `agx config validate --yaml-only`
checks only the YAML configuration and its packages, without managed definitions
or a daemon. Home Manager uses this mode when checking its generated overlay.

## Scopes

Rules apply from the shallowest directory to the deepest, so more specific
rules win:

1. Rules without `directory` come first, wherever they are written.
2. Then directory rules, parents before children. At the same directory, local
   rules come before YAML rules; each group keeps its configured order.
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

## The root file and fragments

The root file holds only `fragments` and [daemon settings](#daemon-settings);
declarative definitions and rules live in the fragments it lists. Relative entries are
relative to the root file's directory, beside the link when the root is a
symlink:

```yaml
fragments:
  - personal.yaml           # must exist
  - agx.d/*.yaml            # every match, in name order; may match nothing
```

An entry containing `*`, `?`, or `[` is a glob; any other entry is a path.
Fragments load in list order and cannot list others. Name drop-ins
`10-team.yaml`, `50-local.yaml` to control their order.

When there is no root file at all, agx behaves as if it contained just
`fragments: [agx.d/*.yaml]`, so a directory of fragments works without one.

Generated configuration belongs in a fragment too. The Home Manager module
(`programs.agx.settings`, see the README) writes `agx.d/10-home-manager.yaml`,
writes the root file only when its settings include `daemon`, and warns at
activation when an existing root file does not list `agx.d/*.yaml`.

Within YAML overlays, defining the same ID twice is an error unless the later
definition says `override: true`. YAML harness definitions merge field by field.
A YAML definition always replaces a same-name local definition as a whole.

### Paths

- Relative paths in a file are relative to that file.
- `~/` expands to your home directory in directories, fragment entries, resource paths,
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
   machine's encrypted secret store.

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
| `--define-only` | define without enabling (`add` commands) |
| `-y`, `--yes` | write without asking |

On a terminal, missing arguments become prompts and commands ask before saving.
Without a terminal, commands save without asking and a missing argument is an
error. Every change is validated against the combined configuration before its
local definitions, rules, and package locks are committed in one transaction.
Concurrent edits fail with a conflict and can be retried.

- A same-directory YAML rule that opposes `use` or `unuse` is reported as
  read-only, so a command cannot silently save a change that YAML defeats.
- `unuse` removes an ID from a matching local rule's `add` list, otherwise adds
  it to `remove`. `use` removes a matching `remove`, otherwise adds it.
- `remove --force` also removes local references. YAML and package definitions
  stay read-only; a reference in an overlay may require changing that overlay.
- `agx skill remove <package-id>` removes a local package, its locks, and the
  local rules enabling its skills. Other references need `--force`.
- Local resource files remain ordinary files. The web UI can edit configured
  skill/instructions contents, including files named by YAML; package/cache
  contents and configuration files cannot be edited this way.

`agx config export` prints local definitions and rules as YAML, including
injection tags. Save it outside the root's overlay globs until you intend to
make it authoritative. Local relative paths are based at the canonical root's
directory; exported relative paths need that same base. Package locks of local
sources live in the database; run `agx install` after promoting them to YAML to
create portable lockfiles. Bundles and other declarations not covered by CLI
add commands can be supplied by the web UI or a YAML overlay.

## Editor support

agx publishes JSON Schemas for completion and checking in editors that use
yaml-language-server (VS Code's YAML extension, Neovim, Helix, and others). Add
this line at the top of a configuration file:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/tarik02-org/agx/master/schema/agx.schema.json
```

Use `agx.root.schema.json` for the root file and `agx.package.schema.json` for
package manifests. `agx config schema` (with `--root` or `--package`) prints the
schema matching your agx version. To
stop the editor complaining about agx's tags, declare them:

```json
"yaml.customTags": ["!env scalar", "!file scalar", "!secret scalar"]
```

agx checks every fragment and package manifest against the same schema when it
loads them, so an error names the key path, such as `mcp.github.command: not
allowed here`. `agx config validate` also checks references and paths.

## Daemon settings

These go in the root file:

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
