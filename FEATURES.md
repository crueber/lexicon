# Lexicon Features

A comprehensive feature list for Lexicon, a self-hosted digital library manager.

---

## Core & Deployment

- **Single-container deployment** — Runs as one Docker image with no external database or service dependencies.
- **SQLite database with WAL mode** — Single-file database supporting concurrent reads with one writer.
- **Embedded web frontend** — Single binary serves its own web UI; no separate web server required.
- **Environment-variable configuration only** — All settings loaded from environment variables; no config files.
- **Structured logging** — JSON or text log output with configurable level.
- **Health check endpoint** — Returns service status with database connectivity verification.
- **User/group ID remapping for Docker** — Container entrypoint remaps the internal user to match host filesystem ownership.
- **Docker Compose support** — Provided `docker-compose.yml` for quick homelab deployment.

---

## Authentication & Authorization

- **Local JWT authentication** — Username/password login with short-lived access tokens and refresh token rotation.
- **Role-based access control** — Two roles: ADMIN and USER, with permission-bit granularity for fine-grained access.
- **Per-library access control** — Admins can restrict which libraries each user can see and manage.
- **Permission-based feature gating** — Individual permissions for actions like book deletion, library management, and email sending.
- **Token refresh with rotation** — Refresh tokens are SHA-256 hashed server-side and rotated on use.
- **Session revocation** — Refresh tokens can be revoked; WebSocket pushes revocation notice to clients.
- **HTTP Bearer token middleware** — Extracts JWT from `Authorization: Bearer` header for all API routes.
- **Query-parameter token extraction** — Supports `?token=` for HTML5 `<audio>` and `<video>` elements that cannot set headers.
- **OpenID Connect authentication** — External login via OIDC providers with authorization code flow.
- **Reverse-proxy header authentication** — SSO authentication via configurable headers (`Remote-User`, `Remote-Email`, `Remote-Groups`).
- **Auto-user-creation on first OIDC/Remote login** — New users are created automatically on first successful external authentication.
- **Group-to-permission mapping** — OIDC group claims can be mapped to Lexicon roles and permissions.

---

## Library Management

- **Multiple libraries** — Create and manage multiple independent libraries, each with its own scan paths and settings.
- **Multiple watch paths per library** — Each library can watch several directories on the filesystem.
- **Filesystem watching with debounce** — Uses `fsnotify` to detect new, modified, and deleted files with a 5-second debounce.
- **Automatic library scanning** — Scans watch paths on startup and when filesystem events are detected.
- **Manual library scan trigger** — Admins can trigger a scan on demand via API or UI.
- **Book-per-file and book-per-folder modes** — Scanner supports both single-file books and folder-based books (audiobooks).
- **Fingerprint-based file tracking** — MD5 hash of file head and tail detects moved or renamed files without re-importing.
- **Mark missing files** — Files no longer found on disk can be marked as missing in the database.

---

## Book Management

- **Multiple files per book** — A single book record can have multiple files of different formats.
- **Format support: EPUB** — Ebook format with reflowable text and OPF metadata.
- **Format support: PDF** — Document format with page-based reading.
- **Format support: CBZ, CBR, CB7** — Comic archive formats with page extraction.
- **Format support: MOBI, AZW3, FB2** — Additional ebook formats recognized and stored.
- **Format support: M4B, M4A, MP3, OPUS** — Audiobook and audio formats with metadata extraction.
- **Cover extraction** — Automatically extracts covers from EPUB, PDF, CBZ/CBR/CB7, and audio tags.
- **Cover variants** — Generates full-size (800px), thumbnail (200x300), and audiobook square (600x600) covers.
- **Cover serving** — Serves covers and thumbnails via HTTP with appropriate cache headers.
- **Custom cover upload** — Users can upload custom cover images for any book.
- **Cover deletion** — Removes custom covers and falls back to extracted covers.
- **Duplicate detection** — Finds duplicate books using strict, moderate, loose, or title-only comparison presets.
- **Duplicate dismissal** — Admins can dismiss specific duplicate pairs to exclude them from future results.
- **Book merging** — Combine two book records and their files into one.
- **File organization task** — Background task renames and moves book files using token patterns like `{author}/{title}{ext}`.

