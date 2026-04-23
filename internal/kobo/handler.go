// Package kobo implements the Kobo store API proxy for device sync.
//
// Kobo devices are configured with a custom store URL pointing to Lexicon.
// Authentication uses a per-user token stored in app_settings as
// "kobo_token_{userID}". The token is sent by the device in the
// X-Kobo-UserKey header.
package kobo

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/crueber/lexicon/internal/book"
	"github.com/crueber/lexicon/internal/contentrestriction"
	"github.com/crueber/lexicon/internal/user"
)

// Handler handles Kobo store API proxy requests.
type Handler struct {
	db                    *sql.DB
	dataDir               string
	logger                *slog.Logger
	principalExtractor    func(*http.Request) (int64, bool)
	contentRestrictionSvc *contentrestriction.Service
}

// Compile-time interface check.
var _ http.Handler = (*Handler)(nil)

// NewHandler creates a new Kobo Handler.
func NewHandler(db *sql.DB, dataDir string, logger *slog.Logger) *Handler {
	return &Handler{
		db:      db,
		dataDir: dataDir,
		logger:  logger,
	}
}

// WithPrincipalExtractor sets the function used to extract the user ID from
// an authenticated request. This avoids an import cycle between kobo and auth.
// Must be called before serving token requests.
func (h *Handler) WithPrincipalExtractor(fn func(*http.Request) (int64, bool)) {
	h.principalExtractor = fn
}

// WithContentRestrictionService sets the content restriction service for filtering Kobo sync results.
func (h *Handler) WithContentRestrictionService(svc *contentrestriction.Service) {
	h.contentRestrictionSvc = svc
}

// ServeHTTP implements http.Handler (required for compile-time check).
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

// Routes registers all Kobo store API proxy routes.
// No JWT middleware is applied — Kobo uses X-Kobo-UserKey token auth.
func (h *Handler) Routes(r chi.Router) {
	r.Use(h.koboAuth)

	r.Get("/v1/initialization", h.handleInitialization)
	r.Get("/v1/library/sync", h.handleLibrarySync)
	r.Get("/v1/library/{contentId}/metadata", h.handleBookMetadata)
	r.Get("/v1/library/{contentId}/download", h.handleDownload)
	r.Post("/v1/library/sync/reading-state", h.handleSyncReadingState)
	r.Get("/v1/user/profile", h.handleUserProfile)
	r.Get("/v1/user/wishlist", h.handleWishlist)
	r.Get("/v1/products/prices", h.handlePrices)
	r.Get("/v1/products/recommendations", h.handleRecommendations)
}

// TokenRoutes registers the token management API endpoint.
// RequireAuth must already be applied by the caller.
func (h *Handler) TokenRoutes(r chi.Router) {
	r.Post("/token", h.handleGenerateToken)
}

// koboAuth is middleware that validates the X-Kobo-UserKey header against
// stored tokens and injects the user ID into the request context.
func (h *Handler) koboAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Kobo-UserKey")
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing X-Kobo-UserKey header")
			return
		}

		userID, err := h.findUserByToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) || errors.Is(err, errTokenNotFound) {
				writeError(w, http.StatusUnauthorized, "invalid kobo token")
				return
			}
			h.logger.Error("kobo auth lookup", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		ctx := context.WithValue(r.Context(), koboUserIDKey{}, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// koboUserIDKey is the context key for the authenticated Kobo user ID.
type koboUserIDKey struct{}

// errTokenNotFound is returned when no user has the given token.
var errTokenNotFound = errors.New("token not found")

// findUserByToken searches app_settings for a matching kobo token.
func (h *Handler) findUserByToken(ctx context.Context, token string) (int64, error) {
	uq := user.New(h.db)

	users, err := uq.ListUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("list users: %w", err)
	}

	for _, u := range users {
		key := fmt.Sprintf("kobo_token_%d", u.ID)
		setting, err := uq.GetAppSetting(ctx, key)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return 0, fmt.Errorf("get app setting: %w", err)
		}
		if setting.Valid && setting.String == token {
			return u.ID, nil
		}
	}

	return 0, errTokenNotFound
}

