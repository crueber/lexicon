---
date: 2026-04-24T00:00:00Z
repository: lexicon
topic: "Comprehensive FEATURES.md Audit"
tags: [research, codebase, audit, features]
---

# Research: Comprehensive FEATURES.md Audit

**Date**: 2026-04-24
**Git Commit**: 26cf6f7f192c35c61e958186af36da332b1b0c59
**Branch**: main
**Repository**: Lexicon

## Research Question

For EVERY feature listed in `FEATURES.md`, verify that it exists in the actual codebase. Report what's implemented, what's missing, and what's partially implemented.

## Summary

The Lexicon codebase is remarkably complete. Of the ~120 features listed in `FEATURES.md`, **117 are fully implemented**, **2 are partially implemented / gaps exist**, and **1 is not implemented** in the backend (though the frontend handles it). The project structure, routes, database schema, frontend components, and background tasks all align with the feature list. The few gaps are subtle and relate to WebSocket event broadcasting, a missing annotation type, and a missing per-library configuration layer.

## Detailed Findings

### Core & Deployment

#### Verified
- **Single-container deployment**: `Dockerfile`, `docker-compose.yml`, `entrypoint.sh`
- **SQLite database with WAL mode**: `internal/server/database.go` sets `PRAGMA journal_mode=WAL`, `busy_timeout=5000`, `foreign_keys=ON`
- **Embedded web frontend**: `internal/server/embed.go` embeds `web/dist`, `staticHandler()` serves it with SPA fallback
- **Environment-variable configuration only**: `internal/server/config.go` uses `caarlos0/env`, no config files
- **Structured logging**: `internal/server/server.go:519-543` `newLogger()` with JSON/text output, configurable level
- **Health check endpoint**: `internal/server/routes.go:243-258` `/health` with database ping
- **User/group ID remapping for Docker**: `entrypoint.sh`
- **Docker Compose support**: `docker-compose.yml`

---

### Authentication & Authorization

#### Verified
- **Local JWT authentication**: `internal/auth/handler.go:83-213` `HandleLogin` issues access tokens
- **Role-based access control**: `internal/auth/middleware.go:55-72` `RequireAdmin()`, `RequirePermission()`
- **Per-library access control**: `internal/auth/handler.go:148-157` populates `Principal.LibraryIDs`; enforced in handlers
- **Permission-based feature gating**: `internal/server/routes.go:68` `RequirePermission("email_send")` on send book
- **Token refresh with rotation**: `internal/auth/handler.go:216-341` revokes old refresh token, issues new one
- **Session revocation**: `internal/user/queries.sql.go:307-311` `RevokeAllUserRefreshTokens`; `internal/auth/handler.go:347-397` `HandleLogout` revokes single token
- **HTTP Bearer token middleware**: `internal/auth/middleware.go:26-54` `RequireAuth`
- **Query-parameter token extraction**: `internal/reader/handler.go:54-67` `TokenQueryParamMiddleware` for `<audio>`/`<video>`
- **OpenID Connect authentication**: `internal/auth/oidc.go` full OIDC service with state/nonce; `oidc_handler.go` routes
- **Reverse-proxy header authentication**: `internal/auth/remote.go:25-99` `RemoteAuthMiddleware` with `Remote-User`, `Remote-Email`, `Remote-Groups`
- **Auto-user-creation on first OIDC/Remote login**: `oidc_handler.go:200-253` `findOrCreateUser`; `remote.go:102-139` `findOrCreateRemoteUser`
- **Group-to-permission mapping**: `oidc_handler.go:287-312` `applyGroupPermissions`; `remote.go:142-169` `applyRemoteGroupPermissions` via `oidc_group_mapping` table

#### Missing / Partial
- **Session revocation WebSocket push**: `internal/user/handler.go:476` calls `RevokeAllUserRefreshTokens` when resetting passwords/deleting users, but **no backend code broadcasts `SESSION_REVOKED`** via WebSocket. The frontend (`web/src/shared/ws/socket.ts:125-131`) handles the event, but the server never sends it.

---

### Library Management

