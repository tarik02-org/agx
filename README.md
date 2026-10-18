# agx

[![CI](https://github.com/tarik02-org/agx/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/tarik02-org/agx/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/tarik02-org/agx?sort=semver)](https://github.com/tarik02-org/agx/releases)

agx launches coding agents (Codex, Claude Code, and Pi) with the instructions,
skills, and MCP servers that fit the directory you are in. You describe once
which projects get what; `agx codex` or `agx claude` then starts the agent with
exactly that. A small background daemon runs the MCP servers and holds their
credentials, so tokens never sit in config files or in the agents' settings.

One configuration serves every harness, so you stop copying the same MCP
servers and prompts into three tools' settings, and a work repository never
gets your personal servers by accident.

## Install

**Release archive.** Download the archive for your platform (Linux, macOS, or
Windows; amd64 or arm64) from the
[releases page](https://github.com/tarik02-org/agx/releases), check it against
`checksums.txt`, and put `agx` (`agx.exe` on Windows) on your `PATH`. The
[nightly prerelease](https://github.com/tarik02-org/agx/releases/tag/nightly)
is rebuilt from every `master` commit that passes CI.

On macOS, binaries downloaded with a browser are quarantined and Gatekeeper
refuses to run them because they are not notarized. Clear the flag once:

```sh
xattr -d com.apple.quarantine ./agx
```

**Nix.** Run `nix run github:tarik02-org/agx`, install it with `nix profile
install github:tarik02-org/agx`, or add the flake's `packages.<system>.agx` to
your system or Home Manager configuration.

**Go.** With Go 1.26 or newer:

```sh
go install github.com/tarik02-org/agx/cmd/agx@latest
```

Install the harnesses you use separately; agx does not install or update them.
Git is needed for skills and packages from Git.

## Quick start

```sh
agx instructions add --id pnpm --text 'Use pnpm, not npm.'
agx skill add anthropics/skills --skill pdf
cd ~/work/my-project
agx mcp add fs --scope here -- npx -y @modelcontextprotocol/server-filesystem .
agx explain        # what this directory gets, and why
agx codex          # or: agx claude, agx pi, or just agx for the launcher
```

Each command shows its change as a diff before writing your configuration.
`agx doctor` checks the whole setup and tells you how to fix what it finds.

## Documentation

- [Getting started](docs/getting-started.md)
- [Configuration](docs/configuration.md)
- [Harnesses and shims](docs/harnesses.md)
- [MCP servers and logins](docs/mcp.md)
- [Skills, instructions, and packages](docs/skills-and-packages.md)
- [Secrets](docs/secrets.md)
- [Launcher and sessions](docs/sessions.md)
- [The daemon](docs/daemon.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Security model](docs/security.md)
- [CLI reference](docs/cli-reference.md)

Working on agx itself? See the [development notes](docs/development/README.md).
Changes are listed in the [changelog](CHANGELOG.md).

Licensed under the [MIT License](LICENSE).
