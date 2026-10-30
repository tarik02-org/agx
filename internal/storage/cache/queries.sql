-- name: ListSessionFiles :many
SELECT * FROM session_files WHERE store = ?;
-- name: DeleteSessionFile :exec
DELETE FROM session_files WHERE store = ? AND path = ? AND modified = ? AND size = ? AND version = ?;
-- name: InsertSessionFile :execrows
INSERT INTO session_files(store, path, size, modified, version, malformed, listed,
    session_id, directory, title, message, started, updated)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING;
-- name: UpdateSessionFile :execrows
UPDATE session_files SET size = sqlc.arg(size), modified = sqlc.arg(modified), version = sqlc.arg(version),
    malformed = sqlc.arg(malformed), listed = sqlc.arg(listed), session_id = sqlc.arg(session_id),
    directory = sqlc.arg(directory), title = sqlc.arg(title), message = sqlc.arg(message),
    started = sqlc.arg(started), updated = sqlc.arg(updated)
WHERE store = sqlc.arg(store) AND path = sqlc.arg(path)
  AND size = sqlc.arg(original_size) AND modified = sqlc.arg(original_modified) AND version = sqlc.arg(original_version);
-- name: ListIcons :many
SELECT * FROM icons;
-- name: DeleteIcon :exec
DELETE FROM icons WHERE id = ?;
-- name: PutIcon :exec
INSERT INTO icons(id, source, icon) VALUES (?, ?, ?)
ON CONFLICT(id) DO UPDATE SET source = excluded.source, icon = excluded.icon;
