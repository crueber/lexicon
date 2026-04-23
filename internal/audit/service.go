package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
)

// Action type constants.
const (
	ActionUserLogin           = "USER_LOGIN"
	ActionUserLogout          = "USER_LOGOUT"
	ActionUserCreated         = "USER_CREATED"
	ActionUserUpdated         = "USER_UPDATED"
	ActionUserDeleted         = "USER_DELETED"
	ActionBookDownloaded      = "BOOK_DOWNLOADED"
	ActionBookSent            = "BOOK_SENT"
	ActionBookMetadataUpdated = "BOOK_METADATA_UPDATED"
	ActionBookCoverUpdated    = "BOOK_COVER_UPDATED"
	ActionBookDeleted         = "BOOK_DELETED"
	ActionLibraryCreated      = "LIBRARY_CREATED"
	ActionLibraryUpdated      = "LIBRARY_UPDATED"
	ActionLibraryDeleted      = "LIBRARY_DELETED"
	ActionLibraryScanned      = "LIBRARY_SCANNED"
	ActionShelfCreated        = "SHELF_CREATED"
	ActionShelfDeleted        = "SHELF_DELETED"
	ActionBookdropImported    = "BOOKDROP_IMPORTED"
	ActionOPDSAccess          = "OPDS_ACCESS"
	ActionKoboSync            = "KOBO_SYNC"
	ActionKOReaderSync        = "KOREADER_SYNC"
	ActionAdminAction         = "ADMIN_ACTION"
	ActionOIDCUserCreated     = "OIDC_USER_CREATED"
	ActionRemoteAuthLogin     = "REMOTE_AUTH_LOGIN"
)

// LogParams contains all fields for an audit log entry.
type LogParams struct {
	UserID       *int64
	Username     string
	Action       string
	ResourceType string
	ResourceID   *int64
	Details      map[string]any
	IPAddress    string
	Country      string
}

// Service provides audit logging.
type Service struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewService creates a new audit Service.
func NewService(db *sql.DB, logger *slog.Logger) *Service {
	return &Service{db: db, logger: logger}
}

// Log creates an audit log entry asynchronously.
func (s *Service) Log(ctx context.Context, params LogParams) error {
	go s.logAsync(context.Background(), params)
	return nil // always return nil so callers don't have to handle it
}

func (s *Service) logAsync(ctx context.Context, params LogParams) {
	q := New(s.db)

	var detailsJSON sql.NullString
	if len(params.Details) > 0 {
		b, err := json.Marshal(params.Details)
		if err != nil {
			s.logger.Warn("failed to marshal audit details", "action", params.Action, "error", err)
		} else {
			detailsJSON = sql.NullString{String: string(b), Valid: true}
		}
	}

	var userID sql.NullInt64
	if params.UserID != nil {
		userID = sql.NullInt64{Int64: *params.UserID, Valid: true}
	}

	var resourceID sql.NullInt64
	if params.ResourceID != nil {
		resourceID = sql.NullInt64{Int64: *params.ResourceID, Valid: true}
	}

	_, err := q.CreateAuditLog(ctx, CreateAuditLogParams{
		UserID:       userID,
		Username:     sql.NullString{String: params.Username, Valid: params.Username != ""},
		Action:       params.Action,
		ResourceType: sql.NullString{String: params.ResourceType, Valid: params.ResourceType != ""},
		ResourceID:   resourceID,
		Details:      detailsJSON,
		IpAddress:    sql.NullString{String: params.IPAddress, Valid: params.IPAddress != ""},
		Country:      sql.NullString{String: params.Country, Valid: params.Country != ""},
	})
	if err != nil {
		// Audit logging should never break the main flow.
		s.logger.Warn("failed to create audit log", "action", params.Action, "error", err)
	}
}

// Cleanup removes audit log entries older than the given number of days.
func (s *Service) Cleanup(ctx context.Context, retentionDays int) error {
	q := New(s.db)
	return q.DeleteOldAuditLogs(ctx, fmt.Sprintf("-%d days", retentionDays))
}
