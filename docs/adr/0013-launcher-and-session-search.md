# Open a launcher with session search when agx runs bare

Status: accepted.

`agx` without arguments, with stdin and stdout on a terminal, opens a full-screen
launcher; otherwise it prints help, as before. The launcher has one search field
over two groups: harnesses to start a new session in the current directory
(built-ins, then definitions), and existing sessions to resume. Enter starts or
resumes; Tab toggles the session scope between **here** (sessions whose
directory is the current directory or below it) and **all**.

## Sessions from the harnesses' stores

Sessions come from the harnesses' own stores, so sessions started without agx
appear too. The adapter interface of [ADR 0012](0012-harness-adapters-and-shims.md)
gains two methods; every built-in has a store, so they are part of `Adapter`
rather than an optional interface:

```go
// Sessions lists the sessions in the harness's own store; files caches parsing.
Sessions(ctx context.Context, files SessionFiles) ([]Session, error)
// Resume returns the native arguments that resume a session in its directory.
Resume(session Session) []string

type Session struct {
	ID, Directory, Title, Message string // Title and Message are excerpts
	Started, Updated              time.Time
}
```

| Harness | Store | Title | Resume |
| --- | --- | --- | --- |
| codex | `$CODEX_HOME/sessions/YYYY/MM/DD/rollout-*.jsonl` | `thread_name` in `session_index.jsonl` | `resume <id>` |
| claude | `$CLAUDE_CONFIG_DIR/projects/<escaped>/<id>.jsonl` | `custom-title`, else `ai-title`, else `summary` | `--resume <id>` |
| pi | `$PI_CODING_AGENT_SESSION_DIR`, the settings' `sessionDir`, else `$PI_CODING_AGENT_DIR/sessions` | `session_info` name | `--session <id>` |

Each falls back to its default home (`~/.codex`, `~/.claude`, `~/.pi/agent`).
The directory comes from the session's own metadata (Codex `session_meta`,
Claude's entries' `cwd`, Pi's header), never from the escaped folder name, and
is canonicalized like configuration scopes, so a session recorded through a
symlink lists under its target and one in a deleted directory still lists (and
fails clearly when resumed). Subagent sessions (Codex `thread_source: subagent`,
Claude sidechains and `agent-*` files) are not listed. The first user message
skips harness-injected content: Codex AGENTS.md and environment items, Claude
meta entries and content starting with `<`.

Parsers read at most the first 1 MiB of a file and, for longer files, the last
64 KiB, where a later rename lands; histories reach tens of megabytes. Title and
message are whitespace-collapsed excerpts (120 and 200 characters); nothing
else of a transcript is kept. A file that cannot be parsed is skipped and
counted. The same ID in two files keeps the more recently modified one. Stores
are only read.

The index is cached in `<cache>/sessions/index.json`, keyed by path
and valid while a file's size and modification time are unchanged; files are
parsed concurrently, and one the cache still holds is not even opened. On real
stores of 72 session files (51 listed, 320 MB) a listing takes 0.1 s with an empty cache and 0.03 s with a current one.

## Launch records

The wrapper records each managed launch in `<state>/launch-records.jsonl`, a log
it appends one line to when the launch starts and one when it finishes, under a
lock file held only for the append, so recording costs the same however many
records there are and concurrent launches barely wait for each other. Appends
are not synced: losing the last line in a crash only makes a session resume with
its built-in. A listing folds the log into one line per record and rewrites it,
pruned. The lock is `launch-records.jsonl.lock`. `<state>` is the XDG state directory
(`$XDG_STATE_HOME/agx`, by default `~/.local/state/agx`, `~/Library/Application Support/agx` on macOS, and
`%LOCALAPPDATA%\agx` on Windows), not the cache, because records cannot be
rebuilt and a cache wipe must not lose them; `AGX_STATE` overrides it, and a
non-default `AGX_CACHE` or `--cache` keeps state in `<cache>/state` so isolated
runs stay isolated. Each record holds the definition name, its built-in, the
canonical directory, and the start and finish times. A launch the launcher started as a resume records that
session's ID directly. A new Claude launch starts its session with
`--session-id` and a UUID agx chooses, and its record lists exactly that
session: no session matches it by time, so one started later in the launch
(`/clear`) resumes with the built-in. Otherwise a session belongs to the latest launch of the
same built-in and directory whose window contains the session's start: from
5 seconds before the launch started to 5 seconds after it finished, or 24 hours
after it started when the wrapper died without finishing. Launch duration rather
than a fixed window is needed because Claude stamps a session at its first
message, and one launch can create several sessions (`/clear`, `/new`). Both
times come from the same clock; the 5 seconds absorb the gap between the
wrapper's reading and the harness's. Matched IDs are stored in the record, so a
match survives later launches.

A recorded session resumes with its definition while the definition exists with
the same built-in; others resume with their built-in. Ctrl-H picks another
harness of the same built-in. Listing drops record IDs whose sessions are gone
and records left without sessions once their window has closed, except for a
store that failed to list.

## Resuming and the launcher

Resuming changes to the session's directory and runs the normal harness path, as
`agx <name> <resume arguments>` would, so the directory's skills, MCP servers,
and instructions apply and the launch classifies as `resume`. Pi gains a
`resume` mode for `--session`, `--continue`, `--resume`, and `--fork`; as with
Claude, print and RPC runs keep their mode. The launcher exits and restores the
terminal before the harness starts, which then owns it.

Search matches every whitespace-separated term fuzzily (`sahilm/fuzzy`) within
one field: harness, title, first message, or directory. Matching across fields
let scattered letters match nearly everything. Without a query sessions are
newest first; with one, best match first. The launcher uses Bubble Tea and
Bubbles' text input with the existing lipgloss styling and honours `--color` and
`NO_COLOR`; unstyled, a `>` marks the selection. `agx sessions [query] [--all]
[--json]` lists the same results without the launcher.

Full-transcript search, recent directories, launch status, and authentication
warnings on the start screen are deferred.
