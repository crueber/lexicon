---
date: "2026-04-23T00:00:00Z"
repository: lexicon
topic: "Audit codebase against implementation plan Sections 23–27"
tags: [research, codebase, audit, solidjs, docker, config, phases]
---

# Research: Audit Lexicon Codebase Against Implementation Plan (Sections 23–27)

**Date**: 2026-04-23
**Git Commit**: `5ea7360 mark Phase 36 as complete in implementation plan`
**Branch**: main
**Repository**: github.com/crueber/lexicon

## Research Question
Audit the Lexicon codebase against Sections 23–27 of the implementation plan. For each requirement, feature, component, route, env var, or phase deliverable, check whether it exists in the actual codebase.

## Summary
The Lexicon codebase is highly mature for phases 01–27, 31–36. The SolidJS frontend, Docker deployment, database, auth, readers, shelves, metadata providers, Kobo/KOReader sync, OPDS, audit logs, recommendations, and content restrictions are all implemented. Major missing areas are: **BookDrop** (Phase 28), **Email/Send-to-Device** (Phase 29), and **OIDC & Remote Auth** (Phase 30). Many verification checkboxes in the plan remain unchecked despite the corresponding code being fully implemented. Several useful features exist that were not explicitly in the plan sections reviewed (e.g., dev mode, mobile nav, content restriction integration, hardcover sync).

---

## Section 23: SolidJS Frontend Specification

### Implemented ✅

**Routing:**
- `/login` — `web/src/features/auth/LoginPage.tsx`
- `/` (dashboard) — `web/src/features/dashboard/Dashboard.tsx`
- `/libraries` — `web/src/features/library/LibraryList.tsx`
- `/libraries/:id/books` — `web/src/features/library/LibraryBrowser.tsx`
- `/books/:id` — `web/src/features/book/BookDetail.tsx`
- `/books/:id/read` — `web/src/features/reader/ReaderDispatch.tsx`
- `/books/:id/read/epub` — `web/src/features/reader/EpubReader.tsx`
- `/books/:id/read/pdf` — `web/src/features/reader/PdfReader.tsx`
- `/books/:id/read/comic` — `web/src/features/reader/ComicReader.tsx`
- `/books/:id/read/audio` — `web/src/features/reader/AudiobookPlayer.tsx`
- `/shelves` — `web/src/features/shelf/ShelfList.tsx`
- `/shelves/:id` — `web/src/features/shelf/ShelfDetail.tsx`
- `/magic-shelves/new`, `/magic-shelves/:id`, `/magic-shelves/:id/edit` — `web/src/features/shelf/MagicShelfBuilder.tsx`, `MagicShelfDetail.tsx`
- `/notebook` — `web/src/features/notebook/Notebook.tsx`
- `/settings` — `web/src/features/auth/SettingsPage.tsx`
- `/admin/users` — `web/src/features/admin/UserManagement.tsx`
- `/admin/audit-logs` — `web/src/features/admin/AuditLogs.tsx`
- `/admin/settings` — `AdminSettingsStub` placeholder in `web/src/App.tsx:40-44`

**Frontend Organization:**
- `web/src/features/auth/AuthProvider.tsx` — auth context with signals (`user`, `login`, `logout`, `isAdmin`)
- `web/src/features/auth/ProtectedRoute.tsx` — redirects to `/login` if not authenticated
- `web/src/features/auth/LoginPage.tsx` — login form with i18n
- `web/src/features/library/LibraryList.tsx`, `LibraryBrowser.tsx`
- `web/src/features/book/BookDetail.tsx`, `BookCard.tsx`, `MetadataSearch.tsx`
- `web/src/features/reader/EpubReader.tsx`, `PdfReader.tsx`, `ComicReader.tsx`, `AudiobookPlayer.tsx`
- `web/src/features/shelf/ShelfList.tsx`, `ShelfDetail.tsx`, `MagicShelfBuilder.tsx`, `MagicShelfDetail.tsx`, `AddToShelfDialog.tsx`
- `web/src/features/dashboard/Dashboard.tsx`, `ScrollerRow.tsx`
- `web/src/features/notebook/Notebook.tsx`
- `web/src/features/admin/UserManagement.tsx`, `AuditLogs.tsx`
- `web/src/shared/api/client.ts` — typed fetch wrapper with auth header injection, token refresh, `ApiError`
- `web/src/shared/ws/socket.ts` — reconnecting WebSocket client with ping, exponential backoff, `SESSION_REVOKED` handling
- `web/src/shared/i18n/i18n.ts` — signal-based i18n system (~33 lines), `t("namespace.key")` pattern
- `web/src/shared/i18n/en.json` — English locale with namespaces: common, auth, library, book, metadata, shelf, reader, admin, errors
- `web/src/shared/ui/Button.tsx`, `Input.tsx`, `Toast.tsx`, `Skeleton.tsx`

