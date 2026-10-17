# Configuration reference

Configuration and package manifests use version `1`. Unknown fields, duplicate
YAML keys, import cycles, bundle cycles, missing resources, and conflicting
resource definitions fail validation. An empty file is an error. A replacement
definition needs `override: true`; root-fragment definitions with `override`
take precedence over package definitions, and packages never replace root
definitions. Redefining a harness needs `override: true` too. `daemon` settings
merge field by field; `daemon.override: true` replaces the whole section.
Imports run in their declared order before the importing file. An optional
import is skipped only when its own file is missing; errors inside it fail. A
`glob` import loads every matching regular file in lexical order and may match
nothing; name drop-ins `10-team.yaml`, `20-local.yaml` to order them. Each import
sets exactly one of `path` or `glob`, and `optional` applies only to `path`.

When the root file does not exist, Weft uses a built-in default that imports
`weft.d/*.yaml` beside it, so drop-in fragments work without a root file:

```yaml
version: 1
imports:
  - glob: weft.d/*.yaml
```

```yaml
version: 1
imports:
  - path: personal.yaml
  - path: local.yaml
    optional: true
  - glob: weft.d/*.yaml
lockfile: weft.lock.json
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
auth:
  work-token:
    kind: environment
    variable: WORK_TOKEN
connections:
  work:
    transport: http
    url: https://mcp.example.com/mcp
    auth: work-token
  local-tools:
    transport: stdio
    command: [python3, /absolute/path/server.py]
    environment:
      LOG_LEVEL: warning
    auth_environment:
      API_TOKEN: work-token
bundles:
  development:
    skills: [workflow, team/review]
    instructions: [conventions]
    connections: [work, local-tools]
scopes:
  - directory: /home/me/work
    add:
      bundles: [development]
  - directory: /absolute/work/private
    exclude:
      skills: [team/review]
harnesses:
  codex:
    executable: /absolute/path/to/codex
```

Use literal paths, with absolute machine-specific scope directories in local
fragments. Relative paths are relative to their declaring file, including scope
roots, imports, local package sources, and resource files. Bare command names use
the daemon's PATH; use absolute command/script paths for portable definitions.
A stdio connection's optional `cwd` is fragment-relative; when omitted it uses
the launch directory. Its environment contains a small OS baseline plus declared
`environment` and `auth_environment` entries, rather than the daemon's whole
environment. Use named auth sources for secrets; literals belong in non-secret
settings only. HTTP connections use bearer authorization from their `auth` source. Harnesses see each
connection as a `weft-<id>` MCP server (`team/github` becomes `weft-team-github`);
long or colliding names get a short hash suffix.

Rules match canonical directory roots and descendants by path components. Rules
run from shallowest to deepest, preserving import/rule order at equal depth.
Within each rule, additions precede exclusions. Bundle exclusions remove all
expanded leaves, even if another bundle also added them. A deeper rule can add
those leaves again. `explain` includes each operation and its source location.

## Packages and locks

A package root contains `weft.package.yaml`:

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

Package manifests may declare resources, connections, auth sources, bundles,
and dependencies. They cannot declare directory scopes, imports, daemon settings,
harness executables, or lockfile destinations. Their Git sources must be
`https`, `ssh`, or scp-style URLs. Package references are relative
to their namespace: `shared/review` inside package `team` names
`team/shared/review`. Root fragments use full qualified IDs. Dependencies share
the declaring root package's lockfile. Resource paths and local dependencies must
stay within the installed package. Internal symlinks are materialized; escaping
or cyclic symlinks and special files are rejected.

Each fragment defaults to `weft.lock.json` beside itself. Set `lockfile` in a
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

## Credential sources

- `environment`: requires `variable`; the wrapper forwards only selected names.
- `daemon_environment`: reads `variable` from the daemon environment.
- `command`: executes a command array with a bounded lifetime and an OS environment
  baseline; its trimmed stdout is the credential.
- `keychain`: `auth set <id> --stdin` stores a secret in the OS keychain.
- `oauth`: uses discovery, optional public-client registration, PKCE, and the OS
  keychain. Optional fields: `issuer`, `client_id`, `resource`, and `scopes`.

OAuth supports public clients using authorization code and refresh tokens.
`auth login <connection>` prints an authorization URL; open it in a browser on
this machine. The daemon receives the loopback callback. A configured public
`client_id` is useful when the issuer does not support registration. Credential
helpers and environment sources provide headless alternatives. OAuth sources
currently authenticate HTTP connections; use a helper, environment, or keychain
source for credentials needed in a stdio child. There is no
plaintext fallback when keychain storage is unavailable. Logout closes affected
connections; environment/helper sources stay disabled until daemon restart.

## Runtime settings

`daemon.lease_seconds` defaults to 60 and must be at least 3.
`daemon.renew_seconds` defaults to 20 and must be positive and shorter than the
lease. Wrappers follow the renewal interval granted by the daemon. Discovery and logs live
under the cache's `runtime` directory, protected for the current user.

`WEFT_CONFIG` and `WEFT_CACHE` select absolute paths. Management commands also
accept `--config` and `--cache` before the command. Native harness arguments pass
through their dedicated entry points; put Weft flags before the harness name.
