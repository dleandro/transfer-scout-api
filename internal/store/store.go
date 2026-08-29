// Package store contains the Postgres-backed repository methods shared by
// the api, ingest, and extract binaries.
package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	Pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

// Ping verifies the database is reachable. Used by the API's /healthz
// endpoint to distinguish "process is up" from "DB is reachable" — these
// are genuinely different facts under a database with autosuspend/resume
// behavior (e.g. Neon).
func (s *Store) Ping(ctx context.Context) error {
	return s.Pool.Ping(ctx)
}
