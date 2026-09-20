package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/auth"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

func TestHandleLikeRumour_Success(t *testing.T) {
	rumourID := uuid.New()
	userID := uuid.New()
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	token, err := auth.IssueToken(userID, "test-secret", time.Hour)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/rumours/"+rumourID.String()+"/like", nil), "id", rumourID.String())
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleLikeRumour)).ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusNoContent, w.Body.String())
	}
	if fs.gotLikeRumourID != rumourID || fs.gotLikeUserID != userID {
		t.Errorf("store called with rumour=%v user=%v, want rumour=%v user=%v",
			fs.gotLikeRumourID, fs.gotLikeUserID, rumourID, userID)
	}
}

func TestHandleLikeRumour_IsIdempotent(t *testing.T) {
	rumourID := uuid.New()
	userID := uuid.New()
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	token, _ := auth.IssueToken(userID, "test-secret", time.Hour)

	for i := 0; i < 2; i++ {
		req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/rumours/"+rumourID.String()+"/like", nil), "id", rumourID.String())
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleLikeRumour)).ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("like #%d: status = %d, want %d", i+1, w.Code, http.StatusNoContent)
		}
	}
}

func TestHandleLikeRumour_NoTokenReturns401(t *testing.T) {
	rumourID := uuid.New()
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/rumours/"+rumourID.String()+"/like", nil), "id", rumourID.String())
	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleLikeRumour)).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleLikeRumour_UnknownRumourReturns404(t *testing.T) {
	rumourID := uuid.New()
	userID := uuid.New()
	fs := &fakeStore{likeRumourErr: store.ErrRumourNotFound}
	srv := NewServer(fs, "test-secret", nil)

	token, _ := auth.IssueToken(userID, "test-secret", time.Hour)
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/rumours/"+rumourID.String()+"/like", nil), "id", rumourID.String())
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleLikeRumour)).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleLikeRumour_MalformedRumourIDReturns400(t *testing.T) {
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/rumours/not-a-uuid/like", nil), "id", "not-a-uuid")
	w := httptest.NewRecorder()
	srv.handleLikeRumour(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleUnlikeRumour_Success(t *testing.T) {
	rumourID := uuid.New()
	userID := uuid.New()
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	token, err := auth.IssueToken(userID, "test-secret", time.Hour)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/rumours/"+rumourID.String()+"/like", nil), "id", rumourID.String())
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleUnlikeRumour)).ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusNoContent, w.Body.String())
	}
	if fs.gotUnlikeRumourID != rumourID || fs.gotUnlikeUserID != userID {
		t.Errorf("store called with rumour=%v user=%v, want rumour=%v user=%v",
			fs.gotUnlikeRumourID, fs.gotUnlikeUserID, rumourID, userID)
	}
}

func TestHandleUnlikeRumour_NeverLikedIsStillNoContent(t *testing.T) {
	// UnlikeRumour is idempotent at the store layer (see
	// internal/store/likes.go) — the handler doesn't add its own
	// "was it liked" branch on top, so unliking something never liked
	// still returns 204, not an error.
	rumourID := uuid.New()
	userID := uuid.New()
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	token, _ := auth.IssueToken(userID, "test-secret", time.Hour)
	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/rumours/"+rumourID.String()+"/like", nil), "id", rumourID.String())
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleUnlikeRumour)).ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestHandleUnlikeRumour_NoTokenReturns401(t *testing.T) {
	rumourID := uuid.New()
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/rumours/"+rumourID.String()+"/like", nil), "id", rumourID.String())
	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleUnlikeRumour)).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleUnlikeRumour_UnknownRumourReturns404(t *testing.T) {
	rumourID := uuid.New()
	userID := uuid.New()
	fs := &fakeStore{unlikeRumourErr: store.ErrRumourNotFound}
	srv := NewServer(fs, "test-secret", nil)

	token, _ := auth.IssueToken(userID, "test-secret", time.Hour)
	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/rumours/"+rumourID.String()+"/like", nil), "id", rumourID.String())
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleUnlikeRumour)).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleUnlikeRumour_MalformedRumourIDReturns400(t *testing.T) {
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/rumours/not-a-uuid/like", nil), "id", "not-a-uuid")
	w := httptest.NewRecorder()
	srv.handleUnlikeRumour(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
