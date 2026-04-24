# Lexicon — Missing Functionality Implementation Plan

> **Date**: 2026-04-23
> **Base**: Commit `5ea7360` on `main`
> **Purpose**: Implement the three major phases skipped during initial development (BookDrop, Email, OIDC/Remote Auth) and close remaining API/frontend gaps identified in the codebase audit.

---

## 1. Executive Summary

The initial 36-phase implementation of Lexicon is functionally complete for all committed phases. Three planned phases were skipped:

| Phase | Feature | Reason for Skip |
|---|---|---|
| 28 | BookDrop | Deferred to focus on core library management |
| 29 | Email / Send-to-Device | Deferred as non-critical for initial release |
| 30 | OIDC & Remote Auth | Deferred; local JWT auth sufficient for MVP |

This plan implements those three phases plus a final polish pass to close smaller gaps in endpoints, audit logging, and admin APIs.

**Migration numbering note**: The original plan assigned migrations 015/016/017 to these phases. Those numbers were used by content_restrictions (015), duplicates (016), fonts (017), and hardcover (018). New migrations will use sequential numbers starting at 019.

---

## 2. Phase A: BookDrop (Watch-Folder Ingest)

**Goal**: Allow users to drop files into a watched folder and review them in a queue before importing.

**Dependencies**: Phase 07 (Scanner), Phase 12 (WebSocket)

### Schema
- `migrations/019_bookdrop.up.sql` — `bookdrop_file` table
  - `id`, `original_filename`, `file_path`, `file_size`, `status` (PENDING / IMPORTED / REJECTED)
  - `extracted_title`, `extracted_authors`, `extracted_cover_path`
  - `created_at`, `processed_at`, `imported_book_id`

### Backend
- `internal/bookdrop/watcher.go`
  - `fsnotify` watcher on `BOOKDROP_PATH` env var (default: `/bookdrop`)
  - Stability detection: file size unchanged for 5 seconds
  - Extract metadata and cover using existing `internal/storage/metadata.go` and `internal/storage/cover.go`
  - Insert `bookdrop_file` record with status PENDING
  - Emit `BOOKDROP_FILE_ARRIVED` WebSocket event
- `internal/bookdrop/service.go`
  - `ImportFile(id, targetLibraryID)` — copies file to library path, runs scanner logic on single file, updates status to IMPORTED
  - `RejectFile(id)` — deletes file from bookdrop folder, updates status to REJECTED
  - `ListPending()` — returns all PENDING files with extracted metadata
- `internal/bookdrop/handler.go`
  - `GET /api/bookdrop/files` — list queue (auth required)
  - `POST /api/bookdrop/files/{id}/import` — import to library (body: `{"libraryId": N}`)
  - `POST /api/bookdrop/files/{id}/reject` — reject and delete
  - `POST /api/bookdrop/files/import-all` — bulk import all pending to default library

### Frontend
- `web/src/features/bookdrop/BookDropQueue.tsx`
  - Table/grid of pending files with extracted metadata preview
  - Import / Reject buttons per file
  - Bulk import button
  - Real-time updates via WebSocket `BOOKDROP_FILE_ARRIVED`

### Integration
- Register `BOOKDROP_SCAN` task type in `internal/server/server.go`
- Add `BOOKDROP_IMPORTED` audit log entries in import flow
- Add `/bookdrop` route in `App.tsx`
- Add nav item in sidebar and mobile bottom nav

### Verification
- [ ] Drop EPUB/PDF into `/bookdrop` → appears in queue
- [ ] Extracted title/authors/cover shown in queue
- [ ] Import file → copied to library, book record created, scanner metadata run
- [ ] Reject file → removed from queue and disk
- [ ] Bulk import works
- [ ] WebSocket `BOOKDROP_FILE_ARRIVED` received by frontend
- [ ] `go test -race ./...` passes
- [ ] `npm run build` passes

---

## 3. Phase B: Email / Send-to-Device

**Goal**: Send book files via SMTP email.

**Dependencies**: Phase 11 (Book Detail)

### Schema
- `migrations/020_email.up.sql` — `email_provider` and `email_recipient` tables
  - `email_provider`: `id`, `name`, `host`, `port`, `username`, `password`, `from_address`, `use_tls`, `is_default`, `created_at`
  - `email_recipient`: `id`, `user_id`, `name`, `email_address`, `created_at`

### Backend
- `internal/email/service.go`
  - SMTP connection with TLS/startTLS support
  - MIME multipart message construction (text + attachment)
  - Attachment streaming from `book_file.file_path` (no full-memory load)
  - `SendBook(bookID, recipientIDs, providerID)`
