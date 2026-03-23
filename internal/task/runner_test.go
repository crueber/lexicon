package task

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/crueber/lexicon/internal/ws"
)

// openTestDB opens an in-memory SQLite database and applies the tasks schema.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	// Use a named in-memory database with shared cache so all connections in
	// the pool see the same data. Each test gets a unique name to avoid
	// cross-test interference.
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	// Keep a single connection so the in-memory DB is not dropped between uses.
	db.SetMaxOpenConns(1)

	// Apply pragmas.
	_, err = db.Exec(`
		PRAGMA busy_timeout=5000;
		PRAGMA foreign_keys=ON;
	`)
	if err != nil {
		t.Fatalf("set pragmas: %v", err)
	}

	// Create the tasks table.
	_, err = db.Exec(`
		CREATE TABLE tasks (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			task_type       TEXT NOT NULL,
			status          TEXT NOT NULL DEFAULT 'QUEUED',
			progress        INTEGER DEFAULT 0,
			total           INTEGER DEFAULT 0,
			message         TEXT,
			error           TEXT,
			payload         TEXT,
			created_at      TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
			started_at      TEXT,
			completed_at    TEXT
		);
		CREATE TABLE task_cron_configuration (
			task_type   TEXT PRIMARY KEY,
			cron_expr   TEXT NOT NULL,
			enabled     INTEGER NOT NULL DEFAULT 1
		);
	`)
	if err != nil {
		t.Fatalf("create tasks table: %v", err)
	}

	t.Cleanup(func() { db.Close() })
	return db
}

// newTestRunner creates a Runner with a real in-memory DB and a no-op hub.
func newTestRunner(t *testing.T) (*Runner, *sql.DB) {
	t.Helper()
	db := openTestDB(t)
	hub := ws.NewHub(slog.Default())
	runner := NewRunner(db, hub, slog.Default())
	return runner, db
}

// waitForStatus polls the task record until it reaches one of the expected
// statuses or the timeout expires.
func waitForStatus(t *testing.T, db *sql.DB, taskID int64, timeout time.Duration, wantStatuses ...string) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		q := New(db)
		task, err := q.GetTask(context.Background(), taskID)
		if err != nil {
			t.Fatalf("get task %d: %v", taskID, err)
		}
		for _, s := range wantStatuses {
			if task.Status == s {
				return task.Status
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	q := New(db)
	task, _ := q.GetTask(context.Background(), taskID)
	t.Fatalf("task %d did not reach status %v within %v; got %q", taskID, wantStatuses, timeout, task.Status)
	return ""
}

func TestRunner_TaskRunsAndCompletes(t *testing.T) {
	runner, db := newTestRunner(t)

	var called bool
	runner.Register("TEST_TASK", func(ctx context.Context, payload string, reporter Reporter) error {
		called = true
		reporter.Progress(1, 1, "done")
		return nil
	})

	taskID, err := runner.Enqueue(context.Background(), "TEST_TASK", "")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if taskID <= 0 {
		t.Fatalf("expected positive task ID; got %d", taskID)
	}

	status := waitForStatus(t, db, taskID, 5*time.Second, StatusCompleted)
	if status != StatusCompleted {
		t.Errorf("got status %q; want %q", status, StatusCompleted)
	}

	if !called {
		t.Error("task function was not called")
	}

	// Verify progress was recorded.
	q := New(db)
	task, err := q.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if !task.Progress.Valid || task.Progress.Int64 != 1 {
		t.Errorf("got progress %v; want 1", task.Progress)
	}
}

func TestRunner_DuplicateTaskTypeRejected(t *testing.T) {
	runner, db := newTestRunner(t)

	started := make(chan struct{})
	unblock := make(chan struct{})

	runner.Register("SLOW_TASK", func(ctx context.Context, payload string, reporter Reporter) error {
		close(started)
		<-unblock
		return nil
	})

	// Enqueue the first task.
	taskID, err := runner.Enqueue(context.Background(), "SLOW_TASK", "")
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}

	// Wait until the task has actually started running.
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("task did not start within timeout")
	}

	// Attempt to enqueue a second task of the same type — should fail.
	_, err = runner.Enqueue(context.Background(), "SLOW_TASK", "")
	if err == nil {
		t.Error("expected error for duplicate task type; got nil")
	}

	// Unblock the first task and wait for it to complete.
	close(unblock)
	waitForStatus(t, db, taskID, 5*time.Second, StatusCompleted)
}

