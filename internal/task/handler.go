package task

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/crueber/lexicon/internal/auth"
)

// Handler handles HTTP requests for the task system.
type Handler struct {
	runner    *Runner
	scheduler *Scheduler
	db        *sql.DB
	logger    *slog.Logger
}

// NewHandler creates a new task Handler.
func NewHandler(runner *Runner, scheduler *Scheduler, db *sql.DB, logger *slog.Logger) *Handler {
	return &Handler{
		runner:    runner,
		scheduler: scheduler,
		db:        db,
		logger:    logger,
	}
}

// Routes registers all task routes on the given router.
// RequireAuth must already be applied by the caller.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.handleList)
	r.Get("/cron", h.handleListCron)
	r.Get("/{id}", h.handleGet)
	r.With(auth.RequireAdmin()).Post("/{type}/run", h.handleRun)
	r.With(auth.RequireAdmin()).Delete("/{id}", h.handleCancel)
	r.With(auth.RequireAdmin()).Put("/cron/{type}", h.handleUpdateCron)
}

// --- Response types ---

// TaskResponse is the JSON representation of a task.
type TaskResponse struct {
	ID          int64   `json:"id"`
	TaskType    string  `json:"taskType"`
	Status      string  `json:"status"`
	Progress    int64   `json:"progress"`
	Total       int64   `json:"total"`
	Message     *string `json:"message,omitempty"`
	Error       *string `json:"error,omitempty"`
	Payload     *string `json:"payload,omitempty"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
	StartedAt   *string `json:"startedAt,omitempty"`
	CompletedAt *string `json:"completedAt,omitempty"`
}

// CronConfigResponse is the JSON representation of a cron configuration.
type CronConfigResponse struct {
	TaskType string `json:"taskType"`
	CronExpr string `json:"cronExpr"`
	Enabled  bool   `json:"enabled"`
}

// UpdateCronRequest is the JSON body for PUT /api/tasks/cron/{type}.
type UpdateCronRequest struct {
	CronExpr string `json:"cronExpr"`
	Enabled  bool   `json:"enabled"`
}

// EnqueueResponse is the JSON response for POST /api/tasks/{type}/run.
type EnqueueResponse struct {
	TaskID int64 `json:"taskId"`
}

// --- Handlers ---

// handleList handles GET /api/tasks.
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	q := New(h.db)
	tasks, err := q.ListRecentTasks(r.Context())
	if err != nil {
		h.logger.Error("list recent tasks", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	responses := make([]TaskResponse, len(tasks))
	for i, t := range tasks {
		responses[i] = toTaskResponse(t)
	}

	writeJSON(w, http.StatusOK, responses)
}

// handleGet handles GET /api/tasks/{id}.
func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	q := New(h.db)
	t, err := q.GetTask(r.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		h.logger.Error("get task", "task_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, toTaskResponse(t))
}

// handleRun handles POST /api/tasks/{type}/run (admin only).
func (h *Handler) handleRun(w http.ResponseWriter, r *http.Request) {
	taskType := chi.URLParam(r, "type")
	if taskType == "" {
		writeError(w, http.StatusBadRequest, "task type is required")
		return
	}

	// Optional payload from request body.
	var payload string
	if r.ContentLength > 0 {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			if b, err := json.Marshal(body); err == nil {
				payload = string(b)
			}
		}
	}

	taskID, err := h.runner.Enqueue(r.Context(), taskType, payload)
	if err != nil {
		h.logger.Warn("enqueue task", "task_type", taskType, "error", err)
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	h.logger.Info("task enqueued via API", "task_type", taskType, "task_id", taskID)
	writeJSON(w, http.StatusAccepted, EnqueueResponse{TaskID: taskID})
}

// handleCancel handles DELETE /api/tasks/{id} (admin only).
func (h *Handler) handleCancel(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r, "id")
	if !ok {
		return
	}

	if err := h.runner.Cancel(r.Context(), id); err != nil {
		h.logger.Warn("cancel task", "task_id", id, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.logger.Info("task cancelled", "task_id", id)
	w.WriteHeader(http.StatusNoContent)
}

// handleListCron handles GET /api/tasks/cron.
func (h *Handler) handleListCron(w http.ResponseWriter, r *http.Request) {
	q := New(h.db)
	configs, err := q.ListCronConfigs(r.Context())
	if err != nil {
		h.logger.Error("list cron configs", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	responses := make([]CronConfigResponse, len(configs))
	for i, c := range configs {
		responses[i] = CronConfigResponse{
			TaskType: c.TaskType,
			CronExpr: c.CronExpr,
			Enabled:  c.Enabled != 0,
		}
	}

	writeJSON(w, http.StatusOK, responses)
}

// handleUpdateCron handles PUT /api/tasks/cron/{type} (admin only).
func (h *Handler) handleUpdateCron(w http.ResponseWriter, r *http.Request) {
	taskType := chi.URLParam(r, "type")
	if taskType == "" {
		writeError(w, http.StatusBadRequest, "task type is required")
		return
	}

	var req UpdateCronRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CronExpr == "" {
		writeError(w, http.StatusBadRequest, "cronExpr is required")
		return
	}

	enabled := int64(0)
	if req.Enabled {
		enabled = 1
	}

	q := New(h.db)
	if err := q.UpsertCronConfig(r.Context(), UpsertCronConfigParams{
		TaskType: taskType,
		CronExpr: req.CronExpr,
		Enabled:  enabled,
	}); err != nil {
		h.logger.Error("upsert cron config", "task_type", taskType, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.logger.Info("cron config updated", "task_type", taskType, "cron_expr", req.CronExpr, "enabled", req.Enabled)
	w.WriteHeader(http.StatusNoContent)
}

// --- Helpers ---

// toTaskResponse converts a Task model to a TaskResponse.
func toTaskResponse(t Task) TaskResponse {
	resp := TaskResponse{
		ID:        t.ID,
		TaskType:  t.TaskType,
		Status:    t.Status,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}

	if t.Progress.Valid {
		resp.Progress = t.Progress.Int64
	}
	if t.Total.Valid {
		resp.Total = t.Total.Int64
	}
	if t.Message.Valid {
		resp.Message = &t.Message.String
	}
	if t.Error.Valid {
		resp.Error = &t.Error.String
	}
	if t.Payload.Valid {
		resp.Payload = &t.Payload.String
	}
	if t.StartedAt.Valid {
		resp.StartedAt = &t.StartedAt.String
	}
	if t.CompletedAt.Valid {
		resp.CompletedAt = &t.CompletedAt.String
	}

	return resp
}

// parseID extracts and parses an integer URL parameter by name.
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
