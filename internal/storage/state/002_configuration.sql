-- +goose Up

CREATE TABLE configurations (
    root TEXT NOT NULL,
    revision INTEGER NOT NULL,
    PRIMARY KEY(root)
);

CREATE TABLE local_scopes (
    root TEXT NOT NULL,
    position INTEGER NOT NULL,
    directory TEXT NOT NULL,
    PRIMARY KEY(root, position),
    FOREIGN KEY(root) REFERENCES configurations(root) ON DELETE CASCADE
);

CREATE TABLE local_definitions (
    root TEXT NOT NULL,
    key TEXT NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('skills','instructions','mcp','secrets','bundles','packages','harnesses')),
    name TEXT NOT NULL,
    parent TEXT,
    scope INTEGER,
    override BOOLEAN NOT NULL,
    PRIMARY KEY(root, key),
    FOREIGN KEY(root) REFERENCES configurations(root) ON DELETE CASCADE,
    FOREIGN KEY(root, parent) REFERENCES local_definitions(root, key) ON DELETE CASCADE,
    FOREIGN KEY(root, scope) REFERENCES local_scopes(root, position) ON DELETE CASCADE,
    CHECK(parent IS NULL OR scope IS NULL)
);

CREATE UNIQUE INDEX local_definition_names ON local_definitions(root,kind,name) WHERE parent IS NULL AND scope IS NULL;

CREATE TABLE local_skills (
    root TEXT NOT NULL,
    key TEXT NOT NULL,
    path TEXT,
    git TEXT,
    ref TEXT NOT NULL,
    hash TEXT NOT NULL,
    native_name TEXT NOT NULL,
    description TEXT NOT NULL,
    content TEXT,
    frontmatter TEXT CHECK(frontmatter IS NULL OR json_valid(frontmatter)),
    PRIMARY KEY(root, key),
    FOREIGN KEY(root, key) REFERENCES local_definitions(root, key) ON DELETE CASCADE
);

CREATE TABLE local_instructions (
    root TEXT NOT NULL,
    key TEXT NOT NULL,
    path TEXT,
    git TEXT,
    ref TEXT NOT NULL,
    hash TEXT NOT NULL,
    text TEXT,
    sort_order INTEGER NOT NULL,
    PRIMARY KEY(root, key),
    FOREIGN KEY(root, key) REFERENCES local_definitions(root, key) ON DELETE CASCADE
);

CREATE TABLE local_packages (
    root TEXT NOT NULL,
    key TEXT NOT NULL,
    path TEXT,
    git TEXT,
    ref TEXT NOT NULL,
    hash TEXT NOT NULL,
    subdir TEXT NOT NULL,
    PRIMARY KEY(root, key),
    FOREIGN KEY(root, key) REFERENCES local_definitions(root, key) ON DELETE CASCADE
);

CREATE TABLE local_harnesses (
    root TEXT NOT NULL,
    key TEXT NOT NULL,
    base TEXT NOT NULL,
    executable TEXT NOT NULL,
    hidden BOOLEAN,
    settings TEXT CHECK(settings IS NULL OR json_valid(settings)),
    PRIMARY KEY(root, key),
    FOREIGN KEY(root, key) REFERENCES local_definitions(root, key) ON DELETE CASCADE
);

CREATE TABLE local_mcp (
    root TEXT NOT NULL,
    key TEXT NOT NULL,
    url TEXT NOT NULL,
    cwd TEXT NOT NULL,
    description TEXT NOT NULL,
    load TEXT NOT NULL,
    share_type TEXT NOT NULL,
    share_idle TEXT,
    auth_kind TEXT NOT NULL,
    auth_value TEXT NOT NULL,
    inline_secret TEXT,
    PRIMARY KEY(root, key),
    FOREIGN KEY(root, key) REFERENCES local_definitions(root, key) ON DELETE CASCADE,
    FOREIGN KEY(root, inline_secret) REFERENCES local_definitions(root, key),
    CHECK(auth_kind IN ('default','none','literal','env','file','secret','inline')),
    CHECK((auth_kind = 'inline') = (inline_secret IS NOT NULL))
);

CREATE TABLE local_secrets (
    root TEXT NOT NULL,
    key TEXT NOT NULL,
    secret_kind TEXT NOT NULL,
    file TEXT,
    age_key_kind TEXT NOT NULL,
    age_key_value TEXT NOT NULL,
    issuer TEXT NOT NULL,
    client_id TEXT NOT NULL,
    resource TEXT NOT NULL,
    PRIMARY KEY(root, key),
    FOREIGN KEY(root, key) REFERENCES local_definitions(root, key) ON DELETE CASCADE
);

CREATE TABLE local_skill_files (
    root TEXT NOT NULL,
    key TEXT NOT NULL,
    name TEXT NOT NULL,
    content TEXT NOT NULL,
    path TEXT NOT NULL,
    executable BOOLEAN NOT NULL,
    PRIMARY KEY(root,key,name),
    FOREIGN KEY(root, key) REFERENCES local_definitions(root, key) ON DELETE CASCADE
);

CREATE TABLE local_commands (
    root TEXT NOT NULL,
    key TEXT NOT NULL,
    position INTEGER NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY(root,key,position),
    FOREIGN KEY(root, key) REFERENCES local_definitions(root, key) ON DELETE CASCADE
);

