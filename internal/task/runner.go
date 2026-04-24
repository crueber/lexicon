package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/crueber/lexicon/internal/ws"
)

// Runner manages background task execution.
type Runner struct {
	db      *sql.DB
	hub     *ws.Hub
	logger  *slog.Logger
	mu      sync.Mutex
	running map[string]context.CancelFunc // taskType → cancel func
	funcs   map[string]TaskFunc           // registered task implementations
}

// NewRunner creates a new Runner.
func NewRunner(db *sql.DB, hub *ws.Hub, logger *slog.Logger) *Runner {
	return &Runner{
		db:      db,
		hub:     hub,
		logger:  logger,
		running: make(map[string]context.CancelFunc),
		funcs:   make(map[string]TaskFunc),
	}
}

// Register registers a task implementation for the given task type.
func (r *Runner) Register(taskType string, fn TaskFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.funcs[taskType] = fn
}

// Enqueue creates a task record and runs it in a goroutine.
// Returns an error if a task of this type is already running.
func (r *Runner) Enqueue(ctx context.Context, taskType, payload string) (int64, error) {
	r.mu.Lock()
	if _, running := r.running[taskType]; running {
		r.mu.Unlock()
		return 0, fmt.Errorf("task type %q is already running", taskType)
	}

	fn, ok := r.funcs[taskType]
	if !ok {
		r.mu.Unlock()
		return 0, fmt.Errorf("no implementation registered for task type %q", taskType)
	}

	// Create the task record while holding the lock to prevent races.
	q := New(r.db)
	t, err := q.CreateTask(ctx, CreateTaskParams{
		TaskType: taskType,
		Payload:  sql.NullString{String: payload, Valid: payload != ""},
	})
	if err != nil {
		r.mu.Unlock()
		return 0, fmt.Errorf("create task record: %w", err)
	}

	taskCtx, cancel := context.WithCancel(context.Background())
	r.running[taskType] = cancel
	r.mu.Unlock()

	go r.execute(taskCtx, cancel, t.ID, taskType, payload, fn)

	return t.ID, nil
}

// execute runs the task function and updates the task record on completion.
func (r *Runner) execute(ctx context.Context, cancel context.CancelFunc, taskID int64, taskType, payload string, fn TaskFunc) {
	defer cancel()
	defer func() {
		r.mu.Lock()
		delete(r.running, taskType)
		r.mu.Unlock()
	}()

	q := New(r.db)

	// Mark task as running.
	if err := q.StartTask(ctx, taskID); err != nil {
		r.logger.Error("start task record", "task_id", taskID, "error", err)
	}

	rep := &reporter{
		taskID:   taskID,
		taskType: taskType,
		db:       r.db,
		hub:      r.hub,
	}

	r.broadcastProgress(taskID, taskType, StatusRunning, 0, 0, "")

	// Run the task function.
	taskErr := fn(ctx, payload, rep)

	if taskErr != nil {
		errMsg := taskErr.Error()
		if dbErr := q.FailTask(ctx, FailTaskParams{
			Error: sql.NullString{String: errMsg, Valid: true},
			ID:    taskID,
		}); dbErr != nil {
			r.logger.Error("fail task record", "task_id", taskID, "error", dbErr)
		}
		r.broadcastFailed(taskID, taskType, errMsg)
		if r.hub != nil {
			r.hub.BroadcastNotification([]int64{}, "Task Failed", fmt.Sprintf("%s failed: %s", taskType, errMsg))
		}
		r.logger.Error("task failed", "task_id", taskID, "task_type", taskType, "error", taskErr)
		return
	}

	if err := q.CompleteTask(ctx, taskID); err != nil {
		r.logger.Error("complete task record", "task_id", taskID, "error", err)
	}
	r.broadcastComplete(taskID, taskType)
	if r.hub != nil {
		r.hub.BroadcastNotification([]int64{}, "Task Complete", fmt.Sprintf("%s completed", taskType))
	}
	r.logger.Info("task completed", "task_id", taskID, "task_type", taskType)
}