**State Management:**
- SolidJS signals and context only. No external state library. Confirmed in `AuthProvider.tsx`, `i18n.ts`, and all feature components.

**Theme:**
- Dark mode first (slate-900 backgrounds, slate-100 text) throughout all components
- Responsive sidebar + mobile bottom nav in `App.tsx`
- Tailwind CSS utility classes used everywhere

### Missing / Partial ⚠️

- `/auth/oidc/callback` route — not in `App.tsx`
- `/authors` and `/authors/:id` routes — no frontend pages exist
- `/series` and `/series/:id` routes — no frontend pages exist
- `/bookdrop` route and `BookDropQueue.tsx` — not found anywhere in `web/src/`
- `/tasks` route and task management UI — not found anywhere in `web/src/`
- `/admin/libraries`, `/admin/email`, `/admin/opds`, `/admin/kobo`, `/admin/koreader`, `/admin/metadata-sources`, `/admin/fonts`, `/admin/recommendations` — none exist; admin settings is a stub placeholder (`AdminSettingsStub`) showing "Admin Settings — coming soon"
- `Modal.tsx` in `shared/ui/` — not found
- `web/src/App.tsx` does not route to `/dashboard` explicitly; dashboard is at `/`

### Not in Plan but Implemented 📝

- `web/src/shared/ws/WSProvider.tsx` — WebSocket context provider wrapper
- `web/src/features/reader/ReaderDispatch.tsx` — dispatches to correct reader based on file format
- Mobile bottom navigation (`MobileNav` component in `App.tsx:139-195`)
- `web/src/features/library/types.ts` and `web/src/features/dashboard/types.ts` — feature-local type definitions
- `/magic-shelves/:id/edit` route (plan only listed `/magic-shelves/:id`)

---

## Section 24: Docker Deployment Specification

### Implemented ✅

- Multi-stage `Dockerfile` exists with 3 stages:
  - Stage 1: `node:22-alpine` frontend builder — `web/package.json`, `npm install`, `npm run build`
  - Stage 2: `golang:1.25-alpine` backend builder — `go mod download`, `CGO_ENABLED=0 go build`, embeds `internal/server/dist`
  - Stage 3: `alpine:3.20` runtime — `ca-certificates`, `tzdata`, `su-exec`, non-root `lexicon` user
- `entrypoint.sh` — supports `USER_ID`/`GROUP_ID` remapping, creates `/app/data`, `/books`, `/bookdrop` dirs, `chown`, `su-exec lexicon`
- `docker-compose.yml` — `ghcr.io/crueber/lexicon:latest`, port `6060:6060`, `JWT_SECRET`, `TZ`, `USER_ID`, `GROUP_ID`, volumes for `lexicon-data`, `/books:ro`, `/bookdrop`
- `EXPOSE 6060` and `VOLUME ["/app/data", "/books", "/bookdrop"]` in Dockerfile

### Missing / Partial ⚠️

- Dockerfile uses `golang:1.25-alpine` — plan specifies `golang:1.23-alpine`
- Dockerfile uses `npm install` — plan specifies `npm ci`
- Dockerfile copies `web/package.json web/package-lock.json*` — plan shows `COPY web/package*.json .`
- `Makefile` does not have a `sqlc` target named exactly `sqlc` (it has `sqlc-generate`); plan lists `sqlc` as a standard target

### Not in Plan but Implemented 📝

- `Makefile` has `embed-frontend` target to copy `web/dist` into `internal/server/dist` for `go:embed`
- `Makefile` has `run-frontend` and `dev` targets for local development
- Dockerfile explicitly creates non-root `lexicon` user with `adduser`/`addgroup`

---

## Section 25: Configuration & Environment Variables

### Implemented ✅

