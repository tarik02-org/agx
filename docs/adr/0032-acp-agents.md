# Relay ACP agents, giving each session its directory's resources

Status: accepted. Extends ADR 0015 to the Agent Client Protocol.

The [Agent Client Protocol](https://agentclientprotocol.com) connects clients
(editors, T3 Code) to coding agents over newline-delimited JSON-RPC on standard
input and output. One agent process serves many sessions, and each session
names its working directory and MCP servers in its setup request. Supporting it
scopes resources for any ACP agent without an adapter per agent.

## A built-in to base definitions on

`acp` is a built-in harness that only definitions run: ACP agents share no
executable name, so a definition sets `base: acp` and an absolute `executable`.
`agx <definition>` is then the command a client runs as the agent. Many agents
are also terminal CLIs that start their ACP server with a subcommand or flag
(`agy acp`, `opencode acp`, `qwen --acp`), and clients such as T3 Code pass it
themselves. The definition's `settings.serve` lists those leading arguments: a
launch that starts with them is a thread server, and any other launch passes
through, so one definition and its shim serve both the client and a terminal.
Without `serve`, every launch is a thread server, which suits dedicated ACP
executables (`claude-agent-acp`) and clients that pass no arguments; a
definition's `args` can then start the server. A help or version probe always
passes through. The launcher offers only definitions with `serve`, since a
terminal cannot drive a bare thread server; `agx doctor` checks each
definition's executable instead of the bare base. ACP sessions belong to the
client, so launches are not recorded for `agx sessions`.

`serve` matches a prefix of the caller's arguments rather than reading the
definition's own `args`, because those are prepended to every launch and could
not tell a terminal launch from a server one. In a served launch the
definition's `args` go after the serve arguments instead of before them:
agents parse flags per subcommand, so the server's own flags must follow it
(`agy --disable-telemetry acp` is rejected). It lives in `settings`, the
place for base-specific options, instead of a field every harness would carry.

## The relay

The Codex app-server relay's core moved into a shared package: copying lines,
preparing intercepted requests off the client loop, matching responses, the
relay's own requests, and the thread launches it binds and ends. A protocol
names the requests it rewrites and what their responses settle. Codex keeps its
rules; ACP adds its own, so each protocol is one file of rules over the same
lifecycle.

`initialize` settles the protocol version and the agent's capabilities.
`session/new`, `session/fork`, `session/load`, and `session/resume` get a thread
launch for the request's `cwd` (else the session's known directory, else the
process's), bound to the session ID from the result or the request; a failed
response ends it. `session/close` and `session/delete` end the session's launch,
and the process's exit ends the rest. Everything else passes byte for byte,
`session/prompt` included unless it carries pending instructions, so a
`session/cancel` behind a prompt cannot overtake it.

Each resource takes the form the agent advertises, and agx never sends a field
the agent did not:

| Resource     | Advertised                                                              | Otherwise                                                 |
| ------------ | ----------------------------------------------------------------------- | --------------------------------------------------------- |
| MCP servers  | HTTP servers appended to the client's, with a per-session grant         | stdio servers that bridge to them (below)                 |
| Instructions | a `systemPrompt` text block after the client's, in its mode or `append` | a text block before a new session's first prompt          |
| Skills       | none in ACP                                                             | not applied, with a warning per session that selects some |

Antigravity advertises no standard system prompt but an extension of its own:
`_meta.antigravity.systemPrompt.append` in its capabilities, and the same path
in a session setup's `_meta` for text appended to its system prompt. agx
appends the instructions there, after the client's text and beside the rest of
the client's `_meta`, for every setup, as it would to a standard system
prompt. One vendor's extension lives in the capability table beside the
version differences instead of a per-agent adapter; the standard field wins
when an agent advertises both.

A client server of the same name wins. Forked, loaded, and resumed sessions
carry instructions in their history, so without a system prompt they get none.
An agent that refuses the system prompt it advertises with invalid params, as
agents implementing an earlier draft of the RFD do (a bare content array with
`systemPromptMode`), gets the setup request again without it, and the client
sees only that response; the process's later sessions take instructions as if
the agent had no system prompt. The capability cannot tell the drafts apart, so
the refusal is the only signal.

## MCP servers over stdio

Version 1 requires every agent to accept stdio MCP servers; HTTP ones are
optional, and agents such as Mistral Vibe take none. Such an agent gets each
server as a stdio server whose command is the relaying agx's own executable
with `mcp bridge <url>`, and whose environment carries the session's grant as
`AGX_MCP_GRANT`, since command lines are visible to other processes. The bridge
copies MCP messages between its standard input and output and the daemon's
HTTP endpoint unchanged, so the agent negotiates with the server itself, and
the grant's revocation ends its access as it ends an HTTP server's. Version 2
makes stdio a capability too and types its entries; an agent with neither
transport gets no agx MCP servers, with one warning per process.

The bridge uses the MCP SDK's transports. Its HTTP connection opens the stream
for messages outside a request's response only for the SDK's own client, so
such messages, list change notifications among them, do not reach the agent.

## Versions

Only what differs between versions in these messages is per version: where the
capabilities are (v1 `agentCapabilities.mcpCapabilities.http` and
`.sessionCapabilities.systemPrompt`; v2 `capabilities.session.mcp.http` and
`.systemPrompt`) and which methods open or close a session (v2 has no
`session/load`). Version 2 is a draft; supporting it costs one table entry, and
a later revision changes that entry. A version agx does not know passes through
unchanged with a warning.

Rejected: the `coder/acp-go-sdk` types, which drop fields they do not know when a
relay re-encodes a message and lag the specification (no `systemPrompt`); a full
ACP client inside agx, which would duplicate the editor's job; per-agent
adapters, which capability negotiation makes unnecessary; proxy chains and MCP
over ACP, still drafts that no agent agx was tested with speaks, where the
stdio bridge already reaches every agent that MCP over ACP would; resolving
resources for `additionalDirectories`, whose rules would conflict with the
session's own directory.
