# Lexicon — Gap Closure Plan

> **Date**: 2026-04-24
> **Base**: Commit `26cf6f7` on `main`
> **Purpose**: Close all remaining implementation gaps identified in the comprehensive audit.

---

## Executive Summary

The comprehensive audit identified **5 true functional gaps** and **5 missing WebSocket event emissions** (out of 11 total events). This plan closes all of them in a single cohesive branch.

| # | Gap | Phase | Effort |
|---|---|---|---|
| 1 | WebSocket `SESSION_REVOKED` not broadcast | Auth | 15 min |
| 2 | WebSocket `BOOK_UPDATED` not emitted | Library/Book | 30 min |
| 3 | WebSocket `BOOK_DELETED` not emitted | Library/Book | 30 min |
| 4 | WebSocket `METADATA_PROPOSAL_READY` not emitted | Metadata | 30 min |
| 5 | WebSocket `NOTIFICATION` mechanism missing | General | 45 min |
| 6 | Book detail: authors/series not clickable links | Frontend | 30 min |
| 7 | `PUT /api/books/{id}/metadata` missing | API | 1 hour |
| 8 | `canDownload` permission not enforced | Auth | 15 min |
| 9 | Per-field priority matrix for metadata providers | Metadata | 2-3 hours |

---

## Phase 1: WebSocket Event Completeness

**Goal**: All 11 WebSocket events are emitted by the backend and handled by the frontend.

### 1.1 `SESSION_REVOKED`

**Current state**: `RevokeAllUserRefreshTokens` revokes tokens in the DB, but never tells connected WebSocket clients.
**Fix**: After revoking tokens, broadcast `SESSION_REVOKED` to all connections for that user.

**Files to modify**:
- `internal/user/service.go` — `RevokeAllUserRefreshTokens`: inject `*ws.Hub` and broadcast after revocation
- `internal/user/handler.go` — pass `wsHub` to `RevokeAllUserRefreshTokens` calls
- `internal/server/server.go` — wire hub into user service
- `web/src/shared/ws/socket.ts` — already handles `SESSION_REVOKED`; verify it clears tokens and redirects

**Verification**:
- [ ] Admin revokes a user's sessions → user is immediately logged out on all connected browsers
- [ ] `go test -race ./...` passes

### 1.2 `BOOK_UPDATED`

**Current state**: When book metadata is updated (e.g., proposal accepted, manual edit), no WebSocket event is sent.
**Fix**: Broadcast `BOOK_UPDATED` after any book metadata mutation.

**Files to modify**:
- `internal/metadata/service.go` — `AcceptProposal`: broadcast `BOOK_UPDATED` with book ID after applying metadata
- `internal/book/handler.go` — `handleMergeBooks`: broadcast `BOOK_UPDATED` for target book
- `internal/book/handler.go` — new `handleUpdateMetadata` (Phase 3): broadcast after metadata update
- `internal/ws/hub.go` — add `BroadcastBookUpdated(bookID int64)` helper if not already present

**Frontend handling**:
- `web/src/shared/ws/socket.ts` — add `onBookUpdated` handler that invalidates relevant `createResource` caches

**Verification**:
- [ ] Accept a metadata proposal → connected clients receive `BOOK_UPDATED`
- [ ] Library browser refreshes book card without page reload

### 1.3 `BOOK_DELETED`

**Current state**: When a book is deleted, no WebSocket event is sent.
**Fix**: Broadcast `BOOK_DELETED` after successful deletion.

**Files to modify**:
- `internal/book/handler.go` — `handleDelete`: broadcast `BOOK_DELETED` with book ID before returning
- `internal/library/scanner.go` — when a file is removed and book is deleted: broadcast `BOOK_DELETED`
- `internal/ws/hub.go` — add `BroadcastBookDeleted(bookID int64)` helper

**Frontend handling**:
- `web/src/shared/ws/socket.ts` — add `onBookDeleted` handler that removes the book from UI caches

**Verification**:
- [ ] Delete a book → connected clients receive `BOOK_DELETED`
- [ ] Book disappears from library browser without page reload

### 1.4 `METADATA_PROPOSAL_READY`

**Current state**: When a metadata proposal is created (e.g., from a scan or manual search), no WebSocket event notifies admins.
**Fix**: Broadcast `METADATA_PROPOSAL_READY` after creating a proposal.

**Files to modify**:
- `internal/metadata/service.go` — `CreateProposal` or search flow: broadcast after inserting proposal
- `internal/ws/hub.go` — add `BroadcastMetadataProposalReady(proposalID int64)` helper

**Frontend handling**:
- `web/src/shared/ws/socket.ts` — add `onMetadataProposalReady` handler that shows a toast notification
- `web/src/features/admin/AdminDashboard.tsx` or similar — show pending proposals badge

**Verification**:
- [ ] Run metadata search that produces proposals → admin receives `METADATA_PROPOSAL_READY`
- [ ] Toast notification appears in admin UI

### 1.5 `NOTIFICATION` (General)

**Current state**: No general notification mechanism exists.
**Fix**: Add a generic `NOTIFICATION` event type for ad-hoc messages.

**Files to modify**:
- `internal/ws/hub.go` — add `BroadcastNotification(userIDs []int64, title, message string)` helper
- `internal/task/runner.go` — `TASK_COMPLETE` and `TASK_FAILED`: also send `NOTIFICATION` with summary
- `internal/library/watcher.go` — after scan complete: send `NOTIFICATION` with "X books added"
- `web/src/shared/ws/socket.ts` — add `onNotification` handler that shows toast notifications

