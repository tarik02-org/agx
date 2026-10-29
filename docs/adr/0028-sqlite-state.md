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

Earlier JSON/JSONL stores and experimental SQLite layouts have no compatibility
reader or import;
existing logins and launch associations must be recreated. Cache data rebuilds
from harness stores. A corrupt durable database is an error, not a reason to
silently replace credentials or revocations.
