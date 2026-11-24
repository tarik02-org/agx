-- +goose Up
CREATE TABLE launch_attachments (
    configuration TEXT NOT NULL,
    launch TEXT NOT NULL,
    mcp TEXT NOT NULL,
    -- The server and secret definitions and environment inputs, sealed under the launch grant's
    -- hash: ad-hoc definitions may carry literal credentials.
    sealed TEXT NOT NULL,
    PRIMARY KEY(configuration, launch, mcp)
);
CREATE INDEX launch_attachments_launch ON launch_attachments(launch);

PRAGMA user_version = 3;
