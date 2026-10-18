# Per-thread resources for multi-session hosts

Status: accepted.

Hosts such as T3 Code do not start one harness process per thread. They start
one `codex app-server` (in the user's home) and create every thread through it,
each with its own working directory, and they drive Claude Code through the Agent
SDK, which sends its own system prompt append on standard input. A launch
resolved once per process gave every Codex thread the home directory's
resources, and Claude lost agx instructions.

## Codex app server: one launch per thread

`agx codex app-server` on standard input and output relays newline-delimited
JSON-RPC between the host and the real app server. Other transports (`--listen
ws://…`, `unix://`) keep one per-process launch, and app-server subcommands stay
`passthrough`.

The process's launch carries no snapshot resources: no MCP servers, no
instructions, no plugin toggles. Every `thread/start`, `thread/fork`, and
`thread/resume` gets a **thread launch** under it for the thread's directory
(`cwd`, else the forked or resumed thread's directory, asked of the server with
`thread/read` when the relay has not seen it, else the process's), with its own
MCP endpoints and grant. The relay deep-merges the launch into the request's
`config`: `mcp_servers.<name> = {url, http_headers: {Authorization: "Bearer
<grant>"}}` and `plugins."<plugin>@agx-managed".enabled` per installed skill
plugin. Keys the host set win; new keys come first, because Codex applies a
dotted key such as `mcp_servers.x` only against the keys before it. Instructions
are prepended to `developerInstructions`. The response binds the launch to the
thread ID; an error response ends it.

`turn/start` sets `disabledPluginIds` to the plugins the thread's directory does
not select, merged with the host's list; a turn moving to another directory
switches plugins at once, while its MCP servers change on the next resume.
`thread/resume` gets a fresh launch with new MCP servers and plugins, and ends
the old one, but cannot change instructions: Codex keeps those in the thread's
history. Subagents (`thread/started` with a `parentThreadId`) share their
parent's launch. `thread/closed`, `thread/archived`, and `thread/deleted` end
the thread's launch once no thread uses it.

Codex lists plugin skills in `skills/list` as configured for the process, so the
relay marks the skills of plugins a directory does not select `enabled: false`.
Disabling every agx plugin in the process's arguments was rejected: Codex then
leaves agx skills out of `skills/list` altogether, and hosts could not offer a
directory's selected skills.

The daemon adds `ResolveThread`, which resolves a directory for a thread of a
running launch and names the plugins it keeps disabled, and
`PrepareThreadLaunch`, which creates the thread launch. Thread launches share
their parent's lease and lifecycle token, renew to report their thread ID, and
end with their parent. `agx launch list` shows them under it; launch records
name the thread as the launch's session, so `agx sessions` resumes it with the
same definition.

## Claude Agent SDK: merge the initialize request

The SDK runs Claude Code with `--input-format stream-json` and no `-p`, and its
first input line is a `control_request` `initialize` whose `appendSystemPrompt`
replaces `--append-system-prompt-file`. Such launches are `print` mode. When
instructions are selected, the wrapper copies standard input line by line and,
in the first `initialize` request only, prepends them (and a blank line) to a
string `appendSystemPrompt`, keeping fields it does not know. A request without
one keeps the flag's append; every other line passes byte for byte.

Rejected: a plugin hook adding instructions as session context, which reaches
the model as a message rather than the system prompt and repeats on resume.
