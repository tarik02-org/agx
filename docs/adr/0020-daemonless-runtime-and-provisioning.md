# Run without a daemon; provision configuration from a service

Status: proposed.

The daemon suits a workstation: one long-lived owner of MCP sessions, grants,
OAuth, and state. Other environments don't want one: CI jobs, containers,
sandboxes, and platforms that start a harness for a task and discard it. They
also tend to hold configuration elsewhere, in a database or a control plane,
rather than in files under `XDG_CONFIG_HOME`. agx gains a daemonless runtime and
a provisioning source for configuration.

## Daemonless runtime

`agx exec` runs one harness with everything in its own process:

```sh
agx exec --harness codex --dir /work [--config <file|->] [--provision <url>] -- <native args>
```

A configuration may also opt in for every launch with `daemon.runtime: local`.

| Concern | Daemon | Daemonless |
| --- | --- | --- |
| Resolution, skills, instructions, tags | daemon | the wrapper, same code |
| HTTP MCP servers | daemon proxy with launch grants | the same proxy in-process, on an ephemeral loopback port, with a grant only this harness holds |
| stdio MCP servers | daemon children | wrapper children, bridged by the same proxy |
| Codex app-server / ACP sessions | per-thread launches in the daemon | the same relays in-process, issuing per-thread grants locally |
| OAuth login | daemon callback | not interactive: tokens come from the secret provider; `agx secret login --local` can run a one-shot callback listener when a person is present |
| Persistence, `launch list`, revoke | yes | not needed: everything ends with the process |

The MCP runtime and launch bookkeeping move behind one interface with two
implementations, RPC to the daemon and in-process, so both modes share
endpoints, relays, and grant checks. Daemonless runs write nothing to the
user's state or cache unless `--cache` points somewhere; package and harness
sources must then be pinned (`rev`, `hash`, store paths) or pre-installed.

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
- Provisioning works in both runtimes. In daemon mode the daemon calls the
  endpoint per launch and per Codex/ACP session, honouring `cache`.

The request and response schemas are published next to the configuration
schema, with an example server in `examples/`.

## Secret providers

Daemonless and provisioned runs need secrets without a keychain or a person. A
`provider` secret kind delegates to the host: `{kind: provider, url|command}`
returning the value for an ID, alongside the existing `environment`,
`command`, `sops`, and provisioned values.

Rejected: a mandatory daemon, which excludes ephemeral environments; a separate
"managed agx" binary, which would fork resolution; pushing configuration only as
files, which loses per-launch answers and short-lived secrets.
