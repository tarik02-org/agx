# Provision configuration from a service

Status: proposed.

Platforms that run harnesses for users, such as hosted sandboxes or a company
control plane, often hold configuration in a database rather than in files under
`XDG_CONFIG_HOME`, and want to answer per project and harness, with short-lived
secrets. agx can hand part of resolution to such a service. It works with the
daemon and with the daemonless runtime of [ADR 0020](0020-daemonless-runtime.md).

## Provisioning

A provisioning endpoint answers "what does this harness get in this directory"
on behalf of a platform. agx calls it during resolution:

```yaml
provision:
  url: https://platform.example/agx/v1/resolve
  auth: !secret platform-token          # bearer; or mTLS via client_cert/key
  cache: 5m                             # reuse an answer for this long
  mode: merge                           # merge | replace
```

Request (POST, JSON): the canonical directory, harness and built-in, mode, the
repository's remote and commit when inside Git, agx version, and a host-supplied
`context` map (`--provision-context k=v`, or `AGX_PROVISION_CONTEXT`). Response: a
configuration fragment in the normal schema (skills, instructions, MCP servers,
secrets, scopes, harness settings), plus optional `secrets` values scoped to this
launch and `ttl`.

- `merge` adds the fragment after local configuration under the usual override
  rules, so a platform can supply MCP servers and policy while users keep their
  own; `replace` uses only the provisioned fragment, for fully managed
  environments.
- Provisioned secrets are injected only into this launch, never stored, and
  shown as `!secret` references in `explain`.
- The fragment is validated like any other; an invalid or unreachable response
  fails the launch with the endpoint's error, unless `required: false`, in which
  case agx warns and continues with local configuration.
- Provisioning works in both runtimes: In daemon mode the daemon calls the
  endpoint per launch and per Codex/ACP session, honouring `cache`.

The request and response schemas are published next to the configuration
schema, with an example server in `examples/`.

Rejected: pushing configuration only as files, which loses per-launch answers and
short-lived secrets; a provisioning protocol separate from the configuration
schema, which would need its own validation and documentation.