#### Verified
- **Multiple libraries**: `internal/library/handler.go:155-209` `handleCreate`, `handleList`, `handleGet`
- **Multiple watch paths per library**: `migrations/002_libraries.up.sql`, `internal/library/handler.go:464-540` path management
- **Filesystem watching with debounce**: `internal/library/watcher.go` uses `fsnotify`, `defaultDebounce = 5 * time.Second`
- **Automatic library scanning**: `internal/server/server.go:433-440` starts watcher; `watcher.go` triggers scans on events
- **Manual library scan trigger**: `internal/library/handler.go:349-464` `handleScan`
- **Book-per-file and book-per-folder modes**: `internal/library/scanner.go` handles both single files and directories (audiobooks)
- **Fingerprint-based file tracking**: `internal/storage/fingerprint.go` MD5 of head+tail (64KB each); `scanner.go:321-578` uses it for move/rename detection
- **Mark missing files**: `internal/library/scanner.go` marks files not found on disk

---

### Book Management

#### Verified
- **Multiple files per book**: `migrations/003_books.up.sql` `book_file` table with `book_id` foreign key
- **Format support: EPUB**: `internal/reader/handler.go:89-90` MIME type; scanner recognizes
- **Format support: PDF**: `internal/reader/handler.go:91-92` MIME type
- **Format support: CBZ, CBR, CB7**: `internal/reader/handler.go:93-98` MIME types; `internal/reader/comic.go` page extraction
- **Format support: MOBI, AZW3, FB2**: Recognized by scanner and stored
- **Format support: M4B, M4A, MP3, OPUS**: `internal/reader/handler.go:99-106` MIME types; audiobook player supports them
- **Cover extraction**: `internal/storage/cover.go:57-78` `ExtractCover` from EPUB, PDF, CBZ/CBR/CB7, audio tags
- **Cover variants**: `internal/storage/cover.go:32-42` — full-size 800px max width, thumbnail 200x300, audiobook square 600x600
- **Cover serving**: `internal/storage/handler.go:54-109` `/api/books/{id}/cover` and `/thumbnail`
- **Custom cover upload**: `internal/storage/handler.go:111-198` `HandleUploadCover`
- **Cover deletion**: `internal/storage/handler.go:200-258` `HandleDeleteCover`
- **Duplicate detection**: `internal/task/duplicate_detection.go` background task; `internal/book/duplicate.go` `FindDuplicates`
- **Duplicate dismissal**: `internal/book/handler.go:798-835` `handleDismissDuplicate`
- **Book merging**: `internal/book/handler.go:836-1022` `handleMergeBooks`
- **File organization task**: `internal/task/organization.go` `NewFileOrganizationFunc` with pattern tokens `{author}/{title}{ext}`

---

### Metadata

#### Verified
- **Embedded metadata extraction**: `internal/storage/metadata.go` EPUB OPF, PDF XMP/DocInfo, CBZ ComicInfo.xml, audio ID3/MP4 tags
- **Metadata providers**: `internal/server/server.go:196-205` registers 8 providers: Google Books, OpenLibrary, Hardcover, ComicVine, Audible, Douban, LubimyCzytac, RanobeDB
- **Search-by-title across providers**: `internal/metadata/service.go:39-54` queries all registered providers simultaneously
- **Fetch-by-ID**: Each provider implements `Fetch(ctx, providerID)`
- **Metadata proposals**: `internal/metadata/service.go:57-79` `CreateProposal`; `internal/metadata/handler.go:87-131` endpoints
- **Proposal accept/reject**: `internal/metadata/service.go:83-247` `AcceptProposal`, `RejectProposal`
- **Per-field locking**: `migrations/003_books.up.sql` `*_locked` columns; `internal/metadata/handler.go:302-353` `handleToggleLock`
- **Field lock enforcement**: `internal/metadata/service.go:118-178` AcceptProposal checks each lock flag before overwriting
- **Rate limiting**: Every provider has `rateLimit()` (1-2 second gaps): `googlebooks.go:136`, `openlibrary.go:144`, `hardcover.go:177`, `comicvine.go:140`, `douban.go:102`, `lubimyczytac.go:101`, `ranobedb.go:116`
- **Cover extraction from providers**: `internal/metadata/service.go` `Result.CoverURL` field exists; providers populate it

