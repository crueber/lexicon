---
date: 2026-04-23T12:00:00Z
repository: github.com/crueber/lexicon
topic: "Audit of Sections 8-15 of Implementation Plan"
tags: [research, audit, websocket, storage, library, metadata, reader, shelves, sync, tasks]
---

# Research: Audit of Lexicon Implementation Plan Sections 8-15

**Date**: 2026-04-23
**Git Commit**: `5ea736039514c03a7d22f8699ead537d31f945fd`
**Branch**: `main`
**Repository**: github.com/crueber/lexicon

## Research Question

Audit the Lexicon codebase against Sections 8-15 of the implementation plan (`plans/2026-03-23-lexicon-implementation-plan.md`). For each requirement, feature, table, endpoint, or design decision, check whether it exists in the actual codebase.

## Summary

The codebase has substantial implementation across all eight audited sections, with the core architecture fully in place. Most critical features are implemented, but there are notable gaps in:

1. **WebSocket events**: Several planned event types are never broadcast.
2. **Metadata providers**: No per-field priority matrix; Amazon/GoodReads replaced by OpenLibrary.
3. **Magic shelves**: Far fewer rule fields (11 vs. 50+) and operators (8 vs. 19) than specified.
4. **Device sync**: OPDS lacks OpenSearch and separate user tables; Kobo lacks snapshot-based sync; KOReader uses filename matching instead of MD5.
5. **Background tasks**: Three task types have no registered implementations.
6. **Annotations**: Unified `annotation` table replaces the planned separate `book_marks`, `book_notes`, and `pdf_annotations` tables.

---

## Detailed Findings

### Section 8: WebSocket Events

#### Implemented ✅
- **Endpoint `/ws`**: JWT auth via query param `?token=` or `Authorization` header (`internal/ws/handler.go:46-63`)
- **JSON message structure** with `type` and `payload` (`internal/ws/hub.go:11-14`)
- **Hub with user-scoped and broadcast-to-all** (`internal/ws/hub.go:25-116`)
- **Client → Server PING / Server PONG** (`internal/ws/handler.go:121-126`, `web/src/shared/ws/socket.ts:60-64`)
- **TASK_PROGRESS, TASK_COMPLETE, TASK_FAILED** broadcast from task runner (`internal/task/runner.go:163-198`)
- **LIBRARY_SCAN_COMPLETE, BOOK_ADDED** broadcast from filesystem watcher (`internal/library/watcher.go:235-253`)
- **SESSION_REVOKED** handled in frontend (clears tokens, redirects to `/login`) (`web/src/shared/ws/socket.ts:125-131`)
- **Reconnecting WebSocket client** with exponential backoff (`web/src/shared/ws/socket.ts`)

#### Missing / Partial ⚠️
- **BOOK_UPDATED**: Not broadcast anywhere in backend.
- **BOOK_DELETED**: Not broadcast anywhere in backend.
- **BOOKDROP_FILE_ARRIVED**: Not broadcast (BookDrop system does not exist).
- **METADATA_PROPOSAL_READY**: Not broadcast anywhere.
- **NOTIFICATION**: No general notification broadcast mechanism exists.

---

### Section 9: File Storage & Cover Management

#### Implemented ✅
- **Cover extraction per format**:
  - EPUB: unzip + OPF manifest + fallback filenames (`internal/storage/cover.go:138-169`)
  - PDF: first image from first page via `pdfcpu` (`internal/storage/cover.go:295-325`)
  - CBZ: first image alphabetically (`internal/storage/cover.go:328-361`)
  - CBR/CB7: via `mholt/archives`, first image alphabetically (`internal/storage/cover.go:365-421`)
  - M4B/M4A/MP3: embedded artwork via `dhowden/tag` (`internal/storage/cover.go:424-443`)
- **Decompression bomb protection**: reject decoded images > 50 MP (`internal/storage/cover.go:30,89-92`)
- **Full-size cover**: max 800px wide (`internal/storage/cover.go:33,106`)
- **Thumbnail**: 200x300 crop-fit (`internal/storage/cover.go:39-42,115`)
- **Audiobook square crop**: 600x600 (`internal/storage/cover.go:36,101-103`)
- **JPEG quality 85** (`internal/storage/cover.go:45,132`)
- **File fingerprinting**: first 64KB + last 64KB → MD5 (`internal/storage/fingerprint.go:12-69`)
- **Cover serving**: `GET /api/books/{id}/cover` and `/thumbnail` (`internal/storage/handler.go:38-41`)
- **Font management**: upload, serve, delete (`internal/storage/font.go`)
- **KEPUB cache directory**: used by `internal/kobo/kepub.go:18-20`

