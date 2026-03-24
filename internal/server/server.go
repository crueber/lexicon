package server

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/crueber/lexicon/internal/auth"
	"github.com/crueber/lexicon/internal/book"
	"github.com/crueber/lexicon/internal/dashboard"
	"github.com/crueber/lexicon/internal/library"
	"github.com/crueber/lexicon/internal/metadata"
	"github.com/crueber/lexicon/internal/notebook"
	"github.com/crueber/lexicon/internal/opds"
	"github.com/crueber/lexicon/internal/reader"
	"github.com/crueber/lexicon/internal/shelf"
	"github.com/crueber/lexicon/internal/storage"
	"github.com/crueber/lexicon/internal/task"
	"github.com/crueber/lexicon/internal/user"
	"github.com/crueber/lexicon/internal/ws"
)

// Server is the main HTTP server for Lexicon.
type Server struct {
	cfg              Config
	db               *sql.DB
	router           *chi.Mux
	logger           *slog.Logger
	authHandler      *auth.Handler
	userHandler      *user.Handler
	libraryHandler   *library.Handler
	bookHandler      *book.Handler
	storageHandler   *storage.Handler
	readerHandler    *reader.Handler
	notebookHandler  *notebook.Handler
	shelfHandler     *shelf.Handler
	dashboardHandler *dashboard.Handler
	metadataHandler  *metadata.Handler
	opdsHandler      *opds.Handler
	hub              *ws.Hub
	wsHandler        *ws.Handler
	watcher          *library.Watcher
	taskRunner       *task.Runner
	taskScheduler    *task.Scheduler
	taskHandler      *task.Handler
}

// New creates a new Server with the given configuration, opens the database,
// runs migrations, and sets up the router and middleware.
func New(cfg Config) (*Server, error) {
	logger := newLogger(cfg.LogLevel, cfg.LogFormat)

	db, err := OpenDatabase(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := RunMigrations(db, logger); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	librarySvc := library.NewService(db, logger)
	libraryScanner := library.NewScanner(db, cfg.DataDir, logger)

	hub := ws.NewHub(logger)
	wsHandler := ws.NewHandler(hub, cfg.JWTSecret, logger)

	watcher, err := library.NewWatcher(db, libraryScanner, hub, logger)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create file watcher: %w", err)
	}

	// Set up the background task system.
	taskRunner := task.NewRunner(db, hub, logger)
	taskRunner.Register(task.TypeLibraryScan, task.NewLibraryScanFunc(db, librarySvc, libraryScanner, logger))

	taskScheduler := task.NewScheduler(taskRunner, db, logger)
	taskHandler := task.NewHandler(taskRunner, taskScheduler, db, logger)

	libraryHandler := library.NewHandler(librarySvc, libraryScanner, logger)
	libraryHandler.WithTaskEnqueue(func(taskType, payload string) (int64, error) {
		return taskRunner.Enqueue(context.Background(), taskType, payload)
	})

	shelfSvc := shelf.NewService(db, logger)
	shelfHdlr := shelf.NewHandler(shelfSvc, logger)

	bookHdlr := book.NewHandler(db, logger)
	bookHdlr.WithShelfHandler(shelfHdlr)

	// Build the user handler with injected dependencies to avoid import cycles.
	// The user package cannot import auth (auth imports user), so we inject
	// the principal extractor and library access setter as functions.
	userHdlr := user.NewHandler(db, logger,
		func(ctx context.Context) *user.Principal {
			p := auth.PrincipalFromContext(ctx)
			if p == nil {
				return nil
			}
			return &user.Principal{
				UserID:   p.UserID,
				Username: p.Username,
				Role:     p.Role,
			}
		},
		func(ctx context.Context, db *sql.DB, userID int64, libraryIDs []int64) error {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				return fmt.Errorf("begin transaction: %w", err)
			}
			defer tx.Rollback()

			lq := library.New(tx)
			if err := lq.ClearUserLibraryPermissions(ctx, userID); err != nil {
				return fmt.Errorf("clear library permissions: %w", err)
			}
			for _, libID := range libraryIDs {
				if err := lq.GrantLibraryAccess(ctx, library.GrantLibraryAccessParams{
					UserID:    userID,
					LibraryID: libID,
				}); err != nil {
					return fmt.Errorf("grant library access %d: %w", libID, err)
				}
			}
			return tx.Commit()
		},
	)

	// Set up the metadata service and register providers.
	metadataSvc := metadata.NewService(db, logger)
	metadataSvc.RegisterProvider(metadata.NewGoogleBooksProvider(cfg.GoogleBooksAPIKey, logger))
	metadataSvc.RegisterProvider(metadata.NewOpenLibraryProvider(logger))
	metadataSvc.RegisterProvider(metadata.NewHardcoverProvider(cfg.HardcoverAPIKey, logger))
	metadataSvc.RegisterProvider(metadata.NewComicVineProvider(cfg.ComicVineAPIKey, logger))
	metadataSvc.RegisterProvider(metadata.NewAudibleProvider(logger))
	metadataHdlr := metadata.NewHandler(metadataSvc, logger)

	s := &Server{
		cfg:              cfg,
		db:               db,
		router:           chi.NewRouter(),
		logger:           logger,
		authHandler:      auth.NewHandler(db, cfg.JWTSecret, logger),
		userHandler:      userHdlr,
		libraryHandler:   libraryHandler,
		bookHandler:      bookHdlr,
		storageHandler:   storage.NewHandler(db, cfg.DataDir, logger),
		readerHandler:    reader.NewHandler(db, logger),
		notebookHandler:  notebook.NewHandler(db, logger),
		shelfHandler:     shelfHdlr,
		dashboardHandler: dashboard.NewHandler(db, logger),
		metadataHandler:  metadataHdlr,
		opdsHandler:      opds.NewHandler(db, logger),
		hub:              hub,
		wsHandler:        wsHandler,
		watcher:          watcher,
		taskRunner:       taskRunner,
		taskScheduler:    taskScheduler,
		taskHandler:      taskHandler,
	}

	if err := s.ensureDefaultAdmin(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensure default admin: %w", err)
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s, nil
}