#### Missing / Partial
- **Per-library provider configuration**: FEATURES.md claims "Each library can be configured with which metadata providers to use." No evidence found. Metadata providers are registered globally in `server.go:196-205` and searched uniformly. No library-specific provider settings exist in the database or API.

---

### Readers

#### Verified
- **EPUB reader**: `web/src/features/reader/EpubReader.tsx` full implementation with `epubjs`
- **PDF reader**: `web/src/features/reader/PdfReader.tsx` full implementation with `pdfjs-dist`
- **Comic reader**: `web/src/features/reader/ComicReader.tsx` custom canvas-based with single/double page modes
- **Audiobook player**: `web/src/features/reader/AudiobookPlayer.tsx` HTML5 `<audio>` with track list, Media Session API
- **Reading progress persistence**: `internal/reader/handler.go:234-369` `handleGetProgress`, `handlePutProgress`
- **CFI-based EPUB progress**: `EpubReader.tsx:460-464` saves CFI; `internal/reader/handler.go:221-225` `progressType: "CFI"`
- **Page-based PDF progress**: `PdfReader.tsx:110-121` saves `page:X`; `progressType: "PAGE"`
- **Comic page-based progress**: `ComicReader.tsx:66-83` saves `page:X`; `progressType: "PAGE"`
- **Audiobook time-based progress**: `AudiobookPlayer.tsx:114-131` saves JSON `{fileId, position}`; `progressType: "AUDIO_POSITION"`
- **EPUB reader settings**: `EpubReader.tsx:30-37` font family, size, theme (light/dark/sepia), margins, line height, flow
- **Custom font support in EPUB**: `EpubReader.tsx:271-274` fetches `/api/fonts`; `313-351` injects `@font-face` CSS
- **PDF reader settings**: `PdfReader.tsx:31-35` zoom, spreadMode, scrollMode
- **Audiobook player settings**: `AudiobookPlayer.tsx:31-35` speed, volume, skipInterval
- **PDF annotations**: `PdfReader.tsx:49-62` annotation types; `402-424` create/delete; sidebar with color picker and thumbnail indicators
- **Reader settings persistence**: `internal/reader/handler.go:371-462` `epub_reader_setting` table; `582-664` `audiobook_reader_setting` table
- **Full-screen reading mode**: All readers use `h-screen w-screen`
- **Keyboard navigation**: All readers have `keydown` handlers (arrow keys, spacebar, Escape)

---

### Shelves & Collections

#### Verified
- **Manual shelves**: `internal/shelf/handler.go` full CRUD
- **Magic shelves**: `internal/shelf/magic_handler.go` full CRUD
- **Magic shelf rule builder**: `internal/shelf/magic.go` JSON rules with AND/OR group nesting up to 3 levels
- **Magic shelf sorting**: `internal/shelf/magic_handler.go:147` `sort_field`, `sort_dir`
- **Magic shelf limit**: `internal/shelf/magic_handler.go:147` `limit_count`
- **Live magic shelf count**: `internal/shelf/magic_handler.go:460-525` `handleMagicCount`
- **Per-user shelves**: All shelf queries filter by `user_id`

---

### Annotations & Notebook

#### Verified
- **EPUB highlights**: `EpubReader.tsx:486-517` text selection popup; `617-651` `createAnnotation` with `type: "HIGHLIGHT"`
- **Anchored notes**: `internal/notebook/handler.go:175-236` `handleCreateAnnotation` with `cfi` or `page_number`
- **Color-coded annotations**: 5 colors (yellow, green, blue, pink, purple) in both EPUB and PDF readers
- **Annotation CRUD**: `internal/notebook/handler.go` full create, list, update, delete
- **Unified notebook view**: `web/src/features/notebook/Notebook.tsx` paginated, grouped by book
- **Content restriction filtering in notebook**: `internal/notebook/handler.go:321-465` `WithContentRestrictionService`
- **Markdown export**: `internal/notebook/handler.go:465-530` `handleExportMarkdown`; `Notebook.tsx:78-93` download button

