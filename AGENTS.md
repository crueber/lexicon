# Lexicon — Agent Guidelines

## Project Overview

Lexicon is a self-hosted, multi-user digital library manager built with Go (backend) and SolidJS (frontend). It manages ebooks, comics, and audiobooks with in-browser readers, device sync, and metadata management.

**Module path**: `github.com/crueber/lexicon`
**Database**: SQLite with WAL mode (no external database)
**Deployment**: Single Docker image, single binary with embedded frontend

## Architecture & Philosophy

### Minimalist Principles

1. **Self-contained**: Single binary, embedded frontend, SQLite database. No external services required.
2. **Minimal dependencies**: Use focused, lightweight libraries — not frameworks. Prefer stdlib where it provides good ergonomics.
3. **Developer ergonomics over purity**: We use libraries where they meaningfully reduce boilerplate (chi, sqlc). We don't avoid libraries just for the sake of it.
4. **Domain-organized code**: Both backend and frontend are organized by feature/domain, not by technical layer (no `handlers/`, `models/`, `services/` folders mixing unrelated concerns).
5. **Subagent-friendly**: Implementation is broken into granular phases. Each phase has clear boundaries, inputs, outputs, and verification criteria.

### What "Minimalist" Means Here

- **YES**: chi (lightweight router), sqlc (build-time codegen), `log/slog` (stdlib), `caarlos0/env` (tiny config lib)
- **NO**: Spring-style frameworks, ORMs with runtime reflection, viper (massive config framework), zerolog (when slog exists in stdlib)
- **Rule of thumb**: If the stdlib does it well, use stdlib. If a small focused library does it much better, use the library. If only a framework does it, reconsider whether we need the feature.
- **Pure Go, no CGO**: All dependencies must be pure Go. The binary is built with `CGO_ENABLED=0` for a fully static binary. Never add a dependency that requires CGO without explicit discussion.

---

## Technology Stack

### Backend (Go)

**All libraries below are pure Go. No CGO required. The entire binary builds with `CGO_ENABLED=0`.**

| Concern | Library | Why |
|---|---|---|
| HTTP router | `go-chi/chi` v5 | Lightweight, middleware-friendly, not a framework |
| Database driver | `modernc.org/sqlite` | Pure Go SQLite, no CGO required |
| Query codegen | `sqlc` | Build-time type-safe query generation, zero runtime dep |
| Migrations | `golang-migrate` | Use `database/sqlite` driver (pure Go), NOT `database/sqlite3` (CGO) |
| Auth/JWT | `golang-jwt/jwt` v5 | JWT is complex enough to warrant a library |
| OIDC | `coreos/go-oidc` v3 | OIDC is complex enough to warrant a library |
| WebSocket | `coder/websocket` | Modern, maintained (gorilla is in maintenance mode) |
| Config | `caarlos0/env` | Tiny — env vars → struct with tags |
| Logging | `log/slog` (stdlib) | Built-in structured logging since Go 1.21 |
| File watching | `fsnotify` | No stdlib alternative |
| Image processing | `disintegration/imaging` | Resize, crop, JPEG encode |
| PDF processing | `pdfcpu` | Cover extraction |
| Archive extraction | `mholt/archives` | Pure Go RAR/7z/ZIP extraction (CBR/CB7/CBZ) |
| Audio metadata | `dhowden/tag` | ID3/MP4 tag reading |
| Cron | `robfig/cron` v3 | Cron expression scheduling |
| EPUB parsing | Custom zip reader | Parse OPF from EPUB zip |
| Password hashing | `golang.org/x/crypto/bcrypt` | Extended stdlib |
| HTTP client | `net/http` (stdlib) | With timeouts |
| Vector similarity | Pure Go | 128-dim float32, cosine similarity |
| Testing | `testing` (stdlib) | Table-driven tests |

**Do NOT add** without discussion: ORMs (gorm, ent), large config frameworks (viper, koanf), logging frameworks (zerolog, zap), HTTP frameworks (gin, fiber, echo).

### Frontend (SolidJS + TypeScript)

