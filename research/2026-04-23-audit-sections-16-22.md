---
date: "2026-04-23T00:00:00Z"
last_updated: "2026-04-23T00:00:00Z"
last_updated_by: "system"
repository: crueber/lexicon
topic: "Audit of Sections 16-22 against actual codebase"
tags: [research, audit, email, bookdrop, dashboard, notebook, recommendations, audit-logs, content-restrictions]
---

# Research: Audit of Implementation Plan Sections 16-22

**Date**: 2026-04-23
**Git Commit**: `5ea736039514c03a7d22f8699ead537d31f945fd`
**Branch**: `main`
**Repository**: crueber/lexicon

## Research Question

For EACH requirement, feature, table, endpoint, or design decision mentioned in Sections 16-22 of the implementation plan, check whether it exists in the actual codebase. Be thorough — check files, routes, tables, migrations, and configurations.

---

## Section 16: Email / Send-to-Device

### Implemented ✅
- **`can_email_send` permission bit**: Exists in `user_permissions` table (`migrations/001_users.up.sql:17`)
- **`email` column on `users` table**: Exists (`migrations/001_users.up.sql:5`)
- **JWT claims include `can_email_send`**: `internal/auth/jwt.go:37`
- **Auth middleware checks `email_send` permission**: `internal/auth/middleware.go:104`

### Missing / Partial ⚠️
- **`email_provider` table**: Not created in any migration. Schema specifies columns: id, name, host, port, username, password, from_address, encryption, is_shared.
- **`email_recipient` table**: Not created in any migration. Schema specifies columns: id, user_id, provider_id, recipient_email, label.
- **`internal/email/` package**: Directory does not exist. No handler.go, service.go.
- **SMTP service / MIME construction**: No code for SMTP connection, MIME email with attachment, or SSL/STARTTLS/PLAIN detection.
- **Email routes**: None of the following are registered:
  - `GET /api/email/providers`
  - `POST /api/email/providers`
  - `PUT /api/email/providers/{id}`
  - `DELETE /api/email/providers/{id}`
  - `POST /api/email/providers/{id}/test`
  - `GET /api/email/recipients`
  - `POST /api/email/recipients`
  - `DELETE /api/email/recipients/{id}`
  - `POST /api/books/{id}/send`
- **Admin UI for email provider configuration**: No frontend components exist.
- **User UI for recipient management**: No frontend components exist.
- **`BOOK_SENT` audit action**: Defined in `internal/audit/service.go:19` but never logged (no send endpoint).
- **`can_email_send` permission enforcement**: The middleware checks the bit, but no endpoint actually requires it because the email feature is absent.
- **`EMAIL_ENABLED` app setting**: Not referenced anywhere.

### Not in Plan but Implemented 📝
- None for this section.

---

## Section 17: BookDrop

### Implemented ✅
- **`BOOKDROP_SCAN` task type constant**: `internal/task/types.go:10`
- **`BOOKDROP_IMPORTED` audit action constant**: `internal/audit/service.go:29`

### Missing / Partial ⚠️
- **`bookdrop_file` table**: Not created in any migration. Schema specifies columns: id, file_path, file_name, file_size, format, status, target_library_id, matched_book_id, extracted_metadata, cover_path, error_message, created_at, updated_at.
- **`internal/bookdrop/` package**: Directory does not exist. No handler.go, service.go, watcher.go.
- **BookDrop fsnotify watcher**: No code watching `BOOKDROP_PATH` for file stability.
- **BookDrop routes**: None of the following are registered:
  - `GET /api/bookdrop/files`
  - `GET /api/bookdrop/files/{id}`
  - `POST /api/bookdrop/files/{id}/import`
  - `POST /api/bookdrop/files/{id}/reject`
  - `POST /api/bookdrop/bulk-import`
  - `DELETE /api/bookdrop/files/{id}`
