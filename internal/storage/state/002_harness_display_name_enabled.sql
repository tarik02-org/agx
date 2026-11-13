-- +goose Up
ALTER TABLE local_harnesses ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
ALTER TABLE local_harnesses ADD COLUMN enabled BOOLEAN;

PRAGMA user_version = 2;
