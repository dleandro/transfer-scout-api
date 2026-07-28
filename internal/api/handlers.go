package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleListRumours returns the rumours feed, enriched with player and
// club names/crests.
func (s *Server) handleListRumours(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListRumours(r.Context(), 50, 0)
	if err != nil {
		http.Error(w, "failed to list rumours", http.StatusInternalServerError)
		return
	}

	views := make([]rumourView, len(items))
	for i, item := range items {
		views[i] = newRumourView(item)
	}
	writeJSON(w, http.StatusOK, views)
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
