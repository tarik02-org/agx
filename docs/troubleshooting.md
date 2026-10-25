# Troubleshooting

## Start with `agx doctor`

```sh
agx doctor            # checks for the current directory
agx doctor ~/work/app # for another directory
agx doctor --all      # test every MCP server, not only the ones selected here
```

`doctor` prints one line per check, `ok`, `warn`, or `fail`, then the next step
for every problem. It checks:

- that the configuration loads, and from which files;
- that packages are installed and lockfiles are current;
- each harness: found, and new enough;
- Codex skill plugins, per Codex-based harness definition;
- the daemon: running, the same agx version, serving this configuration;
- every MCP server selected here, by connecting to it;
- secrets, and `sops` when you use it;
- shims: installed and on `PATH`, or definitions already linked to agx on `PATH`.

It changes nothing, except starting the daemon if needed. It exits with status
1 when a check fails.

## Common problems

**"configuration failed to load" or an edit command refuses to write.**
Run `agx config validate`; it names the file and line. Edit commands never
write an invalid configuration, so the message explains what the change would
have broken.

**A launch warns "package … is not installed; run agx install".**
A package or Git skill is not installed yet, or changed since it was installed
(for example after `git pull` in your configuration repository). The launch
goes on without that package's skills, instructions, and MCP servers; run
`agx install` to get them. Launches never download anything themselves.
`agx config validate` and `agx doctor` list every missing package.

**`agx install` says "git is not on PATH".**
Git packages and Git skills are fetched with `git`. Install it, or add it to
`PATH` for the shell that runs `agx install`. When the daemon installs, it uses
the `PATH` of the shell that started it; `agx daemon restart` picks up a new one.

**An MCP server is missing in the harness.**
Run `agx explain` in that directory to see whether it is selected and which
rule removed it. Harness-specific rules only show with `agx explain --harness
<name>`. The server appears in the harness as `agx-<id>`.

**An MCP server's tools fail with an authorization error.**
Run `agx secret login <id>` for an OAuth server, or store a new token with
`agx secret set <id>` for a `keychain` one. `agx launch show
<launch-id>` shows each server's state in a running session, for example
`needs-auth`.

**An MCP server does not start or answer.**
Run `agx mcp test <id>`: it connects outside any harness and shows the server's
error. For local servers, check that the command is on the daemon's `PATH` (or
use an absolute path), and look at the [daemon log](#logs).

**A changed environment variable, `PATH`, or sops key has no effect.**
The daemon keeps the environment of the shell that started it. Run `agx daemon
restart` from a shell that has the new value. See
[The daemon](daemon.md#its-environment).

**The wrong harness version runs.**
agx runs the harness found on your shell's `PATH` (`which codex` shows which).
Set an absolute `executable` in the harness definition to pin one. `agx doctor`
reports versions that are too old.

**Codex shows no skills.**
Codex loads skills only as plugins: run `agx install`, and check that Codex
plugins are not turned off (`features.plugins = false` in Codex's
`config.toml`). `agx doctor` checks both. agx installs only skills that a rule
selects, so check with `agx explain` that the skill is selected.

**"daemon of another build" or "version mismatch".**
A daemon from another agx build is running and the command could not replace
it. Run `agx daemon stop`, then try again.

**"upgrade agx".**
A newer agx wrote one of agx's own files (package pins, the shim manifest), and
this older agx will not overwrite them. Use the newer agx.

**"the agx wrapper started agx again instead of the native harness".**
A link or shim named after the harness, and pointing at agx, comes before the
real harness on your `PATH`. Set the harness's `executable` to the absolute path
of the real binary (see [Harnesses](harnesses.md#which-executable-runs)).

**"daemon unavailable".**
Automatic starting is off (`daemon.autostart: false` or `AGX_NO_AUTOSTART=1`).
Run `agx daemon start`, or check the [log](#logs) if it does not stay up.

**The daemon serves another configuration.**
You used a different `AGX_CONFIG` or `--config` with the same cache while a
harness launched with the other configuration still runs. agx replaces a daemon
of another configuration only when it has no active launches (`agx launch
list`), so their MCP servers keep working. Exit those harnesses, rerun with the
configuration the message names, or give the other setup its own `AGX_CACHE`.

**A shim is not found.**
Add `~/.local/share/agx/shims` to your `PATH`, and run `agx shims install`
after adding harness definitions.

**macOS: "agx cannot be opened because the developer cannot be verified".**
Release binaries are not notarized. Remove the quarantine flag the browser
added: `xattr -d com.apple.quarantine /path/to/agx`.

## Logs

The daemon writes `daemon.log` in the `runtime` directory of the cache:

| Platform | Default log location |
| --- | --- |
| Linux | `~/.cache/agx/runtime/daemon.log` |
| macOS | `~/Library/Caches/agx/runtime/daemon.log` |
| Windows | `%LOCALAPPDATA%\cache\agx\runtime\daemon.log` |

`agx daemon start` prints the path.

## Starting over

To reset agx without touching your configuration, stop the daemon and delete the
cache directory, then reinstall:

```sh
agx daemon stop --force
rm -rf ~/.cache/agx          # ~/Library/Caches/agx on macOS
agx install
```

Stored secrets and OAuth logins live in the OS keychain and survive this; remove
them with `agx secret logout <id>`.
