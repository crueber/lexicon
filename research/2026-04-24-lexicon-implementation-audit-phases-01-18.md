# Lexicon Implementation Audit — Phases 01-18

**Date**: 2026-04-24
**Git Commit**: `26cf6f7f192c35c61e958186af36da332b1b0c59`
**Branch**: `main`
**Repository**: `/Users/crueber/dev/github.com/crueber/lexicon`

---

## Phase 01: Project Scaffold & Build Pipeline
- **Status**: COMPLETE
- **Evidence**:
  - `go.mod:1` — module `github.com/crueber/lexicon`
  - `cmd/lexicon/main.go:1-44` — thin entry point with `run()` pattern, `os.Exit` only in `main`
  - `internal/server/server.go:1-543` — chi router, HTTP server setup, health check at `/health` (`routes.go:18`)
  - `Makefile:1-69` — targets: `build`, `build-frontend`, `embed-frontend`, `run`, `run-frontend`, `dev`, `test`, `lint`, `sqlc-generate`, `docker-build`, `migrate-up`, `migrate-down`, `clean`
  - `web/package.json` — Vite + SolidJS + TypeScript + Tailwind CSS scaffold
  - `Dockerfile:1-41` — multi-stage build (node frontend → golang backend → alpine runtime)
  - `internal/server/embed.go` / `embed_dev.go` — `go:embed` for serving `internal/server/dist` in production
  - `internal/server/routes.go:260-284` — `viteProxy()` reverse proxies to `localhost:5173` in dev mode
- **Gaps**:
  - `web/src/App.tsx` no longer shows "Hello Lexicon"; it was replaced by the full application shell in later phases. This is expected evolution, not a missing item.
  - Verification items `make run`, `curl /health`, browser check, and `docker build` are runtime/integration tests that cannot be statically verified, but all prerequisite code is present.

---

## Phase 02: Database Foundation
- **Status**: COMPLETE
- **Evidence**:
  - `internal/server/database.go:1-58` — `OpenDatabase()` opens SQLite, sets `PRAGMA journal_mode=WAL`, `PRAGMA busy_timeout=5000`, `PRAGMA foreign_keys=ON`, `PRAGMA synchronous=NORMAL`
  - `sqlc.yaml:1-139` — configured for SQLite with 15 query packages
  - `migrations/001_users.up.sql:1-47` — `users`, `user_permissions`, `user_settings`, `refresh_tokens`, `app_settings`
  - `migrations/001_users.down.sql` — exists
  - `internal/user/queries.sql` — sqlc queries for user CRUD (e.g., `GetUserByUsername`, `CreateRefreshToken`)
  - `internal/server/migrate.go:1-51` — `RunMigrations()` using `golang-migrate` with embedded `iofs` source
  - `internal/server/server.go:85-93` — `OpenDatabase()` and `RunMigrations()` called on startup
- **Gaps**: None

---

## Phase 03: Authentication (Local JWT)
- **Status**: COMPLETE
- **Evidence**:
  - `internal/auth/jwt.go:1-130` — `IssueAccessToken` (HS256, 15 min expiry), `IssueRefreshToken` (random 32-byte + SHA-256 hash), `ValidateAccessToken`
  - `internal/auth/middleware.go:1-125` — `RequireAuth` (Bearer token extraction + validation), `RequireAdmin`, `RequirePermission`
  - `internal/auth/handler.go:1-498` — `HandleLogin`, `HandleRefresh`, `HandleLogout`, `HandleMe`, `HandleChangePassword`
  - `internal/auth/types.go:1-24` — `Principal`, `Permissions` structs
  - `internal/user/` — password hashing (`HashPassword`, `VerifyPassword` using bcrypt) in generated/sqlc code; `CreateAdminUser` helper
  - `internal/server/server.go:367-407` — `ensureDefaultAdmin()` creates `admin/admin` if no users exist
  - `internal/server/routes.go:26-43` — auth routes registered at `/api/auth/...`
- **Gaps**: None

---

## Phase 04: Frontend Auth Shell
- **Status**: COMPLETE
- **Evidence**:
  - `web/src/shared/api/client.ts:1-118` — typed `api<T>()` wrapper with `Authorization: Bearer` injection, 401 → `tryRefreshToken()` → retry
  - `web/src/features/auth/AuthProvider.tsx` — auth context with signals: `user`, `login`, `logout`, `isAdmin`, `isAuthenticated`
  - `web/src/features/auth/LoginPage.tsx` — login form
  - `web/src/features/auth/ProtectedRoute.tsx` — redirects to `/login` if not authenticated
  - `web/src/shared/ui/Button.tsx`, `Input.tsx`, `Toast.tsx`, `Skeleton.tsx` — Kobalte-based UI primitives
  - `web/src/App.tsx:54-251` — layout shell with sidebar navigation + main content area + mobile bottom nav
  - `web/src/App.tsx:266-311` — router setup with protected routes via `AppLayout` wrapping `ProtectedRoute`
  - Dashboard route `/` exists (real `Dashboard` component from Phase 18 replaced any earlier stub)