#### Missing / Partial ⚠️
- **MOBI/AZW3/FB2/OPUS cover extraction**: returns `nil` (not implemented) (`internal/storage/cover.go:74-75`)
- **`covers/authors/{authorId}.jpg`**: Not implemented.
- **`covers/bookdrop/{bookdropFileId}.jpg`**: Not implemented (BookDrop does not exist).
- **`tools/kepubify` and `tools/ffprobe` downloaded at startup**: Not implemented.
- **`cache/comics/{fileId}/` pre-extraction**: Comic pages are extracted on-the-fly; no disk cache.
- **File organization**: Token-based naming exists (`internal/task/organization.go`) but is simplified; not all tokens/modifiers from the plan are implemented.

#### Not in Plan but Implemented 📝
- **Font management system** (`internal/storage/font.go`, `migrations/017_fonts.up.sql`)
- **Content restriction filtering** integrated into cover/book queries (`internal/contentrestriction/`)

---

### Section 10: Library & Book Management

#### Implemented ✅
- **Library scanner** with `BOOK_PER_FILE` and `BOOK_PER_FOLDER` modes (`internal/library/scanner.go:240-281`)
- **Fingerprint-based file move detection** (`internal/library/scanner.go:356-372`)
- **Audiobook detection**: folder with all audio files → `AUDIOBOOK` (`internal/library/scanner.go:192-212`)
- **Comic detection**: CBZ/CBR/CB7 → `COMIC` (`internal/library/scanner.go:169-185`)
- **fsnotify watching** with 5-second debounce (`internal/library/watcher.go:18,154-197`)
- **Embedded metadata extraction**:
  - EPUB OPF: title, authors, description, publisher, date, language, ISBN, series (`internal/storage/metadata.go:107-222`)
  - PDF XMP/DocInfo: title, author, subject, keywords, creation date (`internal/storage/metadata.go:226-269`)
  - CBZ ComicInfo.xml: full comic metadata (`internal/storage/metadata.go:294-394`)
  - Audio tags (ID3/MP4): title, artist, album, genre, track, year (`internal/storage/metadata.go:398-450`)

#### Missing / Partial ⚠️
- **Mark missing book_files**: Scanner does not mark files not found on disk as missing; no "missing" status exists.
- **MOBI/AZW3/FB2 metadata extraction**: Not implemented.
- **Audio duration extraction**: `duration_secs` is not populated during scan.
- **Re-scan affected directory on fsnotify**: The watcher triggers a full library scan, not just the affected directory.

---

### Section 11: Metadata Providers

#### Implemented ✅
- **Provider interface**: `Name()`, `Search()`, `FetchByID()` (`internal/metadata/types.go:6-10`)
- **Google Books** provider with API key support (`internal/metadata/googlebooks.go`)
- **Hardcover** GraphQL provider (`internal/metadata/hardcover.go`)
- **OpenLibrary** provider (replaced Amazon/GoodReads per Phase 20) (`internal/metadata/openlibrary.go`)
- **ComicVine** provider (`internal/metadata/comicvine.go`)
- **Douban** provider (`internal/metadata/douban.go`)
- **LubimyCzytac** provider (`internal/metadata/lubimyczytac.go`)
- **RanobeDB** provider (`internal/metadata/ranobedb.go`)
- **Audible** stub provider (`internal/metadata/audible.go`)
- **Proposal system**: create, accept, reject, with field lock enforcement (`internal/metadata/service.go:57-247`, `internal/metadata/handler.go`)
- **Field lock flags**: 10 lockable fields in `book_metadata` table (`migrations/003_books.up.sql`)

#### Missing / Partial ⚠️
- **Amazon provider**: Replaced by OpenLibrary (documented in Phase 20).
- **GoodReads provider**: Replaced by OpenLibrary (documented in Phase 20).
- **`SupportedFields()` method**: Not present on `Provider` interface.
- **Per-field priority matrix**: No merging logic across providers; `Search()` returns separate provider results without field-level merging (`internal/metadata/service.go:39-54`).
- **`library_metadata_source` configuration**: Table exists (`migrations/002_libraries.up.sql`) but no per-library provider priority configuration UI or API.

