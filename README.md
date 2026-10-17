# weft

Directory-scoped skills, MCP connections, and instructions for Codex, Claude Code,
and Pi. A Go daemon owns MCP upstreams and credentials; wrappers resolve the
working directory, apply native configuration, and supervise each harness.

## Build and try

Requires Go 1.26 or newer. Git is required for Git package sources. Install the
native harnesses separately; Weft does not download or update them.

```sh
go build -o weft ./cmd/weft
./weft --config examples/weft.yaml config validate
./weft --config examples/weft.yaml explain .
./weft --config examples/weft.yaml install
./weft --config examples/weft.yaml daemon start
./weft --config examples/weft.yaml codex
./weft daemon stop
```

With Nix, `nix build` produces `result/bin/weft` and `nix develop` provides Go,
Buf, golangci-lint, and gopls. Lint with `golangci-lint run`.

Release builds set the version reported to wrappers, which must match the
daemon: `go build -ldflags "-X github.com/tarik02/weft/internal/daemon.Version=0.1.0" ./cmd/weft`.

Licensed under the [MIT License](LICENSE).

The example adds always-on instructions for this checkout. Start from
[examples/weft.yaml](examples/weft.yaml), then add your packages, skills, and MCP
connections using the [configuration reference](docs/configuration.md).

By default, the root is `weft/weft.yaml` under `XDG_CONFIG_HOME`, which falls
back to `~/.config` on Linux, `~/Library/Application Support` on macOS, and
`%LOCALAPPDATA%` on Windows. Without that file, every `weft.d/*.yaml` beside it
loads instead. Packages and artifacts live under `weft` in `XDG_CACHE_HOME`. `WEFT_CONFIG` and `WEFT_CACHE`
select explicit absolute paths; `--config` and `--cache` work before the command.
The daemon and wrapper must select the same root and cache. Daemon startup is
explicit. `daemon start --foreground` stays attached to the terminal.

## Launch and inspect

```sh
weft resolve /absolute/work/project --json
weft explain /absolute/work/project
weft install --frozen
weft update team
weft codex app-server --stdio
weft claude --print 'Review the current change'
weft pi --mode rpc
weft launch list --json
weft auth login work/docs
weft daemon status --json
```

Native arguments, standard streams, exit codes, and interrupts pass through.
Help/version and administrative native commands work without a managed launch.
A harness-named symlink can point to Weft; executable lookup excludes Weft itself.
Use a configured absolute native executable when another wrapper shadows PATH.

Scopes compose additions and exclusions with provenance. Packages use pinned
JSON locks and immutable content-addressed installations. Launching never
fetches packages or modifies native account configuration. Explicit `install`
registers managed skill plugins when Codex is installed; each launch selects its
plugins independently. Unmanaged native global skills remain outside exclusions.

MCP connections retain their own HTTP endpoints and native tool schemas/results,
resources, prompts, notifications, and cancellation. Management credentials,
wrapper lifecycle credentials, and expiring harness grants are separate. Status
and JSON inspection omit grants, secret values, and native prompt bodies. OAuth
and stored secrets use the OS keychain, with explicit errors when unavailable.

## Verification and support

[Verification notes](docs/verification.md) distinguish exercised behavior from
platform and protocol limits. The native launch path was exercised on macOS with
Codex 0.162.1, Claude Code 2.1.300, and Pi 1.2.0. Linux and Windows binaries
cross-compile but have not been run natively; native Windows launches require
`.exe` binaries. Managed endpoints support the stateful MCP handshake through 2025-11-25. OAuth currently supports public clients.

## Design and API

- [Terminology](GLOSSARY.md) and [implementation milestones](docs/implementation-plan.md)
- [CLI](docs/cli.md), [configuration](docs/configuration.md), and [runtime contracts](docs/contracts.md)
- [Management API](docs/api.md) and [Protobuf](api/weft/v1/weft.proto)
- [Library choices](docs/libraries.md)

Architecture records: [directory scopes](docs/adr/0001-additive-directory-configuration.md),
[packages and locks](docs/adr/0002-packages-bundles-and-lockfiles.md),
[native integration](docs/adr/0003-native-harness-integration.md),
[MCP runtime](docs/adr/0004-daemon-owned-http-mcp-runtime.md),
[authorization and lifecycle](docs/adr/0005-launch-authorization-and-lifecycle.md),
[Go and Connect](docs/adr/0006-go-and-connect-management-interface.md),
[harness definitions](docs/adr/0007-harness-definitions-and-modes.md), and
[injected values and secrets](docs/adr/0008-injected-values-and-secrets.md).

Go messages and Connect handlers live in `gen/weft/v1`; TypeScript Protobuf
messages and service descriptors live in `gen/typescript`. Regenerate with
`buf generate`; TypeScript consumers need `@bufbuild/protobuf` and can construct
Connect clients with `@connectrpc/connect`. Generation versions are pinned in
[buf.gen.yaml](buf.gen.yaml), and Go dependency versions in [go.mod](go.mod).