- **Gaps**:
  - "Dashboard stub page (placeholder)" checklist item is superseded by the real Dashboard implemented in Phase 18. No stub remains.

---

## Phase 05: Library & Book Data Model
- **Status**: COMPLETE
- **Evidence**:
  - `migrations/002_libraries.up.sql:1-30` — `library`, `library_path`, `library_metadata_source`, `user_library_permission`
  - `migrations/003_books.up.sql:1-75` — `book`, `book_file`, `book_metadata`, `comic_metadata`
  - `migrations/004_taxonomy.up.sql:1-60` — `author`, `series`, `category`, `tag`, `mood` + all junction tables (`book_author`, `book_series`, `book_category`, `book_tag`, `book_mood`)
  - `migrations/005_progress.up.sql:1-21` — `user_book_file_progress`, `reading_sessions`
  - `internal/book/queries.sql` — sqlc queries for book CRUD
  - `internal/library/queries.sql` — sqlc queries for library CRUD
- **Gaps**: None

---

## Phase 06: Library Management API
- **Status**: COMPLETE
- **Evidence**:
  - `internal/library/handler.go:1-624` — `handleList`, `handleCreate`, `handleGet`, `handleUpdate`, `handleDelete`, `handleScan`, `handleListPaths`, `handleAddPath`, `handleRemovePath`
  - `internal/library/service.go:1-271` — `ListForUser` (admin sees all, users see permitted libraries), `GetByID` with access check, `Create`, `Update`, `Delete`, `AddPath`, `RemovePath`, `ListPaths`
  - `internal/library/queries.sql` — sqlc queries
  - `internal/server/routes.go:47-50` — `/api/libraries` mounted with `auth.RequireAuth`, admin-only routes enforced inside handler with `auth.RequireAdmin()`
- **Gaps**:
  - `GET /api/libraries/{id}/metadata-sources` and `PUT /api/libraries/{id}/metadata-sources` (mentioned in plan Section 7.3) are **not implemented**. These belong to Phase 19+ (metadata provider configuration).

---

## Phase 07: File Scanner & Fingerprinting
- **Status**: COMPLETE
- **Evidence**:
  - `internal/library/scanner.go:1-719` — `Scanner` with `ScanLibrary`, `scanBookPerFile`, `scanBookPerFolder`
  - `internal/storage/fingerprint.go:1-69` — `Fingerprint()` reads first 64KB + last 64KB → MD5 hash
  - `internal/library/scanner.go:320-381` — `processFileBookPerFile`: path lookup, fingerprint match, move detection, new book creation
  - `internal/library/scanner.go:384-589` — `processFolderBookPerFolder`: groups files by directory
  - `internal/library/scanner.go:176-185` — `bookTypeForExtension` returns `COMIC` for `.cbz/.cbr/.cb7`, `AUDIOBOOK` for `.m4b/.m4a/.mp3/.opus`
  - `internal/library/scanner.go:192-212` — `bookTypeForFolder`: all-audio → `AUDIOBOOK`, any comic → `COMIC`, else `EBOOK`
  - `internal/library/handler.go:349-461` — `POST /api/libraries/{id}/scan` endpoint (synchronous fallback + async task enqueue)
- **Gaps**: None

---

## Phase 08: Cover Extraction & Thumbnails
- **Status**: COMPLETE
- **Evidence**:
  - `internal/storage/cover.go:1-486` — `ExtractCover` for EPUB (OPF manifest + fallback filenames), PDF (`pdfcpu` first-page image), CBZ (first image alphabetically), CBR/CB7 (`mholt/archives`), audio (`dhowden/tag` embedded artwork)
  - `internal/storage/cover.go:82-122` — `ProcessCover`: decode → 50MP bomb check (`maxMegapixels`) → full-size resize (max 800px) or audiobook square crop (600×600) → thumbnail crop-fit (200×300) → JPEG quality 85
  - `internal/storage/handler.go:48-109` — `GET /api/books/{id}/cover` and `/api/books/{id}/cover/thumbnail`
  - `internal/library/scanner.go:682-710` — `extractAndSaveCover()` called during scan flow
  - Cover directory: `dataDir/covers/books/{bookId}/cover.jpg` and `thumbnail.jpg`
