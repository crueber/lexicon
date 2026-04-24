---
date: 2026-04-23T00:00:00Z
repository: lexicon
topic: "Audit of Lexicon codebase against implementation plan sections 1-7"
tags: [research, codebase, audit, implementation-plan]
---

# Research: Audit of Lexicon Codebase Against Implementation Plan (Sections 1-7)

**Date**: 2026-04-23
**Git Commit**: 5ea7360
**Branch**: main
**Repository**: github.com/crueber/lexicon

## Research Question
Audit the Lexicon codebase against Sections 1-7 of the implementation plan (`plans/2026-03-23-lexicon-implementation-plan.md`). For each requirement, feature, table, endpoint, or design decision, verify whether it exists in the actual codebase.

## Summary
The Lexicon codebase is substantially implemented through **Phase 36** (marked complete in the plan). Core architecture, authentication, database schema, library/book management, shelves, magic shelves, metadata providers, recommendations, content restrictions, audit logging, Kobo/KOReader sync, OPDS, dashboard, notebook, and task system are all present. However, several planned features are **missing entirely**: **OIDC authentication**, **Remote Auth Header**, **BookDrop**, **Email send-to-device**, and **marked (Markdown) support**. Several API endpoints from Section 7 are also missing or implemented at different paths.

## Detailed Findings

---

## Section 1: Project Overview

### Implemented ✅
- **Multi-user with role-based permissions (ADMIN / USER)**: Implemented via `users` and `user_permissions` tables (`migrations/001_users.up.sql`), JWT claims include `role` and `permissions` (`internal/auth/jwt.go:25-40`), middleware `RequireAdmin` and `RequirePermission` (`internal/auth/middleware.go:55-113`).
- **Multiple libraries with filesystem watch paths**: `library` and `library_path` tables (`migrations/002_libraries.up.sql`), `fsnotify` watcher (`internal/library/watcher.go`), scanner (`internal/library/scanner.go`).
- **Supports: EPUB, PDF, CBZ, CBR, CB7, MOBI, AZW3, FB2, M4B, M4A, MP3, OPUS**: `supportedFormats` map in `internal/library/scanner.go:146-159`.
- **Multiple files per book record**: `book_file` table with `book_id` foreign key (`migrations/003_books.up.sql:11-22`).
- **Folder-based audiobook detection**: `bookTypeForFolder()` in `internal/library/scanner.go:192-212` — if all files in a folder are audio, `book_type = AUDIOBOOK`.
- **In-browser readers: PDF, EPUB, Comic, Audiobook**: Frontend components exist (`web/src/features/reader/{EpubReader,PdfReader,ComicReader,AudiobookPlayer}.tsx`).
- **EPUB highlights, bookmarks, anchored notes, CFI-based progress**: `annotation` table with `cfi`, `text`, `note`, `color` fields (`migrations/010_annotations.up.sql`), CRUD endpoints at `/api/reader/books/{bookId}/annotations` (`internal/notebook/handler.go:46-51`).
- **OPDS 1.2 catalog**: `internal/opds/handler.go` serves Atom feeds at `/opds` with Basic Auth.
- **Kobo device sync**: Full Kobo store API proxy (`internal/kobo/handler.go`), KEPUB conversion (`internal/kobo/kepub.go`), reading state sync (`kobo_reading_state` table).
- **KOReader sync**: KOSync protocol implementation (`internal/koreader/handler.go`), MD5-based Basic Auth, progress sync.
- **Magic Shelves: rule-based dynamic collections**: `magic_shelf` table (`migrations/009_magic_shelves.up.sql`), rule evaluation engine (`internal/shelf/magic.go`), frontend builder (`MagicShelfBuilder.tsx`).
- **Recommendations: feature-hashing vector similarity**: `book_vectors` table (`migrations/013_book_vectors.up.sql`), 128-dim float32 FNV-1a hashing with cosine similarity (`internal/recommendation/vector.go`).
- **Audit logging**: `audit_log` table (`migrations/014_audit.up.sql`), 21 action types defined (`internal/audit/service.go`), admin viewing endpoint (`/api/admin/audit-logs`).

### Missing / Partial ⚠️
- **BookDrop: watch-folder ingest queue**: ❌ Missing entirely. No `internal/bookdrop/` package, no `bookdrop_file` table, no routes.
- **Email send-to-device**: ❌ Missing entirely. No `internal/email/` package, no `email_provider` or `email_recipient` tables. The `can_email_send` permission exists in the schema and JWT but has no functional backend.

