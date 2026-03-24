package opds

import (
	"context"
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/crueber/lexicon/internal/book"
	"github.com/crueber/lexicon/internal/library"
	"github.com/crueber/lexicon/internal/shelf"
	"github.com/crueber/lexicon/internal/user"
)

const (
	_pageSize = 20

	// OPDS namespace constants.
	_nsAtom = "http://www.w3.org/2005/Atom"
	_nsDC   = "http://purl.org/dc/terms/"
	_nsOPDS = "http://opds-spec.org/2010/catalog"
)

// Handler handles OPDS catalog HTTP requests.
// OPDS uses HTTP Basic Auth rather than JWT Bearer tokens.
type Handler struct {
	db     *sql.DB
	logger *slog.Logger
}

// Compile-time interface check.
var _ http.Handler = (*Handler)(nil)

// NewHandler creates a new OPDS Handler.
func NewHandler(db *sql.DB, logger *slog.Logger) *Handler {
	return &Handler{
		db:     db,
		logger: logger,
	}
}

// ServeHTTP implements http.Handler (required for compile-time check).
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

// Routes registers all OPDS routes on the given router.
// No auth middleware is applied here — OPDS uses its own Basic Auth.
func (h *Handler) Routes(r chi.Router) {
	r.Use(h.basicAuth)

	r.Get("/", h.handleRoot)
	r.Get("/books", h.handleAllBooks)
	r.Get("/libraries", h.handleLibraries)
	r.Get("/libraries/{id}/books", h.handleLibraryBooks)
	r.Get("/shelves", h.handleShelves)
	r.Get("/shelves/{id}/books", h.handleShelfBooks)
	r.Get("/books/{id}/files/{fileId}/download", h.handleDownload)
}

// basicAuth is middleware that validates HTTP Basic Auth credentials against
// the database and checks that the user has opds_access permission.
func (h *Handler) basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username == "" || password == "" {
			h.requireAuth(w)
			return
		}

		ctx := r.Context()
		q := user.New(h.db)

		u, err := q.GetUserByUsername(ctx, username)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				h.requireAuth(w)
				return
			}
			h.logger.Error("opds basic auth: get user", "username", username, "error", err)
			h.requireAuth(w)
			return
		}

		if u.Enabled == 0 {
			h.requireAuth(w)
			return
		}

		if !u.PasswordHash.Valid || u.PasswordHash.String == "" {
			h.requireAuth(w)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash.String), []byte(password)); err != nil {
			h.requireAuth(w)
			return
		}

		perms, err := q.GetUserPermissions(ctx, u.ID)
		if err != nil {
			h.logger.Error("opds basic auth: get permissions", "user_id", u.ID, "error", err)
			h.requireAuth(w)
			return
		}

		// Admins always have OPDS access; regular users need opds_access permission.
		if perms.Role != "ADMIN" && perms.OpdsAccess == 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"opds access not permitted"}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// requireAuth sends a 401 response with WWW-Authenticate header.
func (h *Handler) requireAuth(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="Lexicon"`)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"authentication required"}`))
}

// handleRoot handles GET /opds — the root navigation feed.
func (h *Handler) handleRoot(w http.ResponseWriter, _ *http.Request) {
	now := time.Now().UTC().Format(time.RFC3339)
	feed := Feed{
		XMLNS:     _nsAtom,
		XMLNSDC:   _nsDC,
		XMLNSOPDS: _nsOPDS,
		ID:        "urn:lexicon:root",
		Title:     "Lexicon Library",
		Updated:   now,
		Links: []Link{
			{Rel: "self", Href: "/opds", Type: typeNavigation},
			{Rel: "start", Href: "/opds", Type: typeNavigation},
		},
		Entries: []Entry{
			{
				ID:      "urn:lexicon:all-books",
				Title:   "All Books",
				Updated: now,
				Links: []Link{
					{Rel: "subsection", Href: "/opds/books", Type: typeAcquisition},
				},
			},
			{
				ID:      "urn:lexicon:libraries",
				Title:   "Libraries",
				Updated: now,
				Links: []Link{
					{Rel: "subsection", Href: "/opds/libraries", Type: typeNavigation},
				},
			},
			{
				ID:      "urn:lexicon:shelves",
				Title:   "Shelves",
				Updated: now,
				Links: []Link{
					{Rel: "subsection", Href: "/opds/shelves", Type: typeNavigation},
				},
			},
		},
	}

	writeXML(w, feed)
}

