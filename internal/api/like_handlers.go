package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/auth"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

// handleLikeRumour requires authentication (see auth.RequireAuth in
// Router). Idempotent: liking an already-liked rumour still returns 204
// (see store.LikeRumour).
func (s *Server) handleLikeRumour(w http.ResponseWriter, r *http.Request) {
	rumourID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid rumour id", http.StatusBadRequest)
		return
	}

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		// Unreachable in practice: this handler is only ever mounted
		// behind auth.RequireAuth, which already 401s before this runs.
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := s.store.LikeRumour(r.Context(), rumourID, userID); err != nil {
		if errors.Is(err, store.ErrRumourNotFound) {
			http.Error(w, "rumour not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to like rumour", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleUnlikeRumour requires authentication (see auth.RequireAuth in
// Router). Idempotent: unliking a rumour that was never liked (or is
// already unliked) still returns 204 (see store.UnlikeRumour).
func (s *Server) handleUnlikeRumour(w http.ResponseWriter, r *http.Request) {
	rumourID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid rumour id", http.StatusBadRequest)
		return
	}

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := s.store.UnlikeRumour(r.Context(), rumourID, userID); err != nil {
		if errors.Is(err, store.ErrRumourNotFound) {
			http.Error(w, "rumour not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to unlike rumour", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