### Not in Plan but Implemented 📝
- **Content restrictions per user**: `user_content_restriction` table with `EXCLUDE`/`ALLOW_ONLY` modes (`migrations/015_content_restrictions.up.sql`), filtering service (`internal/contentrestriction/`).
- **Hardcover sync integration**: `hardcover_sync` table (`migrations/018_hardcover.up.sql`), settings API (`/api/users/me/hardcover`).
- **Duplicate detection**: `duplicate_dismiss` table (`migrations/016_duplicates.up.sql`), `FindDuplicates()` function with strict/moderate/loose/title-only presets (`internal/book/duplicates.go`).
- **Font management**: `custom_font` table (`migrations/017_fonts.up.sql`), upload/serve endpoints (`/api/fonts`).
- **Reading sessions tracking**: `reading_sessions` table (`migrations/005_progress.up.sql:11-21`), created/updated on progress save (`internal/reader/handler.go:335-366`).
- **Health check endpoint**: `GET /health` with DB ping (`internal/server/routes.go:18-201`).

---

## Section 2: Architecture & Philosophy

### Implemented ✅
- **Self-contained deployment**: Single Docker image (`Dockerfile`), embedded frontend (`internal/server/dist`), SQLite database. Multi-stage build: frontend → Go → alpine runtime.
- **Minimal dependencies**: `go.mod` confirms pure Go, no CGO (`CGO_ENABLED=0`). Uses `chi`, `sqlc`, `caarlos0/env`, `log/slog` (stdlib), `modernc.org/sqlite`.
- **Developer ergonomics over purity**: `sqlc` for type-safe queries, `chi` for routing, but stdlib `log/slog` and `net/http` used where sufficient.
- **Domain-organized code**: Both backend (`internal/{auth,book,library,shelf,...}`) and frontend (`web/src/features/{auth,book,library,shelf,...}`) organized by domain.
- **Subagent-friendly phasing**: Plan lists 36 phases; git history shows incremental phase completion.

### Missing / Partial ⚠️
- None — architectural principles are fully reflected in the codebase.

---

## Section 3: Technology Stack

### Backend (Go) — Implemented ✅
| Library | Status | Evidence |
|---|---|---|
| `go-chi/chi` v5 | ✅ | `go.mod:11`, `internal/server/routes.go` |
| `modernc.org/sqlite` | ✅ | `go.mod:19`, `internal/server/database_test.go` |
| `sqlc` | ✅ | `sqlc.yaml`, generated `queries.sql.go` files |
| `golang-migrate` | ✅ | `go.mod:13`, `internal/server/migrate.go` |
| `golang-jwt/jwt` v5 | ✅ | `go.mod:12`, `internal/auth/jwt.go` |
| `coreos/go-oidc` v3 | ❌ | **Not in `go.mod`** — OIDC not implemented |
| `coder/websocket` | ✅ | `go.mod:7`, `internal/ws/handler.go` |
| `caarlos0/env` | ✅ | `go.mod:6`, `cmd/lexicon/main.go:21` |
| `log/slog` (stdlib) | ✅ | `cmd/lexicon/main.go:5`, throughout |
| `fsnotify` | ✅ | `go.mod:10`, `internal/library/watcher.go` |
| `disintegration/imaging` | ✅ | `go.mod:9`, `internal/storage/cover.go` |
| `pdfcpu` | ✅ | `go.mod:15`, `internal/storage/metadata.go` |
| `mholt/archives` | ✅ | `go.mod:14`, `internal/reader/comic.go` |
| `dhowden/tag` | ✅ | `go.mod:8`, `internal/storage/metadata.go` |
| `robfig/cron` v3 | ✅ | `go.mod:16`, `internal/task/scheduler.go` |
| `golang.org/x/crypto/bcrypt` | ✅ | `go.mod:17`, `internal/user/service.go` |

### Frontend (SolidJS) — Implemented ✅
| Library | Status | Evidence |
|---|---|---|
| SolidJS + TypeScript | ✅ | `web/package.json:17` |
| Vite | ✅ | `web/package.json:23` |
| `@solidjs/router` | ✅ | `web/package.json:13` |
| `@kobalte/core` | ✅ | `web/package.json:12` |
| Tailwind CSS | ✅ | `web/package.json:21` |
| `lucide-solid` | ✅ | `web/package.json:15` |
| `pdfjs-dist` | ✅ | `web/package.json:16` |
| `epubjs` | ✅ | `web/package.json:14` |
| `marked` | ❌ | **Not in `web/package.json`** — Markdown parser not used |

### Infrastructure — Implemented ✅
| Concern | Status | Evidence |
|---|---|---|
| `alpine:3.20` runtime | ✅ | `Dockerfile:25` |
| Multi-stage Dockerfile | ✅ | `Dockerfile` (frontend-builder → backend-builder → runtime) |
| SQLite (embedded) | ✅ | `internal/server/server.go:74` |

