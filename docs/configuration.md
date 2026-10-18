# Configuration reference

Configuration fragments and package manifests are unversioned YAML. Unknown
fields, duplicate YAML keys, import cycles, bundle cycles, missing resources, and
conflicting resource definitions fail validation. An empty file is an error;
write `{}` for a fragment that declares nothing. A replacement definition needs
`override: true`; root-fragment definitions with `override` take precedence over
package definitions, and packages never replace root definitions. Harnesses and
`daemon` settings merge field by field; `override: true` replaces the whole
definition or section.
Imports run in their declared order before the importing file. An optional
import is skipped only when its own file is missing; errors inside it fail. A
`glob` import loads every matching regular file in lexical order and may match
nothing; name drop-ins `10-team.yaml`, `20-local.yaml` to order them. Each import
sets exactly one of `path` or `glob`, and `optional` applies only to `path`.

When the root file does not exist, agx uses a built-in default that imports
`agx.d/*.yaml` beside it, so drop-in fragments work without a root file:

```yaml
imports:
  - glob: agx.d/*.yaml
```

```yaml
imports:
  - path: personal.yaml
  - path: local.yaml
    optional: true
  - glob: agx.d/*.yaml
lockfile: agx.lock.json
packages:
  team:
    git: https://example.com/team/agent-resources.git
    ref: main
    subdir: resources
  local:
    path: ./local-resources
skills:
  workflow:
    path: ./skills/workflow
instructions:
  conventions:
    path: ./conventions.md
secrets:
  work-token:
    kind: environment
    variable: WORK_TOKEN
mcp:
  work:
    url: https://mcp.example.com/mcp
    auth: !secret work-token
  local-tools:
    command: [~/.local/bin/local-tools, --stdio]
    environment:
      LOG_LEVEL: warning
      API_TOKEN: !secret work-token
bundles:
  development:
    skills: [workflow, team/review]
    instructions: [conventions]
    mcp: [work, local-tools]
scopes:
  - add:                                # no directory: everywhere
      instructions: [conventions]
  - directory: ~/work
    add:
      bundles: [development]
      skills:
        - ./skills/local-thing            # path; ID = folder name
        - id: deploy                      # generated from content
          description: Deploy this service.
          content: |
            Run `make deploy` and report the release URL.
        - git: https://github.com/team/skills.git   # inline Git source
          rev: 3f2c9e1a8b7d6c5e4f3a2b1c0d9e8f7a6b5c4d3e
          path: skills/review-strict      # ID = folder name
      instructions:
        - ./AGENTS.md                     # ID = file stem
        - id: pnpm
          text: Use pnpm, not npm.
      mcp:
        - id: aperture
          url: https://aperture.tarik02.me/mcp
  - directory: ~/work/private
    remove:
      skills: [team/review]
  - directory: ~/work
    harnesses: [claude]
    add:
      mcp: [work]
harnesses:
  codex:
    executable: ~/.local/bin/codex
```

