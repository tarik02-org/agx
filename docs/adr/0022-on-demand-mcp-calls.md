# Call MCP servers on demand from the shell

Status: accepted.

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
             [--save-dir <dir>] [--timeout 60s] [--all] [--json]
agx mcp types [<server> ...] [--out <dir>] [--all]
```

- `tools` without a server lists each selected server with its state, load
  mode, description, and tool names. With a server it prints each tool's
  signature, compact and TypeScript-like from its `inputSchema`, such as
  `search_issues(query: string, status?: "open" | "closed", limit?: number)`,
  with the first line of its description; `--schema` adds the raw `inputSchema`
  and `outputSchema`.
- `call` takes arguments as `key=value` pairs, coerced by the tool's schema:
  enums, numbers, integers, booleans, `null`, a single value into a one-item
  array, and objects or arrays given as JSON literals. `key=@file` reads a
  file's content, `@@` escapes a literal `@`, and `--raw-strings` turns coercion
  off. `--args` takes a whole JSON object, or `-` to read it from standard input.
  There is no function-call syntax.
- Text content stays inline. Binary content (images, audio, embedded binary
  resources) is always written to files: into `--save-dir` when given, otherwise
  into a new temporary directory per call (`agx-mcp-*`). Human output prints a
  line per file, `[image/png, 34 KB] /tmp/agx-mcp-123/1.png`. `--json` prints
  `{"version":1,"data":{"mcp_id","tool","is_error","content":[…],"structured_content"}}`,
  where content items are as the server sent them, except that binary data is
  replaced by its `path` and `size`.
- Exit codes: 0 on success, 1 on failure or a result with `isError` (the
  document still prints), 2 on usage errors. An unknown tool or invalid
  arguments fail with `invalid_request` and the closest tool or argument names;
  a server needing a login hints `agx secret login <id>`. Failures inside the
  server produce a message and a nonzero exit status.
- `types` emits TypeScript declarations with a typed function per tool, plus a
  small dependency-free ESM client (see Code mode).

The commands work for every server, not only on-demand ones, so a person can
also try a tool without a harness.

## Which servers a call reaches

Inside a launch, the harness environment carries the launch's ID
(`AGX_LAUNCH_ID`) next to its grant (`AGX_MCP_GRANT`). The CLI authenticates
with the grant and reaches only the servers of that launch's snapshot, native and
on-demand, through the endpoints the launch already holds, so injected values
and environment inputs are the launch's own. Thread launches get their own ID
and grant as they do for native MCP traffic.

Outside a launch, the CLI uses the management token and resolves the current
directory's scopes, as a launch there would. Environment inputs are forwarded as
for `agx mcp tools`: only the variables the servers' values read.

`--all` reaches every configured server, inside a launch too, through the
management token. agx serves a single user, and any process of that user can
read the management token from the daemon's discovery file, so strict scoping
inside a launch would not stop anything; it only keeps the default list short.

## RPCs

`RuntimeService` gains `ListTools` and `CallTool(launch_id?, directory, all,
mcp_id, tool, arguments_json, environment_inputs)`, using the same endpoint
client flow as launches. Both accept either the management token, scoped by directory or
`all`, or a launch grant with its launch ID, scoped to that launch. Errors use standard Connect codes, including `Unauthenticated` when a login
is needed and `FailedPrecondition` for server failures. Tool execution and credentials stay inside the daemon; the
CLI receives only results. This replaces the API rule that there is no
client-facing `CallTool`, which existed to keep credentials out of clients, and
that property holds.

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
`agx mcp call`. `explain`, `resolve`, `mcp list`, and `mcp tools` show the load
mode.

[ADR 0030](0030-session-mcp.md) adds an always-present `agx` MCP server for
discovery and calls, alongside these shell commands. It replaces the original
choice to require user-authored instructions for agents to find on-demand servers.
No generated instruction or skill listing every server is needed.

## Pooled sessions

The daemon keeps client sessions per launch, or per CLI scope outside a launch,
and server, closed after about five idle minutes, when the launch ends, and when
the daemon shuts down, so repeated calls neither respawn stdio servers nor
initialize again. Sessions sit on the existing endpoints, so their session
limits, idle expiry, and reset on login apply; a call whose pooled session broke
reconnects once.

## Code mode

`agx mcp types [server...] [--out dir]` writes `agx-mcp.d.mts`, a typed function
per tool from its input and output schemas, and `agx-mcp.mjs`, a dependency-free
ESM client that calls the daemon directly with the same `CallTool` RPC as
`agx mcp call`: with `AGX_MCP_GRANT` and `AGX_LAUNCH_ID` inside a launch, or the
management token from the daemon's discovery file outside one. Agents can then
write node, bun, or deno scripts that chain calls and filter results before they
reach the context, without agx embedding a JavaScript runtime. Without `--out`
it prints the declarations.

## Phases

1. `ListTools`, `CallTool`, `agx mcp tools`, `agx mcp call`,.
2. `load`, `description`, launch identity in the harness environment, and
   adapters skipping on-demand servers.
3. Pooled sessions, schema-driven coercion, `@file` arguments, binary content in
   files, error hints.
4. `agx mcp types` and the ESM client.

## Out of scope

From mcporter's feature set, agx skips: function-call syntax
(`server.tool(a: 1)`), recording and replaying calls, ad-hoc servers given on the
command line, importing other tools' server configuration, and a credential
vault of its own. Generated client code is limited to the declarations and the
client of Code mode. Elicitation requests are rejected with a hint to call the
tool from a harness that supports them. Resources and prompts may follow later.

Rejected: leaving shell calls to mcporter, which duplicates server definitions
and credentials outside agx's scoping; having the CLI connect to launch
endpoints itself, which spawns a stdio server per call; per-harness defaults
for on-demand loading, which make the same configuration behave differently
across harnesses; strict scoping of `--all` inside a launch, which the shared
management token would not enforce.
