# Launches follow their directory's configuration

Status: accepted. Revises ADR 0031 for resource leases; extends ADR 0016, where an attached launch already gets the MCP servers its directory selects, to running launches.

A launch used to keep the snapshot it started with. A resource consumer that wanted
newer configuration prepared a second lease, reloaded its catalogs from it and
released the first, so every change opened a window with two live leases. Process
launches kept their MCP servers until the harness restarted.

The daemon now keeps every running launch on its directory's current configuration
as far as the launch can take it. It resolves each launch's directory once a
second, and at once after an edit committed through the daemon, and reconciles the
launch's MCP endpoints in place. A server keeps its endpoint while its definition,
share root, secret definitions and inputs are unchanged. Otherwise it restarts
behind the same URL. A deselected server's endpoint stops answering, and selecting
it again brings it back at the same URL. Configuration that cannot be resolved
leaves every launch as it is.

What else follows depends on the launch:

- A resource lease also takes the directory's current content and settings. Its
  integration holds `WatchResources` open, authorized by the lifecycle token, and
  gets the lease's resources first and then on each change, including an invalid
  state while configuration cannot be resolved. It reloads only the catalogs that
  changed and has no reload command. The lease keeps its ID, grant and lifecycle
  token for its whole life, so revocation, finishing and restart attachment only
  ever see one lease.
- A process launch keeps the snapshot its harness loaded: files, arguments,
  settings and its native tool list cannot change under a running process, and its
  view still names that snapshot. A server selected after it started is reachable
  with `agx mcp call` through the launch's grant.

Resource environment inputs follow ADR 0008. The daemon names the variables the
current selection reads, and the integration sends values for those names only. A
watch carries the inputs it opened with. An integration holding a value for a newly
named variable opens a new watch with it, and until then the server reading it fails
alone, as after attaching. Names the configuration stopped reading are dropped
instead of refused.

## Considered options

- Keeping one resource lease per snapshot, with a lifecycle-authorized handover from
  each lease to its successor. Rejected for the window above: two authorizations per
  change, a lease family to carry revocation across them, and a persisted record of
  retired members to keep terminal outcomes terminal.
- Reconciling only on request, through a reload command or a sync call. Rejected:
  the daemon already sees every change, and a request path is a second mechanism
  users must remember.
- Restarting a process launch to apply what it cannot take while running. Deferred:
  it ends the harness's session, so an agent or user should ask for it.
- Rolling back to the previous resources when a catalog reload fails. Rejected: with
  one lease there is nothing to return to. The integration retries the reload with
  the next change instead.
