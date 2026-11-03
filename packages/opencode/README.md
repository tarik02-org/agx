# AGX for OpenCode 2

This server plugin brings AGX's directory-scoped skills, instructions and MCP
servers to OpenCode 2.2.5. It uses the native Effect plugin interface and AGX's
portable resource API. OpenCode owns its processes and conversations.

## Install from this checkout

Build the AGX client/daemon from this version and make `agx` available on the
OpenCode server's PATH. Then build the plugin:

```sh
pnpm install
pnpm build
opencode2 plugin add /absolute/path/to/agx/packages/opencode
```

Configured local plugins must name the package directory, not `dist/index.mjs`.
Alternatively, add the package to your OpenCode configuration:

```json
{
  "plugins": [
    {
      "package": "/absolute/path/to/agx/packages/opencode",
      "options": {
        "executable": "/absolute/path/to/agx",
        "config": "/absolute/path/to/agx.yaml"
      }
    }
  ]
}
```

The plugin bootstraps the daemon using `agx --json daemon connect`. If you provide
`discovery`, it reads that private discovery file directly and leaves daemon
startup to you. Credentials are read from discovery, not written into OpenCode's
configuration.

AGX recognizes the `opencode` configuration consumer, so scopes can filter on
`harnesses: [opencode]` and definitions can extend it:

```yaml
harnesses:
  opencode:
    add:
      instructions: [house-rules]
scopes:
  - directory: ~/work/team
    harnesses: [opencode]
    add:
      skills: [review]
      mcp: [tracker]
```

Check a directory's selection with `agx explain --harness opencode`.

## Options

| Option           | Default                     | Meaning                                                                                                        |
| ---------------- | --------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `executable`     | `agx`                       | AGX client executable on the OpenCode server.                                                                  |
| `config`         | AGX's normal root           | Explicit AGX root configuration file.                                                                          |
| `discovery`      | CLI discovery               | Explicit private discovery file; disables automatic daemon startup.                                            |
| `harness`        | `opencode`                  | AGX consumer or a definition based on it.                                                                      |
| `scopeDirectory` | OpenCode location directory | Directory in AGX's scope namespace. Override when mapping a workspace.                                         |
| `mcpDirectory`   | OpenCode location directory | Explicit MCP execution directory on the daemon host. Set `""` when unnecessary, for example for HTTP-only MCP. |
| `codemode`       | OpenCode's default          | Explicitly enable/disable Code Mode for AGX's native MCP servers.                                              |

## Behavior

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

## Verification

The repository development shell supplies Go 1.26, Node 24 and pnpm 12 for these
checks.

```sh
pnpm check
pnpm test
```

The end-to-end test builds isolated AGX binaries and runs the pinned OpenCode
server with a local model fixture and stdio MCP server. It uses temporary
configuration, data and cache directories and does not access real model accounts.