---

### Section 12: Reader Support

#### Implemented ✅
- **PDF Reader frontend** (`web/src/features/reader/PdfReader.tsx`)
- **EPUB Reader frontend** (`web/src/features/reader/EpubReader.tsx`)
- **Comic Reader frontend** (`web/src/features/reader/ComicReader.tsx`)
- **Audiobook Player frontend** (`web/src/features/reader/AudiobookPlayer.tsx`)
- **Reader dispatch routing** (`web/src/features/reader/ReaderDispatch.tsx`)
- **Streaming endpoint with Range request support** (`internal/reader/handler.go:130-218`)
- **Comic page endpoints**: `GET /api/reader/books/{bookId}/files/{fileId}/pages` and `.../pages/{pageIndex}` (`internal/reader/handler.go:464-578`, `internal/reader/comic.go`)
- **Progress GET/PUT** (`internal/reader/handler.go:234-369`)
- **EPUB reader settings** (`internal/reader/handler.go:371-462`)
- **Audiobook reader settings** (`internal/reader/handler.go:580-664`)
- **Reading sessions tracking** (30-minute session heuristic) (`internal/reader/handler.go:336-366`)
- **Annotation API**: create, list, update, delete (`internal/notebook/handler.go`)
- **Token query param middleware** for `<audio>` elements (`internal/reader/handler.go:54-67`)

#### Missing / Partial ⚠️
- **`pdf_annotations` table**: Defined in plan Section 5.4 but not in migrations; no PDF annotation endpoints.
- **`book_marks` and `book_notes` tables**: Defined in plan Section 5.4 but not in migrations. The unified `annotation` table is used instead.
- **Audiobook chapters from M4B via ffprobe**: Not implemented. Playback is track-based only.
- **PDF annotation endpoints**: Not implemented.
- **Separate bookmark/note endpoints**: Not implemented; unified annotation API used.

#### Not in Plan but Implemented 📝
- **Unified `annotation` table** with `type`, `cfi`, `page_number`, `text`, `note`, `color` fields replaces separate tables (`migrations/010_annotations.up.sql`)
- **Notebook view** with pagination and content restriction filtering (`internal/notebook/handler.go:316-459`)

---

### Section 13: Shelves & Magic Shelves

#### Implemented ✅
- **Manual shelves**: full CRUD, add/remove books, list books (`internal/shelf/handler.go`)
- **Magic shelves**: full CRUD, evaluate rules, live count (`internal/shelf/magic_handler.go`)
- **Rule evaluation engine**: SQL WHERE clause builder (`internal/shelf/magic.go`)
- **AND/OR group nesting** (up to 3 levels supported via recursion) (`internal/shelf/magic.go:124-168`)
- **Content restriction filtering** on shelf and magic shelf results (`internal/shelf/handler.go:351-374`, `internal/shelf/magic_handler.go:412-434`)
- **Magic shelf schema**: `magic_shelf` table with `rules`, `sort_field`, `sort_dir`, `limit_count` (`migrations/009_magic_shelves.up.sql`)

#### Missing / Partial ⚠️
- **Rule fields**: Only 11 fields implemented (`internal/shelf/magic.go:42-54`):
  `title`, `author`, `category`, `tag`, `series`, `language`, `book_type`, `format`, `added_date`, `page_count`, `publisher`
  - Plan specifies 50+ fields including: `mood`, `isbn10`, `isbn13`, `publish_year`, `last_read_date`, `progress_percent`, `read_status`, `has_cover`, `has_description`, `has_series`, `file_size`, `duration`, `rating`, `content_rating`, `age_rating`, `character`, `team`, `location`, `story_arc`, `community_rating`, `google_books_id`, `amazon_id`, `goodreads_id`, `hardcover_id`, etc.
