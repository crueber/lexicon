package library

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/crueber/lexicon/internal/auth"
)

// Handler handles HTTP requests for library management.
type Handler struct {
	svc     *Service
	scanner *Scanner
	logger  *slog.Logger
}

// NewHandler creates a new library Handler.
func NewHandler(svc *Service, scanner *Scanner, logger *slog.Logger) *Handler {
	return &Handler{
		svc:     svc,
		scanner: scanner,
		logger:  logger,
	}
}

// Routes registers all library routes on the given router.
// RequireAuth must already be applied by the caller.
// Admin-only routes apply RequireAdmin middleware internally.
func (h *Handler) Routes(r chi.Router) {
	// All authenticated users can list libraries.
	r.Get("/", h.handleList)

	// Admin-only: create library.
	r.With(auth.RequireAdmin()).Post("/", h.handleCreate)

	r.Route("/{id}", func(r chi.Router) {
		// All authenticated users can read.
		r.Get("/", h.handleGet)
		r.With(auth.RequireAdmin()).Post("/scan", h.handleScan)

		// Admin-only: update and delete.
		r.With(auth.RequireAdmin()).Put("/", h.handleUpdate)
		r.With(auth.RequireAdmin()).Delete("/", h.handleDelete)

		r.Route("/paths", func(r chi.Router) {
			// All authenticated users can list paths.
			r.Get("/", h.handleListPaths)

			// Admin-only: add and remove paths.
			r.With(auth.RequireAdmin()).Post("/", h.handleAddPath)
			r.With(auth.RequireAdmin()).Delete("/{pathId}", h.handleRemovePath)
		})
	})
}

// --- Request/Response types ---

// LibraryResponse is the JSON representation of a library.
type LibraryResponse struct {
	ID                int64          `json:"id"`
	Name              string         `json:"name"`
	Icon              *string        `json:"icon,omitempty"`
	IconColor         *string        `json:"iconColor,omitempty"`
	OrganizationMode  string         `json:"organizationMode"`
	FileNamingPattern *string        `json:"fileNamingPattern,omitempty"`
	CreatedAt         string         `json:"createdAt"`
	Paths             []PathResponse `json:"paths"`
}

// PathResponse is the JSON representation of a library path.
type PathResponse struct {
	ID   int64  `json:"id"`
	Path string `json:"path"`
}

// CreateLibraryRequest is the JSON body for POST /api/libraries.
type CreateLibraryRequest struct {
	Name              string   `json:"name"`
	Icon              *string  `json:"icon"`
	IconColor         *string  `json:"iconColor"`
	OrganizationMode  string   `json:"organizationMode"`
	FileNamingPattern *string  `json:"fileNamingPattern"`
	Paths             []string `json:"paths"`
}

// UpdateLibraryRequest is the JSON body for PUT /api/libraries/{id}.
type UpdateLibraryRequest struct {
	Name              string  `json:"name"`
	Icon              *string `json:"icon"`
	IconColor         *string `json:"iconColor"`
	OrganizationMode  string  `json:"organizationMode"`
	FileNamingPattern *string `json:"fileNamingPattern"`
}

// AddPathRequest is the JSON body for POST /api/libraries/{id}/paths.
type AddPathRequest struct {
	Path string `json:"path"`
}

// --- Handlers ---

