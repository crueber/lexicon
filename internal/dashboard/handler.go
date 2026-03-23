package dashboard

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/crueber/lexicon/internal/auth"
	"github.com/crueber/lexicon/internal/user"
)

// Handler handles HTTP requests for the dashboard.
type Handler struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewHandler creates a new dashboard Handler.
func NewHandler(db *sql.DB, logger *slog.Logger) *Handler {
	return &Handler{
		db:     db,
		logger: logger,
	}
}

// Routes registers all dashboard routes on the given router.
// RequireAuth must already be applied by the caller.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.handleGet)
	r.Get("/settings", h.handleGetSettings)
	r.Put("/settings", h.handlePutSettings)
}

// dashboardRowType represents the type of a dashboard row.
type dashboardRowType string

const (
	rowTypeContinueReading dashboardRowType = "CONTINUE_READING"
	rowTypeRecentlyAdded   dashboardRowType = "RECENTLY_ADDED"
	rowTypeRandomPicks     dashboardRowType = "RANDOM_PICKS"
)

// dashboardRowConfig is the per-row configuration stored in user_settings.
type dashboardRowConfig struct {
	Type    dashboardRowType `json:"type"`
	Enabled bool             `json:"enabled"`
	Title   string           `json:"title"`
}

// defaultRowConfigs returns the default dashboard row configuration.
func defaultRowConfigs() []dashboardRowConfig {
	return []dashboardRowConfig{
		{Type: rowTypeContinueReading, Enabled: true, Title: "Continue Reading"},
		{Type: rowTypeRecentlyAdded, Enabled: true, Title: "Recently Added"},
		{Type: rowTypeRandomPicks, Enabled: true, Title: "Random Picks"},
	}
}

// dashboardBook is the book representation used in dashboard rows.
type dashboardBook struct {
	ID        int64    `json:"id"`
	LibraryID int64    `json:"libraryId"`
	BookType  string   `json:"bookType"`
	Title     *string  `json:"title"`
	Authors   []string `json:"authors"`
	CoverPath *string  `json:"coverPath"`
	AddedDate *string  `json:"addedDate"`
}

// dashboardRow is a single row in the dashboard response.
type dashboardRow struct {
	Type  dashboardRowType `json:"type"`
	Title string           `json:"title"`
	Books []dashboardBook  `json:"books"`
}

// dashboardStats holds aggregate statistics for the dashboard.
type dashboardStats struct {
	TotalBooks         int64 `json:"totalBooks"`
	TotalLibraries     int64 `json:"totalLibraries"`
	BooksReadThisMonth int64 `json:"booksReadThisMonth"`
	TotalReadingTime   int64 `json:"totalReadingTime"`
}

// dashboardResponse is the full dashboard API response.
type dashboardResponse struct {
	Rows  []dashboardRow `json:"rows"`
	Stats dashboardStats `json:"stats"`
}

// dashboardSettingsResponse is the response for GET/PUT /api/dashboard/settings.
type dashboardSettingsResponse struct {
	Rows []dashboardRowConfig `json:"rows"`
}

// buildInClause builds a safe SQL IN clause for a slice of int64 IDs.
// It returns the clause string (e.g. "IN (?,?,?)") and the args slice.
// If ids is empty, it returns "IN (NULL)" to match nothing.
func buildInClause(ids []int64) (string, []any) {
	if len(ids) == 0 {
		return "IN (NULL)", nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1] // trim trailing comma
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return fmt.Sprintf("IN (%s)", placeholders), args
}

// getLibraryIDs returns the library IDs accessible to the principal.
// Admins get all library IDs; regular users get their permitted IDs from the JWT.
func (h *Handler) getLibraryIDs(ctx context.Context, principal *auth.Principal) ([]int64, error) {
	if principal.IsAdmin() {
		rows, err := h.db.QueryContext(ctx, "SELECT id FROM library")
		if err != nil {
			return nil, fmt.Errorf("list all libraries: %w", err)
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, fmt.Errorf("scan library id: %w", err)
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate libraries: %w", err)
		}
		return ids, nil
	}
	return principal.LibraryIDs, nil
}

