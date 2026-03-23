package user

import (
	"context"
	"database/sql"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword hashes a plaintext password using bcrypt with the default cost.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword compares a bcrypt hashed password with a plaintext candidate.
func VerifyPassword(hashedPassword, plainPassword string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(plainPassword))
}

// CreateUserServiceParams holds the parameters for creating a new user.
type CreateUserServiceParams struct {
	Username string
	Password string
	Name     string
	Email    string
}

// CreateUserWithDefaults creates a new user with a hashed password and default
// permissions and settings. It returns the created user. All database operations
// are performed within a single transaction.
func CreateUserWithDefaults(ctx context.Context, db *sql.DB, params CreateUserServiceParams) (User, error) {
	hash, err := HashPassword(params.Password)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	q := New(tx)

	u, err := q.CreateUser(ctx, CreateUserParams{
		Username:     params.Username,
		PasswordHash: sql.NullString{String: hash, Valid: true},
		Name:         sql.NullString{String: params.Name, Valid: params.Name != ""},
		Email:        sql.NullString{String: params.Email, Valid: params.Email != ""},
	})
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}

	// Create default permissions (USER role, no special permissions).
	if err := q.UpsertUserPermissions(ctx, UpsertUserPermissionsParams{
		UserID: u.ID,
		Role:   "USER",
	}); err != nil {
		return User{}, fmt.Errorf("create user permissions: %w", err)
	}

	// Create default settings.
	if err := q.UpsertUserSettings(ctx, UpsertUserSettingsParams{
		UserID: u.ID,
	}); err != nil {
		return User{}, fmt.Errorf("create user settings: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return User{}, fmt.Errorf("commit transaction: %w", err)
	}

	return u, nil
}

// CreateAdminUser creates a user with the ADMIN role and all permissions enabled.
// All database operations are performed within a single transaction.
func CreateAdminUser(ctx context.Context, db *sql.DB, params CreateUserServiceParams) (User, error) {
	hash, err := HashPassword(params.Password)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	q := New(tx)

	u, err := q.CreateUser(ctx, CreateUserParams{
		Username:     params.Username,
		PasswordHash: sql.NullString{String: hash, Valid: true},
		Name:         sql.NullString{String: params.Name, Valid: params.Name != ""},
		Email:        sql.NullString{String: params.Email, Valid: params.Email != ""},
	})
	if err != nil {
		return User{}, fmt.Errorf("create admin user: %w", err)
	}

	// Create admin permissions with all flags enabled.
	if err := q.UpsertUserPermissions(ctx, UpsertUserPermissionsParams{
		UserID:          u.ID,
		Role:            "ADMIN",
		CanDownload:     1,
		CanUpload:       1,
		CanEmailSend:    1,
		CanEditMetadata: 1,
		OpdsAccess:      1,
	}); err != nil {
		return User{}, fmt.Errorf("create admin permissions: %w", err)
	}

	// Create default settings.
	if err := q.UpsertUserSettings(ctx, UpsertUserSettingsParams{
		UserID: u.ID,
	}); err != nil {
		return User{}, fmt.Errorf("create admin settings: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return User{}, fmt.Errorf("commit transaction: %w", err)
	}

	return u, nil
}
