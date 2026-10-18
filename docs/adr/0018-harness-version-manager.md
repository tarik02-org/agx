# Install and pin harness versions

Status: proposed.

Harnesses today are whatever is on PATH, so their versions drift per machine and
new agents need manual installation. agx installs harnesses itself, side by side,
pinned by lockfile, from two sources: the built-ins' upstream packages and the
[ACP agent registry](https://agentclientprotocol.com/get-started/registry).

## Sources

| Harness | Source | Installed as |
| --- | --- | --- |
| codex | npm `@openai/codex` | npm package |
| claude | npm `@anthropic-ai/claude-code` | npm package |
| pi | npm `@earendil-works/pi-coding-agent` (formerly `@mariozechner/pi-coding-agent`, which stopped at 0.73) | npm package |
| registry agents | `registry.json` entry: `binary`, `npx`, or `uvx` | archive, npm, or Python package |

The registry (`https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json`)
gives each agent an ID, version, and distribution. `binary` archives are verified
against their `sha256` and unpacked; `npx` packages are installed with npm into an
agx-owned prefix; `uvx` packages into an agx-owned uv tool directory. Nothing
runs `npx` or `uvx` at launch, so launches never fetch.

## Layout and commands

Installations live under `<cache>/harnesses/<id>/<version>/`, immutable, several
versions side by side.

```sh
agx harness install codex@0.162.1
agx harness install gemini           # latest registry version
agx harness list [--available]
agx harness update [id]              # advance pins explicitly
agx harness remove <id>@<version>
```

Configuration selects a managed installation instead of PATH:

```yaml
harnesses:
  codex:
    version: 0.162.1        # managed; omit for PATH lookup
  gemini:
    source: acp:gemini       # registry agent; launched through the ACP proxy
    version: latest
```

`version` pins are recorded in the declaring fragment's lockfile with the resolved
version and content hash, like packages: `install` resolves `latest`, launches use
the locked version and never fetch, `--frozen` requires a match, `update` advances
it. A harness without `version` keeps today's PATH lookup, so managed installs are
opt-in per harness. `executable` remains for wrappers and overrides both.

## Runtimes

Node.js and Python are provided externally. By default agx uses `node`/`npm` and
`python`/`uv` from PATH; a harness can name others:

```yaml
harnesses:
  claude:
    version: 2.1.300
    runtime: {node: ~/.local/share/mise/installs/node/24/bin/node}
```

Launches run npm-installed harnesses with the configured `node` (and Python ones
with the configured interpreter) rather than their shebang, so a managed harness
does not depend on which runtime PATH happens to hold. `agx doctor` checks that
each needed runtime exists and meets the package's declared engine range.
Managing runtimes themselves (downloading Node or Python) is deferred to existing
tools such as mise or Nix.

## Relation to other features

Registry agents are harnesses: they get definitions, shims, completions, the
launcher, and `agx doctor` like built-ins. Native adapters keep handling codex,
claude, and pi whether installed by agx or found on PATH; the ACP proxy of
[ADR 0017](0017-acp-proxy.md) handles the rest. Garbage collection of unused
versions follows package-cache retention.

Rejected: running `npx`/`uvx` at launch, which fetches unpinned code on every run;
downloading Node or Python, which duplicates mise and Nix; replacing PATH lookup by
default, which would surprise users with existing installs.
