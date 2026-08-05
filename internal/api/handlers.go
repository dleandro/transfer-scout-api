package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const (
	// defaultRumoursLimit is used when the `limit` query param is absent.
	defaultRumoursLimit = 50
	// maxRumoursLimit caps `limit` regardless of what the client asks
	// for, to prevent a client from requesting an unbounded page.
	maxRumoursLimit = 100
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleListRumours returns a page of the rumours feed. It does not yet
// enrich player/club names or crests — see milestone 1.5.
//
// Query params:
//   - limit: page size. Default 50. Values outside [1, 100] are clamped
//     into range rather than rejected (e.g. limit=500 silently becomes
//     100). A value that isn't a valid integer (e.g. limit=abc) is a
//     client error worth surfacing, so that returns 400.
//   - offset: rows to skip. Default 0. Negative values are clamped to 0
//     for the same reason limit's range is clamped. A non-integer value
//     returns 400.
//
// Response shape: {"rumours": [...], "has_more": bool}. has_more tells
// the caller whether requesting the next offset would return more rows.
// This replaces the previous bare-array response shape.
func (s *Server) handleListRumours(w http.ResponseWriter, r *http.Request) {
	limit, err := parseIntParam(r, "limit", defaultRumoursLimit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	switch {
	case limit < 1:
		limit = 1
	case limit > maxRumoursLimit:
		limit = maxRumoursLimit
	}

	offset, err := parseIntParam(r, "offset", 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if offset < 0 {
		offset = 0
	}

	rumours, hasMore, err := s.store.ListRumours(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, "failed to list rumours", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"rumours":  rumours,
		"has_more": hasMore,
	})
}

// parseIntParam parses the named query param as an int, returning def if
// the param is absent. An error is returned (the caller turns this into
// a 400) only when the param is present but not a valid integer — range
// clamping for valid-but-out-of-bounds values is the caller's job.
func parseIntParam(r *http.Request, name string, def int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: must be an integer", name)
	}
	return v, nil
}

// handleGetRumour returns a single rumour with its raw event timeline. It
// does not yet enrich player/club names or crests in the response shape —
// see milestone 1.5.
func (s *Server) handleGetRumour(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid rumour id", http.StatusBadRequest)
		return
	}

	rumour, events, err := s.store.GetRumourByID(r.Context(), id)
	if err != nil {
		http.Error(w, "rumour not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"rumour": rumour,
		"events": events,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
