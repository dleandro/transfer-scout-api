// Command ingest polls configured RSS feeds and stores new articles for
// later extraction. It is a one-shot batch job, not a ticker loop — run
// it on a schedule (Cloud Scheduler triggering a Cloud Run Job; cron/
// systemd timer locally). See internal/ingest for the poll logic.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dleandro/transfer-scout-api/internal/config"
	"github.com/dleandro/transfer-scout-api/internal/db"
	"github.com/dleandro/transfer-scout-api/internal/ingest"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
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

	poller := ingest.NewPoller(store.New(pool))

	slog.Info("ingest: starting")
	poller.PollOnce(ctx)
	slog.Info("ingest: done")
}
