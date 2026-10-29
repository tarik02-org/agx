# Developing agx

These notes are for people working on agx itself. For using agx, start at the
[documentation index](../README.md).

- [Building and testing](#building-and-testing)
- [Management API](api.md): the Connect services the CLI talks to, and the
  [Protobuf contract](../../api/agx/v1/agx.proto).
- [Verification](verification.md): how behavior is tested, what was checked by
  hand, and known limits.
- [Library choices](libraries.md).
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

Release builds set the version that `agx --version` reports and that commands
compare to decide whether they replace an older daemon:
`-ldflags "-X github.com/tarik02-org/agx/internal/daemon.Version=1.2.3"`. Until
the first tag, the Nix package reports `0.0.0-dev+<commit>`, which is not
ordered against any version. Raise `daemon.Protocol` with every change to the
daemon API or its files that an older agx or daemon cannot handle; commands use
any daemon of their own protocol.

Go messages and Connect handlers live in `gen/agx/v1`; TypeScript Protobuf
messages and service descriptors live in `gen/typescript`. Regenerate them with
`buf generate`; TypeScript consumers need `@bufbuild/protobuf` and can build
Connect clients with `@connectrpc/connect`. Generator versions are pinned in
[buf.gen.yaml](../../buf.gen.yaml).

## Releasing

Tag a commit that passed CI on `master`. Pushing a `v*` tag runs
`.github/workflows/release.yml`, which publishes a GitHub release with GoReleaser
(`.goreleaser.yaml`); archives include shell completions and the man page. Every
`master` commit that passes CI publishes its own
`nightly-<version>.<commits>-<commit>` prerelease. Run
`goreleaser build --snapshot --clean` to build every target locally. With the tag,
rename `## Unreleased` in `CHANGELOG.md` to the version and set `version` in
`package.nix`.
