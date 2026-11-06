-- +goose Up
CREATE TABLE session_configurations (
    configuration TEXT NOT NULL,
    launch TEXT NOT NULL,
    owner TEXT NOT NULL UNIQUE REFERENCES configurations(root) ON DELETE CASCADE,
    PRIMARY KEY(configuration, launch)
);
CREATE INDEX session_configurations_launch ON session_configurations(launch);
CREATE TABLE session_environment (
    owner TEXT NOT NULL REFERENCES configurations(root) ON DELETE CASCADE,
    mcp TEXT NOT NULL,
    name TEXT NOT NULL,
    ciphertext TEXT NOT NULL,
    PRIMARY KEY(owner, mcp, name)
);
PRAGMA user_version = 2;
