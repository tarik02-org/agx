# Run without a daemon

Status: deferred. Not implemented.

The daemon suits a workstation: one long-lived owner of MCP sessions, grants,
OAuth, and state. Other environments don't want one: CI jobs, containers,
sandboxes, and platforms that start a harness for a task and discard it. agx
gains a daemonless runtime; [ADR 0021](0021-configuration-provisioning.md) adds
configuration supplied by such platforms.

## Daemonless runtime

`agx exec` runs one harness with everything in its own process:

```sh
agx exec --harness codex --dir /work [--config <file|->] -- <native args>
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
sources must then be pinned (commit `ref`, `hash`, store paths) or pre-installed.

## Secret providers

Daemonless and provisioned runs need secrets without a keychain or a person. A
`provider` secret kind delegates to the host: `{kind: provider, url|command}`
returning the value for an ID, alongside the existing `environment`,
`command`, `sops`, and provisioned values.

Rejected: a mandatory daemon, which excludes ephemeral environments; a separate
"managed agx" binary, which would fork resolution.
