# Weft

Weft composes reusable agent resources according to the directory where a
harness runs.

## Language

**Fragment**: An importable portion of configuration, such as personal, work,
or machine-local settings.

**Scope**: A directory rule that adds resources to, or excludes resources from,
the configuration inherited by that directory and its descendants.
_Avoid_: Environment, profile

**Skill**: A reusable workflow described by `SKILL.md`, together with its
supporting scripts, references, and assets.

**Instructions**: Guidance applied to a launch without requiring skill invocation.

**Package**: A versioned distribution unit containing skills, instructions,
MCP definitions, or bundle definitions.

**Bundle**: A named selection of skills, MCP connections, and instructions.

**Connection**: A named association between an MCP server and the account and
settings used to access it.

**Snapshot**: The resolved selection of resources and settings for a working
directory at launch time.

**Launch**: One running harness instance associated with a snapshot.

**Launch grant**: Authorization to use the selected MCP connections during a launch.

**Lifecycle credential**: Authorization held by the wrapper to renew or finish
one launch, separate from the harness's launch grant.

**Secret**: A named definition of how a credential is obtained, used by a
connection's `auth` field or injected with `!secret`.
_Avoid_: Auth source

**Harness**: A coding agent Weft launches. Built-ins (Codex, Claude Code, Pi) have
native adapters; a harness definition extends one with arguments, environment,
and resource selection.

**Mode**: The kind of invocation an adapter recognizes, such as interactive,
non-interactive, or a protocol server; argument groups apply per mode.