- **`BOOKDROP_FILE_ARRIVED` WebSocket event**: Not emitted anywhere (no `bookdrop` package).
- **Frontend `BookDropQueue.tsx`**: Does not exist (`web/src/features/bookdrop/` missing).
- **`BOOKDROP_PATH` env var / configuration**: Not referenced in server config.
- **`BOOKDROP_ENABLED` app setting**: Not referenced anywhere.
- **BookDrop route in frontend router**: `/bookdrop` not present in `web/src/App.tsx`.

### Not in Plan but Implemented 📝
- None for this section.

---

## Section 18: Dashboard

### Implemented ✅
- **`user_settings.dashboard_setting` JSON column**: Exists (`migrations/001_users.up.sql:31`).
- **`GET /api/dashboard` endpoint**: `internal/dashboard/handler.go:151`
- **`GET /api/dashboard/settings`**: `internal/dashboard/handler.go:437`
- **`PUT /api/dashboard/settings`**: `internal/dashboard/handler.go:467`
- **Dashboard stats computation**: `internal/dashboard/handler.go:398` — computes totalBooks, totalLibraries, booksReadThisMonth, totalReadingTime from `reading_sessions`.
- **Row type `CONTINUE_READING`** (plan calls it `LAST_READ`): Fetches from `user_book_file_progress` ordered by `updated_at DESC`. `internal/dashboard/handler.go:287`
- **Row type `RECENTLY_ADDED`** (plan calls it `LATEST_ADDED`): Fetches from `book` ordered by `added_date DESC`. `internal/dashboard/handler.go:273`
- **Row type `RANDOM_PICKS`** (plan calls it `RANDOM`): Fetches 20 random books via `ORDER BY RANDOM()`. `internal/dashboard/handler.go:303`
- **Content restriction filtering on dashboard**: `internal/dashboard/handler.go:220`, `filterDashboardBooks()` at line 246.
- **Frontend `Dashboard.tsx`**: `web/src/features/dashboard/Dashboard.tsx`
- **Frontend `ScrollerRow.tsx`**: `web/src/features/dashboard/ScrollerRow.tsx` — horizontal scrollable book cards with prev/next buttons.
- **Default route `/` lands on Dashboard**: `web/src/App.tsx:268`
- **Stats bar with 4 stat cards**: Total books, libraries, books read this month, reading time.

### Missing / Partial ⚠️
- **Row type `LAST_LISTENED`** (audiobook-only continue reading): Not implemented. The dashboard only has a generic `CONTINUE_READING` that includes all book types.
- **Row type `MAGIC_SHELF`**: Not implemented as a dashboard row type. Magic shelves exist, but cannot be added as a dashboard scroller row.
- **Frontend UI for configuring/reordering dashboard rows**: The API endpoints (`/api/dashboard/settings`) exist, but there is no frontend page or UI controls to enable/disable, reorder, or rename rows. Users cannot customize their dashboard through the UI.
- **Row naming mismatch**: Plan specifies `LAST_READ`, `LATEST_ADDED`, `RANDOM`. Implemented names are `CONTINUE_READING`, `RECENTLY_ADDED`, `RANDOM_PICKS`.
- **Up to 5 configurable rows**: Default configuration only provides 3 rows, and no UI exists to add more.

### Not in Plan but Implemented 📝
- **Dashboard stats aggregation** (`totalBooks`, `totalLibraries`, `booksReadThisMonth`, `totalReadingTime`) is more detailed than the plan's basic scroller description.

---

## Section 19: Notebook (Annotations)

### Implemented ✅
- **`annotation` table**: Created in `migrations/010_annotations.up.sql`. Columns: id, user_id, book_id, book_file_id, type, cfi, page_number, text, note, color, created_at, updated_at.
- **Annotation CRUD endpoints under `/api/reader/books/{bookId}/annotations`**:
  - `GET` — list annotations for a book (`internal/notebook/handler.go:139`)
  - `POST` — create annotation (`internal/notebook/handler.go:172`)
  - `PUT /{id}` — update annotation note/color (`internal/notebook/handler.go:234`)
  - `DELETE /{id}` — delete annotation (`internal/notebook/handler.go:284`)
