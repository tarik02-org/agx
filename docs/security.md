# Security model

agx is a convenience and organization tool, not a sandbox. Read this before
relying on it to keep anything from your agents.

## Everything runs as you

agx, its daemon, your MCP servers, and every harness it launches run as your
user account, with your permissions. An agent started by agx can read and write
whatever you can: your files, your shell's tools, your SSH keys, your Git
credentials, and other programs' configuration.

That includes agx itself. An agent can run `agx` commands, read agx's files,
and talk to the daemon, just as you can. Choosing which MCP servers and skills a
directory gets controls what an agent is *offered*, not what it is *able* to
reach.

If you need to contain an agent, run it in a container, a virtual machine, a
separate user account, or the harness's own sandbox mode. agx's settings do not
replace that.

## What agx does protect

**Against other users on the same machine.** The daemon listens only on
`127.0.0.1`, and every request needs a token. The runtime directory, with the
token and the daemon's log, and the state directory, with the secret store, the
daemon's state, and launch records, are readable only by your account (private
ACLs on Windows). Other local users cannot use your MCP servers or credentials.

Nothing else is hidden from them. Caches (installed packages, launch artifacts,
per-launch files) hold only what your configuration already says, and agx
creates them with default permissions; your configuration directory and its
lockfiles keep whatever permissions you give them.

The daemon's loopback token does not protect against your own processes: any
program running as you can read it.

**Secret values stay out of files and output.** Values from `!secret`, `!env`,
and `!file` are resolved at launch and kept in memory. They are not written to
agx's generated files, lockfiles, logs, `explain`, `--json` output, or error
messages. Keychain secrets and OAuth tokens live in `secrets.json` in the state
directory, encrypted under a key that the OS keychain holds; there is no
plaintext fallback.

**MCP credentials stay in the daemon.** Harnesses never receive the API keys or
OAuth tokens of remote MCP servers; they talk to the daemon, which adds them.
Each harness gets a grant that works only for its own launch's MCP servers and
expires when the launch ends or is revoked (`agx launch revoke <id>`).
`agx mcp call` and the client `agx mcp types` generates receive only tool
results; the daemon runs the calls. Since the management token is readable by
your processes, an agent can reach every configured server with
`agx mcp call --all`; the grant scopes what a launch reaches by default, not
what your account can do.

Some values necessarily reach the harness or a server: whatever you put in a
harness's `environment`, and a local MCP server's `environment`. Those are
visible to that process and to anything it runs.

## Things to keep in mind

- **Skills and packages are code.** A skill can include scripts that the agent
  runs as you. Add skills and packages only from sources you trust, and pin or
  lock them (`agx install` writes lockfiles; `agx update` is the only command
  that moves to newer versions).
- **MCP servers are code too.** A local MCP server runs as you, with the
  environment you give it.
- **Harness arguments are public.** Other processes can see command lines,
  which is why `!secret` and `!env` are not allowed in harness `args`.
- **Your configuration can be shared.** Keep credentials in secrets, not in
  YAML literals, so the configuration directory can live in Git.