CREATE TABLE local_environment (
    root TEXT NOT NULL,
    key TEXT NOT NULL,
    name TEXT NOT NULL,
    value_kind TEXT NOT NULL CHECK(value_kind IN ('literal','env','file','secret')),
    value TEXT NOT NULL,
    PRIMARY KEY(root,key,name),
    FOREIGN KEY(root, key) REFERENCES local_definitions(root, key) ON DELETE CASCADE
);

CREATE TABLE local_headers (
    root TEXT NOT NULL,
    key TEXT NOT NULL,
    name TEXT NOT NULL,
    value_kind TEXT NOT NULL CHECK(value_kind IN ('literal','env','file','secret')),
    value TEXT NOT NULL,
    PRIMARY KEY(root,key,name),
    FOREIGN KEY(root, key) REFERENCES local_definitions(root, key) ON DELETE CASCADE
);

CREATE TABLE local_arg_groups (
    root TEXT NOT NULL,
    key TEXT NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY(root,key,position),
    FOREIGN KEY(root, key) REFERENCES local_definitions(root, key) ON DELETE CASCADE
);

CREATE TABLE local_arguments (
    root TEXT NOT NULL,
    key TEXT NOT NULL,
    group_position INTEGER NOT NULL,
    position INTEGER NOT NULL,
    value_kind TEXT NOT NULL CHECK(value_kind IN ('literal','env','file','secret')),
    value TEXT NOT NULL,
    PRIMARY KEY(root,key,group_position,position),
    FOREIGN KEY(root,key,group_position) REFERENCES local_arg_groups(root,key,position) ON DELETE CASCADE
);

CREATE TABLE local_argument_modes (
    root TEXT NOT NULL,
    key TEXT NOT NULL,
    group_position INTEGER NOT NULL,
    position INTEGER NOT NULL,
    mode TEXT NOT NULL,
    PRIMARY KEY(root,key,group_position,position),
    FOREIGN KEY(root,key,group_position) REFERENCES local_arg_groups(root,key,position) ON DELETE CASCADE
);

CREATE TABLE local_secret_keys (
    root TEXT NOT NULL,
    key TEXT NOT NULL,
    position INTEGER NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY(root,key,position),
    FOREIGN KEY(root, key) REFERENCES local_definitions(root, key) ON DELETE CASCADE
);

CREATE TABLE local_secret_scopes (
    root TEXT NOT NULL,
    key TEXT NOT NULL,
    position INTEGER NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY(root,key,position),
    FOREIGN KEY(root, key) REFERENCES local_definitions(root, key) ON DELETE CASCADE
);

CREATE TABLE local_scope_harnesses (
    root TEXT NOT NULL,
    scope_position INTEGER NOT NULL,
    position INTEGER NOT NULL,
    name TEXT NOT NULL,
    PRIMARY KEY(root,scope_position,position),
    FOREIGN KEY(root,scope_position) REFERENCES local_scopes(root,position) ON DELETE CASCADE
);

CREATE TABLE local_definition_items (
    root TEXT NOT NULL,
    key TEXT NOT NULL,
    action TEXT NOT NULL CHECK(action IN ('add','remove')),
    kind TEXT NOT NULL CHECK(kind IN ('skills','instructions','mcp','bundles')),
    position INTEGER NOT NULL,
    reference TEXT,
    inline_key TEXT,
    PRIMARY KEY(root,key,action,kind,position),
    FOREIGN KEY(root, key) REFERENCES local_definitions(root, key) ON DELETE CASCADE,
    FOREIGN KEY(root,inline_key) REFERENCES local_definitions(root,key),
    CHECK((reference IS NULL) <> (inline_key IS NULL))
);

CREATE TABLE local_scope_items (
    root TEXT NOT NULL,
    scope_position INTEGER NOT NULL,
    action TEXT NOT NULL CHECK(action IN ('add','remove')),
    kind TEXT NOT NULL CHECK(kind IN ('skills','instructions','mcp','bundles')),
    position INTEGER NOT NULL,
    reference TEXT,
    inline_key TEXT,
    PRIMARY KEY(root,scope_position,action,kind,position),
    FOREIGN KEY(root,scope_position) REFERENCES local_scopes(root,position) ON DELETE CASCADE,
    FOREIGN KEY(root,inline_key) REFERENCES local_definitions(root,key),
    CHECK((reference IS NULL) <> (inline_key IS NULL))
);

CREATE TABLE local_package_locks (
    root TEXT NOT NULL,
    id TEXT NOT NULL,
    path TEXT NOT NULL,
    git TEXT NOT NULL,
    ref TEXT NOT NULL,
    source_hash TEXT NOT NULL,
    subdir TEXT NOT NULL,
    revision TEXT NOT NULL,
    hash TEXT NOT NULL,
    PRIMARY KEY(root,id),
    FOREIGN KEY(root) REFERENCES configurations(root) ON DELETE CASCADE
);

CREATE TABLE local_package_dependencies (
    root TEXT NOT NULL,
    package TEXT NOT NULL,
    position INTEGER NOT NULL,
    dependency TEXT NOT NULL,
    PRIMARY KEY(root,package,position),
    FOREIGN KEY(root,package) REFERENCES local_package_locks(root,id) ON DELETE CASCADE
);

PRAGMA user_version = 2;
