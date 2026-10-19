# Share MCP server processes across launches on request

Status: proposed.

Each launch gets its own stdio MCP server processes, and each harness session
on a launch gets another. That is the safe default, but it is costly for servers
whose value is warm state: fff, a fast file search server, indexes the directory
and keeps frecency, so a fresh process per launch rebuilds the index and forgets
what the user opened. One warm process per project would serve better. MCP
server definitions gain an opt-in `share` setting.

## Current model

- `PrepareLaunch` (and thread launches, and `revive` for restored launches)
  builds one `mcpruntime.Endpoint` per MCP server of the launch's snapshot,
  native and on-demand, with the definition, the snapshot's secrets, and the
  launch's environment inputs. A stdio definition without `cwd` gets the launch
  directory. Native servers are handed to the harness as
  `/launch/<id>/mcp/<server>`.
- `serveMCP` checks the launch grant and that the server is selected for the
  launch, then passes the request to its endpoint.
- A stdio endpoint starts **one process per MCP session**: every `initialize`
  without `Mcp-Session-Id` resolves the environment's injected values, spawns
  the command, and relays messages unchanged between a streamable HTTP server
  transport and the process's stdio. An endpoint holds at most 8 sessions;
  sessions idle for 30 minutes are closed; closing a session closes the
  process's input, then terminates and kills it after grace periods. Undeliverable
  server requests are answered with an error. An HTTP endpoint proxies to the
  upstream URL per session, with credentials resolved per request.
- The endpoints close when the launch ends; `Reset` ends their sessions when a
  secret they use changes or the user logs in again; daemon shutdown closes, and
  after a deadline kills, them.
- `agx mcp call` ([ADR 0022](0022-on-demand-mcp-calls.md)) keeps a pool of
  client sessions keyed per launch and server, on the launch's endpoint, or per
  CLI scope outside a launch, keyed by server and a digest of the definition,
  secrets, and forwarded inputs, on an endpoint the pool owns. Pooled sessions
  close after five idle minutes, when their launch ends, and at shutdown.

So a project with three launches and an `agx mcp call` from a shell runs at
least four copies of a stdio server.

## Decision

An MCP server definition gains `share: none | directory | global` (default
`none`):

```yaml
mcp:
  fff:
    command: [fff-mcp]
    share: directory
```

- `none`: today's behaviour.
- `directory`: one upstream process per **share key** of (server, resolved
  launch directory, digest of the resolved definition, the identities of the
  secrets it uses, and the values of the environment inputs it reads). Every
  launch, thread launch, and `agx mcp call` whose key matches uses that process.
- `global`: the same key without the directory. Only for servers that do not
  depend on their working directory; a `global` stdio server without an explicit
  `cwd` runs in the daemon's home directory, not a launch's, and `agx config
  validate` warns when the command or environment references the launch
  directory.

Launch endpoints stay per launch: grants, selection, `/launch/<id>/mcp/<server>`
URLs, and runtime views are unchanged. A shared endpoint replaces only the
per-session process start: each harness session (and each pooled CLI session)
becomes a **client session** of a daemon-owned **shared server**, which holds
one upstream MCP session to the process.

## Multiplexing

A stdio server speaks to exactly one MCP client. The shared server therefore
stops relaying bytes and multiplexes: it parses JSON-RPC, owns the upstream
session, and maps client sessions onto it.

- **initialize**: the first client's request initializes upstream with agx's own
  client capabilities (see below), at the newest protocol version both support.
  Every client gets the cached upstream result, with the protocol version
  negotiated per client from the versions the bridge already supports. Clients'
  `notifications/initialized` are absorbed after the first.
- **Requests** (`tools/*`, `resources/list`, `resources/templates/list`,
  `resources/read`, `prompts/*`, `completion/complete`, `ping`) are forwarded
  with request IDs remapped to upstream-unique IDs and mapped back on the
  response. `notifications/cancelled` from a client is forwarded only for that
  client's own requests.
- **Progress**: `progressToken`s are remapped like IDs, and
  `notifications/progress` is routed to the client that owns the token.
- **List changes** (`tools/list_changed`, `resources/list_changed`,
  `prompts/list_changed`) fan out to every client.
- **Resource subscriptions**: the shared server reference-counts subscriptions
  per URI, subscribes upstream on the first and unsubscribes on the last, and
  routes `notifications/resources/updated` to the subscribed clients.
- **Logging**: `logging/setLevel` is not forwarded; upstream runs at the most
  verbose level any client asked for, and each client receives
  `notifications/message` at or above its own level.
