# Lexicon

> **Work in progress** — Lexicon is under active development and not yet ready for use.

Lexicon is a self-hosted, multi-user digital library manager for ebooks, comics, and audiobooks. It provides in-browser readers, device sync (Kobo, KOReader), metadata management, and OPDS support — all from a single binary.

## Design Goals

- **Self-contained**: Single binary with embedded frontend and SQLite database. No external services required.
- **Minimal dependencies**: Focused libraries over frameworks, stdlib where it makes sense.
- **Single Docker image**: One image, one binary, one database file.

## Tech Stack

- **Backend**: Go (chi, sqlc, pure-Go SQLite)
- **Frontend**: SolidJS + TypeScript + Tailwind CSS
- **Database**: SQLite with WAL mode
- **Build**: `CGO_ENABLED=0` — fully static binary, no CGO

## Features (Planned)

- Multi-user with JWT auth and optional OIDC
- Library scanning with file watching
- EPUB, PDF, and comic (CBZ/CBR/CB7) readers
- Audiobook player with progress sync
- Shelves and smart/magic shelves
- Metadata fetching from multiple providers
- OPDS catalog
- Kobo and KOReader device sync
- BookDrop file ingest
- Send-to-device via email
- Recommendations engine
- Annotations and notebook

## License

[MIT](LICENSE)
