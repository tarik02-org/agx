-- name: ListLocalScopes :many
SELECT * FROM local_scopes WHERE root = ? ORDER BY root, position;

-- name: InsertLocalScope :exec
INSERT INTO local_scopes(root, position, directory) VALUES (?, ?, ?);

-- name: ListLocalDefinitions :many
SELECT * FROM local_definitions WHERE root = ? ORDER BY root, key;

-- name: InsertLocalDefinition :exec
INSERT INTO local_definitions(root, key, kind, name, parent, scope, override) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListLocalSkills :many
SELECT * FROM local_skills WHERE root = ? ORDER BY root, key;

-- name: InsertLocalSkill :exec
INSERT INTO local_skills(root, key, path, git, ref, hash, native_name, description, content, frontmatter) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListLocalInstructions :many
SELECT * FROM local_instructions WHERE root = ? ORDER BY root, key;

-- name: InsertLocalInstruction :exec
INSERT INTO local_instructions(root, key, path, git, ref, hash, text, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListLocalPackages :many
SELECT * FROM local_packages WHERE root = ? ORDER BY root, key;

-- name: InsertLocalPackage :exec
INSERT INTO local_packages(root, key, path, git, ref, hash, subdir) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListLocalHarnesses :many
SELECT * FROM local_harnesses WHERE root = ? ORDER BY root, key;

-- name: InsertLocalHarness :exec
INSERT INTO local_harnesses(root, key, base, executable, hidden, settings) VALUES (?, ?, ?, ?, ?, ?);

-- name: ListLocalMCPs :many
SELECT * FROM local_mcp WHERE root = ? ORDER BY root, key;

-- name: InsertLocalMCP :exec
INSERT INTO local_mcp(root, key, url, cwd, description, load, share_type, share_idle, auth_kind, auth_value, inline_secret) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListLocalSecrets :many
SELECT * FROM local_secrets WHERE root = ? ORDER BY root, key;

-- name: InsertLocalSecret :exec
INSERT INTO local_secrets(root, key, secret_kind, file, age_key_kind, age_key_value, issuer, client_id, resource) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListLocalSkillFiles :many
SELECT * FROM local_skill_files WHERE root = ? ORDER BY root,key,name;

-- name: InsertLocalSkillFile :exec
INSERT INTO local_skill_files(root, key, name, content, path, executable) VALUES (?, ?, ?, ?, ?, ?);

-- name: ListLocalCommands :many
SELECT * FROM local_commands WHERE root = ? ORDER BY root,key,position;

-- name: InsertLocalCommand :exec
INSERT INTO local_commands(root, key, position, value) VALUES (?, ?, ?, ?);

-- name: ListLocalEnvironments :many
SELECT * FROM local_environment WHERE root = ? ORDER BY root,key,name;

-- name: InsertLocalEnvironment :exec
INSERT INTO local_environment(root, key, name, value_kind, value) VALUES (?, ?, ?, ?, ?);

-- name: ListLocalHeaders :many
SELECT * FROM local_headers WHERE root = ? ORDER BY root,key,name;

-- name: InsertLocalHeader :exec
INSERT INTO local_headers(root, key, name, value_kind, value) VALUES (?, ?, ?, ?, ?);

-- name: ListLocalArgGroups :many
SELECT * FROM local_arg_groups WHERE root = ? ORDER BY root,key,position;

-- name: InsertLocalArgGroup :exec
INSERT INTO local_arg_groups(root, key, position) VALUES (?, ?, ?);

-- name: ListLocalArguments :many
SELECT * FROM local_arguments WHERE root = ? ORDER BY root,key,group_position,position;

-- name: InsertLocalArgument :exec
INSERT INTO local_arguments(root, key, group_position, position, value_kind, value) VALUES (?, ?, ?, ?, ?, ?);

-- name: ListLocalArgumentModes :many
SELECT * FROM local_argument_modes WHERE root = ? ORDER BY root,key,group_position,position;

-- name: InsertLocalArgumentMode :exec
INSERT INTO local_argument_modes(root, key, group_position, position, mode) VALUES (?, ?, ?, ?, ?);

-- name: ListLocalSecretKeys :many
SELECT * FROM local_secret_keys WHERE root = ? ORDER BY root,key,position;

-- name: InsertLocalSecretKey :exec
INSERT INTO local_secret_keys(root, key, position, value) VALUES (?, ?, ?, ?);

-- name: ListLocalSecretScopes :many
SELECT * FROM local_secret_scopes WHERE root = ? ORDER BY root,key,position;

-- name: InsertLocalSecretScope :exec
INSERT INTO local_secret_scopes(root, key, position, value) VALUES (?, ?, ?, ?);

-- name: ListLocalScopeHarnesses :many
SELECT * FROM local_scope_harnesses WHERE root = ? ORDER BY root,scope_position,position;

-- name: InsertLocalScopeHarness :exec
INSERT INTO local_scope_harnesses(root, scope_position, position, name) VALUES (?, ?, ?, ?);

-- name: ListLocalDefinitionItems :many
SELECT * FROM local_definition_items WHERE root = ? ORDER BY root,key,action,kind,position;

-- name: InsertLocalDefinitionItem :exec
INSERT INTO local_definition_items(root, key, action, kind, position, reference, inline_key) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListLocalScopeItems :many
SELECT * FROM local_scope_items WHERE root = ? ORDER BY root,scope_position,action,kind,position;

-- name: InsertLocalScopeItem :exec
INSERT INTO local_scope_items(root, scope_position, action, kind, position, reference, inline_key) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListLocalPackageLocks :many
SELECT * FROM local_package_locks WHERE root = ? ORDER BY root,id;

-- name: InsertLocalPackageLock :exec
INSERT INTO local_package_locks(root, id, path, git, ref, source_hash, subdir, revision, hash) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListLocalPackageDependencies :many
SELECT * FROM local_package_dependencies WHERE root = ? ORDER BY root,package,position;

-- name: InsertLocalPackageDependency :exec
INSERT INTO local_package_dependencies(root, package, position, dependency) VALUES (?, ?, ?, ?);

-- name: GetConfigRevision :one
SELECT revision FROM configurations WHERE root = ?;
-- name: EnsureConfigRoot :exec
INSERT INTO configurations(root,revision) VALUES (?,0) ON CONFLICT DO NOTHING;
-- name: AdvanceConfigRevision :one
UPDATE configurations SET revision = revision + 1 WHERE root = ? AND revision = ? RETURNING revision;
-- name: DeleteLocalDefinition :exec
DELETE FROM local_definitions WHERE root = ? AND key = ?;
-- name: DeleteLocalScope :exec
DELETE FROM local_scopes WHERE root = ? AND position = ?;
-- name: DeleteLocalPackageLock :exec
DELETE FROM local_package_locks WHERE root = ? AND id = ?;
-- name: ResetSecrets :exec
DELETE FROM secrets;
-- name: PutSessionConfiguration :exec
INSERT INTO session_configurations(configuration, launch, owner) VALUES (?, ?, ?)
ON CONFLICT(configuration, launch) DO NOTHING;

-- name: DeleteConfigOwner :exec
DELETE FROM configurations WHERE root = ?;

-- name: ListSessionEnvironment :many
SELECT mcp, name, ciphertext FROM session_environment WHERE owner = ? ORDER BY mcp, name;

-- name: DeleteSessionEnvironment :exec
DELETE FROM session_environment WHERE owner = ?;

-- name: PutSessionEnvironment :exec
INSERT INTO session_environment(owner, mcp, name, ciphertext) VALUES (?, ?, ?, ?);

-- name: DeleteFinishedSessionConfiguration :exec
DELETE FROM configurations WHERE root IN (
    SELECT owner FROM session_configurations JOIN launches ON launch = launches.id
    WHERE launches.id = ? AND launches.finished IS NOT NULL
);
