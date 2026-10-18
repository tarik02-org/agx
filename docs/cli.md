# CLI design

This interface is implemented. See [verification notes](verification.md) for
platform and protocol limits.

The default root is `$XDG_CONFIG_HOME/agx/agx.yaml`. When it is unset, that is
`~/.config` on Linux, `~/Library/Application Support` on macOS, and
`%LOCALAPPDATA%` on Windows; `daemon start --config` selects another root.
Generated artifacts and installed packages live under `$XDG_CACHE_HOME/agx`
(`~/.cache`, `~/Library/Caches`, or `%LOCALAPPDATA%\cache` when unset).
Root selection is explicit, not automatic discovery of project-local files.

## Harness entry points

```text
agx codex [native arguments...]
agx claude [native arguments...]
agx pi [native arguments...]
agx <definition> [native arguments...]
agx run <harness> -- [native arguments...]
```

Each harness definition in the configuration is a command of its own. If the
configuration cannot be read, only the built-ins remain, with a warning when one
launches.

Harness entry points preserve native flags, including `app-server`, print/RPC
modes, resume, and version/help probes. Codex's `-C`/`--cd` changes the
directory used for resolution; Claude and Pi have no such flag. Do not run native arguments through agx's
ordinary flag parser. Forward stdout, stdin, exit codes, and interrupts correctly;
agx diagnostics go to stderr. Each invocation is classified into a
[mode](configuration.md#harnesses): Codex subcommands select `exec` (`exec`,
`review`), `resume` (`resume`, `fork`), or `app-server`; Claude
`-p`/`--print` selects `print`, else `-r`/`--resume`/`-c`/`--continue` select
`resume`; Pi `--mode rpc` selects `rpc`, and `-p`/`--print` or `--mode json`
select `print`. Help/version probes and non-session native commands such as
login, plugin management, and app-server tooling are `passthrough`: they run
without a managed launch, receiving only the definition's `passthrough` groups and
`env`, whose [injected values](configuration.md#injected-values) the wrapper
resolves itself.

Support invocation through a harness-named wrapper or symlink. Resolve a configured
absolute native executable first; otherwise search PATH while excluding agx
itself, so a `codex` symlink cannot recurse. Handle Windows executable/script
launching explicitly rather than constructing a POSIX shell command.

## Shims

```text
agx shims install [--dir <directory>]
agx shims list [--dir <directory>] [--json]
agx shims remove [--dir <directory>]
```

Shims put harness definitions on PATH: after `install`, `codex-cpa` runs `agx
codex-cpa`. Each is agx under the definition's name: a symlink on Unix, a hard
link or copy named `<name>.exe` on Windows, and a script running `agx <name>`
only where linking fails. Built-ins get no shim. The directory defaults to
`~/.local/bin` (`%LOCALAPPDATA%\agx\bin` on Windows). Shims target the path agx
was invoked through when it is a stable link to the running binary, such as
`~/.nix-profile/bin/agx`, and otherwise the resolved binary, with a warning when
that lies in `/nix/store`.

`install` synchronizes the directory with the definitions: it creates missing
shims, updates those whose target changed, and removes those of deleted
definitions. `remove` deletes every shim. Both touch only files recorded in the
directory's `.agx-shims.json` and unchanged since; others are reported and left
in place. `list` shows each shim's name, kind (`symlink`, `hardlink`, `copy`, or
`script`), state (`current`, `stale`, `missing`, `modified`, or `conflict`), and
target. `install` and `list` warn when the directory is not on PATH and when
another executable on PATH has a shim's name.

## Configuration and packages

```text
agx config validate
agx config schema [--package]
agx resolve [directory] [--harness <name>] [--json]
agx explain [directory] [--harness <name>] [--json]
agx install [--frozen]
agx update [package-id...]
```

