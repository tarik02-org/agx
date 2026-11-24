# Resource leases follow their directory's configuration

Status: accepted. Revises ADR 0031 for resource leases.

A resource lease used to stand for one snapshot. An integration that wanted newer
configuration prepared a second lease, reloaded its catalogs from it and released
the first, after the user asked it to reload. Every change then opened a window
with two live leases. Revoking or finishing one had to reach the other, and a
failed reload needed the old one back.

A resource lease now stands for a consumer of a directory and harness. Its ID,
grant and lifecycle token stay the same for its whole life. While its integration
holds `WatchResources` open, authorized by the lifecycle token, the daemon keeps
the lease on the directory's current configuration. It resolves the directory
once a second, and at once after an edit committed through the daemon. Each pass
reconciles the lease's MCP endpoints in place. A server keeps its endpoint while
its definition, share root, inputs and secret definitions are unchanged. Otherwise
it restarts behind the same URL, and a deselected server's endpoint closes.

The watch first sends what the lease provides, then sends again whenever that
changes: the snapshot, content, endpoint list, settings and an MCP revision that
changes when any endpoint starts or stops. Unrelated configuration edits send
nothing. The integration reloads only the catalogs that changed and has no reload
command. Configuration that cannot be resolved sends an invalid state and leaves
the lease as it was. Revocation, finishing and restart attachment need no change,
since they only ever see one lease.

Resource environment inputs follow the same rule as ADR 0008. The daemon names the
variables the current selection reads, and the integration sends values for those
names only. A watch carries the inputs it opened with. An integration holding a
value for a newly named variable opens a new watch with it, and until then the
server reading it fails alone, as after attaching. Names the configuration stopped
reading are dropped instead of refused.

Process launches keep their snapshot. A running harness cannot take new native
arguments, files or settings, so the configuration it started with stays its
configuration.

## Considered options

- Keeping one lease per snapshot, with a lifecycle-authorized handover from each
  lease to its successor. Rejected for the window above: two authorizations per
  change, a lease family to carry revocation across them, and a persisted record
  of retired members to keep terminal outcomes terminal.
- A separate call that syncs a lease, made on a reload command or a watch event.
  Rejected: the watch already knows when to sync, and a second path adds a command
  users must remember.
- Reconciling every lease whenever the daemon reloads configuration, watched or
  not. Rejected: an integration that is not watching would have tools change under
  a running session without reloading its catalogs.
- Rolling back to the previous resources when a catalog reload fails. Rejected:
  with one lease there is nothing to return to. The integration retries the reload
  with the next event instead.