### Missing / Partial ⚠️
- **OIDC provider integration**: ❌ `coreos/go-oidc` not in `go.mod`; no OIDC handlers.
- **Markdown parser (`marked`)**: ❌ Not in frontend dependencies; no Markdown rendering features.

---

## Section 4: Go Project Structure

### Implemented ✅
- **`cmd/lexicon/main.go`**: Thin entry point, parses env, calls `run()`, `os.Exit` only here (`cmd/lexicon/main.go:13-17`).
- **`internal/`**: All application code is unexported.
- **Domain packages own their queries**: Each domain has its own `queries.sql` and generated `queries.sql.go` (`internal/{user,library,book,shelf,metadata,notebook,kobo,koreader,task,audit,contentrestriction,recommendation,reader}/`).
- **`migrations/`**: Top-level directory with 18 migration pairs (`001_users` through `018_hardcover`).
- **`web/`**: SolidJS app organized by feature (`web/src/features/{auth,library,book,reader,shelf,dashboard,notebook,admin,bookdrop}/`).
- **No `internal/config/` package**: Config struct defined in `internal/server/server.go` and parsed in `cmd/lexicon/main.go`.
- **No `internal/db/` package**: Database interactions owned by each domain via sqlc.

### Missing / Partial ⚠️
- **`internal/auth/oidc.go`**: ❌ Missing — OIDC not implemented.
- **`internal/auth/remote.go`**: ❌ Missing — Remote Auth Header not implemented.
- **`internal/bookdrop/`**: ❌ Missing entirely.
- **`internal/email/`**: ❌ Missing entirely.
- **`internal/appsettings/`**: ❌ Missing — app settings are handled via `user` package queries (`GetAppSetting`, `UpsertAppSetting` in `internal/user/queries.sql`).

### Not in Plan but Implemented 📝
- **`internal/contentrestriction/`**: Per-user content restriction service and handler.
- **`internal/hardcover/`**: Hardcover sync service and handler.
- **`internal/task/duplicate_detection.go`**: Background task for duplicate detection.
- **`internal/task/organization.go`**: Background task for file organization.
- **`internal/server/migrate.go`**: Migration runner using `golang-migrate`.
- **`internal/server/dist/`**: Embedded frontend build output.

---

## Section 5: Database Schema

### Implemented ✅ (Tables match plan)
- `users`, `user_permissions`, `user_library_permission`, `user_settings`, `refresh_tokens`
- `library`, `library_path`, `library_metadata_source`
- `book`, `book_file`, `book_metadata`, `comic_metadata`
- `author`, `book_author`, `series`, `book_series`, `category`, `book_category`, `tag`, `book_tag`, `mood`, `book_mood`
- `user_book_file_progress`, `reading_sessions`
- `annotation` (plan says `annotations`)
- `shelf`, `shelf_book` (plan says `book_shelf_mapping`)
- `magic_shelf`
- `kobo_device`, `kobo_reading_state`
- `koreader_user`, `koreader_progress`
- `metadata_fetch_job` (plan says `metadata_fetch_jobs`)
- `metadata_proposal` (plan says `metadata_fetch_proposals`)
- `tasks`, `task_cron_configuration`
- `audit_log`
- `app_settings`
- `custom_font`
- `user_content_restriction`
- `duplicate_dismiss`
- `hardcover_sync`
- `book_vectors`

### Missing / Partial ⚠️
| Planned Table | Status | Notes |
|---|---|---|
| `oidc_session` | ❌ Missing | OIDC not implemented |
| `oidc_group_mapping` | ❌ Missing | OIDC not implemented |
| `book_marks` | ❌ Missing | No separate bookmarks table; bookmarks may be stored as `annotation` with `type='BOOKMARK'` |
| `book_notes` | ❌ Missing | No separate book-level notes table |
| `pdf_annotations` | ❌ Missing | No PDF-specific annotations table |
| `opds_user` | ❌ Missing | OPDS uses main `users` table with Basic Auth |
| `kobo_user_settings` | ❌ Missing | Replaced by `app_settings` keys (`kobo_token_{userID}`) |
| `kobo_library_snapshot` | ❌ Missing | Not implemented |
| `bookdrop_file` | ❌ Missing | BookDrop not implemented |
| `email_provider` | ❌ Missing | Email not implemented |
| `email_recipient` | ❌ Missing | Email not implemented |

---

## Section 6: Authentication & Authorization

### Implemented ✅
- **Local JWT Auth**:
  - `POST /api/auth/login` → returns `{accessToken, refreshToken, user}` (`internal/auth/handler.go:82-213`)
  - `POST /api/auth/refresh` → token rotation with old token revocation (`internal/auth/handler.go:215-341`)
  - `POST /api/auth/logout` → revokes refresh token (`internal/auth/handler.go:343-397`)
  - Access tokens: 15 min, HS256, claims include `sub`, `username`, `role`, `permissions`, `library_ids` (`internal/auth/jwt.go:14-71`)
  - Refresh tokens: 30 days, SHA-256 hashed in `refresh_tokens` table (`internal/auth/jwt.go:73-90`)
  - Middleware: `RequireAuth` extracts Bearer token, validates, injects `Principal` (`internal/auth/middleware.go:26-51`)
