-- name: CreateLibrary :one
INSERT INTO library (name, icon, icon_color, organization_mode, file_naming_pattern)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetLibraryByID :one
SELECT * FROM library WHERE id = ? LIMIT 1;

-- name: ListLibraries :many
SELECT * FROM library ORDER BY name;

-- name: UpdateLibrary :exec
UPDATE library SET name = ?, icon = ?, icon_color = ?, organization_mode = ?, file_naming_pattern = ?
WHERE id = ?;

-- name: DeleteLibrary :exec
DELETE FROM library WHERE id = ?;

-- name: CreateLibraryPath :one
INSERT INTO library_path (library_id, path) VALUES (?, ?) RETURNING *;

-- name: ListLibraryPaths :many
SELECT * FROM library_path WHERE library_id = ?;

-- name: DeleteLibraryPath :exec
DELETE FROM library_path WHERE id = ?;

-- name: ListUserLibraryIDs :many
SELECT library_id FROM user_library_permission WHERE user_id = ?;

-- name: GrantLibraryAccess :exec
INSERT OR IGNORE INTO user_library_permission (user_id, library_id) VALUES (?, ?);

-- name: RevokeLibraryAccess :exec
DELETE FROM user_library_permission WHERE user_id = ? AND library_id = ?;

-- name: SetUserLibraryPermissions :exec
DELETE FROM user_library_permission WHERE user_id = ?;
