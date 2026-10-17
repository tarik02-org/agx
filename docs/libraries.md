# Libraries

Runtime dependencies are pinned in `go.mod`; schema generators are pinned in
`buf.gen.yaml`.

| Purpose | Choice | Notes |
| --- | --- | --- |
| MCP clients/server | `github.com/modelcontextprotocol/go-sdk` | SDK transport interfaces give lossless stdio forwarding. |
| Management RPC | `connectrpc.com/connect` | Standard `net/http` integration; generated Go/TypeScript clients. |
| Schema tooling | Buf, Protobuf, Connect generators | Generated from `api/weft/v1` into `gen/`. The Connect generator (v1.19.1) trails the runtime library because the registry has no matching release. |
| CLI | `github.com/spf13/cobra` | Harness subcommands bypass flag parsing to forward native arguments unchanged. |
| YAML | `go.yaml.in/yaml/v4` | Typed decoding, unknown-field rejection, source positions. Release candidate. |
| Native TOML | `github.com/pelletier/go-toml/v2` | Codex configuration artifacts. |
| OAuth | `golang.org/x/oauth2` | Discovery, dynamic registration, PKCE, callback handling, and refresh. |
| Secret storage | `github.com/zalando/go-keyring` | Keychain/Credential Manager/Secret Service; unavailable storage is explicit. |
| Paths | `github.com/adrg/xdg` | XDG base directories with native macOS/Windows fallbacks. |
| Locks | `github.com/gofrs/flock` | Daemon startup and installer locks. |
| OS integration | `golang.org/x/sys` | Process groups, Windows ACLs, and native lifecycle details. |

Diagnostics go to stderr through the standard library; configuration changes
are detected on demand by comparing file modification times and sizes. Use standard `crypto/rand`, hashing,
`encoding/json`, `os/exec`, `embed`, and filesystem APIs where sufficient. Git
sources use an installed Git executable instead of a Git client library.

Gin is unnecessary alongside Connect and `net/http`. Viper must not own the
domain's composition rules. Defer SQLite until durable runtime queries require
it; configuration, lockfiles, immutable cache entries, and keychain state cover
v1.
