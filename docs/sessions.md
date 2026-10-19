# Launcher and sessions

## The launcher

Run `agx` on its own in a terminal:

```sh
agx
```

A full-screen picker opens with one search field and two lists:

- **New session**: the built-in harnesses and your harness definitions, except
  [hidden](harnesses.md#hiding-harnesses-from-the-launcher) ones. Enter starts
  one in the current directory.
- **Sessions**: earlier Codex, Claude Code, and Pi conversations, with their
  harness, age, title or first message, and directory. Enter resumes one.

| Key | Action |
| --- | --- |
| type | filter harnesses and sessions |
| ↑ ↓, Ctrl-P Ctrl-N, PgUp PgDn | move |
| Enter | start the harness, or resume the session |
| Tab | show sessions from this directory and below, or from everywhere |
| Ctrl-H | resume with another harness based on the same one |
| Esc | quit; in a sub-picker, go back |
| Ctrl-C | quit |

Resuming changes to the session's directory and runs the harness's own resume
command through agx (`agx codex resume <id>`, `agx claude --resume <id>`,
`agx pi --session <id>`), so the session gets that directory's resources. A
session started with one of your harness definitions resumes with the same
definition, as long as agx started it and the definition still exists.

When stdin or stdout is not a terminal, `agx` prints its help instead.

## Listing sessions

```sh
agx sessions                  # this directory and below, newest first
agx sessions --all            # every directory
agx sessions auth bug         # sessions matching every word
agx sessions --all --json
```

A query matches words against the harness, title, first message, or directory,
and sorts the best matches first.

Sessions are read from each harness's own storage, so sessions started without
agx show up too. agx honours `CODEX_HOME`, `CLAUDE_CONFIG_DIR`,
`PI_CODING_AGENT_DIR`, and Pi's session directory setting. It only reads them;
it never changes or deletes sessions. Subagent sessions are not listed.