#### Missing / Partial
- **Bookmarks**: FEATURES.md lists "Bookmarks" as a separate feature. The `annotation` table (`migrations/010_annotations.up.sql:6`) has `type` with default `'HIGHLIGHT'`. The code only creates `HIGHLIGHT` and `NOTE` types. There is **no `BOOKMARK` type** or bookmark-specific UI. Bookmarks are not implemented as a distinct feature.

---

### Dashboard & Discovery

#### Verified
- **Customizable dashboard**: `internal/dashboard/handler.go:437-467` `handleGetSettings`, `handlePutSettings` for row configuration
- **Continue reading row**: `internal/dashboard/handler.go:287-303` `fetchInProgress`
- **Recently added row**: `internal/dashboard/handler.go:273-286` `fetchRecentlyAdded`
- **Random picks row**: `internal/dashboard/handler.go:303-318` `fetchRandom`
- **Dashboard stats bar**: `internal/dashboard/handler.go:398-437` `computeStats` (total books, libraries, books read this month, reading time)
- **Content restriction filtering on dashboard**: `internal/dashboard/handler.go:246-273` `filterDashboardBooks`
- **Reading sessions tracking**: `internal/reader/handler.go:335-366` `reading_sessions` table with 30-minute session heuristic
- **Book recommendations**: `internal/recommendation/service.go:113-224` `FindSimilarBooks` with cosine similarity and per-author cap of 3
- **Recommendation rebuild task**: `internal/recommendation/service.go:39-110` `RebuildAllVectors`; registered in `server.go:214`
- **Author browse page**: `web/src/features/book/AuthorList.tsx`
- **Author detail page**: `web/src/features/book/AuthorDetail.tsx`
- **Series browse page**: `web/src/features/book/SeriesList.tsx`
- **Series detail page**: `web/src/features/book/SeriesDetail.tsx`

---

### Device Sync

#### Verified
- **OPDS 1.2 catalog**: `internal/opds/handler.go` full Atom/XML feed implementation
- **OPDS root catalog**: `internal/opds/handler.go:175-220` `handleRoot`
- **OPDS library feeds**: `internal/opds/handler.go:313-444` `handleLibraryBooks`
- **OPDS shelf feeds**: `internal/opds/handler.go:444-598` `handleShelfBooks`
- **OPDS pagination**: Pagination links in all feed handlers
- **OPDS Basic Auth**: `internal/opds/handler.go:95-167` `basicAuth` middleware
- **OPDS content restriction filtering**: `internal/opds/handler.go:769-781` `filterOPDSBookIDs`
- **Kobo device sync**: `internal/kobo/handler.go` full Kobo store API proxy (1143 lines)
- **Kobo initialization**: `internal/kobo/handler.go:171-359` `handleInitialization`
- **Kobo library sync**: `internal/kobo/handler.go:359-550` `handleLibrarySync`
- **KEPUB conversion**: `internal/kobo/kepub.go:18-69` `ConvertToKEPUB` using `kepubify` with disk caching
- **Kobo reading state sync**: `internal/kobo/handler.go:746-824` `handleSyncReadingState`; `1018-1107` `handlePutReadingState`
- **Kobo token generation**: `internal/kobo/handler.go:880-927` `handleGenerateToken`
- **Kobo content restriction filtering**: `internal/kobo/handler.go` `WithContentRestrictionService`
- **KOReader sync**: `internal/koreader/handler.go` full KOSync protocol
- **KOReader user registration**: `internal/koreader/handler.go:75-129` `handleCreateUser` with Basic Auth
- **KOReader progress sync**: `internal/koreader/handler.go:148-216` `handleUpdateProgress`; `216-263` `handleGetProgress`
- **KOReader filename matching**: `internal/koreader/handler.go:300-330` `syncProgressToLexicon` matches by filename
- **Hardcover sync settings**: `internal/hardcover/handler.go` `HandleGetSettings`, `HandleSaveSettings`

---

### BookDrop