func TestRunner_DuplicateTaskTypeRejected_AfterCompletion(t *testing.T) {
	runner, db := newTestRunner(t)

	var mu sync.Mutex
	callCount := 0

	runner.Register("SEQUENTIAL_TASK", func(ctx context.Context, payload string, reporter Reporter) error {
		mu.Lock()
		callCount++
		mu.Unlock()
		return nil
	})

	// First enqueue.
	taskID1, err := runner.Enqueue(context.Background(), "SEQUENTIAL_TASK", "")
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	waitForStatus(t, db, taskID1, 5*time.Second, StatusCompleted)

	// Second enqueue after first completes — should succeed.
	taskID2, err := runner.Enqueue(context.Background(), "SEQUENTIAL_TASK", "")
	if err != nil {
		t.Fatalf("second enqueue after completion: %v", err)
	}
	waitForStatus(t, db, taskID2, 5*time.Second, StatusCompleted)

	mu.Lock()
	got := callCount
	mu.Unlock()
	if got != 2 {
		t.Errorf("got call count %d; want 2", got)
	}
}

func TestRunner_CancellationWorks(t *testing.T) {
	runner, db := newTestRunner(t)

	started := make(chan struct{})

	runner.Register("CANCELLABLE_TASK", func(ctx context.Context, payload string, reporter Reporter) error {
		close(started)
		// Block until context is cancelled.
		<-ctx.Done()
		return ctx.Err()
	})

	taskID, err := runner.Enqueue(context.Background(), "CANCELLABLE_TASK", "")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// Wait for the task to start.
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("task did not start within timeout")
	}

	// Cancel the task.
	if err := runner.Cancel(context.Background(), taskID); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	// The task should end up FAILED (context.Canceled is returned as an error)
	// or CANCELLED (if the DB update races ahead).
	status := waitForStatus(t, db, taskID, 5*time.Second, StatusFailed, StatusCancelled)
	if status != StatusFailed && status != StatusCancelled {
		t.Errorf("got status %q; want FAILED or CANCELLED", status)
	}
}

func TestRunner_MarkInterruptedFailed(t *testing.T) {
	runner, db := newTestRunner(t)

	// Manually insert a RUNNING task to simulate an interrupted server.
	q := New(db)
	task, err := q.CreateTask(context.Background(), CreateTaskParams{
		TaskType: "INTERRUPTED_TASK",
		Payload:  sql.NullString{},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := q.StartTask(context.Background(), task.ID); err != nil {
		t.Fatalf("start task: %v", err)
	}

	// Verify it is RUNNING.
	got, err := q.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != StatusRunning {
		t.Fatalf("expected RUNNING before MarkInterruptedFailed; got %q", got.Status)
	}

	// Call MarkInterruptedFailed.
	if err := runner.MarkInterruptedFailed(context.Background()); err != nil {
		t.Fatalf("mark interrupted failed: %v", err)
	}

	// Verify it is now FAILED.
	got, err = q.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task after mark: %v", err)
	}
	if got.Status != StatusFailed {
		t.Errorf("got status %q; want FAILED", got.Status)
	}
	if !got.Error.Valid || got.Error.String != "interrupted by server restart" {
		t.Errorf("got error %v; want 'interrupted by server restart'", got.Error)
	}
}

func TestRunner_TaskFails(t *testing.T) {
	runner, db := newTestRunner(t)

	wantErr := errors.New("something went wrong")
	runner.Register("FAILING_TASK", func(ctx context.Context, payload string, reporter Reporter) error {
		return wantErr
	})

	taskID, err := runner.Enqueue(context.Background(), "FAILING_TASK", "")
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	waitForStatus(t, db, taskID, 5*time.Second, StatusFailed)

	q := New(db)
	task, err := q.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if !task.Error.Valid || task.Error.String != wantErr.Error() {
		t.Errorf("got error %v; want %q", task.Error, wantErr.Error())
	}
}

func TestRunner_UnregisteredTaskType(t *testing.T) {
	runner, _ := newTestRunner(t)

	_, err := runner.Enqueue(context.Background(), "NONEXISTENT_TASK", "")
	if err == nil {
		t.Error("expected error for unregistered task type; got nil")
	}
}
