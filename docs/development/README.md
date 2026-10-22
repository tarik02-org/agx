# Developing agx

These notes are for people working on agx itself. For using agx, start at the
[documentation index](../README.md).

- [Building and testing](#building-and-testing)
- [Runtime and configuration contracts](contracts.md): the rules the
  implementation follows for resolution, launches, credentials, and MCP.
- [Management API](api.md): the Connect services the CLI talks to, and the
  [Protobuf contract](../../api/agx/v1/agx.proto).
- [Verification](verification.md): what the test suite covers, what was checked
  by hand, and known limits.
- [Library choices](libraries.md).
- [Implementation plan](implementation-plan.md): the original milestones.
- [Architecture decision records](../adr/) and the [glossary](../../GLOSSARY.md).

## Building and testing

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
network. CI also checks that generated code is current.

Release builds set the version that `agx --version` reports and that wrappers
and the daemon compare:
`-ldflags "-X github.com/tarik02-org/agx/internal/daemon.Version=0.1.0"`.

Go messages and Connect handlers live in `gen/agx/v1`; TypeScript Protobuf
messages and service descriptors live in `web/src/gen`, used by the web UI. Regenerate them with
`buf generate`; TypeScript consumers need `@bufbuild/protobuf` and can build
Connect clients with `@connectrpc/connect`. Generator versions are pinned in
[buf.gen.yaml](../../buf.gen.yaml).

## Web UI

The UI lives in `web/`: TypeScript, React, and Vite+, managed with pnpm (the
development shell provides Node and pnpm). It builds into `internal/ui/dist`,
which the Go binary embeds.

```sh
pnpm --dir web install
pnpm --dir web build          # then go build embeds it
pnpm --dir web check          # types, lint, format
pnpm --dir web test
AGX_DAEMON_URL=http://127.0.0.1:<port> pnpm --dir web dev
```

`vp dev` serves the UI with hot reload and forwards RPCs to the running daemon
at `AGX_DAEMON_URL`; sign in by opening the `agx ui --print` link with the dev
server's origin. The Nix build fetches pnpm dependencies with a fixed hash
(`nix/web.nix`): update it after changing `web/pnpm-lock.yaml`.

## Releasing

Pushing a `v*` tag runs `.github/workflows/release.yml`, which tests and
publishes a GitHub release with GoReleaser (`.goreleaser.yaml`). Every `master`
commit that passes CI replaces the rolling `nightly` prerelease. Run
`goreleaser build --snapshot --clean` to build every target locally. Bump the
version in `package.nix` and `CHANGELOG.md` together with the tag.
