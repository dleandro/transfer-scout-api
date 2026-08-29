package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// healthzTimeout bounds how long the DB ping in handleHealth can take,
// so a slow/unreachable database doesn't hang the health check itself.
const healthzTimeout = 2 * time.Second

const (
	// defaultRumoursLimit is used when the `limit` query param is absent.
	defaultRumoursLimit = 50
	// maxRumoursLimit caps `limit` regardless of what the client asks
	// for, to prevent a client from requesting an unbounded page.
	maxRumoursLimit = 100
)

// handleHealth reports liveness plus DB connectivity. The two are
// genuinely different facts under a database with autosuspend/resume
// behavior (e.g. Neon) — the process can be up while the DB is not yet
// reachable.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), healthzTimeout)
	defer cancel()

	if err := s.store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "degraded",
			"db":     "unreachable",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleListRumours returns a page of the rumours feed, enriched with
// player and club names/crests.
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

	items, hasMore, err := s.store.ListRumours(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, "failed to list rumours", http.StatusInternalServerError)
		return
	}

	views := make([]rumourView, len(items))
	for i, item := range items {
		views[i] = newRumourView(item)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rumours":  views,
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

// handleGetRumour returns a single rumour, enriched with player/club
// names and crests, together with its full event timeline (each event
// enriched with its source name and article URL/title).
func (s *Server) handleGetRumour(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid rumour id", http.StatusBadRequest)
		return
	}

	item, events, err := s.store.GetRumourByID(r.Context(), id)
	if err != nil {
		http.Error(w, "rumour not found", http.StatusNotFound)
		return
	}

	eventViews := make([]rumourEventView, len(events))
	for i, ev := range events {
		eventViews[i] = newRumourEventView(ev)
	}

	writeJSON(w, http.StatusOK, rumourDetailView{
		rumourView: newRumourView(*item),
		Events:     eventViews,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
