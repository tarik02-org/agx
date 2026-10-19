<div align="center">

# agx

**One config for every coding agent.**<br>
Skills, MCP servers, and instructions — scoped to the directory you're in.

Codex · Claude Code · Pi

[![CI](https://github.com/tarik02-org/agx/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/tarik02-org/agx/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/tarik02-org/agx?sort=semver)](https://github.com/tarik02-org/agx/releases)

<img src="docs/assets/explain.png" alt="agx explain: what ~/work/acme-web gets, and why" width="760">

</div>

## What it looks like

```yaml
# ~/.config/agx/agx.yaml
packages:
  mattpocock:
    git: https://github.com/mattpocock/skills.git

mcp:
  context7:   { url: https://mcp.context7.com/mcp, auth: none }
  github:     { url: https://api.githubcopilot.com/mcp/, auth: keychain }
  playwright: { command: [npx, -y, '@playwright/mcp@latest'] }

instructions:
  house-rules:
    text: Use pnpm, not npm. Write conventional commits.

scopes:
  - add:                                  # everywhere
      skills: [mattpocock/tdd, mattpocock/grill-me, mattpocock/diagnosing-bugs]
      instructions: [house-rules]
      mcp: [context7, github]
  - directory: ~/work                     # work projects only
    add:
      skills: [mattpocock/pr]
  - directory: ~/work/acme-web            # just this repo
    add:
      mcp: [playwright]
```

```console
$ cd ~/work/acme-web
$ agx claude   # 4 skills, house rules, context7 + github + playwright
$ agx codex    # same
$ agx pi       # same — MCP included

$ cd ~/oss/blog
$ agx claude   # no pr skill, no playwright
```

## Features

<table>
<tr>
<td width="33%" valign="top">

**📁 Directory scopes**<br>
Add or remove anything for a folder and everything below it. Work repos get work tools; personal ones stay clean.

</td>
<td width="33%" valign="top">

**📦 Skills from Git**<br>
`agx skill add mattpocock/skills` — pick skills, lock the commit, `agx update` later.

</td>
<td width="33%" valign="top">

**🔌 MCP for every harness**<br>
HTTP and stdio servers, served to Codex, Claude Code, and Pi — which has no MCP built in.

</td>
</tr>
<tr>
<td valign="top">

**🔐 No tokens in files**<br>
Keychain secrets and OAuth logins, held by a local daemon. Agents never see them.

</td>
<td valign="top">

**🔍 Explainable**<br>
`agx explain` shows every resource a directory gets and the line that decided it.

</td>
<td valign="top">

**⏯ Launcher**<br>
Run `agx` to start an agent or resume any past session, from any harness.

</td>
</tr>
</table>

## Install

Download the archive for your platform (Linux, macOS, or Windows; amd64 or arm64) from the [releases page](https://github.com/tarik02-org/agx/releases), check it against `checksums.txt`, and put `agx` on your `PATH`. On macOS, clear the quarantine flag once: `xattr -d com.apple.quarantine ./agx`.

<details>
<summary><b>mise</b></summary>

```sh
mise use -g github:tarik02-org/agx
```
</details>

<details>
<summary><b>Nix</b></summary>

```sh
nix profile install github:tarik02-org/agx
```
</details>

<details>
<summary><b>Go 1.26+</b></summary>

```sh
go install github.com/tarik02-org/agx/cmd/agx@latest
```
</details>

agx wraps the agents you already have; install Codex, Claude Code, or Pi separately.

## Get started

```sh
agx skill add mattpocock/skills --skill tdd --skill grill-me
agx mcp add context7 https://mcp.context7.com/mcp --auth none
agx mcp add playwright --scope here -- npx -y @playwright/mcp@latest
agx explain
agx claude
```

Every command shows a diff before it writes. Continue with the [getting started guide](docs/getting-started.md).

## Docs

[Getting started](docs/getting-started.md) · [Configuration](docs/configuration.md) · [Harnesses](docs/harnesses.md) · [MCP servers](docs/mcp.md) · [Skills & packages](docs/skills-and-packages.md) · [Secrets](docs/secrets.md) · [Sessions](docs/sessions.md) · [Daemon](docs/daemon.md) · [Troubleshooting](docs/troubleshooting.md) · [Security](docs/security.md) · [CLI reference](docs/cli-reference.md)

Working on agx itself? See the [development notes](docs/development/README.md) and the [changelog](CHANGELOG.md).

<sub>MIT licensed</sub>
