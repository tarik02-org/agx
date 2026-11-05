# The daemon

The `agx` client starts `agxd`, a background daemon running as you. It:

- runs your local MCP servers and connects to remote ones;
- holds MCP credentials and OAuth tokens, and refreshes them;
- gives each running harness access to exactly the MCP servers selected for it,
  and takes that access away when the harness exits;
- receives OAuth logins in your browser.

You rarely need to think about it. Any command that needs it (launching a
harness, reading or editing managed configuration, browsing sessions, or
using `agx secret …`) starts it in the background if it is not running.
Only daemon processes open SQLite and run migrations. Each configuration has
its own daemon; those daemons can share the database and coordinate through SQLite.
`agx --help`, `--version`, schema output and completion-script generation stay
local. Shell completion asks an existing daemon briefly and never starts one.
`agx config validate --yaml-only` checks declarative overlays locally for builds.
The terminal picker opens immediately while configuration and session history load.
Native harness help, version and login commands ask the daemon to resolve their
configured environment, without creating a managed launch.

## Commands

```sh
agx daemon status        # running? which version, how many launches
agx daemon start         # start it in the background
agx daemon stop          # stop it; running harnesses keep their access afterwards
agx daemon restart       # stop and start again, e.g. after changing its environment
```

`daemon stop` waits up to 10 seconds for MCP calls in progress to finish
(`--timeout 30s` to change that). Running harnesses keep working: the next
daemon, started by `daemon start`, `restart`, or any command that needs it,
picks their launches up again, and the harnesses reconnect on their own. Their
MCP servers start fresh.

`daemon stop --force` instead cuts off every running harness's MCP access. The
harnesses themselves keep running.

Upgrading agx works the same way: the first command of a newer version stops
the older daemon gracefully and starts its own, keeping running launches. Only
a newer agx replaces a daemon. Versions follow semantic version ordering, with
nightly commit counts compared when the base versions are equal. Unknown
versions such as `dev` are unordered. A daemon that is too busy to answer in
time is left running. `agx daemon status` only reports its state.

The client finds `agxd` beside the real `agx` executable, following symlinks and
Windows shim targets. Install or upgrade both binaries together. To use another
location, set `AGX_DAEMON` to the absolute path of `agxd`; there is no PATH search.
Local help, version, schema and YAML-only validation work without the helper.
Harness launches still require a daemon.

## Its environment

The daemon inherits the environment of the command that started it. That
matters for:

- sops keys from `SOPS_AGE_KEY_FILE` and similar variables;
- local MCP servers whose commands are looked up on `PATH`.

After changing any of these in your shell, run `agx daemon restart` from that
shell. Values your configuration reads with `!env` are different: they come from the shell you launch each harness from, every time.

## Starting it yourself

To run the daemon under a service manager such as launchd or systemd, start it
in the foreground:

```sh
agxd
# or: agxd --config /path/to/agx.yaml --cache /path/to/cache
```

and give the manager exclusive ownership in the root `agx.yaml`:

```yaml
daemon:
  managed: true
```

In this mode CLI commands, harness wrappers, and native plugin recovery never
spawn a daemon, including after crashes or when a newer client is installed.
`agx daemon start` only checks an existing daemon; if none is ready, it tells you
to start it through the manager. `agx daemon restart` asks the daemon to drain
and exit with status **75**, then waits for a different, authenticated, ready
instance. It reports the version the manager actually started. If the manager
does not bring one back, the command fails after a bounded wait and never falls
back to starting its own process. Running harnesses reattach to the next daemon.

Configure the manager to restart on failure, with status **0** classified as
success. `agx daemon stop` and a normal shutdown signal then exit with status 0
and stay stopped for the current manager session. This is not a persistent
disable: the manager may start the workload again at the next login or boot.
Intentional restarts use the manager's normal retry budget and backoff, so choose
those limits accordingly.

| Manager | Restart configuration                                                          |
| ------- | ------------------------------------------------------------------------------ |
| SUSM    | `success_exit_codes = [0]` and `[restart] policy = "on-failure"`               |
| systemd | `Restart=on-failure`                                                           |
| launchd | `KeepAlive = { SuccessfulExit = false; }`, without other keep-alive conditions |

On Windows, run the per-user manager and `agxd` in the interactive user's logon
session. A daemon started by SSH can lack access to that user's Credential
Manager even when both processes have the same username. Keep the manager's
launch command pointed at the selected installation so its next start picks up
both updated binaries; package updates alone do not restart a managed daemon.

