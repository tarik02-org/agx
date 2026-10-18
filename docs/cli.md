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
`-p`/`--print` or `--input-format stream-json` (Agent SDK hosts) selects
`print`, else `-r`/`--resume`/`-c`/`--continue` select `resume`; Pi `--mode rpc` selects `rpc`, `-p`/`--print` or `--mode json`
select `print`, else `--session`/`-c`/`--continue`/`-r`/`--resume`/`--fork`
select `resume`. Help/version probes and non-session native commands such as
login, plugin management, and app-server tooling are `passthrough`: they run
without a managed launch, receiving only the definition's `passthrough` groups and
`env`, whose [injected values](configuration.md#injected-values) the wrapper
resolves itself.

`codex app-server` on standard input and output (no `--listen`, `--stdio`, or
`--listen stdio://`) serves many threads, each in its own directory. The wrapper
relays its JSON-RPC and gives every thread started, forked, or resumed a thread
launch for that directory: its MCP servers, instructions, and skills
([ADR 0015](adr/0015-per-thread-resources-for-multi-session-hosts.md)). A
resumed thread keeps the instructions it started with; a turn in another
directory switches skills at once and MCP servers on the next resume. Claude
Code reading `--input-format stream-json` (the Agent SDK) gets agx instructions
prepended to the system prompt append of its `initialize` request.

Support invocation through a harness-named wrapper or symlink. When agx runs under
another name and the configuration fails to load, it reports `agx: <name>:
configuration failed to load: <error>` and exits 1 instead of running its own
CLI; names starting with `agx` (a renamed agx binary) still run the CLI. Resolve a configured
absolute native executable first; otherwise search PATH while excluding agx
itself, so a `codex` symlink cannot recurse. Handle Windows executable/script
launching explicitly rather than constructing a POSIX shell command.

### Harness versions

| Harness | Needs | Tested |
| --- | --- | --- |
| Codex | plugins and marketplaces (`codex plugin`, `features.plugins`), `-c` overrides for `mcp_servers` and `developer_instructions` | 0.160 and later (0.162.1) |
| Claude Code | `--plugin-dir`, `--append-system-prompt-file`, `--mcp-config` | 2.1.29x (2.1.300) |
| Pi | `pi.registerMcpServer()` with `exposure`, from 0.99.0 | 1.2.0 |

Older versions may launch but lose skills, instructions, or MCP servers;
`agx doctor` reports each harness's version against these minimums.

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
agx shims install [--dir <directory>] [--json]
agx shims list [--dir <directory>] [--json]
agx shims remove [--dir <directory>] [--json]
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
agx config validate [--json]
agx config schema [--package]
agx resolve [directory] [--harness <name>] [--json]
agx explain [directory] [--harness <name>] [--json]
agx install [--frozen] [--json]
agx update [package-id...] [--json]
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

`install` also registers native harness plugins: Codex loads skills only as
plugins of a registered marketplace, so each configured skill becomes a plugin
of agx's local marketplace. A Codex launch whose skill plugin is missing runs
that step itself, once, under the same lock. When Codex's `config.toml` (in
`CODEX_HOME`, default `~/.codex`) sets `features.plugins = false`, agx leaves the
setting alone: `install` warns and skips Codex, and Codex launches warn and start
without skills; MCP servers and instructions still apply.

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

