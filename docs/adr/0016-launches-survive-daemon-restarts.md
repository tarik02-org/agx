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

A state file of a newer format version, left by a downgrade, keeps the daemon
from starting with a hint to upgrade agx: replacing it would lose the launches
it holds. Writes are atomic, so a file that does not decode comes from an edit
by hand; the daemon moves it aside to `daemon-state.json.bad-<UTC time>`, starts
without restoring, and keeps every launch file, since the launches of the
moved file may still need them.

A restored launch keeps its definitions: when its directory no longer resolves
to the saved snapshot ID, its MCP servers answer 403 until relaunch. Upstream
sessions are not restored; endpoints answer 404 for unknown session IDs, and
clients must start a new session. If the saved port is taken, endpoint URLs held
by harnesses are lost until relaunch or, for Codex app-server threads,
`thread/resume`.

`daemon stop --force` still revokes everything and clears the file. A command
finding a daemon of another build that advertises `restores_launches` in its
discovery replaces it even with active launches; older builds still refuse.
Discovery also carries a `format` version; a build finding a newer one neither
uses nor replaces that daemon and asks for an agx upgrade. `version` keeps
naming the agx release, which builds without `format` compare.

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

## Addendum: launches survive a daemon that dies

A daemon killed before it saved its state, for example with the job object of the editor or
harness whose agx command started it, left its launches nowhere: the next daemon restored an
older state or none, and wrappers gave up once their renewal found the launch unknown. Now:

- the daemon starts outside its caller's Windows job object, and in a session of its own on Unix;
- a wrapper whose launch the daemon does not hold, or let expire, re-attaches it with
  `RestoreLaunch`, sending the launch as `PrepareLaunch` returned it and its grant, so the
  harness's MCP configuration stays valid; thread launches are not re-attached;
- a wrapper that finds the discovery file of a daemon that no longer answers, which a stopping
  daemon removes last, starts the next daemon, which binds that discovery's port when its state
  file names none;
- the state file keeps revoked launch IDs for a week, so neither a forced stop nor `launch revoke`
  can be undone by re-attaching;
- wrappers retry for as long as their harness runs instead of until their lease ends.

Re-attaching needs the lifecycle token and the grant, which only the wrapper holds, and management
authorization, which any agx command has; it grants nothing a new launch would not.

