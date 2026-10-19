# Call MCP servers on demand from the shell

Status: proposed.

Every MCP server a launch selects lands in the harness's tool list, and its tool
definitions cost context in every request whether the agent uses them or not.
Servers used now and then (a ticket tracker, a cloud console, a docs search) are
better reached on demand: the agent learns they exist, looks up a tool when it
needs one, and calls it from its shell. Today that takes a separate client such
as mcporter, with its own server list, credentials, and stdio processes. agx
already owns all three, so it offers the calls itself.

## Commands

```sh
agx mcp tools [<server>] [--schema] [--all] [--json]
agx mcp call <server>.<tool> [key=value ...] [--args <json|->] [--raw-strings]
             [--timeout 60s] [--all] [--json]
```

- `tools` without a server lists each selected server with its state, load
  mode, description, and tool names. With a server it prints each tool's
  signature, compact and TypeScript-like from its `inputSchema`, and
  description; `--schema` adds the raw `inputSchema`.
- `call` takes arguments as `key=value` pairs, coerced by the tool's schema
  (enums, numbers, booleans, a single value into an array), `key=@file` for a
  file's content with `@@` escaping a literal `@`, or a whole JSON object with
  `--args`; `--raw-strings` turns coercion off. Human output prints text content
  and summarizes other kinds. `--json` prints
  `{"version":1,"data":{"mcp_id","tool","is_error","content":[…],"structured_content"}}`.
- Exit codes: 0 on success, 1 on failure or a result with `isError` (the
  document still prints), 2 on usage errors. An unknown tool or invalid
  arguments fail with `invalid_request` and the closest tool names; a server
  needing a login hints `agx secret login <id>`. Failures inside the server use
  a new `mcp_failed` code in the closed JSON error-code set.

The commands work for every server, not only on-demand ones, so a person can
also try a tool without a harness.

## Which servers a call reaches

Inside a launch, the harness environment carries the launch's ID
(`AGX_LAUNCH_ID`) next to its grant (`AGX_MCP_GRANT`). The CLI authenticates
with the grant and reaches only the servers of that launch's snapshot, through
the endpoints the launch already holds, so injected values and environment
inputs are the launch's own. Thread launches get their own ID and grant as they
do for native MCP traffic.

Outside a launch, the CLI uses the management token and resolves the current
directory's scopes, as a launch there would; `--all` reaches every configured
server. Environment inputs are forwarded as for `agx mcp test`: only the
variables the server's values read.

## RPCs

`RuntimeService` gains `ListTools` and `CallTool(launch_id?, directory, mcp_id,
tool, arguments_json, environment_inputs)`, built on the client flow of
`TestMcpServer`. Both accept either the management token, scoped by directory,
or a launch grant with its launch ID, scoped to that launch. Errors carry the
usual `ErrorDetail` reasons. Tool execution and credentials stay inside the
daemon; the CLI receives only results. This replaces the API rule that there is
no client-facing `CallTool`, which existed to keep credentials out of clients,
and that property holds.

## On-demand servers

An MCP server definition gains `description` and `load: native | on_demand`
(default `native`):

```yaml
mcp:
  jira:
    url: https://mcp.atlassian.example/v1
    description: Jira issues and Confluence pages for the platform team
    load: on_demand
```

`load: on_demand` behaves the same in every harness. Many harnesses already
defer loading tool details; the point is to keep the server out of the agent's
tool list entirely. Adapters leave on-demand servers out of the harness
configuration, while the launch still holds their daemon endpoints for
`agx mcp call`. `explain`, `resolve`, and `mcp list` show the load mode.

When a launch selects on-demand servers, agx appends a generated instruction
listing them (`id: description`) and how to use `agx mcp tools <id>` and
`agx mcp call <id>.<tool> … --json`. A server without a description is listed
by ID alone.

## Pooled sessions

The daemon keeps client sessions per launch, or per CLI scope outside a launch,
and server, closed after about five idle minutes, so repeated calls neither
respawn stdio servers nor initialize again. Sessions sit on the existing
endpoints, so their session limits, idle expiry, and reset on login apply.

## Phases

1. `ListTools`, `CallTool`, `agx mcp tools`, `agx mcp call`, and `mcp_failed`.
2. `load`, `description`, launch identity in the harness environment, adapters
   skipping on-demand servers, and the generated instruction.
3. Pooled sessions, schema-driven coercion, `@file` arguments, error hints.

## Out of scope

From mcporter's feature set, agx skips: generated client code, function-call
syntax (`server.tool(a: 1)`), recording and replaying calls, ad-hoc servers
given on the command line, importing other tools' server configuration, and a
credential vault of its own. Elicitation requests are rejected with a hint to
call the tool from a harness that supports them. Resources and prompts may
follow later.

## Open questions

- Whether `tools` and `call` from a launch should also reach servers outside
  its snapshot when the user passes `--all`, or stay strictly scoped.
- How large results (images, embedded resources) print in human output, and
  whether `call` should write binary content to files.
- Whether the generated instruction should become a skill for harnesses that
  load skills on demand, keeping even the server list out of context.

Rejected: leaving shell calls to mcporter, which duplicates server definitions
and credentials outside agx's scoping; having the CLI connect to launch
endpoints itself, which spawns a stdio server per call; per-harness defaults
for on-demand loading, which make the same configuration behave differently
across harnesses.
