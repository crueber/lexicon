package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/caarlos0/env/v11"

	"github.com/crueber/lexicon/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := env.ParseAs[server.Config]()
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Handle JWT_SECRET: require it in production, use a dev default in dev mode.
	if cfg.JWTSecret == "" {
		if !cfg.DevMode {
			return fmt.Errorf("JWT_SECRET is required (set DEV_MODE=true for development)")
		}
		slog.Warn("JWT_SECRET not set — using insecure dev default (do not use in production)")
		if err := os.Setenv("JWT_SECRET", "dev-insecure-jwt-secret-do-not-use-in-production"); err != nil {
			return fmt.Errorf("set dev JWT_SECRET: %w", err)
		}
		cfg.JWTSecret = "dev-insecure-jwt-secret-do-not-use-in-production"
	}

	srv, err := server.New(cfg)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	return srv.Start()
}