// handleList handles GET /api/libraries.
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	p := auth.PrincipalFromContext(r.Context())
	if p == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	libs, err := h.svc.ListForUser(r.Context(), p)
	if err != nil {
		h.logger.Error("list libraries", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Fetch paths for each library.
	responses := make([]LibraryResponse, 0, len(libs))
	for _, lib := range libs {
		paths, err := h.svc.ListPaths(r.Context(), lib.ID)
		if err != nil {
			h.logger.Error("list paths for library", "library_id", lib.ID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		responses = append(responses, toLibraryResponse(lib, paths))
	}

	writeJSON(w, http.StatusOK, responses)
}

// handleCreate handles POST /api/libraries (admin only).
func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateLibraryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !validOrganizationMode(req.OrganizationMode) {
		writeError(w, http.StatusBadRequest, "organizationMode must be BOOK_PER_FILE or BOOK_PER_FOLDER")
		return
	}

	lib, err := h.svc.Create(r.Context(), CreateParams{
		Name:              req.Name,
		Icon:              req.Icon,
		IconColor:         req.IconColor,
		OrganizationMode:  req.OrganizationMode,
		FileNamingPattern: req.FileNamingPattern,
		Paths:             req.Paths,
	})
	if err != nil {
		h.logger.Error("create library", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	paths, err := h.svc.ListPaths(r.Context(), lib.ID)
	if err != nil {
		h.logger.Error("list paths after create", "library_id", lib.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.logger.Info("library created", "library_id", lib.ID, "name", lib.Name)
	writeJSON(w, http.StatusCreated, toLibraryResponse(*lib, paths))
}

// handleGet handles GET /api/libraries/{id}.
func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	p := auth.PrincipalFromContext(r.Context())
	if p == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	lib, err := h.svc.GetByID(r.Context(), id, p)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "library not found")
			return
		}
		if errors.Is(err, ErrAccessDenied) {
			writeError(w, http.StatusForbidden, "access denied")
			return
		}
		h.logger.Error("get library", "library_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	paths, err := h.svc.ListPaths(r.Context(), lib.ID)
	if err != nil {
		h.logger.Error("list paths for library", "library_id", lib.ID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, toLibraryResponse(*lib, paths))
}

// handleUpdate handles PUT /api/libraries/{id} (admin only).
func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	var req UpdateLibraryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !validOrganizationMode(req.OrganizationMode) {
		writeError(w, http.StatusBadRequest, "organizationMode must be BOOK_PER_FILE or BOOK_PER_FOLDER")
		return
	}

	if err := h.svc.Update(r.Context(), id, UpdateParams{
		Name:              req.Name,
		Icon:              req.Icon,
		IconColor:         req.IconColor,
		OrganizationMode:  req.OrganizationMode,
		FileNamingPattern: req.FileNamingPattern,
	}); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "library not found")
			return
		}
		h.logger.Error("update library", "library_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.logger.Info("library updated", "library_id", id)
	w.WriteHeader(http.StatusNoContent)
}

// handleDelete handles DELETE /api/libraries/{id} (admin only).
func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "library not found")
			return
		}
		h.logger.Error("delete library", "library_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.logger.Info("library deleted", "library_id", id)
	w.WriteHeader(http.StatusNoContent)
}

// ScanResponse is the JSON response for POST /api/libraries/{id}/scan.
type ScanResponse struct {
	BooksAdded   int `json:"booksAdded"`
	FilesAdded   int `json:"filesAdded"`
	FilesUpdated int `json:"filesUpdated"`
	ErrorCount   int `json:"errorCount"`
}

// handleScan handles POST /api/libraries/{id}/scan.
func (h *Handler) handleScan(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	p := auth.PrincipalFromContext(r.Context())
	if p == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Verify the library exists and the user has access.
	lib, err := h.svc.GetByID(r.Context(), id, p)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "library not found")
			return
		}
		if errors.Is(err, ErrAccessDenied) {
			writeError(w, http.StatusForbidden, "access denied")
			return
		}
		h.logger.Error("get library for scan", "library_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	paths, err := h.svc.ListPaths(r.Context(), id)
	if err != nil {
		h.logger.Error("list paths for scan", "library_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	result, err := h.scanner.ScanLibrary(r.Context(), *lib, paths)
	if err != nil {
		h.logger.Error("scan library", "library_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "scan failed")
		return
	}

	h.logger.Info("library scan complete",
		"library_id", id,
		"books_added", result.BooksAdded,
		"files_added", result.FilesAdded,
		"errors", len(result.Errors),
	)

	writeJSON(w, http.StatusOK, ScanResponse{
		BooksAdded:   result.BooksAdded,
		FilesAdded:   result.FilesAdded,
		FilesUpdated: result.FilesUpdated,
		ErrorCount:   len(result.Errors),
	})
}

// handleListPaths handles GET /api/libraries/{id}/paths.
func (h *Handler) handleListPaths(w http.ResponseWriter, r *http.Request) {
	p := auth.PrincipalFromContext(r.Context())
	if p == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	// Verify access.
	if _, err := h.svc.GetByID(r.Context(), id, p); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "library not found")
			return
		}
		if errors.Is(err, ErrAccessDenied) {
			writeError(w, http.StatusForbidden, "access denied")
			return
		}
		h.logger.Error("get library for list paths", "library_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	paths, err := h.svc.ListPaths(r.Context(), id)
	if err != nil {
		h.logger.Error("list library paths", "library_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	responses := make([]PathResponse, len(paths))
	for i, lp := range paths {
		responses[i] = PathResponse{ID: lp.ID, Path: lp.Path}
	}

	writeJSON(w, http.StatusOK, responses)
}

// handleAddPath handles POST /api/libraries/{id}/paths (admin only).
func (h *Handler) handleAddPath(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	var req AddPathRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	lp, err := h.svc.AddPath(r.Context(), id, req.Path)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "library not found")
			return
		}
		h.logger.Error("add library path", "library_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.logger.Info("library path added", "library_id", id, "path", lp.Path)
	writeJSON(w, http.StatusCreated, PathResponse{ID: lp.ID, Path: lp.Path})
}

// handleRemovePath handles DELETE /api/libraries/{id}/paths/{pathId} (admin only).
func (h *Handler) handleRemovePath(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	pathID, ok := parseID(w, r, "pathId")
	if !ok {
		return
	}

	if err := h.svc.RemovePath(r.Context(), id, pathID); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "path not found")
			return
		}
		h.logger.Error("remove library path", "library_id", id, "path_id", pathID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.logger.Info("library path removed", "library_id", id, "path_id", pathID)
	w.WriteHeader(http.StatusNoContent)
}

// --- Helpers ---

// toLibraryResponse converts a Library model and its paths to a LibraryResponse.
func toLibraryResponse(lib Library, paths []LibraryPath) LibraryResponse {
	pathResponses := make([]PathResponse, len(paths))
	for i, p := range paths {
		pathResponses[i] = PathResponse{ID: p.ID, Path: p.Path}
	}

	resp := LibraryResponse{
		ID:               lib.ID,
		Name:             lib.Name,
		OrganizationMode: lib.OrganizationMode,
		CreatedAt:        lib.CreatedAt,
		Paths:            pathResponses,
	}

	if lib.Icon.Valid {
		resp.Icon = &lib.Icon.String
	}
	if lib.IconColor.Valid {
		resp.IconColor = &lib.IconColor.String
	}
	if lib.FileNamingPattern.Valid {
		resp.FileNamingPattern = &lib.FileNamingPattern.String
	}

	return resp
}

// validOrganizationMode returns true if the mode is a valid organization mode.
func validOrganizationMode(mode string) bool {
	return mode == "BOOK_PER_FILE" || mode == "BOOK_PER_FOLDER"
}

// parseID extracts and parses an integer URL parameter by name.
// It writes an error response and returns false if parsing fails.
func parseID(w http.ResponseWriter, r *http.Request, param string) (int64, bool) {
	raw := chi.URLParam(r, param)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid "+param)
		return 0, false
	}
	return id, true
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
