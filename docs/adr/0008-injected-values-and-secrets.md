# Inject launch-time values with YAML tags; call credential sources secrets

Status: proposed.

Configuration values that must not live in files, or that depend on the caller,
use YAML tags resolved at launch:

```yaml
env:
  REGION: eu
  API_KEY: !secret cpa-key   # a configured secret
  TOKEN: !env WORK_TOKEN     # the caller's environment
  PROMPT: !file ./prompt.txt # file contents, relative to the declaring fragment
```

Tags are allowed in harness `env` and `args`, and in connection `environment`
and `headers`. `!env` reads the wrapper's environment, which the wrapper forwards
only for names the snapshot requires. `!file` reads at launch, so edits apply
without reinstalling. Resolved values never enter snapshots, artifacts, `explain`,
`--json` output, or logs; those show the tag and its argument.

The top-level `auth:` section becomes `secrets:`, matching `!secret`. Secret kinds
are unchanged: `environment`, `daemon_environment`, `command`, `keychain`, `oauth`.
An HTTP connection's `auth: <secret>` field stays: OAuth bearer tokens need login
and refresh, not just a value. The `weft auth` command group becomes `weft secret`
(`login`, `set`, `status`, `logout`).

`auth_environment` is removed; `environment: {NAME: !secret id}` replaces it.
Nothing has shipped, so there is no compatibility alias.

Rejected: an `{auth: id}` mapping form, which is noisier and cannot appear inside
argument lists; a general templating language, which invites logic in configuration.