// handleGet handles GET /api/dashboard.
func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	ctx := r.Context()

	// Load user's dashboard settings to determine which rows to show.
	uq := user.New(h.db)
	settings, err := uq.GetUserSettings(ctx, principal.UserID)
	if err != nil && err != sql.ErrNoRows {
		h.logger.Error("get user settings for dashboard", "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Parse row configuration from user settings.
	rowConfigs := defaultRowConfigs()
	if settings.DashboardSetting.Valid && settings.DashboardSetting.String != "" {
		var stored struct {
			Rows []dashboardRowConfig `json:"rows"`
		}
		if err := json.Unmarshal([]byte(settings.DashboardSetting.String), &stored); err == nil && len(stored.Rows) > 0 {
			rowConfigs = stored.Rows
		}
	}

	// Determine accessible library IDs.
	libraryIDs, err := h.getLibraryIDs(ctx, principal)
	if err != nil {
		h.logger.Error("get library ids", "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	inClause, inArgs := buildInClause(libraryIDs)

	// Build each enabled row.
	dashRows := make([]dashboardRow, 0, len(rowConfigs))
	for _, cfg := range rowConfigs {
		if !cfg.Enabled {
			continue
		}

		var books []dashboardBook
		var fetchErr error

		switch cfg.Type {
		case rowTypeRecentlyAdded:
			books, fetchErr = h.fetchRecentlyAdded(ctx, inClause, inArgs)
		case rowTypeContinueReading:
			books, fetchErr = h.fetchInProgress(ctx, principal.UserID)
		case rowTypeRandomPicks:
			books, fetchErr = h.fetchRandom(ctx, inClause, inArgs)
		default:
			continue
		}

		if fetchErr != nil {
			h.logger.Warn("fetch dashboard row", "type", cfg.Type, "error", fetchErr)
			books = []dashboardBook{}
		}
		if books == nil {
			books = []dashboardBook{}
		}

		dashRows = append(dashRows, dashboardRow{
			Type:  cfg.Type,
			Title: cfg.Title,
			Books: books,
		})
	}

	// Compute stats.
	stats, err := h.computeStats(ctx, principal, inClause, inArgs)
	if err != nil {
		h.logger.Warn("compute dashboard stats", "error", err)
		// Non-fatal: return zero stats.
		stats = dashboardStats{}
	}

	writeJSON(w, http.StatusOK, dashboardResponse{
		Rows:  dashRows,
		Stats: stats,
	})
}

// fetchRecentlyAdded returns the most recently added books across the given libraries.
func (h *Handler) fetchRecentlyAdded(ctx context.Context, inClause string, inArgs []any) ([]dashboardBook, error) {
	query := fmt.Sprintf(`
		SELECT b.id, b.library_id, b.book_type, b.added_date,
		       bm.title, bm.cover_path
		FROM book b
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		WHERE b.library_id %s
		ORDER BY b.added_date DESC, b.id DESC
		LIMIT 20`, inClause)

	return h.queryBooks(ctx, query, inArgs)
}

// fetchInProgress returns books the user has reading progress on, most recently read first.
func (h *Handler) fetchInProgress(ctx context.Context, userID int64) ([]dashboardBook, error) {
	query := `
		SELECT b.id, b.library_id, b.book_type, b.added_date,
		       bm.title, bm.cover_path
		FROM user_book_file_progress p
		JOIN book_file bf ON bf.id = p.book_file_id
		JOIN book b ON b.id = bf.book_id
		LEFT JOIN book_metadata bm ON bm.book_id = b.id
		WHERE p.user_id = ?
		ORDER BY p.updated_at DESC
		LIMIT 20`

	return h.queryBooks(ctx, query, []any{userID})
}

// fetchRandom returns a random selection of books across the given libraries.
func (h *Handler) fetchRandom(ctx context.Context, inClause string, inArgs []any) ([]dashboardBook, error) {
	query := fmt.Sprintf(`
		SELECT b.id, b.library_id, b.book_type, b.added_date,
		       bm.title, bm.cover_path
		FROM book b
		LEFT JOIN book_metadata bm ON b.id = bm.book_id
		WHERE b.library_id %s
		ORDER BY RANDOM()
		LIMIT 20`, inClause)

	return h.queryBooks(ctx, query, inArgs)
}

// queryBooks executes a book query and returns the results as dashboardBook slices.
// Authors are fetched per-book.
func (h *Handler) queryBooks(ctx context.Context, query string, args []any) ([]dashboardBook, error) {
	rows, err := h.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query books: %w", err)
	}
	defer rows.Close()

	var books []dashboardBook
	for rows.Next() {
		var (
			id        int64
			libraryID int64
			bookType  string
			addedDate sql.NullString
			title     sql.NullString
			coverPath sql.NullString
		)
		if err := rows.Scan(&id, &libraryID, &bookType, &addedDate, &title, &coverPath); err != nil {
			return nil, fmt.Errorf("scan book row: %w", err)
		}

		b := dashboardBook{
			ID:        id,
			LibraryID: libraryID,
			BookType:  bookType,
			Authors:   []string{},
		}
		if title.Valid {
			b.Title = &title.String
		}
		if coverPath.Valid {
			b.CoverPath = &coverPath.String
		}
		if addedDate.Valid {
			b.AddedDate = &addedDate.String
		}
		books = append(books, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate book rows: %w", err)
	}

	// Fetch authors for each book.
	for i := range books {
		authors, err := h.fetchAuthors(ctx, books[i].ID)
		if err != nil {
			h.logger.Warn("fetch authors for dashboard book", "book_id", books[i].ID, "error", err)
			continue
		}
		books[i].Authors = authors
	}

	return books, nil
}

// fetchAuthors returns the author names for a given book ID.
func (h *Handler) fetchAuthors(ctx context.Context, bookID int64) ([]string, error) {
	rows, err := h.db.QueryContext(ctx,
		`SELECT a.name FROM author a JOIN book_author ba ON a.id = ba.author_id WHERE ba.book_id = ? ORDER BY ba.sort_order`,
		bookID)
	if err != nil {
		return nil, fmt.Errorf("query authors: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan author name: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate author rows: %w", err)
	}
	return names, nil
}

// computeStats computes aggregate statistics for the dashboard.
func (h *Handler) computeStats(ctx context.Context, principal *auth.Principal, inClause string, inArgs []any) (dashboardStats, error) {
	var stats dashboardStats

	// Total books across accessible libraries.
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM book WHERE library_id %s`, inClause)
	if err := h.db.QueryRowContext(ctx, countQuery, inArgs...).Scan(&stats.TotalBooks); err != nil {
		return stats, fmt.Errorf("count books: %w", err)
	}

	// Total libraries (admins see all; users see their permitted count).
	if principal.IsAdmin() {
		if err := h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM library`).Scan(&stats.TotalLibraries); err != nil {
			return stats, fmt.Errorf("count libraries: %w", err)
		}
	} else {
		stats.TotalLibraries = int64(len(principal.LibraryIDs))
	}

	// Books read this month: distinct books with reading sessions started this calendar month.
	if err := h.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT book_id) FROM reading_sessions
		WHERE user_id = ? AND strftime('%Y-%m', started_at) = strftime('%Y-%m', 'now')`,
		principal.UserID).Scan(&stats.BooksReadThisMonth); err != nil {
		// reading_sessions may be empty; treat as 0.
		stats.BooksReadThisMonth = 0
	}

	// Total reading time in seconds from reading_sessions.
	var totalSecs sql.NullInt64
	if err := h.db.QueryRowContext(ctx,
		`SELECT SUM(duration_secs) FROM reading_sessions WHERE user_id = ?`,
		principal.UserID).Scan(&totalSecs); err == nil && totalSecs.Valid {
		stats.TotalReadingTime = totalSecs.Int64
	}

	return stats, nil
}

// handleGetSettings handles GET /api/dashboard/settings.
func (h *Handler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	ctx := r.Context()
	uq := user.New(h.db)
	settings, err := uq.GetUserSettings(ctx, principal.UserID)
	if err != nil && err != sql.ErrNoRows {
		h.logger.Error("get user settings", "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rowConfigs := defaultRowConfigs()
	if settings.DashboardSetting.Valid && settings.DashboardSetting.String != "" {
		var stored struct {
			Rows []dashboardRowConfig `json:"rows"`
		}
		if err := json.Unmarshal([]byte(settings.DashboardSetting.String), &stored); err == nil && len(stored.Rows) > 0 {
			rowConfigs = stored.Rows
		}
	}

	writeJSON(w, http.StatusOK, dashboardSettingsResponse{Rows: rowConfigs})
}

// handlePutSettings handles PUT /api/dashboard/settings.
func (h *Handler) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req struct {
		Rows []dashboardRowConfig `json:"rows"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate row types.
	for _, row := range req.Rows {
		switch row.Type {
		case rowTypeContinueReading, rowTypeRecentlyAdded, rowTypeRandomPicks:
			// valid
		default:
			writeError(w, http.StatusBadRequest, "invalid row type: "+string(row.Type))
			return
		}
	}

	// Serialize to JSON for storage.
	stored := struct {
		Rows []dashboardRowConfig `json:"rows"`
	}{Rows: req.Rows}
	data, err := json.Marshal(stored)
	if err != nil {
		h.logger.Error("marshal dashboard settings", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	ctx := r.Context()
	uq := user.New(h.db)

	// Ensure user_settings row exists before updating.
	if _, err := uq.GetUserSettings(ctx, principal.UserID); err == sql.ErrNoRows {
		if err := uq.UpsertUserSettings(ctx, user.UpsertUserSettingsParams{
			UserID: principal.UserID,
		}); err != nil {
			h.logger.Error("upsert user settings", "user_id", principal.UserID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	if err := uq.UpdateDashboardSetting(ctx, user.UpdateDashboardSettingParams{
		DashboardSetting: sql.NullString{String: string(data), Valid: true},
		UserID:           principal.UserID,
	}); err != nil {
		h.logger.Error("update dashboard setting", "user_id", principal.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, dashboardSettingsResponse{Rows: req.Rows})
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