// Cancel cancels a running task by its ID.
func (r *Runner) Cancel(ctx context.Context, taskID int64) error {
	q := New(r.db)

	t, err := q.GetTask(ctx, taskID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("task %d not found", taskID)
		}
		return fmt.Errorf("get task: %w", err)
	}

	r.mu.Lock()
	cancel, running := r.running[t.TaskType]
	r.mu.Unlock()

	if running {
		cancel()
	}

	if err := q.CancelTask(ctx, taskID); err != nil {
		return fmt.Errorf("cancel task record: %w", err)
	}

	return nil
}

// MarkInterruptedFailed marks any RUNNING tasks as FAILED on startup.
// This handles the case where the server was restarted while tasks were running.
func (r *Runner) MarkInterruptedFailed(ctx context.Context) error {
	q := New(r.db)
	if err := q.MarkInterruptedTasksFailed(ctx); err != nil {
		return fmt.Errorf("mark interrupted tasks failed: %w", err)
	}
	return nil
}

// broadcastProgress sends a TASK_PROGRESS WebSocket event to all clients.
func (r *Runner) broadcastProgress(taskID int64, taskType, status string, progress, total int, message string) {
	r.hub.BroadcastToAll(ws.Message{
		Type: "TASK_PROGRESS",
		Payload: map[string]any{
			"taskId":   taskID,
			"taskType": taskType,
			"status":   status,
			"progress": progress,
			"total":    total,
			"message":  message,
		},
	})
}

// broadcastComplete sends a TASK_COMPLETE WebSocket event to all clients.
func (r *Runner) broadcastComplete(taskID int64, taskType string) {
	r.hub.BroadcastToAll(ws.Message{
		Type: "TASK_COMPLETE",
		Payload: map[string]any{
			"taskId":   taskID,
			"taskType": taskType,
		},
	})
}

// broadcastFailed sends a TASK_FAILED WebSocket event to all clients.
func (r *Runner) broadcastFailed(taskID int64, taskType, errMsg string) {
	r.hub.BroadcastToAll(ws.Message{
		Type: "TASK_FAILED",
		Payload: map[string]any{
			"taskId":   taskID,
			"taskType": taskType,
			"error":    errMsg,
		},
	})
}

// reporter implements Reporter and updates the task record on progress calls.
type reporter struct {
	taskID   int64
	taskType string
	db       *sql.DB
	hub      *ws.Hub
}

// Progress updates the task progress in the database and broadcasts a WebSocket event.
func (rep *reporter) Progress(current, total int, message string) {
	q := New(rep.db)
	if err := q.UpdateTaskProgress(context.Background(), UpdateTaskProgressParams{
		Progress: sql.NullInt64{Int64: int64(current), Valid: true},
		Total:    sql.NullInt64{Int64: int64(total), Valid: true},
		Message:  sql.NullString{String: message, Valid: message != ""},
		ID:       rep.taskID,
	}); err != nil {
		// Non-fatal: progress update failure should not stop the task.
		return
	}

	rep.hub.BroadcastToAll(ws.Message{
		Type: "TASK_PROGRESS",
		Payload: map[string]any{
			"taskId":   rep.taskID,
			"taskType": rep.taskType,
			"status":   StatusRunning,
			"progress": current,
			"total":    total,
			"message":  message,
		},
	})
}

// taskPayloadLibraryScan is the JSON payload for LIBRARY_SCAN tasks.
type taskPayloadLibraryScan struct {
	LibraryID *int64 `json:"libraryId"`
}

// ParseLibraryScanPayload parses a LIBRARY_SCAN task payload.
// Returns nil if the payload is empty or has no libraryId.
func ParseLibraryScanPayload(payload string) (*int64, error) {
	if payload == "" {
		return nil, nil
	}
	var p taskPayloadLibraryScan
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return nil, fmt.Errorf("parse library scan payload: %w", err)
	}
	return p.LibraryID, nil
}
