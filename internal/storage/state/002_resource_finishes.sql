-- +goose Up
CREATE TABLE resource_finishes (
    configuration TEXT NOT NULL REFERENCES daemon_state(configuration) ON DELETE CASCADE,
    launch TEXT NOT NULL,
    finished_at INTEGER NOT NULL,
    PRIMARY KEY(configuration, launch)
);

PRAGMA user_version = 2;