- `internal/email/handler.go`
  - `GET /api/email/providers` — list providers (admin only)
  - `POST /api/email/providers` — create provider (admin only)
  - `PUT /api/email/providers/{id}` — update provider (admin only)
  - `DELETE /api/email/providers/{id}` — delete provider (admin only)
  - `GET /api/email/recipients` — list current user's recipients
  - `POST /api/email/recipients` — add recipient
  - `DELETE /api/email/recipients/{id}` — delete recipient
  - `POST /api/books/{id}/send` — send book to selected recipients (requires `can_email_send` permission)

### Frontend
- Admin UI: Email provider configuration page (`/admin/email`)
  - SMTP host, port, username, password, from address, TLS toggle
  - Test send button
- User UI: Recipient management in Settings page
  - Add/remove recipient email addresses
- Book detail page: "Send to Device" button
  - Dropdown of user's saved recipients
  - Send button with confirmation

### Integration
- Add `BOOK_SENT` audit log entry in send flow
- Add `can_email_send` permission enforcement (bit already exists in schema)
- Add nav/route for `/admin/email`

### Verification
- [ ] Admin configures SMTP provider
- [ ] Test email sends successfully
- [ ] User adds recipient email in settings
- [ ] Send book from book detail → email received with attachment
- [ ] Shared providers visible to all users
- [ ] Audit log records the send
- [ ] `go test -race ./...` passes
- [ ] `npm run build` passes

---

## 4. Phase C: OIDC & Remote Auth

**Goal**: External authentication via OpenID Connect and reverse-proxy headers.

**Dependencies**: Phase 03 (Local JWT Auth)

### Schema
- `migrations/021_oidc.up.sql` — `oidc_session` and `oidc_group_mapping` tables
  - `oidc_session`: `id`, `state`, `nonce`, `redirect_url`, `user_id`, `created_at`
  - `oidc_group_mapping`: `id`, `group_name`, `permission_bit`, `created_at`

### Backend
- Add `coreos/go-oidc/v3` to `go.mod`
- `internal/auth/oidc.go`
  - `OIDCConfig` struct: provider name, client ID, client secret, issuer URI, scopes
  - `GetOIDCProviders()` — returns configured providers
  - `InitiateOIDCAuth(provider, redirectURL)` — generates state/nonce, returns authorization URL
  - `HandleOIDCCallback(ctx, state, code)` — exchanges code for token, validates ID token, creates user on first login, issues Lexicon JWT
- `internal/auth/remote.go`
  - `RemoteAuthMiddleware` — extracts configurable headers (`Remote-User`, `Remote-Email`, `Remote-Groups`)
  - Creates user on first appearance if `REMOTE_AUTH_AUTO_CREATE` is enabled
  - Maps group headers to permissions via `oidc_group_mapping`
  - Issues Lexicon JWT