---

## Metadata

- **Embedded metadata extraction** — Reads metadata from EPUB OPF, PDF XMP/DocInfo, CBZ ComicInfo.xml, and audio ID3/MP4 tags.
- **Metadata providers** — Fetches metadata from external sources including Google Books, Hardcover, OpenLibrary, ComicVine, Douban, LubimyCzytac, RanobeDB, and Audible.
- **Search-by-title across providers** — Queries multiple providers simultaneously and returns aggregated results.
- **Fetch-by-ID** — Retrieves full metadata from a provider using its native identifier.
- **Metadata proposals** — Creates a proposal record for admin review before applying fetched metadata.
- **Proposal accept/reject** — Admins can accept (applies metadata) or reject (discards) each proposal.
- **Per-field locking** — Individual fields on book metadata can be locked to prevent automatic overwriting.
- **Field lock enforcement** — Applying a proposal respects locked fields and only updates unlocked ones.
- **Per-library provider configuration** — Each library can be configured with which metadata providers to use.
- **Rate limiting** — Metadata provider requests are rate-limited to respect external APIs.
- **Cover extraction from providers** — Fetched metadata can include cover images downloaded from provider URLs.

---

## Readers

- **EPUB reader** — In-browser EPUB reader with chapter navigation, table of contents, and CFI-based progress.
- **PDF reader** — In-browser PDF reader with page navigation, thumbnails, text search, and zoom.
- **Comic reader** — Custom canvas-based reader for CBZ/CBR/CB7 with single-page and double-page modes.
- **Audiobook player** — HTML5 audio player with track/chapter navigation, playback speed control, and Media Session API integration.
- **Reading progress persistence** — Saves and resumes exact reading position for all formats.
- **CFI-based EPUB progress** — Uses EPUB Canonical Fragment Identifier for precise location within a chapter.
- **Page-based PDF progress** — Saves and resumes at the exact page number.
- **Comic page-based progress** — Saves and resumes at the specific page index.
- **Audiobook time-based progress** — Saves and resumes at the exact timestamp within a track.
- **EPUB reader settings** — Configurable font family, font size, theme (light/dark/sepia), margins, and line height.
- **Custom font support in EPUB** — Users can upload custom fonts and use them in the EPUB reader.
- **PDF reader settings** — Configurable spread mode and theme.
- **Audiobook player settings** — Configurable playback speed and theme.
- **PDF annotations** — Page-based notes with color picker, annotation sidebar, and thumbnail indicators.
- **Reader settings persistence** — Per-user reader preferences are saved to the database.
- **Full-screen reading mode** — All readers support full-screen display.
- **Keyboard navigation** — Arrow keys and spacebar for page/chapter navigation.

---

## Shelves & Collections

- **Manual shelves** — Users can create named shelves and add/remove books from them.
- **Magic shelves** — Rule-based dynamic shelves that automatically include books matching configurable criteria.
- **Magic shelf rule builder** — Supports AND/OR group nesting up to 3 levels with field, operator, and value conditions.
- **Magic shelf sorting** — Configurable sort field and direction for dynamic results.
- **Magic shelf limit** — Optional maximum number of books displayed in a magic shelf.
- **Live magic shelf count** — Shows the current number of matching books without loading the full list.
- **Per-user shelves** — Shelves are private to the user who created them.

---

## Annotations & Notebook

- **EPUB highlights** — Select text in the EPUB reader and create highlighted annotations with colors.
- **Anchored notes** — Attach notes to specific locations in a book using CFI or page number.
- **Bookmarks** — Save quick-access bookmarks at any position in a book.
- **Color-coded annotations** — Yellow, green, blue, pink, and purple highlight colors.
- **Annotation CRUD** — Create, list, update, and delete annotations via API.
- **Unified notebook view** — Paginated cross-book view of all annotations with filtering by book, color, and text search.
- **Content restriction filtering in notebook** — Restrictions apply to notebook results so users only see annotations for accessible books.
- **Markdown export** — Export all annotations to a downloadable Markdown file grouped by book.

