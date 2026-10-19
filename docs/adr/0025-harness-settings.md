# Give harness definitions native settings

Status: proposed.

A harness definition changes how a harness runs through `args` and
`environment`. For settings that is awkward. A Codex definition for a gateway
writes TOML by hand inside YAML (`[-c, 'model_provider="cpa"']`), and the
provider it names must already exist in the user's `~/.codex/config.toml` (or a
`CODEX_HOME` set up for it), so `codex-cpa` is not self-contained. A Claude Code
definition that wants its own model or permission rules has to put a JSON string
in `--settings`, which a derived definition cannot extend. Pi has no comparable
entry point at all.

Harness definitions, built-in overrides included, gain a `settings:` map written
in the harness's own configuration schema, which agx delivers per launch without
touching the user's files.

## Decision

```yaml
harnesses:
  codex: {hidden: true}
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
          env_key: CPA_API_KEY
          wire_api: responses

  claude-work:
    base: claude
    settings:
      model: opus
      permissions:
        defaultMode: acceptEdits
        allow: ["Bash(npm test:*)", "Bash(npm run lint:*)"]
        deny: ["Read(./.env)"]
```

`settings` is a free-form map. Keys and values are those of the harness's own
user configuration: Codex `config.toml`, Claude Code `settings.json`. agx does
not know or validate them; the harness does, at launch.

