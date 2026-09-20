package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/auth"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

// handleFollowClub requires authentication (see auth.RequireAuth in
// Router). Idempotent: following an already-followed club still returns
// 204 (see store.FollowClub).
func (s *Server) handleFollowClub(w http.ResponseWriter, r *http.Request) {
	clubID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid club id", http.StatusBadRequest)
		return
	}

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		// Unreachable in practice: this handler is only ever mounted
		// behind auth.RequireAuth, which already 401s before this runs.
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := s.store.FollowClub(r.Context(), userID, clubID); err != nil {
		if errors.Is(err, store.ErrClubNotFound) {
			http.Error(w, "club not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to follow club", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleUnfollowClub requires authentication (see auth.RequireAuth in
// Router). Idempotent: unfollowing a club that was never followed (or is
// already unfollowed) still returns 204 (see store.UnfollowClub).
func (s *Server) handleUnfollowClub(w http.ResponseWriter, r *http.Request) {
	clubID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid club id", http.StatusBadRequest)
		return
	}

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := s.store.UnfollowClub(r.Context(), userID, clubID); err != nil {
		if errors.Is(err, store.ErrClubNotFound) {
			http.Error(w, "club not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to unfollow club", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