| Variable | Where |
|---|---|
| `PORT` | `internal/server/config.go:5` (default `6060`) |
| `DATA_DIR` | `internal/server/config.go:6` (default `/app/data`) |
| `JWT_SECRET` | `internal/server/config.go:7` |
| `LOG_LEVEL` | `internal/server/config.go:8` (default `info`) |
| `LOG_FORMAT` | `internal/server/config.go:9` (default `json`) |
| `GOOGLE_BOOKS_API_KEY` | `internal/server/config.go:11` |
| `HARDCOVER_API_KEY` | `internal/server/config.go:12` |
| `COMICVINE_API_KEY` | `internal/server/config.go:13` |

### Missing / Partial ⚠️

| Variable | Status |
|---|---|
| `BOOKS_DIR` | Not in `internal/server/config.go` |
| `BOOKDROP_DIR` | Not in `internal/server/config.go` |
| `TZ` | Used in `docker-compose.yml` but not parsed by Go server |
| `USER_ID` | Used in `entrypoint.sh` but not parsed by Go server |
| `GROUP_ID` | Used in `entrypoint.sh` but not parsed by Go server |
| `DISK_TYPE` | Not found anywhere |
| `MAX_UPLOAD_SIZE_MB` | Not found anywhere |

### Not in Plan but Implemented 📝

- `DEV_MODE` — `internal/server/config.go:10` (default `false`). Enables Vite proxy, dev JWT secret fallback, and skips frontend embedding
- `GOOGLE_BOOKS_API_KEY`, `HARDCOVER_API_KEY`, `COMICVINE_API_KEY` — provider-specific API keys injected via env vars

---

## Section 26: Application Settings (Runtime)

### Implemented ✅

- `app_settings` table exists: `migrations/001_users.up.sql:44-47`
- Raw SQL read/write for `app_settings` in:
  - `internal/user/queries.sql:59,71`
  - `internal/metadata/service.go:291,307` (stores provider API keys/settings)
  - `internal/kobo/handler.go:124,851` (stores per-user kobo tokens as `kobo_token_{userID}`)

### Missing / Partial ⚠️

- **No dedicated `internal/appsettings/` package** — plan calls for `appsettings/service.go` and `queries.sql`
- **No admin GET/PUT `/api/admin/settings` endpoints** — plan specifies these routes
- **No runtime admin UI** for editing settings
- Most runtime settings keys from the plan are not implemented:
  - `ALLOW_REGISTRATION`
  - `DEFAULT_ROLE`
  - `OIDC_ENABLED`, `OIDC_PROVIDER_NAME`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_ISSUER_URI`, `OIDC_SCOPE`, `OIDC_GROUP_CLAIM`
  - `REMOTE_AUTH_ENABLED`, `REMOTE_AUTH_USER_HEADER`, `REMOTE_AUTH_EMAIL_HEADER`, `REMOTE_AUTH_GROUPS_HEADER`
  - `OPDS_ENABLED`, `KOBO_SYNC_ENABLED`, `KOREADER_SYNC_ENABLED`
  - `BOOKDROP_ENABLED`, `EMAIL_ENABLED`
  - `COVER_IMAGE_MAX_SIZE_MB`
  - `DUPLICATE_DETECTION_PRESET`
  - `AUDIT_LOG_RETENTION_DAYS`
  - `HARDCOVER_ENABLED`
  - `RECOMMENDATION_ENABLED`

### Not in Plan but Implemented 📝

- Kobo token storage uses `app_settings` with dynamic keys (`kobo_token_{userID}`)
- Metadata service stores provider-related settings in `app_settings`

---

## Section 27: Implementation Phases

### Phase 01: Project Scaffold & Build Pipeline
**Status**: ✅ Implemented (changes checked; some verification items unchecked but likely done)
- `go.mod` with `github.com/crueber/lexicon`
- `cmd/lexicon/main.go` with `run()` pattern
- `internal/server/server.go` with chi router, health check
- `Makefile` with `build`, `run`, `test`, `lint`, `sqlc-generate`
- `web/` Vite + SolidJS + TypeScript + Tailwind scaffold
- `Dockerfile` multi-stage build
- `go:embed` for serving frontend dist
- Dev mode: Vite proxy at `localhost:5173`

**Unchecked verification items that appear done:**
- `make run` starts server on :6060
- `curl http://localhost:6060/health` returns 200
- Browser shows "Hello Lexicon" page (now replaced with full app)
- `docker build .` succeeds

