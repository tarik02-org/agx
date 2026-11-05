# Developing the OpenCode plugin

The [user guide](../../packages/opencode/README.md) covers installation and everyday use.
The reusable `agx-client` workspace owns the daemon connection and RPCs; the native
OpenCode plugin owns resource leases, hooks, skill deployment and session discovery.
The Go adapter starts the native client and consumes generic provider summaries
and transient launch data. It does not call OpenCode's API or read its database.

## Build and verify

From the repository root:

```sh
pnpm install
pnpm build
pnpm exec vp run agx-opencode#bundle
pnpm check
pnpm test
```

`build` produces the reusable client and normal package outputs. `bundle` produces
`packages/opencode/dist/git`, the self-contained package published to the
`opencode-plugin` Git branch. It bundles runtime dependencies while importing
Effect and OpenCode APIs from the host. The directory includes a standalone
manifest, the user guide, and license notices.

The native end-to-end fixture installs this bundle from a temporary Git repository
using OpenCode's plugin manager, then runs the real server and daemon. It covers
resource selection, virtual skills, executable assets, instructions, MCP calls,
credential isolation, cleanup, restart/crash/port recovery, reload, revocation and
OpenCode-managed Git package updates.

For local development, configure the package directory directly in OpenCode:

```json
{
  "plugins": [
    {
      "package": "/absolute/path/to/agx/packages/opencode/dist/git",
      "options": { "executable": "/absolute/path/to/agx" }
    }
  ]
}
```

OpenCode's `plugin add` command accepts registry and Git packages. Local package
directories are configured directly rather than passed to that command.

## Publishing

After successful CI on `master`, the Nightly workflow publishes the AGX binaries,
then builds the standalone plugin and commits its contents to `opencode-plugin`.
The first successful run creates that branch. Pull requests do not publish it.
A build whose source is no longer the current `master` is skipped, and unchanged
package contents create no commit. Updates preserve the branch's history.

OpenCode installs the Git branch and tracks its revision. Publishing uses the
repository's GitHub token and does not require npm credentials. Only Effect and
`@opencode/plugin` are imported from the host; other runtime dependencies,
including `@opencode/schema`, are bundled. `deps.onlyImport` enforces this boundary.
Bundled license notices are collected automatically; upstream texts omitted from
npm packages live under `packages/opencode/licenses`.

## Options

| Option            | Default                     | Meaning                                                                                                                      |
| ----------------- | --------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `executable`      | `agx`                       | AGX client executable on the OpenCode server.                                                                                |
| `config`          | AGX's normal root           | Explicit AGX root configuration file.                                                                                        |
| `discovery`       | CLI discovery               | Explicit private discovery file; disables automatic daemon startup.                                                          |
| `harness`         | `opencode`                  | AGX consumer or a definition based on it.                                                                                    |
| `scopeDirectory`  | OpenCode location directory | Directory in AGX's scope namespace. Override when mapping a workspace.                                                       |
| `mcpDirectory`    | OpenCode location directory | Explicit MCP execution directory on the daemon host. Set `""` when unnecessary, for example for HTTP-only MCP.               |
| `codemode`        | OpenCode's default          | Explicitly enable/disable Code Mode for AGX's native MCP servers.                                                            |
| `sessionSourceId` | Persisted native profile ID | Stable source identity; set distinct values for independent servers.                                                         |
| `sessionServer`   | Registered native service   | Server URL reachable by the client; required for custom/manual servers. Credentials come from the native server environment. |

## Runtime behavior

One resource lease belongs to each OpenCode location. Sessions in that location
share its skills and MCP catalog; locations resolve independently. AGX instruction
text is included in primary, compaction, generation and title requests. `/agx-reload`
acquires current configuration for the location and replaces only AGX's resources.
Native process settings such as executable, argv and environment are not applied.

Text-only skills use OpenCode's virtual `skill://` paths and create no files.
Supporting files are deployed into a private temporary directory on the OpenCode
server, grouped by content digest, with executable permissions preserved. The
directory is removed with the plugin scope. Asset execution requires OpenCode's
tools to have access to that server filesystem; this does not deploy files into a
separate remote workspace executor.

Native MCP servers receive AGX endpoint URLs and a restricted lease grant. Servers
with `load: on_demand` stay outside the native catalog and remain accessible with
`agx mcp call`. Shells receive `AGX_CONFIG`, `AGX_LAUNCH_ID` and `AGX_MCP_GRANT` for
that location. Upstream MCP environment inputs and management/lifecycle tokens
are removed from shell environments. The server's global environment is unchanged.

The plugin renews leases and watches discovery. A daemon replacement reattaches
the existing lease; changed configuration or endpoint origin triggers fresh
preparation and native catalog reload. Revocation is respected. A crashed daemon
can be restarted through the CLI's startup-lock checks; an intentional daemon
stop is left stopped. Runtime operations fail while their resources are unavailable.

AGX transport remains authenticated loopback HTTP. For a remote OpenCode server,
run AGX alongside that server; neither resource content nor this package's
interface requires the user's UI to share its filesystem. Remote AGX transport
and a persistent content-addressed store are separate features.

## Session providers and launching

Each activated native location publishes paginated session summaries and follows
native session events. Children and archived sessions are excluded. Source
identity belongs to the native profile; scope identity belongs to the location.
The generic daemon catalog persists summaries and marks disconnected scopes
offline. Listing does not activate OpenCode or create conversations.

Resume sends a command to the live provider, which validates the native session
and returns private connection data. Server credentials are transient: they are
absent from cached summaries and public listings and reach the client through
its environment. Custom servers require `sessionServer`; normal shared services
are discovered inside the plugin and checked against its process identity.

`agx opencode` starts the native client without a process resource lease. Resuming
an offline local location first runs a native read-only listing to activate its
provider; this requires the matching native profile and an existing local directory.
Only activated locations are discoverable. Custom AGX launcher definitions are
rejected because a shared service cannot honor their per-launch resource settings.

## Tooling

Both workspaces use stable TypeScript 7 and inherit `tsconfig.base.json` and the
root Vite lint/format rules. External dependencies share the default pnpm catalog
in `pnpm-workspace.yaml`, with caret ranges and exact resolutions in the lockfile.
The lint baseline enables Oxlint's correctness, suspicious and performance categories;
explicit overrides add type-safety and AGX error-handling policies.
`pnpm check` runs Vite+'s combined format, lint and type checks across the
repository. `pnpm lint` runs type-aware lint separately. `pnpm format` discovers
supported files automatically, excluding generated code and schemas.
Vite tasks build the client before checks and lint, and build workspace
dependencies before the plugin and its end-to-end test. Build results are cached;
checks, lint and the end-to-end test execute on every invocation.
The equivalent task commands are `vp run check:workspace`,
`vp run lint:workspace`, `vp run -r build` and `vp run -r test`. Schema, platform,
configuration and RPC failures are handled by their tags; lint rejects broad
Effect catches, `mapError`, and error-discarding helpers. Unexpected SDK/parser
exceptions remain defects rather than becoming retryable integration failures.
The Effect language-service diagnostics also run in supported editors; the native TypeScript compiler
does not load TypeScript language-service plugins.

`skipLibCheck` is retained for dependency declarations: OpenCode 2.2.5's AI
package has incompatible optional-property declarations under
`exactOptionalPropertyTypes`. First-party strict checks remain enabled.
The client accepts generated Protobuf enums; the plugin and root tooling require
erasable TypeScript syntax.

The end-to-end test builds isolated AGX binaries and runs the pinned OpenCode
server with a local model fixture and stdio MCP server. It uses temporary
configuration, data and cache directories and does not access real model accounts.
