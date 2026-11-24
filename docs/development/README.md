# Developing agx

These notes are for people working on agx itself. For using agx, start at the
[documentation index](../README.md).

- [Building and testing](#building-and-testing)
- [Management API](api.md): the Connect services the CLI talks to, and the
  [Protobuf contract](../../api/agx/v1/agx.proto).
- [Verification](verification.md): how behavior is tested, what was checked by
  hand, and known limits.
- [Library choices](libraries.md).
- [OpenCode plugin](opencode.md): development, distribution and advanced options.
- [Architecture decision records](../adr/) and the [glossary](../../GLOSSARY.md).

## Building and testing

`nix develop` provides Go, Buf, golangci-lint, and gopls; otherwise install Go
1.26 and golangci-lint 2.12.

```sh
go build ./cmd/...                        # client and daemon
go test ./...                              # unit tests
go test -tags e2e ./internal/e2e/...       # builds agx/agxd pairs and runs them end to end
golangci-lint fmt ./... && golangci-lint run ./...
GOOS=windows golangci-lint run ./...
buf lint && buf generate && go generate ./...   # Protobuf bindings, JSON Schemas, and SQL queries
nix build .#agx
nix flake check                            # includes Home Manager overlay validation
```

Tests isolate `HOME`, the `AGX_*` paths, and the harness stores, and need no
network. CI also checks that generated code is current.

SQLite queries live under `internal/storage/{state,cache}`. Embedded Goose
migrations are the schema source for sqlc; `go generate ./internal/storage` uses
the pinned generator. Generated query code is checked in. Add forward migrations
with the next version and update `PRAGMA user_version` in the same migration.
Durable state and disposable cache have independent migration histories.

Release builds set the same version on both binaries, reported by `agx --version`
and `agxd --version`. Commands compare it to decide whether they replace an older
daemon:
`-ldflags "-X github.com/tarik02-org/agx/internal/control.Version=1.2.3"`. Until
the first tag, the Nix package reports `0.0.0-dev.<commit time>+<commit>`, so a
later build replaces the daemon of an earlier one; neither is ordered against
the unknown `dev` version of a plain build. Version comparison uses semver, with
nightly commit counts in build metadata compared separately. Each canonical root
configuration path gets a daemon in its own hashed runtime directory; protocol
numbers and executable-content fingerprints are unnecessary before the first release.

`cmd/agx` links the CLI and RPC client; it must not link the daemon, database/sql,
SQLite or Goose. Its dependency test enforces that boundary. `cmd/agxd` owns
serving and signal handling. Pure configuration, authentication and session models
live separately from `configstore`, `secretstore` and `sessionstore`, which own
persistence. Configuration editing and package installation take local data
explicitly and return changes for the daemon to persist. This prepares reuse by a
future daemonless runtime; no daemonless launch mode is implemented yet.

Go messages and Connect handlers live in `gen/agx/v1`; TypeScript Protobuf
messages and service descriptors live in `packages/client/gen`. Regenerate them with
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

Components come from shadcn (Base UI primitives, the `mira` style, lucide
icons; see `web/components.json`). Add them with its CLI, never by hand, and
update them with `add --diff`:

```sh
cd web && pnpm dlx shadcn@latest add dialog
```

Theme tokens live in `web/src/styles.css`; agx adds `success` and `warning`
next to shadcn's.

`vp dev` serves the UI with hot reload and forwards RPCs to the running daemon
at `AGX_DAEMON_URL`; sign in by opening the `agx ui --print` link with the dev
server's origin. The Nix build fetches pnpm dependencies with a fixed hash
(`nix/web.nix`): update it after changing `web/pnpm-lock.yaml`.

## Releasing

Tag a commit that passed CI on `master`. Pushing a `v*` tag runs
`.github/workflows/release.yml`, which publishes a GitHub release with GoReleaser
(`.goreleaser.yaml`); archives include shell completions and the man page. The
workflow refuses a tag that is not on `master` or whose commit has no successful
CI run, and attests the provenance of the archives listed in `checksums.txt`
(`gh attestation verify <archive> --repo tarik02-org/agx`). Workflow actions are
pinned to commits, which Dependabot proposes to move monthly. Every
`master` commit that passes CI publishes its own
`nightly-<version>.<commits>-<commit>` prerelease. Run
`goreleaser build --snapshot --clean` to build every target locally. With the tag,
rename `## Unreleased` in `CHANGELOG.md` to the version and set `version` in
`package.nix`.
