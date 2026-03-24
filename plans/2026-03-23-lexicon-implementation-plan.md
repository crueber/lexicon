# Lexicon — Implementation Plan

> **Purpose**: Ground-up implementation specification for Lexicon, a self-hosted digital library manager. Inspired by BookLore (now vaporware), rebuilt from scratch with a minimalist philosophy.
> **Backend**: Go (idiomatic, minimal dependencies)
> **Frontend**: SolidJS + TypeScript + Tailwind CSS
> **Database**: SQLite with WAL mode (single-file, zero external dependencies)
> **Deployment**: Single Docker image, homelab-friendly, port 6060

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Architecture & Philosophy](#2-architecture--philosophy)
3. [Technology Stack](#3-technology-stack)
4. [Go Project Structure](#4-go-project-structure)
5. [Database Schema](#5-database-schema)
6. [Authentication & Authorization](#6-authentication--authorization)
7. [REST API Specification](#7-rest-api-specification)
8. [WebSocket Events](#8-websocket-events)
9. [File Storage & Cover Management](#9-file-storage--cover-management)
10. [Library & Book Management](#10-library--book-management)
11. [Metadata Providers](#11-metadata-providers)
12. [Reader Support](#12-reader-support)
13. [Shelves & Magic Shelves](#13-shelves--magic-shelves)
14. [Device Sync Integrations](#14-device-sync-integrations)
15. [Background Tasks](#15-background-tasks)
16. [Email / Send-to-Device](#16-email--send-to-device)
17. [BookDrop](#17-bookdrop)
18. [Dashboard](#18-dashboard)
19. [Notebook (Annotations)](#19-notebook-annotations)
20. [Recommendations](#20-recommendations)
21. [Audit Logs](#21-audit-logs)
22. [Content Restrictions](#22-content-restrictions)
23. [SolidJS Frontend Specification](#23-solidjs-frontend-specification)
24. [Docker Deployment Specification](#24-docker-deployment-specification)
25. [Configuration & Environment Variables](#25-configuration--environment-variables)
26. [Application Settings (Runtime)](#26-application-settings-runtime)
27. [Implementation Phases](#27-implementation-phases)

---

## 1. Project Overview

Lexicon is a self-hosted, multi-user digital library manager. It stores and organizes ebooks, comics, and audiobooks. Users can read books in-browser, sync reading progress to Kobo and KOReader devices, browse an OPDS catalog, and manage metadata from multiple providers.

### Core Capabilities

- Multi-user with role-based permissions (ADMIN / USER)
- Multiple libraries, each with multiple filesystem watch paths
- Supports: EPUB, PDF, CBZ, CBR, CB7, MOBI, AZW3, FB2 (ebooks); M4B, M4A, MP3, OPUS (audiobooks)
- Multiple files per book record
- Folder-based audiobook detection
- In-browser readers: PDF, EPUB, Comic, Audiobook
- EPUB highlights, bookmarks, anchored notes, CFI-based progress
- OPDS 1.2 catalog with per-library/shelf/series/author navigation
- Kobo device sync (full Kobo store API proxy, KEPUB conversion)
- KOReader sync (KOSync protocol)
- BookDrop: watch-folder ingest queue
- Email send-to-device
- Metadata providers with per-field priority matrix
- Magic Shelves: rule-based dynamic collections
- Recommendations: feature-hashing vector similarity
- Audit logging

---

## 2. Architecture & Philosophy

### Minimalist Principles

1. **Self-contained deployment**: Single Docker image with embedded frontend. SQLite database — no external database container required.
2. **Minimal dependencies**: Use focused, lightweight libraries — not frameworks. Prefer stdlib where it provides good ergonomics (e.g., `log/slog` over zerolog). Avoid massive dependency trees.
3. **Developer ergonomics over purity**: We use libraries where they meaningfully reduce boilerplate or complexity (chi for routing, sqlc for type-safe queries). We don't use stdlib just for the sake of it.
4. **Domain-organized code**: Both Go backend and SolidJS frontend are organized by feature/domain, not by technical layer.
5. **Subagent-friendly phasing**: The implementation is broken into 36 granular phases, each independently implementable with clear inputs, outputs, and verification criteria.

### Key Architectural Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Database | SQLite + WAL mode | Single-file, zero external deps, perfect for homelab. Write serialization is irrelevant for this workload. |
| SQLite driver | `modernc.org/sqlite` | Pure Go, no CGO required. Simplifies cross-compilation and Docker builds. |
| Query generation | `sqlc` | Type-safe queries generated at build time. Zero runtime dependency. |
| HTTP router | `chi` | Lightweight middleware-friendly router. Not a framework. |
| Logging | `log/slog` (stdlib) | Go 1.21+ built-in structured logging. No external dependency needed. |
| Config | `caarlos0/env` | Tiny lib — parses env vars into struct with tags. Replaces viper (which is massive). |
| Frontend state | SolidJS signals + context | No external state library. Signals are fine-grained and sufficient. |
| CSS | Tailwind CSS | Utility-first, no runtime cost, standard tooling. |
| UI primitives | `@kobalte/core` | Headless accessible components. Not a full component library. |
| Module path | `github.com/crueber/lexicon` | Public module path. |

---

## 3. Technology Stack

### Backend (Go)

| Concern | Library | Notes |
|---|---|---|
| HTTP router | `go-chi/chi` v5 | Lightweight, middleware-friendly |
| Database driver | `modernc.org/sqlite` | Pure Go SQLite, no CGO |
| Query codegen | `sqlc` | Build-time only, generates type-safe Go from SQL |
| Migrations | `golang-migrate` | Use `database/sqlite` driver (pure Go), NOT `database/sqlite3` (CGO) |
| Auth/JWT | `golang-jwt/jwt` v5 | JWT issue/validate/refresh |
| OIDC | `coreos/go-oidc` v3 | OIDC provider integration |
| WebSocket | `coder/websocket` | Modern, maintained (replaces gorilla) |
| Config | `caarlos0/env` | Env var → struct parsing |
| Logging | `log/slog` (stdlib) | Structured logging, built-in |
| File watching | `fsnotify` | Filesystem event notifications |
| Image processing | `disintegration/imaging` | Resize, crop, JPEG encode |
| PDF processing | `pdfcpu` | Cover extraction from PDF |
| EPUB parsing | Custom zip reader | Parse OPF from EPUB zip |
| Archive extraction | `mholt/archives` | Pure Go RAR/7z/ZIP extraction (CBR/CB7/CBZ) |
| Audio metadata | `dhowden/tag` | ID3/MP4 tag reading |
| HTTP client | `net/http` (stdlib) | With timeouts, for metadata providers |
| Cron | `robfig/cron` v3 | Cron expression scheduling |
| Vector similarity | Pure Go | 128-dim float32, cosine similarity |
| Password hashing | `golang.org/x/crypto/bcrypt` | Extended stdlib |
| Testing | `testing` (stdlib) | Table-driven tests, `go test -race` |

### Frontend (SolidJS)

| Concern | Library | Notes |
|---|---|---|
| Framework | SolidJS + TypeScript | Fine-grained reactivity |
| Build | Vite | Standard build tool |
| Routing | `@solidjs/router` | Standard Solid router |
| UI primitives | `@kobalte/core` | Headless, accessible |
| Styling | Tailwind CSS | Utility-first |
| Icons | `lucide-solid` | Lightweight icon set |
| PDF reader | `pdfjs-dist` | Mozilla's PDF.js |
| EPUB reader | `epubjs` | FolioJS EPUB renderer |
| Comic reader | Custom canvas | No library needed |
| Audio player | HTML5 `<audio>` | + Media Session API |
| HTTP client | `fetch` wrapper | Typed wrapper around browser fetch |
| WebSocket | Native browser API | Reconnecting wrapper |
| i18n | Custom (signal-based) | ~50 lines, no library |
| Markdown | `marked` | Lightweight Markdown parser |

### Infrastructure

| Concern | Choice | Notes |
|---|---|---|
| Container base | `alpine:3.20` | Small runtime base |
| Build | Multi-stage Dockerfile | Go build + Vite build → alpine runtime |
| Database | SQLite (embedded) | WAL mode, single file in data volume |
| External tools | `kepubify`, `ffprobe` | Downloaded to data dir at startup |

---

## 4. Go Project Structure

```
lexicon/
├── cmd/
│   └── lexicon/
│       └── main.go               # entry point: parse env, call run(), handle error
├── internal/
│   ├── server/
│   │   ├── server.go             # HTTP server setup, route registration
│   │   ├── middleware.go         # shared middleware (logging, recovery, CORS)
│   │   └── routes.go            # all route registration in one place
│   ├── auth/
│   │   ├── handler.go           # login/logout/refresh/me endpoints
│   │   ├── jwt.go               # JWT issue/validate/refresh
│   │   ├── oidc.go              # OIDC provider, callback, session
│   │   ├── remote.go            # Remote Auth header middleware
│   │   ├── middleware.go        # HTTP middleware: require auth, require admin
│   │   └── types.go             # Principal, Claims structs
│   ├── user/
│   │   ├── handler.go           # REST handlers
│   │   ├── service.go
│   │   └── queries.sql          # sqlc query file
│   ├── library/
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── queries.sql
│   │   ├── scanner.go           # filesystem scan, fingerprint
│   │   └── watcher.go           # fsnotify watch loop
│   ├── book/
│   │   ├── handler.go
│   │   ├── service.go
│   │   └── queries.sql
│   ├── metadata/
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── provider.go          # provider implementations (one file per provider or grouped)
│   │   └── types.go
│   ├── shelf/
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── queries.sql
│   │   └── magic.go             # rule evaluation engine
│   ├── reader/
│   │   ├── handler.go           # serve book file, progress, annotations
│   │   ├── service.go
│   │   └── queries.sql
│   ├── opds/
│   │   ├── handler.go           # OPDS Atom feeds
│   │   └── auth.go              # OPDS Basic Auth
│   ├── kobo/
│   │   ├── handler.go           # Kobo store API proxy
│   │   ├── service.go
│   │   └── sync.go              # snapshot-based incremental sync
│   ├── koreader/
│   │   ├── handler.go           # KOSync protocol
│   │   └── service.go
│   ├── bookdrop/
│   │   ├── handler.go
│   │   ├── service.go
│   │   └── watcher.go           # fsnotify stability check
│   ├── email/
│   │   ├── handler.go
│   │   └── service.go
│   ├── dashboard/
│   │   ├── handler.go
│   │   └── service.go
│   ├── notebook/
│   │   ├── handler.go
│   │   └── queries.sql
│   ├── recommendation/
│   │   ├── service.go
│   │   └── vector.go            # feature hashing, cosine similarity
│   ├── task/
│   │   ├── handler.go
│   │   ├── runner.go            # goroutine-based task execution
│   │   ├── scheduler.go         # robfig/cron integration
│   │   └── types.go
│   ├── audit/
│   │   ├── handler.go
│   │   └── queries.sql
│   ├── storage/
│   │   ├── cover.go             # cover extraction, thumbnail generation
│   │   ├── handler.go           # serve cover images, fonts
│   │   ├── fingerprint.go       # file fingerprinting
│   │   └── font.go              # font management
│   ├── ws/
│   │   ├── hub.go               # WebSocket connection hub
│   │   └── handler.go           # /ws upgrade endpoint
│   └── appsettings/
│       ├── service.go           # key-value app settings store
│       └── queries.sql
├── migrations/                   # SQL migration files (top-level for visibility)
│   ├── 001_users.up.sql
│   ├── 001_users.down.sql
│   └── ...
├── sqlc.yaml                     # sqlc configuration
├── web/                          # SolidJS app (go:embed in production)
│   ├── src/
│   │   ├── features/
│   │   │   ├── auth/
│   │   │   ├── library/
│   │   │   ├── book/
│   │   │   ├── reader/
│   │   │   ├── shelf/
│   │   │   ├── dashboard/
│   │   │   ├── notebook/
│   │   │   ├── admin/
│   │   │   └── bookdrop/
│   │   ├── shared/
│   │   │   ├── ui/              # reusable UI components
│   │   │   ├── api/             # typed fetch wrapper
│   │   │   ├── ws/              # WebSocket client
│   │   │   └── i18n/            # i18n system
│   │   ├── App.tsx
│   │   └── index.tsx
│   ├── index.html
│   ├── tailwind.config.ts
│   ├── vite.config.ts
│   ├── tsconfig.json
│   └── package.json
├── Dockerfile
├── Makefile
├── AGENTS.md
└── go.mod
```

### Key Structural Notes

- **`cmd/lexicon/main.go`**: Thin entry point. Parses env config, calls `run()`, handles the error. `os.Exit` only here.
- **`internal/`**: All application code. Nothing exported for external use.
- **Domain packages own their queries**: Each domain has its own `queries.sql` for sqlc. No shared `db/` package.
- **`migrations/`**: Top-level for visibility and easy access. Not buried in `internal/`.
- **`web/`**: SolidJS app organized by feature under `src/features/`. Shared UI components under `src/shared/`.
- **No `internal/config/` package**: Config is a struct defined near `main.go` or in `internal/server/`, populated by `caarlos0/env`.
- **No `internal/db/` package**: Each domain owns its own database interactions via sqlc-generated code.

---

## 5. Database Schema

**Database**: SQLite with WAL mode.

All tables below are the canonical target schema. Use `golang-migrate` SQL migration files in `migrations/`.

**SQLite-specific notes**:
- Use `INTEGER PRIMARY KEY` for auto-increment (SQLite's rowid alias)
- Use `TEXT` instead of `VARCHAR(n)` (SQLite doesn't enforce length)
- Use `REAL` instead of `DECIMAL`
- Use `TEXT` for timestamps (ISO 8601 format), with `DEFAULT (datetime('now'))`
- No `BIGSERIAL` — use `INTEGER PRIMARY KEY AUTOINCREMENT`
- No `INET` type — store IP as TEXT
- No `JSONB` — use `TEXT` (JSON stored as text, parsed in Go)
- No `FLOAT4[]` — store vectors as BLOB (binary float32 array)

### 5.1 Users & Auth

```sql
CREATE TABLE users (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    username        TEXT UNIQUE NOT NULL,
    email           TEXT,
    password_hash   TEXT,                  -- bcrypt, NULL for OIDC-only users
    name            TEXT,
    enabled         INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE user_permissions (
    user_id                 INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    role                    TEXT NOT NULL DEFAULT 'USER',  -- ADMIN | USER
    can_download            INTEGER NOT NULL DEFAULT 0,
    can_upload              INTEGER NOT NULL DEFAULT 0,
    can_email_send          INTEGER NOT NULL DEFAULT 0,
    can_edit_metadata       INTEGER NOT NULL DEFAULT 0,
    opds_access             INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_library_permission (
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    library_id  INTEGER NOT NULL REFERENCES library(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, library_id)
);

CREATE TABLE user_settings (
    user_id                     INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    theme                       TEXT DEFAULT 'dark',
    book_cards_per_row          INTEGER DEFAULT 6,
    pdf_reader_setting          TEXT,       -- JSON blob
    epub_reader_setting         TEXT,       -- JSON blob
    comic_reader_setting        TEXT,       -- JSON blob
    audiobook_reader_setting    TEXT,       -- JSON blob
    sidebar_setting             TEXT,       -- JSON blob
    dashboard_setting           TEXT        -- JSON blob
);

CREATE TABLE refresh_tokens (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    device_info TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at  TEXT NOT NULL,
    revoked     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE oidc_session (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL,
    subject         TEXT NOT NULL,
    id_token        TEXT,
    access_token    TEXT,
    refresh_token   TEXT,
    expires_at      TEXT,
    UNIQUE (provider, subject)
);

CREATE TABLE oidc_group_mapping (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    group_name      TEXT NOT NULL,
    permission_key  TEXT NOT NULL,
    permission_value TEXT NOT NULL
);
```

### 5.2 Library & Books

```sql
CREATE TABLE library (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    name                    TEXT NOT NULL,
    icon                    TEXT,
    icon_color              TEXT,
    organization_mode       TEXT NOT NULL DEFAULT 'BOOK_PER_FILE',
                            -- BOOK_PER_FILE | BOOK_PER_FOLDER
    file_naming_pattern     TEXT,
    created_at              TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE library_path (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id  INTEGER NOT NULL REFERENCES library(id) ON DELETE CASCADE,
    path        TEXT NOT NULL,
    UNIQUE (library_id, path)
);

CREATE TABLE library_metadata_source (
    library_id      INTEGER NOT NULL REFERENCES library(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL,
    field_priority  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (library_id, provider)
);

CREATE TABLE book (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    library_id          INTEGER NOT NULL REFERENCES library(id) ON DELETE CASCADE,
    folder_path         TEXT,
    book_type           TEXT NOT NULL DEFAULT 'EBOOK',
                        -- EBOOK | AUDIOBOOK | COMIC
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    added_date          TEXT
);

CREATE TABLE book_file (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id         INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    file_path       TEXT NOT NULL UNIQUE,
    format          TEXT NOT NULL,
                    -- EPUB|PDF|CBZ|CBR|CB7|MOBI|AZW3|FB2|M4B|M4A|MP3|OPUS
    file_size       INTEGER,
    fingerprint     TEXT,                   -- partial MD5
    track_number    INTEGER,                -- for audiobook tracks
    track_title     TEXT,
    duration_secs   INTEGER,                -- for audio files
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE book_metadata (
    book_id             INTEGER PRIMARY KEY REFERENCES book(id) ON DELETE CASCADE,
    title               TEXT,
    original_title      TEXT,
    subtitle            TEXT,
    description         TEXT,
    publisher           TEXT,
    publish_date        TEXT,
    page_count          INTEGER,
    language            TEXT,
    isbn_10             TEXT,
    isbn_13             TEXT,
    google_books_id     TEXT,
    amazon_id           TEXT,
    goodreads_id        TEXT,
    hardcover_id        TEXT,
    audible_id          TEXT,
    comicvine_id        TEXT,
    cover_path          TEXT,
    cover_updated_at    TEXT,
    -- lock flags for each field
    title_locked            INTEGER NOT NULL DEFAULT 0,
    subtitle_locked         INTEGER NOT NULL DEFAULT 0,
    description_locked      INTEGER NOT NULL DEFAULT 0,
    publisher_locked        INTEGER NOT NULL DEFAULT 0,
    publish_date_locked     INTEGER NOT NULL DEFAULT 0,
    page_count_locked       INTEGER NOT NULL DEFAULT 0,
    language_locked         INTEGER NOT NULL DEFAULT 0,
    isbn_10_locked          INTEGER NOT NULL DEFAULT 0,
    isbn_13_locked          INTEGER NOT NULL DEFAULT 0,
    cover_locked            INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE author (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,
    bio         TEXT,
    photo_path  TEXT,
    audnexus_id TEXT
);

CREATE TABLE book_author (
    book_id     INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    author_id   INTEGER NOT NULL REFERENCES author(id) ON DELETE CASCADE,
    sort_order  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (book_id, author_id)
);

CREATE TABLE series (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    name    TEXT NOT NULL UNIQUE
);

CREATE TABLE book_series (
    book_id         INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    series_id       INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    series_number   REAL,
    PRIMARY KEY (book_id, series_id)
);

CREATE TABLE category (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    name    TEXT NOT NULL UNIQUE
);

CREATE TABLE book_category (
    book_id     INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    category_id INTEGER NOT NULL REFERENCES category(id) ON DELETE CASCADE,
    PRIMARY KEY (book_id, category_id)
);

CREATE TABLE tag (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    name    TEXT NOT NULL UNIQUE
);

CREATE TABLE book_tag (
    book_id INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    tag_id  INTEGER NOT NULL REFERENCES tag(id) ON DELETE CASCADE,
    PRIMARY KEY (book_id, tag_id)
);

CREATE TABLE mood (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    name    TEXT NOT NULL UNIQUE
);

CREATE TABLE book_mood (
    book_id INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    mood_id INTEGER NOT NULL REFERENCES mood(id) ON DELETE CASCADE,
    PRIMARY KEY (book_id, mood_id)
);

CREATE TABLE comic_metadata (
    book_id         INTEGER PRIMARY KEY REFERENCES book(id) ON DELETE CASCADE,
    web             TEXT,
    volume          INTEGER,
    black_and_white INTEGER DEFAULT 0,
    manga           TEXT,
    characters      TEXT,
    teams           TEXT,
    locations       TEXT,
    age_rating      TEXT,
    story_arc       TEXT,
    series_group    TEXT,
    alternate_series    TEXT,
    alternate_number    REAL,
    alternate_count     INTEGER,
    count               INTEGER,
    review              TEXT,
    type                TEXT,
    community_rating    REAL
);
```

### 5.3 Reading Progress & Sessions

```sql
CREATE TABLE user_book_file_progress (
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_file_id    INTEGER NOT NULL REFERENCES book_file(id) ON DELETE CASCADE,
    progress        TEXT,           -- CFI for EPUB, page num for PDF, seconds for audio
    progress_type   TEXT,           -- CFI | PAGE | SECONDS | PERCENT
    updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, book_file_id)
);

CREATE TABLE reading_sessions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id         INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    book_file_id    INTEGER REFERENCES book_file(id) ON DELETE SET NULL,
    start_progress  TEXT,
    end_progress    TEXT,
    started_at      TEXT NOT NULL,
    ended_at        TEXT,
    duration_secs   INTEGER
);
```

### 5.4 Annotations

```sql
CREATE TABLE annotations (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_file_id    INTEGER NOT NULL REFERENCES book_file(id) ON DELETE CASCADE,
    cfi             TEXT,
    highlighted_text TEXT,
    color           TEXT,
    note            TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE book_marks (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_file_id    INTEGER NOT NULL REFERENCES book_file(id) ON DELETE CASCADE,
    cfi             TEXT,
    label           TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE book_notes (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id         INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    content         TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE pdf_annotations (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_file_id    INTEGER NOT NULL REFERENCES book_file(id) ON DELETE CASCADE,
    page            INTEGER NOT NULL,
    annotation_data TEXT NOT NULL,       -- JSON
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);
```

### 5.5 Shelves

```sql
CREATE TABLE shelf (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    icon        TEXT,
    icon_color  TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE book_shelf_mapping (
    book_id     INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    shelf_id    INTEGER NOT NULL REFERENCES shelf(id) ON DELETE CASCADE,
    added_at    TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (book_id, shelf_id)
);

CREATE TABLE magic_shelf (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    icon        TEXT,
    icon_color  TEXT,
    rules_json  TEXT NOT NULL,          -- JSON: see Magic Shelf rules schema
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
```

**Magic Shelf rules_json schema:**
```json
{
  "operator": "AND",
  "conditions": [
    { "field": "author", "operator": "CONTAINS", "value": "Brandon Sanderson" },
    { "field": "genre", "operator": "EQUALS", "value": "Fantasy" }
  ],
  "groups": [
    {
      "operator": "OR",
      "conditions": []
    }
  ]
}
```

**Supported rule fields (50+):** title, author, series, category, tag, mood, publisher, language, isbn10, isbn13, page_count, publish_year, added_date, last_read_date, progress_percent, format, library_id, rating, content_rating, age_rating, read_status, has_cover, has_description, has_series, book_type, file_size, duration, character, team, location, story_arc, community_rating, google_books_id, amazon_id, goodreads_id, hardcover_id, and more.

**Supported operators (19):** EQUALS, NOT_EQUALS, CONTAINS, NOT_CONTAINS, STARTS_WITH, ENDS_WITH, GREATER_THAN, LESS_THAN, GREATER_THAN_OR_EQUAL, LESS_THAN_OR_EQUAL, IS_NULL, IS_NOT_NULL, IN, NOT_IN, BEFORE, AFTER, BETWEEN, REGEX, MATCHES.

### 5.6 OPDS

```sql
CREATE TABLE opds_user (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER REFERENCES users(id) ON DELETE CASCADE,
    username        TEXT UNIQUE NOT NULL,
    password_hash   TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);
```

### 5.7 Kobo Sync

```sql
CREATE TABLE kobo_user_settings (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id             INTEGER UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kobo_token          TEXT UNIQUE NOT NULL,
    convert_to_kepub    INTEGER NOT NULL DEFAULT 0,
    sync_reading_state  INTEGER NOT NULL DEFAULT 1,
    last_sync_at        TEXT
);

CREATE TABLE kobo_library_snapshot (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id         INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    book_file_id    INTEGER REFERENCES book_file(id) ON DELETE SET NULL,
    revision_id     TEXT NOT NULL,
    entry_type      TEXT NOT NULL DEFAULT 'BOOK',
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (user_id, book_id)
);

CREATE TABLE kobo_reading_state (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    book_id         INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    content_id      TEXT,
    reading_state   TEXT,           -- JSON
    updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (user_id, book_id)
);
```

### 5.8 KOReader Sync

```sql
CREATE TABLE koreader_user (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    username        TEXT UNIQUE NOT NULL,
    password_md5    TEXT NOT NULL,
    hardcover_forward INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE koreader_progress (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    document        TEXT NOT NULL,
    progress        TEXT NOT NULL,
    percentage      REAL,
    device          TEXT,
    device_id       TEXT,
    updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (user_id, document)
);
```

### 5.9 BookDrop

```sql
CREATE TABLE bookdrop_file (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    file_path       TEXT NOT NULL UNIQUE,
    file_name       TEXT NOT NULL,
    file_size       INTEGER,
    format          TEXT,
    status          TEXT NOT NULL DEFAULT 'PENDING_REVIEW',
                    -- PENDING_REVIEW | IMPORTING | IMPORTED | FAILED
    target_library_id INTEGER REFERENCES library(id) ON DELETE SET NULL,
    matched_book_id INTEGER REFERENCES book(id) ON DELETE SET NULL,
    extracted_metadata TEXT,         -- JSON
    cover_path      TEXT,
    error_message   TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);
```

### 5.10 Email

```sql
CREATE TABLE email_provider (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    host        TEXT NOT NULL,
    port        INTEGER NOT NULL,
    username    TEXT,
    password    TEXT,
    from_address TEXT NOT NULL,
    encryption  TEXT NOT NULL DEFAULT 'STARTTLS',
                -- PLAIN | SSL | STARTTLS
    is_shared   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE email_recipient (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_id     INTEGER NOT NULL REFERENCES email_provider(id) ON DELETE CASCADE,
    recipient_email TEXT NOT NULL,
    label           TEXT
);
```

### 5.11 Metadata Jobs

```sql
CREATE TABLE metadata_fetch_jobs (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id         INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'PENDING',
                    -- PENDING | RUNNING | COMPLETED | FAILED
    error_message   TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE metadata_fetch_proposals (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id         INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL,
    proposed_data   TEXT NOT NULL,       -- JSON
    cover_url       TEXT,
    status          TEXT NOT NULL DEFAULT 'PENDING',
                    -- PENDING | ACCEPTED | REJECTED
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);
```

### 5.12 Tasks

```sql
CREATE TABLE tasks (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    task_type       TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'QUEUED',
                    -- QUEUED | RUNNING | COMPLETED | FAILED | CANCELLED
    progress        INTEGER DEFAULT 0,
    total           INTEGER DEFAULT 0,
    message         TEXT,
    error           TEXT,
    payload         TEXT,               -- JSON
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
    started_at      TEXT,
    completed_at    TEXT
);

CREATE TABLE task_cron_configuration (
    task_type   TEXT PRIMARY KEY,
    cron_expr   TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1
);
```

### 5.13 Audit Log

```sql
CREATE TABLE audit_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER REFERENCES users(id) ON DELETE SET NULL,
    username    TEXT,
    action      TEXT NOT NULL,
    resource_type TEXT,
    resource_id INTEGER,
    details     TEXT,               -- JSON
    ip_address  TEXT,
    country     TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
```

### 5.14 App Settings & Misc

```sql
CREATE TABLE app_settings (
    key     TEXT PRIMARY KEY,
    value   TEXT
);

CREATE TABLE custom_font (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,
    file_path   TEXT NOT NULL,
    format      TEXT NOT NULL,       -- TTF | OTF | WOFF | WOFF2
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE user_content_restriction (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    restriction_type TEXT NOT NULL,  -- CATEGORY | TAG | MOOD | AGE_RATING | CONTENT_RATING
    value           TEXT NOT NULL,
    mode            TEXT NOT NULL,   -- EXCLUDE | ALLOW_ONLY
    UNIQUE (user_id, restriction_type, value)
);

CREATE TABLE duplicate_dismiss (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id_a   INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    book_id_b   INTEGER NOT NULL REFERENCES book(id) ON DELETE CASCADE,
    dismissed_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (book_id_a, book_id_b)
);

CREATE TABLE hardcover_sync (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key         TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE book_vectors (
    book_id     INTEGER PRIMARY KEY REFERENCES book(id) ON DELETE CASCADE,
    vector      BLOB NOT NULL,          -- 128 x float32 = 512 bytes
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);
```

---

## 6. Authentication & Authorization

### 6.1 Local JWT Auth

- **POST /api/auth/login** — accepts `{username, password}`, returns `{accessToken, refreshToken, user}`
- **POST /api/auth/refresh** — accepts `{refreshToken}`, returns new `{accessToken, refreshToken}`
- **POST /api/auth/logout** — revokes refresh token; sends WebSocket `SESSION_REVOKED` to affected user
- Access tokens: short-lived (15 min), HS256, claims: `sub` (userId), `username`, `role`, `permissions`, `libraryIds`
- Refresh tokens: long-lived (30 days), stored hashed in `refresh_tokens` table
- Middleware: extract Bearer token from `Authorization` header, validate, inject `Principal` into context

### 6.2 OIDC

- **GET /api/auth/oidc/providers** — list configured OIDC providers
- **GET /api/auth/oidc/{provider}/authorize** — redirect to provider authorization URL
- **GET /api/auth/oidc/{provider}/callback** — handle code exchange, upsert user, issue JWT
- Store OIDC sessions in `oidc_session` table
- Support group claims → permissions via `oidc_group_mapping`
- OIDC user created on first login; password_hash is NULL

### 6.3 Remote Auth Header

Middleware (when enabled via app settings):
1. Extract username from configurable header (default: `Remote-User`)
2. Extract email from configurable header (default: `Remote-Email`)
3. Extract groups from configurable header (default: `Remote-Groups`)
4. Upsert user in DB
5. Apply group-based permissions via mapping table
6. Issue JWT internally (no login page needed)

### 6.4 Authorization Levels

| Level | Description |
|---|---|
| Public | No auth required (OPDS with Basic Auth, Kobo with token) |
| Authenticated | Valid JWT required |
| Admin | Role = ADMIN |
| Permission-gated | Specific permission bit required (canDownload, canUpload, etc.) |
| Library-scoped | User must have access to the specific library_id |

---

## 7. REST API Specification

All authenticated endpoints require `Authorization: Bearer <token>` unless noted.
Base path: `/api`

### 7.1 Auth

```
POST   /api/auth/login
POST   /api/auth/refresh
POST   /api/auth/logout
GET    /api/auth/me
PATCH  /api/auth/me/password
GET    /api/auth/oidc/providers
GET    /api/auth/oidc/{provider}/authorize
GET    /api/auth/oidc/{provider}/callback
```

### 7.2 Users (Admin)

```
GET    /api/admin/users
POST   /api/admin/users
GET    /api/admin/users/{id}
PUT    /api/admin/users/{id}
DELETE /api/admin/users/{id}
GET    /api/admin/users/{id}/permissions
PUT    /api/admin/users/{id}/permissions
GET    /api/admin/users/{id}/libraries
PUT    /api/admin/users/{id}/libraries
```

### 7.3 Libraries

```
GET    /api/libraries                        -- user's accessible libraries
POST   /api/libraries                        -- admin
GET    /api/libraries/{id}
PUT    /api/libraries/{id}                   -- admin
DELETE /api/libraries/{id}                   -- admin
POST   /api/libraries/{id}/scan             -- trigger library scan
GET    /api/libraries/{id}/metadata-sources
PUT    /api/libraries/{id}/metadata-sources
```

### 7.4 Books

```
GET    /api/books                            -- paginated, filterable
GET    /api/books/{id}
PUT    /api/books/{id}/metadata
DELETE /api/books/{id}
GET    /api/books/{id}/files
GET    /api/books/{id}/cover
PUT    /api/books/{id}/cover                 -- upload custom cover
DELETE /api/books/{id}/cover                 -- reset to extracted
GET    /api/books/{id}/similar               -- recommendations
GET    /api/books/duplicates                 -- grouped duplicates
POST   /api/books/duplicates/dismiss
POST   /api/books/merge                      -- merge books
GET    /api/books/{id}/reading-sessions
```

**Book query parameters:**
- `libraryId`, `authorId`, `seriesId`, `categoryId`, `tagId`, `moodId`
- `search` (full-text title/author)
- `format` (EPUB, PDF, etc.)
- `bookType` (EBOOK, AUDIOBOOK, COMIC)
- `sortBy` (title, addedDate, publishDate, lastRead, author)
- `sortDir` (ASC, DESC)
- `page`, `size`

### 7.5 Book Files

```
GET    /api/files/{id}/download             -- requires canDownload permission
GET    /api/files/{id}/stream               -- streaming (range requests)
GET    /api/files/{id}/progress             -- user's reading progress
PUT    /api/files/{id}/progress
```

### 7.6 Metadata

```
GET    /api/metadata/providers
POST   /api/metadata/search                 -- search all/specific providers
GET    /api/metadata/proposals              -- pending proposals for admin review
PUT    /api/metadata/proposals/{id}/accept
PUT    /api/metadata/proposals/{id}/reject
POST   /api/books/{id}/metadata/fetch       -- trigger provider fetch
POST   /api/books/{id}/metadata/field-lock  -- lock/unlock individual fields
```

### 7.7 Authors & Series

```
GET    /api/authors
GET    /api/authors/{id}
GET    /api/authors/{id}/books
PUT    /api/authors/{id}                    -- admin
GET    /api/series
GET    /api/series/{id}
GET    /api/series/{id}/books
GET    /api/categories
GET    /api/tags
GET    /api/moods
```

### 7.8 Shelves

```
GET    /api/shelves                         -- user's shelves
POST   /api/shelves
GET    /api/shelves/{id}
PUT    /api/shelves/{id}
DELETE /api/shelves/{id}
GET    /api/shelves/{id}/books
POST   /api/shelves/{id}/books              -- add book(s)
DELETE /api/shelves/{id}/books/{bookId}
GET    /api/magic-shelves
POST   /api/magic-shelves
GET    /api/magic-shelves/{id}
PUT    /api/magic-shelves/{id}
DELETE /api/magic-shelves/{id}
GET    /api/magic-shelves/{id}/books        -- evaluate rules and return books
```

### 7.9 Dashboard

```
GET    /api/dashboard                       -- user's dashboard rows (resolved books)
PUT    /api/dashboard/settings             -- save scroller config
```

### 7.10 Notebook

```
GET    /api/notebook                        -- all annotations + notes + bookmarks
GET    /api/notebook/{bookId}
GET    /api/annotations                     -- paginated
POST   /api/annotations
PUT    /api/annotations/{id}
DELETE /api/annotations/{id}
GET    /api/bookmarks
POST   /api/bookmarks
DELETE /api/bookmarks/{id}
GET    /api/books/{id}/notes
POST   /api/books/{id}/notes
PUT    /api/books/{id}/notes/{noteId}
DELETE /api/books/{id}/notes/{noteId}
GET    /api/notebook/export                 -- Markdown export
```

### 7.11 OPDS (separate auth — Basic Auth or unauthenticated)

```
GET    /opds                                -- root catalog
GET    /opds/libraries                      -- libraries feed
GET    /opds/libraries/{id}
GET    /opds/libraries/{id}/books
GET    /opds/shelves
GET    /opds/shelves/{id}
GET    /opds/series
GET    /opds/series/{id}
GET    /opds/authors
GET    /opds/authors/{id}
GET    /opds/search                         -- OpenSearch
GET    /opds/books/{id}/download/{format}
```

### 7.12 Kobo Sync

Kobo uses `X-Kobo-Token` header for auth. All endpoints mimic the Kobo store API.

```
GET    /kobo/{token}/v1/initialization
GET    /kobo/{token}/v1/library/sync
GET    /kobo/{token}/v1/library/{revisionId}/metadata
GET    /kobo/{token}/v1/library/{revisionId}/file
PUT    /kobo/{token}/v1/library/{revisionId}/state
DELETE /kobo/{token}/v1/library/{revisionId}
GET    /kobo/{token}/v1/tags
GET    /kobo/{token}/v1/tags/{tagId}/items
DELETE /kobo/{token}/v1/tags/{tagId}/items/delete
POST   /kobo/{token}/v1/analytics/gettests
GET    /kobo/{token}/v1/user/loyalty/benefits
GET    /kobo/{token}/v1/products/prices
GET    /kobo/{token}/v1/products/featured/list
GET    /kobo/{token}/v1/configuration
```

Also:
```
GET    /api/kobo/settings                   -- authenticated user's Kobo settings
PUT    /api/kobo/settings
POST   /api/kobo/token/generate
```

### 7.13 KOReader Sync

```
GET    /koreader/users/create               -- KOSync user registration
GET    /koreader/users/auth                 -- Basic Auth validation
PUT    /koreader/syncs/progress             -- update progress
GET    /koreader/syncs/progress             -- get progress
```

### 7.14 BookDrop

```
GET    /api/bookdrop/files                  -- review queue
GET    /api/bookdrop/files/{id}
POST   /api/bookdrop/files/{id}/import      -- approve and import
POST   /api/bookdrop/files/{id}/reject
POST   /api/bookdrop/bulk-import
DELETE /api/bookdrop/files/{id}
```

### 7.15 Email

```
GET    /api/email/providers                 -- admin: all providers
POST   /api/email/providers
PUT    /api/email/providers/{id}
DELETE /api/email/providers/{id}
POST   /api/email/providers/{id}/test
GET    /api/email/recipients                -- user's recipients
POST   /api/email/recipients
DELETE /api/email/recipients/{id}
POST   /api/books/{id}/send                 -- send to recipient
```

### 7.16 Tasks

```
GET    /api/tasks                           -- recent tasks
GET    /api/tasks/{id}
POST   /api/tasks/{type}/run               -- trigger named task
DELETE /api/tasks/{id}                      -- cancel
GET    /api/tasks/cron                      -- cron config
PUT    /api/tasks/cron/{type}
```

**Task types:**
- `LIBRARY_SCAN` — scan all libraries for new/changed files
- `METADATA_REFRESH` — bulk refresh metadata for all/selected books
- `COVER_REFRESH` — re-extract covers
- `BOOKDROP_SCAN` — re-scan bookdrop folder
- `DUPLICATE_DETECTION` — run duplicate detection
- `RECOMMENDATION_REBUILD` — rebuild recommendation vectors
- `FILE_ORGANIZATION` — reorganize files to naming pattern
- `AUDIT_LOG_CLEANUP` — prune old audit entries

### 7.17 Audit Log

```
GET    /api/admin/audit-logs                -- paginated, filterable
```

**Audit action types (21):**
USER_LOGIN, USER_LOGOUT, USER_CREATED, USER_UPDATED, USER_DELETED,
BOOK_DOWNLOADED, BOOK_SENT, BOOK_METADATA_UPDATED, BOOK_COVER_UPDATED,
BOOK_DELETED, LIBRARY_CREATED, LIBRARY_UPDATED, LIBRARY_DELETED,
LIBRARY_SCANNED, SHELF_CREATED, SHELF_DELETED, BOOKDROP_IMPORTED,
OPDS_ACCESS, KOBO_SYNC, KOREADER_SYNC, ADMIN_ACTION.

### 7.18 App Settings

```
GET    /api/admin/settings
PUT    /api/admin/settings
```

### 7.19 Fonts & Icons

```
GET    /api/fonts
POST   /api/fonts                           -- upload TTF/OTF/WOFF/WOFF2
DELETE /api/fonts/{id}
GET    /api/fonts/{id}/file
GET    /api/icons                           -- available icon list
```

### 7.20 User Settings

```
GET    /api/users/me/settings
PUT    /api/users/me/settings
GET    /api/users/me/content-restrictions
POST   /api/users/me/content-restrictions
DELETE /api/users/me/content-restrictions/{id}
GET    /api/users/me/reading-stats          -- total read time, books read, etc.
```

---

## 8. WebSocket Events

**Endpoint**: `ws://host/ws`
**Auth**: JWT token sent as query param `?token=<accessToken>` or in `Authorization` header on upgrade.

All messages are JSON with structure:
```json
{
  "type": "EVENT_TYPE",
  "payload": { ... }
}
```

### Server → Client Events

| Event Type | Payload | Description |
|---|---|---|
| `TASK_PROGRESS` | `{taskId, taskType, status, progress, total, message}` | Background task progress update |
| `TASK_COMPLETE` | `{taskId, taskType, result}` | Task finished |
| `TASK_FAILED` | `{taskId, taskType, error}` | Task failed |
| `LIBRARY_SCAN_COMPLETE` | `{libraryId, booksAdded, booksUpdated, booksRemoved}` | After scan |
| `BOOK_ADDED` | `{book}` | New book discovered |
| `BOOK_UPDATED` | `{bookId}` | Book metadata changed |
| `BOOK_DELETED` | `{bookId}` | Book removed |
| `BOOKDROP_FILE_ARRIVED` | `{bookdropFileId, fileName}` | New file in bookdrop |
| `METADATA_PROPOSAL_READY` | `{bookId, proposalCount}` | Proposals ready for review |
| `SESSION_REVOKED` | `{userId}` | User session invalidated (logout) |
| `NOTIFICATION` | `{level, title, message}` | General notification |

### Client → Server Events

| Event Type | Payload | Description |
|---|---|---|
| `PING` | `{}` | Keepalive |

---

## 9. File Storage & Cover Management

### 9.1 Directory Layout (data volume)

```
/app/data/
├── lexicon.db                    -- SQLite database file
├── covers/
│   ├── books/
│   │   ├── {bookId}/
│   │   │   ├── cover.jpg        -- full size (max 800x1200)
│   │   │   └── thumbnail.jpg    -- thumbnail (200x300)
│   │   └── ...
│   ├── authors/
│   │   ├── {authorId}.jpg
│   │   └── ...
│   └── bookdrop/
│       └── {bookdropFileId}.jpg -- temporary cover during review
├── tools/
│   ├── kepubify                 -- downloaded at startup
│   └── ffprobe                  -- downloaded at startup
├── fonts/
│   └── {fontId}.{ext}
└── cache/
    ├── comics/
    │   └── {fileId}/            -- extracted comic pages
    └── kepub/
        └── {fileId}.kepub.epub  -- converted KEPUB files
```

### 9.2 Cover Extraction Logic

Per format:
- **EPUB**: unzip, find OPF manifest, locate cover image (cover-image role or cover.jpg/png), decode, resize
- **PDF**: extract first page as image via `pdfcpu`
- **CBZ**: unzip, first image file alphabetically
- **CBR/CB7**: extract via `mholt/archives` (pure Go), first image
- **MOBI/AZW3**: parse EXTH header for embedded cover
- **M4B/M4A/MP3**: ID3/MP4 tag embedded artwork via `dhowden/tag`
- **FB2**: base64-encoded image in `<binary id="cover.jpg">` element
- **Fallback**: no cover (use placeholder in UI)

**Processing:**
1. Extract raw cover bytes
2. Decode image (JPEG, PNG, GIF, WEBP accepted)
3. Protect against decompression bombs: reject if decoded > 50 MP
4. Resize to full (max 800px wide, maintain aspect) and thumbnail (200x300, crop-fit)
5. Save as JPEG (quality 85) to covers directory
6. Audiobook covers: square crop (max 600x600) for uniform grid display

### 9.3 File Fingerprinting

- Read first 64KB + last 64KB of file
- MD5 hash of those bytes
- Used for duplicate detection and KOReader document matching
- Stored in `book_file.fingerprint`

### 9.4 File Organization

Token-based naming patterns. Example: `{author}/{series}/{series_number} - {title}`

**Supported tokens:**
`{title}`, `{original_title}`, `{author}`, `{author_last_first}`, `{series}`, `{series_number}`, `{year}`, `{publisher}`, `{isbn}`, `{language}`, `{format}`

**Modifiers** (pipe-separated): `lower`, `upper`, `ascii` (transliterate), `sanitize` (remove FS-unsafe chars)
Example: `{author|lower|ascii}/{title|sanitize}`

**Rename process:**
1. Compute new path from pattern + book metadata
2. Sanitize each path segment (remove `\/:*?"<>|` and control chars)
3. Check for collision with existing file at new path
4. Atomic rename: write to temp path, then rename

---

## 10. Library & Book Management

### 10.1 Library Scan

1. For each library_path in the library:
   - Walk directory tree
   - For each file with a supported extension:
     - Compute fingerprint
     - Check if book_file exists by path or fingerprint
     - If new: extract metadata (embedded), optionally fetch from provider, create book + book_file records
     - If changed (size/mtime different): update fingerprint, optionally re-extract metadata
     - If moved: update file_path (matched by fingerprint)
   - Mark book_files not found on disk as missing (don't delete immediately)
2. Organization mode:
   - `BOOK_PER_FILE`: each file is its own book record
   - `BOOK_PER_FOLDER`: files in same directory are grouped as one book with multiple files
3. Audiobook detection: folder contains only audio files (M4B, M4A, MP3, OPUS) → `book_type = AUDIOBOOK`
4. Comic detection: CBZ/CBR/CB7 → `book_type = COMIC`
5. After scan: emit `LIBRARY_SCAN_COMPLETE` WebSocket event
6. File watching: fsnotify watch on all library_paths; debounce 5s, then re-scan affected directory

### 10.2 Metadata Extraction (Embedded)

**EPUB**: Parse OPF file:
- title, creator (authors), description, publisher, date, language, ISBN (dc:identifier)
- Series from calibre:series / OPF meta tags

**PDF**: XMP/DocInfo:
- Title, Author, Subject, Keywords, Creator, Producer, CreationDate

**CBZ**: ComicInfo.xml inside zip:
- Title, Series, Number, Year, Month, Writer, Publisher, Genre, Characters, Teams, Locations, StoryArc, AgeRating, BlackAndWhite, Manga

**MP3**: ID3 tags via dhowden/tag:
- Title, Artist/AlbumArtist, Album (= book title), TrackNumber, Year

**M4B/M4A**: MP4 tags:
- Title, Artist, AlbumArtist, Album, Genre, Track, Year, Description, Chapters

---

## 11. Metadata Providers

### Provider Interface

```go
// Defined in the consumer (metadata service), not in the provider package.
// Each provider implements this interface.
type MetadataProvider interface {
    Name() string
    Search(ctx context.Context, query MetadataQuery) ([]MetadataResult, error)
    FetchByID(ctx context.Context, id string) (*MetadataResult, error)
    SupportedFields() []MetadataField
}
```

### 11.1 Google Books

- API: `https://www.googleapis.com/books/v1/volumes`
- Query: `?q=intitle:{title}+inauthor:{author}` or `?q=isbn:{isbn}`
- Optional API key via app settings
- Parse volumeInfo for all fields
- CoverURL: `imageLinks.thumbnail` (replace `zoom=1` → `zoom=0`, `&edge=curl` → ``)

### 11.2 Amazon

- Scrape `https://www.amazon.com/s?k={isbn_or_title}`
- Parse product page HTML for title, authors, description, publisher, date, ASIN
- Extract cover from product image
- Respect robots.txt; use rotating user-agents
- Rate limit: 1 req/2s

### 11.3 GoodReads

- Scrape `https://www.goodreads.com/search?q={title}+{author}`
- Parse search results and book detail pages
- Extract rating, genres, description, series info
- Rate limit: 1 req/1s

### 11.4 Hardcover

- GraphQL API: `https://api.hardcover.app/v1/graphql`
- API key via app settings
- Supports: title, authors, series, description, cover, genres, ISBN

### 11.5 Audible

- Scrape `https://www.audible.com/search?keywords={title}+{author}`
- Extract ASIN, title, narrator, author, publisher, description, cover, release date, duration
- For audiobooks specifically

### 11.6 Comicvine

- REST API: `https://comicvine.gamespot.com/api/`
- API key required (free registration)
- Fields: title, issue number, volume, publisher, cover date, description, cover, characters, teams, locations, story arcs

### 11.7 Douban

- Scrape `https://book.douban.com/subject_search?search_text={query}`
- Chinese-language books primarily

### 11.8 LubimyCzytac

- Scrape `https://lubimyczytac.pl/szukaj/ksiazki?phrase={query}`
- Polish-language books

### 11.9 RanobeDB

- REST API: `https://ranobedb.org/api/v0/`
- Light novels primarily, open API

### 11.10 Per-Field Priority Matrix

Each library has a `library_metadata_source` config. When multiple providers are configured, fields are merged according to priority order. Locked fields are never overwritten.

---

## 12. Reader Support

### 12.1 PDF Reader

- **Frontend**: `pdfjs-dist` (Mozilla)
- **Backend**: serve `book_file` via range-request-capable streaming endpoint
- **Progress**: store current page number as integer string
- **Annotations**: `pdf_annotations` table; store per-page annotation JSON
- Settings stored per-user in `user_settings.pdf_reader_setting` JSON

### 12.2 EPUB Reader

- **Frontend**: `epubjs`
- **Backend**: serve EPUB file, or serve individual spine items
- **Progress**: CFI string
- **Highlights**: stored in `annotations` table with `cfi`, `highlighted_text`, `color`, optional `note`
- **Bookmarks**: stored in `book_marks` with `cfi`, `label`
- **Notes**: stored in `book_notes` (book-level, no CFI)
- Settings per-user in `user_settings.epub_reader_setting` JSON

### 12.3 Comic Reader

- **Frontend**: custom canvas renderer
- **Backend**: serve individual pages extracted from CBZ/CBR/CB7
  - `GET /api/files/{id}/comic/pages` — list pages with dimensions
  - `GET /api/files/{id}/comic/page/{n}` — serve page image
- **Progress**: page number as integer string
- Page cache: pre-extract and cache all pages on first open
- Settings per-user in `user_settings.comic_reader_setting` JSON

### 12.4 Audiobook Player

- **Frontend**: HTML5 `<audio>` with Media Session API
- **Backend**:
  - `GET /api/books/{id}/audiobook` — book structure (chapters or tracks list with durations)
  - Stream audio files via range-request endpoint
- **Chapter-based (M4B)**: extract chapters from M4B container via ffprobe
- **Track-based (folder)**: book_file records ordered by `track_number`
- **Progress**: seconds elapsed as string
- **Sleep timer**: frontend-only countdown
- Settings per-user in `user_settings.audiobook_reader_setting` JSON

---

## 13. Shelves & Magic Shelves

### 13.1 Manual Shelves

- User-owned collections
- Icon + color picker (stored as icon name string + hex color)
- Many-to-many via `book_shelf_mapping`

### 13.2 Magic Shelves

Rule evaluation engine in `internal/shelf/magic.go`:

Evaluation: Build SQL WHERE clause dynamically from rules JSON, execute against `book` + joined tables. Return paginated results.

**Implementation approach**: translate rules to a parameterized SQL query builder. Do NOT use `eval` or dynamic code — use a switch on field names and operators to produce safe SQL fragments.

---

## 14. Device Sync Integrations

### 14.1 OPDS 1.2

- Atom XML feeds
- Basic Auth (dedicated `opds_user` credentials, separate from main users)
- Root catalog links to libraries, shelves, series, authors, recent
- OpenSearch description at `/opds/search`
- Each book entry includes acquisition links for all available formats

### 14.2 Kobo Sync

**Architecture**: Lexicon acts as a full Kobo store API proxy. Kobo device is configured with a custom store URL pointing to Lexicon.

**Snapshot-based sync:**
- `kobo_library_snapshot` stores one row per user per book
- On sync: compare current book list vs snapshot, compute added/removed/changed
- Return additions as new `BookEntitlement`, removals as `DeletedEntitlement`

**KEPUB conversion:**
- If enabled and file is EPUB: run `kepubify` and cache the result

### 14.3 KOReader Sync

Implements the KOSync protocol.

**Document matching**: `document` field is the file MD5/fingerprint. Match to `book_file.fingerprint`.

### 14.4 Hardcover Sync

- When book progress is updated: if Hardcover sync enabled for user, forward to Hardcover API
- Match books by `book_metadata.hardcover_id`

---

## 15. Background Tasks

### 15.1 Task Runner

- Each task runs in a goroutine with `context.WithCancel`
- Progress updates sent via WebSocket hub
- At most 1 running instance of each task type
- On startup: mark any RUNNING tasks as FAILED (they were interrupted)

### 15.2 Task Types

| TaskType | Description |
|---|---|
| `LIBRARY_SCAN` | Scan all libraries; payload: `{libraryId?: number}` |
| `METADATA_REFRESH` | Batch fetch from providers |
| `COVER_REFRESH` | Re-extract covers |
| `BOOKDROP_SCAN` | Re-scan bookdrop folder |
| `DUPLICATE_DETECTION` | Run dedup algorithm |
| `RECOMMENDATION_REBUILD` | Rebuild all book vectors |
| `FILE_ORGANIZATION` | Rename files to pattern |
| `AUDIT_LOG_CLEANUP` | Delete entries older than N days |

### 15.3 Cron Configuration

Default cron schedules (stored in `task_cron_configuration`, overridable via UI):

| Task | Default Cron |
|---|---|
| LIBRARY_SCAN | `0 */6 * * *` (every 6 hours) |
| METADATA_REFRESH | disabled by default |
| COVER_REFRESH | disabled by default |
| DUPLICATE_DETECTION | `0 2 * * 0` (Sundays 2am) |
| RECOMMENDATION_REBUILD | `0 3 * * *` (daily 3am) |
| AUDIT_LOG_CLEANUP | `0 1 * * *` (daily 1am) |

---

## 16. Email / Send-to-Device

### 16.1 Provider Configuration

Multiple email providers can be configured (admin). A provider can be marked `is_shared` to be available to all users.

### 16.2 Send Flow

1. User selects book + file format + recipient
2. Backend fetches file from disk
3. Construct MIME email with file as attachment
4. Connect to SMTP provider (detect encryption: SSL/STARTTLS/PLAIN)
5. Send email
6. Audit log entry: `BOOK_SENT`

---

## 17. BookDrop

### 17.1 Folder Watching

1. On startup: fsnotify watch on `BOOKDROP_PATH`
2. On file event: wait for stability (poll size every 2s, 3 identical readings)
3. Create `bookdrop_file` record, extract metadata, fetch cover
4. Emit `BOOKDROP_FILE_ARRIVED` WebSocket event

### 17.2 Import

1. User selects target library
2. Backend copies file to library path
3. Creates book + book_file records
4. Fetches full metadata from providers
5. Updates status to IMPORTED

---

## 18. Dashboard

### 18.1 Scroller Rows

Up to 5 configurable rows per user. Stored in `user_settings.dashboard_setting` JSON.

### 18.2 Row Types

| Type | Logic |
|---|---|
| `LAST_READ` | Books with recent progress updates, limit 20 |
| `LAST_LISTENED` | Same but audiobooks only |
| `LATEST_ADDED` | By added_date DESC, limit 20 |
| `RANDOM` | Random selection, refreshed hourly |
| `MAGIC_SHELF` | Evaluate magic shelf rules, first 20 |

---

## 19. Notebook (Annotations)

### 19.1 Unified View

Returns all of the authenticated user's EPUB highlights, bookmarks, book notes, and PDF annotations, grouped by book.

### 19.2 Markdown Export

Generate Markdown document with headings per book, quoted highlights, inline notes, bookmark list.

---

## 20. Recommendations

### 20.1 Feature Vector

For each book, compute a 128-dimensional float32 feature vector using feature hashing (FNV-1a). Weighted by: authors (0.30), series (0.20), categories (0.20), tags (0.15), language (0.10), publisher (0.05). L2-normalize.

### 20.2 Similarity

- Top-N by cosine similarity
- Cap 3 books per author
- Vectors stored as BLOB in `book_vectors` table
- Rebuilt by `RECOMMENDATION_REBUILD` task

---

## 21. Audit Logs

- Every significant action creates an audit log entry
- IP address from `X-Forwarded-For` or `RemoteAddr`
- Admin paginated view: filterable by action, user, date range
- Cleanup task: delete entries older than configurable days

---

## 22. Content Restrictions

Per-user filters applied to all book queries:
- **EXCLUDE**: books matching the restriction value are hidden
- **ALLOW_ONLY**: only books matching the restriction value are shown

Applied as additional WHERE clauses in all book listing queries.

---

## 23. SolidJS Frontend Specification

### 23.1 Routing Structure

```
/login
/auth/oidc/callback
/
  /dashboard
  /libraries
  /libraries/{id}/books
  /books/{id}
  /books/{id}/read
  /books/{id}/read/pdf
  /books/{id}/read/epub
  /books/{id}/read/comic
  /books/{id}/read/audio
  /authors
  /authors/{id}
  /series
  /series/{id}
  /shelves
  /shelves/{id}
  /magic-shelves
  /magic-shelves/{id}
  /notebook
  /notebook/{bookId}
  /bookdrop                       -- admin
  /tasks                          -- admin
  /settings                       -- user settings
  /admin
    /users
    /libraries
    /email
    /opds
    /kobo
    /koreader
    /metadata-sources
    /audit-logs
    /app-settings
    /fonts
    /recommendations
```

### 23.2 Frontend Organization

```
web/src/
├── features/
│   ├── auth/
│   │   ├── AuthProvider.tsx      -- context + signals
│   │   ├── LoginPage.tsx
│   │   └── ProtectedRoute.tsx
│   ├── library/
│   │   ├── LibraryList.tsx
│   │   ├── LibraryBrowser.tsx
│   │   └── BookGrid.tsx
│   ├── book/
│   │   ├── BookDetail.tsx
│   │   └── BookCard.tsx
│   ├── reader/
│   │   ├── EpubReader.tsx
│   │   ├── PdfReader.tsx
│   │   ├── ComicReader.tsx
│   │   └── AudioPlayer.tsx
│   ├── shelf/
│   │   ├── ShelfList.tsx
│   │   ├── ShelfDetail.tsx
│   │   └── MagicShelfBuilder.tsx
│   ├── dashboard/
│   │   ├── Dashboard.tsx
│   │   └── ScrollerRow.tsx
│   ├── notebook/
│   │   ├── NotebookPage.tsx
│   │   └── AnnotationList.tsx
│   ├── admin/
│   │   ├── UserManagement.tsx
│   │   ├── AuditLogs.tsx
│   │   ├── AppSettings.tsx
│   │   └── ...
│   └── bookdrop/
│       └── BookDropQueue.tsx
├── shared/
│   ├── ui/
│   │   ├── Button.tsx
│   │   ├── Input.tsx
│   │   ├── Modal.tsx
│   │   ├── Toast.tsx
│   │   └── ...
│   ├── api/
│   │   └── client.ts             -- typed fetch wrapper
│   ├── ws/
│   │   └── socket.ts             -- reconnecting WebSocket
│   └── i18n/
│       ├── i18n.ts               -- signal-based i18n system
│       └── en.json               -- English locale
├── App.tsx
├── index.tsx
└── index.css                     -- Tailwind imports + global styles
```

### 23.3 State Management

SolidJS signals and context only. No external state library.

### 23.4 Theme

- Dark mode first (homelab users expect dark by default)
- Accent color configurable per user
- CSS custom properties for theming via Tailwind
- Responsive: sidebar collapses to bottom nav on mobile

---

## 24. Docker Deployment Specification

### 24.1 Dockerfile

```dockerfile
# Stage 1: Build SolidJS frontend
FROM node:22-alpine AS frontend-builder
WORKDIR /app/web
COPY web/package*.json .
RUN npm ci
COPY web/ .
RUN npm run build

# Stage 2: Build Go backend
FROM golang:1.23-alpine AS backend-builder
WORKDIR /app
COPY go.mod go.sum .
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o lexicon ./cmd/lexicon

# Stage 3: Runtime
FROM alpine:3.20
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    su-exec

COPY --from=backend-builder /app/lexicon /usr/local/bin/lexicon

EXPOSE 6060
VOLUME ["/app/data", "/books", "/bookdrop"]

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
CMD ["lexicon"]
```

### 24.2 docker-compose.yml

```yaml
services:
  lexicon:
    image: ghcr.io/crueber/lexicon:latest
    ports:
      - "6060:6060"
    environment:
      JWT_SECRET: changeme-use-a-long-random-string
      TZ: America/New_York
      USER_ID: "1000"
      GROUP_ID: "1000"
    volumes:
      - lexicon-data:/app/data
      - /path/to/your/books:/books:ro
      - /path/to/bookdrop:/bookdrop
    restart: unless-stopped

volumes:
  lexicon-data:
```

Note: No separate database container needed. SQLite database is stored in the data volume at `/app/data/lexicon.db`.

---

## 25. Configuration & Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `JWT_SECRET` | Yes | — | HS256 signing secret (min 32 chars) |
| `PORT` | No | `6060` | HTTP listen port |
| `DATA_DIR` | No | `/app/data` | Path for database, covers, cache, tools |
| `BOOKS_DIR` | No | `/books` | Default books root (overridden per-library) |
| `BOOKDROP_DIR` | No | `/bookdrop` | BookDrop watch folder |
| `TZ` | No | `UTC` | Timezone for cron jobs and display |
| `USER_ID` | No | `1000` | UID to run as (entrypoint) |
| `GROUP_ID` | No | `1000` | GID to run as (entrypoint) |
| `DISK_TYPE` | No | `LOCAL` | `LOCAL` or `NETWORK` (affects fsnotify behavior) |
| `LOG_LEVEL` | No | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | No | `json` | `json` or `text` |
| `MAX_UPLOAD_SIZE_MB` | No | `500` | Max file upload size |

---

## 26. Application Settings (Runtime)

Stored in `app_settings` table, editable via admin UI at runtime (no restart needed):

| Key | Type | Default | Description |
|---|---|---|---|
| `ALLOW_REGISTRATION` | bool | `true` | Allow new users to self-register |
| `DEFAULT_ROLE` | string | `USER` | Role assigned to new registrations |
| `OIDC_ENABLED` | bool | `false` | Enable OIDC authentication |
| `OIDC_PROVIDER_NAME` | string | — | Display name for OIDC button |
| `OIDC_CLIENT_ID` | string | — | OIDC client ID |
| `OIDC_CLIENT_SECRET` | string | — | OIDC client secret |
| `OIDC_ISSUER_URI` | string | — | OIDC provider issuer URL |
| `OIDC_SCOPE` | string | `openid email profile` | OIDC scopes |
| `OIDC_GROUP_CLAIM` | string | `groups` | JWT claim containing group membership |
| `REMOTE_AUTH_ENABLED` | bool | `false` | Enable Remote Auth header SSO |
| `REMOTE_AUTH_USER_HEADER` | string | `Remote-User` | Header for username |
| `REMOTE_AUTH_EMAIL_HEADER` | string | `Remote-Email` | Header for email |
| `REMOTE_AUTH_GROUPS_HEADER` | string | `Remote-Groups` | Header for groups |
| `OPDS_ENABLED` | bool | `true` | Enable OPDS catalog |
| `KOBO_SYNC_ENABLED` | bool | `true` | Enable Kobo sync |
| `KOREADER_SYNC_ENABLED` | bool | `true` | Enable KOReader sync |
| `BOOKDROP_ENABLED` | bool | `false` | Enable BookDrop folder |
| `EMAIL_ENABLED` | bool | `false` | Enable email send-to-device |
| `COVER_IMAGE_MAX_SIZE_MB` | int | `10` | Max cover image size |
| `DUPLICATE_DETECTION_PRESET` | string | `MODERATE` | STRICT / MODERATE / LOOSE / TITLE_ONLY |
| `AUDIT_LOG_RETENTION_DAYS` | int | `365` | Days to keep audit log entries |
| `HARDCOVER_ENABLED` | bool | `false` | Enable Hardcover sync globally |
| `RECOMMENDATION_ENABLED` | bool | `true` | Enable recommendation engine |

---

## 27. Implementation Phases

Each phase is designed to be independently implementable by a subagent with clear inputs, outputs, and verification criteria. Phases are ordered by dependency — later phases build on earlier ones.

**Parallelization notes**: Some phases can run concurrently when they don't share dependencies. These are noted where applicable.

---

### Phase 01: Project Scaffold & Build Pipeline

**Goal**: Working build pipeline that produces a single binary serving a "Hello Lexicon" page.

**Changes**:
- [x] `go.mod` with module `github.com/crueber/lexicon`
- [x] `cmd/lexicon/main.go` — thin entry point with `run()` pattern
- [x] `internal/server/server.go` — chi router, health check endpoint
- [x] `Makefile` — targets: `build`, `run`, `test`, `lint`, `sqlc-generate`
- [x] `web/` — Vite + SolidJS + TypeScript + Tailwind scaffold
- [x] `web/src/App.tsx` — "Hello Lexicon" page
- [x] `Dockerfile` — multi-stage build
- [x] `go:embed` for serving frontend dist in production
- [x] Dev mode: proxy Vite dev server

**Verification**:
- [x] `make build` succeeds
- [ ] `make run` starts server on :6060
- [ ] `curl http://localhost:6060/health` returns 200
- [ ] Browser shows "Hello Lexicon" page
- [ ] `docker build .` succeeds

---

### Phase 02: Database Foundation

**Goal**: SQLite database with WAL mode, migration system, and first tables.

**Dependencies**: Phase 01

**Changes**:
- [x] `internal/server/` — SQLite connection setup with WAL mode, busy timeout, foreign keys
- [x] `sqlc.yaml` configuration for SQLite
- [x] `migrations/001_users.up.sql` — users, user_permissions, user_settings, refresh_tokens, app_settings tables
- [x] `migrations/001_users.down.sql`
- [x] First sqlc query files for user CRUD
- [x] Database initialization on startup (run migrations)

**Verification**:
- [x] Server starts and creates `lexicon.db` in data dir
- [x] Migrations apply cleanly
- [x] `sqlc generate` produces valid Go code
- [x] `go test ./...` passes

---

### Phase 03: Authentication (Local JWT)

**Goal**: Working login/logout/refresh flow with JWT middleware.

**Dependencies**: Phase 02

**Changes**:
- [x] `internal/auth/jwt.go` — JWT issue/validate/refresh using `golang-jwt`
- [x] `internal/auth/middleware.go` — chi middleware: extract Bearer token, validate, inject Principal into context
- [x] `internal/auth/handler.go` — POST /api/auth/login, /api/auth/refresh, /api/auth/logout, GET /api/auth/me
- [x] `internal/auth/types.go` — Principal, Claims structs
- [x] `internal/user/service.go` — user lookup, password verification (bcrypt)
- [x] First-run setup: create default admin user if no users exist
- [x] `internal/server/routes.go` — register auth routes

**Verification**:
- [x] POST /api/auth/login with valid credentials returns JWT + refresh token
- [x] GET /api/auth/me with valid JWT returns user info
- [x] GET /api/auth/me without JWT returns 401
- [x] POST /api/auth/refresh rotates tokens
- [x] POST /api/auth/logout revokes refresh token
- [x] Default admin user created on first run

---

### Phase 04: Frontend Auth Shell

**Goal**: Login page, auth context, protected routes, basic layout shell.

**Dependencies**: Phase 03

**Changes**:
- [x] `web/src/shared/api/client.ts` — typed fetch wrapper with auth header injection, token refresh
- [x] `web/src/features/auth/AuthProvider.tsx` — auth context with signals (user, login, logout, isAdmin)
- [x] `web/src/features/auth/LoginPage.tsx` — login form
- [x] `web/src/features/auth/ProtectedRoute.tsx` — redirect to /login if not authenticated
- [x] `web/src/shared/ui/` — Button, Input components (Kobalte-based)
- [x] App layout shell: sidebar navigation + main content area
- [x] `web/src/App.tsx` — router setup with protected routes
- [x] Dashboard stub page (placeholder)

**Verification**:
- [ ] Browser shows login page at /login
- [ ] Can log in with default admin credentials
- [ ] Redirected to dashboard stub after login
- [ ] Sidebar navigation visible
- [ ] Unauthenticated access redirects to /login
- [ ] Token refresh works transparently

---

### Phase 05: Library & Book Data Model

**Goal**: All book-related database tables and sqlc queries.

**Dependencies**: Phase 02

**Parallelizable with**: Phase 03, Phase 04

**Changes**:
- `migrations/002_libraries.up.sql` — library, library_path, library_metadata_source tables
- `migrations/003_books.up.sql` — book, book_file, book_metadata, comic_metadata tables
- `migrations/004_taxonomy.up.sql` — author, series, category, tag, mood + all junction tables
- `migrations/005_progress.up.sql` — user_book_file_progress, reading_sessions tables
- sqlc query files for all book-related CRUD operations
- `internal/book/queries.sql`, `internal/library/queries.sql`

**Verification**:
- All migrations apply cleanly (up and down)
- `sqlc generate` produces valid Go code
- `go build ./...` succeeds
- `go test ./...` passes

---

### Phase 06: Library Management API

**Goal**: CRUD endpoints for libraries with user access control.

**Dependencies**: Phase 03, Phase 05

**Changes**:
- [x] `internal/library/handler.go` — GET/POST/PUT/DELETE /api/libraries, library path management
- [x] `internal/library/service.go` — business logic, user-library permission filtering
- [x] `internal/library/queries.sql` — library queries
- [x] Route registration in `internal/server/routes.go`
- [x] Admin-only guards on create/update/delete

**Verification**:
- [x] Admin can create a library with paths via API
- [x] Non-admin users only see libraries they have access to
- [x] Library CRUD operations work correctly
- [x] `go test ./...` passes

---

### Phase 07: File Scanner & Fingerprinting

**Goal**: Scan library directories, create book/file records, compute fingerprints.

**Dependencies**: Phase 06

**Changes**:
- [x] `internal/library/scanner.go` — directory walker, supported extension detection
- [x] `internal/storage/fingerprint.go` — first 64KB + last 64KB → MD5
- [x] Book/book_file record creation logic
- [x] BOOK_PER_FILE and BOOK_PER_FOLDER organization modes
- [x] Audiobook folder detection (all audio files → AUDIOBOOK type)
- [x] Comic detection (CBZ/CBR/CB7 → COMIC type)
- [x] POST /api/libraries/{id}/scan endpoint (synchronous for now, task system comes later)

**Verification**:
- [x] Create a library pointing to a directory with test ebook files
- [x] Trigger scan via API
- [x] Books appear in database with correct types
- [x] Fingerprints computed and stored
- [x] BOOK_PER_FOLDER mode groups files correctly
- [x] `go test ./...` passes (with test fixtures)

---

### Phase 08: Cover Extraction & Thumbnails

**Goal**: Extract covers from all supported formats, generate thumbnails, serve via API.

**Dependencies**: Phase 07

**Changes**:
- [x] `internal/storage/cover.go` — cover extraction per format (EPUB, PDF, CBZ, audio tags)
- [x] Thumbnail generation (200x300 crop-fit) using `disintegration/imaging`
- [x] Full-size cover (max 800px wide)
- [x] Decompression bomb protection (reject > 50MP)
- [x] `internal/storage/handler.go` — GET /api/books/{id}/cover (serve cover image)
- [x] Cover extraction integrated into scanner flow
- [x] Data directory structure: `/app/data/covers/books/{bookId}/`

**Verification**:
- [x] Scan a library with EPUB, PDF, CBZ files
- [x] Covers extracted and saved as JPEG
- [x] Thumbnails generated at correct dimensions
- [x] GET /api/books/{id}/cover returns the image
- [x] Books without covers return 404 (placeholder handled by frontend)
- [x] `go test ./...` passes

---

### Phase 09: Embedded Metadata Extraction

**Goal**: Parse metadata from book files during scan.

**Dependencies**: Phase 07

**Parallelizable with**: Phase 08

**Changes**:
- [x] EPUB OPF parsing: title, authors, description, publisher, date, language, ISBN, series
- [x] PDF XMP/DocInfo parsing: title, author, subject, keywords
- [x] CBZ ComicInfo.xml parsing: full comic metadata
- [x] Audio tag parsing (ID3, MP4): title, artist, album, track, year
- [x] Author/series/category record creation and linking
- [x] Metadata populated in `book_metadata` during scan

**Verification**:
- [x] Scan EPUBs → title, authors, description populated
- [x] Scan PDFs → title, author populated
- [x] Scan CBZ with ComicInfo.xml → comic metadata populated
- [x] Scan audio files → audiobook metadata populated
- [x] Author and series records created and linked
- [x] `go test ./...` passes

---

### Phase 10: Library Browser Frontend

**Goal**: Browse libraries and books in the UI.

**Dependencies**: Phase 04, Phase 06, Phase 08

**Changes**:
- [x] `web/src/features/library/LibraryList.tsx` — list of user's libraries
- [x] `web/src/features/library/LibraryBrowser.tsx` — book grid for a library
- [x] `web/src/features/book/BookCard.tsx` — cover image + title + author
- [x] Book grid with lazy-loaded cover images
- [x] Filter controls: format, book type
- [x] Sort controls: title, added date, author
- [x] Pagination
- [x] Route: /libraries, /libraries/{id}/books

**Verification**:
- [ ] Libraries page shows all accessible libraries
- [ ] Clicking a library shows book grid with covers
- [ ] Filters narrow results correctly
- [ ] Sort changes order
- [ ] Pagination works
- [ ] Covers load lazily

---

### Phase 11: Book Detail Page

**Goal**: Full book detail view with metadata display.

**Dependencies**: Phase 10

**Changes**:
- [x] Book detail API: GET /api/books/{id} — returns full metadata, files, authors, series, categories, tags
- [x] `web/src/features/book/BookDetail.tsx` — full detail page
- [x] Large cover display
- [x] Metadata fields: title, authors (linked), series, publisher, date, language, ISBN, page count
- [x] Truncatable description
- [x] File list with format badges
- [x] Route: /books/{id}

**Verification**:
- [ ] Book detail page shows all metadata
- [ ] Authors and series are clickable links
- [ ] File list shows all associated files with formats
- [ ] Cover displays correctly
- [ ] Description truncates with "show more"

---

### Phase 12: File Watching & WebSocket Events

**Goal**: Live filesystem watching and real-time UI updates via WebSocket.

**Dependencies**: Phase 07, Phase 04

**Changes**:
- [x] `internal/ws/hub.go` — WebSocket connection hub (register, unregister, broadcast)
- [x] `internal/ws/handler.go` — /ws upgrade endpoint with JWT auth
- [x] `internal/library/watcher.go` — fsnotify watch on all library paths, debounce 5s
- [x] `web/src/shared/ws/socket.ts` — reconnecting WebSocket client
- [x] WebSocket events: BOOK_ADDED, BOOK_UPDATED, BOOK_DELETED, LIBRARY_SCAN_COMPLETE
- [x] Frontend reacts to WebSocket events (update book store)

**Verification**:
- [ ] WebSocket connects with valid JWT
- [ ] Add a file to a watched library directory
- [ ] After debounce, file is scanned
- [ ] BOOK_ADDED event received by frontend
- [ ] UI updates without page refresh
- [ ] WebSocket reconnects after disconnect

---

### Phase 13: Background Task System

**Goal**: Goroutine-based task runner with progress tracking and cron scheduling.

**Dependencies**: Phase 12

**Changes**:
- [x] `migrations/006_tasks.up.sql` — tasks, task_cron_configuration tables
- [x] `internal/task/runner.go` — task execution with context cancellation, progress reporting
- [x] `internal/task/scheduler.go` — robfig/cron integration
- [x] `internal/task/handler.go` — GET /api/tasks, POST /api/tasks/{type}/run, DELETE /api/tasks/{id}
- [x] `internal/task/types.go` — task type definitions
- [x] LIBRARY_SCAN converted to a background task
- [x] TASK_PROGRESS/COMPLETE/FAILED WebSocket events
- [x] On startup: mark interrupted RUNNING tasks as FAILED

**Verification**:
- [ ] Trigger LIBRARY_SCAN task via API
- [ ] Task progress events received via WebSocket
- [ ] Task completes and status updated in DB
- [ ] Can cancel a running task
- [ ] Cron schedules fire correctly
- [ ] Only one instance of each task type runs at a time
- [x] `go test ./...` passes

---

### Phase 14: EPUB Reader

**Goal**: In-browser EPUB reading with progress persistence.

**Dependencies**: Phase 11

**Changes**:
- [x] `internal/reader/handler.go` — file streaming endpoint (range requests), progress save/load
- [x] `web/src/features/reader/EpubReader.tsx` — epub.js integration
- [x] Full-screen reading mode
- [x] Top bar: book title, chapter title, progress %
- [x] Bottom toolbar: font settings, theme, TOC, bookmarks panel
- [x] CFI-based progress auto-save
- [x] Settings panel: font, size, line height, margins, flow, theme
- [x] Settings stored in user_settings.epub_reader_setting
- [x] Route: /books/{id}/read/epub

**Verification**:
- [ ] Open an EPUB book in the reader
- [ ] Navigate between chapters
- [ ] Close and reopen — resumes at same position (CFI)
- [ ] Font/theme settings persist
- [ ] TOC navigation works
- [ ] Full-screen mode works

---

### Phase 15: PDF Reader

**Goal**: In-browser PDF reading with progress persistence.

**Dependencies**: Phase 11

**Parallelizable with**: Phase 14

**Changes**:
- [x] `web/src/features/reader/PdfReader.tsx` — pdfjs-dist integration
- [x] Page-based progress save/restore
- [x] Sidebar: thumbnails, TOC, search
- [x] Settings: spread mode, scroll mode, zoom level
- [x] Settings stored in user_settings.pdf_reader_setting
- [x] Route: /books/{id}/read/pdf

**Verification**:
- [ ] Open a PDF book in the reader
- [ ] Navigate between pages
- [ ] Close and reopen — resumes at same page
- [ ] Sidebar thumbnails and TOC work
- [ ] Search within PDF works
- [ ] Settings persist

---

### Phase 16: Shelves (Manual)

**Goal**: User-owned book collections.

**Dependencies**: Phase 11

**Parallelizable with**: Phase 14, Phase 15

**Changes**:
- [x] `migrations/007_shelves.up.sql` — shelf, shelf_book tables
- [x] `internal/shelf/queries.sql` — sqlc queries for shelf operations
- [x] `internal/shelf/service.go` — business logic
- [x] `internal/shelf/handler.go` — shelf CRUD, add/remove books
- [x] `internal/book/handler.go` — added `/api/books/{id}/shelves` endpoint via shelf handler
- [x] `internal/server/server.go` — wired up shelf service and handler
- [x] `internal/server/routes.go` — mounted shelf routes at `/api/shelves`
- [x] `web/src/features/library/types.ts` — added Shelf and ShelfBook types
- [x] `web/src/features/shelf/ShelfList.tsx` — list of user's shelves
- [x] `web/src/features/shelf/ShelfDetail.tsx` — books in a shelf
- [x] `web/src/features/shelf/AddToShelfDialog.tsx` — add-to-shelf dialog
- [x] `web/src/features/book/BookDetail.tsx` — wired up Add to Shelf button and shelf chips
- [x] `web/src/App.tsx` — replaced ShelvesStub with real ShelfList and ShelfDetail routes
- [x] `sqlc.yaml` — added shelf queries entry

**Verification**:
- Create a shelf with name and icon
- Add books to shelf from book detail page
- View shelf contents
- Remove books from shelf
- Delete shelf
- Shelves are per-user (user A can't see user B's shelves)

---

### Phase 17: User Management & Permissions

**Goal**: Admin user management and permission system.

**Dependencies**: Phase 04

**Parallelizable with**: Phase 10-16

**Changes**:
- [x] `internal/user/handler.go` — admin CRUD endpoints for users
- [x] Permission management: role, per-feature flags (canDownload, canUpload, etc.)
- [x] User-library access control management
- [x] `web/src/features/admin/UserManagement.tsx` — user list, create, edit, permissions
- [x] User settings page: theme, reader preferences
- [x] `web/src/features/auth/SettingsPage.tsx` — user self-service settings
- [x] Routes: /admin/users, /settings

**Verification**:
- Admin can create new users
- Admin can set roles and permissions
- Admin can grant/revoke library access
- Non-admin cannot access admin endpoints
- User can change own theme and reader settings
- Permission flags are enforced (e.g., canDownload)

---

### Phase 18: Dashboard

**Goal**: Configurable dashboard with horizontal book scrollers.

**Dependencies**: Phase 10

**Changes**:
- [x] `internal/dashboard/handler.go` — GET /api/dashboard, PUT /api/dashboard/settings
- [x] `internal/dashboard/service.go` — row type resolution (LAST_READ, LATEST_ADDED, RANDOM)
- [x] `web/src/features/dashboard/Dashboard.tsx` — configurable rows
- [x] `web/src/features/dashboard/ScrollerRow.tsx` — horizontal book card scroller
- [x] Row configuration UI (enable/disable, reorder)
- [x] Route: /dashboard (default landing page after login)

**Verification**:
- Dashboard shows configured rows
- LATEST_ADDED shows recently added books
- RANDOM shows random selection
- LAST_READ shows books with recent progress (once reading is implemented)
- Row configuration persists per user
- Horizontal scroll works on desktop and mobile

---

### Phase 19: Metadata Providers — Google Books

**Goal**: First metadata provider integration with proposal system.

**Dependencies**: Phase 09

**Changes**:
- [x] `internal/metadata/types.go` — MetadataProvider interface, MetadataQuery, MetadataResult types
- [x] `internal/metadata/service.go` — provider registry, search orchestration, proposal management
- [x] `internal/metadata/googlebooks.go` — Google Books implementation
- [x] `migrations/008_metadata_jobs.up.sql` — metadata_fetch_jobs, metadata_fetch_proposals tables
- [x] `internal/metadata/handler.go` — search, proposals, accept/reject, field-lock endpoints
- [x] Per-field lock flags enforcement
- [x] Library metadata source configuration
- [x] `web/src/features/book/MetadataSearch.tsx` — metadata search panel
- [x] `web/src/features/book/BookDetail.tsx` — "Find Metadata" button added

**Verification**:
- [x] Search Google Books by title/author returns results
- [x] Search by ISBN returns results
- [x] Create proposal from search result
- [x] Accept proposal → book metadata updated
- [x] Reject proposal → no changes
- [x] Locked fields not overwritten
- [x] `go test ./...` passes

---

### Phase 20: Additional Metadata Providers

**Goal**: Remaining metadata provider implementations.

**Dependencies**: Phase 19

**Changes**:
- [x] Hardcover provider (GraphQL API)
- [x] Amazon provider (replaced with OpenLibrary — free, no API key, no legal risk)
- [x] GoodReads provider (replaced with OpenLibrary — free, no API key, no legal risk)
- [x] Audible provider (stub — no public API, scraping blocked aggressively)
- [x] ComicVine provider (REST API)
- Per-field priority matrix implementation
- Metadata provider UI in admin settings

**Verification**:
- Each provider returns results for known books
- Priority matrix merges fields correctly
- Rate limiting works (no provider bans)
- Admin can configure provider order per library
- `go test ./...` passes

---

### Phase 21: Magic Shelves

**Goal**: Rule-based dynamic book collections.

**Dependencies**: Phase 16

**Changes**:
- [x] `migrations/009_magic_shelves.up.sql` — magic_shelf table
- [x] `internal/shelf/magic.go` — rule evaluation engine (SQL WHERE builder)
- [x] `internal/shelf/magic_handler.go` — magic shelf CRUD, evaluate rules
- [x] `internal/shelf/magic_queries.sql` — sqlc queries for magic shelves
- [x] `internal/shelf/magic_test.go` — tests for BuildQuery rule engine
- [x] `web/src/features/shelf/MagicShelfBuilder.tsx` — visual rule builder
- [x] Field type selector, operator selector, value input
- [x] AND/OR group nesting (up to 3 levels)
- [x] Live preview count of matching books
- [x] `web/src/features/shelf/MagicShelfDetail.tsx` — magic shelf detail page
- [x] `web/src/features/shelf/ShelfList.tsx` — updated to show magic shelves section
- [x] Routes: /magic-shelves/new, /magic-shelves/{id}, /magic-shelves/{id}/edit

**Verification**:
- [x] Create magic shelf with rules (e.g., author CONTAINS "Sanderson" AND category EQUALS "Fantasy")
- [x] Shelf shows matching books
- [x] Edit rules → results update
- [x] Nested AND/OR groups work
- [x] Live preview count matches actual results
- [x] All supported fields and operators work

---

### Phase 22: Comic Reader

**Goal**: In-browser comic reading with page extraction.

**Dependencies**: Phase 11

**Changes**:
- [x] Page extraction from CBZ (zip), CBR (rar via `mholt/archives`), CB7 (7z via `mholt/archives`)
- [x] `internal/reader/handler.go` — GET /api/reader/books/{bookId}/files/{fileId}/pages, GET /api/reader/books/{bookId}/files/{fileId}/pages/{pageIndex}
- [x] `internal/reader/comic.go` — ListComicPages and GetComicPage functions
- [x] `web/src/features/reader/ComicReader.tsx` — full-screen comic reader with img-based rendering
- [x] Reading direction (LTR/RTL), display mode (single/double), fit mode (width/height/original)
- [x] Page turn via click zones or keyboard (arrow keys)
- [x] Pre-fetch next 2 pages in background
- [x] Progress save (page number, debounced 2s)
- [x] Route: /books/{id}/read/comic
- [x] ReaderDispatch updated to route CBZ/CBR/CB7 to comic reader

**Verification**:
- [x] `go build -tags dev ./...` passes
- [x] `go vet -tags dev ./...` passes
- [x] `go test -tags dev -race ./...` passes (all tests green)
- [x] `npm run build` passes (TypeScript compiles, Vite builds)
- Open a CBZ comic in the reader
- Navigate pages via click/keyboard
- Double-page spread mode works
- Close and reopen — resumes at same page
- CBR and CB7 formats also work
- Settings persist

---

### Phase 23: Audiobook Player

**Goal**: In-browser audiobook playback with chapter support.

**Dependencies**: Phase 11

**Parallelizable with**: Phase 22

**Changes**:
- [x] Track-based playback for folder audiobooks (M4B, M4A, MP3, OPUS, FLAC)
- [x] `internal/reader/handler.go` — token query param middleware, audiobook settings endpoints
- [x] `internal/reader/queries.sql` — GetAudiobookReaderSetting, UpsertAudiobookReaderSetting
- [x] `web/src/features/reader/AudiobookPlayer.tsx` — full-screen audiobook player
- [x] Cover art, title, author, track name display
- [x] Scrubable progress bar with buffered indicator
- [x] Track list panel (collapsible sidebar)
- [x] Playback speed control (0.75×, 1×, 1.25×, 1.5×, 2×)
- [x] Skip back/forward buttons (configurable interval)
- [x] Volume slider with mute toggle
- [x] Progress save (AUDIO_POSITION, debounced 5s)
- [x] Media Session API (lock screen controls)
- [x] Auto-advance to next track on end
- [x] `web/src/features/reader/ReaderDispatch.tsx` — routes audio formats to /audio
- [x] `web/src/App.tsx` — added /books/:id/read/audio route
- [x] Route: /books/{id}/read/audio

**Verification**:
- [x] Go build passes (`go build -tags dev ./...`)
- [x] Go vet passes (`go vet -tags dev ./...`)
- [x] Go tests pass with race detector (`go test -tags dev -race ./...`)
- [x] Frontend builds (`npm run build`)
- [ ] Play an M4B audiobook with chapters
- [ ] Chapter navigation works
- [ ] Play a folder-based audiobook (multiple MP3s)
- [ ] Track transitions are seamless
- [ ] Close and reopen — resumes at same position
- [ ] Playback speed changes work

---

### Phase 24: Annotations & Notebook

**Goal**: EPUB highlights, bookmarks, notes, and unified notebook view.

**Dependencies**: Phase 14

**Changes**:
- [x] `migrations/010_annotations.up.sql` — annotation table with CFI, color, type, note
- [x] EPUB highlight creation (color picker: yellow/green/blue/pink/purple)
- [x] Display existing highlights in EPUB reader on load
- [x] Annotation panel (slide-in) in EPUB reader with delete support
- [x] `internal/notebook/handler.go` — notebook API endpoints
- [x] `web/src/features/notebook/Notebook.tsx` — unified view grouped by book
- [x] Color and book filter, text search in notebook page
- [x] Routes: /notebook (with ?bookId= filter support)
- [x] `internal/server/server.go` and `routes.go` updated to mount notebook handler

**Verification**:
- [x] Highlight text in EPUB reader → appears in notebook
- [x] Notebook shows all annotations grouped by book
- [x] Filter by color and book works
- [x] Delete annotation removes it
- [x] `go build -tags dev ./...` passes
- [x] `go vet -tags dev ./...` passes
- [x] `go test -tags dev -race ./...` passes (all existing tests green)
- [x] `npm run build` passes (TypeScript + Vite)

---

### Phase 25: OPDS Catalog

**Goal**: OPDS 1.2 feed for e-reader clients.

**Dependencies**: Phase 06, Phase 16

**Changes**:
- [x] `migrations/012_opds.up.sql` — opds_user table
- [x] `internal/opds/auth.go` — Basic Auth for OPDS users
- [x] `internal/opds/handler.go` — Atom XML feed generation
- [x] Root catalog, library feeds, shelf feeds, series feeds, author feeds
- [x] OpenSearch description
- [x] Acquisition links for all available formats
- [x] OPDS admin settings (enable/disable)

**Verification**:
- OPDS client (e.g., KOReader, Calibre) can connect with Basic Auth
- Root catalog shows navigation links
- Browse libraries, shelves, series, authors
- Search returns matching books
- Download book files via acquisition links
- OPDS disabled when setting is off

---

### Phase 26: Kobo Sync

**Goal**: Full Kobo store API proxy for device sync.

**Dependencies**: Phase 07, Phase 14

**Changes**:
- [x] `migrations/011_kobo.up.sql` — kobo_device, kobo_reading_state tables
- [x] `internal/kobo/handler.go` — all Kobo store API proxy endpoints
- [x] `internal/kobo/kepub.go` — KEPUB conversion via kepubify (with caching)
- [x] `internal/kobo/queries.sql` — sqlc queries for kobo tables
- [x] KEPUB conversion via kepubify (pure Go, with caching)
- [x] Reading state sync (Kobo → Lexicon progress)
- [x] Token generation via POST /api/kobo/token

**Verification**:
- [x] Generate Kobo token via API
- [x] Kobo device initialization succeeds
- [x] Library sync returns correct book list
- [x] Book download works (including KEPUB conversion)
- [x] Reading state syncs from Kobo to Lexicon
- [x] `CGO_ENABLED=0 go build -tags dev ./...` passes
- [x] `go test -tags dev -race ./...` passes
- [x] `npm run build` passes

---

### Phase 27: KOReader Sync

**Goal**: KOSync protocol for KOReader devices.

**Dependencies**: Phase 07

**Parallelizable with**: Phase 26

**Changes**:
- `migrations/014_koreader.up.sql` — koreader_user, koreader_progress tables
- `internal/koreader/handler.go` — KOSync protocol endpoints
- `internal/koreader/service.go` — user management, progress sync
- MD5 password authentication
- Document matching via fingerprint
- Optional Hardcover forwarding

**Verification**:
- Register KOReader user
- Authenticate via Basic Auth
- Update progress from KOReader
- Retrieve progress for a document
- Progress matches by fingerprint to correct book_file
- Hardcover forwarding works when enabled

---

### Phase 28: BookDrop

**Goal**: Watch-folder ingest queue with review UI.

**Dependencies**: Phase 07, Phase 12

**Changes**:
- `migrations/015_bookdrop.up.sql` — bookdrop_file table
- `internal/bookdrop/watcher.go` — fsnotify watch on BOOKDROP_PATH, stability detection
- `internal/bookdrop/service.go` — metadata extraction, import flow
- `internal/bookdrop/handler.go` — review queue API, import/reject endpoints
- `web/src/features/bookdrop/BookDropQueue.tsx` — review queue UI
- BOOKDROP_FILE_ARRIVED WebSocket event
- Route: /bookdrop

**Verification**:
- Drop a file in the bookdrop folder
- File appears in review queue after stability check
- Extracted metadata and cover shown
- Import file → copied to library, book record created
- Reject file → removed from queue
- Bulk import works
- WebSocket notification received

---

### Phase 29: Email / Send-to-Device

**Goal**: Send book files via email.

**Dependencies**: Phase 11

**Changes**:
- `migrations/016_email.up.sql` — email_provider, email_recipient tables
- `internal/email/handler.go` — provider CRUD, recipient management, send endpoint
- `internal/email/service.go` — SMTP connection, MIME construction, send
- Admin UI: email provider configuration
- User UI: recipient management
- POST /api/books/{id}/send endpoint
- Audit log entry on send

**Verification**:
- Admin configures SMTP provider
- Test email sends successfully
- User adds recipient email
- Send book to recipient → email received with attachment
- Shared providers visible to all users
- Audit log records the send

---

### Phase 30: OIDC & Remote Auth

**Goal**: External authentication providers.

**Dependencies**: Phase 03

**Changes**:
- `migrations/017_oidc.up.sql` — oidc_session, oidc_group_mapping tables
- `internal/auth/oidc.go` — OIDC provider configuration, authorization code flow, callback
- `internal/auth/remote.go` — Remote Auth header middleware
- Group → permission mapping
- OIDC user creation on first login
- Admin UI: OIDC and Remote Auth configuration
- Login page: OIDC provider button

**Verification**:
- Configure OIDC provider in admin settings
- Login via OIDC redirects to provider and back
- User created on first OIDC login
- Group claims map to permissions
- Remote Auth header creates/authenticates user
- Both methods issue valid JWTs

---

### Phase 31: Recommendations

**Goal**: Feature-vector-based book similarity.

**Dependencies**: Phase 09

**Changes**:
- `internal/recommendation/vector.go` — feature hashing (FNV-1a, 128-dim), L2 normalization
- `internal/recommendation/service.go` — cosine similarity, top-N with per-author cap
- `book_vectors` table (BLOB storage)
- RECOMMENDATION_REBUILD task type
- GET /api/books/{id}/similar endpoint
- Similar books section on book detail page

**Verification**:
- Rebuild recommendation vectors for all books
- GET /api/books/{id}/similar returns relevant books
- Results capped at 3 per author
- Books with similar authors/categories/tags rank higher
- Vector rebuild task completes successfully
- `go test ./...` passes

---

### Phase 32: Audit Logs

**Goal**: Action logging with admin viewer.

**Dependencies**: Phase 03

**Changes**:
- `migrations/018_audit.up.sql` — audit_log table
- `internal/audit/` — logging helpers, query endpoints
- Audit middleware/helpers integrated into key actions
- `internal/audit/handler.go` — GET /api/admin/audit-logs (paginated, filterable)
- `web/src/features/admin/AuditLogs.tsx` — table with filters
- AUDIT_LOG_CLEANUP task type
- Route: /admin/audit-logs

**Verification**:
- Login creates audit log entry
- Book download creates audit log entry
- Admin can view audit logs with pagination
- Filter by action type, user, date range works
- Cleanup task removes old entries
- IP address captured correctly

---

### Phase 33: Content Restrictions

**Goal**: Per-user content filtering.

**Dependencies**: Phase 17

**Changes**:
- `migrations/019_content_restrictions.up.sql` — user_content_restriction table
- Content restriction CRUD endpoints
- EXCLUDE and ALLOW_ONLY modes
- Restriction enforcement as additional WHERE clauses in all book queries
- User settings UI for managing restrictions

**Verification**:
- User adds EXCLUDE restriction for category "Horror"
- Horror books no longer appear in any book listing
- User adds ALLOW_ONLY restriction for tag "Favorites"
- Only books with "Favorites" tag appear
- Restrictions are per-user (don't affect other users)
- Admin can see all books regardless

---

### Phase 34: Remaining Metadata Providers

**Goal**: Niche metadata providers.

**Dependencies**: Phase 20

**Changes**:
- Douban provider (Chinese books, scraper)
- LubimyCzytac provider (Polish books, scraper)
- RanobeDB provider (light novels, REST API)

**Verification**:
- Each provider returns results for known books in their domain
- Providers integrate with existing priority matrix
- Rate limiting works correctly

---

### Phase 35: i18n

**Goal**: Internationalization system with English locale.

**Dependencies**: Phase 04

**Changes**:
- `web/src/shared/i18n/i18n.ts` — signal-based i18n system (~50 lines)
- `web/src/shared/i18n/en.json` — English locale file with all UI strings
- Key namespaces: common, auth, library, book, metadata, shelf, reader, admin, errors
- All frontend components updated to use i18n keys instead of hardcoded strings

**Verification**:
- All UI text comes from locale file
- No hardcoded English strings in components
- i18n system is reactive (locale change updates all text)
- Missing keys fall back gracefully (show key name)

---

### Phase 36: Polish & Deployment

**Goal**: Final features, optimization, and deployment readiness.

**Dependencies**: All previous phases

**Changes**:
- Duplicate detection system (STRICT/MODERATE/LOOSE/TITLE_ONLY presets)
- `migrations/020_duplicates.up.sql` — duplicate_dismiss table
- File organization (rename patterns with token substitution)
- Reading sessions and stats (total read time, books read)
- Font management (upload, serve, use in EPUB reader)
- `migrations/021_fonts.up.sql` — custom_font table
- Hardcover sync integration
- `migrations/022_hardcover.up.sql` — hardcover_sync table
- Final Dockerfile optimization
- docker-compose.yml
- entrypoint.sh with user/group ID support
- Health check endpoint enhancement

**Verification**:
- Duplicate detection finds and groups duplicates
- File organization renames files correctly
- Reading stats show accurate data
- Custom fonts work in EPUB reader
- `docker build .` produces working image
- `docker compose up` starts Lexicon successfully
- Full end-to-end workflow: create library → scan → browse → read → sync

---

## Appendix A: Duplicate Detection Presets

| Preset | Comparison Fields |
|---|---|
| `STRICT` | title (exact) + author (exact) + ISBN |
| `MODERATE` | title (normalized) + author (normalized) |
| `LOOSE` | title (normalized, ignore subtitle) |
| `TITLE_ONLY` | title only (very broad) |

Normalization: lowercase, remove punctuation, collapse whitespace, ignore "The"/"A"/"An" prefixes.

## Appendix B: File Format Support Matrix

| Format | Extension | Type | Cover | Metadata | Stream | Download |
|---|---|---|---|---|---|---|
| EPUB | .epub | Ebook | ✓ | ✓ | ✓ | ✓ |
| PDF | .pdf | Ebook | ✓ (page 1) | ✓ | ✓ | ✓ |
| CBZ | .cbz | Comic | ✓ (first img) | ✓ (ComicInfo.xml) | ✓ | ✓ |
| CBR | .cbr | Comic | ✓ (first img) | ✓ | ✓ | ✓ |
| CB7 | .cb7 | Comic | ✓ (first img) | ✓ | ✓ | ✓ |
| MOBI | .mobi | Ebook | ✓ | ✓ | — | ✓ |
| AZW3 | .azw3 | Ebook | ✓ | ✓ | — | ✓ |
| FB2 | .fb2 | Ebook | ✓ | ✓ | — | ✓ |
| M4B | .m4b | Audiobook | ✓ | ✓ (chapters) | ✓ | ✓ |
| M4A | .m4a | Audiobook | ✓ | ✓ | ✓ | ✓ |
| MP3 | .mp3 | Audiobook | ✓ | ✓ | ✓ | ✓ |
| OPUS | .opus | Audiobook | — | ✓ | ✓ | ✓ |

---

*Lexicon Implementation Plan*
*Created: 2026-03-23*
*Derived from analysis of BookLore, rebuilt with minimalist philosophy*
