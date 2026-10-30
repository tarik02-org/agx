# Keep machine-managed state in SQLite

Status: accepted. Replaces the JSON secret store in ADR 0026, session index and
launch-record persistence in ADR 0013, and daemon-state persistence in ADR 0016.

agx needs atomic updates from wrappers and multiple configuration daemons.
JSON rewrites, append-log folding, file locks, and cache-file replacement checks
implemented database behavior separately in each store.

Use `modernc.org/sqlite`, keeping releases free of cgo. Durable state lives in
`<state>/state.db`: encrypted secrets, launches, session associations, daemon ports, and
revocations. Rebuildable session-file metadata and remembered MCP icons live in
`<cache>/cache.db`. Stable session metadata and launch fields use columns and indexes, with a child
table for launch/session associations. Launch matching inserts and removes only
changed associations; it never rewrites the retained launch collection. Historical
launches do not depend on mutable configuration or disposable cache rows.
SQLite transactions serialize writers. Parsing runs outside
write transactions; session cache updates compare the metadata they read before
replacing another scanner's work. Incomplete scans never prune unseen files.

Secrets retain AES-256-GCM encryption with their record key as authenticated
additional data. The OS keychain still holds only the master key, requested
lazily. Database files and their parent directories are private. SQLite stores
ciphertext, never decrypted credentials.

Only `agxd` daemon processes open these databases or perform migrations. The
`agx` client has no SQLite or Goose dependency. Persistence lives in
`configstore`, `secretstore` and `sessionstore`, separate from configuration
loading, authentication and session models so a future local runtime can reuse
the core without durable storage. CLI commands
obtain managed definitions and locks through RPC, stage YAML validation locally,
and send semantic changes for the daemon to validate and commit. Native plugin
installation stays with the caller so its PATH and environment keep deciding
which native installation is updated. Passthrough environment resolution also
runs in the daemon. Shell completion never starts a daemon; the terminal picker
loads configuration and history asynchronously after its first frame.

Wrappers report start and finish history through a management-authorized,
idempotent RPC independent of active leases or current configuration validity.
The wrapper captures its exit timestamp before reconnecting; completion can start
a daemon after a graceful stop or crash. Replaying a start cannot reopen finished
history. There is no direct-SQLite or history-file fallback in the CLI.

Connections and databases open only when used. Short transactions use SQLite's
normal rollback journal and a busy timeout; ordinary store operations need no
application file lock. Embedded Goose migrations define both database schemas, and sqlc generates typed
Go queries from those migrations. A schema-version check skips migration work
on ordinary opens. Only upgrades take a cross-process migration lock, because
Goose checks versions outside its migration transactions. Each migration is
transactional; failed upgrades preserve the previous schema and data. A newer
schema is refused by an older binary. Discovery
and the daemon ownership lock remain files, because callers need them before
connecting. Resource contents, immutable artifacts, and portable package
lockfiles remain files as well.

The unshipped runtime and configuration schemas are squashed into one initial
state migration. Future released upgrades use forward migrations. Earlier
JSON/JSONL stores and experimental SQLite layouts have no compatibility reader
or import;
existing logins and launch associations must be recreated. Cache data rebuilds
from harness stores. A corrupt durable database is an error, not a reason to
silently replace credentials or revocations.