- **Gaps**: None

---

## Phase 09: Embedded Metadata Extraction
- **Status**: COMPLETE
- **Evidence**:
  - `internal/storage/metadata.go:107-222` — `extractEPUBMetadata`: OPF parsing for title, creators (authors), description, publisher, date, language, ISBN, subjects → categories, Calibre `calibre:series` / `calibre:series_index`
  - `internal/storage/metadata.go:226-269` — `extractPDFMetadata`: `pdfcpu` `PDFInfo` for title, author, subject, keywords, creation date
  - `internal/storage/metadata.go:294-394` — `extractCBZMetadata`: `ComicInfo.xml` parsing for full comic metadata (series, volume, writer, publisher, genre, characters, teams, locations, story arc, age rating, etc.)
  - `internal/storage/metadata.go:398-450` — `extractAudioMetadata`: `dhowden/tag` for title, artist, album, genre, year, track number
  - `internal/library/scanner.go:21-143` — `applyMetadata()` creates/updates `book_metadata`, authors, series, categories, tags in a transaction
- **Gaps**: None

---

## Phase 10: Library Browser Frontend
- **Status**: COMPLETE
- **Evidence**:
  - `web/src/features/library/LibraryList.tsx:1-153` — lists user's libraries with icon, name, path count
  - `web/src/features/library/LibraryBrowser.tsx:1-359` — book grid for a library with filter bar (ALL/EBOOK/AUDIOBOOK/COMIC), sort dropdown (title/addedDate ASC/DESC), pagination
  - `web/src/features/book/BookCard.tsx:1-94` — cover image (lazy `loading="lazy"`), title, author badge, book type badge, placeholder for missing covers
  - `web/src/App.tsx:288-289` — routes `/libraries` and `/libraries/:id/books`
- **Gaps**: None

---

## Phase 11: Book Detail Page
- **Status**: PARTIAL
- **Evidence**:
  - `internal/book/handler.go:465-666` — `handleGet` returns full book with metadata, files, authors, series, categories, tags
  - `web/src/features/book/BookDetail.tsx:1-734` — full detail page with large cover, metadata fields (title, subtitle, authors, series, publisher, date, language, ISBN, page count, added date), truncatable description with show more/less, file list with format badges, categories/tags chips, similar books section, shelves chips
  - `web/src/App.tsx:290` — route `/books/:id`
- **Gaps**:
  - **Authors are not clickable links** (`BookDetail.tsx:438` renders `<span>{author.name}</span>` without navigation).
  - **Series is not a clickable link** (`BookDetail.tsx:450` renders plain text `<p class="text-sm font-medium text-indigo-400">{seriesLabel()}</p>` without navigation).
  - `PUT /api/books/{id}/metadata` (plan Section 7.4) is **not implemented** on the book handler. Metadata updates are handled via the metadata provider/proposal system (Phase 19+).

---

## Phase 12: File Watching & WebSocket Events
- **Status**: PARTIAL
- **Evidence**:
  - `internal/ws/hub.go:1-125` — `Hub` with `BroadcastToUser`, `BroadcastToAll`, connection registry
  - `internal/ws/handler.go:1-125` — `/ws` upgrade endpoint with JWT auth via query param `?token=`
  - `internal/library/watcher.go:1-284` — `fsnotify` watcher on all library paths with 5s debounce (`WithDebounce`)
  - `web/src/shared/ws/socket.ts:1-150` — reconnecting WebSocket client with exponential backoff
  - `web/src/shared/ws/WSProvider.tsx` — SolidJS context provider for WebSocket
  - `internal/library/watcher.go:235-253` — broadcasts `LIBRARY_SCAN_COMPLETE` and `BOOK_ADDED` events after scan
  - `web/src/features/library/LibraryBrowser.tsx:188-205` — frontend subscribes to `LIBRARY_SCAN_COMPLETE` and `BOOK_ADDED` to refetch books
- **Gaps**:
  - **`BOOK_UPDATED` and `BOOK_DELETED` WebSocket events are never broadcast**. The plan lists them as checklist items, but only `LIBRARY_SCAN_COMPLETE` and `BOOK_ADDED` are emitted from `internal/library/watcher.go`. There is no code that broadcasts `BOOK_UPDATED` or `BOOK_DELETED`.

---