The default directory is the current directory. `resolve` reports selected
resources; `explain` also reports removals, source locations, bundle chains,
non-default instruction `order`,
and the injected values a launch would resolve, shown as configured (`!secret
cpa-key`). Without
`--harness` they apply only rules without a harness filter; with it, they also
apply matching scope rules and the harness's own selection, as a launch does.
These commands can read local configuration without daemon startup. They do not
fetch missing packages; diagnostics direct the user to `install`. `config schema`
prints the JSON Schema of configuration fragments, or with `--package` of package
manifests ([editor support](configuration.md#editor-support)).

`install` materializes the declared graph, including the anonymous packages of
Git skills and instructions, and may create/update its lockfiles; pinned sources
need no lock entry, so a fragment declaring only pinned ones gets no lockfile.
`--frozen` permits no lockfile changes. `update` refreshes selected packages
(an anonymous one by its lock key, such as `skill:review-strict`), or all packages
when none are named. Running a harness requires a complete, matching
lock/cache; it never silently updates dependencies.

## Daemon and launches

```text
agx daemon start [--foreground] [--config <path>]
agx daemon status [--json]
agx daemon stop [--force]
agx launch list [--json]
agx launch show <launch-id> [--json]
agx launch revoke <launch-id>
```

Use one daemon per OS user. v1 requires explicit startup; automatic
startup can be added later. Stopping refuses active launches unless
forced. Revocation closes MCP access and upstream sessions; it does not kill the
harness. Launch inspection excludes secrets. A graceful wrapper exit releases
its launch; abrupt exits are recovered through lease expiry.

## Secrets

```text
agx secret login <secret-id> [--no-browser]
agx secret set <secret-id> --stdin
agx secret status [secret-id] [--json]
agx secret logout <secret-id>
```

A secret ID names a top-level secret or, for [inline and on-demand
auth](configuration.md#mcp-auth), the MCP server that declares it.

Login runs an OAuth secret's authorization and blocks until it succeeds, fails,
or expires (after five minutes). It opens the authorization URL in the default
browser (`open`, `xdg-open`, or `rundll32 url.dll,FileProtocolHandler`, each given
the URL as one argument) and prints it too; `--no-browser` only prints it. A
browser that fails to open is a warning. On a terminal, stderr shows the
instructions and a waiting line; when stdout is not a terminal it carries only the
URL, and the command still waits. Success prints `Logged in to <id>.` and exits 0;
failure or expiry exits 1. Ctrl-C, SIGTERM, or SIGHUP closes the login stream, so
the daemon cancels the attempt, and exits 128 plus the signal number (130 for
Ctrl-C). Concurrent logins for the same secret share one attempt, which is
cancelled when the last one stops waiting. A daemon that stops meanwhile fails
the login; run it again.

`set` writes a configured keychain secret; values are never CLI arguments.
Status returns metadata only: each secret's kind, whether it is inline in an MCP
server or on-demand OAuth, its state, and its expiry. Logout clears the secret's
stored credentials and closes upstream sessions of MCP servers using it. It does
not revoke unrelated launches or secrets.

## Output contract

JSON commands produce one versioned document on stdout; diagnostics use stderr.
Human output is styled per stream: `--color=auto` (the default) styles a stream
only when it is a terminal, `NO_COLOR` is unset, and `TERM` is not `dumb`;
`CLICOLOR_FORCE` or `FORCE_COLOR` styles it anyway. `--color=always` and
`--color=never` override detection. Styled `explain` shows one line per resource with shortened
paths; `--verbose` shows every step with full paths, as unstyled output does. Unstyled output keeps the same words and
order, one record per line, for pipes and grep. JSON is never styled.
Harness entry points style only their own stderr diagnostics.
JSON and inspection never reveal grants, credential values, or prompt bodies.
Usage errors exit 2, operational failures exit 1, success exits 0. Harness entry
points preserve native exit behavior. Before spawning, wrappers require the daemon to run the
same agx version and fail with actionable instructions instead of printing into a
harness protocol stream.
