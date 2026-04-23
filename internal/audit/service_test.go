package audit

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	schema := `
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    password_hash TEXT,
    email TEXT,
    name TEXT,
    enabled INTEGER DEFAULT 1,
    created_at TEXT DEFAULT (datetime('now'))
);
CREATE TABLE audit_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id         INTEGER REFERENCES users(id) ON DELETE SET NULL,
    username        TEXT,
    action          TEXT NOT NULL,
    resource_type   TEXT,
    resource_id     INTEGER,
    details         TEXT,
    ip_address      TEXT,
    country         TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);
`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestServiceLog(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	svc := NewService(db, testLogger())
	ctx := context.Background()

	userID := int64(1)
	err := svc.Log(ctx, LogParams{
		UserID:       &userID,
		Username:     "alice",
		Action:       ActionUserLogin,
		ResourceType: "user",
		ResourceID:   &userID,
		Details:      map[string]any{"ip": "127.0.0.1"},
		IPAddress:    "127.0.0.1",
		Country:      "US",
	})
	if err != nil {
		t.Fatalf("Log returned error: %v", err)
	}

	// Wait for async log to complete.
	time.Sleep(100 * time.Millisecond)

	q := New(db)
	logs, err := q.ListAuditLogsFiltered(ctx, ListAuditLogsFilteredParams{
		Action:      "",
		Column1:     "",
		Column3:     -1,
		UserID:      sql.NullInt64{Valid: false},
		Column5:     "",
		CreatedAt:   "",
		Column7:     "",
		CreatedAt_2: "",
		Limit:       10,
		Offset:      0,
	})
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("got %d logs; want 1", len(logs))
	}
	log := logs[0]
	if log.Action != ActionUserLogin {
		t.Errorf("action = %q; want %q", log.Action, ActionUserLogin)
	}
	if log.Username.String != "alice" {
		t.Errorf("username = %q; want %q", log.Username.String, "alice")
	}
	if log.IpAddress.String != "127.0.0.1" {
		t.Errorf("ip = %q; want %q", log.IpAddress.String, "127.0.0.1")
	}
	if !log.UserID.Valid || log.UserID.Int64 != 1 {
		t.Errorf("user_id = %v; want 1", log.UserID)
	}
}

func TestServiceLogNeverFails(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	svc := NewService(db, testLogger())
	ctx := context.Background()

	// Log with no params should not error.
	err := svc.Log(ctx, LogParams{
		Action: ActionUserLogin,
	})
	if err != nil {
		t.Fatalf("Log should never return error: %v", err)
	}
}

func TestServiceCleanup(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	svc := NewService(db, testLogger())
	ctx := context.Background()
	q := New(db)

	// Insert old log.
	_, err := db.Exec(`INSERT INTO audit_log (action, created_at) VALUES (?, datetime('now', '-2 days'))`, ActionUserLogin)
	if err != nil {
		t.Fatalf("insert old log: %v", err)
	}

	// Insert recent log.
	_, err = db.Exec(`INSERT INTO audit_log (action, created_at) VALUES (?, datetime('now'))`, ActionUserLogout)
	if err != nil {
		t.Fatalf("insert recent log: %v", err)
	}

	if err := svc.Cleanup(ctx, 1); err != nil {
		t.Fatalf("Cleanup error: %v", err)
	}

	count, err := q.CountAuditLogsFiltered(ctx, CountAuditLogsFilteredParams{
		Action:      "",
		Column1:     "",
		Column3:     -1,
		UserID:      sql.NullInt64{Valid: false},
		Column5:     "",
		CreatedAt:   "",
		Column7:     "",
		CreatedAt_2: "",
	})
	if err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d; want 1", count)
	}
}

func TestListAuditLogsFiltered(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	q := New(db)

	_, err := db.Exec(`INSERT INTO audit_log (action, username, created_at) VALUES (?, ?, datetime('now'))`, ActionUserLogin, "alice")
	if err != nil {
		t.Fatalf("insert log: %v", err)
	}
	_, err = db.Exec(`INSERT INTO audit_log (action, username, created_at) VALUES (?, ?, datetime('now'))`, ActionUserLogout, "bob")
	if err != nil {
		t.Fatalf("insert log: %v", err)
	}

	// Filter by action.
	logs, err := q.ListAuditLogsFiltered(ctx, ListAuditLogsFilteredParams{
		Action:      ActionUserLogin,
		Column1:     ActionUserLogin,
		Column3:     -1,
		UserID:      sql.NullInt64{Valid: false},
		Column5:     "",
		CreatedAt:   "",
		Column7:     "",
		CreatedAt_2: "",
		Limit:       10,
		Offset:      0,
	})
	if err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("got %d logs; want 1", len(logs))
	}
	if logs[0].Username.String != "alice" {
		t.Errorf("username = %q; want alice", logs[0].Username.String)
	}
}