- **Authorization Levels**:
  - Public: OPDS with Basic Auth, Kobo with `X-Kobo-UserKey`
  - Authenticated: `RequireAuth` middleware
  - Admin: `RequireAdmin` middleware (`internal/auth/middleware.go:55-70`)
  - Permission-gated: `RequirePermission` middleware (`internal/auth/middleware.go:74-91`)
  - Library-scoped: `Principal.LibraryIDs` checked in handlers (e.g., `internal/book/handler.go:116-126`)

### Missing / Partial ⚠️
- **OIDC**:
  - `GET /api/auth/oidc/providers` ❌
  - `GET /api/auth/oidc/{provider}/authorize` ❌
  - `GET /api/auth/oidc/{provider}/callback` ❌
  - No `oidc_session` table, no `oidc_group_mapping` table, no `coreos/go-oidc` dependency.
- **Remote Auth Header**:
  - Middleware for configurable header extraction (`Remote-User`, `Remote-Email`, `Remote-Groups`) ❌
  - No `remote.go`, no group-based permission mapping.

---

## Section 7: REST API Specification

### 7.1 Auth — Implemented ✅
| Endpoint | Status | Location |
|---|---|---|
| `POST /api/auth/login` | ✅ | `internal/auth/handler.go:82` |
| `POST /api/auth/refresh` | ✅ | `internal/auth/handler.go:215` |
| `POST /api/auth/logout` | ✅ | `internal/auth/handler.go:343` |
| `GET /api/auth/me` | ✅ | `internal/auth/handler.go:399` |
| `PATCH /api/auth/me/password` | ✅ | `internal/auth/handler.go:426` |
| `GET /api/auth/oidc/providers` | ❌ | Not implemented |
| `GET /api/auth/oidc/{provider}/authorize` | ❌ | Not implemented |
| `GET /api/auth/oidc/{provider}/callback` | ❌ | Not implemented |

### 7.2 Users (Admin) — Implemented ✅ / Partial
| Endpoint | Status | Location |
|---|---|---|
| `GET /api/admin/users` | ✅ | `internal/user/handler.go:67-68` |
| `POST /api/admin/users` | ✅ | `internal/user/handler.go:69` |
| `GET /api/admin/users/{id}` | ✅ | `internal/user/handler.go:71` |
| `PUT /api/admin/users/{id}` | ✅ | `internal/user/handler.go:72` |
| `DELETE /api/admin/users/{id}` | ✅ | `internal/user/handler.go:73` |
| `GET /api/admin/users/{id}/permissions` | ⚠️ | Permissions returned inline in `GET /api/admin/users/{id}`; no standalone endpoint |
| `PUT /api/admin/users/{id}/permissions` | ✅ | `internal/user/handler.go:74` |
| `GET /api/admin/users/{id}/libraries` | ⚠️ | Library IDs returned inline in `GET /api/admin/users/{id}`; no standalone endpoint |
| `PUT /api/admin/users/{id}/libraries` | ✅ | `internal/user/handler.go:76` |

### 7.3 Libraries — Implemented ✅ / Partial
| Endpoint | Status | Location |
|---|---|---|
| `GET /api/libraries` | ✅ | `internal/library/handler.go:53` |
| `POST /api/libraries` | ✅ | `internal/library/handler.go:56` |
| `GET /api/libraries/{id}` | ✅ | `internal/library/handler.go:60` |
| `PUT /api/libraries/{id}` | ✅ | `internal/library/handler.go:64` |
| `DELETE /api/libraries/{id}` | ✅ | `internal/library/handler.go:65` |
| `POST /api/libraries/{id}/scan` | ✅ | `internal/library/handler.go:61` |
| `GET /api/libraries/{id}/metadata-sources` | ❌ | Not implemented |
| `PUT /api/libraries/{id}/metadata-sources` | ❌ | Not implemented |