---

## Dashboard & Discovery

- **Customizable dashboard** — Users can configure which scroller rows appear on their home page.
- **Continue reading row** — Shows books the user has started reading, ordered by most recent progress.
- **Recently added row** — Shows books most recently added to the library.
- **Random picks row** — Shows a random selection of books from the library.
- **Dashboard stats bar** — Displays total books, total libraries, books read this month, and total reading time.
- **Content restriction filtering on dashboard** — Restrictions apply to all dashboard rows.
- **Reading sessions tracking** — Records reading activity to compute accurate reading time and books-completed statistics.
- **Book recommendations** — Suggests similar books based on feature-hashing vector similarity with a per-author cap.
- **Recommendation rebuild task** — Background task recomputes feature vectors for all books.
- **Author browse page** — Dedicated page for browsing all authors with book counts.
- **Author detail page** — Shows all books by a selected author.
- **Series browse page** — Dedicated page for browsing all series with book counts.
- **Series detail page** — Shows all books in a selected series.

---

## Device Sync

- **OPDS 1.2 catalog** — Serves Atom/XML OPDS feeds for compatible e-reader clients.
- **OPDS root catalog** — Top-level navigation feed with libraries and shelves.
- **OPDS library feeds** — Per-library book listings with acquisition links.
- **OPDS shelf feeds** — Per-shelf book listings.
- **OPDS pagination** — Paginated feeds for large libraries.
- **OPDS Basic Auth** — Authenticates OPDS clients using username/password over HTTP Basic Auth.
- **OPDS content restriction filtering** — Restrictions apply to OPDS feeds.
- **Kobo device sync** — Full Kobo store API proxy for syncing books and reading progress to Kobo e-readers.
- **Kobo initialization** — Device initialization endpoint returns user profile and library state.
- **Kobo library sync** — Returns book metadata, covers, and download URLs in Kobo's native format.
- **KEPUB conversion** — Converts EPUB files to Kobo's KEPUB format on demand with disk caching.
- **Kobo reading state sync** — Syncs reading progress, bookmarks, and annotations between Kobo and Lexicon.
- **Kobo token generation** — Users can generate a unique sync token for their Kobo device.
- **Kobo content restriction filtering** — Restrictions apply to Kobo library sync.
- **KOReader sync** — Implements the KOSync protocol for syncing reading progress with KOReader.
- **KOReader user registration** — KOReader devices can register and authenticate via Basic Auth.
- **KOReader progress sync** — Upload and download reading progress for individual documents.
- **KOReader filename matching** — Matches KOReader documents to book files by filename.
- **Hardcover sync settings** — Stores Hardcover API key and sync preferences per user.

---

## BookDrop

- **Watch-folder ingest queue** — Monitors a drop folder for new files and adds them to a review queue.
- **File stability detection** — Waits for file size to stabilize before processing to avoid catching incomplete copies.
- **Automatic metadata extraction** — Extracts title, authors, and cover from dropped files before import.
- **Reviewable import UI** — Users can review extracted metadata and choose to import or reject each file.
- **Bulk import** — Import all pending files at once to a target library.
- **BookDrop scan task** — Background task that scans the drop directory on demand.
- **WebSocket notifications** — Real-time notification when a new file arrives in the drop folder.

---

## Email / Send-to-Device

- **SMTP provider configuration** — Admin-configurable SMTP hosts with TLS/startTLS support.
- **Provider test send** — Send a test email to verify SMTP configuration.
- **Recipient management** — Users can save personal recipient email addresses for send-to-device.
- **MIME multipart book delivery** — Sends book files as email attachments with text body.
- **Streaming attachment** — Attachments are streamed from disk without loading entire files into memory.
- **Permission-gated sending** — The `can_email_send` permission controls access to the send feature.
- **Send from book detail** — One-click send to any saved recipient from the book detail page.

---

## Background Tasks

