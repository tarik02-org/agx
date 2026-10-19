# Skills, instructions, and packages

- **Instructions** are text added to every session's system prompt, such as
  "Use pnpm, not npm." The agent follows them without being asked.
- **Skills** are folders with a `SKILL.md` describing a workflow, plus any
  scripts or references it needs. The agent loads one when it is relevant or
  when you invoke it.
- **Packages** are folders or Git repositories that provide skills,
  instructions, MCP servers, and bundles, installed and locked to a version.

## Instructions

```sh
agx instructions add ./AGENTS.md                       # from a file; ID "AGENTS"
agx instructions add --id pnpm --text 'Use pnpm, not npm.'
agx instructions list
```

In YAML:

```yaml
instructions:
  conventions:
    path: ./conventions.md
  pnpm:
    text: Use pnpm, not npm.
  team-rules:
    git: https://github.com/example/handbook.git
    path: agents/rules.md
  closing:
    text: Summarize what you changed at the end.
    order: 100
```

When several instructions apply, they are joined in order: broader scopes
first, then deeper directories, then harness selections, so specific guidance
comes last. `order` (default `0`) moves an entry earlier (lower) or later
(higher) regardless of where it was selected.

## Skills

```sh
agx skill add ./skills/deploy                         # a local folder
agx skill add anthropics/skills                       # pick from a GitHub repository
agx skill add anthropics/skills --skill pdf --skill docx
agx skill add https://git.example.com/team/skills.git --all
agx skill add owner/repo@v2/skills --skill review     # a ref and a subdirectory
```

`skill add` finds every `SKILL.md` in the source and asks which to add, unless
`--skill` or `--all` decides. A Git source becomes a [package](#packages) named
after the repository (`--package` picks another name); its skills get IDs like
`skills/pdf`. `--ref` and `--path` set the branch, tag, or commit and the
subdirectory for full Git URLs.

agx clones with your own git setup and never prompts for a password. For a
private repository, set up git credentials first (for GitHub over HTTPS, `gh
auth setup-git`), or use an SSH URL such as `git@github.com:owner/repo.git`.

To remove a skill again, use its ID; to remove a whole Git source, use the
package ID:

```sh
agx skill remove deploy
agx skill remove skills  # the package for anthropics/skills, with its lock entries
```

Removing a package also drops the rule entries that enable its skills, as
`skill add` and `use` write them, so it works right after `skill add`. Other
references, such as a bundle listing one of its skills, a rule's `remove` list,
or a harness selection, make `remove` refuse and list them; `--force` removes
those entries too. Removing a single skill refuses while rules or bundles still
use it, and `--force` works the same way.

In YAML, a skill comes from a folder, a folder in a Git repository, or is
written inline:

```yaml
skills:
  review:
    path: ~/skills/review
  strict-review:
    git: https://github.com/example/skills.git
    path: skills/strict-review
  deploy:
    description: Deploy this service and report the release URL.
    content: |
      Run `scripts/deploy.sh`, then follow `references/checklist.md`.
    files:
      scripts/deploy.sh:
        executable: true
        content: |
          #!/bin/sh
          set -eu
          make deploy
      references/checklist.md: ./docs/release-checklist.md   # copied from a file
      assets/: ./deploy-assets/                              # copied directory
```

For an inline skill, `description` is required and `content` is the body of
`SKILL.md`. Optional fields:

- `name`: the name harnesses show; defaults to the last part of the ID.
  Lowercase letters, digits, and hyphens, up to 64 characters.
- `frontmatter`: extra `SKILL.md` front matter, such as `allowed-tools: [Bash]`.
- `files`: other files in the skill, as inline text, `{content, executable}`, or
  a path to copy (a directory when both sides end in `/`).

Folder and Git skills take their name and description from their own
`SKILL.md`.

## Packages

A package is a folder or Git repository of resources:

```yaml
packages:
  team:
    git: https://github.com/example/agent-resources.git
    ref: main
  local:
    path: ./local-resources

scopes:
  - directory: ~/work
    add:
      bundles: [team/defaults]
      skills: [team/review]
```

Everything a package defines is named with its ID as a prefix: `team/review`,
`team/defaults`. A package's MCP servers appear in harnesses as
`agx-team-<id>`.

A repository without a manifest is read as a plain collection of skills: every
folder containing a `SKILL.md` becomes a skill named after the folder. To
provide more, add `agx.package.yaml` at the package root:

```yaml
skills:
  review:
    path: skills/review
instructions:
  standards:
    path: standards.md
mcp:
  tracker:
    url: https://tracker.example.com/mcp
bundles:
  defaults:
    skills: [review]
    instructions: [standards]
packages:
  shared:                      # a dependency, named team/shared
    git: https://github.com/example/shared.git
```

A manifest can define skills, instructions, MCP servers, secrets, bundles, and
dependencies. It cannot add directory rules, imports, or harnesses: the user
decides where a package's resources apply. Inside a manifest, references are
relative to the package (`shared/review` there is `team/shared/review` from
outside).

## Installing and locking

```sh
agx install            # fetch what is missing and write lockfiles
agx install --frozen   # install exactly what the lockfiles say; fail on any change
agx update             # move every Git package to the newest commit of its ref
agx update team        # just one
```

- `install` fetches packages and Git skills and instructions, then records the
  exact commit or content hash in `agx.lock.json` next to the configuration
  file that declares them. Commit lockfiles along with the configuration.
- Launching never fetches or updates anything. A missing package makes the
  launch fail with a hint to run `agx install`.
- `update` is the only command that moves a package to a newer commit. A Git
  skill or instructions file updates by kind and ID: `agx update
  skill:strict-review`.
- `--frozen` is for new machines and CI: it fails instead of changing a
  lockfile.

### Pinning without a lockfile

A source that names an exact version needs no lock entry:

```yaml
packages:
  team:
    git: https://github.com/example/agent-resources.git
    rev: 3f2c9e1a8b7d6c5e4f3a2b1c0d9e8f7a6b5c4d3e   # a full commit
  vendored:
    path: ./vendor/resources
    hash: 2610e87326d59c6c9d74031fc4e5bbec8f87d9cf0f63cf89ee7373113a1cc88d  # from the lockfile
```

A `path` inside `/nix/store` is pinned too, which suits configuration generated
by Nix. `update` leaves pinned sources alone.

Set `lockfile: <path>` in a configuration file to keep its lock somewhere else,
for example a machine-local file.

## Seeing what applies

```sh
agx explain           # what this directory gets, and why
agx skill list        # every configured skill
agx instructions list
```