| Concern | Library | Why |
|---|---|---|
| Framework | SolidJS + TypeScript | Fine-grained reactivity, components run once |
| Build | Vite | Standard build tool |
| Routing | `@solidjs/router` | Standard Solid router |
| UI primitives | `@kobalte/core` | Headless, accessible components |
| Styling | Tailwind CSS | Utility-first, no runtime cost |
| Icons | `lucide-solid` | Lightweight icon set |
| PDF reader | `pdfjs-dist` | Mozilla's PDF.js, no alternative |
| EPUB reader | `epubjs` | FolioJS EPUB renderer |
| Comic reader | Custom canvas | No library needed |
| Audio player | HTML5 `<audio>` | + Media Session API |
| Markdown | `marked` | Lightweight parser |
| HTTP client | Browser `fetch` | Typed wrapper, no library needed |
| WebSocket | Browser native | Reconnecting wrapper, no library needed |
| i18n | Custom signal-based | ~50 lines of code, no library needed |

**Do NOT add** without discussion: React-style state managers (Redux, MobX), CSS-in-JS libraries, heavy component libraries (PrimeNG, Material UI), axios/ky/ofetch.

---

## Go Conventions

### Project Structure

```
lexicon/
├── cmd/lexicon/main.go          # thin entry point
├── internal/                     # all application code
│   ├── server/                   # HTTP server, routes, shared middleware
│   ├── auth/                     # authentication & authorization
│   ├── user/                     # user management
│   ├── library/                  # library management, scanning, watching
│   ├── book/                     # book CRUD
│   ├── metadata/                 # metadata providers
│   ├── shelf/                    # shelves & magic shelves
│   ├── reader/                   # file serving, progress, annotations
│   ├── opds/                     # OPDS catalog
│   ├── kobo/                     # Kobo sync
│   ├── koreader/                 # KOReader sync
│   ├── bookdrop/                 # BookDrop ingest
│   ├── email/                    # email send-to-device
│   ├── dashboard/                # dashboard
│   ├── notebook/                 # annotations notebook
│   ├── recommendation/           # recommendation engine
│   ├── task/                     # background task system
│   ├── audit/                    # audit logging
│   ├── storage/                  # covers, fonts, fingerprinting
│   ├── ws/                       # WebSocket hub
│   └── appsettings/              # runtime app settings
├── migrations/                   # SQL migration files (top-level)
├── sqlc.yaml                     # sqlc configuration
├── web/                          # SolidJS frontend
├── Dockerfile
├── Makefile
└── go.mod
```

### Entry Point Pattern

```go
func main() {
    if err := run(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}

func run() error {
    // all initialization and business logic here
}
```

`os.Exit` and `log.Fatal` belong in `main` only. Never in library code.

### Naming

- **MixedCaps everywhere**: `maxLength`, `userID`, `parseURL` — no underscores
- **Package names**: all lowercase, no underscores, no mixedCaps
- **Don't repeat package name in exports**: `book.Book`, not `book.BookRecord`
- **Getters have no prefix**: `obj.Owner()`, not `obj.GetOwner()`
- **Receiver names**: short, consistent — `func (s *Service)`, `func (h *Handler)`
- **Context is always first**: `func DoThing(ctx context.Context, arg string) error`
- **Constants**: MixedCaps, not ALL_CAPS — `const MaxPacketSize = 1024`

### Error Handling

- **Errors are the last return value**: `func Open(path string) (*File, error)`
- **Always check errors**: Never discard with `_` without documented reason
- **Error strings**: lowercase, no trailing punctuation — `errors.New("connection refused")`
- **Indent error flow, keep happy path left**:
  ```go
  if err != nil {
      return fmt.Errorf("open config: %w", err)
  }
  // happy path continues here
  ```
- **Wrap with `%w`** when callers should inspect: `return fmt.Errorf("read config: %w", err)`
- **Wrap with `%v`** at system boundaries to hide internals
- **Handle each error once**: either log it or return it, never both
- **No panic for normal error handling**: panic is for invariant violations and bugs only

### Interfaces

