# Edit configuration from the CLI

Status: proposed.

Configuration stays hand-written YAML; commands edit it for the common cases.

```sh
agx mcp add github https://api.githubcopilot.com/mcp
agx mcp add fs -- npx -y @modelcontextprotocol/server-filesystem .
agx mcp add docs https://docs.example/mcp --header 'X-Key=!secret docs' --auth none
agx mcp list | remove <id> | login <id> | test <id>

agx skill add ./skills/deploy
agx skill add anthropics/skills                   # several skills: pick interactively
agx skill add anthropics/skills --skill pdf --skill docx
agx skill list | remove <id>

agx instructions add ./AGENTS.md
agx instructions add --id pnpm --text 'Use pnpm, not npm.'
agx harness add codex-cpa --base codex -- -c 'model_provider="cpa"'

agx use <id> [--scope …]      # enable an existing definition
agx unuse <id> [--scope …]    # remove it from a scope
```

## Scopes

`--scope user` (the default) applies everywhere, `--scope here` to the current
directory, `--scope <dir>` to that directory; `--harness <name>` narrows any of
them. A scope rule without `directory` applies everywhere and sorts before every
directory rule, so deeper rules can still remove what it adds. A filesystem root
was rejected as the global form: Windows has one per drive.

`add` commands define and enable in one step; `--define-only` skips enabling.
Enabling appends to the first existing rule with the same directory and harness
filter, or creates one. `unuse` removes the ID from that rule's `add`, or, when
another rule added it, appends it to `remove`.

## Interaction

On a terminal, missing arguments become prompts, and each command shows the
change as a diff before writing. Without a terminal, missing arguments are errors
and `--yes` is implied. After `mcp add` of an HTTP server, agx probes it; when it
asks for OAuth, agx offers to log in now with the blocking `secret login` flow.
`skill add` accepts a path, a Git URL, or GitHub's `owner/repo[@ref][/path]`. It
fetches the source, finds every `SKILL.md`, and offers a multi-select picker
unless `--skill` or `--all` decides; the selection becomes a locked package whose
chosen skills are enabled.

## Writing

Edits go through the YAML node tree, preserving comments, ordering, quoting, and
tags. New definitions go to the root configuration file, or to `--file`; scope
changes go to the file that holds the matching rule, otherwise with the
definition. The result must validate before anything is written, and the write
is atomic. Files that are read-only or inside `/nix/store` are refused with the
reason.