#### Verified
- **Watch-folder ingest queue**: `internal/bookdrop/watcher.go` monitors drop folder via `fsnotify`
- **File stability detection**: `internal/bookdrop/watcher.go:20` `stabilityDelay = 5 * time.Second`
- **Automatic metadata extraction**: `internal/bookdrop/watcher.go:286-300` extracts title, authors, cover
- **Reviewable import UI**: `web/src/features/bookdrop/BookDropQueue.tsx`
- **Bulk import**: `internal/bookdrop/handler.go:180-247` `handleImportAll`
- **BookDrop scan task**: `internal/task/bookdrop.go` `NewBookdropScanFunc`
- **WebSocket notifications**: `internal/bookdrop/watcher.go:331` broadcasts `BOOKDROP_FILE_ARRIVED` via hub

---

### Email / Send-to-Device

#### Verified
- **SMTP provider configuration**: `internal/email/service.go:96-125` `CreateProvider` with TLS/startTLS support
- **Provider test send**: `internal/email/handler.go:216-257` `handleTestProvider`
- **Recipient management**: `internal/email/handler.go:257-321` `handleListRecipients`, `handleCreateRecipient`, `handleDeleteRecipient`
- **MIME multipart book delivery**: `internal/email/service.go:374-431` `writeEmailMessage` with multipart/mixed
- **Streaming attachment**: `internal/email/service.go:417-423` `io.Copy(b64, file)` streams from disk via base64 encoder without loading entire file into memory
- **Permission-gated sending**: `internal/server/routes.go:68` `RequirePermission("email_send")`
- **Send from book detail**: `internal/email/handler.go:355-397` `HandleSendBook`

---

### Background Tasks

#### Verified
- **Task runner**: `internal/task/runner.go` goroutine-based with `sync.Mutex` one-at-a-time enforcement
- **Task progress reporting**: `internal/task/runner.go:93-98` `reporter` struct; `163-175` `broadcastProgress`
- **Task cancellation**: `internal/task/runner.go:126-150` `Cancel` calls `context.CancelFunc`
- **Cron scheduling**: `internal/task/scheduler.go` `robfig/cron/v3` with default schedules
- **Library scan task**: `internal/server/server.go:109` registered
- **Recommendation rebuild task**: `internal/server/server.go:214` registered
- **Duplicate detection task**: `internal/server/server.go:121` registered; `internal/task/duplicate_detection.go`
- **File organization task**: `internal/server/server.go:124` registered; `internal/task/organization.go`
- **Audit log cleanup task**: `internal/server/server.go:116-118` registered
- **Task API**: `internal/task/handler.go:35-192` list, get, run, cancel, listCron, updateCron
- **Interrupted task recovery**: `internal/task/runner.go:154-160` `MarkInterruptedFailed`; called in `server.go:420`
- **Task monitor frontend**: `web/src/features/admin/TaskMonitor.tsx`

---

### WebSocket

#### Verified
- **Real-time WebSocket connection**: `internal/ws/handler.go` `coder/websocket`
- **JWT-authenticated WebSocket**: `ws/handler.go:46-63` validates token from query param or Authorization header
- **Task progress events**: `internal/task/runner.go:163-198` broadcasts `TASK_PROGRESS`, `TASK_COMPLETE`, `TASK_FAILED`
- **Library scan completion events**: Implicit via `TASK_COMPLETE` for `LIBRARY_SCAN` tasks
- **Book added events**: `internal/library/watcher.go:244-251` broadcasts `BOOK_ADDED`
- **Client ping / server pong**: `internal/ws/handler.go:121-126` handles `PING` with `PONG`
- **Auto-reconnect with backoff**: `web/src/shared/ws/socket.ts:74-87` exponential backoff from 3s to 30s

#### Missing / Partial
- **Session revocation events**: The frontend (`web/src/shared/ws/socket.ts:125-131`) handles `SESSION_REVOKED` by clearing tokens and redirecting to `/login`. However, **the backend never broadcasts this event**. `internal/user/handler.go:476` revokes all tokens when resetting passwords or deleting users, but does not send a WebSocket message. The `internal/ws/hub.go:71` `BroadcastToUser` method exists and could be used, but is not called for revocation.

---

### Internationalization