### 7.4 Books — Implemented ✅ / Partial
| Endpoint | Status | Location |
|---|---|---|
| `GET /api/books` | ✅ | `internal/book/handler.go:62` |
| `GET /api/books/{id}` | ✅ | `internal/book/handler.go:64` |
| `PUT /api/books/{id}/metadata` | ❌ | Not implemented |
| `DELETE /api/books/{id}` | ✅ | `internal/book/handler.go:65` |
| `GET /api/books/{id}/files` | ✅ | `internal/book/handler.go:66` |
| `GET /api/books/{id}/cover` | ✅ | `internal/storage/handler.go:39` |
| `PUT /api/books/{id}/cover` | ❌ | Not implemented |
| `DELETE /api/books/{id}/cover` | ❌ | Not implemented |
| `GET /api/books/{id}/similar` | ✅ | `internal/recommendation/handler.go:35` |
| `GET /api/books/duplicates` | ✅ | `internal/book/handler.go:63` |
| `POST /api/books/duplicates/dismiss` | ❌ | Not implemented |
| `POST /api/books/merge` | ❌ | Not implemented |
| `GET /api/books/{id}/reading-sessions` | ❌ | Not implemented |

### 7.5 Book Files — Partial ⚠️
| Endpoint | Status | Location |
|---|---|---|
| `GET /api/files/{id}/download` | ❌ | Not implemented |
| `GET /api/files/{id}/stream` | ⚠️ | Implemented at `/api/reader/books/{bookId}/files/{fileId}/stream` (`internal/reader/handler.go:75`) |
| `GET /api/files/{id}/progress` | ❌ | Not implemented |
| `PUT /api/files/{id}/progress` | ❌ | Progress is at `/api/reader/books/{bookId}/progress` (`internal/reader/handler.go:78-79`) |

### 7.6 Metadata — Implemented ✅ / Partial
| Endpoint | Status | Location |
|---|---|---|
| `GET /api/metadata/providers` | ❌ | Not implemented |
| `POST /api/metadata/search` | ✅ | `internal/metadata/handler.go:41` |
| `GET /api/metadata/proposals` | ❌ | Not implemented (admin-wide pending proposals) |
| `PUT /api/metadata/proposals/{id}/accept` | ✅ | `internal/metadata/handler.go:44` |
| `PUT /api/metadata/proposals/{id}/reject` | ✅ | `internal/metadata/handler.go:45` |
| `POST /api/books/{id}/metadata/fetch` | ❌ | Not implemented |
| `POST /api/books/{id}/metadata/field-lock` | ✅ | Implemented as `PUT /api/metadata/books/{bookId}/lock` (`internal/metadata/handler.go:46`) |

### 7.7 Authors & Series — ❌ Missing
| Endpoint | Status |
|---|---|
| `GET /api/authors` | ❌ |
| `GET /api/authors/{id}` | ❌ |
| `GET /api/authors/{id}/books` | ❌ |
| `PUT /api/authors/{id}` | ❌ |
| `GET /api/series` | ❌ |
| `GET /api/series/{id}` | ❌ |
| `GET /api/series/{id}/books` | ❌ |
| `GET /api/categories` | ❌ |
| `GET /api/tags` | ❌ |
| `GET /api/moods` | ❌ |

### 7.8 Shelves — Implemented ✅
| Endpoint | Status | Location |
|---|---|---|
| `GET /api/shelves` | ✅ | `internal/shelf/handler.go:48` |
| `POST /api/shelves` | ✅ | `internal/shelf/handler.go:49` |
| `GET /api/shelves/{id}` | ✅ | `internal/shelf/handler.go:50` |
| `PUT /api/shelves/{id}` | ✅ | `internal/shelf/handler.go:51` |
| `DELETE /api/shelves/{id}` | ✅ | `internal/shelf/handler.go:52` |
| `GET /api/shelves/{id}/books` | ✅ | `internal/shelf/handler.go:53` |
| `POST /api/shelves/{id}/books` | ✅ | `internal/shelf/handler.go:54` |
| `DELETE /api/shelves/{id}/books/{bookId}` | ✅ | `internal/shelf/handler.go:55` |
| `GET /api/magic-shelves` | ✅ | `internal/shelf/magic_handler.go` |
| `POST /api/magic-shelves` | ✅ | `internal/shelf/magic_handler.go` |
| `GET /api/magic-shelves/{id}` | ✅ | `internal/shelf/magic_handler.go` |
| `PUT /api/magic-shelves/{id}` | ✅ | `internal/shelf/magic_handler.go` |
| `DELETE /api/magic-shelves/{id}` | ✅ | `internal/shelf/magic_handler.go` |
| `GET /api/magic-shelves/{id}/books` | ✅ | `internal/shelf/magic_handler.go` |

### 7.9 Dashboard — Implemented ✅
| Endpoint | Status | Location |
|---|---|---|
| `GET /api/dashboard` | ✅ | `internal/dashboard/handler.go:42` |
| `PUT /api/dashboard/settings` | ✅ | `internal/dashboard/handler.go:44` |

