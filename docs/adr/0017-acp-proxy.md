# Proxy ACP v1 agents, applying resources per capability

Status: deferred. Not implemented.

The [Agent Client Protocol](https://agentclientprotocol.com) connects clients
(editors, T3 Code) to coding agents over JSON-RPC, with sessions that carry their
own working directory and MCP servers. Supporting it lets agx scope resources for
any ACP agent, including the [registry](0018-harness-version-manager.md)'s, without
an adapter per agent.

## agx as a proxy

`agx acp <harness>` runs the agent's ACP command behind a stdio proxy, like the
Codex app-server relay of [ADR 0015](0015-per-thread-resources-for-multi-session-hosts.md).
agx does not implement a client: it rewrites a few messages and forwards the rest
byte for byte.

- `initialize`: record the agent's capabilities and the negotiated version.
- `session/new`, `session/load`, `session/resume`, `session/fork`: resolve the
  session's `cwd` (and `additionalDirectories`) into a snapshot, prepare a
  per-session launch under the process launch, and inject its resources.
- `session/close`, `session/delete`: finish that session's launch. Process exit
  ends all of them.

Only protocol version 1 is supported. A different negotiated version passes
through without injection and with a warning. ACP v2 is a set of draft RFDs whose
changes mostly concern what the client renders, which agx forwards untouched; its
session setup differs in field paths (`capabilities.session.*`, `session/resume`),
so supporting it later is an adapter over the same rewrite, added once agents ship
it.

## Per capability

Each resource uses the best mechanism the agent advertises, else a fallback.

| Resource | Preferred | Fallback |
| --- | --- | --- |
| MCP servers | `mcpCapabilities.acp` ([MCP over ACP](https://agentclientprotocol.com/rfds/mcp-over-acp)): `acp:` servers relayed by agx to the daemon, with no URL or token | `mcpCapabilities.http`: HTTP endpoints with a per-session grant in `headers` |
| Instructions | `sessionCapabilities.systemPrompt` ([client system prompt](https://agentclientprotocol.com/rfds/client-system-prompt)) in `append` mode, merged after any client prompt | the native adapter's mechanism for built-ins; otherwise a context block prepended to the first `session/prompt` |
| Skills | none in ACP | the native adapter's plugins for built-ins; otherwise not applied, reported by `agx doctor` |
| Extra directories | `additionalDirectories`, from the snapshot's scope | none |

The proxy never sends a field the agent did not advertise, as the RFDs require.
When an agent advertises neither `acp` nor `http` MCP support, MCP servers are not
applied and `agx doctor` says so. Override-mode system prompts are never used:
agx adds guidance, it does not replace the agent's.

Draft RFD features (MCP over ACP, client system prompt, proxy chains) each live
in one module, so renamed fields are a one-file change; agx follows their latest
revision and tests against the reference SDK's conductor.

## Proxy chains

The same component answers `proxy/initialize` ([proxy chains](https://agentclientprotocol.com/rfds/proxy-chains)),
so a conductor can place agx in a chain with other proxies. In a chain agx
provides its MCP servers as `acp:` servers, which the conductor bridges for agents
without native support.

## Built-ins

Codex, Claude Code, and Pi keep their native adapters: native modes deliver skills
and instructions that ACP cannot express yet. Their ACP forms (`codex-acp`,
`claude-acp`, `pi-acp` from the registry) are ordinary ACP harnesses through this
proxy, with built-in skill delivery applied when agx knows the underlying harness.

Rejected: implementing ACP v2 now, against drafts few agents speak; a full ACP
client inside agx, which would duplicate the editor's job; per-agent ACP adapters,
which capability negotiation makes unnecessary.
