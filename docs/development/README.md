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
messages and service descriptors live in `gen/typescript`. Regenerate them with
`buf generate`; TypeScript consumers need `@bufbuild/protobuf` and can build
Connect clients with `@connectrpc/connect`. Generator versions are pinned in
[buf.gen.yaml](../../buf.gen.yaml).

## Releasing

Pushing a `v*` tag runs `.github/workflows/release.yml`, which tests and
publishes a GitHub release with GoReleaser (`.goreleaser.yaml`). Every `master`
commit that passes CI publishes its own `nightly-<version>.<commits>-<commit>`
prerelease, and the 10 newest are kept. Run
`goreleaser build --snapshot --clean` to build every target locally. Bump the
version in `package.nix` and `CHANGELOG.md` together with the tag.