### 7.10 Notebook — Partial ⚠️
| Endpoint | Status | Location |
|---|---|---|
| `GET /api/notebook` | ✅ | `internal/notebook/handler.go:56` |
| `GET /api/notebook/{bookId}` | ❌ | Not implemented |
| `GET /api/annotations` | ❌ | Not implemented (standalone paginated annotations) |
| `POST /api/annotations` | ⚠️ | At `/api/reader/books/{bookId}/annotations` (`internal/notebook/handler.go:48`) |
| `PUT /api/annotations/{id}` | ⚠️ | At `/api/reader/books/{bookId}/annotations/{id}` (`internal/notebook/handler.go:49`) |
| `DELETE /api/annotations/{id}` | ⚠️ | At `/api/reader/books/{bookId}/annotations/{id}` (`internal/notebook/handler.go:50`) |
| `GET /api/bookmarks` | ❌ | Not implemented |
| `POST /api/bookmarks` | ❌ | Not implemented |
| `DELETE /api/bookmarks/{id}` | ❌ | Not implemented |
| `GET /api/books/{id}/notes` | ❌ | Not implemented |
| `POST /api/books/{id}/notes` | ❌ | Not implemented |
| `PUT /api/books/{id}/notes/{noteId}` | ❌ | Not implemented |
| `DELETE /api/books/{id}/notes/{noteId}` | ❌ | Not implemented |
| `GET /api/notebook/export` | ❌ | Not implemented |

### 7.11 OPDS — Partial ⚠️
| Endpoint | Status | Location |
|---|---|---|
| `GET /opds` | ✅ | `internal/opds/handler.go:69` |
| `GET /opds/libraries` | ✅ | `internal/opds/handler.go:71` |
| `GET /opds/libraries/{id}` | ❌ | Not implemented (has `/opds/libraries/{id}/books`) |
| `GET /opds/libraries/{id}/books` | ✅ | `internal/opds/handler.go:72` |
| `GET /opds/shelves` | ✅ | `internal/opds/handler.go:73` |
| `GET /opds/shelves/{id}` | ❌ | Not implemented (has `/opds/shelves/{id}/books`) |
| `GET /opds/series` | ❌ | Not implemented |
| `GET /opds/series/{id}` | ❌ | Not implemented |
| `GET /opds/authors` | ❌ | Not implemented |
| `GET /opds/authors/{id}` | ❌ | Not implemented |
| `GET /opds/search` | ❌ | Not implemented |
| `GET /opds/books/{id}/download/{format}` | ❌ | Implemented as `/opds/books/{id}/files/{fileId}/download` (`internal/opds/handler.go:75`) |

### 7.12 Kobo Sync — Partial ⚠️
| Endpoint | Status | Location |
|---|---|---|
| `GET /kobo/{token}/v1/initialization` | ✅ | `internal/kobo/handler.go:75` (uses `X-Kobo-UserKey` header, not URL token) |
| `GET /kobo/{token}/v1/library/sync` | ✅ | `internal/kobo/handler.go:76` |
| `GET /kobo/{token}/v1/library/{revisionId}/metadata` | ✅ | `internal/kobo/handler.go:77` |
| `GET /kobo/{token}/v1/library/{revisionId}/file` | ✅ | `internal/kobo/handler.go:78` (as `/download`) |
| `PUT /kobo/{token}/v1/library/{revisionId}/state` | ❌ | Not implemented |
| `DELETE /kobo/{token}/v1/library/{revisionId}` | ❌ | Not implemented |
| `GET /kobo/{token}/v1/tags` | ❌ | Not implemented |
| `GET /kobo/{token}/v1/tags/{tagId}/items` | ❌ | Not implemented |
| `DELETE /kobo/{token}/v1/tags/{tagId}/items/delete` | ❌ | Not implemented |
| `POST /kobo/{token}/v1/analytics/gettests` | ❌ | Not implemented |
| `GET /kobo/{token}/v1/user/loyalty/benefits` | ❌ | Not implemented |
| `GET /kobo/{token}/v1/products/prices` | ✅ | Stub (`internal/kobo/handler.go:82`) |
| `GET /kobo/{token}/v1/products/featured/list` | ❌ | Not implemented |
| `GET /kobo/{token}/v1/configuration` | ❌ | Not implemented |
| `GET /api/kobo/settings` | ❌ | Not implemented |
| `PUT /api/kobo/settings` | ❌ | Not implemented |
| `POST /api/kobo/token/generate` | ✅ | Implemented as `POST /api/kobo/token` (`internal/kobo/handler.go:89`) |

### 7.13 KOReader Sync — Implemented ✅
| Endpoint | Status | Location |
|---|---|---|
| `GET /koreader/users/create` | ✅ | Implemented as `POST /kosync/users/create` (`internal/koreader/handler.go:50`) |
| `GET /koreader/users/auth` | ✅ | Implemented as `GET /kosync/users/auth` (`internal/koreader/handler.go:51`) |
| `PUT /koreader/syncs/progress` | ✅ | Implemented as `PUT /kosync/syncs/progress` (`internal/koreader/handler.go:52`) |
| `GET /koreader/syncs/progress` | ✅ | Implemented as `GET /kosync/syncs/progress/{document}` (`internal/koreader/handler.go:53`) |

