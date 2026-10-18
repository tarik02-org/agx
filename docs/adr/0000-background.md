# Background: the core model

Status: accepted. Condenses the original design records; later ADRs refine it.

agx composes agent resources (skills, instructions, MCP servers) for the
directory a harness runs in, and launches the harness with them natively. The
decisions below still hold. [Configuration](../configuration.md) and
[contracts](../contracts.md) specify the details.

## Additive directory scopes

Configuration is a root YAML file with explicit, ordered imports, so personal,
work, and machine-local fragments can live in separate or ignored files.
Machine-specific directory mappings stay out of shared configuration.

Scopes add to what a directory inherits and may `remove` resources; they compose,
they never select a replacement environment. Rules apply from shallow to deep,
then in import and rule order. Bundles expand before removals, so a deeper scope
can drop one skill from an inherited bundle and a still deeper one can re-add it.
Resources have qualified IDs; replacing a definition needs an explicit
`override`, never generic deep merging. Every inclusion and removal keeps its
source location, so `explain` can show why a resource is present.

Rejected: profiles or environments chosen per directory, which duplicate shared
selections; generic map merging, which cannot express removal or provenance.
Refined by [ADR 0007](0007-harness-definitions-and-modes.md) (harness-filtered
rules), [ADR 0011](0011-straightforward-configuration.md) (inline definitions,
`remove`), and [ADR 0014](0014-configuration-editing-commands.md) (rules
without a directory).

## Packages, bundles, and locks

A package is a versioned distribution unit; a bundle selects resources from
installed packages, so several bundles share one copy. Skills keep their whole
directory so relative scripts and references work.

Lockfiles (JSON) record resolved revisions, dependencies, and content hashes.
Contents install into a shared immutable cache. Locks change only through
explicit installation commands; resolution and launches never fetch. Each launch resolves a
snapshot first, so updates affect later launches, not running ones.

[ADR 0011](0011-straightforward-configuration.md) adds anonymous Git packages
and pinned sources that need no lock entry.

## Native harness integration

Each built-in harness (Codex, Claude Code, Pi) has an adapter that turns a
snapshot into native arguments, configuration, and skill or plugin artifacts in
agx's cache. agx keeps the harness's protocol, standard streams, exit status,
and native account state; it does not own a harness home or hide existing global
skills. Native plugins are an output format, not agx's package model: artifacts
are generated per snapshot, so removing one resource does not require removing a
plugin. Instructions are injected explicitly, because a guidance file inside a
plugin is not guaranteed to load. Hosts that serve many threads from one
process, such as T3 Code with `codex app-server`, get per-thread launches
([ADR 0015](0015-per-thread-resources-for-multi-session-hosts.md)). macOS,
Linux, and Windows are all targets.

Rejected: manager-owned harness homes, which split the user's native state.
[ADR 0007](0007-harness-definitions-and-modes.md) adds harness definitions and
modes; [ADR 0012](0012-harness-adapters-and-shims.md) fixes the adapter
interface and per-harness mechanisms.

## Daemon-owned MCP runtime

A per-user daemon owns upstream MCP sessions, stdio child processes, credentials
and OAuth refresh, routing, and runtime state. Harnesses reach it with their
native HTTP MCP support: one endpoint per selected MCP server,
`/launch/<id>/mcp/<server>`, all on one loopback listener. Separate endpoints
keep each server's identity, tools, and guidance instead of merging catalogs.
Native tools, schemas, resources, prompts, notifications, and cancellation pass
through.

Upstream sessions are per launch; pooling may come later where server identity
and working directory allow. Central ownership means a daemon failure affects
every launch, so restarts and reconnection are handled explicitly.

Rejected: per-server stdio connector processes; a generic
`search_tools`/`call_tool` facade, which hides native tools from the harness.
[ADR 0008](0008-injected-values-and-secrets.md) and
[ADR 0009](0009-inline-mcp-auth-and-interactive-login.md) define secrets and MCP
auth.

## Launch grants and leases

The wrapper asks the daemon to prepare a launch; the daemon resolves the
snapshot itself and returns the plan, a launch grant scoped to the selected MCP
servers, and a separate lifecycle credential for the wrapper. Only the daemon
mints grants. The harness receives only its grant, never secrets or management
credentials; upstream credentials go only to stdio upstreams that need them.
Generated artifacts, packages, and locks hold no secret values.

The wrapper supervises the harness, renews a lease, and reports exit. Ended or
expired launches lose MCP access and release their runtime; an HTTP disconnect
alone does not end a launch, and a grant never changes an upstream credential's
own lifetime. A daemon restart or upgrade does not end launches either: the
daemon saves them, without secrets, and the next one restores them
([ADR 0016](0016-launches-survive-daemon-restarts.md)). See [contracts](../contracts.md#launch-preparation-and-credentials)
and the [launch flow](../api.md#launch-flow).

## Go and Connect

agx is written in Go: a networking and subprocess workload, shipped as native
executables for every platform. The MCP runtime uses the official MCP Go SDK
over `net/http`. Rust's added complexity showed no benefit here.

Management (configuration resolution, launches, packages, secrets, runtime
watches) is a Protobuf API served with Connect, generated for Go and TypeScript
with Buf, so the CLI and a future UI share it. MCP traffic stays on the native
endpoints rather than being tunneled through RPCs. Both interfaces authenticate
on loopback, with management authority separate from launch grants. YAML decodes
into typed documents and composition is explicit code; Viper's generic merging
cannot express these rules. See [API](../api.md) and [libraries](../libraries.md).
