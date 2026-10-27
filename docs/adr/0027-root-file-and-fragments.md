# Split configuration into a root file and fragments

Status: accepted. Replaces nested imports ([ADR 0011](0011-straightforward-configuration.md)),
the "each daemon setting from one fragment" rule, and where editing commands put new
definitions ([ADR 0014](0014-configuration-editing-commands.md)).

Any file could import others and hold definitions, so the root file was both the list of
fragments and a place the editing commands wrote to. Generated configuration, such as Home
Manager's drop-in, had to share it, daemon settings needed a rule about which fragment sets
each one, and a new definition landed in the root whatever fragments existed.

## Decision

- **Root file** (`agx.yaml`, `AGX_CONFIG`, `--config`): `imports` and `daemon` only. Its
  schema, `agx.root.schema.json` (`agx config schema --root`), rejects every definition
  section with "the root configuration holds imports and daemon settings only; move skills
  into a fragment such as agx.d/main.yaml". A missing root reads as
  `imports: [agx.d/*.yaml]`; nothing is written on read.
- **Fragments**: everything the root imports. They hold definitions and rules, and cannot
  import or set daemon settings; `agx.schema.json` rejects both keys. Only the root imports,
  so there are no import cycles, and a file matched twice, or the root matched by its own
  glob, loads once. Relative imports are relative to the root as given, so a root linked into
  the Nix store imports the fragments beside the link. Package manifests are unchanged.
- **Where edits go**: a changed or removed definition stays in the file that defines it, and
  `use`/`unuse` edit the first writable matching rule. Anything new goes to the target
  fragment: `--file` (relative to the root's directory; it must be a writable fragment the
  root imports, or a new file its imports match), else the only writable fragment, else a
  new `agx.d/main.yaml` (with the root, when it is missing). With several, a terminal picks
  one, or a new file in `agx.d/`; without a terminal, or with `--json`, the command fails
  with `choose_file` listing them. A new rule no longer goes beside its definition: that
  fragment is often generated or read-only, and the target rule keeps one answer.
- **API**: `PlanChangesRequest.file` and `ApplyChangesRequest.file` name the target for the
  whole request, replacing `DefineChange.file`. Without one and with several candidates the
  plan fails with `FAILED_PRECONDITION`, reason `REASON_CHOOSE_FILE`, and the candidates in
  `ErrorDetail.files`.
- **Home Manager** writes `programs.agx.settings` to its drop-in, and `settings.daemon` to a
  root file holding `imports: [agx.d/*.yaml]` and those settings.

Configurations that put definitions in the root fail validation with that message; there is no
migration.

Rejected: nested imports, which only a configuration repository imported as one file would
use, and which the root's globs cover; keeping definitions allowed in the root, which keeps
the question of where new ones go.