- **`GET /api/notebook`**: Unified paginated view with `?bookId=` filter (`internal/notebook/handler.go:318`)
- **Content restriction filtering on notebook**: Applied to both book-specific and all-annotations listings (`internal/notebook/handler.go:347`, `internal/notebook/handler.go:428`)
- **Frontend `Notebook.tsx`**: `web/src/features/notebook/Notebook.tsx` — grouped by book, with color filter, book filter, text search, pagination, delete support.
- **Color picker support**: Yellow, green, blue, pink, purple.
- **Annotation type field**: Supports `"HIGHLIGHT"` default and custom types.
- **`reading_sessions` table**: Exists (`migrations/005_progress.up.sql:11`) — tracks start/end progress, duration.

### Missing / Partial ⚠️
- **`book_marks` table**: Not created. Plan specifies: id, user_id, book_file_id, cfi, label, created_at.
- **`book_notes` table**: Not created. Plan specifies: id, user_id, book_id, content, created_at, updated_at.
- **`pdf_annotations` table**: Not created. Plan specifies: id, user_id, book_file_id, page, annotation_data (JSON), created_at.
- **Bookmark endpoints**: None of the following exist:
  - `GET /api/bookmarks`
  - `POST /api/bookmarks`
  - `DELETE /api/bookmarks/{id}`
- **Book note endpoints**: None of the following exist:
  - `GET /api/books/{id}/notes`
  - `POST /api/books/{id}/notes`
  - `PUT /api/books/{id}/notes/{noteId}`
  - `DELETE /api/books/{id}/notes/{noteId}`
- **Markdown export endpoint**: `GET /api/notebook/export` does not exist.
- **Unified view completeness**: The plan says "Returns all of the authenticated user's EPUB highlights, bookmarks, book notes, and PDF annotations, grouped by book." The actual implementation only returns annotations (highlights). No bookmarks, notes, or PDF annotations are included.
- **Table name mismatch**: Plan specifies `annotations` (plural), but the actual migration creates `annotation` (singular). All queries use `annotation`.

### Not in Plan but Implemented 📝
- **Annotation `page_number` field**: Added to the `annotation` table, allowing it to serve double-duty for PDF page-based annotations in addition to CFI-based EPUB annotations.

---

## Section 20: Recommendations

### Implemented ✅
- **`book_vectors` table**: Created in `migrations/013_book_vectors.up.sql` — stores 512-byte BLOB (128 × float32).
- **Feature hashing with FNV-1a**: `internal/recommendation/vector.go:22`
- **128-dimensional float32 vectors**: `internal/recommendation/vector.go:20`
- **Feature weights match plan exactly**:
  - Authors: 0.30 (`internal/recommendation/vector.go:11`)
  - Series: 0.20 (`internal/recommendation/vector.go:12`)
  - Categories: 0.20 (`internal/recommendation/vector.go:13`)
  - Tags: 0.15 (`internal/recommendation/vector.go:14`)
  - Language: 0.10 (`internal/recommendation/vector.go:15`)
  - Publisher: 0.05 (`internal/recommendation/vector.go:16`)
- **L2 normalization**: `internal/recommendation/vector.go:70`
- **Cosine similarity**: `internal/recommendation/vector.go:87`
- **`GET /api/books/{id}/similar` endpoint**: `internal/server/routes.go:56`, handler at `internal/recommendation/handler.go:35`
- **Per-author cap of 3**: `internal/recommendation/service.go:211`
- **`RECOMMENDATION_REBUILD` task**: Registered in `internal/server/server.go:192`
- **Content restriction filtering on recommendations**: `internal/recommendation/handler.go:66`
- **Frontend "Similar Books" section on Book Detail**: `web/src/features/book/BookDetail.tsx:141` (`SimilarBooksSection` component)
- **Recommender service tests**: `internal/recommendation/service_test.go`, `internal/recommendation/vector_test.go`

### Missing / Partial ⚠️
- None significant. The recommendation engine is fully implemented according to the plan.

### Not in Plan but Implemented 📝
- **`libraryIDs` filtering on similar books**: The handler accepts the user's accessible library IDs from the JWT and filters candidates, which adds a security/privacy layer not explicitly described in Section 20.

