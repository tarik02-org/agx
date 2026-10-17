# Define harnesses by extending built-ins, with mode-scoped arguments

Status: proposed.

Codex, Claude Code, and Pi remain the built-in harnesses with native adapters.
Configuration defines additional harnesses under `harnesses:`, each with a
`base` naming a built-in or another definition. `weft <name>` and `weft run
<name>` launch it. Names cannot shadow Weft commands; `base` chains reject cycles.
Package manifests cannot define harnesses.

```yaml
harnesses:
  codex:
    executable: /opt/codex
    add: {skills: [workflow]}
  codex-cpa:
    base: codex
    args:
      - [-c, 'model_provider="cpa"']
      - values: [--profile, cpa]
        modes: [interactive, exec, resume]
    env:
      CPA_KEY: !secret cpa-key
    add: {connections: [github]}
    exclude: {skills: [workflow]}
```

Native CLIs accept different flags per invocation, so arguments are grouped and
scoped to modes. Each adapter classifies an invocation into one fixed mode:

| Harness | Modes |
| --- | --- |
| codex | `interactive`, `exec`, `resume`, `app-server`, `mcp-server`, `passthrough` |
| claude | `interactive`, `print`, `resume`, `passthrough` |
| pi | `interactive`, `print`, `rpc`, `passthrough` |

A group without `modes` applies to every mode except `passthrough`. Groups are
inserted before the user's native arguments in order. `env` applies in every mode.
An inherited definition accepts only its base built-in's modes.

Extension layers the child over its parent without conflicts: `executable` is
replaced, argument groups follow the parent's, `env` keys are replaced per key,
and the parent's `add`/`exclude` apply before the child's.

Redefining a name in another fragment merges field by field. `base`,
`executable`, and each `env` key may be set once; argument groups append in
fragment order; `add` and `exclude` accumulate. `override: true` replaces the
earlier definition entirely. This matches daemon settings and resource overrides.

Selection gains a harness dimension. A harness's `add`/`exclude` apply after
directory scopes, so they hold everywhere for that harness. Scope rules accept an
optional `harnesses: [...]` filter, matching a definition or any of its bases
(`codex` matches `codex-cpa`). Resolution therefore takes a harness:
`resolve`/`explain` accept `--harness`, and snapshots identify both directory
and harness.

Rejected: a separate `profiles:` section, which would duplicate executable
selection; unconditional arguments, which break modes such as Codex's
`app-server` that reject `--profile`.
