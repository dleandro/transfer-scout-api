package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/auth"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

const (
	// defaultCommentsLimit is used when the `limit` query param is absent.
	defaultCommentsLimit = 20
	// maxCommentsLimit caps `limit` regardless of what the client asks for.
	maxCommentsLimit = 100

	minCommentBodyLength = 1
	maxCommentBodyLength = 2000
)

// validateCommentBody trims whitespace and enforces the same length rule
// as the comments table's CHECK constraint, so a violation returns a
// clean 400 with a specific message before ever reaching the database.
func validateCommentBody(raw string) (string, error) {
	body := strings.TrimSpace(raw)
	if len(body) < minCommentBodyLength {
		return "", errors.New("body must not be empty")
	}
	if len(body) > maxCommentBodyLength {
		return "", errors.New("body must be at most 2000 characters")
	}
	return body, nil
}

// handleCreateComment requires authentication (see auth.RequireAuth in
// Router). The rumour id comes from the URL, the author from the
// authenticated request context — never from the request body.
func (s *Server) handleCreateComment(w http.ResponseWriter, r *http.Request) {
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

	var reqBody struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	commentBody, err := validateCommentBody(reqBody.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	comment, err := s.store.CreateComment(r.Context(), rumourID, userID, commentBody)
	if err != nil {
		if errors.Is(err, store.ErrRumourNotFound) {
			http.Error(w, "rumour not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to create comment", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, comment)
}

// handleListComments is public — no auth required to read comments, only
// to post them.
//
// Query params: limit/offset, same absent-default/invalid-400/
// out-of-range-clamped convention as handleListRumours's parseIntParam.
func (s *Server) handleListComments(w http.ResponseWriter, r *http.Request) {
	rumourID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid rumour id", http.StatusBadRequest)
		return
	}

	limit, err := parseIntParam(r, "limit", defaultCommentsLimit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	switch {
	case limit < 1:
		limit = 1
	case limit > maxCommentsLimit:
		limit = maxCommentsLimit
	}

	offset, err := parseIntParam(r, "offset", 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if offset < 0 {
		offset = 0
	}

	exists, err := s.store.RumourExists(r.Context(), rumourID)
	if err != nil {
		http.Error(w, "failed to look up rumour", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "rumour not found", http.StatusNotFound)
		return
	}

	comments, hasMore, err := s.store.ListComments(r.Context(), rumourID, limit, offset)
	if err != nil {
		http.Error(w, "failed to list comments", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"comments": comments,
		"has_more": hasMore,
	})
}
