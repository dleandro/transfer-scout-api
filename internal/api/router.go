// Package api implements the Transfer Scout REST API.
package api

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/models"
)

// Store is the subset of store.Store the API needs. Defined here (rather
// than depending on the concrete *store.Store) so Server can be exercised
// in tests against a fake, without a real Postgres connection — mirrors
// the pattern already used in internal/ingest.
type Store interface {
	ListRumours(ctx context.Context, limit, offset int) ([]models.Rumour, bool, error)
	GetRumourByID(ctx context.Context, id uuid.UUID) (*models.Rumour, []models.RumourEvent, error)
	Ping(ctx context.Context) error
}

type Server struct {
	store Store
}

func NewServer(s Store) *Server {
	return &Server{store: s}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", s.handleHealth)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/rumours", s.handleListRumours)
		r.Get("/rumours/{id}", s.handleGetRumour)
	})

	return r
}
