# Libraries

Runtime dependencies are pinned in `go.mod`; code generators in `buf.gen.yaml`. The
non-obvious choices:

| Purpose | Choice | Notes |
| --- | --- | --- |
| MCP | `github.com/modelcontextprotocol/go-sdk` | Its transport interfaces allow lossless stdio forwarding. |
| Management RPC | `connectrpc.com/connect` | Plain `net/http`; the Connect generator (v1.19.1) trails the runtime because the registry has no matching release. |
| CLI | `github.com/spf13/cobra`, `charm.land/fang/v2`, `github.com/muesli/mango-cobra` | Harness subcommands bypass flag parsing to forward native arguments unchanged; fang styles help, mango renders `agx man`. |
| Terminal UI | `charm.land/lipgloss/v2`, `bubbletea/v2`, `bubbles/v2`, `huh/v2` | `colorprofile` strips styling for pipes and `NO_COLOR`; prompts use stock huh forms; Ctrl-C and Esc abort with exit status 130. |
| YAML | `go.yaml.in/yaml/v4` | Release candidate, for node trees with positions that configuration edits rewrite in place. |
| Diffs | `github.com/aymanbagabas/go-udiff` | Places edits in the original text and renders previews. |
| JSON Schema | `github.com/invopop/jsonschema` | Reflects configuration types into the published schemas. |
| Keychain | `github.com/zalando/go-keyring` | Holds only the key of the encrypted secret store (ADR 0026); needs no cgo. |

Browser opening uses `github.com/pkg/browser`; relative times use `go-humanize`.
OAuth refreshes use `golang.org/x/oauth2`, and session parsing uses a limited
`errgroup.Group`. PATH candidates share one platform iterator.

Git sources use an installed `git` executable instead of a Git library.
