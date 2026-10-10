package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/dleandro/transfer-scout-api/internal/config"
	"github.com/dleandro/transfer-scout-api/internal/db"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

func main() {
	if len(os.Args) < 2 || (os.Args[1] != "up" && os.Args[1] != "down") {
		fmt.Fprintln(os.Stderr, "usage: migrate <up|down>")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}

	m, err := migrate.New("file://migrations", cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate init error:", err)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "up":
		err = m.Up()
	case "down":
		err = m.Down()
	}

	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		fmt.Fprintln(os.Stderr, "migrate error:", err)
		os.Exit(1)
	}

	fmt.Println("migrate:", os.Args[1], "done")

	if os.Args[1] == "up" {
		if err := syncRoster(cfg.DatabaseURL); err != nil {
			fmt.Fprintln(os.Stderr, "roster sync error:", err)
			os.Exit(1)
		}
	}
}

func syncRoster(databaseURL string) error {
	ctx := context.Background()
	pool, err := db.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	started := time.Now()
	result, err := store.New(pool).SyncRoster(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("roster sync: %d leagues, %d clubs upserted, %d clubs detached, %d renamed, %d left unmerged in %s\n",
		result.Leagues, result.Clubs, result.Detached, len(result.Renamed), len(result.Unmerged),
		time.Since(started).Round(time.Millisecond))
	for _, rename := range result.Renamed {
		fmt.Println("roster sync: renamed", rename)
	}
	for _, duplicate := range result.Unmerged {
		fmt.Println("roster sync: left unmerged", duplicate)
	}
	return nil
}
