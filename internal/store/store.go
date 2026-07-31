// Package store contains the Postgres-backed repository methods shared by
// the api, ingest, and extract binaries.
package store

import "github.com/jackc/pgx/v5/pgxpool"

type Store struct {
	Pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}