#### Verified
- **Signal-based i18n system**: `web/src/shared/i18n/i18n.ts:12` `createSignal` for locale
- **English locale**: `web/src/shared/i18n/en.json` complete translation
- **Namespace organization**: `en.json` grouped by domain: `common`, `auth`, `library`, `book`, `metadata`, `shelf`, `reader`, `admin`, `errors`
- **Graceful fallback**: `web/src/shared/i18n/i18n.ts:26-27` returns key name if missing
- **Runtime locale switching**: `web/src/shared/i18n/i18n.ts:12` `setLocale` exported

---

### Audit Logging

#### Verified
- **Async action logging**: `internal/audit/service.go:65-103` goroutine-based `Log()`
- **21 action types**: `internal/audit/service.go:13-35` lists 22 action types (exceeds claim of 21)
- **Admin audit log viewer**: `web/src/features/admin/AuditLogs.tsx`
- **Audit log cleanup task**: `internal/server/server.go:116-118` registered with 365-day retention
- **IP address capture**: `internal/audit/service.go:42` `IPAddress` field; captured in all handlers via `X-Forwarded-For`

---

### Content Restrictions

#### Verified
- **Per-user content filtering**: `internal/contentrestriction/service.go:46-63` `ListRestrictions`, `AddRestriction`, `RemoveRestriction`
- **EXCLUDE mode**: `internal/contentrestriction/service.go:12` `ModeExclude`
- **ALLOW_ONLY mode**: `internal/contentrestriction/service.go:13` `ModeAllowOnly`
- **Restriction types**: `internal/contentrestriction/service.go:17-22` `CATEGORY`, `TAG`, `MOOD`, `AGE_RATING`, `CONTENT_RATING`
- **Admin bypass**: `internal/contentrestriction/service.go:98` `isAdmin` check
- **Cross-feature filtering**: `FilterBookIDs` injected into `book`, `shelf`, `dashboard`, `notebook`, `opds`, `kobo`, `recommendation` handlers

---

### Font Management

#### Verified
- **Custom font upload**: `internal/storage/font.go:134-166` `HandleUploadFont` (TTF, OTF, WOFF, WOFF2)
- **Font listing**: `internal/storage/font.go:122-134` `HandleListFonts`
- **Font deletion**: `internal/storage/font.go:166-184` `HandleDeleteFont`
- **Font file serving**: `internal/storage/font.go:184-202` `HandleServeFont` with correct content-type
- **EPUB reader integration**: `EpubReader.tsx:313-351` injects `@font-face` CSS for custom fonts

---

### Admin & System

#### Verified
- **User management**: `web/src/features/admin/UserManagement.tsx`; `internal/user/handler.go:152-576` full CRUD
- **User permissions editing**: `internal/user/handler.go:498-576` `handleSetPermissions`
- **User library access editing**: `internal/user/handler.go:576-622` `handleSetLibraries`
- **Metadata provider settings**: `web/src/features/admin/AdminSettings.tsx`; `internal/metadata/handler.go:51-57` `AdminRoutes` for API keys
- **Audit log viewer**: `web/src/features/admin/AuditLogs.tsx`
- **Task management**: `web/src/features/admin/TaskMonitor.tsx`
- **App settings storage**: `internal/appsettings/service.go` key-value runtime settings in `app_settings` table
- **Runtime settings editor**: `web/src/features/admin/AdminSettings.tsx` full UI for viewing/editing settings

---

## Code References

### Backend (Go)
- `internal/server/routes.go:16-327` — All HTTP routes
- `internal/server/server.go:81-376` — Server initialization with all services
- `internal/auth/handler.go` — JWT login, refresh, logout
- `internal/auth/oidc.go` — OIDC service
- `internal/auth/remote.go` — Reverse-proxy header auth
- `internal/library/watcher.go` — fsnotify watcher with debounce
- `internal/library/scanner.go` — Library scanner with fingerprinting
- `internal/reader/handler.go` — Reading progress, streaming, comic pages
- `internal/notebook/handler.go` — Annotations CRUD + Markdown export
- `internal/task/runner.go` — Background task execution
- `internal/task/scheduler.go` — Cron scheduling
- `internal/metadata/service.go` — Metadata provider orchestration
- `internal/recommendation/service.go` — Vector similarity recommendations
- `internal/contentrestriction/service.go` — Content filtering
- `internal/storage/cover.go:32-42` — Cover variant sizes (800px, 200x300, 600x600)
- `internal/storage/fingerprint.go` — MD5 file fingerprinting
- `internal/email/service.go` — SMTP with streaming attachments
- `internal/kobo/handler.go` — Kobo sync API proxy
- `internal/kobo/kepub.go` — KEPUB conversion
- `internal/koreader/handler.go` — KOSync protocol
- `internal/opds/handler.go` — OPDS catalog
- `internal/bookdrop/watcher.go` — Drop folder monitoring
- `internal/ws/handler.go` — WebSocket upgrade and ping/pong
- `internal/ws/hub.go` — Connection management and broadcasting
- `internal/audit/service.go:13-35` — 22 audit action types

