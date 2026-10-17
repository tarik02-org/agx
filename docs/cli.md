# CLI design

This interface is implemented. See [verification notes](verification.md) for
platform and protocol limits.

The default root is `$XDG_CONFIG_HOME/weft/weft.yaml`. When it is unset, that is
`~/.config` on Linux, `~/Library/Application Support` on macOS, and
`%LOCALAPPDATA%` on Windows; `daemon start --config` selects another root.
Generated artifacts and installed packages live under `$XDG_CACHE_HOME/weft`
(`~/.cache`, `~/Library/Caches`, or `%LOCALAPPDATA%\cache` when unset).
Root selection is explicit, not automatic discovery of project-local files.

## Harness entry points

```text
weft codex [native arguments...]
weft claude [native arguments...]
weft pi [native arguments...]
weft run <harness> -- [native arguments...]
```

Harness entry points preserve native flags, including `app-server`, print/RPC
modes, resume, and version/help probes. Codex's `-C`/`--cd` changes the
directory used for resolution; Claude and Pi have no such flag. Do not run native arguments through Weft's
ordinary flag parser. Forward stdout, stdin, exit codes, and interrupts correctly;
Weft diagnostics go to stderr. Help/version probes and non-session native commands
such as login and plugin management pass through without a managed launch.

Support invocation through a harness-named wrapper or symlink. Resolve a configured
absolute native executable first; otherwise search PATH while excluding Weft
itself, so a `codex` symlink cannot recurse. Handle Windows executable/script
launching explicitly rather than constructing a POSIX shell command.

## Configuration and packages

```text
weft config validate
weft resolve [directory] [--json]
weft explain [directory] [--json]
weft install [--frozen]
weft update [package-id...]
```

The default directory is the current directory. `resolve` reports selected
resources; `explain` also reports exclusions, source locations, and bundle chains.
These commands can read local configuration without daemon startup. They do not
fetch missing packages; diagnostics direct the user to `install`.

`install` materializes the declared graph and may create/update its lockfile;
`--frozen` permits no lockfile changes. `update` refreshes selected packages, or
all packages when none are named. Running a harness requires a complete, matching
lock/cache; it never silently updates dependencies.

## Daemon and launches

```text
weft daemon start [--foreground] [--config <path>]
weft daemon status [--json]
weft daemon stop [--force]
weft launch list [--json]
weft launch show <launch-id> [--json]
weft launch revoke <launch-id>
```

Use one daemon per OS user. v1 requires explicit startup; automatic
startup can be added later. Stopping refuses active launches unless
forced. Revocation closes MCP access and upstream sessions; it does not kill the
harness. Launch inspection excludes secrets. A graceful wrapper exit releases
its launch; abrupt exits are recovered through lease expiry.

## Authentication

```text
weft auth login <connection-id>
weft auth set <auth-id> --stdin
weft auth status [auth-id] [--json]
weft auth logout <auth-id>
```

Login resolves the connection in the current directory and starts its OAuth flow.
`set` writes a configured keychain-backed secret; values are never CLI arguments.
Status returns metadata only. Logout clears the identity's credentials and closes
affected upstream sessions. It does not revoke unrelated launches or identities.

## Output contract

JSON commands produce one versioned document on stdout; diagnostics use stderr.
JSON and inspection never reveal grants, credential values, or prompt bodies.
Usage errors exit 2, operational failures exit 1, success exits 0. Harness entry
points preserve native exit behavior. Before spawning, wrappers require the daemon to run the
same weft version and fail with actionable instructions instead of printing into a
harness protocol stream.
