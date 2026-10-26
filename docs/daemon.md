# The daemon

agx keeps a small background process, the daemon, running as you. It:

- runs your local MCP servers and connects to remote ones;
- holds MCP credentials and OAuth tokens, and refreshes them;
- gives each running harness access to exactly the MCP servers selected for it,
  and takes that access away when the harness exits;
- receives OAuth logins in your browser.

You rarely need to think about it. Any command that needs it (launching a
harness, `agx mcp test`, `agx mcp login`, `agx secret …`, `agx doctor`) starts
it in the background if it is not running. Help, version, and login commands of
harnesses, and commands that only read or edit configuration, work without it.

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

Upgrading agx works the same way: the first command of the new version stops
the old daemon gracefully and starts its own, keeping running launches.
`agx daemon status` does not replace it: it reports a daemon of another agx
version or build with a warning to run `agx daemon restart`.

Downgrading is different. An older agx does not touch a daemon or saved
launches written by a newer one, and asks you to upgrade agx instead. To go
back anyway, run `agx daemon stop --force` with the newer agx first; that ends
the saved launches' MCP access.

## Its environment

The daemon inherits the environment of the command that started it. That
matters for:

- `daemon_environment` secrets;
- sops keys from `SOPS_AGE_KEY_FILE` and similar variables;
- local MCP servers whose commands are looked up on `PATH`.

After changing any of these in your shell, run `agx daemon restart` from that
shell. Values your configuration reads with `!env` or `environment` secrets are
different: they come from the shell you launch each harness from, every time.

## Starting it yourself

To run the daemon under a service manager such as launchd or systemd, start it
in the foreground:

```sh
agx daemon start --foreground
```

and turn off automatic starting, so commands fail instead of starting a second
copy outside the service manager:

```yaml
daemon:
  autostart: false
```

`AGX_NO_AUTOSTART=1` in the environment does the same for one shell.

There is one daemon per cache directory, and it serves the configuration it was
started with. Commands find it through files in the cache directory. A command
using another configuration replaces the daemon when it has no active launches;
while it has some, harness launches with the other configuration are refused, so
the running harnesses keep their MCP servers. For a separate setup with another
configuration used at the same time, also use another cache.

Starting is safe to race. A starting daemon takes a per-user lock before it
binds a loopback port, and writes `daemon.json` (with an ID that changes on every
start) only once it serves. Commands that start one at the same time may each
spawn a daemon; the extras exit and every command connects to the winner. A
command reports a failed start, with the log's last line, only once the lock is
free and nothing answers.

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
after `daemon.lease_seconds` (60 by default).

## Graceful shutdown

When the daemon stops (`daemon stop`, `daemon restart`, an upgrade, Ctrl-C on a
foreground daemon, or SIGTERM), it:

1. stops taking new launches, logins, and MCP sessions; agx commands waiting on
   it retry with the next daemon;
2. lets MCP requests and tool calls in progress finish, for up to
   `daemon.shutdown_grace_seconds` (10 by default) or `--timeout`;
3. saves its running launches for the next daemon (unless `--force`);
4. closes local MCP servers: their input first, then SIGTERM, then a kill;
5. exits.

A second Ctrl-C or SIGTERM skips the wait.

## Files

| File | Location |
| --- | --- |
| Log | `daemon.log` in `runtime/` in the [cache directory](configuration.md#where-files-live) |
| Saved launches | `daemon-state.json` in the state directory |

Both are readable only by you.

If `daemon-state.json` is damaged, for example by an edit by hand, the daemon
moves it aside to `daemon-state.json.bad-<UTC time>`, says so in its log, and
starts without the saved launches; harnesses that were running lose their MCP
access and need a relaunch.
