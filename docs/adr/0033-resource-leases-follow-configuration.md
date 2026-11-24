# Resource leases follow their directory's configuration

Status: accepted. Revises ADR 0031 for resource leases.

A resource lease used to stand for one snapshot. An integration that wanted newer
configuration prepared a second lease, reloaded its catalogs from it and released
the first. Every change then opened a window with two live leases. Revoking or
finishing one had to reach the other, and a failed reload needed the old one back.
Keeping both safe took a lease family shared across generations, an explicit
handover call and a persisted record of released members.

A resource lease now stands for a consumer of a directory and harness. Its ID,
grant and lifecycle token stay the same for its whole life. `SyncResources`,
authorized by the lifecycle token, resolves the directory again and reconciles the
lease's MCP endpoints in place. A server keeps its endpoint while its definition,
share root, inputs and secret definitions are unchanged. Otherwise it restarts
behind the same URL, and a deselected server's endpoint closes. The response holds
the current snapshot, content, endpoint list and an MCP revision that changes when
any endpoint starts or stops. The integration reloads only the catalogs that
changed. Revocation, finishing and restart attachment need no change, since they
only ever see one lease.

Resource environment inputs follow the same rule as ADR 0008. The daemon names the
variables the current selection reads, and the integration sends values for those
names only. Names the configuration stopped reading are dropped instead of refused,
since the integration cannot know what changed before it asks. A server reading an
input the integration has not sent fails alone until the next sync supplies it, as
after attaching.

Process launches keep their snapshot. A running harness cannot take new native
arguments, files or settings, so the configuration it started with stays its
configuration.

## Considered options

- Keeping one lease per snapshot and a lifecycle-authorized handover between them.
  Rejected for the window above: two authorizations per change and a third record
  of ended members to keep terminal outcomes terminal.
- Reconciling endpoints whenever the daemon reloads configuration. Rejected:
  integrations reload their catalogs only when told, so a change they have not
  synced would alter tools underneath a running session without the integration
  knowing.
- Rolling back to the previous resources when a catalog reload fails. Rejected:
  with one lease there is nothing to return to. The integration retries the reload
  on its next sync instead.
