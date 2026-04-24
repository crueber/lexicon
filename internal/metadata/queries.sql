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

-- name: GetProviderPriority :one
SELECT priority FROM provider_priority WHERE provider_name = ?;

-- name: UpsertProviderPriority :exec
INSERT INTO provider_priority (provider_name, priority) VALUES (?, ?)
ON CONFLICT(provider_name) DO UPDATE SET priority = excluded.priority;

-- name: ListProviderPriorities :many
SELECT provider_name, priority FROM provider_priority ORDER BY provider_name;