// handleAllBooks handles GET /opds/books?page={n} — paginated acquisition feed of all books.
func (h *Handler) handleAllBooks(w http.ResponseWriter, r *http.Request) {
	page := parsePage(r)
	offset := int64((page - 1) * _pageSize)

	ctx := r.Context()
	lq := library.New(h.db)

	libs, err := lq.ListLibraries(ctx)
	if err != nil {
		h.logger.Error("opds all books: list libraries", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	bq := book.New(h.db)

	var entries []Entry
	remaining := int64(_pageSize)

	for _, lib := range libs {
		if remaining <= 0 {
			break
		}

		books, err := bq.ListBooksWithMetadata(ctx, book.ListBooksWithMetadataParams{
			LibraryID: lib.ID,
			Limit:     remaining,
			Offset:    offset,
		})
		if err != nil {
			h.logger.Error("opds all books: list books", "library_id", lib.ID, "error", err)
			continue
		}

		for _, b := range books {
			entry, err := h.buildBookEntry(ctx, b.ID, b.Title, b.CoverPath, b.AddedDate)
			if err != nil {
				h.logger.Error("opds all books: build entry", "book_id", b.ID, "error", err)
				continue
			}
			entries = append(entries, entry)
		}

		remaining -= int64(len(books))
		if offset > 0 {
			count, err := bq.CountBooksByLibrary(ctx, lib.ID)
			if err == nil {
				offset -= count
				if offset < 0 {
					offset = 0
				}
			}
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	feed := Feed{
		XMLNS:     _nsAtom,
		XMLNSDC:   _nsDC,
		XMLNSOPDS: _nsOPDS,
		ID:        "urn:lexicon:all-books",
		Title:     "All Books",
		Updated:   now,
		Links:     paginationLinks("/opds/books", page, len(entries) == _pageSize),
		Entries:   entries,
	}

	writeXML(w, feed)
}

// handleLibraries handles GET /opds/libraries — navigation feed listing all libraries.
func (h *Handler) handleLibraries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lq := library.New(h.db)

	libs, err := lq.ListLibraries(ctx)
	if err != nil {
		h.logger.Error("opds libraries: list", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	entries := make([]Entry, 0, len(libs))
	for _, lib := range libs {
		entries = append(entries, Entry{
			ID:      fmt.Sprintf("urn:lexicon:library:%d", lib.ID),
			Title:   lib.Name,
			Updated: now,
			Links: []Link{
				{
					Rel:  "subsection",
					Href: fmt.Sprintf("/opds/libraries/%d/books", lib.ID),
					Type: typeAcquisition,
				},
			},
		})
	}

	feed := Feed{
		XMLNS:     _nsAtom,
		XMLNSDC:   _nsDC,
		XMLNSOPDS: _nsOPDS,
		ID:        "urn:lexicon:libraries",
		Title:     "Libraries",
		Updated:   now,
		Links: []Link{
			{Rel: "self", Href: "/opds/libraries", Type: typeNavigation},
			{Rel: "start", Href: "/opds", Type: typeNavigation},
		},
		Entries: entries,
	}

	writeXML(w, feed)
}

// handleLibraryBooks handles GET /opds/libraries/{id}/books?page={n}.
func (h *Handler) handleLibraryBooks(w http.ResponseWriter, r *http.Request) {
	libraryID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid library id", http.StatusBadRequest)
		return
	}

	page := parsePage(r)
	offset := int64((page - 1) * _pageSize)

	ctx := r.Context()
	lq := library.New(h.db)

	lib, err := lq.GetLibraryByID(ctx, libraryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "library not found", http.StatusNotFound)
			return
		}
		h.logger.Error("opds library books: get library", "library_id", libraryID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	bq := book.New(h.db)
	books, err := bq.ListBooksWithMetadata(ctx, book.ListBooksWithMetadataParams{
		LibraryID: libraryID,
		Limit:     _pageSize,
		Offset:    offset,
	})
	if err != nil {
		h.logger.Error("opds library books: list", "library_id", libraryID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	entries := make([]Entry, 0, len(books))
	for _, b := range books {
		entry, err := h.buildBookEntry(ctx, b.ID, b.Title, b.CoverPath, b.AddedDate)
		if err != nil {
			h.logger.Error("opds library books: build entry", "book_id", b.ID, "error", err)
			continue
		}
		entries = append(entries, entry)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	selfHref := fmt.Sprintf("/opds/libraries/%d/books", libraryID)
	feed := Feed{
		XMLNS:     _nsAtom,
		XMLNSDC:   _nsDC,
		XMLNSOPDS: _nsOPDS,
		ID:        fmt.Sprintf("urn:lexicon:library:%d:books", libraryID),
		Title:     lib.Name,
		Updated:   now,
		Links:     paginationLinks(selfHref, page, len(entries) == _pageSize),
		Entries:   entries,
	}

	writeXML(w, feed)
}

// handleShelves handles GET /opds/shelves — navigation feed listing the authenticated user's shelves.
func (h *Handler) handleShelves(w http.ResponseWriter, r *http.Request) {
	u, ok := h.userFromRequest(r)
	if !ok {
		h.requireAuth(w)
		return
	}

	ctx := r.Context()
	sq := shelf.New(h.db)

	shelves, err := sq.ListShelvesForUser(ctx, u.ID)
	if err != nil {
		h.logger.Error("opds shelves: list", "user_id", u.ID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	entries := make([]Entry, 0, len(shelves))
	for _, s := range shelves {
		entries = append(entries, Entry{
			ID:      fmt.Sprintf("urn:lexicon:shelf:%d", s.ID),
			Title:   s.Name,
			Updated: now,
			Links: []Link{
				{
					Rel:  "subsection",
					Href: fmt.Sprintf("/opds/shelves/%d/books", s.ID),
					Type: typeAcquisition,
				},
			},
		})
	}

	feed := Feed{
		XMLNS:     _nsAtom,
		XMLNSDC:   _nsDC,
		XMLNSOPDS: _nsOPDS,
		ID:        "urn:lexicon:shelves",
		Title:     "Shelves",
		Updated:   now,
		Links: []Link{
			{Rel: "self", Href: "/opds/shelves", Type: typeNavigation},
			{Rel: "start", Href: "/opds", Type: typeNavigation},
		},
		Entries: entries,
	}

	writeXML(w, feed)
}

// handleShelfBooks handles GET /opds/shelves/{id}/books?page={n}.
func (h *Handler) handleShelfBooks(w http.ResponseWriter, r *http.Request) {
	shelfID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid shelf id", http.StatusBadRequest)
		return
	}

	u, ok := h.userFromRequest(r)
	if !ok {
		h.requireAuth(w)
		return
	}

	ctx := r.Context()
	sq := shelf.New(h.db)

	s, err := sq.GetShelfByID(ctx, shelfID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "shelf not found", http.StatusNotFound)
			return
		}
		h.logger.Error("opds shelf books: get shelf", "shelf_id", shelfID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Only the shelf owner can access it (unless it's public).
	if s.UserID != u.ID && s.IsPublic == 0 {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	booksInShelf, err := sq.ListBooksInShelf(ctx, shelfID)
	if err != nil {
		h.logger.Error("opds shelf books: list", "shelf_id", shelfID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Apply pagination manually since ListBooksInShelf doesn't support it.
	page := parsePage(r)
	start := (page - 1) * _pageSize
	end := start + _pageSize
	if start > len(booksInShelf) {
		start = len(booksInShelf)
	}
	if end > len(booksInShelf) {
		end = len(booksInShelf)
	}
	paginated := booksInShelf[start:end]

	entries := make([]Entry, 0, len(paginated))
	for _, b := range paginated {
		entry, err := h.buildBookEntry(ctx, b.ID, b.Title, b.CoverPath, b.AddedDate)
		if err != nil {
			h.logger.Error("opds shelf books: build entry", "book_id", b.ID, "error", err)
			continue
		}
		entries = append(entries, entry)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	selfHref := fmt.Sprintf("/opds/shelves/%d/books", shelfID)
	feed := Feed{
		XMLNS:     _nsAtom,
		XMLNSDC:   _nsDC,
		XMLNSOPDS: _nsOPDS,
		ID:        fmt.Sprintf("urn:lexicon:shelf:%d:books", shelfID),
		Title:     s.Name,
		Updated:   now,
		Links:     paginationLinks(selfHref, page, len(entries) == _pageSize),
		Entries:   entries,
	}

	writeXML(w, feed)
}

// handleDownload handles GET /opds/books/{id}/files/{fileId}/download.
// Streams the book file as an attachment.
func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request) {
	bookID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid book id", http.StatusBadRequest)
		return
	}

	fileID, err := strconv.ParseInt(chi.URLParam(r, "fileId"), 10, 64)
	if err != nil {
		http.Error(w, "invalid file id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	bq := book.New(h.db)

	// Fetch the book file and verify it belongs to the book.
	bookFile, err := bq.GetBookFileByID(ctx, fileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		h.logger.Error("opds download: get book file", "file_id", fileID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if bookFile.BookID != bookID {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	// Open the file from disk.
	f, err := os.Open(bookFile.FilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "file not found on disk", http.StatusNotFound)
			return
		}
		h.logger.Error("opds download: open file", "path", bookFile.FilePath, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		h.logger.Error("opds download: stat file", "path", bookFile.FilePath, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	contentType := "application/octet-stream"
	if ct, ok := opdsContentTypes[bookFile.Format]; ok {
		contentType = ct
	}

	filename := filepath.Base(bookFile.FilePath)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
}

// buildBookEntry constructs an OPDS Entry for a book, including cover links
// and acquisition links for each available file format.
func (h *Handler) buildBookEntry(
	ctx context.Context,
	bookID int64,
	title, coverPath, addedDate sql.NullString,
) (Entry, error) {
	titleStr := ""
	if title.Valid {
		titleStr = title.String
	}
	if titleStr == "" {
		titleStr = fmt.Sprintf("Book #%d", bookID)
	}

	updated := ""
	if addedDate.Valid {
		updated = addedDate.String
	}
	if updated == "" {
		updated = time.Now().UTC().Format(time.RFC3339)
	}

	links := []Link{
		{
			Rel:  "alternate",
			Href: fmt.Sprintf("/api/books/%d", bookID),
			Type: "text/html",
		},
	}

	// Add cover link if available.
	if coverPath.Valid && coverPath.String != "" {
		links = append(links, Link{
			Rel:  "http://opds-spec.org/image",
			Href: fmt.Sprintf("/api/books/%d/cover", bookID),
			Type: "image/jpeg",
		})
		links = append(links, Link{
			Rel:  "http://opds-spec.org/image/thumbnail",
			Href: fmt.Sprintf("/api/books/%d/cover?size=thumb", bookID),
			Type: "image/jpeg",
		})
	}

	// Fetch book files for acquisition links.
	bq := book.New(h.db)
	files, err := bq.ListBookFiles(ctx, bookID)
	if err != nil {
		return Entry{}, fmt.Errorf("list book files for book %d: %w", bookID, err)
	}

	for _, f := range files {
		ct, ok := opdsContentTypes[f.Format]
		if !ok {
			ct = "application/octet-stream"
		}
		links = append(links, Link{
			Rel:  "http://opds-spec.org/acquisition",
			Href: fmt.Sprintf("/opds/books/%d/files/%d/download", bookID, f.ID),
			Type: ct,
		})
	}

	return Entry{
		ID:      fmt.Sprintf("urn:lexicon:book:%d", bookID),
		Title:   titleStr,
		Updated: updated,
		Links:   links,
	}, nil
}

// userFromRequest extracts the authenticated user from the Basic Auth header.
// This is needed for endpoints that require knowing which user is making the request.
func (h *Handler) userFromRequest(r *http.Request) (user.User, bool) {
	username, _, ok := r.BasicAuth()
	if !ok || username == "" {
		return user.User{}, false
	}

	ctx := r.Context()
	q := user.New(h.db)

	u, err := q.GetUserByUsername(ctx, username)
	if err != nil {
		return user.User{}, false
	}

	return u, true
}

// writeXML writes an XML response with the OPDS content type.
func writeXML(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(v)
}

// parsePage extracts the page number from the query string, defaulting to 1.
func parsePage(r *http.Request) int {
	p, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || p < 1 {
		return 1
	}
	return p
}

// paginationLinks builds the standard OPDS pagination link set.
func paginationLinks(selfHref string, page int, hasNext bool) []Link {
	links := []Link{
		{Rel: "self", Href: fmt.Sprintf("%s?page=%d", selfHref, page), Type: typeAcquisition},
		{Rel: "start", Href: "/opds", Type: typeNavigation},
	}
	if page > 1 {
		links = append(links, Link{
			Rel:  "previous",
			Href: fmt.Sprintf("%s?page=%d", selfHref, page-1),
			Type: typeAcquisition,
		})
	}
	if hasNext {
		links = append(links, Link{
			Rel:  "next",
			Href: fmt.Sprintf("%s?page=%d", selfHref, page+1),
			Type: typeAcquisition,
		})
	}
	return links
}