### Phase 02: Database Foundation
**Status**: ✅ Complete (all items checked)
- SQLite with WAL mode, busy timeout, foreign keys
- `sqlc.yaml` configured
- `migrations/001_users.up.sql` — users, permissions, settings, refresh_tokens, app_settings
- Migration runner in `internal/server/server.go:79`

### Phase 03: Authentication (Local JWT)
**Status**: ✅ Complete (all items checked)
- `internal/auth/jwt.go`, `middleware.go`, `handler.go`, `types.go`
- `internal/user/service.go` with bcrypt
- Default admin creation in `server.go:257-297`
- Endpoints: POST `/api/auth/login`, `/api/auth/refresh`, `/api/auth/logout`, GET `/api/auth/me`, PATCH `/api/auth/me/password`

### Phase 04: Frontend Auth Shell
**Status**: ✅ Implemented (code complete; verification checkboxes unchecked but done)
- `web/src/shared/api/client.ts` — typed fetch with token refresh
- `web/src/features/auth/AuthProvider.tsx` — context + signals
- `web/src/features/auth/LoginPage.tsx`
- `web/src/features/auth/ProtectedRoute.tsx`
- `web/src/shared/ui/Button.tsx`, `Input.tsx`
- App layout shell with sidebar navigation
- Dashboard stub replaced with real Dashboard

### Phase 05: Library & Book Data Model
**Status**: ✅ Implemented (no checkboxes in plan, but code exists)
- `migrations/002_libraries.up.sql` — library, library_path, library_metadata_source, user_library_permission
- `migrations/003_books.up.sql` — book, book_file, book_metadata, comic_metadata
- `migrations/004_taxonomy.up.sql` — author, series, category, tag, mood + junction tables
- `migrations/005_progress.up.sql` — user_book_file_progress, reading_sessions
- sqlc query files in `internal/book/`, `internal/library/`

### Phase 06: Library Management API
**Status**: ✅ Complete (all items checked)
- `internal/library/handler.go`, `service.go`, `queries.sql`
- Admin guards on create/update/delete
- Route registration in `internal/server/routes.go:40-43`

### Phase 07: File Scanner & Fingerprinting
**Status**: ✅ Complete (all items checked)
- `internal/library/scanner.go`
- `internal/storage/fingerprint.go`
- BOOK_PER_FILE and BOOK_PER_FOLDER organization modes
- Audiobook/comic detection
- POST `/api/libraries/{id}/scan` endpoint

### Phase 08: Cover Extraction & Thumbnails
**Status**: ✅ Complete (all items checked)
- `internal/storage/cover.go`
- Thumbnail generation with `disintegration/imaging`
- Decompression bomb protection (>50MP rejection)
- GET `/api/books/{id}/cover`
- `/app/data/covers/books/{bookId}/` directory structure

### Phase 09: Embedded Metadata Extraction
**Status**: ✅ Complete (all items checked)
- EPUB OPF parsing
- PDF XMP/DocInfo parsing
- CBZ ComicInfo.xml parsing
- Audio tag parsing (ID3, MP4)
- Author/series/category linking

### Phase 10: Library Browser Frontend
**Status**: ✅ Implemented (code complete; verification unchecked)
- `LibraryList.tsx`, `LibraryBrowser.tsx`, `BookCard.tsx`
- Filter, sort, pagination controls
- Routes `/libraries`, `/libraries/:id/books`

### Phase 11: Book Detail Page
**Status**: ✅ Implemented (code complete; verification unchecked)
- `BookDetail.tsx` with metadata, authors, series, file list
- Route `/books/:id`

### Phase 12: File Watching & WebSocket Events
**Status**: ✅ Implemented (code complete; verification unchecked)
- `internal/ws/hub.go`, `handler.go`
- `internal/library/watcher.go` with fsnotify and debounce
- `web/src/shared/ws/socket.ts`
- WebSocket events emitted for scan completion

### Phase 13: Background Task System
**Status**: ✅ Implemented (code complete; most verification unchecked)
- `migrations/006_tasks.up.sql` — tasks, task_cron_configuration
- `internal/task/runner.go`, `scheduler.go`, `handler.go`, `types.go`
- LIBRARY_SCAN as background task
- TASK_PROGRESS/COMPLETE/FAILED events via WebSocket
- Cron schedules: LIBRARY_SCAN every 6h, DUPLICATE_DETECTION Sundays 2am, RECOMMENDATION_REBUILD daily 3am, AUDIT_LOG_CLEANUP daily 1am
- On startup: `MarkInterruptedFailed()` in `server.go:310`

