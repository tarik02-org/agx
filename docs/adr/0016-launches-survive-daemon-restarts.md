# Launches survive daemon restarts

Status: accepted. Replaces "a daemon restart invalidates active grants" in
[contracts](../development/contracts.md#launch-preparation-and-credentials).

Launches lived only in daemon memory, so stopping or upgrading the daemon cut
every running harness off its MCP servers, and an upgraded agx could not replace
a busy daemon at all.

## Decision

The daemon keeps its active launches in `daemon-state.json` in the state
directory: a versioned JSON file, written atomically with mode 0600 (a private
ACL on Windows) whenever a launch or thread launch starts, ends, or changes state,
and for lease renewals at most every five seconds. It holds the listener port,
the configuration root, and per launch the metadata that rebuilds it (snapshot
ID, directory, harness, mode, parent, session, lease, expiry) and SHA-256
verifiers of its grant and lifecycle token. Tokens, environment inputs, and
native arguments never reach the file; `agx doctor` checks its permissions.

A daemon stopped without `--force`, or by a signal, writes the file and ends only
its MCP sessions, keeping launch files. The next daemon drops expired launches,
those of another root, and launch files nobody owns; binds the saved port when
free; and restores the rest. When a renewal fails, the wrapper rereads discovery
and renews with the daemon it names, of any build, until its lease runs out. The
renewal lists launches waiting for `RestoreLaunch`, with which the wrapper
resends their environment inputs and native arguments under management
authorization; that rebuilds endpoints and, for a process serving threads, its
harness process. Endpoints needing no inputs rebuild on first use.

A restored launch keeps its definitions: when its directory no longer resolves
to the saved snapshot ID, its MCP servers answer 403 until relaunch. Upstream
sessions are not restored; endpoints answer 404 for unknown session IDs, and
clients must start a new session. If the saved port is taken, endpoint URLs held
by harnesses are lost until relaunch or, for Codex app-server threads,
`thread/resume`.

`daemon stop --force` still revokes everything and clears the file. A command
finding a daemon of another build that advertises `restores_launches` in its
discovery replaces it even with active launches; older builds still refuse.

## Addendum: graceful shutdown

Keeping launches across a restart still cut off whatever was in flight: the
daemon killed its MCP servers the moment it was asked to stop. Every stop that
keeps launches, and a forced one briefly, now drains first. The daemon refuses
new launches and MCP sessions with a retryable error marked
`Agx-Daemon-Stopping`, lets requests in flight finish for
`daemon.shutdown_grace_seconds` (default 10, `--timeout` on `daemon stop` and
the new `daemon restart`), writes the state file, closes its listener, and only
then lets stdio servers exit (input closed, SIGTERM, kill) and deletes remote
sessions; discovery goes last. SIGTERM and SIGINT, and the Windows console
events Go reports as SIGTERM, stop it the same way; a second signal skips the
rest. A detached daemon ignores SIGHUP. Wrappers poll discovery and renew with a
successor at once, so restoration does not wait for the next renewal.

Rejected: closing the listener at once and letting `http.Server.Shutdown` drain.
Callers would see connection errors instead of a refusal they can tell apart
from a crash, and the next daemon could not start before every request ended
anyway, as it needs the startup lock.

## Rejected

- Storing environment inputs encrypted under a keychain key: it puts secrets on
  disk and adds a keychain dependency to every restart, while the wrapper that
  supplied them is still running and can resend them.
- Persisting snapshots: they embed the whole configuration, which may hold
  literal credentials, and resolving again with an ID check is enough.
- Re-initializing upstream sessions behind an unknown `Mcp-Session-Id`: the
  client's capabilities are unknown, so the new session could not negotiate
  honestly.
