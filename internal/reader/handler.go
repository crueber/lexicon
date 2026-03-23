package reader

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/crueber/lexicon/internal/auth"
)

// Handler handles HTTP requests for the reader endpoints.
type Handler struct {
	db     *sql.DB
	logger *slog.Logger
}

// Compile-time interface check.
var _ http.Handler = (*Handler)(nil)

// NewHandler creates a new reader Handler.
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

// TokenQueryParamMiddleware extracts a JWT from the ?token= query parameter
// and injects it as a Bearer token in the Authorization header. This allows
// HTML5 <audio> and <video> elements (which cannot set custom headers) to
// authenticate with the streaming endpoint.
//
// If an Authorization header is already present, it is left unchanged.
func TokenQueryParamMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			if token := r.URL.Query().Get("token"); token != "" {
				// Clone the request so we can modify the header safely.
				r2 := r.Clone(r.Context())
				r2.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
				next.ServeHTTP(w, r2)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Routes registers all reader routes on the given router.
// RequireAuth must already be applied by the caller.
func (h *Handler) Routes(r chi.Router) {
	// Apply token query param middleware so <audio src="...?token=..."> works.
	r.Use(TokenQueryParamMiddleware)

	r.Get("/books/{bookId}/files/{fileId}/stream", h.handleStream)
	r.Get("/books/{bookId}/files/{fileId}/pages", h.handleListComicPages)
	r.Get("/books/{bookId}/files/{fileId}/pages/{pageIndex}", h.handleGetComicPage)
	r.Get("/books/{bookId}/progress", h.handleGetProgress)
	r.Put("/books/{bookId}/progress", h.handlePutProgress)
	r.Get("/books/{bookId}/settings", h.handleGetSettings)
	r.Put("/books/{bookId}/settings", h.handlePutSettings)
	r.Get("/books/{bookId}/audiobook-settings", h.handleGetAudiobookSettings)
	r.Put("/books/{bookId}/audiobook-settings", h.handlePutAudiobookSettings)
}

// contentTypeForFormat returns the appropriate MIME type for a book file format.
func contentTypeForFormat(format string) string {
	switch format {
	case "EPUB":
		return "application/epub+zip"
	case "PDF":
		return "application/pdf"
	case "CBZ":
		return "application/zip"
	case "CBR":
		return "application/x-rar-compressed"
	case "CB7":
		return "application/x-7z-compressed"
	case "MP3":
		return "audio/mpeg"
	case "M4B":
		return "audio/mp4"
	case "M4A":
		return "audio/mp4"
	case "OPUS":
		return "audio/ogg; codecs=opus"
	case "FLAC":
		return "audio/flac"
	default:
		return "application/octet-stream"
	}
}

// hasLibraryAccess checks whether the principal has access to the given library.
// Admins always have access. Regular users must have an explicit library permission.
func hasLibraryAccess(principal *auth.Principal, libraryID int64) bool {
	if principal.IsAdmin() {
		return true
	}
	for _, id := range principal.LibraryIDs {
		if id == libraryID {
			return true
		}
	}
	return false
}

// handleStream handles GET /api/reader/books/{bookId}/files/{fileId}/stream.
// It streams the book file with Range request support.
func (h *Handler) handleStream(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	bookID, err := strconv.ParseInt(chi.URLParam(r, "bookId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	fileID, err := strconv.ParseInt(chi.URLParam(r, "fileId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid file id")
		return
	}

	q := New(h.db)
	ctx := r.Context()

	// Fetch the book file and verify it belongs to the book.
	bookFile, err := q.GetBookFileForReader(ctx, GetBookFileForReaderParams{
		ID:     fileID,
		BookID: bookID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		h.logger.Error("get book file for reader", "file_id", fileID, "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Verify the user has access to the library containing this book.
	if !hasLibraryAccess(principal, bookFile.LibraryID) {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	// Open the file from disk.
	f, err := os.Open(bookFile.FilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "file not found on disk")
			return
		}
		h.logger.Error("open book file", "path", bookFile.FilePath, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		h.logger.Error("stat book file", "path", bookFile.FilePath, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Set headers before serving.
	w.Header().Set("Content-Type", contentTypeForFormat(bookFile.Format))
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Accept-Ranges", "bytes")

	// http.ServeContent handles Range requests, ETags, and Last-Modified automatically.
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
}

// progressRequest is the JSON body for PUT /api/reader/books/{bookId}/progress.
type progressRequest struct {
	FileID       int64  `json:"fileId"`
	Progress     string `json:"progress"`
	ProgressType string `json:"progressType"`
}

// progressResponse is the JSON response for GET /api/reader/books/{bookId}/progress.
type progressResponse struct {
	FileID       int64  `json:"fileId"`
	Progress     string `json:"progress"`
	ProgressType string `json:"progressType"`
}

// handleGetProgress handles GET /api/reader/books/{bookId}/progress.
func (h *Handler) handleGetProgress(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	bookID, err := strconv.ParseInt(chi.URLParam(r, "bookId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	q := New(h.db)
	ctx := r.Context()

	progress, err := q.GetReadingProgress(ctx, GetReadingProgressParams{
		UserID: principal.UserID,
		BookID: bookID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "no progress found")
			return
		}
		h.logger.Error("get reading progress", "user_id", principal.UserID, "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := progressResponse{
		FileID: progress.BookFileID,
	}
	if progress.Progress.Valid {
		resp.Progress = progress.Progress.String
	}
	if progress.ProgressType.Valid {
		resp.ProgressType = progress.ProgressType.String
	}

	writeJSON(w, http.StatusOK, resp)
}

// handlePutProgress handles PUT /api/reader/books/{bookId}/progress.
func (h *Handler) handlePutProgress(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	bookID, err := strconv.ParseInt(chi.URLParam(r, "bookId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	var req progressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.FileID == 0 {
		writeError(w, http.StatusBadRequest, "fileId is required")
		return
	}

	q := New(h.db)
	ctx := r.Context()

	// Verify the file belongs to the book before saving progress.
	_, err = q.GetBookFileForReader(ctx, GetBookFileForReaderParams{
		ID:     req.FileID,
		BookID: bookID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		h.logger.Error("verify book file for progress", "file_id", req.FileID, "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := q.UpsertReadingProgress(ctx, UpsertReadingProgressParams{
		UserID:     principal.UserID,
		BookFileID: req.FileID,
		Progress:   sql.NullString{String: req.Progress, Valid: req.Progress != ""},
		ProgressType: sql.NullString{
			String: req.ProgressType,
			Valid:  req.ProgressType != "",
		},
	}); err != nil {
		h.logger.Error("upsert reading progress", "user_id", principal.UserID, "file_id", req.FileID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleGetSettings handles GET /api/reader/books/{bookId}/settings.
// Returns the epub_reader_setting for the authenticated user.
func (h *Handler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// bookId is accepted in the URL for future per-book settings, but currently
	// we return the global epub reader settings for the user.
	_, err := strconv.ParseInt(chi.URLParam(r, "bookId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	q := New(h.db)
	ctx := r.Context()

	settings, err := q.GetReaderSettings(ctx, principal.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No settings row yet — return empty object.
			writeJSON(w, http.StatusOK, map[string]any{})
			return
		}
		h.logger.Error("get reader settings", "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if !settings.EpubReaderSetting.Valid || settings.EpubReaderSetting.String == "" {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}

	// Parse the stored JSON and return it as-is.
	var parsed any
	if err := json.Unmarshal([]byte(settings.EpubReaderSetting.String), &parsed); err != nil {
		// Stored value is not valid JSON — return empty object.
		h.logger.Warn("epub reader setting is not valid JSON", "user_id", principal.UserID)
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}

	writeJSON(w, http.StatusOK, parsed)
}

// handlePutSettings handles PUT /api/reader/books/{bookId}/settings.
// Saves the epub reader settings for the authenticated user.
func (h *Handler) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	_, err := strconv.ParseInt(chi.URLParam(r, "bookId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	// Read the raw body — we store it as JSON text.
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	// Validate that the body is valid JSON.
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be valid JSON")
		return
	}

	q := New(h.db)
	ctx := r.Context()

	if err := q.UpsertEpubReaderSetting(ctx, UpsertEpubReaderSettingParams{
		UserID:            principal.UserID,
		EpubReaderSetting: sql.NullString{String: string(body), Valid: true},
	}); err != nil {
		h.logger.Error("upsert epub reader setting", "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleListComicPages handles GET /api/reader/books/{bookId}/files/{fileId}/pages.
// Returns the ordered list of image pages in the comic archive.
func (h *Handler) handleListComicPages(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	bookID, err := strconv.ParseInt(chi.URLParam(r, "bookId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	fileID, err := strconv.ParseInt(chi.URLParam(r, "fileId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid file id")
		return
	}

	q := New(h.db)
	ctx := r.Context()

	bookFile, err := q.GetBookFileForReader(ctx, GetBookFileForReaderParams{
		ID:     fileID,
		BookID: bookID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		h.logger.Error("get book file for comic pages", "file_id", fileID, "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if !hasLibraryAccess(principal, bookFile.LibraryID) {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	pages, err := ListComicPages(bookFile.FilePath, bookFile.Format)
	if err != nil {
		h.logger.Error("list comic pages", "file_id", fileID, "format", bookFile.Format, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, pages)
}

// handleGetComicPage handles GET /api/reader/books/{bookId}/files/{fileId}/pages/{pageIndex}.
// Returns the image bytes for a specific page.
func (h *Handler) handleGetComicPage(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	bookID, err := strconv.ParseInt(chi.URLParam(r, "bookId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	fileID, err := strconv.ParseInt(chi.URLParam(r, "fileId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid file id")
		return
	}

	pageIndex, err := strconv.Atoi(chi.URLParam(r, "pageIndex"))
	if err != nil || pageIndex < 0 {
		writeError(w, http.StatusBadRequest, "invalid page index")
		return
	}

	q := New(h.db)
	ctx := r.Context()

	bookFile, err := q.GetBookFileForReader(ctx, GetBookFileForReaderParams{
		ID:     fileID,
		BookID: bookID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		h.logger.Error("get book file for comic page", "file_id", fileID, "book_id", bookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if !hasLibraryAccess(principal, bookFile.LibraryID) {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	data, mimeType, err := GetComicPage(bookFile.FilePath, bookFile.Format, pageIndex)
	if err != nil {
		h.logger.Error("get comic page", "file_id", fileID, "page", pageIndex, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleGetAudiobookSettings handles GET /api/reader/books/{bookId}/audiobook-settings.
// Returns the audiobook reader settings for the authenticated user.
func (h *Handler) handleGetAudiobookSettings(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	_, err := strconv.ParseInt(chi.URLParam(r, "bookId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	q := New(h.db)
	ctx := r.Context()

	setting, err := q.GetAudiobookReaderSetting(ctx, principal.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]any{})
			return
		}
		h.logger.Error("get audiobook reader setting", "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if !setting.Valid || setting.String == "" {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}

	var parsed any
	if err := json.Unmarshal([]byte(setting.String), &parsed); err != nil {
		h.logger.Warn("audiobook reader setting is not valid JSON", "user_id", principal.UserID)
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}

	writeJSON(w, http.StatusOK, parsed)
}

// handlePutAudiobookSettings handles PUT /api/reader/books/{bookId}/audiobook-settings.
// Saves the audiobook reader settings for the authenticated user.
func (h *Handler) handlePutAudiobookSettings(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	_, err := strconv.ParseInt(chi.URLParam(r, "bookId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be valid JSON")
		return
	}

	q := New(h.db)
	ctx := r.Context()

	if err := q.UpsertAudiobookReaderSetting(ctx, UpsertAudiobookReaderSettingParams{
		UserID:                 principal.UserID,
		AudiobookReaderSetting: sql.NullString{String: string(body), Valid: true},
	}); err != nil {
		h.logger.Error("upsert audiobook reader setting", "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