`daemon.autostart: false`, or `AGX_NO_AUTOSTART=1` for one shell, only disables
implicit startup by ordinary commands. Explicit lifecycle commands and existing
wrappers' recovery still work in that mode. Use `managed: true` when the manager
must be the only process owner. Managed ownership is advertised in discovery
when `agxd` starts; stop the existing daemon through its current owner before
changing ownership modes. Client-side spawning requires a valid root file;
foreground `agxd` can still serve diagnostics when configuration is broken.
Upgrade old CLI binaries and let wrappers from versions without managed mode
finish before migrating: those older processes do not enforce this ownership.

Each canonical root configuration path gets its own daemon within a cache.
Discovery, the lock and log live in `<cache>/runtime/<root-hash>/`, where
`root-hash` is a short hash of that path. The saved port and revocations live in
configuration-scoped rows in `<state>/state.db`.
Configurations can run at the same time and share the package and artifact cache.

Starting is safe to race. A starting daemon takes its runtime directory's lock before it
binds a loopback port, and writes `daemon.json` (with an ID that changes on every
start) only once it serves. Commands that start one at the same time may each
spawn a daemon; the extras exit and every command connects to the winner. A
command reports a failed start, with the log's last line, only once the lock is
free and nothing answers.

When a self-managed daemon dies without shutting down, the wrappers of running harnesses
start a new one: a wrapper does so only when no daemon holds the lock and the
discovery file still names the daemon it used, so it never races a daemon
another command is starting.

Wrappers report durable launch history through the daemon, separately from leased
MCP access. When a wrapper finishes while no daemon runs, it reconnects or starts
one to record the original exit time, even after a graceful stop or broken YAML.
This recovery also applies when initial autostart was disabled, but never when
`daemon.managed` is true: those wrappers wait for their manager instead. An
unreadable database remains an error; wrappers never write a
fallback history file. The daemon can start with invalid YAML so diagnostics,
history reporting and credential reset remain available.

## Launches

Every running harness is a launch:

```sh
agx launch list          # running and recently finished launches
agx launch show <id>     # one launch, with the state of each of its MCP servers
agx launch revoke <id>   # cut off its MCP access; the harness keeps running
```

`launch show` is the place to see why an MCP server does not work in a session,
for example `needs-auth` with the command to log in. A `codex app-server`
process shows one launch per thread, indented under the process.

If a harness's agx wrapper dies without saying goodbye, its access expires
after `daemon.lease_seconds` (60 by default). The log's `launch ended` line for
an expired launch has a `reason`: `harness gone` or `wrapper and harness gone`
when the harness exited too; `wrapper gone, harness still running` for a
harness left without its wrapper; `renewals stopped, wrapper still running` for
a wrapper that did not renew in time, as after the computer slept; and `no
renewal` for a launch whose harness never started.

On Windows, a host that ends a harness by terminating the agx wrapper it spawned
ends the harness and its descendants with it, as their wrapper keeps them in a
job object. When the harness exits on its own, what it started keeps running.
On Unix, killing the wrapper with SIGKILL leaves the harness running until the
host closes its terminal or pipes; SIGTERM and SIGINT reach the harness.

A wrapper whose launch the daemon refuses to take back, because the launch's
directory no longer resolves or the configuration does not load, retries for 5
minutes and then warns that MCP access is lost.

## Cache cleanup

A lease after it starts, and hourly from then on, the daemon deletes the
cached copies of skills and instructions that no launch used for 7 days, and
files that interrupted writes, launches, and package installations left in the
cache more than an hour ago. A copy a launch uses is kept for as long as the
launch runs.

## Graceful shutdown

When the daemon stops (`daemon stop`, `daemon restart`, an upgrade, Ctrl-C on a
foreground daemon, or SIGTERM), it:

1. stops taking new launches, logins, and MCP sessions; agx commands waiting on
   it retry with the next daemon;
2. lets MCP requests and tool calls in progress finish, for up to
   10 seconds, or `--timeout`;
3. closes local MCP servers: their input first, then SIGTERM, then a kill;
4. exits. Unless `--force` revoked them, wrappers attach their launches to the
   next daemon, which starts on the same port.

A second Ctrl-C or SIGTERM skips the wait.

## Files

| File                      | Location                                                                                           |
| ------------------------- | -------------------------------------------------------------------------------------------------- |
| Log                       | `daemon.log` in `runtime/<root-hash>/` in the [cache directory](configuration.md#where-files-live) |
| Port and revoked launches | configuration-scoped rows in `<state>/state.db`                                                    |

Both are readable only by you.
