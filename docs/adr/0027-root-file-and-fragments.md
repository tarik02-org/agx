# Split configuration into a root file and fragments

Status: accepted. Replaces nested imports ([ADR 0011](0011-straightforward-configuration.md)),
the "each daemon setting from one fragment" rule, and where editing commands put new
definitions ([ADR 0014](0014-configuration-editing-commands.md)).

Any file could import others (`imports`) and hold definitions, so the root file was
both the list of fragments and a place the editing commands wrote to. Generated
configuration, such as Home Manager's drop-in, had to share it, daemon settings needed a rule
about which fragment sets each one, and a new definition landed in the root whatever
fragments existed.

## Decision

- **Root file** (`agx.yaml`, `AGX_CONFIG`, `--config`): `fragments` and `daemon` only. Its
  schema, `agx.root.schema.json` (`agx config schema --root`), rejects every definition
  section with "the root configuration holds fragments and daemon settings only; move skills
  into a fragment such as agx.d/main.yaml". A missing root reads as
  `fragments: [agx.d/*.yaml]`; nothing is written on read. The key is `fragments`, not
  `imports`, because it names what the root lists and fragments no longer import anything.
- **Fragments**: everything the root lists. They hold definitions and rules, and cannot
  list fragments or set daemon settings; `agx.schema.json` rejects both keys. Only the root
  lists fragments, so there are no cycles, and a file matched twice, or the root matched by its own
  glob, loads once. Relative entries are relative to the root as given, so a root linked into
  the Nix store loads the fragments beside the link. Package manifests are unchanged.
- **Where edits go**: a changed or removed definition stays in the file that defines it, and
  `use`/`unuse` edit the first writable matching rule. Anything new goes to the target
  fragment: `--file` (relative to the root's directory; it must be a writable fragment the
  root lists, or a new file its globs match), else the only writable fragment, else a
  new `agx.d/main.yaml` (with the root, when it is missing). With several, a terminal picks
  one, or a new file in `agx.d/`; without a terminal, or with `--json`, the command fails
  with `choose_file` listing them. A new rule no longer goes beside its definition: that
  fragment is often generated or read-only, and the target rule keeps one answer.
- **API**: `PlanChangesRequest.file` and `ApplyChangesRequest.file` name the target for the
  whole request, replacing `DefineChange.file`. Without one and with several candidates the
  plan fails with `FAILED_PRECONDITION` and the candidates in
  `ErrorDetail.files`.
- **Home Manager** writes `programs.agx.settings` to its drop-in, and `settings.daemon` to a
  root file holding `fragments: [agx.d/*.yaml]` and those settings.

Configurations that put definitions in the root fail validation with that message; there is no
migration.

Rejected: nested imports, which only a configuration repository imported as one file would
use, and which the root's globs cover; keeping definitions allowed in the root, which keeps
the question of where new ones go.