---

## Section 21: Audit Logs

### Implemented ✅
- **`audit_log` table**: Created in `migrations/014_audit.up.sql` — all columns match plan: id, user_id, username, action, resource_type, resource_id, details (JSON), ip_address, country, created_at.
- **Indexes on audit_log**: `idx_audit_log_action`, `idx_audit_log_user_id`, `idx_audit_log_created_at` (`migrations/014_audit.up.sql:14-16`)
- **`internal/audit/` package**: Full implementation — `service.go`, `handler.go`, `queries.sql`, models.
- **Async audit logging**: `internal/audit/service.go:60` — fires in a goroutine so it never blocks the main flow.
- **`GET /api/admin/audit-logs` endpoint**: `internal/audit/handler.go:25` — paginated, filterable by action, userId, from, to.
- **`AUDIT_LOG_CLEANUP` task type**: Registered in `internal/server/server.go:105`
- **Cleanup function**: `internal/audit/service.go:105` — deletes entries older than N days via `datetime('now', ?)`.
- **Frontend `AuditLogs.tsx`**: `web/src/features/admin/AuditLogs.tsx` — table with action filter, user ID filter, date range filter, pagination.
- **All 21 action type constants defined**: `internal/audit/service.go:13-33`

### Missing / Partial ⚠️
- **Several action types are defined but never actually logged**:
  - `BOOK_SENT` — never logged (no email feature).
  - `BOOK_COVER_UPDATED` — constant defined but no handler calls `auditSvc.Log` with this action.
  - `BOOKDROP_IMPORTED` — never logged (no bookdrop feature).
  - `OPDS_ACCESS` — constant defined but not logged in `internal/opds/handler.go`.
  - `KOBO_SYNC` — constant defined but not logged in `internal/kobo/handler.go`.
  - `KOREADER_SYNC` — constant defined but not logged in `internal/koreader/handler.go`.
  - `ADMIN_ACTION` — constant defined but never used as a generic catch-all.
- **`Country` field**: Always empty string. No geo-IP lookup is performed; the field exists in the schema and is inserted as empty.
- **Frontend action dropdown is incomplete**: `web/src/features/admin/AuditLogs.tsx:42-58` only lists 15 action types, missing: `BOOK_SENT`, `BOOK_COVER_UPDATED`, `BOOKDROP_IMPORTED`, `OPDS_ACCESS`, `KOBO_SYNC`, `KOREADER_SYNC`.
- **Audit log integration gaps**: While many handlers call `auditSvc.Log`, some significant actions are not audited:
  - Book cover updates (`PUT /api/books/{id}/cover`) do not create audit entries.
  - OPDS downloads do not create audit entries.
  - Kobo sync operations do not create audit entries.
  - KOReader sync operations do not create audit entries.

### Not in Plan but Implemented 📝
- **`internal/audit/service_test.go`**: Comprehensive tests for audit log creation, filtering, counting, and cleanup.

---

## Section 22: Content Restrictions

### Implemented ✅
- **`user_content_restriction` table**: Created in `migrations/015_content_restrictions.up.sql` — all columns match plan: id, user_id, restriction_type, value, mode, UNIQUE constraint on (user_id, restriction_type, value).
- **`EXCLUDE` and `ALLOW_ONLY` modes**: Implemented (`internal/contentrestriction/service.go:12-13`).
- **Restriction type constants**: `CATEGORY`, `TAG`, `MOOD`, `AGE_RATING`, `CONTENT_RATING` (`internal/contentrestriction/service.go:17-23`).
- **REST API endpoints**:
  - `GET /api/users/me/content-restrictions` — `internal/contentrestriction/handler.go:26`
  - `POST /api/users/me/content-restrictions` — `internal/contentrestriction/handler.go:44`
  - `PUT /api/users/me/content-restrictions/{id}` — `internal/contentrestriction/handler.go:117`
  - `DELETE /api/users/me/content-restrictions/{id}` — `internal/contentrestriction/handler.go:93`
