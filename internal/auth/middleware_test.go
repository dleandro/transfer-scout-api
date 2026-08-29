package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func handlerRecordingContextUserID(t *testing.T, got *uuid.UUID) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := UserIDFromContext(r.Context())
		if !ok {
			t.Error("expected a user ID in context, found none")
		}
		*got = userID
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequireAuth_MissingHeaderReturns401(t *testing.T) {
	var got uuid.UUID
	h := RequireAuth("test-secret")(handlerRecordingContextUserID(t, &got))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_MalformedHeaderReturns401(t *testing.T) {
	var got uuid.UUID
	h := RequireAuth("test-secret")(handlerRecordingContextUserID(t, &got))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "not-bearer-format")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_ExpiredTokenReturns401(t *testing.T) {
	token, err := IssueToken(uuid.New(), "test-secret", -time.Hour)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	var got uuid.UUID
	h := RequireAuth("test-secret")(handlerRecordingContextUserID(t, &got))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_ValidTokenPopulatesContextAndCallsNext(t *testing.T) {
	userID := uuid.New()
	token, err := IssueToken(userID, "test-secret", time.Hour)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	var got uuid.UUID
	h := RequireAuth("test-secret")(handlerRecordingContextUserID(t, &got))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got != userID {
		t.Errorf("context user ID = %v, want %v", got, userID)
	}
}

func TestKeyByUserID_ReturnsUserIDFromContext(t *testing.T) {
	userID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	// Simulate RequireAuth having already run by using its own context key
	// indirectly: run RequireAuth with a valid token, capture the request
	// it hands to next, and feed that into KeyByUserID.
	token, err := IssueToken(userID, "test-secret", time.Hour)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	var capturedReq *http.Request
	h := RequireAuth("test-secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
	}))
	h.ServeHTTP(httptest.NewRecorder(), req)

	key, err := KeyByUserID(capturedReq)
	if err != nil {
		t.Fatalf("KeyByUserID: %v", err)
	}
	if key != userID.String() {
		t.Errorf("key = %q, want %q", key, userID.String())
	}
}

func TestKeyByUserID_NoUserInContextReturnsError(t *testing.T) {
	if _, err := KeyByUserID(httptest.NewRequest(http.MethodGet, "/", nil)); err == nil {
		t.Fatal("expected an error when no user ID is in context, got none")
	}
}