- **Task runner** — Goroutine-based background task system with one-at-a-time enforcement per task type.
- **Task progress reporting** — Tasks report progress steps and broadcast via WebSocket.
- **Task cancellation** — Running tasks can be cancelled via context cancellation.
- **Cron scheduling** — Tasks can be scheduled with standard cron expressions.
- **Library scan task** — Scheduled background scanning of all library watch paths.
- **Recommendation rebuild task** — Scheduled recomputation of book recommendation vectors.
- **Duplicate detection task** — Scheduled scanning for duplicate books across the library.
- **File organization task** — On-demand task to rename and reorganize book files.
- **Audit log cleanup task** — Scheduled deletion of old audit log entries.
- **Task API** — List tasks, view details, trigger manually, cancel running tasks, and manage cron schedules.
- **Interrupted task recovery** — Tasks marked RUNNING at startup are automatically marked FAILED.
- **Task monitor frontend** — Real-time background task viewer with progress bars, run controls, and cancel buttons.

---

## WebSocket

- **Real-time WebSocket connection** — Bidirectional communication between frontend and backend.
- **JWT-authenticated WebSocket** — Connection established with JWT via query parameter or Authorization header.
- **Task progress events** — Real-time progress updates for background tasks.
- **Library scan completion events** — Notification when a library scan finishes.
- **Book added events** — Notification when a new book is discovered during scanning.
- **Session revocation events** — Pushes token revocation to connected clients for immediate logout.
- **Client ping / server pong** — Keeps connections alive with heartbeat messages.
- **Auto-reconnect with backoff** — Frontend automatically reconnects with exponential backoff on disconnect.

---

## Internationalization

- **Signal-based i18n system** — Reactive locale system that updates all UI text without page reload.
- **English locale** — Complete English translation covering all UI text across all namespaces.
- **Namespace organization** — Keys grouped by domain: common, auth, library, book, metadata, shelf, reader, admin, errors.
- **Graceful fallback** — Missing keys display the key name instead of crashing or showing blank text.
- **Runtime locale switching** — Users can switch languages at runtime.

---

## Audit Logging

- **Async action logging** — Non-blocking audit log creation via goroutine.
- **21 action types** — Covers user login/logout, user CRUD, book download/deletion, library management, shelf management, and sync events.
- **Admin audit log viewer** — Paginated, filterable table with action type, user, date range, and IP address filters.
- **Audit log cleanup task** — Automatically removes audit entries older than a configurable retention period.
- **IP address capture** — Records the remote IP address for each logged action.

---

## Content Restrictions

- **Per-user content filtering** — Each user can configure their own content restrictions.
- **EXCLUDE mode** — Hide books matching specific categories, tags, moods, or age ratings.
- **ALLOW_ONLY mode** — Show only books matching specific criteria; hide everything else.
- **Restriction types** — Filter by category, tag, mood, age rating, or content rating.
- **Admin bypass** — Administrators see all books regardless of restrictions.
- **Cross-feature filtering** — Restrictions apply to book listings, shelves, dashboard, notebook, OPDS, Kobo sync, and recommendations.

---

## Font Management

- **Custom font upload** — Users can upload TTF, OTF, WOFF, and WOFF2 font files.
- **Font listing** — Browse all uploaded fonts with format and name.
- **Font deletion** — Remove uploaded fonts from the system.
- **Font file serving** — Serves font files with correct content-type and caching headers.
- **EPUB reader integration** — Uploaded fonts can be selected and applied in the EPUB reader.

---

## Admin & System

- **User management** — Admins can create, edit, delete users and assign roles/permissions.
- **User permissions editing** — Granular permission bits can be set per user.
- **User library access editing** — Admins can control which libraries each user can access.
- **Metadata provider settings** — Admin-configurable API keys for external metadata providers.
- **Audit log viewer** — Browse and filter the full audit log history.
- **Task management** — View running and completed tasks, trigger tasks manually, manage cron schedules.
- **App settings storage** — Key-value runtime settings stored in the database.
- **Runtime settings editor** — Full admin UI for viewing and editing all runtime application settings.

---

## Not Yet Implemented

All planned features have been implemented.