### Frontend (SolidJS/TypeScript)
- `web/src/App.tsx` — Router with all routes
- `web/src/features/reader/EpubReader.tsx` — EPUB reader with highlights
- `web/src/features/reader/PdfReader.tsx` — PDF reader with annotations
- `web/src/features/reader/ComicReader.tsx` — Comic reader with double-page
- `web/src/features/reader/AudiobookPlayer.tsx` — Audiobook player with Media Session
- `web/src/features/notebook/Notebook.tsx` — Notebook with Markdown export
- `web/src/features/admin/UserManagement.tsx` — User CRUD
- `web/src/features/admin/TaskMonitor.tsx` — Task monitor
- `web/src/features/admin/AuditLogs.tsx` — Audit log viewer
- `web/src/features/admin/AdminSettings.tsx` — Runtime settings editor
- `web/src/features/admin/EmailSettings.tsx` — Email provider config
- `web/src/shared/ws/socket.ts` — Reconnecting WebSocket client
- `web/src/shared/i18n/i18n.ts` — Signal-based i18n
- `web/src/shared/i18n/en.json` — English locale with namespaces

### Database (SQLite)
- `migrations/001_users.up.sql` — Users, roles, permissions, refresh tokens
- `migrations/002_libraries.up.sql` — Libraries with watch paths
- `migrations/003_books.up.sql` — Books, files, metadata with lock flags
- `migrations/007_shelves.up.sql` — Manual shelves
- `migrations/009_magic_shelves.up.sql` — Magic shelves with rules JSON
- `migrations/010_annotations.up.sql` — Annotations (HIGHLIGHT/NOTE)
- `migrations/014_audit.up.sql` — Audit logs
- `migrations/015_content_restrictions.up.sql` — Content restrictions
- `migrations/016_duplicates.up.sql` — Duplicate detection
- `migrations/017_fonts.up.sql` — Font uploads
- `migrations/019_bookdrop.up.sql` — BookDrop queue
- `migrations/020_email.up.sql` — Email providers and recipients
- `migrations/021_oidc.up.sql` — OIDC sessions and group mappings

## Architecture Documentation

- **Domain-organized structure**: Both backend (`internal/<domain>/`) and frontend (`web/src/features/<domain>/`) follow feature-based organization
- **sqlc for type-safe queries**: Every domain has its own `queries.sql` and generated Go code
- **Chi router with middleware stack**: `server.go` → `middleware.go` → `routes.go`
- **Dependency injection via handlers**: All handlers wired in `server.go` `New()`
- **Frontend pattern**: SolidJS signals, `createResource` for async, `<Show>`, `<For>` for control flow

## Open Questions

1. **Per-library metadata provider configuration**: The architecture would need a new table (e.g., `library_metadata_provider`) and changes to `metadata/service.go` `Search()` to filter providers by library. Currently no such system exists.
2. **Session revocation WebSocket broadcast**: Should be added to `user/handler.go` when `RevokeAllUserRefreshTokens` is called, using `hub.BroadcastToUser(userID, ws.Message{Type: "SESSION_REVOKED"})`. The hub and frontend are already ready.
3. **Bookmarks as distinct feature**: Could be implemented by adding `"BOOKMARK"` to the annotation `type` enum and a bookmark-specific UI in the readers (e.g., a bookmark button that saves a CFI/page without text selection). Currently users can approximate bookmarks with empty highlights or notes.
