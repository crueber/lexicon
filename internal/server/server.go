package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/go-chi/chi/v5"
)

// Server is the main HTTP server for Lexicon.
type Server struct {
	cfg    Config
	router *chi.Mux
	logger *slog.Logger
}

// New creates a new Server by parsing configuration from environment variables
// and setting up the router and middleware.
func New() (*Server, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	logger := newLogger(cfg.LogLevel, cfg.LogFormat)

	s := &Server{
		cfg:    cfg,
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