| Harness | Delivery | Native precedence |
| --- | --- | --- |
| Codex | one `-c <dotted.path>=<TOML value>` per leaf | above `config.toml` and `--profile`; later `-c` win |
| Claude Code | a generated file passed as `--settings <file>` | above user, project, and local settings; below managed settings |
| Pi | not supported; `settings` on a Pi-based definition is a validation error | (see [Pi](#pi)) |

### Codex

The resolved map is flattened to leaves, sorted by path, and each leaf becomes
`-c path=value`, the value encoded as TOML by agx (strings quoted and escaped,
numbers, booleans, arrays, and inline tables for maps inside arrays). The
example above becomes:

```
-c model_provider="cpa"
-c model_providers.cpa.base_url="http://127.0.0.1:8317/v1"
-c model_providers.cpa.env_key="CPA_API_KEY"
-c model_providers.cpa.name="CPA"
-c model_providers.cpa.wire_api="responses"
-c model_reasoning_effort="high"
```

- Leaves rather than one inline table per top-level key, because Codex applies a
  dotted override by walking and creating tables, so a leaf merges with the
  user's table at that path while an inline table replaces it. A
  `model_providers.cpa` in the user's `config.toml` keeps its other keys.
- Codex parses each value as TOML and, when that fails, silently uses the raw
  text as a string (`-c model=o3` works, `-c 'x=[a'` becomes the string `[a`).
  agx always encodes, so a YAML string is always a TOML string and a YAML list
  always a TOML array; there is no hand quoting.
- Codex splits the override path on every `.` with no quoting
  (`codex-rs/config/src/overrides.rs`, `path.split('.')`). A key that itself
  contains a dot (`projects."/src/a.b"`) cannot be a path segment; agx emits the
  nearest map above it as one inline table, which replaces that table, and
  `explain` says so.
- `null` has no TOML form. In a derived definition it removes an inherited key
  (below) and is never delivered.
- Codex validates when it loads configuration and fails the launch with its own
  message. In Codex 0.162.1, with an empty `CODEX_HOME`, `codex -c
  'model_provider="cpa"' features list` fails with "Model provider `cpa` not
  found"; adding `-c 'model_providers.cpa={name="CPA",
  base_url="http://127.0.0.1:9/v1", wire_api="responses"}'`, or the same as
  separate leaf overrides, succeeds; a provider with only `base_url` fails with
  "model_providers.cpa: provider name must not be empty".

`codex app-server` gets the same `-c` arguments on its process; threads add
their resources per request as before.

### Claude Code

agx writes the resolved map as JSON to `settings.json` in the launch's directory
under the cache (`launches/<id>/`, mode 0600, removed with the launch, as the Pi
MCP extension is) and passes `--settings <path>`. Claude Code applies it above
`~/.claude/settings.json`, `.claude/settings.json`, and
`.claude/settings.local.json` and below managed settings, for that session
only; it writes nothing back. Keys left out keep their values from the user's
files. List keys such as `permissions.allow` are combined with the lists in
those files rather than replacing them, so a definition can add rules but not
remove the user's. A file instead of inline JSON keeps the command line short
(Windows) and readable in `ps`.

Claude Code's own per-key flags (`--model`, `--effort`) override the matching
setting, so `agx claude-work --model sonnet` still picks Sonnet.

### Pi

Pi 1.2.0 (`@earendil-works/pi-coding-agent`, from
[earendil-works/pi](https://github.com/earendil-works/pi), to which
`badlogic/pi-mono` redirects) has no per-launch settings override that
leaves the user's files alone:

- No flag reads settings: `pi --help` lists none, and `cli/args.ts` parses no
  settings or config path. `SettingsManager.applyOverrides` exists, but only
  `--use-theme` calls it, and only on the startup settings manager.
- `settings.json` loads from the agent directory and then the trusted project's
  `.pi/settings.json`, deep-merged (`deepMergeSettings`, objects recursively,
  `defaultTools` with its `+name`/`-name` rule). `models.json` is read only from
  the agent directory. Writing either means writing the user's or the
  repository's files.
- `PI_CODING_AGENT_DIR` replaces the whole agent directory, not one file: auth,
  sessions, `models.json`, `mcp.json`, extensions, skills, and themes move with
  it, and `/login` or `/settings` would write into the replacement. An overlay
  directory would have to mirror and sync all of that.
- Extensions, which agx already generates per launch for MCP, cannot change
  settings. `pi.getSettings()` returns a copy; `ExtensionAPI` exposes no
  `SettingsManager`. They can call `pi.setModel()` and `pi.setThinkingLevel()`
  (session only, not saved) and `pi.registerProvider()`. But `registerProvider`
  takes a different model shape from `models.json` (`name`, `input`, `cost`,
  `contextWindow`, `maxTokens`, and `reasoning` required, no `modelOverrides`),
  and the user's `models.json` composes above registered providers.

So `settings` is rejected for Pi definitions, with an error pointing to `args`.
The common cases already have flags: `--provider`, `--model` (including
`provider/id:thinking`), `--thinking`, `--models`, `--tools`, and
`--exclude-tools`. See the open questions for ways to support Pi later.

## Merging

- **Base to derived**: settings deep-merge along the base chain, the built-in's
  own entry (`harnesses.codex.settings`) first. Maps merge key by key; scalars
  and lists from the derived definition replace the inherited value; `null`
  removes an inherited key. Lists replace rather than append so the result is
  what the definition says, not a concatenation the reader must reconstruct;
  Claude Code appends them to the user's own lists anyway.
- **Fragments**: as with `environment`, each leaf path may be set by one
  fragment of a definition; a second one is a conflict naming both sources.
  `override: true` replaces the earlier definition, settings included.
- **With `args`**: settings are placed before agx's own resource arguments,
  which come before the definition's `args`, which come before the arguments
  typed at launch: `settings`, agx resources, `args`, typed arguments. For Codex
  later `-c` win, so a definition's `args` and the user's own `-c` override its
  settings, and agx's `mcp_servers.agx-*`, `developer_instructions`, and plugin
  switches override a setting of the same path; validation rejects settings
  under `mcp_servers.agx-*` and `plugins.<name>@agx-managed` since they cannot
  take effect. For Claude Code there is one `--settings`; a definition that sets
  `settings` and also passes `--settings` in `args` is a validation error.
- **Against native configuration**: settings sit where each harness puts
  per-invocation overrides: above every user and project file, below
  organization-managed settings and the harness's dedicated flags.
- **Modes**: settings apply in every mode except `passthrough`, like ungrouped
  `args`. Management commands (`codex login`, `claude mcp list`) run as the user
  configured them; the open questions cover Codex commands that read the model
  provider.

## Tags

`settings` values are plain YAML. `!file` is allowed on a string and resolves
to the file's contents when the configuration loads, like in `args`. `!secret`
and `!env` are rejected everywhere in `settings`, for every harness:

- For Codex they would reach argv, which other processes can read; `args`
  already rejects them for that reason.
- For Claude Code they would go to a private file and would be safe there, but
  then the same YAML would be accepted for one harness and refused for another,
  and `explain`, `harness list --json`, and doctor output would have to redact
  some settings and not others.
- Each harness already takes credentials by indirection: Codex `env_key` and
  `env_http_headers`, Claude Code's environment variables and `apiKeyHelper`.
  The value goes into the definition's `environment` with `!secret` or `!env`,
  and `settings` names the variable, as `codex-cpa` above does with
  `CPA_API_KEY`.

Settings therefore never hold secrets and can be shown in full anywhere.

## Interactions

- **Schema**: `settings` is `{"type": "object"}` with additional properties
  allowed and a description naming each harness's native schema. Editors
  validate the agx side only.
- **`agx harness list --json`** shows each definition's own and resolved
  settings; **`explain --harness <name>`** shows the resolved map with the
  source of every leaf and, with `--verbose`, the delivered form (the `-c`
  arguments, or the path and contents of the Claude settings file).
  **`resolve`** is about resources and does not show them.
- **`agx doctor`** reports validation errors (Pi, reserved Codex paths,
  `--settings` in `args`) and, for Codex definitions with settings, runs
  `codex -c … features list` in the definition's environment, which loads the
  configuration and surfaces Codex's own error. There is no equally cheap check
  for Claude Code (below).
- **Launcher and sessions**: unchanged. A resumed session runs with the current
  settings of the definition it is resumed under.
- **`agx harness add`** gains no flag in this change; settings are edited in
  YAML.

## Alternatives

- **Typed fields** (`model:`, `model_provider:`, `permissions:`): every harness
  names and shapes these differently, agx would chase each harness's schema
  across releases, and any field it did not model would send users back to
  `args`. Free-form settings cost agx nothing per key and are validated by the
  only program that knows the schema.
- **Writing the harness's configuration files** (`config.toml`,
  `settings.json`, a generated `CODEX_HOME` or `CLAUDE_CONFIG_DIR`): changes
  files the user owns, races with harnesses that write them (`/model`,
  `/config`), and a separate configuration directory splits authentication and
  session history from the user's normal one.
- **Raw `args` only**: keeps TOML and JSON hand-quoted inside YAML, invites
  Codex's silent string fallback, and gives base and derived definitions no
  merge: two `-c` for one key both appear and the last wins, and two
  `--settings` JSON strings cannot be combined at all.
- **Validating settings in agx** against vendored schemas: goes stale with
  each harness release and rejects keys the installed version accepts.
- **A Pi overlay agent directory** via `PI_CODING_AGENT_DIR`: see [Pi](#pi).

## Effort

About two and a half days:

1. `settings` in the harness schema, chain merge with `null` removal, fragment
   conflicts, tag rules, and the reserved-path and `--settings` checks: 0.5
   days.
2. Codex: a general YAML-to-TOML encoder (today's `TOMLAssignment` handles only
   strings and string maps), flattening, the dotted-key fallback, and argument
   order, in launches and `app-server`: 0.5 days.
3. Claude Code: the per-launch settings file and its cleanup: 0.5 days.
4. `harness list --json`, `explain`, `doctor`, the JSON schema, docs (replacing
   the `-c 'model_provider="cpa"'` examples), and tests: 1 day.

## Open questions

- Whether Claude Code accepts several `--settings` (merging them or keeping the
  last); if it merges, agx could let a typed `--settings` layer over the
  definition's instead of rejecting it in `args`. Needs a spike against 2.1.300.
- How to check Claude settings in `agx doctor`: Claude Code ignores invalid
  settings files in some modes rather than failing.
- Whether derived definitions should be able to append to inherited lists
  (`permissions.allow`), for example with a `+` key suffix, instead of
  replacing them.
- Whether settings should apply to some passthrough commands, such as `codex
  login` or `codex doctor` for a definition whose provider exists only in its
  settings.
- Pi: propose an upstream `--settings <file-or-json>` flag (the merge already
  exists as `SettingsManager.applyOverrides`), or map a fixed subset in agx
  (`defaultProvider`, `defaultModel`, `defaultThinkingLevel`, `enabledModels`,
  `defaultTools` to flags, and `models.json`-shaped `providers` to
  `registerProvider` in the generated extension, filling the fields it
  requires). The first keeps "native schema, harness validates"; the second
  makes agx own part of Pi's schema.
- Whether OpenCode definitions ([ADR 0023](0023-opencode-plugin.md)) get
  settings through `OPENCODE_CONFIG_CONTENT` under `--standalone`.
