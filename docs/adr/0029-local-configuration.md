# Local configuration with declarative overlays

Status: accepted. Replaces the YAML editing engine and placement policy in
[ADR 0014](0014-configuration-editing-commands.md) and
[ADR 0027](0027-root-file-and-fragments.md).

CLI and web-UI changes used to require choosing a writable fragment, preserving
YAML layout and comments, planning file diffs, and comparing hashes before
applying. These concerns disappear when machine-managed configuration has its
own store. Declarative configuration still needs portable, reviewable YAML.

Local configuration uses relational tables scoped to a canonical root path, with
a monotonic revision. Definitions have typed columns and ordered child rows for
commands, injected values, arguments and selections. Scope rules, harness filters,
package locks and dependency edges have their own rows. Arbitrary native settings
and skill frontmatter remain JSON because their schemas belong to other tools.
Commands and the web UI stage semantic edits; both commit through the daemon.
The CLI reads local definitions and typed package locks through RPC, and its
preview session has no state directory from which to open SQLite. The existing loader validates staged local data together
with packages and YAML before a revision-checked transaction commits. Concurrent
edits fail instead of losing a writer's changes. Reads use a consistent read-only
transaction. Writes replace only changed definitions, scope rules and lock entries;
foreign keys remove owned inline declarations and child rows with their parent.
Resource references remain names, because they may resolve through built-ins,
packages or YAML overlays rather than another local row. A missing database stays absent
on read. Configuration watches compare this root's revision, so unrelated
credential and launch writes do not invalidate it.

Built-ins load first, then local data, then YAML overlays. A YAML definition
replaces a same-name local definition as a whole, including a harness. Within
YAML, existing conflict/override and harness field-merge rules remain. Scope
rules retain directory specificity; YAML follows local rules at equal
specificity. Agx refuses edits of YAML-owned or package-owned definitions and
names the source. Opposing YAML scope rules at the same directory are refused
by `use`/`unuse` as well. Removing an overlay reveals any local definition beneath it.

The protocol accepts semantic changes in `UpdateConfiguration`, with an expected
revision and structured definition values. Injection tags use explicit `$env`,
`$file`, and `$secret` objects. There are no file plans, YAML text editing,
fragment choosers, or diff/hash apply protocol. Configured resource file contents
have a separate write RPC; package and cache contents remain read-only.

Local packages keep lock entries in the same transaction as their declarations.
YAML packages retain portable lockfiles. Package fetching can populate the cache
before a local transaction commits; those immutable artifacts can be reused or
collected. Edits that need changed overlay locks require `agx install` first.
`agx config export` emits local definitions/rules as a YAML overlay for deliberate
promotion into declarative configuration; it never mirrors edits into YAML.

Embedded Goose migrations and sqlc queries share the schema source with runtime
storage (ADR 0028). Opening an existing runtime-only database upgrades it before
reading local configuration; reading a missing database leaves it absent. No import
of legacy files or the experimental JSON-in-SQLite layout is provided. Existing YAML remains
an overlay, not imported local data. Credentials remain encrypted under the OS
keychain master key. `secret logout --all` removes every credential, including
orphaned definitions, without deleting configuration or launch history.

Rejected: continuing to edit YAML alongside the database, which keeps two
mutable authorities; deep-merging local and YAML definitions, which makes it
hard to tell which fields a declaration controls; storing secret plaintext in
the configuration document instead of the encrypted credential store.