// ensureDefaultAdmin checks if any users exist and creates a default admin
// user if the database is empty. This enables first-run setup.
func (s *Server) ensureDefaultAdmin() error {
	ctx := context.Background()
	q := user.New(s.db)

	count, err := q.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("count users: %w", err)
	}

	if count > 0 {
		return nil
	}

	_, err = user.CreateAdminUser(ctx, s.db, user.CreateUserServiceParams{
		Username: "admin",
		Password: "admin",
		Name:     "Administrator",
	})
	if err != nil {
		return fmt.Errorf("create default admin: %w", err)
	}

	s.logger.Warn("default admin user created — change the password immediately",
		"username", "admin",
	)

	return nil
}

// Start begins listening for HTTP requests and shuts down gracefully on
// SIGINT or SIGTERM.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.cfg.Port)
	s.logger.Info("starting server",
		"addr", addr,
		"dev_mode", s.cfg.DevMode,
		"data_dir", s.cfg.DataDir,
	)

	// Mark any tasks that were running when the server last stopped as failed.
	if err := s.taskRunner.MarkInterruptedFailed(context.Background()); err != nil {
		s.logger.Error("mark interrupted tasks failed", "error", err)
	}

	// Start the task scheduler.
	s.taskScheduler.Start(context.Background())

	srv := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	// Start the file watcher in a background goroutine.
	watcherCtx, watcherCancel := context.WithCancel(context.Background())
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		if err := s.watcher.Start(watcherCtx); err != nil {
			s.logger.Error("file watcher error", "error", err)
		}
	}()

	// Channel to capture server errors from ListenAndServe.
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for interrupt signal or server error.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-quit:
		s.logger.Info("received shutdown signal", "signal", sig.String())
	case err := <-errCh:
		s.taskScheduler.Stop()
		watcherCancel()
		_ = s.watcher.Close()
		<-watcherDone
		return fmt.Errorf("server listen: %w", err)
	}

	// Stop the task scheduler.
	s.taskScheduler.Stop()

	// Stop the file watcher.
	watcherCancel()
	_ = s.watcher.Close()
	<-watcherDone

	// Give active connections 10 seconds to finish.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	// Close the database after the HTTP server has stopped accepting requests.
	if err := s.db.Close(); err != nil {
		s.logger.Error("database close error", "error", err)
	}

	s.logger.Info("server stopped gracefully")
	return nil
}

// newLogger creates a structured logger based on the configured level and format.
func newLogger(level, format string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	switch strings.ToLower(format) {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
