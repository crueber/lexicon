-- name: CreateMetadataProposal :one
INSERT INTO metadata_proposal (book_id, provider, provider_id, status, data)
VALUES (?, ?, ?, 'PENDING', ?)
RETURNING *;

-- name: GetMetadataProposal :one
SELECT * FROM metadata_proposal WHERE id = ? LIMIT 1;

-- name: ListMetadataProposals :many
SELECT * FROM metadata_proposal WHERE book_id = ? ORDER BY created_at DESC;

-- name: UpdateProposalStatus :exec
UPDATE metadata_proposal SET status = ? WHERE id = ?;

-- name: ListPendingProposals :many
SELECT mp.*, bm.title as book_title
FROM metadata_proposal mp
LEFT JOIN book_metadata bm ON mp.book_id = bm.book_id
WHERE mp.status = 'PENDING'
ORDER BY mp.created_at DESC;