- **Rule operators**: Only 8 operators implemented (`internal/shelf/magic.go:58-67`):
  `contains`, `equals`, `starts_with`, `ends_with`, `greater_than`, `less_than`, `is_empty`, `is_not_empty`
  - Plan specifies 19 operators including: `NOT_EQUALS`, `NOT_CONTAINS`, `GREATER_THAN_OR_EQUAL`, `LESS_THAN_OR_EQUAL`, `IS_NULL`, `IS_NOT_NULL`, `IN`, `NOT_IN`, `BEFORE`, `AFTER`, `BETWEEN`, `REGEX`, `MATCHES`

---

### Section 14: Device Sync Integrations

#### Implemented ✅
- **OPDS Atom XML feeds** with HTTP Basic Auth (`internal/opds/handler.go`)
- **OPDS root catalog**, libraries feed, library books, shelves feed, shelf books (`internal/opds/handler.go:66-76`)
- **OPDS acquisition links** for all available formats (`internal/opds/handler.go:685-695`)
- **OPDS pagination** (`internal/opds/handler.go:768-788`)
- **Content restriction filtering** in OPDS (`internal/opds/handler.go:739-748`)
- **Kobo store API proxy** (`internal/kobo/handler.go`)
- **Kobo endpoints**: initialization, library sync, metadata, download, reading-state sync, user profile, wishlist, prices, recommendations (`internal/kobo/handler.go:72-84`)
- **KEPUB conversion** with caching (`internal/kobo/kepub.go`)
- **Kobo token generation** via `POST /api/kobo/token` (`internal/kobo/handler.go:852-896`)
- **KOReader KOSync protocol** (`internal/koreader/handler.go`)
- **KOReader endpoints**: user create, auth, progress update, progress get (`internal/koreader/handler.go:49-54`)
- **KOReader → Lexicon progress sync** via filename matching (`internal/koreader/handler.go:275-307`)
- **Hardcover sync settings** storage (`internal/hardcover/handler.go`, `internal/hardcover/service.go`)

#### Missing / Partial ⚠️
- **OPDS `opds_user` table**: Not implemented. OPDS Basic Auth validates against main `users` table with bcrypt (`internal/opds/handler.go:80-134`).
- **OPDS OpenSearch description**: No `/opds/search` endpoint.
- **OPDS series/author feeds**: Not implemented.
- **OPDS recent feed**: Not implemented.
- **Kobo `kobo_user_settings` table**: Not in migrations.
- **Kobo `kobo_library_snapshot` table**: Not in migrations. Snapshot-based sync not implemented; `handleLibrarySync` returns the direct book list instead of computing add/remove/changed against a snapshot.
- **KOReader `koreader_user` schema**: `user_id` is nullable (`migrations/012_koreader.up.sql:3`) instead of `NOT NULL UNIQUE` as in plan. No `hardcover_forward` column.
- **KOReader document matching**: Uses `file_path LIKE '%' || ? || '%'` filename matching (`internal/koreader/handler.go:278`) instead of MD5/fingerprint matching as specified.
- **Hardcover progress forwarding**: Settings UI exists but no progress forwarding to Hardcover API is implemented.

---

### Section 15: Background Tasks

#### Implemented ✅
- **Task runner**: goroutine with `context.WithCancel`, max 1 per type (`internal/task/runner.go:14-75`)
- **Mark interrupted RUNNING tasks as FAILED on startup** (`internal/task/runner.go:154-160`)
- **WebSocket progress broadcasts** (`internal/task/runner.go:163-198`)
- **Cron scheduler** with `robfig/cron/v3` (`internal/task/scheduler.go`)
- **Default cron schedules seeded** (`internal/task/scheduler.go:13-22`):
  - LIBRARY_SCAN: `0 */6 * * *`
  - DUPLICATE_DETECTION: `0 2 * * 0`
  - RECOMMENDATION_REBUILD: `0 3 * * *`
  - AUDIT_LOG_CLEANUP: `0 1 * * *`
- **All 8 task type constants** defined (`internal/task/types.go`)
- **Task API endpoints**: list, get, run, cancel, cron list/update (`internal/task/handler.go`)
- **LIBRARY_SCAN task** (`internal/task/library_scan.go`)
- **DUPLICATE_DETECTION task** (`internal/task/duplicate_detection.go`)
- **FILE_ORGANIZATION task** (`internal/task/organization.go`)
- **RECOMMENDATION_REBUILD task** (registered in `internal/server/server.go`)
- **AUDIT_LOG_CLEANUP task** (registered as inline func in `internal/server/server.go`)