### Runtime Settings
Add to `app_settings` via admin API:
- `OIDC_ENABLED`, `OIDC_PROVIDER_NAME`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_ISSUER_URI`, `OIDC_SCOPE`
- `REMOTE_AUTH_ENABLED`, `REMOTE_AUTH_USER_HEADER`, `REMOTE_AUTH_EMAIL_HEADER`, `REMOTE_AUTH_GROUPS_HEADER`, `REMOTE_AUTH_AUTO_CREATE`

### Frontend
- Login page: "Login with {Provider}" button (shown when OIDC_ENABLED)
- `/auth/oidc/callback` route — receives authorization code, calls backend, stores JWT
- Admin settings: OIDC and Remote Auth configuration forms

### API Endpoints
- `GET /api/auth/oidc/providers` — list configured OIDC providers
- `GET /api/auth/oidc/{provider}/authorize` — redirect to provider
- `GET /api/auth/oidc/{provider}/callback` — callback handler

### Integration
- Register OIDC routes in `internal/server/routes.go`
- Add `OIDC_USER_CREATED` audit log action
- Add `REMOTE_AUTH_LOGIN` audit log action

### Verification
- [ ] Configure OIDC provider in admin settings
- [ ] Login via OIDC redirects to provider and back
- [ ] User created on first OIDC login
- [ ] Group claims map to permissions
- [ ] Remote Auth header creates/authenticates user
- [ ] Both methods issue valid Lexicon JWTs
- [ ] `go test -race ./...` passes
- [ ] `npm run build` passes

---

## 5. Phase D: Final Polish (API & Frontend Gaps)

**Goal**: Close smaller endpoint and feature gaps identified in the audit.

### 5.1 Missing REST Endpoints

| Endpoint | Status | Action |
|---|---|---|
| `GET /api/authors` | Missing | List all authors with book count |
| `GET /api/authors/{id}` | Missing | Author detail |
| `GET /api/authors/{id}/books` | Missing | Books by author |
| `GET /api/series` | Missing | List all series with book count |
| `GET /api/series/{id}` | Missing | Series detail |
| `GET /api/series/{id}/books` | Missing | Books in series |
| `GET /api/categories` | Missing | List all categories |
| `GET /api/tags` | Missing | List all tags |
| `GET /api/moods` | Missing | List all moods |
| `GET /api/users/me/reading-stats` | Missing | Total books read, total reading time |
| `POST /api/books/duplicates/dismiss` | Missing | Dismiss a duplicate pair |
| `POST /api/books/merge` | Missing | Merge two book records |
| `PUT /api/books/{id}/cover` | Missing | Upload custom cover |
| `DELETE /api/books/{id}/cover` | Missing | Delete custom cover |
| `GET /api/metadata/proposals` | Missing | List pending metadata proposals |

### 5.2 OPDS Gaps

| Endpoint | Status | Action |
|---|---|---|
| `GET /opds/libraries/{id}` | Missing | Library detail feed |
| `GET /opds/shelves/{id}` | Missing | Shelf detail feed |
| `GET /opds/series` | Missing | Series listing feed |
| `GET /opds/series/{id}` | Missing | Series books feed |
| `GET /opds/authors` | Missing | Author listing feed |
| `GET /opds/authors/{id}` | Missing | Author books feed |
| `GET /opds/search` | Missing | OpenSearch description + search results |

### 5.3 Kobo Gaps

| Endpoint | Status | Action |
|---|---|---|
| `GET /api/kobo/settings` | Missing | Kobo sync settings |
| `PUT /api/kobo/settings` | Missing | Update Kobo settings |
| `PUT /kobo/{token}/v1/library/{revisionId}/state` | Missing | Reading state update from device |
| `DELETE /kobo/{token}/v1/library/{revisionId}` | Missing | Delete from library |

### 5.4 Audit Log Completeness

The following action types are defined but never logged:

| Action | Where to Log |
|---|---|
| `BOOK_COVER_UPDATED` | `internal/storage/handler.go` when cover is uploaded/deleted |
| `OPDS_ACCESS` | `internal/opds/handler.go` on each authenticated feed request |
| `KOBO_SYNC` | `internal/kobo/handler.go` on library sync |
| `KOREADER_SYNC` | `internal/koreader/handler.go` on progress sync |
| `ADMIN_ACTION` | Use for admin settings changes |

### 5.5 Admin Runtime Settings API

- `GET /api/admin/settings` — return all `app_settings` key/value pairs
- `PUT /api/admin/settings` — update `app_settings` (admin only)
- Frontend: Replace `AdminSettingsStub` with real settings editor

### 5.6 Frontend Pages

- `/authors` — Author browse page
- `/authors/:id` — Author detail with book list
- `/series` — Series browse page
- `/series/:id` — Series detail with book list
- `/tasks` — Background task monitor (list, run, cancel, view progress)

### Verification
- [ ] All new endpoints return correct data
- [ ] OPDS feeds validate in e-reader clients
- [ ] Audit log captures all defined action types
- [ ] Admin settings page is functional
- [ ] Frontend navigation works for new pages
- [ ] `go test -race ./...` passes
- [ ] `npm run build` passes

---

## 6. Migration Strategy

| Migration | Table | Phase |
|---|---|---|
| `019_bookdrop.up.sql` | `bookdrop_file` | A |
| `020_email.up.sql` | `email_provider`, `email_recipient` | B |
| `021_oidc.up.sql` | `oidc_session`, `oidc_group_mapping` | C |

All new migrations must include matching `.down.sql` files.

---

## 7. Parallelization Guide

```
Phase A (BookDrop)
├── Phase B (Email) ← parallelizable with A
├── Phase C (OIDC)  ← parallelizable with A and B
└── Phase D (Polish) ← needs A, B, C for full coverage but can start in parallel
```

Recommended order:
1. Phase D partial — implement standalone endpoints (authors, series, reading-stats, etc.) first
2. Phase A, B, C in parallel
3. Phase D completion — admin settings, audit log wiring, frontend pages

---

## 8. Environment Variables (New)

| Variable | Default | Description |
|---|---|---|
| `BOOKDROP_PATH` | `/bookdrop` | Path to BookDrop watch folder |
| `BOOKDROP_ENABLED` | `true` | Enable BookDrop watcher |
| `EMAIL_ENABLED` | `false` | Enable email sending |
| `OIDC_ENABLED` | `false` | Enable OIDC authentication |
| `REMOTE_AUTH_ENABLED` | `false` | Enable reverse-proxy header auth |

---

## 9. Success Criteria

- All three originally skipped phases (BookDrop, Email, OIDC/Remote Auth) are fully implemented
- All 21 audit log action types are actively logged
- No `[no test files]` packages in newly created directories
- `go build ./...`, `go vet ./...`, `go test -race ./...`, and `npm run build` all pass
- README updated to remove "Not Yet Implemented" labels for completed features
