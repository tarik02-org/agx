-- name: GetSecret :one
SELECT ciphertext FROM secrets WHERE key = ?;
-- name: PutSecret :exec
INSERT INTO secrets(key, ciphertext) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET ciphertext = excluded.ciphertext;
-- name: DeleteSecret :exec
DELETE FROM secrets WHERE key = ?;
-- name: HasSecrets :one
SELECT EXISTS(SELECT 1 FROM secrets);
-- name: StartLaunch :exec
INSERT INTO launches(id, harness, builtin, store, directory, started, finished, exact)
VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO NOTHING;
-- name: FinishLaunch :exec
UPDATE launches SET finished = ? WHERE id = ?;
-- name: AddLaunchSession :exec
INSERT INTO launch_sessions(builtin, store, session, launch)
SELECT l.builtin, l.store, sqlc.arg(session), l.id FROM launches AS l WHERE l.id = sqlc.arg(launch)
ON CONFLICT(builtin, store, session) DO UPDATE SET launch = excluded.launch
WHERE (SELECT started FROM launches WHERE id = excluded.launch) >
      (SELECT started FROM launches WHERE id = launch_sessions.launch);
-- name: DeleteLaunchSession :exec
DELETE FROM launch_sessions WHERE launch = ? AND session = ?;
-- name: ListLaunches :many
SELECT * FROM launches ORDER BY id;
-- name: ListLaunchSessions :many
SELECT * FROM launch_sessions ORDER BY launch, session;
-- name: ListStoreLaunchSessions :many
SELECT ls.launch, ls.session, l.harness
FROM launch_sessions AS ls JOIN launches AS l ON l.id = ls.launch
WHERE l.builtin = ? AND l.store = ?
ORDER BY l.started DESC, l.id DESC;
-- name: MatchSession :one
SELECT id, harness FROM launches
WHERE builtin = sqlc.arg(builtin) AND store = sqlc.arg(store) AND directory = sqlc.arg(directory)
  AND exact = 0
  AND started <= CAST(sqlc.arg(session_started) AS INTEGER) + CAST(sqlc.arg(skew) AS INTEGER)
  AND coalesce(finished, started + sqlc.arg(max_launch)) >= CAST(sqlc.arg(session_started) AS INTEGER) - CAST(sqlc.arg(skew) AS INTEGER)
ORDER BY started DESC, id DESC LIMIT 1;
-- name: PruneLaunches :exec
DELETE FROM launches
WHERE launches.builtin = sqlc.arg(builtin) AND launches.store = sqlc.arg(store)
  AND coalesce(finished, started + sqlc.arg(max_launch)) < CAST(sqlc.arg(now) AS INTEGER) - CAST(sqlc.arg(skew) AS INTEGER)
  AND NOT EXISTS(SELECT 1 FROM launch_sessions WHERE launch = launches.id);
-- name: GetDaemonPort :one
SELECT port FROM daemon_state WHERE configuration = ?;
-- name: PutDaemonPort :exec
INSERT INTO daemon_state(configuration, port) VALUES (?, ?)
ON CONFLICT(configuration) DO UPDATE SET port = excluded.port;
-- name: ListRevocations :many
SELECT launch, revoked_at FROM revocations WHERE configuration = ? AND revoked_at > ?;
-- name: PutRevocation :exec
INSERT INTO revocations(configuration, launch, revoked_at) VALUES (?, ?, ?)
ON CONFLICT(configuration, launch) DO UPDATE SET revoked_at = max(revocations.revoked_at, excluded.revoked_at);
-- name: PruneRevocations :exec
DELETE FROM revocations WHERE configuration = ? AND revoked_at <= ?;
