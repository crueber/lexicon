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
)

// Server is the main HTTP server for Lexicon.
type Server struct {
	cfg    Config
	db     *sql.DB
	router *chi.Mux
	logger *slog.Logger
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

	s := &Server{
		cfg:    cfg,
		db:     db,
		router: chi.NewRouter(),
		logger: logger,
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s, nil
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

	srv := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

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
		return fmt.Errorf("server listen: %w", err)
	}

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
