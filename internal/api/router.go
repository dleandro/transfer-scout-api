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

	"github.com/dleandro/transfer-scout-api/internal/auth"
	"github.com/dleandro/transfer-scout-api/internal/models"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

// requestTimeout bounds how long any single request may take.
const requestTimeout = 10 * time.Second

// rumoursRateLimit caps requests per IP to /api/v1 — the GET endpoints
// stay public/unauthenticated by design, but sit in front of a free-tier
// database, so this is cheap insurance against scraping or accidental
// hammering rather than an access-control mechanism. Mutating endpoints
// (comments, reactions) get their own, stricter, per-user limiter on top
// of this one. /healthz is deliberately excluded so Cloud Run's own
// health probes are never throttled.
const rumoursRateLimit = 60 // requests per minute per IP

// mutationsRateLimit caps authenticated mutating requests (comments,
// reactions) per user — much stricter than rumoursRateLimit, and keyed by
// user id rather than IP now that auth exists, since multiple users can
// share an IP (NAT) and a single spammer with one account is what this
// guards against.
const mutationsRateLimit = 10 // requests per minute per user

// Store is the subset of store.Store the API needs. Defined here (rather
// than depending on the concrete *store.Store) so Server can be exercised
// in tests against a fake, without a real Postgres connection — mirrors
// the pattern already used in internal/ingest.
type Store interface {
	ListRumours(ctx context.Context, limit, offset int) ([]store.RumourFeedItem, bool, error)
	GetRumourByID(ctx context.Context, id uuid.UUID) (*store.RumourFeedItem, []store.RumourEventItem, error)
	RumourExists(ctx context.Context, id uuid.UUID) (bool, error)
	Ping(ctx context.Context) error
	UpsertUser(ctx context.Context, googleSub, email, displayName, avatarURL string) (*models.User, error)
	CreateComment(ctx context.Context, rumourID, userID uuid.UUID, body string) (*models.Comment, error)
	ListComments(ctx context.Context, rumourID uuid.UUID, limit, offset int) (comments []models.Comment, hasMore bool, err error)
}

type Server struct {
	store          Store
	authSecret     string
	googleVerifier *auth.GoogleVerifier
}

func NewServer(s Store, authSecret string, googleVerifier *auth.GoogleVerifier) *Server {
	return &Server{store: s, authSecret: authSecret, googleVerifier: googleVerifier}
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
		r.Get("/rumours/{id}/comments", s.handleListComments)
		r.Post("/auth/google", s.handleGoogleAuth)

		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(s.authSecret))
			r.Use(httprate.LimitBy(mutationsRateLimit, time.Minute, auth.KeyByUserID))
			r.Post("/rumours/{id}/comments", s.handleCreateComment)
		})
	})

	return r
}