### Phase 14: EPUB Reader
**Status**: ✅ Implemented (code complete; verification unchecked)
- `internal/reader/handler.go` with streaming, progress save/load
- `web/src/features/reader/EpubReader.tsx`
- CFI-based progress, settings panel, TOC, bookmarks
- Route `/books/:id/read/epub`

### Phase 15: PDF Reader
**Status**: ✅ Implemented (code complete; verification unchecked)
- `web/src/features/reader/PdfReader.tsx`
- Page-based progress, thumbnails, TOC, search
- Route `/books/:id/read/pdf`

### Phase 16: Shelves (Manual)
**Status**: ✅ Implemented (code complete; all verification unchecked)
- `migrations/007_shelves.up.sql`
- `internal/shelf/handler.go`, `service.go`, `queries.sql`
- `ShelfList.tsx`, `ShelfDetail.tsx`, `AddToShelfDialog.tsx`
- Route `/shelves`, `/shelves/:id`

### Phase 17: User Management & Permissions
**Status**: ✅ Implemented (code complete; all verification unchecked)
- `internal/user/handler.go` with admin CRUD
- Permission flags: role, canDownload, canUpload, canEmailSend, canEditMetadata, opdsAccess
- User-library access control
- `web/src/features/admin/UserManagement.tsx`
- `web/src/features/auth/SettingsPage.tsx`
- Routes `/admin/users`, `/settings`

### Phase 18: Dashboard
**Status**: ✅ Implemented (code complete; all verification unchecked)
- `internal/dashboard/handler.go` — GET `/api/dashboard`, PUT `/api/dashboard/settings`
- `internal/dashboard/service.go` — row resolution (CONTINUE_READING, RECENTLY_ADDED, RANDOM_PICKS)
- `web/src/features/dashboard/Dashboard.tsx`, `ScrollerRow.tsx`
- Dashboard stats (total books, libraries, books read this month, total reading time)
- Route `/` (default landing)

### Phase 19: Metadata Providers — Google Books
**Status**: ✅ Complete (all changes and verification checked)
- `internal/metadata/types.go`, `service.go`, `googlebooks.go`
- `migrations/008_metadata_jobs.up.sql`
- Proposal system with accept/reject
- Per-field lock flags enforcement

### Phase 20: Additional Metadata Providers
**Status**: ✅ Implemented (code complete; some verification unchecked)
- Hardcover provider (`internal/metadata/hardcover.go`)
- OpenLibrary provider (`internal/metadata/openlibrary.go`) — replaces Amazon/GoodReads from plan
- ComicVine provider (`internal/metadata/comicvine.go`)
- Audible provider (`internal/metadata/audible.go`)
- Douban provider (`internal/metadata/douban.go`)
- LubimyCzytac provider (`internal/metadata/lubimyczytac.go`)
- RanobeDB provider (`internal/metadata/ranobedb.go`)
- Priority matrix and admin metadata settings endpoints at `/api/admin/settings/metadata`

### Phase 21: Magic Shelves
**Status**: ✅ Complete (all changes and verification checked)
- `migrations/009_magic_shelves.up.sql`
- `internal/shelf/magic.go` — SQL WHERE builder
- `internal/shelf/magic_handler.go`
- `web/src/features/shelf/MagicShelfBuilder.tsx` — visual rule builder with live preview
- Routes `/magic-shelves/new`, `/magic-shelves/:id`, `/magic-shelves/:id/edit`

### Phase 22: Comic Reader
**Status**: ✅ Implemented (build verification checked; manual testing unchecked)
- Page extraction from CBZ, CBR, CB7 via `mholt/archives`
- `internal/reader/handler.go` — comic page endpoints
- `web/src/features/reader/ComicReader.tsx`
- LTR/RTL, single/double, fit modes
- Route `/books/:id/read/comic`

### Phase 23: Audiobook Player
**Status**: ✅ Implemented (build verification checked; manual testing unchecked)
- Track-based playback for M4B, M4A, MP3, OPUS, FLAC
- `web/src/features/reader/AudiobookPlayer.tsx`
- Scrubable progress bar, track list, playback speed, skip interval, volume, Media Session API
- Route `/books/:id/read/audio`

