# Edit configuration from the CLI

Status: accepted.

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

## Decisions made in implementation

- **Engine.** An edit changes the parsed node tree, which is encoded with the
  file's detected indentation; a line diff between the encodings before and
  after the change (`go-udiff`) is applied to the original text, aligned to it
  line by line, so untouched lines, blank lines, and comment spacing stay byte
  for byte. When the result would not parse back to the edited tree, the whole
  tree is re-encoded instead. New lists use flow style unless their siblings are
  block lists; an empty `{}` grows in block style. Lists, `add`/`remove`
  mappings, and rules an edit empties are pruned; untouched empty rules stay.
- **Validation.** Edited fragments, and the lockfiles a Git skill source needs,
  are passed to the loader as an in-memory overlay; nothing is written unless
  the whole configuration loads. New definitions go to a fragment the root
  lists ([ADR 0027](0027-root-file-and-fragments.md)). Writes keep the file's mode, land at a symlink's target,
  and replace the file atomically; lockfiles are written the same way.
- **Placement.** Matching rules in files that cannot be written are skipped, so a
  read-only or generated fragment does not block enabling elsewhere; only the
  chosen target is refused. `use`/`unuse` create a missing rule in the target
  fragment ([ADR 0027](0027-root-file-and-fragments.md)). New directory
  rules spell paths under the home directory as `~/…`; new resource paths are
  relative when inside the fragment's directory.
- **Login and test.** The probe is the unauthenticated request on-demand OAuth
  already uses: a 401 that leads to protected resource metadata. Logging in
  needs the daemon, which owns credentials; without it agx names the commands to
  run later. `mcp tools <server>` uses the daemon RPC `ListTools` serving the server
  through a launch-less endpoint over in-memory connections, so credentials,
  tags, and diagnostics behave as in a launch and no value leaves the daemon.
- **Skills from Git.** The source becomes a package, not anonymous Git skills,
  so one checkout and one lock entry serve every chosen skill. A package root
  without `agx.package.yaml` declares the skills found in it. The repository is
  installed with its lock staged in memory before the picker opens, so the
  picker lists what loading will define. Only the requested ref is fetched,
  without history.
- **Prompts.** `huh` fields: text input, a yes/no confirmation, and a
  filterable multi-select for packages with dozens of skills, drawn inline on
  stderr with the launcher's styling and `--color`. They began as hand-written
  Bubble Tea models; the launcher stays one.
- **Hidden testing aid.** `skill add --github-base` replaces
  `https://github.com` for `owner/repo` sources; only builds with the
  `agx_testing` tag have it.