Each of these commands also takes `--json` ([scripting](#scripting)). They edit
the YAML fragments for the common cases
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
it, appends it to `remove`. `use` after such an `unuse` only deletes the `remove`
entry, since another rule still adds the definition, so the fragment returns to
its earlier text. `--kind skills|instructions|mcp|bundles` picks one when
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
agx daemon start [--foreground] [--config <path>] [--json]
agx daemon status [--json]
agx daemon stop [--force] [--json]
agx launch list [--json]
agx launch show <launch-id> [--json]
agx launch revoke <launch-id> [--json]
```

Use one daemon per OS user. `daemon start` is optional: a command that needs the
daemon (a managed harness launch, `secret`, `mcp test`, `mcp login`, and `launch`)
starts it in the background when it is not running, as `daemon start` does, and
waits up to 10 seconds for it. Concurrent commands start one daemon between them.
A daemon of another agx build is stopped and replaced, with one line on stderr,
only when it serves the same root and has no active launches; otherwise the
command fails with instructions. A daemon serving another root is never
replaced. `daemon status` and `daemon stop` never start it, and passthrough
harness commands never need it. `daemon.autostart: false`, or `AGX_NO_AUTOSTART=1`
in the environment, turns automatic startup off. The daemon inherits the
environment of the command that starts it and runs in the cache's `runtime`
directory, where `daemon.log` collects its output. `daemon stop`, with or without `--force`, succeeds with
"Daemon is not running." when no daemon answers. Stopping refuses active launches unless
forced: `daemon stop` then fails with error code `conflict` and points to
`daemon stop --force`, as does a command that finds a busy daemon of another
build. Revocation closes MCP access and upstream sessions; it does not kill the
harness. Launch inspection excludes secrets. A graceful wrapper exit releases
its launch; abrupt exits are recovered through lease expiry. `launch list` shows
the thread launches of a `codex app-server` process (mode `thread`) indented
under its launch; `launch show` names a thread launch's parent and thread
(`session`), and the JSON carries `parent_launch_id` and `session_id`.

## Doctor

```text
agx doctor [directory] [--all] [--json]
```

`doctor` checks the setup and prints one row per check, `ok`, `warn`, or `fail`,
followed by the next steps for every warning and failure; it exits 1 when any
check fails. It checks that the configuration loads (and from which files),
that packages are installed with current lockfiles, each built-in harness's
executable and version against the [minimum](#harness-versions), Codex's
plugins feature, agx's marketplace and installed skill plugins (when skills are
configured and Codex is found), and the daemon: running, of this build, and
serving this root. It starts the daemon as other commands do. Through the
daemon it tests the MCP servers selected for the directory (default: the
current one), or every server with `--all`, in parallel with a 20-second limit
each, as `mcp test` does; a server that needs authorization is a warning naming
`agx secret login`. It then lists secret states (environment secrets are checked
against this shell), whether `sops` is on PATH when sops secrets exist, and, for
harness definitions, whether shims are installed in the default directory, on
PATH, and not shadowed. A missing harness is a warning unless its executable is
configured; an older harness is a warning. `doctor` changes nothing besides
starting the daemon.

## Secrets

```text
agx secret login <secret-id> [--no-browser] [--json]
agx secret set <secret-id> --stdin [--json]
agx secret status [secret-id] [--json]
agx secret logout <secret-id> [--json]
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
`set` and `login` reject [sops secrets](configuration.md#sops-secrets) with a
hint to edit them with `sops <file>`. Status returns metadata only: each secret's
kind, whether it is inline in an MCP server or on-demand OAuth, its state, and its
expiry. A sops secret is `ready` once it decrypts and `unavailable` otherwise, with
sops's first error line as a diagnostic; status decrypts it to check, reusing the
cached value while the file is unchanged. Logout clears the secret's
stored credentials and closes upstream sessions of MCP servers using it. It does
not revoke unrelated launches or secrets.

## Shell completion

`agx completion <shell>` prints a completion script for zsh, bash, fish, or
PowerShell. Load it once in the current shell, or add the line to the shell's
startup file:

```sh
source <(agx completion zsh)            # zsh: ~/.zshrc, after compinit
source <(agx completion bash)           # bash: ~/.bashrc; needs bash-completion
agx completion fish | source            # fish: ~/.config/fish/config.fish
agx completion powershell | Out-String | Invoke-Expression   # PowerShell: $PROFILE
```

Completion reads the configuration that `--config` (or `AGX_CONFIG`) selects
and offers its IDs with a short description: MCP servers, skills, instructions,
bundles, secrets, packages, and harnesses wherever a command or `--harness`,
`--base`, or `--kind` takes one. `use` and `unuse` describe each ID by kind and
honour `--kind`. `--scope` offers `user` and `here`, then directories; `--file`
offers YAML files. A configuration that does not load offers nothing. Launch IDs
for `launch show` and `launch revoke` come from a daemon that already runs,
asked with a short timeout; completion never starts the daemon. Harness
commands complete their native arguments as files.

## Output contract

JSON commands produce one versioned document on stdout, and logins a stream of
them ([scripting](#scripting)); diagnostics use stderr.
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
same agx build (version and executable contents), replacing an idle daemon of another build, and fail with actionable instructions instead of printing into a
harness protocol stream. Starting the daemon writes nothing to stdout; its notes go to stderr, a progress
line only on a terminal.

## Scripting

Every command that acts takes `--json`, except the harness entry points, `run`,
the launcher, `config schema`, `completion`, and `help`. Then stdout carries only JSON, one
document per line, and nothing styled:

```json
{"version":1,"data":{"secret_id":"token","logged_out":true}}
{"version":1,"error":{"code":"not_found","message":"unknown MCP server docs"}}
```

`version` is 1; it changes only when a document changes incompatibly, while new
fields may appear in any version. `--json` implies `--yes` and never prompts: a
missing argument is a usage error, and edits are written without showing a diff.
Warnings stay diagnostics on stderr; progress lines and hints appear on stderr
only when it is a terminal. `--json` counts anywhere before `--`, so usage errors
that occur before a command runs, such as an unknown flag, are JSON too.

Exit codes are those of the human output: 0 for success, 1 for operational
failures, 2 for usage errors, and 128 plus the signal number when a signal stops
a command, which then prints an `interrupted` error. A failure prints an error
document instead of `data`, except where a command reports the failure in its
data below.

| Command | `data` |
| --- | --- |
| `config validate` | `{valid, root, files, counts, diagnostics}`: `counts` has `skills`, `instructions`, `mcp`, `secrets`, `harnesses`, `scopes`, `packages`; each diagnostic is `{severity, message, source?: {path, line?}}`. An invalid configuration exits 1 with `valid: false`, empty `files`, `counts: null`, and one error diagnostic. |
| `resolve`, `explain`, `sessions`, `list`, `shims list` | as described with each command |
| `install`, `update` | `{packages, lockfiles, frozen, updated}`: the package IDs installed and locked, and the lockfiles written (checked, with `--frozen`) |
| `mcp`, `skill`, `instructions`, `harness` `add` and `remove`; `use`, `unuse` | `{files, defined, enabled, disabled, removed, scope}`; `mcp add` adds `needs_login` |
| `mcp test` | `{id, transport, server: {name, version}, tools: [{name, description}], state}`; a server that does not answer exits 1 with `state` `needs_auth` or `failed`, `server: null`, and `message` |
| `shims install`, `shims remove` | `{directory, on_path, target, changes}`; each change is a shim (`name`, `file`, `kind`, `target`) with its `action` (`created`, `updated`, `removed`, `unchanged`, `kept`); a kept shim has its `state` |
| `doctor` | `{directory, root, files, checks, summary}`: each check is `{group, name, status, detail, hint?}` with `status` `pass`, `warn`, or `fail`; `group` is `config`, `harness`, `codex`, `daemon`, `mcp`, `secret`, or `shims`; `summary` counts `pass`, `warn`, and `fail`. A failing check exits 1 after the document. |
| `daemon start` | `{started, version, log}` |
| `daemon stop` | `{running, stopping, forced, version}`; `{running: false}` when no daemon was running |
| `daemon status`, `launch list`, `launch show`, `secret status` | the Connect message as protobuf JSON (`GetRuntimeResponse`, `RuntimeView`, `Launch`, `GetSecretStatusResponse`) |
| `launch revoke` | `{launch_id, revoked}` |
| `secret set` | `{secret_id, stored}` |
| `secret logout` | `{secret_id, logged_out}` |

In edit documents, `files` lists each fragment and lockfile written, as `{path,
diff, written}` with a unified diff; it is empty when nothing changed. `defined`,
`enabled`, `disabled`, and `removed` list `{kind, id}`, where `kind` is the
configuration section: `skills`, `instructions`, `mcp`, `bundles`, `harnesses`, or
`packages`. `enabled` and `disabled` name what the command enabled or disabled
even when it already was. `scope` is `{directory, harnesses}`, with `directory`
`null` for everywhere, or `null` when the command changed no scope.

`secret login` and `mcp login` stream events instead, one document per line, as
they happen:

```json
{"version":1,"event":"started","secret":"github","mcp_id":"github","url":"https://github.com/login/oauth/authorize?…","expires_at":"2026-10-18T12:05:00Z"}
{"version":1,"event":"finished","secret":"github","outcome":"success","message":""}
```

`mcp_id` names the MCP server whose URL the login authorizes. `outcome` is
`success`, `failure`, or `expired`; the command exits 0 only on success. An error
before or during the login is an error document as the last line. With `--json`,
the browser opens only when stdout is a terminal and `--no-browser` is not given,
so a script reading the stream opens the URL itself.

| Code | Meaning |
| --- | --- |
| `usage` | invalid arguments or flags (exit 2) |
| `config_invalid` | the configuration does not load, or an edit would make it invalid; `source` locates the cause when known |
| `not_found` | an unknown ID, launch, package, or file |
| `conflict` | the target exists, is still referenced, or is busy: an ID already defined, a definition other entries use, a running daemon, a lockfile that `--frozen` would change |
| `read_only` | a file agx may not write, a definition inside a package, or a built-in |
| `invalid_request` | a request agx or the daemon cannot carry out as given |
| `failed_precondition` | the target does not support the operation, such as logging in to a stdio server or setting a sops secret |
| `needs_login` | the operation needs an OAuth login first |
| `daemon_unavailable` | no daemon answers, or it stopped |
| `version_mismatch` | the daemon runs another agx build and cannot be replaced |
| `network` | fetching a package failed |
| `timeout` | the daemon or an upstream did not answer in time |
| `interrupted` | a signal or cancellation stopped the command |
| `internal` | anything unclassified, which is a defect |