### 7.14 BookDrop — ❌ Missing Entirely
All endpoints missing. No package, no table, no routes.

### 7.15 Email — ❌ Missing Entirely
All endpoints missing. No package, no table, no routes.

### 7.16 Tasks — Implemented ✅
| Endpoint | Status | Location |
|---|---|---|
| `GET /api/tasks` | ✅ | `internal/task/handler.go:36` |
| `GET /api/tasks/{id}` | ✅ | `internal/task/handler.go:38` |
| `POST /api/tasks/{type}/run` | ✅ | `internal/task/handler.go:39` (admin only) |
| `DELETE /api/tasks/{id}` | ✅ | `internal/task/handler.go:40` (admin only) |
| `GET /api/tasks/cron` | ✅ | `internal/task/handler.go:37` |
| `PUT /api/tasks/cron/{type}` | ✅ | `internal/task/handler.go:41` (admin only) |

**Task types registered** (`internal/server/server.go:96-113`):
- `LIBRARY_SCAN` ✅
- `AUDIT_LOG_CLEANUP` ✅
- `DUPLICATE_DETECTION` ✅
- `FILE_ORGANIZATION` ✅
- `METADATA_REFRESH` ❌ Not registered
- `COVER_REFRESH` ❌ Not registered
- `BOOKDROP_SCAN` ❌ Not registered (BookDrop missing)
- `RECOMMENDATION_REBUILD` ✅

### 7.17 Audit Log — Implemented ✅
| Endpoint | Status | Location |
|---|---|---|
| `GET /api/admin/audit-logs` | ✅ | `internal/audit/handler.go:23` |

**Audit action types present** (`internal/audit/service.go`):
- `USER_LOGIN`, `USER_LOGOUT`, `USER_CREATED`, `USER_UPDATED`, `USER_DELETED` ✅
- `BOOK_DOWNLOADED`, `BOOK_DELETED` ✅
- `BOOK_METADATA_UPDATED`, `BOOK_COVER_UPDATED` ✅ (cover updated logged implicitly)
- `BOOK_SENT` ❌ (Email not implemented)
- `LIBRARY_CREATED`, `LIBRARY_UPDATED`, `LIBRARY_DELETED`, `LIBRARY_SCANNED` ✅
- `SHELF_CREATED`, `SHELF_DELETED` ✅
- `BOOKDROP_IMPORTED` ❌ (BookDrop missing)
- `OPDS_ACCESS` ❌ Not logged
- `KOBO_SYNC` ❌ Not logged
- `KOREADER_SYNC` ❌ Not logged
- `ADMIN_ACTION` ✅ (covered by user/library CRUD actions)

### 7.18 App Settings — Partial ⚠️
| Endpoint | Status | Location |
|---|---|---|
| `GET /api/admin/settings` | ❌ | Not implemented |
| `PUT /api/admin/settings` | ❌ | Not implemented |

**Note**: Only metadata-specific admin settings exist:
- `GET /api/admin/settings/metadata` → `internal/metadata/handler.go:51`
- `PUT /api/admin/settings/metadata` → `internal/metadata/handler.go:52`

### 7.19 Fonts & Icons — Partial ⚠️
| Endpoint | Status | Location |
|---|---|---|
| `GET /api/fonts` | ✅ | `internal/server/routes.go:109` |
| `POST /api/fonts` | ✅ | `internal/server/routes.go:110` |
| `DELETE /api/fonts/{id}` | ✅ | `internal/server/routes.go:111` |
| `GET /api/fonts/{id}/file` | ✅ | `internal/server/routes.go:112` |
| `GET /api/icons` | ❌ | Not implemented |

### 7.20 User Settings — Implemented ✅ / Partial
| Endpoint | Status | Location |
|---|---|---|
| `GET /api/users/me/settings` | ✅ | `internal/user/handler.go:86` |
| `PUT /api/users/me/settings` | ✅ | `internal/user/handler.go:87` |
| `GET /api/users/me/content-restrictions` | ✅ | `internal/server/routes.go:91` |
| `POST /api/users/me/content-restrictions` | ✅ | `internal/server/routes.go:92` |
| `DELETE /api/users/me/content-restrictions/{id}` | ✅ | `internal/server/routes.go:96` |
| `GET /api/users/me/reading-stats` | ❌ | Not implemented |

---

## Code References