- **Server requests**: agx declares no `sampling`, `elicitation`, or `roots`
  capability upstream, so a conforming server does not ask. Requests that come
  anyway are answered with an error naming the server as shared: no single
  client owns the session, and choosing one (the caller of the request in
  flight, the oldest client) would leak one launch's model or user prompt into
  another's work. `roots/list` would also be wrong for `global`, and is
  redundant for `directory`, whose process already runs in the root.
- Anything else the shared server does not know is rejected with "method not
  supported for shared MCP servers", rather than forwarded blindly.

Servers that keep per-session state (a selected workspace, an authenticated
user, a conversation) see one session for all clients and will mix them up. agx
cannot detect this; it is why sharing is opt-in and documented as "only for
servers whose state is the same for every client".

## Lifetime

- A shared server starts on its first client session and is reference-counted
  by client sessions: harness sessions on launch endpoints and pooled CLI
  sessions. When the last ends (a launch ending closes its endpoints and thus
  its client sessions), it stays warm for an idle period like the ADR 0022 pool,
  about five minutes, then its process gets the usual input close, termination,
  and kill.
- The per-endpoint limit of 8 sessions counts client sessions; a shared server
  takes a separate limit of 64 client sessions.
- A config change that alters a share key's inputs simply yields a new key; the
  old server drains as its launches end, since launches keep their snapshots.
  `Reset` after a secret change or login stops the shared servers using that
  secret; their clients reconnect as they do today.
- When the process crashes, in-flight requests fail (`mcp_failed` for
  `agx mcp call`, an internal error response for harnesses), client sessions are
  ended so harnesses reinitialize, and the next session starts a new process.
- Daemon shutdown closes shared servers with the launches. Restored launches
  ([ADR 0016](0016-launches-survive-daemon-restarts.md)) start new shared
  servers on their next session, as they start endpoints today.

## Security

Scoping is unchanged: a launch reaches only the servers its snapshot selects,
through its grant, and `agx mcp call` follows ADR 0022's rules. Sharing only
joins requests that already passed those checks. The share key holds secret
identities and the definition digest, so different credentials, commands, or
environments never share a process; environment input values enter the key
only as a digest. Rotating a secret's value under the same identity is covered
by `Reset`. agx serves a single user, and every process of a shared server runs
as that user, as today; sharing introduces no cross-user boundary. What it does
introduce is cross-launch visibility through the server's own state (frecency,
caches, logs), which is the point and why the user opts in per server.

## HTTP servers

`share` is stdio-only and rejected on HTTP definitions by validation. An HTTP
upstream already multiplexes its own sessions, keeps its state server-side, and
costs nothing per launch beyond an HTTP session; joining clients onto one
upstream session would only add the multiplexing caveats. Connection reuse to
the same host is already handled by the HTTP client.

## Alternatives

- Always share: breaks servers with per-session state and server requests,
  silently, for every user.
- Share stdio servers by default and let users opt out: the failure is subtle
  (one launch's state appearing in another), so users would not know to opt out.
- A per-harness setting: whether a server can be shared depends on the server,
  not on the harness, and a per-harness switch would make one process's
  behaviour depend on which harness happened to start it.
- Starting servers outside the daemon (a user-run fff shared over HTTP): works
  today but moves command, environment, and secrets out of agx's configuration.

## Effort

About a week:

1. `share` in the schema, validation, `explain`, `resolve`, and `mcp list`:
   0.5 days.
2. The multiplexer (initialize caching, ID and progress-token remapping,
   cancellation, fan-out, subscriptions, logging levels, refusals) with tests
   against a fake stdio server: 2.5 days.
3. Shared server registry with share keys, reference counting, idle expiry,
   crash handling, `Reset`, and shutdown; endpoints and the CLI pool using it:
   1.5 days.
4. Runtime views listing shared servers and their clients, docs, end-to-end
   tests with several launches: 1 day.

## Open questions

- Whether the multiplexer should also serve `share: none` stdio servers, so one
  code path parses messages; that changes today's byte-level relay, which
  forwards protocol features agx does not yet know.
- How long the idle period should be for servers that index, and whether it
  should be configurable per server (`share_idle`).
- Whether `directory` should key on the repository root rather than the launch
  directory, so launches in subdirectories of one project share.
- Whether to forward `roots/list` for `directory` servers by answering it from
  the daemon with the shared directory, instead of refusing.
- How runtime views attribute a shared server's state and failures to launches.