#### Missing / Partial ⚠️
- **METADATA_REFRESH task**: Type constant exists but no implementation registered.
- **COVER_REFRESH task**: Type constant exists but no implementation registered.
- **BOOKDROP_SCAN task**: Type constant exists but no implementation registered (BookDrop system does not exist).

---

## Code References

### WebSocket
- `internal/ws/hub.go:25` — Hub struct with user-scoped client maps
- `internal/ws/handler.go:46` — `/ws` upgrade endpoint with JWT auth
- `web/src/shared/ws/socket.ts:25` — Reconnecting WebSocket client

### Storage & Covers
- `internal/storage/cover.go:59` — `ExtractCover()` per-format dispatcher
- `internal/storage/fingerprint.go:21` — `Fingerprint()` MD5 partial hash
- `internal/storage/handler.go:38` — Cover serving routes

### Library & Scanner
- `internal/library/scanner.go:240` — `ScanLibrary()` with organization modes
- `internal/library/watcher.go:154` — fsnotify event loop with debounce
- `internal/storage/metadata.go:54` — `ExtractMetadata()` per-format dispatcher

### Metadata Providers
- `internal/metadata/types.go:6` — Provider interface
- `internal/metadata/googlebooks.go:16` — Google Books implementation
- `internal/metadata/hardcover.go:37` — Hardcover GraphQL implementation
- `internal/metadata/service.go:39` — Provider search orchestration

### Reader
- `internal/reader/handler.go:70` — Reader routes (stream, progress, settings, comic pages)
- `internal/reader/comic.go:33` — `ListComicPages()` and `GetComicPage()`
- `web/src/features/reader/ReaderDispatch.tsx:61` — Reader route dispatcher

### Shelves
- `internal/shelf/handler.go:47` — Manual shelf routes
- `internal/shelf/magic_handler.go:68` — Magic shelf routes
- `internal/shelf/magic.go:92` — `BuildQuery()` SQL WHERE builder

### Device Sync
- `internal/opds/handler.go:66` — OPDS routes
- `internal/kobo/handler.go:72` — Kobo store API proxy routes
- `internal/koreader/handler.go:49` — KOReader KOSync routes

### Tasks
- `internal/task/runner.go:14` — Task Runner struct
- `internal/task/scheduler.go:24` — Cron Scheduler struct
- `internal/task/types.go:6` — Task type constants
- `internal/server/server.go` — Task registrations

---

## Architecture Documentation

### Implemented Patterns
- **Domain-organized code**: Each feature lives in `internal/<domain>/` with its own handlers, services, queries, and types.
- **sqlc for type-safe queries**: Every domain package owns its `queries.sql` and generated `queries.sql.go`.
- **WebSocket hub pattern**: Central hub with user-scoped broadcast sets; tasks broadcast progress to all connected clients.
- **Rule-to-SQL translation**: Magic shelves compile JSON rules to parameterized SQL using allowlists for fields and operators.
- **Token query param auth**: Reader endpoints support `?token=` for HTML5 `<audio>` elements that cannot set headers.

### Deviations from Plan
- **Unified annotation model**: The plan specified separate `annotations`, `book_marks`, `book_notes`, and `pdf_annotations` tables. The implementation uses a single `annotation` table with a `type` column.
- **OPDS auth against main users**: The plan specified a dedicated `opds_user` table. The implementation reuses the main `users` table with `opds_access` permission flag.
- **Kobo direct sync vs. snapshot sync**: The plan specified snapshot-based incremental sync using `kobo_library_snapshot`. The implementation returns the full current book list on each sync.
- **KOReader filename matching**: The plan specified MD5/fingerprint document matching. The implementation uses `file_path LIKE '%filename%'`.

---

## Open Questions

1. Are `METADATA_REFRESH`, `COVER_REFRESH`, and `BOOKDROP_SCAN` tasks planned for future implementation, or should their constants be removed?
2. Should the magic shelf rule engine be expanded to support the full 50+ fields and 19 operators, or is the current subset sufficient?
3. Is Kobo snapshot-based sync still a goal, or is the current direct-list approach adequate for the target use case?
4. Should KOReader document matching be migrated from filename-based to fingerprint-based as originally planned?
5. Are PDF annotations and separate bookmarks/notes tables intentionally replaced by the unified annotation model, or should they be added later?
