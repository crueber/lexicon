package bookdrop

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"context"

	"github.com/crueber/lexicon/internal/audit"
	"github.com/crueber/lexicon/internal/auth"
	"github.com/crueber/lexicon/internal/library"
)

// Handler handles HTTP requests for BookDrop management.
type Handler struct {
	svc      *Service
	db       *sql.DB
	logger   *slog.Logger
	auditSvc *audit.Service
}

// NewHandler creates a new bookdrop Handler.
func NewHandler(svc *Service, db *sql.DB, logger *slog.Logger) *Handler {
	return &Handler{
		svc:    svc,
		db:     db,
		logger: logger,
	}
}

// WithAuditService sets the audit service for logging bookdrop events.
func (h *Handler) WithAuditService(svc *audit.Service) {
	h.auditSvc = svc
}

// Routes registers all bookdrop routes on the given router.
// RequireAuth must already be applied by the caller.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.handleList)
	r.Post("/import-all", h.handleImportAll)
	r.Route("/{id}", func(r chi.Router) {
		r.Post("/import", h.handleImport)
		r.Post("/reject", h.handleReject)
	})
}

// --- Request/Response types ---

// ImportRequest is the JSON body for importing a bookdrop file.
type ImportRequest struct {
	LibraryID *int64 `json:"libraryId"`
}

// ImportResponse is the JSON response for a successful import.
type ImportResponse struct {
	BookID int64 `json:"bookId"`
}

// BookdropFileResponse is the JSON representation of a bookdrop file.
type BookdropFileResponse struct {
	ID                 int64   `json:"id"`
	OriginalFilename   string  `json:"originalFilename"`
	FileSize           int64   `json:"fileSize"`
	Status             string  `json:"status"`
	ExtractedTitle     *string `json:"extractedTitle,omitempty"`
	ExtractedAuthors   *string `json:"extractedAuthors,omitempty"`
	ExtractedCoverPath *string `json:"extractedCoverPath,omitempty"`
	CreatedAt          string  `json:"createdAt"`
}

// --- Handlers ---

// handleList handles GET /api/bookdrop/files.
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	files, err := h.svc.ListPending(r.Context())
	if err != nil {
		h.logger.Error("list pending bookdrop files", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]BookdropFileResponse, len(files))
	for i, f := range files {
		resp[i] = toBookdropFileResponse(f)
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleImport handles POST /api/bookdrop/files/{id}/import.
func (h *Handler) handleImport(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	var req ImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	libraryID := req.LibraryID
	if libraryID == nil {
		defaultID, err := h.firstLibraryID(r.Context())
		if err != nil {
			writeError(w, http.StatusBadRequest, "no libraries exist")
			return
		}
		libraryID = &defaultID
	}

	bookID, err := h.svc.ImportFile(r.Context(), id, *libraryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "bookdrop file not found")
			return
		}
		h.logger.Error("import bookdrop file", "id", id, "library_id", *libraryID, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.logger.Info("bookdrop file imported via api", "id", id, "book_id", bookID)
	if h.auditSvc != nil {
		p := auth.PrincipalFromContext(r.Context())
		var userID int64
		if p != nil {
			userID = p.UserID
		}
		ip := r.RemoteAddr
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.Split(xff, ",")[0]
		}
		h.auditSvc.Log(r.Context(), audit.LogParams{
			UserID:       &userID,
			Action:       audit.ActionBookdropImported,
			ResourceType: "bookdrop",
			ResourceID:   &id,
			Details: map[string]any{
				"library_id": *libraryID,
				"book_id":    bookID,
			},
			IPAddress: ip,
		})
	}

	writeJSON(w, http.StatusOK, ImportResponse{BookID: bookID})
}

// handleReject handles POST /api/bookdrop/files/{id}/reject.
func (h *Handler) handleReject(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	if err := h.svc.RejectFile(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "bookdrop file not found")
			return
		}
		h.logger.Error("reject bookdrop file", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleImportAll handles POST /api/bookdrop/files/import-all.
func (h *Handler) handleImportAll(w http.ResponseWriter, r *http.Request) {
	var req ImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	libraryID := req.LibraryID
	if libraryID == nil {
		defaultID, err := h.firstLibraryID(r.Context())
		if err != nil {
			writeError(w, http.StatusBadRequest, "no libraries exist")
			return
		}
		libraryID = &defaultID
	}

	files, err := h.svc.ListPending(r.Context())
	if err != nil {
		h.logger.Error("list pending for import-all", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	imported := 0
	for _, f := range files {
		_, err := h.svc.ImportFile(r.Context(), f.ID, *libraryID)
		if err != nil {
			h.logger.Warn("import-all failed for file", "id", f.ID, "error", err)
			continue
		}
		imported++

		if h.auditSvc != nil {
			p := auth.PrincipalFromContext(r.Context())
			var userID int64
			if p != nil {
				userID = p.UserID
			}
			ip := r.RemoteAddr
			if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				ip = strings.Split(xff, ",")[0]
			}
			h.auditSvc.Log(r.Context(), audit.LogParams{
				UserID:       &userID,
				Action:       audit.ActionBookdropImported,
				ResourceType: "bookdrop",
				ResourceID:   &f.ID,
				Details: map[string]any{
					"library_id": *libraryID,
				},
				IPAddress: ip,
			})
		}
	}

	h.logger.Info("bookdrop import-all complete", "library_id", *libraryID, "imported", imported, "total", len(files))
	writeJSON(w, http.StatusOK, map[string]any{
		"imported": imported,
		"total":    len(files),
		"errors":   len(files) - imported,
	})
}

// --- Helpers ---

// firstLibraryID returns the ID of the first library in the database.
func (h *Handler) firstLibraryID(ctx context.Context) (int64, error) {
	q := library.New(h.db)
	libs, err := q.ListLibraries(ctx)
	if err != nil {
		return 0, err
	}
	if len(libs) == 0 {
		return 0, fmt.Errorf("no libraries exist")
	}
	return libs[0].ID, nil
}

func toBookdropFileResponse(f BookdropFile) BookdropFileResponse {
	resp := BookdropFileResponse{
		ID:               f.ID,
		OriginalFilename: f.OriginalFilename,
		FileSize:         f.FileSize,
		Status:           f.Status,
		CreatedAt:        f.CreatedAt,
	}
	if f.ExtractedTitle.Valid {
		resp.ExtractedTitle = &f.ExtractedTitle.String
	}
	if f.ExtractedAuthors.Valid {
		resp.ExtractedAuthors = &f.ExtractedAuthors.String
	}
	if f.ExtractedCoverPath.Valid {
		resp.ExtractedCoverPath = &f.ExtractedCoverPath.String
	}
	return resp
}

func parseID(w http.ResponseWriter, r *http.Request, param string) (int64, bool) {
	raw := chi.URLParam(r, param)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid "+param)
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
