# agx

[![CI](https://github.com/tarik02-org/agx/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/tarik02-org/agx/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/tarik02-org/agx?sort=semver)](https://github.com/tarik02-org/agx/releases/latest)

Directory-scoped skills, MCP servers, and instructions for Codex, Claude Code,
and Pi. A Go daemon owns MCP upstreams and credentials; wrappers resolve the
working directory, apply native configuration, and supervise each harness.

## Install

Release archives for Linux, macOS, and Windows on amd64 and arm64 are on the
[releases page](https://github.com/tarik02-org/agx/releases/latest); unpack
`agx` (`agx.exe` on Windows) onto PATH and check `checksums.txt`. The rolling
[nightly prerelease](https://github.com/tarik02-org/agx/releases/tag/nightly) is
rebuilt from every `master` commit that passes CI. Its version is the latest
release tag plus the commits since it, such as `0.1.0+nightly.12.abc1234`; until
the first tag it counts from `0.0.0` (`0.0.0+nightly.57.abc1234`). Its archive
names stay the same between builds.

With Nix, run `nix run github:tarik02-org/agx`, or add the flake's
`packages.<system>.agx` to a profile or system configuration.

From source, with Go 1.26 or newer: `go install github.com/tarik02-org/agx/cmd/agx@latest`.
Release builds set the version reported by `agx --version`, which wrappers and
the daemon compare: `-ldflags "-X github.com/tarik02-org/agx/internal/daemon.Version=0.1.0"`.

Shell completion for zsh, bash, fish, and PowerShell completes configured IDs,
such as `agx mcp remove <TAB>`; load it with `source <(agx completion zsh)`
([setup](docs/cli.md#shell-completion)).

Git is required for Git package sources. Install the native harnesses
separately; agx does not download or update them. `agx doctor` checks the
setup: configuration, packages, harness versions, Codex plugins, the daemon, MCP
servers, secrets, and shims, each with a fix.

## Try it

```sh
agx --config examples/agx.yaml config validate
agx --config examples/agx.yaml explain .
agx --config examples/agx.yaml install
agx --config examples/agx.yaml codex
agx daemon stop
```

The example adds always-on instructions for this checkout. Start from
[examples/agx.yaml](examples/agx.yaml), then add your packages, skills, and MCP
servers using the [configuration reference](docs/configuration.md).

By default, the root is `agx/agx.yaml` under `XDG_CONFIG_HOME`, which falls
back to `~/.config` on Linux, `~/Library/Application Support` on macOS, and
`%LOCALAPPDATA%` on Windows. Without that file, every `agx.d/*.yaml` beside it
loads instead. Packages and artifacts live under `agx` in `XDG_CACHE_HOME`. `AGX_CONFIG` and `AGX_CACHE`
select explicit absolute paths; `--config` and `--cache` work before the command.
The daemon and wrapper must select the same root and cache. Commands that need
the daemon start it in the background when it is not running; `agx daemon start`
does so explicitly, and `daemon start --foreground` stays attached to the terminal.
Set `daemon.autostart: false` or `AGX_NO_AUTOSTART=1` to start it only by hand.

## Quick start

```sh
agx mcp add github https://api.githubcopilot.com/mcp    # offers the OAuth login
agx mcp add fs --scope ~/work -- npx -y @modelcontextprotocol/server-filesystem .
agx skill add anthropics/skills --skill pdf --skill docx
agx instructions add --id pnpm --text 'Use pnpm, not npm.' --scope ~/work
agx explain ~/work
agx codex
```

Each command shows its change to the configuration as a diff and writes it
after validating the result; comments and layout are kept. `--scope` defaults
to everywhere, `agx use` and `agx unuse` toggle a definition per directory, and
`agx skill add` without `--skill` lets you pick from the repository's skills
([editing commands](docs/cli.md#editing-configuration)).

## Launch and inspect

```sh
agx resolve /absolute/work/project --json
agx explain /absolute/work/project
agx install --frozen
agx update team
agx codex app-server --stdio
agx claude --print 'Review the current change'
agx pi --mode rpc
agx launch list --json
agx secret login work/docs
agx daemon status --json
```

Run `agx` alone on a terminal for a launcher: type to search, Enter starts a
harness or resumes a Codex, Claude, or Pi session (Tab: this directory or all),
with the same directory-scoped resources. `agx sessions [query] --all` lists them.

Native arguments, standard streams, exit codes, and interrupts pass through.
Help/version and administrative native commands work without a managed launch.
A harness-named symlink can point to agx; executable lookup excludes agx itself.
`agx shims install` creates such links in `~/.local/share/agx/shims` for every harness
definition, so `codex-cpa` works from any shell.
Use a configured absolute native executable when another wrapper shadows PATH.

Scopes compose additions and removals with provenance. Packages use JSON locks,
or pins by commit or content hash, and immutable content-addressed installations. Launching never
fetches packages. `install` registers managed skill plugins when Codex is
installed, and a Codex launch that finds its plugin missing registers it first;
each launch selects its plugins independently, in the `CODEX_HOME` it runs
with. With Codex plugins disabled (`features.plugins = false`, after profiles and
`-c`/`--enable`/`--disable` arguments), Codex launches warn and run without skills. Unmanaged native global skills remain outside removals.

MCP servers retain their own HTTP endpoints and native tool schemas/results,
resources, prompts, notifications, and cancellation. Management credentials,
wrapper lifecycle credentials, and expiring harness grants are separate. Status
and JSON inspection omit grants, secret values, and native prompt bodies. OAuth
and stored secrets use the OS keychain, with explicit errors when unavailable.

## Verification and support

CI runs the unit and end-to-end tests on Linux, macOS, and Windows; the
[verification notes](docs/verification.md) say what they cover and what remains
unverified. The real native harnesses were exercised on macOS with Codex
0.162.1, Claude Code 2.1.300, and Pi 1.2.0. Native Windows launches require
`.exe` binaries. Managed endpoints support the stateful MCP handshake through
2025-11-25. OAuth currently supports public clients. Changes are listed in the
[changelog](CHANGELOG.md).

## Development

`nix develop` provides Go, Buf, golangci-lint, and gopls; otherwise install Go
1.26 and golangci-lint 2.12.

```sh
go test ./...                              # unit tests
go test -tags e2e ./internal/e2e/...       # builds agx and runs it end to end
golangci-lint fmt ./... && golangci-lint run ./...
GOOS=windows golangci-lint run ./...
buf lint && buf generate && go generate ./...   # Protobuf bindings and JSON Schemas
nix build .#agx
```

Tests isolate `HOME`, the `AGX_*` paths, and the harness stores, and need no
network. CI also checks that generated code is current. Pushing a `v*` tag
publishes a release with GoReleaser (`.goreleaser.yaml`); `goreleaser build
--snapshot --clean` builds every target locally.

## Design and API

- [Terminology](GLOSSARY.md) and [implementation milestones](docs/implementation-plan.md)
- [CLI](docs/cli.md), [configuration](docs/configuration.md), and [runtime contracts](docs/contracts.md)
- [Management API](docs/api.md) and [Protobuf](api/agx/v1/agx.proto)
- [Library choices](docs/libraries.md)

Architecture records: [background and core model](docs/adr/0000-background.md),
[harness definitions](docs/adr/0007-harness-definitions-and-modes.md),
[injected values and secrets](docs/adr/0008-injected-values-and-secrets.md),
[inline MCP auth and interactive login](docs/adr/0009-inline-mcp-auth-and-interactive-login.md),
[straightforward configuration](docs/adr/0011-straightforward-configuration.md),
[harness adapters and shims](docs/adr/0012-harness-adapters-and-shims.md),
[launcher and sessions](docs/adr/0013-launcher-and-session-search.md),
[configuration editing](docs/adr/0014-configuration-editing-commands.md),
[per-thread resources for multi-session hosts](docs/adr/0015-per-thread-resources-for-multi-session-hosts.md), and
[launches surviving daemon restarts](docs/adr/0016-launches-survive-daemon-restarts.md).

Go messages and Connect handlers live in `gen/agx/v1`; TypeScript Protobuf
messages and service descriptors live in `gen/typescript`. Regenerate with
`buf generate`; TypeScript consumers need `@bufbuild/protobuf` and can construct
Connect clients with `@connectrpc/connect`. Generation versions are pinned in
[buf.gen.yaml](buf.gen.yaml), and Go dependency versions in [go.mod](go.mod).

Licensed under the [MIT License](LICENSE).