// userIDFromContext extracts the Kobo user ID from the request context.
func userIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(koboUserIDKey{}).(int64)
	return id, ok
}

// --- Kobo API Endpoints ---

// handleInitialization handles GET /kobo/v1/initialization.
// Registers/updates the device and returns resource URLs pointing to Lexicon.
func (h *Handler) handleInitialization(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	deviceID := r.Header.Get("X-Kobo-DeviceId")
	deviceName := r.Header.Get("X-Kobo-DeviceModel")
	firmware := r.Header.Get("X-Kobo-Firmware")

	if deviceID != "" {
		q := New(h.db)
		if err := q.UpsertKoboDevice(r.Context(), UpsertKoboDeviceParams{
			UserID:     userID,
			DeviceID:   deviceID,
			DeviceName: sql.NullString{String: deviceName, Valid: deviceName != ""},
			Model:      sql.NullString{String: deviceName, Valid: deviceName != ""},
			Firmware:   sql.NullString{String: firmware, Valid: firmware != ""},
		}); err != nil {
			h.logger.Error("upsert kobo device", "device_id", deviceID, "error", err)
			// Non-fatal — continue with initialization.
		}
	}

	baseURL := h.baseURL(r) + "/kobo"

	resp := map[string]any{
		"Resources": map[string]string{
			"account_page":                  baseURL + "/v1/user/profile",
			"book_detail_page":              baseURL + "/v1/library/{ContentId}/metadata",
			"book_detail_page_rakuten":      baseURL + "/v1/library/{ContentId}/metadata",
			"book_purchase_page":            baseURL + "/v1/library/{ContentId}/metadata",
			"checkout_borrowed_book_page":   baseURL + "/v1/library/{ContentId}/metadata",
			"content_access_book":           baseURL + "/v1/library/{ContentId}/download",
			"customer_care_live_chat_page":  baseURL + "/v1/user/profile",
			"giftcard_purchase_page":        baseURL + "/v1/user/profile",
			"help_page":                     baseURL + "/v1/user/profile",
			"kobo_audiobooks_enabled":       "false",
			"kobo_nativeborrow_enabled":     "false",
			"kobo_onestorelibrary_enabled":  "false",
			"kobo_redeem_enabled":           "false",
			"kobo_shelfie_enabled":          "false",
			"kobo_subscriptions_enabled":    "false",
			"kobo_superpoints_enabled":      "false",
			"kobo_wishlist_enabled":         "false",
			"library_book":                  baseURL + "/v1/library/{ContentId}/metadata",
			"library_items":                 baseURL + "/v1/library/sync",
			"library_metadata":              baseURL + "/v1/library/{ContentId}/metadata",
			"library_prices":                baseURL + "/v1/products/prices",
			"library_stack":                 baseURL + "/v1/library/sync",
			"library_sync":                  baseURL + "/v1/library/sync",
			"love_dashboard_page":           baseURL + "/v1/user/profile",
			"love_points_page":              baseURL + "/v1/user/profile",
			"overdrive_account":             baseURL + "/v1/user/profile",
			"overdrive_library_finder_page": baseURL + "/v1/user/profile",
			"overdrive_sign_in_page":        baseURL + "/v1/user/profile",
			"overdrive_tags":                baseURL + "/v1/user/profile",
			"password_retrieval_page":       baseURL + "/v1/user/profile",
			"post_lending_experience_page":  baseURL + "/v1/user/profile",
			"privacy_page":                  baseURL + "/v1/user/profile",
			"product_list_page":             baseURL + "/v1/products/recommendations",
			"purchase_successful_page":      baseURL + "/v1/user/profile",
			"rakuten_token_exchange":        baseURL + "/v1/user/profile",
			"reading_state":                 baseURL + "/v1/library/sync/reading-state",
			"recommendations":               baseURL + "/v1/products/recommendations",
			"review_page":                   baseURL + "/v1/user/profile",
			"sign_in_page":                  baseURL + "/v1/user/profile",
			"social_authorization_page":     baseURL + "/v1/user/profile",
			"social_sign_in_page":           baseURL + "/v1/user/profile",
			"store_front":                   baseURL + "/v1/user/profile",
			"subscription_product_page":     baseURL + "/v1/user/profile",
			"support_page":                  baseURL + "/v1/user/profile",
			"sync_url":                      baseURL + "/v1/library/sync",
			"tags":                          baseURL + "/v1/user/wishlist",
			"taste_profile_page":            baseURL + "/v1/user/profile",
			"terms_of_sale_page":            baseURL + "/v1/user/profile",
			"terms_of_service_page":         baseURL + "/v1/user/profile",
			"user_loyalty_benefits":         baseURL + "/v1/user/profile",
			"user_platform_page":            baseURL + "/v1/user/profile",
			"user_profile":                  baseURL + "/v1/user/profile",
			"user_ratings":                  baseURL + "/v1/user/profile",
			"user_wishlist":                 baseURL + "/v1/user/wishlist",
			"wishlist":                      baseURL + "/v1/user/wishlist",
		},
		"Settings": map[string]any{
			"AccountPage":                baseURL + "/v1/user/profile",
			"SyncContinuationToken":      "",
			"SyncToken":                  "",
			"BookEntitlement":            map[string]any{},
			"BookMetadata":               map[string]any{},
			"ContentAccessBook":          map[string]any{},
			"ReadingState":               map[string]any{},
			"SyncTokenAppendix":          "",
			"SyncTokenVersion":           "1",
			"TagItem":                    map[string]any{},
			"UserProfile":                map[string]any{},
			"Wishlist":                   map[string]any{},
			"AnalyticsSettings":          map[string]any{"OptimizelyProjectId": ""},
			"DefaultStoreFront":          "US",
			"DisplayProfile":             "default",
			"ExtraEntitlementParameters": map[string]any{},
			"FeatureSettings":            map[string]any{},
			"OfferManagement":            map[string]any{},
			"PurchaseFlow":               map[string]any{},
			"RecommendationSettings":     map[string]any{},
			"SearchSettings":             map[string]any{},
			"SocialSettings":             map[string]any{},
			"StoreSettings":              map[string]any{},
			"SubscriptionSettings":       map[string]any{},
			"SupportSettings":            map[string]any{},
			"TasteProfileSettings":       map[string]any{},
			"UserAgreementSettings":      map[string]any{},
			"UserManagementSettings":     map[string]any{},
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

// bookEntitlement represents a book in the Kobo library sync response.
type bookEntitlement struct {
	BookEntitlement struct {
		Accessibility string `json:"Accessibility"`
		ActivePeriod  struct {
			From string `json:"From"`
		} `json:"ActivePeriod"`
		Created         string `json:"Created"`
		CrossRevisionID string `json:"CrossRevisionId"`
		ID              string `json:"Id"`
		IsRemoved       bool   `json:"IsRemoved"`
		IsHiddenFromUI  bool   `json:"IsHiddenFromUI"`
		RevisionID      string `json:"RevisionId"`
		Status          string `json:"Status"`
		Type            string `json:"Type"`
	} `json:"BookEntitlement"`
	BookMetadata struct {
		ContentSummary struct {
			ContentType  string `json:"ContentType"`
			CoverImageID string `json:"CoverImageId"`
			Title        string `json:"Title"`
			WorkID       string `json:"WorkId"`
		} `json:"ContentSummary"`
		DownloadUrls []downloadURL `json:"DownloadUrls"`
	} `json:"BookMetadata"`
	ReadingState readingStateEntry `json:"ReadingState"`
}

// downloadURL is a single download URL entry in the Kobo sync response.
type downloadURL struct {
	Format   string `json:"Format"`
	Platform string `json:"Platform"`
	URL      string `json:"Url"`
	DRMType  string `json:"DrmType"`
}

// readingStateEntry is the reading state portion of a Kobo sync entry.
type readingStateEntry struct {
	BookmarkDate      string          `json:"BookmarkDate"`
	CurrentBookmark   currentBookmark `json:"CurrentBookmark"`
	EntitlementID     string          `json:"EntitlementId"`
	LastModified      string          `json:"LastModified"`
	PriorityTimestamp string          `json:"PriorityTimestamp"`
	StatusInfo        statusInfo      `json:"StatusInfo"`
}

// currentBookmark holds the current reading position.
type currentBookmark struct {
	ContentSourceProgressPercent float64   `json:"ContentSourceProgressPercent"`
	Location                     *location `json:"Location,omitempty"`
	ProgressPercent              float64   `json:"ProgressPercent"`
}

// location is a CFI-based reading position.
type location struct {
	Source string `json:"Source"`
	Value  string `json:"Value"`
}

// statusInfo holds the reading status.
type statusInfo struct {
	LastModified        string `json:"LastModified"`
	Status              string `json:"Status"`
	TimesStartedReading int    `json:"TimesStartedReading"`
}

// handleLibrarySync handles GET /kobo/v1/library/sync.
// Returns the full list of EPUB books for the authenticated user.
func (h *Handler) handleLibrarySync(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	ctx := r.Context()
	bq := book.New(h.db)
	kq := New(h.db)
	uq := user.New(h.db)

	libraryIDs, err := uq.ListUserLibraryIDs(ctx, userID)
	if err != nil {
		h.logger.Error("list user library ids", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	readingStates, err := kq.ListKoboReadingStates(ctx, userID)
	if err != nil {
		h.logger.Error("list kobo reading states", "user_id", userID, "error", err)
		readingStates = nil
	}

	stateMap := make(map[string]KoboReadingState, len(readingStates))
	for _, rs := range readingStates {
		stateMap[rs.ContentID] = rs
	}

	koboBase := h.baseURL(r) + "/kobo"
	apiBase := h.baseURL(r)
	now := time.Now().UTC().Format(time.RFC3339)

	var entries []bookEntitlement

	// Check admin status for content restrictions.
	var isAdmin bool
	if perms, permErr := uq.GetUserPermissions(ctx, userID); permErr == nil {
		isAdmin = perms.Role == "ADMIN"
	}

	for _, libID := range libraryIDs {
		books, err := bq.ListBooksByLibrary(ctx, libID)
		if err != nil {
			h.logger.Error("list books by library", "library_id", libID, "error", err)
			continue
		}

		// Apply content restrictions.
		if h.contentRestrictionSvc != nil && !isAdmin && len(books) > 0 {
			bookIDs := make([]int64, len(books))
			for i, b := range books {
				bookIDs[i] = b.ID
			}
			filteredIDs, filterErr := h.contentRestrictionSvc.FilterBookIDs(ctx, userID, isAdmin, bookIDs)
			if filterErr != nil {
				h.logger.Error("filter kobo sync books", "error", filterErr)
				// Non-fatal: continue without filtering.
			} else {
				idSet := make(map[int64]struct{}, len(filteredIDs))
				for _, id := range filteredIDs {
					idSet[id] = struct{}{}
				}
				var filtered []book.Book
				for _, b := range books {
					if _, ok := idSet[b.ID]; ok {
						filtered = append(filtered, b)
					}
				}
				books = filtered
			}
		}

		for _, b := range books {
			files, err := bq.ListBookFiles(ctx, b.ID)
			if err != nil {
				h.logger.Error("list book files", "book_id", b.ID, "error", err)
				continue
			}

			for _, f := range files {
				if f.Format != "EPUB" {
					continue
				}

				meta, metaErr := bq.GetBookMetadata(ctx, b.ID)

				contentID := strconv.FormatInt(f.ID, 10)

				title := contentID
				if metaErr == nil && meta.Title.Valid {
					title = meta.Title.String
				}

				coverURL := ""
				if metaErr == nil && meta.CoverPath.Valid && meta.CoverPath.String != "" {
					coverURL = fmt.Sprintf("%s/api/books/%d/cover", apiBase, b.ID)
				}

				downloadURLStr := fmt.Sprintf("%s/v1/library/%s/download", koboBase, contentID)

				entry := bookEntitlement{}
				entry.BookEntitlement.Accessibility = "Full"
				entry.BookEntitlement.ActivePeriod.From = b.CreatedAt
				entry.BookEntitlement.Created = b.CreatedAt
				entry.BookEntitlement.CrossRevisionID = contentID
				entry.BookEntitlement.ID = contentID
				entry.BookEntitlement.IsRemoved = false
				entry.BookEntitlement.IsHiddenFromUI = false
				entry.BookEntitlement.RevisionID = contentID
				entry.BookEntitlement.Status = "Active"
				entry.BookEntitlement.Type = "ebook"

				entry.BookMetadata.ContentSummary.ContentType = "ebook"
				entry.BookMetadata.ContentSummary.CoverImageID = coverURL
				entry.BookMetadata.ContentSummary.Title = title
				entry.BookMetadata.ContentSummary.WorkID = contentID
				entry.BookMetadata.DownloadUrls = []downloadURL{
					{
						Format:   "KEPUB",
						Platform: "Generic",
						URL:      downloadURLStr,
						DRMType:  "None",
					},
				}

				rs := readingStateEntry{
					EntitlementID:     contentID,
					LastModified:      now,
					PriorityTimestamp: now,
					StatusInfo: statusInfo{
						LastModified: now,
						Status:       "ReadyToRead",
					},
				}

				if state, ok := stateMap[contentID]; ok {
					if state.LastModified.Valid {
						rs.LastModified = state.LastModified.String
						rs.PriorityTimestamp = state.LastModified.String
						rs.StatusInfo.LastModified = state.LastModified.String
					}
					if state.Status.Valid {
						rs.StatusInfo.Status = state.Status.String
					}
					if state.PercentRead.Valid {
						rs.CurrentBookmark.ProgressPercent = state.PercentRead.Float64
						rs.CurrentBookmark.ContentSourceProgressPercent = state.PercentRead.Float64
					}
					if state.CurrentCfi.Valid && state.CurrentCfi.String != "" {
						rs.CurrentBookmark.Location = &location{
							Source: "KoboSpan",
							Value:  state.CurrentCfi.String,
						}
					}
				}

				entry.ReadingState = rs
				entries = append(entries, entry)
			}
		}
	}

	if entries == nil {
		entries = []bookEntitlement{}
	}

	w.Header().Set("x-kobo-sync", "continue")
	w.Header().Set("x-kobo-synctoken", "")
	writeJSON(w, http.StatusOK, entries)
}

// handleBookMetadata handles GET /kobo/v1/library/{contentId}/metadata.
func (h *Handler) handleBookMetadata(w http.ResponseWriter, r *http.Request) {
	_, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	contentID := chi.URLParam(r, "contentId")
	fileID, err := strconv.ParseInt(contentID, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid content id")
		return
	}

	ctx := r.Context()
	bq := book.New(h.db)

	bookFile, err := bq.GetBookFileByID(ctx, fileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "book not found")
			return
		}
		h.logger.Error("get book file by id", "file_id", fileID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	meta, metaErr := bq.GetBookMetadata(ctx, bookFile.BookID)

	title := contentID
	if metaErr == nil && meta.Title.Valid {
		title = meta.Title.String
	}

	base := h.baseURL(r)

	resp := map[string]any{
		"ContentMetadata": map[string]any{
			"BookMetadata": map[string]any{
				"Contributors":    []map[string]any{},
				"CoverImageId":    fmt.Sprintf("%s/api/books/%d/cover", base, bookFile.BookID),
				"CrossRevisionId": contentID,
				"CurrentDisplayPrice": map[string]any{
					"CurrencyCode": "USD",
					"TotalAmount":  0,
				},
				"CurrentLoveDisplayPrice": map[string]any{"TotalAmount": 0},
				"Description":             "",
				"DownloadUrls": []map[string]any{
					{
						"Format":   "KEPUB",
						"Platform": "Generic",
						"Url":      fmt.Sprintf("%s/kobo/v1/library/%s/download", base, contentID),
						"DrmType":  "None",
					},
				},
				"EntitlementId":          contentID,
				"ExternalIds":            []any{},
				"Genre":                  "00000000-0000-0000-0000-000000000001",
				"IsEligibleForKoboLove":  false,
				"IsInternetArchive":      false,
				"IsPreOrder":             false,
				"IsSocialEnabled":        false,
				"Language":               "en",
				"PhoneticPronunciations": map[string]any{},
				"PublicationDate":        "",
				"Publisher":              map[string]any{"Imprint": "", "Name": ""},
				"RevisionId":             contentID,
				"Series":                 nil,
				"Title":                  title,
				"WorkId":                 contentID,
			},
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleDownload handles GET /kobo/v1/library/{contentId}/download.
// Converts EPUB to KEPUB if needed and serves the file.
func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	contentID := chi.URLParam(r, "contentId")
	fileID, err := strconv.ParseInt(contentID, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid content id")
		return
	}

	ctx := r.Context()
	bq := book.New(h.db)
	uq := user.New(h.db)

	bookFile, err := bq.GetBookFileByID(ctx, fileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "book not found")
			return
		}
		h.logger.Error("get book file by id", "file_id", fileID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Verify the user has access to the library containing this book.
	b, err := bq.GetBookByID(ctx, bookFile.BookID)
	if err != nil {
		h.logger.Error("get book by id", "book_id", bookFile.BookID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	libraryIDs, err := uq.ListUserLibraryIDs(ctx, userID)
	if err != nil {
		h.logger.Error("list user library ids", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	hasAccess := false
	for _, id := range libraryIDs {
		if id == b.LibraryID {
			hasAccess = true
			break
		}
	}
	if !hasAccess {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	filePath := bookFile.FilePath

	// For EPUB files, convert to KEPUB and serve the converted file.
	if strings.EqualFold(bookFile.Format, "EPUB") {
		cacheDir := filepath.Join(h.dataDir, "kobo-cache")
		kepubPath, err := ConvertToKEPUB(filePath, cacheDir, fileID)
		if err != nil {
			h.logger.Error("convert to kepub", "file_id", fileID, "error", err)
			// Fall back to serving the original EPUB.
		} else {
			filePath = kepubPath
		}
	}

	f, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "file not found on disk")
			return
		}
		h.logger.Error("open book file", "path", filePath, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		h.logger.Error("stat book file", "path", filePath, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Content-Type", "application/epub+zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.kepub.epub"`, contentID))
	w.Header().Set("Accept-Ranges", "bytes")

	http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
}

// readingStateSyncRequest is the JSON body for POST /kobo/v1/library/sync/reading-state.
type readingStateSyncRequest struct {
	ReadingStates []struct {
		EntitlementID   string `json:"EntitlementId"`
		CurrentBookmark struct {
			ContentSourceProgressPercent float64   `json:"ContentSourceProgressPercent"`
			Location                     *location `json:"Location"`
			ProgressPercent              float64   `json:"ProgressPercent"`
		} `json:"CurrentBookmark"`
		LastModified string `json:"LastModified"`
		StatusInfo   struct {
			LastModified string `json:"LastModified"`
			Status       string `json:"Status"`
		} `json:"StatusInfo"`
	} `json:"ReadingStates"`
}

// handleSyncReadingState handles POST /kobo/v1/library/sync/reading-state.
// Receives reading states from the device and syncs them to Lexicon.
func (h *Handler) handleSyncReadingState(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req readingStateSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ctx := r.Context()
	kq := New(h.db)
	bq := book.New(h.db)

	for _, rs := range req.ReadingStates {
		fileID, err := strconv.ParseInt(rs.EntitlementID, 10, 64)
		if err != nil {
			h.logger.Warn("invalid entitlement id in reading state", "id", rs.EntitlementID)
			continue
		}

		bookFile, err := bq.GetBookFileByID(ctx, fileID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				h.logger.Warn("book file not found for reading state", "file_id", fileID)
				continue
			}
			h.logger.Error("get book file for reading state", "file_id", fileID, "error", err)
			continue
		}

		cfi := ""
		if rs.CurrentBookmark.Location != nil {
			cfi = rs.CurrentBookmark.Location.Value
		}

		lastModified := rs.LastModified
		if lastModified == "" {
			lastModified = time.Now().UTC().Format(time.RFC3339)
		}

		if err := kq.UpsertKoboReadingState(ctx, UpsertKoboReadingStateParams{
			UserID:       userID,
			BookFileID:   bookFile.ID,
			ContentID:    rs.EntitlementID,
			Status:       sql.NullString{String: rs.StatusInfo.Status, Valid: rs.StatusInfo.Status != ""},
			PercentRead:  sql.NullFloat64{Float64: rs.CurrentBookmark.ProgressPercent, Valid: true},
			CurrentCfi:   sql.NullString{String: cfi, Valid: cfi != ""},
			LastModified: sql.NullString{String: lastModified, Valid: true},
		}); err != nil {
			h.logger.Error("upsert kobo reading state", "file_id", fileID, "error", err)
			continue
		}

		// Sync progress back to Lexicon's user_book_file_progress table.
		if rs.CurrentBookmark.ProgressPercent > 0 || cfi != "" {
			progressValue := fmt.Sprintf("%.4f", rs.CurrentBookmark.ProgressPercent)
			if cfi != "" {
				progressValue = cfi
			}
			if err := bq.UpsertProgress(ctx, book.UpsertProgressParams{
				UserID:       userID,
				BookFileID:   bookFile.ID,
				Progress:     sql.NullString{String: progressValue, Valid: true},
				ProgressType: sql.NullString{String: "kobo", Valid: true},
			}); err != nil {
				h.logger.Error("upsert progress from kobo", "file_id", fileID, "error", err)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"ReadingStates": []any{}})
}

// handleUserProfile handles GET /kobo/v1/user/profile.
func (h *Handler) handleUserProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	uq := user.New(h.db)
	u, err := uq.GetUserByID(r.Context(), userID)
	if err != nil {
		h.logger.Error("get user by id", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	name := u.Username
	if u.Name.Valid {
		name = u.Name.String
	}

	resp := map[string]any{
		"UserProfile": map[string]any{
			"Account":                 u.Username,
			"DisplayName":             name,
			"Email":                   "",
			"HasMosaic":               false,
			"IsChild":                 false,
			"IsWeeklyDealsSubscriber": false,
			"PrivacyPermissions":      map[string]any{},
			"Publisher":               map[string]any{},
			"UserId":                  strconv.FormatInt(userID, 10),
			"UserKey":                 strconv.FormatInt(userID, 10),
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleWishlist handles GET /kobo/v1/user/wishlist — stub, returns empty list.
func (h *Handler) handleWishlist(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"Wishlist": []any{}})
}

// handlePrices handles GET /kobo/v1/products/prices — stub, returns empty list.
func (h *Handler) handlePrices(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"Prices": []any{}})
}

// handleRecommendations handles GET /kobo/v1/products/recommendations — stub.
func (h *Handler) handleRecommendations(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"Recommendations": []any{}})
}

// handleGenerateToken handles POST /api/kobo/token.
// Generates a random Kobo token for the authenticated user and stores it in
// app_settings as "kobo_token_{userID}". Returns the token and setup URL.
func (h *Handler) handleGenerateToken(w http.ResponseWriter, r *http.Request) {
	if h.principalExtractor == nil {
		writeError(w, http.StatusInternalServerError, "principal extractor not configured")
		return
	}

	userID, ok := h.principalExtractor(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Generate a cryptographically random 32-byte token.
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		h.logger.Error("generate kobo token", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	token := hex.EncodeToString(b)

	uq := user.New(h.db)
	key := fmt.Sprintf("kobo_token_%d", userID)
	if err := uq.UpsertAppSetting(r.Context(), user.UpsertAppSettingParams{
		Key:   key,
		Value: sql.NullString{String: token, Valid: true},
	}); err != nil {
		h.logger.Error("upsert kobo token", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	setupURL := h.baseURL(r) + "/kobo"

	h.logger.Info("kobo token generated", "user_id", userID)

	writeJSON(w, http.StatusOK, map[string]any{
		"token":    token,
		"setupUrl": setupURL,
		"instructions": fmt.Sprintf(
			"On your Kobo device, go to Settings > Accounts > Kobo Store and set the store URL to: %s",
			setupURL,
		),
	})
}

// baseURL returns the scheme+host base URL for the current request.
func (h *Handler) baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
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