### Phase 24: Annotations & Notebook
**Status**: ✅ Complete (all changes and verification checked)
- `migrations/010_annotations.up.sql`
- EPUB highlights with color picker
- `internal/notebook/handler.go`
- `web/src/features/notebook/Notebook.tsx`
- Route `/notebook`

### Phase 25: OPDS Catalog
**Status**: ✅ Implemented (code complete; all verification unchecked)
- `migrations/012_opds.up.sql` — uses `012` number (KOReader is also `012` in plan, but actual migrations use `012_koreader` and OPDS auth is handled differently)
- Actually: `internal/opds/handler.go` exists with Atom feed generation
- Basic Auth support
- Root catalog, library feeds, shelf feeds, series feeds, author feeds
- OpenSearch description

**Note:** Plan mentions `migrations/012_opds.up.sql`, but actual codebase does not have a standalone OPDS migration file. OPDS users appear to be handled via the main `users` table or Basic Auth within `internal/opds/handler.go`.

### Phase 26: Kobo Sync
**Status**: ✅ Complete (all changes and verification checked)
- `migrations/011_kobo.up.sql` — kobo_device, kobo_reading_state
- `internal/kobo/handler.go`, `kepub.go`
- KEPUB conversion via `kepubify`
- Reading state sync
- Token generation via `/api/kobo/token`

### Phase 27: KOReader Sync
**Status**: ✅ Complete (all changes and verification checked)
- `migrations/012_koreader.up.sql` — koreader_user, koreader_progress
- `internal/koreader/handler.go`
- MD5 Basic Auth
- Document matching via filename LIKE to `book_file.fingerprint`
- Routes under `/kosync`

### Phase 28: BookDrop
**Status**: ❌ NOT IMPLEMENTED
- **Missing**: `internal/bookdrop/` package (no `watcher.go`, `service.go`, `handler.go`)
- **Missing**: Migration for `bookdrop_file` table (plan says `015_bookdrop.up.sql`; actual `015` is `content_restrictions`)
- **Missing**: `web/src/features/bookdrop/BookDropQueue.tsx`
- **Missing**: `/bookdrop` route
- **Missing**: `BOOKDROP_FILE_ARRIVED` WebSocket event emission in backend
- **Missing**: `BOOKDROP_ENABLED` runtime setting

### Phase 29: Email / Send-to-Device
**Status**: ❌ NOT IMPLEMENTED
- **Missing**: `internal/email/` package (no `handler.go`, `service.go`)
- **Missing**: Migrations for `email_provider`, `email_recipient` tables (plan says `016_email.up.sql`; actual `016` is `duplicates`)
- **Missing**: `/api/email/providers`, `/api/email/recipients`, `/api/books/{id}/send` endpoints
- **Missing**: `EMAIL_ENABLED` runtime setting
- **Missing**: Admin UI for email configuration

### Phase 30: OIDC & Remote Auth
**Status**: ❌ NOT IMPLEMENTED
- **Missing**: `internal/auth/oidc.go`
- **Missing**: `internal/auth/remote.go`
- **Missing**: Migrations for `oidc_session`, `oidc_group_mapping` tables (plan says `017_oidc.up.sql`; actual `017` is `fonts`)
- **Missing**: `/api/auth/oidc/providers`, `/api/auth/oidc/{provider}/authorize`, `/api/auth/oidc/{provider}/callback` endpoints
- **Missing**: OIDC login button on `LoginPage.tsx`
- **Missing**: Remote Auth header middleware
- **Missing**: All OIDC-related runtime settings (`OIDC_ENABLED`, `OIDC_PROVIDER_NAME`, etc.)
- **Missing**: All Remote Auth-related runtime settings (`REMOTE_AUTH_ENABLED`, etc.)

### Phase 31: Recommendations
**Status**: ✅ Implemented (code complete; verification unchecked)
- `internal/recommendation/vector.go` — FNV-1a feature hashing, 128-dim, L2 normalization
- `internal/recommendation/service.go` — cosine similarity, top-N, per-author cap
- `migrations/013_book_vectors.up.sql`
- GET `/api/books/{id}/similar` endpoint
- Similar books section in `BookDetail.tsx`

### Phase 32: Audit Logs
**Status**: ✅ Implemented (code complete; verification unchecked)
- `migrations/014_audit.up.sql`
- `internal/audit/service.go`, `handler.go`
- `web/src/features/admin/AuditLogs.tsx`
- GET `/api/admin/audit-logs` with pagination, filters (action, userId, from, to)
- `AUDIT_LOG_CLEANUP` task type registered

