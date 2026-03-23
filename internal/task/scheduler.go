package task

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/robfig/cron/v3"
)

// defaultCronSchedules defines the default cron schedules seeded on startup.
var defaultCronSchedules = []struct {
	taskType string
	cronExpr string
	enabled  int64
}{
	{TypeLibraryScan, "0 */6 * * *", 1},
	{TypeDuplicateDetection, "0 2 * * 0", 1},
	{TypeRecommendationRebuild, "0 3 * * *", 1},
	{TypeAuditLogCleanup, "0 1 * * *", 1},
}

// Scheduler manages cron-based task scheduling.
type Scheduler struct {
	runner *Runner
	db     *sql.DB
	cron   *cron.Cron
	logger *slog.Logger
}

// NewScheduler creates a new Scheduler.
func NewScheduler(runner *Runner, db *sql.DB, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		runner: runner,
		db:     db,
		cron:   cron.New(),
		logger: logger,
	}
}

// Start seeds default cron configs, loads all enabled configs from DB,
// registers them with the cron scheduler, and starts it.
func (s *Scheduler) Start(ctx context.Context) {
	if err := s.seedDefaults(ctx); err != nil {
		s.logger.Error("seed default cron configs", "error", err)
	}

	q := New(s.db)
	configs, err := q.ListCronConfigs(ctx)
	if err != nil {
		s.logger.Error("list cron configs", "error", err)
		return
	}

	for _, cfg := range configs {
		if cfg.Enabled == 0 {
			continue
		}

		taskType := cfg.TaskType
		cronExpr := cfg.CronExpr

		_, err := s.cron.AddFunc(cronExpr, func() {
			runCtx := context.Background()
			taskID, enqueueErr := s.runner.Enqueue(runCtx, taskType, "")
			if enqueueErr != nil {
				s.logger.Warn("cron task enqueue failed",
					"task_type", taskType,
					"error", enqueueErr,
				)
				return
			}
			s.logger.Info("cron task enqueued",
				"task_type", taskType,
				"task_id", taskID,
			)
		})
		if err != nil {
			s.logger.Error("register cron job",
				"task_type", taskType,
				"cron_expr", cronExpr,
				"error", err,
			)
			continue
		}

		s.logger.Info("cron job registered",
			"task_type", taskType,
			"cron_expr", cronExpr,
		)
	}

	s.cron.Start()
	s.logger.Info("task scheduler started")
}

// Stop stops the cron scheduler gracefully.
func (s *Scheduler) Stop() {
	s.cron.Stop()
	s.logger.Info("task scheduler stopped")
}

// seedDefaults inserts default cron configurations if they do not already exist.
func (s *Scheduler) seedDefaults(ctx context.Context) error {
	q := New(s.db)

	for _, d := range defaultCronSchedules {
		_, err := q.GetCronConfig(ctx, d.taskType)
		if err == nil {
			// Already exists — do not overwrite.
			continue
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("get cron config %q: %w", d.taskType, err)
		}

		// Insert the default.
		if upsertErr := q.UpsertCronConfig(ctx, UpsertCronConfigParams{
			TaskType: d.taskType,
			CronExpr: d.cronExpr,
			Enabled:  d.enabled,
		}); upsertErr != nil {
			return fmt.Errorf("upsert cron config %q: %w", d.taskType, upsertErr)
		}

		s.logger.Info("seeded default cron config",
			"task_type", d.taskType,
			"cron_expr", d.cronExpr,
		)
	}

	return nil
}
