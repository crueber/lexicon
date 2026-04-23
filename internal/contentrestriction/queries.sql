-- name: ListRestrictionsByUser :many
SELECT * FROM user_content_restriction WHERE user_id = ? ORDER BY restriction_type, value;

-- name: CreateRestriction :one
INSERT INTO user_content_restriction (user_id, restriction_type, value, mode)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: DeleteRestriction :exec
DELETE FROM user_content_restriction WHERE id = ? AND user_id = ?;

-- name: GetBookIDsByCategoryValue :many
SELECT bc.book_id FROM book_category bc
JOIN category c ON c.id = bc.category_id
WHERE c.name = ?;

-- name: GetBookIDsByTagValue :many
SELECT bt.book_id FROM book_tag bt
JOIN tag t ON t.id = bt.tag_id
WHERE t.name = ?;

-- name: GetBookIDsByMoodValue :many
SELECT bm.book_id FROM book_mood bm
JOIN mood m ON m.id = bm.mood_id
WHERE m.name = ?;

-- name: GetBookIDsByAgeRating :many
SELECT book_id FROM comic_metadata WHERE age_rating = ?;

-- name: UpdateRestrictionMode :exec
UPDATE user_content_restriction SET mode = ? WHERE id = ? AND user_id = ?;