- **Frontend `ContentRestrictionsSection`**: `web/src/features/auth/SettingsPage.tsx:381` — allows adding/removing restrictions with type dropdown, value input, and mode selector.
- **`FilterBookIDs` service method**: `internal/contentrestriction/service.go:97` — applies EXCLUDE and ALLOW_ONLY logic to a slice of book IDs.
- **Admin bypass**: Admins bypass all restrictions (`internal/contentrestriction/service.go:98`).
- **Applied across the codebase**:
  - Book listings (`internal/book/handler.go:302`, `internal/book/handler.go:470`)
  - Shelf books (`internal/shelf/handler.go:352`)
  - Magic shelf results (`internal/shelf/magic_handler.go:413`, `internal/shelf/magic_handler.go:505`)
  - Dashboard (`internal/dashboard/handler.go:220`)
  - Notebook (`internal/notebook/handler.go:347`, `internal/notebook/handler.go:428`)
  - Recommendations (`internal/recommendation/handler.go:66`)
  - OPDS (`internal/opds/handler.go:740`)
  - Kobo sync (`internal/kobo/handler.go:398`)
- **Tests**: `internal/contentrestriction/service_test.go` — tests for EXCLUDE, ALLOW_ONLY, admin bypass, no-restrictions cases.

### Missing / Partial ⚠️
- **`CONTENT_RATING` restriction type is accepted but not functional**: The API validates `CONTENT_RATING` as a valid type (`internal/contentrestriction/handler.go:71-77`), but `contentrestriction/queries.sql` has no query for it, and `service.go` `getAllowedBookIDs` / `getExcludedBookIDs` fall through to `fmt.Errorf("unknown restriction type: %s")` for `CONTENT_RATING`.
- **Post-query filtering instead of SQL WHERE clauses**: The plan says "Applied as additional WHERE clauses in all book listing queries." The actual implementation fetches books first, then calls `FilterBookIDs` to remove restricted IDs from the result set. This is functionally correct but less efficient than plan-level SQL filtering.
- **No `reading-stats` endpoint**: Plan specifies `GET /api/users/me/reading-stats` (total read time, books read, etc.). This endpoint does not exist. The data is computed for the dashboard stats but not exposed as a dedicated user settings API.

### Not in Plan but Implemented 📝
- **Content restriction filtering on OPDS and Kobo sync**: The plan (Section 22) only mentions "all book queries." The implementation additionally applies restrictions to OPDS feeds and Kobo device sync, which is a security enhancement beyond the plan's scope.

---

## Summary Table

| Section | Status | Key Gaps |
|---|---|---|
| 16. Email | ❌ Entirely missing | No tables, no package, no routes, no frontend |
| 17. BookDrop | ❌ Entirely missing | No tables, no package, no routes, no frontend |
| 18. Dashboard | 🟡 Partial | Missing `LAST_LISTENED` and `MAGIC_SHELF` row types; no frontend UI to configure rows |
| 19. Notebook | 🟡 Partial | Only annotations (highlights) implemented; bookmarks, notes, PDF annotations, and Markdown export missing |
| 20. Recommendations | ✅ Complete | Fully implemented per plan |
| 21. Audit Logs | 🟡 Partial | Table, handler, and frontend exist, but 7 of 21 action types are never logged; `country` field always empty |
| 22. Content Restrictions | 🟡 Partial | Core engine works, but `CONTENT_RATING` type accepted but non-functional; post-query filtering instead of SQL WHERE clauses |

---

## Code References

### Email (missing)
- Expected migrations: `email_provider`, `email_recipient` — not found.
- Expected package: `internal/email/` — not found.
- Expected routes: `/api/email/*`, `/api/books/{id}/send` — not registered in `internal/server/routes.go`.

### BookDrop (missing)
- Expected migration: `bookdrop_file` — not found.
- Expected package: `internal/bookdrop/` — not found.
- Expected routes: `/api/bookdrop/*` — not registered.

