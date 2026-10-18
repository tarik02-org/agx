# CLI design

This interface is implemented. See [verification notes](verification.md) for
platform and protocol limits.

The default root is `$XDG_CONFIG_HOME/agx/agx.yaml`. When it is unset, that is
`~/.config` on Linux, `~/Library/Application Support` on macOS, and
`%LOCALAPPDATA%` on Windows; `daemon start --config` selects another root.
Generated artifacts and installed packages live under `$XDG_CACHE_HOME/agx`
(`~/.cache`, `~/Library/Caches`, or `%LOCALAPPDATA%\cache` when unset). Launch
records live under `$XDG_STATE_HOME/agx` (`~/.local/state`, `~/Library/Application
Support`, or `%LOCALAPPDATA%` when unset), which wiping the cache keeps.
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
`resume`; Pi `--mode rpc` selects `rpc`, `-p`/`--print` or `--mode json`
select `print`, else `--session`/`-c`/`--continue`/`-r`/`--resume`/`--fork`
select `resume`. Help/version probes and non-session native commands such as
login, plugin management, and app-server tooling are `passthrough`: they run
without a managed launch, receiving only the definition's `passthrough` groups and
`env`, whose [injected values](configuration.md#injected-values) the wrapper
resolves itself.

Support invocation through a harness-named wrapper or symlink. Resolve a configured
absolute native executable first; otherwise search PATH while excluding agx
itself, so a `codex` symlink cannot recurse. Handle Windows executable/script
launching explicitly rather than constructing a POSIX shell command.

## Launcher and sessions

```text
agx
agx sessions [query] [--all] [--json]
```

Bare `agx` with stdin and stdout on a terminal opens a full-screen launcher;
otherwise it prints help. One search field filters two groups: **New session**,
the built-in harnesses and then the definitions, which start in the current
directory; and **Sessions**, each showing its harness, how long ago it was
updated, its title or first message, and its directory unless it is the current
one (`./sub` below it, `~/...` under the home directory).

| Key | Action |
| --- | --- |
| type | filter harnesses and sessions |
| ↑ ↓, Ctrl-P Ctrl-N, PgUp PgDn | move |
| Enter | start the harness, or resume the session |
| Tab | toggle sessions between **here** (this directory and below) and **all** |
| Ctrl-H | pick another harness of the same built-in to resume with |
| Esc | quit (exit 0); in the picker, back |
| Ctrl-C | quit (exit 130) |

The launcher restores the terminal before the harness starts. Resuming changes
to the session's directory and runs `agx <harness> <resume arguments>`: Codex
`resume <id>`, Claude `--resume <id>`, Pi `--session <id>`. The launch is
therefore managed and classified as `resume`, with the directory's resources and
the harness's `resume` argument groups. A session resumes with the definition
that started it when agx recorded that launch and the definition still exists,
otherwise with its built-in. A session whose directory was deleted is listed but
cannot be resumed.

Sessions are read from the harnesses' own stores, honouring `CODEX_HOME`,
`CLAUDE_CONFIG_DIR`, and `PI_CODING_AGENT_DIR` (and Pi's
`PI_CODING_AGENT_SESSION_DIR` or `sessionDir` setting), so sessions started
without agx appear too; subagent sessions do not. Only a title and the first user
message are shown, shortened; stores are never written. Unreadable session files
are skipped and counted. The index is cached under the cache directory and
re-read per file when its size or modification time changes; launch records live
in the state directory, so wiping the cache keeps them
([ADR 0013](adr/0013-launcher-and-session-search.md)).

`agx sessions` lists the sessions of this directory and below, or of every
directory with `--all`, newest first; a query keeps those matching every word
fuzzily within one field (harness, title, first message, or directory), best
first. The table shows harness, age, session ID, directory, and title or first
message; on a terminal the title is shortened to fit. `--json` prints
`{scope, directory, query, sessions, files, errors}`, where each session has
`id`, `directory`, `title`, `message`, `started`, `updated`, `builtin`,
`harness` (the definition it resumes with), and `recorded`; `files` counts
session files `parsed`, reused from the cache (`cached`), and `malformed`.

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
`$XDG_DATA_HOME/agx/shims` (`~/.local/share/agx/shims`, also on macOS; `%LOCALAPPDATA%\agx\shims` on Windows), a directory only agx writes to; add it to PATH to run shims by name. Shims target the path agx
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

## Editing configuration

```text
agx mcp add <id> <url> | add <id> -- <command> [args...]
agx mcp list [--json] | remove <id> [--force] | login <id> | test <id>
agx skill add <source> [--skill <name>]... [--all] [--ref <ref>] [--path <dir>] [--package <id>]
agx skill list [--json] | remove <id> [--force]
agx instructions add <path> [--id <id>] | add --id <id> --text <text>
agx instructions list [--json] | remove <id> [--force]
agx harness add <name> --base <harness> [--executable <path>] [--env K=V]... [-- args...]
agx harness list [--json] | remove <name> [--force]
agx use <id> [--kind <kind>]
agx unuse <id> [--kind <kind>]
```

These commands edit the YAML fragments for the common cases
([ADR 0014](adr/0014-configuration-editing-commands.md)); everything else stays
hand-written. Commands that write share these flags:

| Flag | Meaning |
| --- | --- |
| `--scope user\|here\|<dir>` | where to enable: everywhere (the default), the current directory, or a directory |
| `--harness <name>` | only for launches of these harnesses (repeatable) |
| `--file <path>` | the fragment new definitions go to (default: the root configuration) |
| `--define-only` | define without enabling (`add` commands) |
| `-y`, `--yes` | write without confirming |

`add` defines a resource and enables it in the scope. Enabling appends the ID to
the first rule with the same directory and harness filter (`--scope user` is a
rule without `directory`), in load order, or creates one in the file holding the
definition. `use` enables an existing definition and drops it from that rule's
`remove`; `unuse` removes it from the rule's `add`, or, when another rule added
it, appends it to `remove`. `--kind skills|instructions|mcp|bundles` picks one when
an ID names several. `remove` refuses while other entries reference the
definition, listing them; `--force` removes those entries too, and rules left
empty. Definitions inside packages cannot be removed this way.

```sh
agx mcp add github https://api.githubcopilot.com/mcp
agx mcp add fs --scope here -- npx -y @modelcontextprotocol/server-filesystem .
agx mcp add docs https://docs.example/mcp --header 'X-Key=!secret docs' --auth none
agx unuse docs --scope ~/work/oss          # docs stays enabled elsewhere
agx skill add anthropics/skills --skill pdf --skill docx
agx skill add ./skills/deploy --harness claude
agx instructions add --id pnpm --text 'Use pnpm, not npm.' --scope ~/work
agx harness add codex-cpa --base codex --env 'CPA_KEY=!secret cpa-key' -- -c 'model_provider="cpa"'
```

`mcp add` takes a URL for an HTTP server or a command after `--` for a stdio
one. `--header` (HTTP) and `--env` (stdio) take `KEY=VALUE`; a value written
`!secret <id>`, `!env NAME`, or `!file PATH` becomes that
[tag](configuration.md#injected-values). `--auth` is `none`, `oauth`,
`keychain`, or `!secret <id>`; omitted, the server uses OAuth on demand. After
adding an HTTP server, agx asks it for an authorization challenge. When it wants
an OAuth login, a terminal offers to log in now through the running daemon,
exactly as `mcp login`, which runs `secret login` for the server's secret
(`--no-browser` prints the URL instead of opening it); otherwise agx prints the
command for later. `mcp test` connects to the server through the daemon outside
any launch, with its credentials and the caller's environment variables its
values name, and lists its tools; failures report the endpoint's diagnosis, never
values.

`skill add` takes a directory, a Git URL, or GitHub's `owner/repo[@ref][/path]`
(`--ref` and `--path` set them for URLs); a relative path without `./` is read as
a path only when it exists. It finds every `SKILL.md` below the source and asks
which to add unless `--skill` or `--all` decides; without a terminal, a source
with several skills needs one of them. A directory becomes path skills named
after their folders. A Git source becomes a package, named after the repository
unless `--package` says otherwise, that agx installs and locks before anything
is written; the chosen skills are enabled by their package-qualified IDs
(`skills/pdf`). A repository without `agx.package.yaml` declares every skill it
holds ([packages](configuration.md#packages-and-locks)).

`list` prints a table, or `{version, data}` with `--json`; values appear as
configured, and headers and environments are left out. `harness list` includes
the built-ins.

On a terminal, missing arguments are asked for, and the change is shown as a
unified diff and confirmed before it is written; Esc or answering no writes
nothing (exit 1), Ctrl-C exits 130. Without a terminal, a missing argument is a
usage error and `--yes` is implied. Prompts draw on stderr; the diff goes to
stdout, styled per `--color`.

Edits keep every line they do not change, including comments, blank lines, key
order, quoting, flow style, and tags; a new list is written in flow style unless
its siblings use block style. The edited configuration, and any lockfile an
install would write, is loaded and validated in memory first; only a valid result
is written, each file atomically with its mode kept. A file that is read-only,
or resolves into `/nix/store`, is refused with the reason; pass `--file` to
write elsewhere. A fragment outside the configuration's imports is written with a
warning.

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
same agx build (version and executable contents) and fail with actionable instructions instead of printing into a
harness protocol stream.
