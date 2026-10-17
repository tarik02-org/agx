# Separate package installation from bundle selection

Treat skills and their supporting files as package resources. A package is a
versioned distribution unit; a bundle selects resources from installed packages.
Several bundles can reference the same resource without installing duplicate
copies. Keep complete skill directories so relative script and reference paths
continue to work.

Record resolved source revisions, dependency relationships, and content hashes
in JSON lockfiles. Install immutable package contents into a shared cache.
Package updates explicitly change the lockfile; harness launches consume the
locked installation rather than silently updating it. Local-only fragments can
use a separate lockfile to avoid changing tracked configuration.

Resolve a launch snapshot before generating harness artifacts. Package updates
affect later launches; they do not mutate resources underneath active launches.