### Dashboard (partial)
- `internal/dashboard/handler.go:47-54` — Row types defined (`CONTINUE_READING`, `RECENTLY_ADDED`, `RANDOM_PICKS`)
- `internal/dashboard/handler.go:64-70` — Default 3 rows
- `web/src/features/dashboard/Dashboard.tsx` — Frontend dashboard
- Missing: `LAST_LISTENED`, `MAGIC_SHELF` row types; row configuration UI.

### Notebook (partial)
- `migrations/010_annotations.up.sql` — `annotation` table (singular, not `annotations`)
- `internal/notebook/handler.go` — Annotation CRUD + notebook listing
- `web/src/features/notebook/Notebook.tsx` — Frontend notebook
- Missing: `book_marks`, `book_notes`, `pdf_annotations` tables and endpoints; Markdown export.

### Recommendations (complete)
- `migrations/013_book_vectors.up.sql` — `book_vectors` table
- `internal/recommendation/vector.go` — Feature hashing, L2 normalization, cosine similarity
- `internal/recommendation/service.go` — Similar book search with per-author cap
- `internal/recommendation/handler.go` — `GET /api/books/{id}/similar`
- `web/src/features/book/BookDetail.tsx:141` — Frontend similar books section

### Audit Logs (partial)
- `migrations/014_audit.up.sql` — `audit_log` table
- `internal/audit/service.go` — Async logging with all 21 action constants
- `internal/audit/handler.go` — `GET /api/admin/audit-logs`
- `web/src/features/admin/AuditLogs.tsx` — Frontend audit log viewer
- Missing log sites: `BOOK_COVER_UPDATED`, `BOOK_SENT`, `BOOKDROP_IMPORTED`, `OPDS_ACCESS`, `KOBO_SYNC`, `KOREADER_SYNC`, `ADMIN_ACTION`

### Content Restrictions (partial)
- `migrations/015_content_restrictions.up.sql` — `user_content_restriction` table
- `internal/contentrestriction/service.go` — `FilterBookIDs` with EXCLUDE/ALLOW_ONLY
- `internal/contentrestriction/handler.go` — CRUD endpoints
- `web/src/features/auth/SettingsPage.tsx:381` — Frontend restriction management
- Bug: `CONTENT_RATING` type accepted by handler but falls through to error in service (`internal/contentrestriction/service.go:175`, `internal/contentrestriction/service.go:217`)

---

## Architecture Documentation

### Post-Query Filtering Pattern
Content restrictions, rather than being injected as SQL WHERE clauses, are applied as a second pass:
1. Query fetches candidate books.
2. Handler extracts book IDs.
3. `contentrestriction.Service.FilterBookIDs()` computes allowed IDs by querying junction tables.
4. Handler rebuilds the result set, keeping only allowed IDs.

This pattern is used consistently across book handler, shelf handler, magic shelf handler, dashboard handler, notebook handler, recommendation handler, OPDS handler, and Kobo handler. It is simpler to implement than dynamic SQL injection but incurs extra queries.

### Audit Service Injection Pattern
The audit service is injected into handlers via `WithAuditService()` methods. However, not all handlers that *could* log events actually do so. The `audit.Service.Log()` method is fire-and-forget (goroutine-based), so it never blocks the caller.

### Dashboard Architecture
The dashboard handler is a single-file package (`internal/dashboard/handler.go`) with no separate service layer. It directly queries the database using raw SQL strings (not sqlc-generated queries), which is an architectural deviation from other domains that use sqlc.

---

## Open Questions

1. **Email and BookDrop implementation status**: These are marked as complete in Phase 29 and Phase 28 of the implementation plan, but no code exists. Were these phases skipped, or is the code in a different branch?
2. **Table naming inconsistency**: Why is the annotation table named `annotation` (singular) when the plan specifies `annotations` (plural)?
3. **CONTENT_RATING query gap**: Is there a missing sqlc query for `CONTENT_RATING`, or should this type be removed from the accepted types?
4. **Audit action coverage**: Several audit action constants are defined but never used. Should they be removed, or should the missing handlers be updated to log them?
5. **Dashboard row configuration UI**: The API supports custom row configs, but the frontend has no UI for it. Is this intentionally deferred?
