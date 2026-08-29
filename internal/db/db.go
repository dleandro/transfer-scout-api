// Package db provides the shared Postgres connection pool.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool tuning for Cloud Run + Neon: Cloud Run's scale-to-zero means every
// cold start opens fresh connections, so per-instance pools must stay
// small and not hold idle connections through Neon's autosuspend.
const (
	maxConns        = 5
	minConns        = 0
	maxConnLifetime = 30 * time.Minute
	maxConnIdleTime = 4 * time.Minute
)

// New opens a pgx connection pool and verifies connectivity with a ping.
func New(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: parse config: %w", err)
	}

	poolConfig.MaxConns = maxConns
	poolConfig.MinConns = minConns
	poolConfig.MaxConnLifetime = maxConnLifetime
	poolConfig.MaxConnIdleTime = maxConnIdleTime

	// NOTE: Neon's pooled (PgBouncer transaction-mode) endpoint can
	// conflict with pgx's prepared-statement caching under concurrent
	// pooled connections ("prepared statement already exists"). Not
	// applied preemptively — no live pooled endpoint has been validated
	// against yet (see PRODUCTION_ROADMAP.md Task 3.1/3.2). If that
	// surfaces once Neon is provisioned, set:
	//   poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("db: create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}

	return pool, nil
}
