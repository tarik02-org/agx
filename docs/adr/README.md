# Architecture decision records

Each record states one decision and why it was made. A later record that
changes an earlier one says so in its status line; the earlier record stays as
written.

| ADR                                                          | Decision                                                                  | Status                                     |
| ------------------------------------------------------------ | ------------------------------------------------------------------------- | ------------------------------------------ |
| [0000](0000-background.md)                                   | The core model, condensed from the original records 0001 to 0006          | accepted; refined by the records below     |
| [0007](0007-harness-definitions-and-modes.md)                | Define harnesses by extending built-ins, with mode-scoped arguments       | accepted                                   |
| [0008](0008-injected-values-and-secrets.md)                  | Inject launch-time values with YAML tags; call credential sources secrets | accepted                                   |
| [0009](0009-inline-mcp-auth-and-interactive-login.md)        | Declare MCP auth inline; log in interactively                             | accepted                                   |
| [0010](0010-portable-configuration-repositories.md)          | Keep configuration portable: sops secrets                                 | accepted                                   |
| [0011](0011-straightforward-configuration.md)                | Simplify configuration before freezing the schema                         | accepted; nested imports replaced by 0027  |
| [0012](0012-harness-adapters-and-shims.md)                   | Isolate harnesses behind adapters; expose definitions as shims            | accepted; adapter contract revised by 0031 |
| [0013](0013-launcher-and-session-search.md)                  | Open a launcher with session search when agx runs bare                    | accepted; session index replaced by 0028   |
| [0014](0014-configuration-editing-commands.md)               | Edit configuration from the CLI                                           | superseded by 0029                         |
| [0015](0015-per-thread-resources-for-multi-session-hosts.md) | Per-thread resources for multi-session hosts                              | accepted; extended to ACP by 0032          |
| [0016](0016-launches-survive-daemon-restarts.md)             | Launches survive daemon restarts                                          | accepted                                   |
| [0022](0022-on-demand-mcp-calls.md)                          | Call MCP servers on demand from the shell                                 | accepted                                   |
| [0024](0024-shared-mcp-server-processes.md)                  | Share MCP server processes across launches on request                     | accepted                                   |
| [0025](0025-harness-settings.md)                             | Give harness definitions native settings                                  | accepted                                   |
| [0026](0026-encrypted-secret-store.md)                       | Keep stored secrets encrypted under one keychain key                      | accepted; the JSON file replaced by 0028   |
| [0027](0027-root-file-and-fragments.md)                      | Split configuration into a root file and fragments                        | accepted                                   |
| [0028](0028-sqlite-state.md)                                 | Keep machine-managed state in SQLite                                      | accepted                                   |
| [0029](0029-local-configuration.md)                          | Local configuration with declarative overlays                             | accepted                                   |
| [0031](0031-portable-resource-consumption.md)                | Consume configuration independently of process launch and storage         | accepted                                   |
| [0032](0032-acp-agents.md)                                   | Relay ACP agents, giving each session its directory's resources           | accepted                                   |

## Numbers without a record

- **0001 to 0006** were the original design records. [0000](0000-background.md)
  condenses them.
- **0017 to 0021 and 0023** were deferred proposals, never implemented and
  removed: an ACP proxy (ACP support was later decided in 0032), a harness
  version manager, agx self-management for agents, a daemonless runtime,
  configuration provisioning, and an OpenCode plugin of an earlier design.
- **0030** is reserved by a proposal that is not merged yet.

Numbers are not reused.