- **Define interfaces at the consumer, not the producer**: The package that *uses* an interface defines it
- **Keep interfaces small**: prefer single-method interfaces
- **Never use a pointer to an interface**: `var r io.Reader`, not `var r *io.Reader`
- **Verify compliance at compile time**: `var _ http.Handler = (*Handler)(nil)`

### Database & Queries

- **SQLite with WAL mode**: Set `PRAGMA journal_mode=WAL`, `PRAGMA busy_timeout=5000`, `PRAGMA foreign_keys=ON` at connection time
- **sqlc for all queries**: Write SQL in `.sql` files, generate Go code. No hand-written SQL string concatenation.
- **Each domain owns its queries**: `internal/book/queries.sql`, not a shared `db/queries/` directory
- **Migrations at top level**: `migrations/` directory, numbered sequentially
- **Use `modernc.org/sqlite`**: Pure Go, no CGO. Import as `_ "modernc.org/sqlite"`

### Concurrency

- **Always know when a goroutine exits**: Use `sync.WaitGroup` or `errgroup`
- **Prefer synchronous APIs**: Let callers add concurrency
- **Context for cancellation**: Pass `context.Context`, use `context.WithCancel` for task cancellation
- **Mutex as unexported field**: `mu sync.Mutex`, never embed `sync.Mutex`
- **Run tests with race detector**: `go test -race ./...`

### Testing

- **Table-driven tests with `t.Run`**: canonical Go testing pattern
- **Mark helpers with `t.Helper()`**: so stack traces point to the caller
- **Prefer real implementations over mocks**: test against the real thing using public API
- **Error message format**: `got X; want Y`
- **Race detector in CI**: `go test -race ./...`

### Logging

Use `log/slog` throughout:

```go
slog.Info("library scan complete",
    "library_id", libraryID,
    "books_added", added,
    "duration", elapsed,
)
```

- Use structured key-value pairs, not formatted strings
- Log levels: `Debug`, `Info`, `Warn`, `Error`
- Never log and return the same error — do one or the other

### Configuration

Use `caarlos0/env` to parse environment variables into a config struct:

```go
type Config struct {
    Port           int    `env:"PORT" envDefault:"6060"`
    DataDir        string `env:"DATA_DIR" envDefault:"/app/data"`
    JWTSecret      string `env:"JWT_SECRET,required"`
    LogLevel       string `env:"LOG_LEVEL" envDefault:"info"`
    LogFormat      string `env:"LOG_FORMAT" envDefault:"json"`
}
```

No config files. Environment variables only. This keeps deployment simple.

---

## SolidJS Conventions

### Core Mental Model

- **Components run once**: The function body is setup code, not a render function. There is no re-render cycle.
- **Signals are functions**: Read by calling: `count()`, not `count`.
- **Never destructure props**: It breaks reactivity.
- **Reactive contexts**: JSX expressions, `createEffect`, `createMemo`, `createResource` track signal reads automatically.

### Props

```tsx
// CORRECT: access props directly
function Greeting(props: { name: string }) {
  return <div>Hello {props.name}</div>;
}

// WRONG: destructuring breaks reactivity
function Greeting({ name }: { name: string }) { ... }
```

- Use `mergeProps` for defaults
- Use `splitProps` to forward subsets of props

### State

- **Primitive values**: `createSignal`
- **Complex objects**: `createStore` from `solid-js/store`
- **Derived state**: `createMemo` — never derive state by setting a signal inside an effect
- **Side effects**: `createEffect` — for side effects only, not state derivation
- **One-time setup**: `onMount` — not `createEffect` with no deps

### Control Flow

Use Solid's built-in components, not JS expressions:

- `<Show when={...}>` for conditionals
- `<For each={...}>` for lists (not `.map()`)
- `<Switch>` / `<Match>` for multi-branch
- `<Suspense>` + `<ErrorBoundary>` for async

### Async Data

- Use `createResource` for data fetching — integrates with `<Suspense>` and `<ErrorBoundary>`
- Wrap async components in `<Suspense>` and `<ErrorBoundary>`

