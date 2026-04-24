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

-- name: GetLibraryPathByID :one
SELECT * FROM library_path WHERE id = ? LIMIT 1;

-- name: DeleteLibraryPath :exec
DELETE FROM library_path WHERE id = ?;

-- name: GrantLibraryAccess :exec
INSERT OR IGNORE INTO user_library_permission (user_id, library_id) VALUES (?, ?);

-- name: RevokeLibraryAccess :exec
DELETE FROM user_library_permission WHERE user_id = ? AND library_id = ?;

-- name: GetLibraryMetadataSources :many
SELECT provider, field_priority FROM library_metadata_source WHERE library_id = ?;

-- name: SetLibraryMetadataSource :exec
INSERT INTO library_metadata_source (library_id, provider, field_priority) VALUES (?, ?, ?)
ON CONFLICT(library_id, provider) DO UPDATE SET field_priority = excluded.field_priority;

-- name: DeleteLibraryMetadataSource :exec
DELETE FROM library_metadata_source WHERE library_id = ? AND provider = ?;

-- name: ClearUserLibraryPermissions :exec
DELETE FROM user_library_permission WHERE user_id = ?;
