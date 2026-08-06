package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/dleandro/transfer-scout-api/internal/auth"
)

// authTokenTTL is deliberately long (30 days, matching the web frontend's
// NextAuth session length) since there's no refresh flow yet — when both
// expire, the user just signs in with Google again. See #45.
const authTokenTTL = 30 * 24 * time.Hour

// handleGoogleAuth verifies a Google-issued ID token, upserts the
// corresponding user, and issues a Transfer-Scout session JWT. This is the
// only endpoint that accepts a Google ID token directly — everything else
// authenticates via the JWT this returns.
func (s *Server) handleGoogleAuth(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.IDToken == "" {
		http.Error(w, "id_token is required", http.StatusBadRequest)
		return
	}

	claims, err := s.googleVerifier.Verify(r.Context(), body.IDToken)
	if err != nil {
		http.Error(w, "invalid google id token", http.StatusUnauthorized)
		return
	}

	user, err := s.store.UpsertUser(r.Context(), claims.Sub, claims.Email, claims.Name, claims.Picture)
	if err != nil {
		http.Error(w, "failed to create or update user", http.StatusInternalServerError)
		return
	}

	token, err := auth.IssueToken(user.ID, s.authSecret, authTokenTTL)
	if err != nil {
		http.Error(w, "failed to issue token", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  user,
	})
}