## Phase 13: Background Task System
- **Status**: COMPLETE
- **Evidence**:
  - `migrations/006_tasks.up.sql:1-25` — `tasks` table (status, progress, total, payload, timestamps) and `task_cron_configuration` table
  - `internal/task/runner.go:1-220` — goroutine-based runner with `context.WithCancel`, progress reporting via `broadcastProgress()`, `MarkInterruptedFailed()`
  - `internal/task/scheduler.go:1-134` — `robfig/cron/v3` integration with default schedules seeded on startup
  - `internal/task/handler.go:1-270` — `GET /api/tasks`, `POST /api/tasks/{type}/run`, `DELETE /api/tasks/{id}` (cancel)
  - `internal/task/types.go:1-50` — task type constants (`TypeLibraryScan`, etc.)
  - `internal/library/handler.go:378-405` — `LIBRARY_SCAN` converted to async background task when `taskEnqueue` is set
  - `internal/task/runner.go:162-169` — `TASK_PROGRESS` WebSocket event broadcast
  - `internal/server/server.go:419-422` — `MarkInterruptedFailed()` called in `Start()`
  - `internal/task/runner.go:46-48` — enforces at most one running instance per task type
  - `internal/task/scheduler.go:18-21` — default cron schedules: `LIBRARY_SCAN` every 6h, `DUPLICATE_DETECTION` Sundays 2am, `RECOMMENDATION_REBUILD` daily 3am, `AUDIT_LOG_CLEANUP` daily 1am
- **Gaps**:
  - Default cron schedules for `METADATA_REFRESH` and `COVER_REFRESH` (marked as "disabled by default" in the plan) are **not seeded** in `defaultCronSchedules`. They are absent entirely rather than seeded as disabled.

---

## Phase 14: EPUB Reader
- **Status**: COMPLETE
- **Evidence**:
  - `internal/reader/handler.go:71-84` — `GET /api/reader/books/{bookId}/files/{fileId}/stream` (range-request streaming), `GET/PUT /api/reader/books/{bookId}/progress`, `GET/PUT /api/reader/books/{bookId}/settings`
  - `web/src/features/reader/EpubReader.tsx:1-900+` — `epubjs` integration, full-screen mode, top bar (title, chapter, progress %), bottom toolbar (font, theme, TOC, bookmarks), CFI-based progress auto-save, settings panel (font family, size, line height, margins, flow, theme)
  - `web/src/features/reader/ReaderDispatch.tsx` — routes EPUB format to `/books/:id/read/epub`
  - `web/src/App.tsx:282` — route `/books/:id/read/epub`
- **Gaps**: None

---

## Phase 15: PDF Reader
- **Status**: COMPLETE
- **Evidence**:
  - `web/src/features/reader/PdfReader.tsx:1-700+` — `pdfjs-dist` integration with worker, page-based progress save/restore, sidebar (thumbnails, TOC, search), settings (spread mode, scroll mode, zoom level)
  - `internal/reader/handler.go:71-84` — progress and settings endpoints shared with EPUB reader
  - `web/src/features/reader/ReaderDispatch.tsx` — routes PDF format to `/books/:id/read/pdf`
  - `web/src/App.tsx:283` — route `/books/:id/read/pdf`
- **Gaps**: None

---

## Phase 16: Shelves (Manual)
- **Status**: COMPLETE
- **Evidence**:
  - `migrations/007_shelves.up.sql:1-22` — `shelf` and `shelf_book` tables with indexes
  - `internal/shelf/queries.sql` — sqlc queries for shelf CRUD
  - `internal/shelf/service.go:1-200+` — business logic for shelves
  - `internal/shelf/handler.go:1-300+` — `GET/POST/PUT/DELETE /api/shelves`, `POST/DELETE /api/shelves/{id}/books`
  - `internal/book/handler.go:69` — `GET /api/books/{id}/shelves` endpoint
  - `web/src/features/shelf/ShelfList.tsx` — user's shelf list
  - `web/src/features/shelf/ShelfDetail.tsx` — books in a shelf
  - `web/src/features/shelf/AddToShelfDialog.tsx` — add-to-shelf dialog
  - `web/src/features/book/BookDetail.tsx:327-330` — "Add to Shelf" button wired up
  - `web/src/App.tsx:295-296` — routes `/shelves` and `/shelves/:id`
- **Gaps**: None

---

