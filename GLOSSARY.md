# agx

agx composes reusable agent resources according to the directory where a
harness runs.

## Language

**Fragment**: An importable portion of configuration, such as personal, work,
or machine-local settings.

**Scope**: A directory rule that adds resources to, or removes resources from,
the configuration inherited by that directory and its descendants.
_Avoid_: Environment, profile

**Skill**: A reusable workflow described by `SKILL.md`, together with its
supporting scripts, references, and assets.

**Instructions**: Guidance applied to a launch without requiring skill invocation.

**Package**: A versioned distribution unit containing skills, instructions,
MCP definitions, or bundle definitions.

**Bundle**: A named selection of skills, MCP servers, and instructions.

**Inline definition**: A skill, instructions, or MCP server defined inside a
selection list rather than its top-level section; its ID shares the top-level
ID space and cannot be redefined.

**Generated skill**: A skill written in configuration with `content` and
optional `files`, materialized as a skill directory for each launch.

**Anonymous package**: The Git source of a skill or instructions definition,
fetched and locked like a package under the resource's kind and ID.

**Pinned source**: A package or Git source whose content is fixed by a full
commit `rev`, a content `hash`, or a `/nix/store` path, so it needs no lock entry.

**MCP server**: A named entry in the `mcp` configuration section: an MCP server
together with the account and settings used to reach it, so `github-work` and
`github-personal` may point at the same server.
_Avoid_: Connection

**Snapshot**: The resolved selection of resources and settings for a working
directory at launch time.

**Launch**: One running harness instance associated with a snapshot.

**Launch grant**: Authorization to use the selected MCP servers during a launch.

**Lifecycle credential**: Authorization held by the wrapper to renew or finish
one launch, separate from the harness's launch grant.

**Secret**: A definition of how a credential is obtained: under `secrets`, shared
through `!secret`, or inline in an MCP server's `auth`, where it belongs to that
entry and takes its ID.
_Avoid_: Auth source

**On-demand OAuth**: The OAuth secret of an HTTP MCP server without `auth`, which
needs a login only once the server answers with an authorization challenge.

**Injected value**: A configuration value written with `!env`, `!file`, or
`!secret` and resolved at launch; snapshots and explanations keep only the tag.

**Harness**: A coding agent agx launches. Built-ins (Codex, Claude Code, Pi) have
native adapters; a harness definition extends one with arguments, environment,
and resource selection.

**Adapter**: The package that integrates one built-in harness: it classifies
invocations into modes and turns a snapshot into that harness's native
arguments, files, and plugins.

**Shim**: An executable named after a harness definition that runs agx as that
definition, so the definition works from any shell or tool.

**Mode**: The kind of invocation an adapter recognizes, such as interactive,
non-interactive, or a protocol server; argument groups apply per mode.