### Frontend Organization

```
web/src/
├── features/           # domain-organized feature modules
│   ├── auth/
│   ├── library/
│   ├── book/
│   ├── reader/
│   ├── shelf/
│   ├── dashboard/
│   ├── notebook/
│   ├── admin/
│   └── bookdrop/
├── shared/
│   ├── ui/             # reusable UI components (Kobalte-based)
│   ├── api/            # typed fetch wrapper
│   ├── ws/             # WebSocket client
│   └── i18n/           # i18n system
├── App.tsx
├── index.tsx
└── index.css           # Tailwind imports
```

- **Organize by feature, not by type**: No flat `components/`, `hooks/`, `utils/` folders
- **Colocate**: Keep component, styles, and related logic together in the feature directory
- **Shared UI**: Only truly reusable components go in `shared/ui/`

### Styling

- **Tailwind CSS**: utility classes in JSX
- **Dark mode first**: homelab users expect dark by default
- **CSS custom properties**: for theme accent colors
- **Responsive**: sidebar collapses to bottom nav on mobile

### HTTP Client

Typed wrapper around browser `fetch`:

```typescript
// shared/api/client.ts
async function api<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`/api${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${getAccessToken()}`,
      ...options?.headers,
    },
  });
  if (!response.ok) throw new ApiError(response);
  return response.json();
}
```

No external HTTP client library.

### WebSocket

Reconnecting wrapper around native WebSocket:

```typescript
// shared/ws/socket.ts
function createReconnectingWebSocket(url: string): WebSocket { ... }
```

No external WebSocket library.

---

## Implementation Phases

The project is implemented in 36 sequential phases. See `plans/2026-03-23-lexicon-implementation-plan.md` Section 27 for the full phase breakdown.

**Key rules for phase implementation:**

1. **One phase at a time**: Complete and verify a phase before starting the next (unless explicitly marked as parallelizable).
2. **Each phase is self-contained**: A subagent implementing a phase should not need to understand the entire codebase — only the files and interfaces relevant to that phase.
3. **Verify before proceeding**: Each phase has explicit verification criteria. All must pass before the phase is considered complete.
4. **Don't break earlier phases**: New code must not break existing functionality. Run `go test -race ./...` and verify the frontend builds after every phase.
5. **Follow the plan**: The implementation plan is the source of truth for what each phase includes. Don't add scope. Don't skip steps.

### Phase Dependencies (Parallelization Guide)

```
Phase 01 (Scaffold)
├── Phase 02 (Database)
│   ├── Phase 03 (Auth)
│   │   ├── Phase 04 (Frontend Auth)
│   │   │   ├── Phase 10 (Library Browser) ← also needs 06, 08
│   │   │   ├── Phase 12 (WebSocket) ← also needs 07
│   │   │   └── Phase 17 (User Mgmt)
│   │   ├── Phase 30 (OIDC)
│   │   └── Phase 32 (Audit Logs)
│   └── Phase 05 (Book Data Model) ← parallelizable with 03, 04
│       └── Phase 06 (Library API) ← needs 03
│           └── Phase 07 (Scanner)
│               ├── Phase 08 (Covers)
│               ├── Phase 09 (Metadata Extraction) ← parallelizable with 08
│               ├── Phase 26 (Kobo) ← also needs 14
│               └── Phase 27 (KOReader) ← parallelizable with 26
Phase 11 (Book Detail) ← needs 10
├── Phase 14 (EPUB Reader) ← parallelizable with 15, 16
├── Phase 15 (PDF Reader) ← parallelizable with 14, 16
├── Phase 16 (Shelves) ← parallelizable with 14, 15
├── Phase 22 (Comic Reader)
├── Phase 23 (Audiobook) ← parallelizable with 22
└── Phase 29 (Email)
Phase 13 (Tasks) ← needs 12
Phase 18 (Dashboard) ← needs 10
Phase 19 (Metadata Providers) ← needs 09
└── Phase 20 (More Providers)
    └── Phase 34 (Niche Providers)
Phase 21 (Magic Shelves) ← needs 16
Phase 24 (Annotations) ← needs 14
Phase 25 (OPDS) ← needs 06, 16
Phase 28 (BookDrop) ← needs 07, 12
Phase 31 (Recommendations) ← needs 09
Phase 33 (Content Restrictions) ← needs 17
Phase 35 (i18n) ← needs 04
Phase 36 (Polish) ← needs all
```

---

## Code Quality Standards

### Before Every Commit

```bash
# Go
go build ./...                    # compiles
go vet ./...                      # catches common bugs
go test -race ./...               # tests pass, no data races
sqlc generate                     # queries up to date (if SQL changed)

# Frontend
npm run build                     # TypeScript compiles, Vite builds
npm run lint                      # ESLint passes (if configured)
```

### Makefile Targets

The project should have these standard targets:

```makefile
build:          # Build Go binary + frontend
run:            # Run in development mode
test:           # Run all tests with race detector
lint:           # Run go vet + staticcheck
sqlc:           # Regenerate sqlc code
migrate-up:     # Apply all pending migrations
migrate-down:   # Roll back last migration
docker-build:   # Build Docker image
```

### File Naming

- Go files: `lowercase.go` (e.g., `handler.go`, `service.go`, `queries.sql`)
- Go test files: `lowercase_test.go`
- Migration files: `NNN_description.up.sql`, `NNN_description.down.sql`
- TypeScript/TSX: `PascalCase.tsx` for components, `camelCase.ts` for utilities
- CSS: component-colocated or in `index.css` for globals

### Git Conventions

- Commit messages: imperative mood, concise — "add library scanner", "fix EPUB cover extraction"
- One logical change per commit
- Don't commit generated files (sqlc output is committed since it's part of the build)
- Don't commit `node_modules/`, `web/dist/`, or `*.db` files

