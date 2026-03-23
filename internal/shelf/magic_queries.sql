-- name: CreateMagicShelf :one
INSERT INTO magic_shelf (user_id, name, description, icon, icon_color, rules, sort_field, sort_dir, limit_count)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetMagicShelfByID :one
SELECT * FROM magic_shelf WHERE id = ? LIMIT 1;

-- name: ListMagicShelvesForUser :many
SELECT * FROM magic_shelf WHERE user_id = ? ORDER BY name;

-- name: UpdateMagicShelf :exec
UPDATE magic_shelf SET name = ?, description = ?, icon = ?, icon_color = ?, rules = ?,
    sort_field = ?, sort_dir = ?, limit_count = ?, updated_at = datetime('now')
WHERE id = ? AND user_id = ?;

-- name: DeleteMagicShelf :exec
DELETE FROM magic_shelf WHERE id = ? AND user_id = ?;