**Verification**:
- [ ] Task completes → toast shows "Library scan complete: 5 books added"
- [ ] Notification is user-scoped (only the user who triggered the task sees it)

---

## Phase 2: API & Frontend Gaps

### 2.1 Clickable Authors/Series in Book Detail

**Current state**: Authors and series are rendered as plain text `<span>` elements.
**Fix**: Wrap them in `<A>` tags with router navigation to `/authors/:id` and `/series/:id`.

**Files to modify**:
- `web/src/features/book/BookDetail.tsx` — find author/series rendering, wrap in `<A href={`/authors/${author.id}`}>` and `<A href={`/series/${series.id}`}>`

**Verification**:
- [ ] Click author name → navigates to AuthorDetail page
- [ ] Click series name → navigates to SeriesDetail page

### 2.2 `PUT /api/books/{id}/metadata` (Admin Metadata Update)

**Current state**: No endpoint exists for admins to directly edit book metadata.
**Fix**: Add endpoint that updates `book_metadata` fields with admin bypass of field locks.

**Files to modify**:
- `internal/book/handler.go` — add `handleUpdateMetadata`:
  - Accept `BookMetadata` fields in request body
  - Require admin or `can_edit_metadata` permission
  - Update `book_metadata` row directly (bypass field locks for admins)
  - Broadcast `BOOK_UPDATED` WebSocket event
- `internal/server/routes.go` — add `r.Put("/{id}/metadata", s.bookHandler.handleUpdateMetadata)`

**Verification**:
- [ ] Admin PUTs metadata → book_metadata updated in DB
- [ ] Non-admin gets 403
- [ ] `BOOK_UPDATED` WebSocket event emitted

### 2.3 `canDownload` Permission Enforcement

**Current state**: The `can_download` permission bit exists in the schema but the file stream endpoint only checks auth + library access.
**Fix**: Add `RequirePermission` check to the stream/download endpoints.

**Files to modify**:
- `internal/server/routes.go` — wrap `/api/reader/books/{bookId}/files/{fileId}/stream` with `RequirePermission(s.cfg.JWTSecret, auth.PermDownload)`
- `internal/server/routes.go` — also check OPDS download and `/api/books/{id}/cover` if appropriate

**Verification**:
- [ ] User without `can_download` gets 403 on stream
- [ ] User with `can_download` can stream normally

---

## Phase 3: Per-Field Priority Matrix (Metadata Providers)

**Current state**: `AcceptProposal` applies a single provider's result. When multiple providers return data, there's no merging logic.
**Fix**: Implement a priority matrix that merges fields from multiple provider results according to configured priorities.

### Design

- Each provider has a priority score (1-10, default 5)
- Each field has a "best source wins" rule
- When multiple proposals exist for the same book, merge them field-by-field using provider priority

**Files to modify**:
- `internal/metadata/service.go` — add `MergeProposals(proposals []Proposal) (BookMetadata, error)`:
  - Group proposals by provider
  - Sort by provider priority (higher = better)
  - For each field, take value from highest-priority provider that provided it
  - Respect field locks (locked fields are never overwritten)
- `internal/metadata/handler.go` — add `POST /api/metadata/merge` or integrate into existing accept flow
- `migrations/022_provider_priority.up.sql` — add `provider_priority` column to `metadata_fetch_job` or create new table

**Verification**:
- [ ] Two proposals for same book from different providers → merged metadata takes best fields from each
- [ ] Locked fields are preserved
- [ ] `go test -race ./...` passes

---

## Phase 4: Final Verification & Cleanup

### Verification Checklist
- [ ] All 11 WebSocket events are emitted by backend
- [ ] All 11 WebSocket events are handled by frontend
- [ ] Authors/series are clickable in book detail
- [ ] `PUT /api/books/{id}/metadata` works for admins
- [ ] `canDownload` permission is enforced
- [ ] Per-field priority matrix merges proposals correctly
- [ ] `go build -tags dev ./...` passes
- [ ] `go vet -tags dev ./...` passes
- [ ] `go test -tags dev -race ./...` passes
- [ ] `npm run build` passes
- [ ] `docker build .` passes

### Migration Strategy

| Migration | Table | Purpose |
|---|---|---|
| `022_provider_priority.up.sql` | `provider_priority` | Metadata provider priority scores |

All new migrations must include matching `.down.sql` files.

---

## Parallelization Guide

```
Phase 1 (WebSocket Events)
├── 1.1 SESSION_REVOKED ← standalone
├── 1.2 BOOK_UPDATED ← needs 2.2 (handleUpdateMetadata) for full coverage
├── 1.3 BOOK_DELETED ← standalone
├── 1.4 METADATA_PROPOSAL_READY ← standalone
└── 1.5 NOTIFICATION ← standalone

Phase 2 (API/Frontend Gaps)
├── 2.1 Clickable authors/series ← standalone, frontend only
├── 2.2 PUT /books/{id}/metadata ← backend only
└── 2.3 canDownload enforcement ← backend only

Phase 3 (Priority Matrix)
└── 3.1 Merge proposals ← backend only, most complex

Phase 4 (Verification)
└── Run all checks, code review, merge
```

**Recommended order**:
1. Phase 1.1, 1.3, 1.4, 1.5, 2.1, 2.3 in parallel (all standalone)
2. Phase 2.2 (needed for 1.2)
3. Phase 1.2 (depends on 2.2)
4. Phase 3 (most complex, do last)
5. Phase 4 (verification)

---

## Success Criteria

- Zero gaps remain from the audit
- All 11 WebSocket events are emitted and handled
- All verification commands pass
- No regressions in existing functionality
- README and FEATURES.md updated if needed