---

## SQLite-Specific Notes

Since we use SQLite instead of PostgreSQL, keep these differences in mind:

- **Types**: Use `INTEGER`, `TEXT`, `REAL`, `BLOB` — SQLite doesn't enforce VARCHAR lengths
- **Booleans**: Use `INTEGER` (0/1), not `BOOLEAN`
- **Decimals**: Use `REAL` instead of `DECIMAL`
- **Timestamps**: Use `TEXT` with ISO 8601 format, `DEFAULT (datetime('now'))`
- **Auto-increment**: `INTEGER PRIMARY KEY AUTOINCREMENT`
- **JSON**: Store as `TEXT`, parse in Go — no `JSONB` type
- **Arrays/Vectors**: Store as `BLOB` (binary) or `TEXT` (JSON) — no `FLOAT4[]` or array types
- **Concurrent writes**: WAL mode allows concurrent reads with one writer. This is fine for our workload.
- **Connection pragmas**: Set these on every connection:
  ```sql
  PRAGMA journal_mode=WAL;
  PRAGMA busy_timeout=5000;
  PRAGMA foreign_keys=ON;
  PRAGMA synchronous=NORMAL;
  ```
- **No `BIGSERIAL`**: Use `INTEGER PRIMARY KEY AUTOINCREMENT`
- **No `INET`**: Store IP addresses as `TEXT`

---

## Security Notes

- **JWT secrets**: Minimum 32 characters, loaded from environment variable
- **Password hashing**: bcrypt only, never store plaintext
- **SQL injection**: sqlc generates parameterized queries — never concatenate user input into SQL
- **File paths**: Sanitize all user-provided paths, prevent directory traversal
- **Cover uploads**: Validate image format, enforce size limits, protect against decompression bombs
- **CORS**: Configure appropriately for production (same-origin by default)
- **Rate limiting**: Metadata provider scrapers must respect rate limits
- **Secrets in logs**: Never log JWT tokens, passwords, or API keys

---

## Reference Documents

- **Implementation Plan**: `plans/2026-03-23-lexicon-implementation-plan.md`
- **Go Conventions**: See golang-conventions skill for comprehensive Go patterns
- **SolidJS Conventions**: See solidjs-conventions skill for comprehensive SolidJS patterns
