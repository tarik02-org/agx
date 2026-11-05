-- +goose Up
CREATE TABLE provider_sessions (
    source_id TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    builtin TEXT NOT NULL,
    harness TEXT NOT NULL,
    session_id TEXT NOT NULL,
    directory TEXT NOT NULL,
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    started INTEGER NOT NULL,
    updated INTEGER NOT NULL,
    PRIMARY KEY(source_id, session_id)
);
CREATE INDEX provider_sessions_updated ON provider_sessions(updated DESC);
PRAGMA user_version = 2;
