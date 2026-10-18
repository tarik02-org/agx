# Implementation plan

The five milestones below now have an initial implementation.
[Verification notes](verification.md) record exercised behavior and remaining
platform/protocol validation limits. Each milestone ends with observable behavior before adding
the next layer. The [ADRs](../adr/) define the architecture;
[contracts](contracts.md) define the operational details.

## 1. Resolve and explain configuration

Decode typed YAML, follow ordered imports, detect cycles, resolve directory
scopes, expand bundles, and apply exclusions. Record file/line provenance.
Implement `config validate`, `resolve`, and `explain` without a running daemon.
Start with local packages; finalize the package manifest before remote fetching.

Done when a nested directory inherits a parent bundle, removes one skill, and
explains both the retained and excluded resources. Include symlinked directories,
path-component matching, import conflicts, and Windows paths in verification.

## 2. Install locked packages

Support local directories and Git repositories, immutable cache entries, JSON
lockfiles, and atomic installation. Implement `install` and `update`; reject
missing or stale locks in frozen mode. Distinguish tracked and local lockfiles.

Done when two fresh installations resolve identical content, an interrupted
installation leaves no usable partial entry, and updates preserve active artifacts.

## 3. Run the daemon and one native MCP server

Implement authenticated Connect management, launch preparation, leases, and one
HTTP MCP endpoint per selected MCP server. First support a local stdio upstream
and a remote HTTP upstream without auth; add static secrets afterward.

Done when native tool schemas/results, cancellation, and MCP server status work,
and an expired launch cannot call its MCP endpoints. Keep sessions isolated per
launch and MCP server. Verify protocol forwarding before adding pooling.

## 4. Launch all three harnesses

Build Codex, Claude Code, and Pi adapters. Compile selected skills and instructions,
inject HTTP MCP configuration, locate the real executable, and supervise it.
Prototype Codex plugin selection first: its native installation and per-launch
enablement need proof before depending on them.

Done when each harness loads a selected skill, receives instructions without
invoking a skill, and calls a native MCP tool. Verify concurrent directories,
exclusions, resumption, terminal interrupts, and redirected protocol streams.
Include T3 Code's per-thread `app-server` invocation and native Windows launches.

## 5. Complete auth and operational behavior

Add OAuth discovery/registration, browser callbacks, serialized refresh, keychain
storage, config watching, and runtime watches. Add headless credential sources
without silently falling back to plaintext storage. Implement the remaining CLI.

Done when concurrent launches reuse an authorized identity without racing refresh,
logout invalidates the corresponding sessions, and daemon/wrapper crashes have
the documented lifecycle behavior. The CLI must expose enough state for a later UI.

## Module ownership

Use `cmd/agx` plus internal modules for config resolution, packages, harness
adapters, auth, launch lifecycle, MCP runtime, and the management interface.
Resolution returns data; adapters return spawn plans. The wrapper owns the harness
process; the daemon owns upstream MCP processes. RPC handlers translate and
validate external messages before calling those modules.

Defer the UI, upstream pooling, remote registries, and automatic harness binary
updates until this launch path works. They remain compatible with the design.
