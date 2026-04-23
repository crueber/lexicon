package hardcover

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
)

// Service manages Hardcover sync settings.
type Service struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewService creates a new Hardcover Service.
func NewService(db *sql.DB, logger *slog.Logger) *Service {
	return &Service{db: db, logger: logger}
}

// Settings holds a user's Hardcover sync configuration.
type Settings struct {
	ID      int64  `json:"id"`
	UserID  int64  `json:"userId"`
	APIKey  string `json:"apiKey"`
	Enabled bool   `json:"enabled"`
}

// GetSettings returns the Hardcover sync settings for a user.
func (s *Service) GetSettings(ctx context.Context, userID int64) (*Settings, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT id, user_id, api_key, enabled FROM hardcover_sync WHERE user_id = ?", userID)

	var st Settings
	var enabled int
	err := row.Scan(&st.ID, &st.UserID, &st.APIKey, &enabled)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get hardcover settings: %w", err)
	}
	st.Enabled = enabled == 1
	return &st, nil
}

// SaveSettings saves or updates Hardcover sync settings for a user.
func (s *Service) SaveSettings(ctx context.Context, userID int64, apiKey string, enabled bool) error {
	en := 0
	if enabled {
		en = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hardcover_sync (user_id, api_key, enabled) VALUES (?, ?, ?)
         ON CONFLICT(user_id) DO UPDATE SET api_key = excluded.api_key, enabled = excluded.enabled`,
		userID, apiKey, en)
	if err != nil {
		return fmt.Errorf("save hardcover settings: %w", err)
	}
	return nil
}
