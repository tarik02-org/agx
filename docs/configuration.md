# Configuration reference

Configuration and package manifests use version `1`. Unknown fields, duplicate
YAML keys, import cycles, bundle cycles, missing resources, and conflicting
resource definitions fail validation. An empty file is an error. A replacement
definition needs `override: true`; root-fragment definitions with `override`
take precedence over package definitions, and packages never replace root
definitions. Harnesses and `daemon` settings merge field by field; `override:
true` replaces the whole definition or section.
Imports run in their declared order before the importing file. An optional
import is skipped only when its own file is missing; errors inside it fail. A
`glob` import loads every matching regular file in lexical order and may match
nothing; name drop-ins `10-team.yaml`, `20-local.yaml` to order them. Each import
sets exactly one of `path` or `glob`, and `optional` applies only to `path`.

When the root file does not exist, agx uses a built-in default that imports
`agx.d/*.yaml` beside it, so drop-in fragments work without a root file:

```yaml
version: 1
imports:
  - glob: agx.d/*.yaml
```

```yaml
version: 1
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
    command: [python3, /absolute/path/server.py]
    environment:
      LOG_LEVEL: warning
      API_TOKEN: !secret work-token
bundles:
  development:
    skills: [workflow, team/review]
    instructions: [conventions]
    mcp: [work, local-tools]
scopes:
  - directory: /home/me/work
    add:
      bundles: [development]
  - directory: /absolute/work/private
    exclude:
      skills: [team/review]
  - directory: /home/me/work
    harnesses: [claude]
    add:
      mcp: [work]
harnesses:
  codex:
    executable: /absolute/path/to/codex
```

Use literal paths, with absolute machine-specific scope directories in local
fragments. Relative paths are relative to their declaring file, including scope
roots, imports, local package sources, and resource files. Bare command names use
the daemon's PATH; use absolute command/script paths for portable definitions.
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

Rules match canonical directory roots and descendants by path components. Rules
run from shallowest to deepest, preserving import/rule order at equal depth.
Within each rule, additions precede exclusions. Bundle exclusions remove all
expanded leaves, even if another bundle also added them. A deeper rule can add
those leaves again. A rule with `harnesses` applies only to launches of those
harnesses or definitions based on them. `explain` includes each operation and its
source location.

## Editor support

JSON Schemas for configuration fragments and package manifests are published in
[`schema/`](../schema); `agx config schema [--package]` prints the ones matching
the installed version. Editors using yaml-language-server pick a schema from a
modeline at the top of the file:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/tarik02/agx/main/schema/agx.schema.json
version: 1
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
and become `agx <name>` commands. Names cannot shadow agx commands, and base
chains cannot form cycles.

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
    exclude: {skills: [workflow]}
```

Each launch is classified into one mode of its built-in:

| Harness | Modes |
| --- | --- |
| codex | `interactive`, `exec`, `resume`, `app-server`, `passthrough` |
| claude | `interactive`, `print`, `resume`, `passthrough` |
| pi | `interactive`, `print`, `rpc`, `passthrough` |

`args` groups whose `modes` include the launch mode are inserted, in order, after
agx's own arguments and before the native ones. A plain list, or a group
without `modes`, applies to every mode except `passthrough`. `env` values apply
in every mode; values in `env` and in argument groups may be
[tags](#injected-values); names starting with `AGX_` are reserved. An extension
replaces `executable`, appends its groups after its base's, and replaces `env`
per key. `executable` must be absolute.

Selection gains a harness dimension: after directory scopes, each harness in the
base chain applies its `add` and then `exclude`, built-in first, so a harness's
selection holds in every directory. Across fragments, `base`, `executable`, and
each `env` key may be set once; groups append in fragment order and `add` and
`exclude` accumulate.

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
- `!file PATH` reads a file relative to the declaring fragment, or to the package
  root inside a package, where it must stay within the package. Trailing newlines
  are dropped, as in shell command substitution; files are limited to 1 MiB.
- `!secret ID` resolves a top-level [secret](#secrets). IDs are qualified like
  other references inside packages. OAuth secrets cannot be injected; they only
  authenticate an HTTP MCP server through `auth`. An MCP server's inline secret
  belongs to it and cannot be referenced.

Tags are allowed in harness `env` and `args`, and in `mcp` `environment`,
`headers`, and `auth` (`!secret` only); anywhere else, and unknown tags, fail
validation. Plain strings
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
version: 1
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

Package manifests may declare resources, MCP servers, secrets, bundles, and
dependencies. They cannot declare directory scopes, imports, daemon settings,
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
lease. Wrappers follow the renewal interval granted by the daemon. Discovery and logs live
under the cache's `runtime` directory, protected for the current user.

`AGX_CONFIG` and `AGX_CACHE` select absolute paths. Management commands also
accept `--config` and `--cache` before the command. Native harness arguments pass
through their dedicated entry points; put agx flags before the harness name.
