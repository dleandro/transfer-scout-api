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

// handleListRumours returns the rumours feed. It does not yet enrich
// player/club names or crests — see milestone 1.5.
func (s *Server) handleListRumours(w http.ResponseWriter, r *http.Request) {
	rumours, err := s.store.ListRumours(r.Context(), 50, 0)
	if err != nil {
		http.Error(w, "failed to list rumours", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, rumours)
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