## Phase 17: User Management & Permissions
- **Status**: PARTIAL
- **Evidence**:
  - `internal/user/handler.go:67-112` — `AdminRoutes`: `GET/POST/PUT/DELETE /api/admin/users`, `GET/PUT /api/admin/users/{id}/permissions`, `GET/PUT /api/admin/users/{id}/libraries`
  - `internal/user/handler.go:107-110` — permission struct with `role`, `canDownload`, `canUpload`, `canEmailSend`, `canEditMetadata`, `opdsAccess`
  - `internal/server/routes.go:108-118` — `/api/admin/users` requires `auth.RequireAdmin()`; `/api/users/me/...` for self-service
  - `web/src/features/admin/UserManagement.tsx:1-824` — user list, create, edit, permissions, library access
  - `web/src/features/auth/SettingsPage.tsx:1-668` — user self-service settings (theme dark/light/system)
  - `web/src/App.tsx:304,302` — routes `/admin/users` and `/settings`
- **Gaps**:
  - **`canDownload` permission is not enforced** on the file streaming endpoint. `GET /api/reader/books/{bookId}/files/{fileId}/stream` only checks authentication and library access (`hasLibraryAccess`), but never calls `auth.RequirePermission("download")`. The permission flag exists in the data model and JWT claims but is not gated at the HTTP handler level for downloads.

---

## Phase 18: Dashboard
- **Status**: PARTIAL
- **Evidence**:
  - `internal/dashboard/handler.go:1-540+` — `GET /api/dashboard` (returns rows + stats), `GET/PUT /api/dashboard/settings`
  - `internal/dashboard/handler.go:51-53` — row types: `CONTINUE_READING`, `RECENTLY_ADDED`, `RANDOM_PICKS`
  - `internal/dashboard/handler.go:273-316` — `fetchRecentlyAdded`, `fetchInProgress`, `fetchRandom` query implementations
  - `web/src/features/dashboard/Dashboard.tsx` — configurable rows UI with enable/disable/reorder
  - `web/src/features/dashboard/ScrollerRow.tsx` — horizontal book card scroller
  - `web/src/App.tsx:286-287` — route `/` (default landing page after login) renders `Dashboard`
- **Gaps**:
  - **`internal/dashboard/service.go` does not exist**. The plan checklist explicitly marks `[x] internal/dashboard/service.go — row type resolution`. All dashboard business logic is colocated in `handler.go` (`fetchRecentlyAdded`, `fetchInProgress`, `fetchRandom`, `computeStats`). The functionality exists but not in the claimed file path.

---

## Summary Table

| Phase | Status | Key Gaps |
|-------|--------|----------|
| 01 | COMPLETE | App.tsx evolved past "Hello Lexicon" stub |
| 02 | COMPLETE | — |
| 03 | COMPLETE | — |
| 04 | COMPLETE | Dashboard stub superseded by Phase 18 |
| 05 | COMPLETE | — |
| 06 | COMPLETE | Library metadata-source endpoints not implemented (Phase 19+) |
| 07 | COMPLETE | — |
| 08 | COMPLETE | — |
| 09 | COMPLETE | — |
| 10 | COMPLETE | — |
| 11 | PARTIAL | Authors/series not clickable links; `PUT /api/books/{id}/metadata` missing |
| 12 | PARTIAL | `BOOK_UPDATED` and `BOOK_DELETED` WebSocket events never broadcast |
| 13 | COMPLETE | `METADATA_REFRESH`/`COVER_REFRESH` cron defaults not seeded as disabled |
| 14 | COMPLETE | — |
| 15 | COMPLETE | — |
| 16 | COMPLETE | — |
| 17 | PARTIAL | `canDownload` permission not enforced on file streaming |
| 18 | PARTIAL | `internal/dashboard/service.go` missing (logic in handler.go) |

---

## Cross-Cutting Notes

- **Magic Shelves (Phase 21)**: The `magic_shelf` table exists in migration `009_magic_shelves.up.sql`, and the frontend/backend for magic shelves is fully implemented. This was implemented later (Phase 21) as planned, not within phases 01-18.
- **Metadata Providers (Phases 19-20)**: `metadata_fetch_jobs` and `metadata_fetch_proposals` tables (migration `008_metadata_jobs.up.sql`) exist but are outside the 01-18 scope.
- **WebSocket Event Coverage**: Only `LIBRARY_SCAN_COMPLETE` and `BOOK_ADDED` are actively broadcast from `internal/library/watcher.go`. `TASK_PROGRESS` is broadcast from `internal/task/runner.go`. No other event types from the plan's Section 8 are emitted.
- **Route Registration**: All Phase 01-18 routes are registered in `internal/server/routes.go` and `server.go` as documented above.
- **Migrations**: All migrations 001-007 (phases 02, 05, 13, 16) exist with matching `.up.sql` and `.down.sql` files.
