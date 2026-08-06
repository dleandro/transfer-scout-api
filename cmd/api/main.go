// Command api serves the Transfer Scout REST API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dleandro/transfer-scout-api/internal/api"
	"github.com/dleandro/transfer-scout-api/internal/auth"
	"github.com/dleandro/transfer-scout-api/internal/config"
	"github.com/dleandro/transfer-scout-api/internal/db"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}
	// AuthJWTSecret/GoogleClientID aren't validated in config.Load() since
	// cmd/ingest/cmd/extract share it and never need them — cmd/api fails
	// fast on them here instead, same spirit as DatabaseURL.
	if cfg.AuthJWTSecret == "" {
		slog.Error("config", "error", "AUTH_JWT_SECRET is required")
		os.Exit(1)
	}
	if cfg.GoogleClientID == "" {
		slog.Error("config", "error", "GOOGLE_CLIENT_ID is required")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("db", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	googleVerifier, err := auth.NewGoogleVerifier(ctx, cfg.GoogleClientID)
	if err != nil {
		slog.Error("auth", "error", err)
		os.Exit(1)
	}

	srv := api.NewServer(store.New(pool), cfg.AuthJWTSecret, googleVerifier)

	httpServer := &http.Server{
		Addr:    ":" + cfg.APIPort,
		Handler: srv.Router(),
	}

	go func() {
		slog.Info("api: listening", "port", cfg.APIPort)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("api: serve", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("api: shutdown", "error", err)
	}
	slog.Info("api: stopped")
}
