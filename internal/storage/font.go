package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// FontService manages custom fonts.
type FontService struct {
	db      *sql.DB
	dataDir string
	logger  *slog.Logger
}

// NewFontService creates a new FontService.
func NewFontService(db *sql.DB, dataDir string, logger *slog.Logger) *FontService {
	return &FontService{db: db, dataDir: dataDir, logger: logger}
}

// Font is a custom font record.
type Font struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	FilePath  string `json:"filePath"`
	Format    string `json:"format"`
	CreatedAt string `json:"createdAt"`
}

// UploadFont saves an uploaded font file and creates a database record.
func (s *FontService) UploadFont(ctx context.Context, name string, reader io.Reader, ext string) (int64, error) {
	// Validate format.
	format := strings.ToUpper(strings.TrimPrefix(ext, "."))
	switch format {
	case "TTF", "OTF", "WOFF", "WOFF2":
		// valid
	default:
		return 0, fmt.Errorf("unsupported font format: %s", format)
	}

	fontsDir := filepath.Join(s.dataDir, "fonts")
	if err := os.MkdirAll(fontsDir, 0755); err != nil {
		return 0, fmt.Errorf("create fonts dir: %w", err)
	}

	filePath := filepath.Join(fontsDir, fmt.Sprintf("%d%s", time.Now().UnixNano(), ext))
	f, err := os.Create(filePath)
	if err != nil {
		return 0, fmt.Errorf("create font file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return 0, fmt.Errorf("write font file: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		"INSERT INTO custom_font (name, file_path, format) VALUES (?, ?, ?)",
		name, filePath, format)
	if err != nil {
		_ = os.Remove(filePath)
		return 0, fmt.Errorf("insert font record: %w", err)
	}

	id, _ := res.LastInsertId()
	return id, nil
}

// ListFonts returns all custom fonts.
func (s *FontService) ListFonts(ctx context.Context) ([]Font, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, file_path, format, created_at FROM custom_font ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list fonts: %w", err)
	}
	defer rows.Close()

	var fonts []Font
	for rows.Next() {
		var f Font
		if err := rows.Scan(&f.ID, &f.Name, &f.FilePath, &f.Format, &f.CreatedAt); err != nil {
			return nil, err
		}
		fonts = append(fonts, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return fonts, nil
}

// DeleteFont removes a font by ID.
func (s *FontService) DeleteFont(ctx context.Context, id int64) error {
	var filePath string
	err := s.db.QueryRowContext(ctx,
		"SELECT file_path FROM custom_font WHERE id = ?", id).Scan(&filePath)
	if err != nil {
		return fmt.Errorf("get font path: %w", err)
	}

	_, err = s.db.ExecContext(ctx, "DELETE FROM custom_font WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete font record: %w", err)
	}

	_ = os.Remove(filePath)
	return nil
}

// HandleListFonts handles GET /api/fonts.
func (h *Handler) HandleListFonts(w http.ResponseWriter, r *http.Request) {
	fonts, err := h.fontSvc.ListFonts(r.Context())
	if err != nil {
		h.logger.Error("list fonts", "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(fonts)
}

// HandleUploadFont handles POST /api/fonts.
func (h *Handler) HandleUploadFont(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, `{"error":"invalid multipart form"}`, http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"file is required"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	name := r.FormValue("name")
	if name == "" {
		name = header.Filename
	}

	ext := filepath.Ext(header.Filename)
	id, err := h.fontSvc.UploadFont(r.Context(), name, file, ext)
	if err != nil {
		h.logger.Error("upload font", "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]int64{"id": id})
}

// HandleDeleteFont handles DELETE /api/fonts/{id}.
func (h *Handler) HandleDeleteFont(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid font id"}`, http.StatusBadRequest)
		return
	}

	if err := h.fontSvc.DeleteFont(r.Context(), id); err != nil {
		h.logger.Error("delete font", "id", id, "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleServeFont handles GET /api/fonts/{id}/file.
func (h *Handler) HandleServeFont(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid font id"}`, http.StatusBadRequest)
		return
	}

	var filePath string
	err = h.db.QueryRowContext(r.Context(),
		"SELECT file_path FROM custom_font WHERE id = ?", id).Scan(&filePath)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, `{"error":"font not found"}`, http.StatusNotFound)
			return
		}
		h.logger.Error("get font path", "id", id, "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	// Determine content type based on extension.
	ext := strings.ToUpper(filepath.Ext(filePath))
	contentType := "application/octet-stream"
	switch ext {
	case ".TTF":
		contentType = "font/ttf"
	case ".OTF":
		contentType = "font/otf"
	case ".WOFF":
		contentType = "font/woff"
	case ".WOFF2":
		contentType = "font/woff2"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, filePath)
}