func TestHandlerListAuditLogs(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	logger := testLogger()
	svc := NewService(db, logger)
	hdlr := NewHandler(db, svc, logger)

	ctx := context.Background()
	q := New(db)
	_, err := q.CreateAuditLog(ctx, CreateAuditLogParams{
		Action:   ActionUserLogin,
		Username: sql.NullString{String: "alice", Valid: true},
	})
	if err != nil {
		t.Fatalf("create audit log: %v", err)
	}

	// We can't easily test the HTTP handler without a full request setup,
	// but we can verify the handler struct is correctly wired.
	if hdlr.db == nil {
		t.Fatal("handler db is nil")
	}
	if hdlr.service == nil {
		t.Fatal("handler service is nil")
	}
	if hdlr.logger == nil {
		t.Fatal("handler logger is nil")
	}
}

func TestLogWithDetailsMarshalError(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	svc := NewService(db, testLogger())
	ctx := context.Background()

	// Pass a value that can't be marshaled to JSON (channel).
	userID := int64(1)
	err := svc.Log(ctx, LogParams{
		UserID:  &userID,
		Action:  ActionUserLogin,
		Details: map[string]any{"ch": make(chan int)},
	})
	if err != nil {
		t.Fatalf("Log should never return error even on marshal failure: %v", err)
	}
}

func TestLogDetailsOmittedWhenEmpty(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	svc := NewService(db, testLogger())
	ctx := context.Background()

	userID := int64(1)
	err := svc.Log(ctx, LogParams{
		UserID:  &userID,
		Action:  ActionUserLogin,
		Details: map[string]any{},
	})
	if err != nil {
		t.Fatalf("Log returned error: %v", err)
	}

	// Wait for async log to complete.
	time.Sleep(100 * time.Millisecond)

	q := New(db)
	logs, err := q.ListAuditLogsFiltered(ctx, ListAuditLogsFilteredParams{
		Action:      "",
		Column1:     "",
		Column3:     -1,
		UserID:      sql.NullInt64{Valid: false},
		Column5:     "",
		CreatedAt:   "",
		Column7:     "",
		CreatedAt_2: "",
		Limit:       10,
		Offset:      0,
	})
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("got %d logs; want 1", len(logs))
	}
	if logs[0].Details.Valid {
		t.Error("details should be NULL when empty map is passed")
	}
}

func TestListAuditLogsPagination(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	q := New(db)

	for i := 0; i < 5; i++ {
		_, err := db.Exec(`INSERT INTO audit_log (action, created_at) VALUES (?, datetime('now', ?))`, ActionUserLogin, "-"+string(rune('0'+i))+" days")
		if err != nil {
			t.Fatalf("insert log %d: %v", i, err)
		}
	}

	// Get page 1 with size 2.
	logs, err := q.ListAuditLogsFiltered(ctx, ListAuditLogsFilteredParams{
		Action:      "",
		Column1:     "",
		Column3:     -1,
		UserID:      sql.NullInt64{Valid: false},
		Column5:     "",
		CreatedAt:   "",
		Column7:     "",
		CreatedAt_2: "",
		Limit:       2,
		Offset:      0,
	})
	if err != nil {
		t.Fatalf("list page 1: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("page 1 size = %d; want 2", len(logs))
	}

	// Get page 2 with size 2.
	logs, err = q.ListAuditLogsFiltered(ctx, ListAuditLogsFilteredParams{
		Action:      "",
		Column1:     "",
		Column3:     -1,
		UserID:      sql.NullInt64{Valid: false},
		Column5:     "",
		CreatedAt:   "",
		Column7:     "",
		CreatedAt_2: "",
		Limit:       2,
		Offset:      2,
	})
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("page 2 size = %d; want 2", len(logs))
	}
}

func TestCountAuditLogsFilteredByDateRange(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Insert logs on different dates.
	_, err := db.Exec(`INSERT INTO audit_log (action, created_at) VALUES (?, datetime('now', '-5 days'))`, ActionUserLogin)
	if err != nil {
		t.Fatalf("insert log: %v", err)
	}
	_, err = db.Exec(`INSERT INTO audit_log (action, created_at) VALUES (?, datetime('now'))`, ActionUserLogout)
	if err != nil {
		t.Fatalf("insert log: %v", err)
	}

	q := New(db)
	from := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	to := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")

	count, err := q.CountAuditLogsFiltered(ctx, CountAuditLogsFilteredParams{
		Action:      "",
		Column1:     "",
		Column3:     -1,
		UserID:      sql.NullInt64{Valid: false},
		Column5:     from,
		CreatedAt:   from,
		Column7:     to,
		CreatedAt_2: to,
	})
	if err != nil {
		t.Fatalf("count filtered: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d; want 1", count)
	}
}
