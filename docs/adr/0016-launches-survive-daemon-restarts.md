# Launches survive daemon restarts

Status: accepted. Replaces "a daemon restart invalidates active grants" from the original
launch contract, and the saved launches this ADR first decided.

Launches lived only in daemon memory, so stopping, upgrading, or losing the daemon cut every
running harness off its MCP servers, and an upgraded agx could not replace a busy daemon.

## Decision

The wrapper owns recovery. It keeps everything a daemon needs to serve its launch: the launch
as `PrepareLaunch` returned it, its grant and lifecycle token, its environment inputs, native
arguments, and executable, and, for a process serving threads, each active thread launch with
its grant and inputs. Whenever the daemon does not hold the launch (a renewal answers
`NotFound`, or that the lease expired), the wrapper sends all of it with `AttachLaunch`, under
management authorization, and renews again. The daemon builds the launch as preparing would,
under the same ID, grant, and lifecycle token, so the endpoint URLs and grant the harness holds
stay valid. Attaching a launch the daemon holds changes nothing but adding thread launches it
misses, so wrappers may send it whenever in doubt.

Wrappers find a new daemon by rereading discovery when a call fails and every second between
renewals, so a restarted daemon normally gets its launches within a second. Until then, MCP requests for
a launch the daemon does not hold, or let expire, answer 503 with `Retry-After`. A wrapper that
finds the discovery file of a daemon that no longer answers, which a stopping daemon removes
last, starts the next daemon; nothing else would, since harnesses talk to the port directly.
Wrappers retry for as long as their harness runs.

The daemon persists configuration-scoped rows in `<state>/state.db`: its port, which the next
daemon binds again when free (or the port of leftover discovery, for a daemon that was killed),
and the launches revoked within a week. A revoked launch never comes back: `AttachLaunch`
refuses it and its MCP endpoints answer 401. A forced stop revokes every launch; any other stop
or a crash leaves them to their wrappers. Launch files no attached launch claims within a lease
of the daemon starting belong to harnesses that ended meanwhile and are removed.

A launch keeps its definitions: when its directory no longer resolves to its snapshot ID, its
MCP servers answer 403 until relaunch. Upstream MCP sessions do not survive; endpoints answer
404 for unknown session IDs and clients start a new session. If the saved port is taken,
endpoint URLs held by harnesses are lost until relaunch or, for Codex app-server threads,
`thread/resume`.

A stopping daemon drains first: it refuses new launches, attachments, and MCP sessions with a
retryable error marked `Agx-Daemon-Stopping`, lets requests in flight finish for 10 seconds
(`--timeout` on `daemon stop` and `daemon restart`), closes its listener, lets stdio servers
exit (input closed, SIGTERM, kill), and deletes remote sessions; discovery goes last. SIGTERM,
SIGINT, and the Windows console events Go reports as SIGTERM stop it the same way; a second
signal skips the rest. A detached daemon ignores SIGHUP, starts in a session of its own on Unix,
and outside its caller's job object on Windows.

## Rejected

- Saving launches in the daemon's state file with token verifiers, and having wrappers resend
  only what the file lacks: two recovery paths, generation-numbered and throttled writes, and a
  handover between stopping and starting daemons, for nothing the wrapper does not already hold.
  A daemon killed before saving lost its launches anyway.
- Storing environment inputs encrypted under a keychain key: it puts secrets on disk while the
  wrapper that supplied them still runs.
- Authorizing `AttachLaunch` by the lifecycle token alone: the daemon cannot verify a token it
  never saw, so any local process, of any user, could then create launches using the owner's
  MCP credentials. Management authorization proves the caller is the owner; the tokens prove it
  holds the launch.
- Re-initializing upstream sessions behind an unknown `Mcp-Session-Id`: the client's
  capabilities are unknown, so the new session could not negotiate honestly.
- Closing the listener at once and letting `http.Server.Shutdown` drain: callers would see
  connection errors instead of a refusal they can tell apart from a crash.