### Critical Missing Components
- `internal/auth/oidc.go` — OIDC authentication (not present)
- `internal/auth/remote.go` — Remote auth header (not present)
- `internal/bookdrop/` — BookDrop ingest (not present)
- `internal/email/` — Email send-to-device (not present)
- `internal/appsettings/` — Runtime app settings (not present; handled in `user` package)

### Key Implemented Components
- `cmd/lexicon/main.go:13-44` — Entry point pattern
- `internal/server/server.go:71-266` — Server initialization with all handlers
- `internal/server/routes.go:16-183` — Route registration
- `internal/auth/jwt.go:14-71` — Access token (15 min, HS256)
- `internal/auth/jwt.go:73-90` — Refresh token (30 days, SHA-256 hash)
- `internal/library/scanner.go:240-281` — Library scan with BOOK_PER_FILE / BOOK_PER_FOLDER
- `internal/shelf/magic.go:92-121` — Magic shelf rule-to-SQL builder
- `internal/recommendation/vector.go:22-48` — 128-dim feature hashing
- `internal/ws/hub.go:25-116` — WebSocket hub with per-user broadcasting
- `internal/task/runner.go:15-123` — Goroutine-based task execution with WS progress

### Database Schema
- `migrations/001_users.up.sql` — Users, permissions, settings, refresh tokens, app settings
- `migrations/002_libraries.up.sql` — Libraries, paths, metadata sources
- `migrations/003_books.up.sql` — Books, files, metadata, comic metadata
- `migrations/004_taxonomy.up.sql` — Authors, series, categories, tags, moods
- `migrations/005_progress.up.sql` — Reading progress, sessions
- `migrations/006_tasks.up.sql` — Tasks, cron config
- `migrations/007_shelves.up.sql` — Shelves, shelf_book
- `migrations/008_metadata_jobs.up.sql` — Metadata fetch jobs, proposals
- `migrations/009_magic_shelves.up.sql` — Magic shelves
- `migrations/010_annotations.up.sql` — Annotations
- `migrations/011_kobo.up.sql` — Kobo devices, reading state
- `migrations/012_koreader.up.sql` — KOReader users, progress
- `migrations/013_book_vectors.up.sql` — Recommendation vectors
- `migrations/014_audit.up.sql` — Audit logs
- `migrations/015_content_restrictions.up.sql` — Content restrictions
- `migrations/016_duplicates.up.sql` — Duplicate dismissals
- `migrations/017_fonts.up.sql` — Custom fonts
- `migrations/018_hardcover.up.sql` — Hardcover sync

---

## Architecture Documentation

### Patterns Found
- **Entry point**: `main()` calls `run()`, `os.Exit` only in `main` (`cmd/lexicon/main.go`)
- **Context-first**: All handlers accept `*http.Request` and use `r.Context()`
- **Error wrapping**: `%w` used throughout (e.g., `internal/auth/jwt.go:68`)
- **Interface at consumer**: `shelfHandler` interface defined in `book` package (`internal/book/handler.go:23-25`)
- **Compile-time checks**: `var _ http.Handler = (*Handler)(nil)` in multiple handlers
- **sqlc per domain**: Each domain owns its `.sql` files and generated Go code
- **Middleware chaining**: `chi` router with `RequireAuth`, `RequireAdmin` applied per route group
- **Audit injection**: `WithAuditService` pattern on handlers
- **Content restriction injection**: `WithContentRestrictionService` pattern on handlers

### Frontend Architecture
- **Feature-based organization**: `web/src/features/{auth,library,book,reader,shelf,dashboard,notebook,admin}/`
- **Shared utilities**: `web/src/shared/{api,ws,i18n,ui}/`
- **Typed fetch wrapper**: `web/src/shared/api/client.ts` with automatic token refresh
- **Reconnecting WebSocket**: `web/src/shared/ws/socket.ts` with exponential backoff
- **Custom i18n**: `web/src/shared/i18n/i18n.ts` (~33 lines, signal-based)
- **No external state library**: SolidJS signals + context used throughout

---

## Open Questions
1. **BookDrop and Email**: These were in the implementation plan but have no code. Were they deferred or cut from scope?
2. **OIDC and Remote Auth**: Planned in Section 6 but entirely absent. Intentionally omitted?
3. **Missing taxonomy endpoints**: `/api/authors`, `/api/series`, `/api/categories`, `/api/tags`, `/api/moods` are not implemented. Is taxonomy browsing handled purely via book detail views?
4. **Notebook sub-features**: `book_marks`, `book_notes`, `pdf_annotations` tables are missing. Are annotations meant to cover all use cases?
5. **OPDS completeness**: Search, series, authors feeds missing. Is the current OPDS implementation considered sufficient for basic reader compatibility?
6. **Metadata fetch jobs**: The `metadata_fetch_job` table exists but there's no background job processor for automatic metadata fetching. Is this handled via proposals only?