Relative paths are relative to their declaring file, including scope roots,
imports, local package sources, resource files, and copied skill files. A
leading `~` or `~/` expands to the home directory of the user running agx in
scope directories, imports, `lockfile`, package and resource paths, `!file`,
harness executables, and an MCP server's `cwd` and executable (`command[0]`);
`~user` and `${VAR}` are not supported, and package manifests cannot use `~`.
Machine-specific settings belong in an ignored fragment such as
`agx.d/50-local.yaml`. Bare command names use the daemon's PATH; use absolute or
`~/` command and script paths for portable definitions.
An MCP entry with `url` is reached over HTTP; one with `command` runs as a stdio
child. `transport: http` or `stdio` may still be written but must agree, and an
entry sets exactly one of `url` and `command`. A stdio MCP server's optional `cwd`
is fragment-relative; when omitted it uses the launch directory. Its environment
contains a small OS baseline plus its declared `environment`, rather than the
daemon's whole environment. Literals belong in non-secret settings only; inject
credentials with [tags](#injected-values). HTTP MCP servers authenticate with
[`auth`](#mcp-auth) and may set extra `headers`; `Authorization`,
`Proxy-Authorization`, `Cookie`, `Host`, and `Mcp-Session-Id` are managed by agx. Harnesses see each
entry as a `agx-<id>` MCP server (`team/github` becomes `agx-team-github`);
long or colliding names get a short hash suffix.

Rules match canonical directory roots and descendants by path components. A rule
without `directory` applies everywhere and runs before every directory rule,
wherever it is written, so a deeper rule can still remove what it adds; a
filesystem root is not used for this because Windows has one per drive. Rules
run from shallowest to deepest, preserving import/rule order at equal depth.
Within each rule, additions precede removals. Bundle removals remove all
expanded leaves, even if another bundle also added them. A deeper rule can add
those leaves again. A rule with `harnesses` applies only to launches of those
harnesses or definitions based on them. `explain` includes each operation and its
source location.

## Selection lists

Every selection list (scope `add`/`remove`, bundles, harness `add`/`remove`)
accepts references and inline definitions:

- A string starting with `.`, `/`, or `~`, or an absolute Windows path, is a
  path: a skill directory, whose ID is its folder name, or an instructions file,
  whose ID is its file stem. MCP servers have no path form.
- Any other string is an ID reference, qualified like other references inside
  packages. IDs cannot start with `.`, `/`, or `~`, so the forms never overlap.
- A mapping is an inline definition with the fields of its top-level section
  (`override` excluded) plus `id`. Path and Git skills default `id` to the folder
  name, instruction files to their stem; skills with `content`, instructions with
  `text`, and MCP servers need an explicit `id`, which logins and native server
  names use.

Bundle lists take bundle IDs only; an inline bundle would just be its own
selection. Inline definitions share the ID space of top-level ones (`team/deploy`
inside package `team`), and an inline ID that collides with any other definition
is an error naming both locations, even with `override`, so a reference never
silently changes meaning. Define a resource once, inline or at top level, and
refer to it by ID elsewhere, including in `remove` lists: repeating a path there
defines it a second time.

## Skills and instructions

A skill is a directory with `SKILL.md` (`path`), a directory in a Git repository
(`git`, with `path` inside the checkout), or generated from configuration
(`content`). Instructions are a file (`path`), a file in a Git repository (`git`
with `path`), or `text`. A Git source is an anonymous package: it is fetched by
`agx install` and locked like a [package](#packages-and-locks) unless
[pinned](#pinned-sources), takes `ref`, `rev`, or `hash` like one, and its
`path` must stay inside the checkout. Configuration fragments may name local
repositories; package manifests need `https`, `ssh`, or scp-style sources.

A skill with `content` is a whole skill directory described in YAML:

```yaml
skills:
  deploy:
    name: deploy-service          # native skill name; default: last ID segment
    description: Deploy this service and report the release URL.
    frontmatter:                  # extra SKILL.md front matter, passed through
      allowed-tools: [Bash]
    content: |                    # SKILL.md body
      Run `scripts/deploy.sh`, then follow `references/checklist.md`.
    files:                        # other files, relative to the skill root
      scripts/deploy.sh:
        executable: true
        content: |
          #!/bin/sh
          set -eu
          make deploy
      references/checklist.md: ./docs/release-checklist.md   # copied file
      assets/: ./deploy-assets/   # copied directory (trailing slash)
```

The ID addresses the skill in configuration; `name` is what harnesses show and
invoke, defaulting to the ID's last segment, and must use lowercase letters,
digits, and single hyphens, at most 64 characters. `description` is required.
Generated `SKILL.md` front matter holds `name`, `description`, and
`frontmatter`, which cannot redefine those two. `files` keys are `/`-separated
relative paths that stay inside the skill root and cannot name `SKILL.md`, `.`,
`..`, or `.git`, or overlap. A value is inline text, a `{content, executable}`
mapping, or a one-line string starting with `.`, `/`, or `~`, which copies that
file, or that directory when both the key and the path end in `/`; write
`{content: ...}` for text that would read as a path. Copied paths resolve like
resource paths and keep their executable bits. Generated skills are hashed into
the snapshot like path skills, so editing a copied file or the configuration
selects new artifacts. Path and Git skills take `name` and `description` from
their own `SKILL.md`; setting them, `frontmatter`, or `files` on such a skill is
an error.

Selected instructions concatenate in resolution order: shallower scopes before
deeper ones, harness rules last, and list order within a rule, so more specific
guidance comes later. An optional integer `order` (default `0`) sorts before
that, lower first, for text that must lead or close regardless of where it was
selected; `explain` shows non-default orders.

## Editor support

JSON Schemas for configuration fragments and package manifests are published in
[`schema/`](../schema); `agx config schema [--package]` prints the ones matching
the installed version. Editors using yaml-language-server pick a schema from a
modeline at the top of the file:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/tarik02-org/agx/master/schema/agx.schema.json
scopes: []
```

Use `agx.package.schema.json` for `agx.package.yaml`. Declare the injected-value
tags so the language server accepts them; it then validates each as a string:

```json
"yaml.customTags": ["!env scalar", "!file scalar", "!secret scalar"]
```

The schema checks structure only; `agx config validate` also checks references,
paths, and conflicts across fragments.

## Harnesses

`codex`, `claude`, and `pi` are built in; configuring one customizes it. Other
names define a harness that extends a `base`, a built-in or another definition,
and become `agx <name>` commands; [shims](cli.md#shims) also put them on PATH.
Names cannot shadow agx commands, and base chains cannot form cycles.

```yaml
harnesses:
  codex:
    add: {skills: [workflow]}
  codex-cpa:
    base: codex
    executable: /opt/codex-cpa/bin/codex
    args:
      - [-c, 'model_provider="cpa"']
      - values: [--profile, cpa]
        modes: [interactive, exec, resume]
    env:
      CPA_REGION: eu
      CPA_KEY: !secret cpa-key
    remove: {skills: [workflow]}
  pi-cpa:
    base: pi
    args: [--model, cpa/gpt-6-sol]   # flat: one group for every session mode
```

Each launch is classified into one mode of its built-in:

| Harness | Modes |
| --- | --- |
| codex | `interactive`, `exec`, `resume`, `app-server`, `passthrough` |
| claude | `interactive`, `print`, `resume`, `passthrough` |
| pi | `interactive`, `print`, `rpc`, `resume`, `passthrough` |

`args` is a flat list of strings, one group for every session mode, or a list of
groups, each a list of strings or `{values, modes}`; mixing strings and groups
is an error. Groups whose `modes` include the launch mode are inserted, in order,
after agx's own arguments and before the native ones. A flat list, a plain group,
or a group without `modes` applies to every mode except `passthrough`. `env` values apply
in every mode; values in `env` and in argument groups may be
[tags](#injected-values); names starting with `AGX_` are reserved. An extension
replaces `executable`, appends its groups after its base's, and replaces `env`
per key. `executable` must be absolute once `~` is expanded.

Selection gains a harness dimension: after directory scopes, each harness in the
base chain applies its `add` and then `remove`, built-in first, so a harness's
selection holds in every directory. Across fragments, `base`, `executable`, and
each `env` key may be set once; groups append in fragment order and `add` and
`remove` accumulate.

## Injected values

Values that must not live in files, or that depend on the caller, use YAML tags
resolved at launch:

```yaml
harnesses:
  codex-cpa:
    base: codex
    args:
      - [--instructions, !file ./prompt.txt]
    env:
      REGION: eu
      API_KEY: !secret cpa-key   # a configured secret
      TOKEN: !env WORK_TOKEN     # the caller's environment
mcp:
  docs:
    url: https://docs.example.com/mcp
    headers:
      X-Api-Key: !secret docs-key
```

- `!env NAME` reads the caller's environment. The wrapper forwards only names the
  snapshot requires; `AGX_` names are reserved. An empty variable counts as unset.
- `!file PATH` reads a file relative to the declaring fragment, `~/` expanded, or
  to the package root inside a package, where it must stay within the package. Trailing newlines
  are dropped, as in shell command substitution; files are limited to 1 MiB.
- `!secret ID` resolves a top-level [secret](#secrets). IDs are qualified like
  other references inside packages. OAuth secrets cannot be injected; they only
  authenticate an HTTP MCP server through `auth`. An MCP server's inline secret
  belongs to it and cannot be referenced.

Tags are allowed in harness `env` and `args`, and in `mcp` `environment`,
`headers`, and `auth` (`!secret` only), including inline MCP servers; anywhere
else, and unknown tags, fail validation. Plain strings
stay literal. Resolution happens at launch, never earlier: snapshot IDs include
the tag and its argument, so editing a `!file` or rotating a secret needs no
reinstall. Resolved values never enter snapshots, artifacts, `explain`, `--json`
output, runtime views, logs, or error messages; `explain` shows each tag as
configured, such as `!secret cpa-key`, with its source location.

For a managed launch, the daemon resolves harness values while preparing it and
stdio MCP server environments when it starts an upstream. HTTP headers resolve
for each request, like the bearer token. A caller variable missing at launch
fails with the variable and the configuration location. Variables that only
MCP servers need are removed from the harness environment.

Passthrough invocations (help, version, native login) run without the daemon, so
the wrapper resolves their harness values itself from configuration fragments:
`environment`, `command`, and `keychain` secrets work, while
`daemon_environment` secrets fail with an error.

## Packages and locks

A package root contains `agx.package.yaml`:

```yaml
skills:
  review:
    path: skills/review
instructions:
  standards:
    path: standards.md
bundles:
  defaults:
    skills: [review]
    instructions: [standards]
packages:
  shared:
    path: ./shared
```

A package root without `agx.package.yaml` declares each directory holding
`SKILL.md` as a skill named after its folder (a `SKILL.md` at the root takes its
`name`), skipping hidden directories, `node_modules`, and directories inside
other skills, so a plain skills repository such as the one `agx skill add`
records needs no manifest.

Package manifests may declare resources, MCP servers, secrets, bundles, and
dependencies, including inline definitions in bundles. They cannot declare directory scopes, imports, daemon settings,
harnesses, or lockfile destinations. Their Git sources must be
`https`, `ssh`, or scp-style URLs. Package references are relative
to their namespace: `shared/review` inside package `team` names
`team/shared/review`. Root fragments use full qualified IDs. Dependencies share
the declaring root package's lockfile. Resource paths and local dependencies must
stay within the installed package. Internal symlinks are materialized; escaping
or cyclic symlinks and special files are rejected.

Each fragment defaults to `agx.lock.json` beside itself. Set `lockfile` in a
machine-local fragment to keep its lock separate from tracked configuration.
Lock sources are stored relative to the lockfile (dependencies relative to their
package root), so committed locks work on other machines and checkouts. Content
hashes ignore executable bits, and Git checkouts ignore user Git configuration.
`install` snapshots local packages and pins Git sources; a matching Git cache
needs no fetch. `update` explicitly advances Git revisions. Frozen installation
requires matching sources, local content, dependencies, and locks. Resolution and
launching never fetch or update packages. Installation writes cache entries before
replacing affected lockfiles together; failed installs leave the previous locks
usable. Entries for removed packages are pruned. Old package and artifact entries are retained for active launches.

Anonymous packages from Git skills and instructions lock under the resource's
kind and ID, such as `skill:review-strict` or `instructions:team/rules`, in the
declaring fragment's lockfile, or with their package's dependencies inside a
manifest; `agx update skill:review-strict` advances one.

### Pinned sources

A source is pinned when it carries a full 40-character commit `rev`, a `hash`,
or a local `path` inside `/nix/store` (literally; symlinks into the store, such
as profiles, can be repointed). Packages and Git skills and instructions accept
these fields; `rev` needs `git` and excludes `ref`. `hash` is the content hash
agx records in locks, verified at install, which fails on a mismatch and names
the actual hash. Pinned sources write no lock entry: `agx install` fetches them
into the cache, and resolution finds them there, so a fragment whose packages
are all pinned, such as one generated by Nix, needs no lockfile and none is
created; an existing one is pruned of their entries. Unpinned sources keep
per-fragment lockfiles, so fragments from different repositories carry their
own locks. `update` leaves pinned sources unchanged.

## MCP auth

An HTTP MCP server's `auth` is the credential agx sends upstream as a bearer
token. It takes one of these forms:

```yaml
mcp:
  aperture:
    url: https://aperture.tarik02.me/mcp   # auth omitted: OAuth on demand
  github:
    url: https://api.githubcopilot.com/mcp
    auth: {kind: oauth, scopes: [repo]}    # inline secret with options
  internal:
    url: https://internal.example/mcp
    auth: keychain                         # kind shorthand
  shared:
    url: https://shared.example/mcp
    auth: !secret team-token               # shared top-level secret
  public:
    url: https://public.example/mcp
    auth: none
```

- A kind (`oauth` or `keychain`) is shorthand for an inline secret of that kind
  with no options.
- A mapping is an inline secret with the fields of a top-level
  [secret](#secrets), `kind` included; `override` belongs to the MCP entry.
- `!secret ID` references a top-level secret, for credentials several entries or
  tags share. A bare ID is read as a kind and fails validation.
- `none` sends no credential.
- Omitted, an HTTP entry uses OAuth on demand; stdio entries have no `auth`.

An inline secret belongs to its entry and takes the entry's ID: `agx secret
login aperture`, `agx secret set internal --stdin`. Inline and top-level secrets
share one ID space for `agx secret`, so an entry with inline or on-demand auth
cannot share its ID with a top-level secret; set `auth: !secret <id>` or `auth:
none`, or rename one. Inside packages, inline secrets take the qualified entry ID
(`team/github`). Stored credentials of an inline secret are kept apart from those
of a top-level secret, and writing `auth: oauth` for an on-demand entry keeps
its login.

OAuth on demand follows the MCP authorization specification. Requests carry no
credential until the server answers `401` and advertises protected resource
metadata, through the challenge's `resource_metadata` or the well-known location;
the server then shows `needs-auth` in `agx launch show` with the diagnostic
`run agx secret login <id>`, and the harness gets that error instead of the
upstream challenge. After the login, requests carry the token. A `401` without
that metadata, or with `auth: none`, stays an ordinary upstream failure, and a
server that never answers `401` is used without a credential.
`agx secret status` lists on-demand OAuth once its server has asked for a login
or a login exists.

## Secrets

`secrets` defines credentials shared by ID: through `auth: !secret` or a
`!secret` tag.

```yaml
secrets:
  work-token: {kind: environment, variable: WORK_TOKEN}
  cpa-key: {kind: command, command: [pass, show, cpa]}
  docs-key: {kind: keychain}
  team-login: {kind: oauth, scopes: [read]}
```

- `environment`: requires `variable`; the wrapper forwards only selected names.
- `daemon_environment`: reads `variable` from the daemon environment.
- `command`: executes a command array with a bounded lifetime and an OS environment
  baseline; its trimmed stdout is the credential.
- `keychain`: `agx secret set <id> --stdin` stores a secret in the OS keychain.
- `oauth`: uses discovery, optional public-client registration, PKCE, and the OS
  keychain. Optional fields: `issuer`, `client_id`, `resource`, and `scopes`.

OAuth supports public clients using authorization code and refresh tokens.
`agx secret login <id>` opens the authorization URL in the browser and waits
until the login finishes; the daemon receives the loopback callback. A shared
OAuth secret logs in through the first MCP server using it, by ID; servers sharing
one must have the same origin, since a token is bound to it. A configured public
`client_id` is useful when the issuer does not support registration. Command and
environment secrets provide headless alternatives. OAuth secrets authenticate
only HTTP MCP servers through `auth`; use another kind for credentials injected
into a stdio child or header. There is no plaintext fallback when keychain storage
is unavailable. Login and logout close sessions of affected MCP servers; one that
cannot resolve a secret, or whose upstream rejects it, reports `needs-auth` with
the reason in its runtime diagnostics.

## Runtime settings

`daemon.lease_seconds` defaults to 60 and must be at least 3.
`daemon.renew_seconds` defaults to 20 and must be positive and shorter than the
lease. Wrappers follow the renewal interval granted by the daemon.
`daemon.autostart` defaults to true: commands that need the daemon start it when
it is not running ([daemon and launches](cli.md#daemon-and-launches)).
`AGX_NO_AUTOSTART=1` turns it off regardless. Discovery and logs live
under the cache's `runtime` directory, protected for the current user.

`AGX_CONFIG`, `AGX_CACHE`, and `AGX_STATE` select absolute paths. State, which holds
launch records, defaults to `<cache>/state` when the cache is not the default one, so
an isolated cache isolates everything. Management commands also
accept `--config` and `--cache` before the command. Native harness arguments pass
through their dedicated entry points; put agx flags before the harness name.
