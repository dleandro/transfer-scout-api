// Package api implements the Transfer Scout REST API.
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/models"
)

// requestTimeout bounds how long any single request may take.
const requestTimeout = 10 * time.Second

// rumoursRateLimit caps requests per IP to /api/v1 — the API is
// unauthenticated and public by design (no auth planned for this launch),
// but sits in front of a free-tier database, so this is cheap insurance
// against scraping or accidental hammering rather than an access-control
// mechanism. /healthz is deliberately excluded so Cloud Run's own health
// probes are never throttled.
const rumoursRateLimit = 60 // requests per minute per IP

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
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(requestTimeout))

	r.Get("/healthz", s.handleHealth)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(httprate.LimitByIP(rumoursRateLimit, time.Minute))
		r.Get("/rumours", s.handleListRumours)
		r.Get("/rumours/{id}", s.handleGetRumour)
	})

	return r
}
