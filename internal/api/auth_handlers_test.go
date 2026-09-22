package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/auth"
	"github.com/dleandro/transfer-scout-api/internal/models"
)

// fakeVerifier implements the api.GoogleVerifier interface for tests,
// without any network call to Google's OIDC endpoints — mirrors the
// fake-backed pattern used by fakeStore.
type fakeVerifier struct {
	claims *auth.GoogleClaims
	err    error
}

func (f *fakeVerifier) Verify(ctx context.Context, rawIDToken string) (*auth.GoogleClaims, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.claims, nil
}

func TestHandleGoogleAuth(t *testing.T) {
	claims := &auth.GoogleClaims{Sub: "google-sub-123", Email: "fan@example.com", Name: "Fan", Picture: "https://example.com/a.png"}
	user := &models.User{ID: uuid.New(), Email: "fan@example.com", DisplayName: "Fan"}

	tests := []struct {
		name     string
		body     string
		store    *fakeStore
		verifier *fakeVerifier
		wantCode int
	}{
		{
			name:     "missing id_token returns 400",
			body:     `{}`,
			store:    &fakeStore{},
			verifier: &fakeVerifier{claims: claims},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "malformed body returns 400",
			body:     `not json`,
			store:    &fakeStore{},
			verifier: &fakeVerifier{claims: claims},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "verifier error returns 401",
			body:     `{"id_token":"bad-token"}`,
			store:    &fakeStore{user: user},
			verifier: &fakeVerifier{err: errors.New("invalid signature")},
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "upsert user error returns 500",
			body:     `{"id_token":"good-token"}`,
			store:    &fakeStore{upsertUserErr: errStoreUnavailable},
			verifier: &fakeVerifier{claims: claims},
			wantCode: http.StatusInternalServerError,
		},
		{
			name:     "success returns 200",
			body:     `{"id_token":"good-token"}`,
			store:    &fakeStore{user: user},
			verifier: &fakeVerifier{claims: claims},
			wantCode: http.StatusOK,
		},
		{
			// Padding kept in an extra ignored field so the request body
			// exceeds the cap while id_token itself stays small — otherwise
			// this would just exercise the normal success path once decoded.
			name:     "oversized body returns 400",
			body:     `{"id_token":"good-token","padding":"` + strings.Repeat("a", 100*1024) + `"}`,
			store:    &fakeStore{user: user},
			verifier: &fakeVerifier{claims: claims},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := NewServer(tt.store, "test-secret", tt.verifier)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/google", strings.NewReader(tt.body))
			srv.handleGoogleAuth(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, tt.wantCode, w.Body.String())
			}

			if tt.wantCode != http.StatusOK {
				return
			}

			var got struct {
				Token string       `json:"token"`
				User  *models.User `json:"user"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if got.Token == "" {
				t.Error("token is empty, want a signed JWT")
			}
			if got.User == nil || got.User.ID != user.ID {
				t.Errorf("user = %v, want id %v", got.User, user.ID)
			}
		})
	}
}
