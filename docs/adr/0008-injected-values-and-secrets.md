# Inject launch-time values with YAML tags; call credential sources secrets

Status: accepted.

Configuration values that must not live in files, or that depend on the caller,
use YAML tags resolved at launch:

```yaml
environment:
  REGION: eu
  API_KEY: !secret cpa-key   # a configured secret
  TOKEN: !env WORK_TOKEN     # the caller's environment
  PROMPT: !file ./prompt.txt # file contents, relative to the declaring fragment
```

Tags are allowed in harness `environment` and `args`, and in `mcp` `environment`
and `headers`. Harness `args` accept only `!file`: command lines are visible to
other processes, so `!secret` and `!env` belong in `environment`. `!env` reads the wrapper's environment, which the wrapper forwards
only for names the snapshot requires. `!file` reads at launch, so edits apply
without reinstalling. Resolved values never enter snapshots, artifacts, `explain`,
`--json` output, or logs; those show the tag and its argument.

The top-level `auth:` section becomes `secrets:`, matching `!secret`. Secret kinds
are unchanged: `environment`, `daemon_environment`, `command`, `keychain`, `oauth`.
An HTTP MCP server's `auth` field stays: OAuth bearer tokens need login and
refresh, not just a value. [ADR 0009](0009-inline-mcp-auth-and-interactive-login.md)
later replaced its bare secret ID with inline secrets and `auth: !secret <id>`.
The `agx auth` command group becomes `agx secret` (`login`, `set`, `status`,
`logout`).

The daemon resolves values for managed launches: harness values while preparing
the launch, stdio MCP server environments when starting an upstream, and HTTP
headers per request. Passthrough invocations run without the daemon, so the
wrapper resolves their harness values itself; `environment`, `command`, and
`keychain` secrets work there, and `daemon_environment` secrets fail. A daemon RPC
returning secret values was rejected: it would let any management client read
every secret. OAuth secrets cannot be injected.

`auth_environment` is removed; `environment: {NAME: !secret id}` replaces it.
Nothing has shipped, so there is no compatibility alias.

Later, MCP `auth` and a sops secret's `age_key` came to accept any value, so the
`environment` and `file` kinds duplicated `!env` and `!file` and were removed,
with `daemon_environment`: the daemon's environment is an accident of how it was
started. The kinds left are `command`, `keychain`, `oauth`, and `sops`.

Rejected: an `{auth: id}` mapping form, which is noisier and cannot appear inside
argument lists; a general templating language, which invites logic in configuration.
