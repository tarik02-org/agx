-- +goose Up
CREATE TABLE session_files (
    store TEXT NOT NULL,
    path TEXT NOT NULL,
    size INTEGER NOT NULL,
    modified INTEGER NOT NULL,
    version INTEGER NOT NULL,
    malformed BOOLEAN NOT NULL,
    listed BOOLEAN NOT NULL,
    session_id TEXT NOT NULL,
    directory TEXT NOT NULL,
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    started INTEGER,
    updated INTEGER,
    PRIMARY KEY(store, path)
);
CREATE INDEX session_files_directory ON session_files(store, directory, updated DESC);
CREATE INDEX session_files_updated ON session_files(store, updated DESC);
CREATE TABLE icons (id TEXT PRIMARY KEY, source TEXT NOT NULL, icon TEXT NOT NULL);
PRAGMA user_version = 1;
