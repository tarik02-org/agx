# Compose configuration through additive directory scopes

Use a global YAML entry point with explicit, ordered imports for personal, work,
and machine-local fragments. Fragments may be tracked separately or ignored by
Git; machine-specific directory mappings need not enter shared configuration.

Resolve matching scopes from parent directories to descendants. Deeper scopes
add to inherited configuration and may explicitly exclude resources. Expand
bundles before applying exclusions, so a scope can remove one inherited skill
without removing its whole bundle. A deeper scope may explicitly re-add it.
For equally specific rules, use declared import and rule order.

Use qualified resource IDs and explicit overrides rather than generic deep
merging. Retain source and rule provenance for every inclusion and exclusion,
so the CLI and future UI can explain the result. Scopes compose resources;
they do not select replacement environments.
