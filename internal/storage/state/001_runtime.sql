-- +goose Up
CREATE TABLE secrets (key TEXT PRIMARY KEY, ciphertext TEXT NOT NULL);
CREATE TABLE launches (
    id TEXT PRIMARY KEY,
    harness TEXT NOT NULL,
    builtin TEXT NOT NULL,
    store TEXT NOT NULL,
    directory TEXT NOT NULL,
    started INTEGER NOT NULL,
    finished INTEGER,
    exact BOOLEAN NOT NULL,
    UNIQUE(id, builtin, store)
);
CREATE INDEX launches_matching ON launches(builtin, store, directory, started DESC);
CREATE TABLE launch_sessions (
    builtin TEXT NOT NULL,
    store TEXT NOT NULL,
    session TEXT NOT NULL,
    launch TEXT NOT NULL,
    PRIMARY KEY(builtin, store, session),
    FOREIGN KEY(launch, builtin, store) REFERENCES launches(id, builtin, store) ON DELETE CASCADE
);
CREATE INDEX launch_sessions_launch ON launch_sessions(launch, session);
CREATE INDEX launch_sessions_session ON launch_sessions(session);
CREATE TABLE daemon_state (configuration TEXT PRIMARY KEY, port INTEGER NOT NULL);
CREATE TABLE revocations (
    configuration TEXT NOT NULL REFERENCES daemon_state(configuration) ON DELETE CASCADE,
    launch TEXT NOT NULL,
    revoked_at INTEGER NOT NULL,
    PRIMARY KEY(configuration, launch)
);
PRAGMA user_version = 1;