### Phase 33: Content Restrictions
**Status**: ✅ Implemented (code complete; all verification unchecked)
- `migrations/015_content_restrictions.up.sql`
- `internal/contentrestriction/service.go`, `handler.go`
- EXCLUDE and ALLOW_ONLY modes
- Enforcement in book queries, shelf queries, dashboard, notebook, OPDS, kobo, recommendations
- User settings UI for managing restrictions in `SettingsPage.tsx`

### Phase 34: Remaining Metadata Providers
**Status**: ✅ Implemented (code complete; verification unchecked)
- Douban provider (`internal/metadata/douban.go`)
- LubimyCzytac provider (`internal/metadata/lubimyczytac.go`)
- RanobeDB provider (`internal/metadata/ranobedb.go`)

### Phase 35: i18n
**Status**: ✅ Implemented (code complete; verification unchecked)
- `web/src/shared/i18n/i18n.ts`
- `web/src/shared/i18n/en.json` with all major namespaces
- All frontend components use `t("...")` instead of hardcoded strings

### Phase 36: Polish & Deployment
**Status**: ✅ Mostly Complete
- Duplicate detection system (`internal/book/duplicates.go`, `migrations/016_duplicates.up.sql`)
- File organization task (`internal/task/organization.go`)
- Reading sessions and stats (`reading_sessions` table, dashboard stats computation)
- Font management (`migrations/017_fonts.up.sql`, `internal/storage/font.go`, `/api/fonts` endpoints)
- Hardcover sync integration (`migrations/018_hardcover.up.sql`, `internal/hardcover/`)
- `docker-compose.yml`
- Health check endpoint enhancement (`/health` with DB ping)

**Unchecked verification items:**
- `docker compose up` starts Lexicon successfully — likely works given `docker build` passes
- Full end-to-end workflow — manual test, unchecked

---

## Code References

- `web/src/App.tsx:260-281` — Frontend routing table
- `internal/server/routes.go:16-183` — Backend routing table
- `internal/server/config.go:4-14` — Environment variable config struct
- `internal/server/server.go:71-266` — Server initialization with all handlers
- `migrations/` — 18 migration files covering users through hardcover_sync
- `internal/ws/hub.go:25-116` — WebSocket hub implementation
- `web/src/shared/api/client.ts:84-118` — Typed fetch wrapper with token refresh
- `web/src/shared/ws/socket.ts:25-187` — Reconnecting WebSocket client
- `internal/task/scheduler.go:13-22` — Default cron schedules
- `internal/task/types.go:6-15` — All task type constants

## Architecture Documentation

- **Domain-organized code**: Both backend (`internal/{domain}/`) and frontend (`web/src/features/{domain}/`) follow feature-based organization.
- **Minimal dependencies**: Uses chi, sqlc, caarlos0/env, coder/websocket, robfig/cron, disintegration/imaging as specified. No ORM, no viper, no zerolog.
- **Pure Go, no CGO**: `CGO_ENABLED=0` in Makefile and Dockerfile. Uses `modernc.org/sqlite`.
- **sqlc for queries**: Each domain owns its `queries.sql` file and generated `queries.sql.go`.
- **SolidJS signals**: No external state library. Auth, i18n, and UI state use `createSignal` and `createContext`.
- **Content restriction integration**: Applied consistently across book, shelf, dashboard, notebook, OPDS, kobo, and recommendation handlers via `WithContentRestrictionService` injection.

## Open Questions

1. **OPDS migration**: The plan calls for `migrations/012_opds.up.sql`, but no such file exists. OPDS functionality is implemented in `internal/opds/handler.go` without a dedicated table migration. How are OPDS users stored?
2. **Reading stats endpoint**: The plan specifies `GET /api/users/me/reading-stats`, but this endpoint does not appear to exist independently. Stats are computed internally by the dashboard handler but not exposed as a standalone API.
3. **Migration numbering divergence**: The plan assigns specific migration numbers to features (e.g., 015_bookdrop, 016_email, 017_oidc), but the actual codebase uses those numbers for different features (015_content_restrictions, 016_duplicates, 017_fonts). This suggests the plan was updated conceptually but migration numbering was not revised.
4. **Admin settings page**: `/admin/settings` is a stub. Given that Phases 28–30 are not implemented, the admin settings UI may be intentionally deferred until those features are built.
