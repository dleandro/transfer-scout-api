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

func TestHandleFollowClub_Success(t *testing.T) {
	clubID := uuid.New()
	userID := uuid.New()
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	token, err := auth.IssueToken(userID, "test-secret", time.Hour)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/clubs/"+clubID.String()+"/follow", nil), "id", clubID.String())
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleFollowClub)).ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusNoContent, w.Body.String())
	}
	if fs.gotFollowClubID != clubID || fs.gotFollowUserID != userID {
		t.Errorf("store called with club=%v user=%v, want club=%v user=%v",
			fs.gotFollowClubID, fs.gotFollowUserID, clubID, userID)
	}
}

func TestHandleFollowClub_IsIdempotent(t *testing.T) {
	clubID := uuid.New()
	userID := uuid.New()
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	token, _ := auth.IssueToken(userID, "test-secret", time.Hour)

	for i := 0; i < 2; i++ {
		req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/clubs/"+clubID.String()+"/follow", nil), "id", clubID.String())
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleFollowClub)).ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("follow #%d: status = %d, want %d", i+1, w.Code, http.StatusNoContent)
		}
	}
}

func TestHandleFollowClub_NoTokenReturns401(t *testing.T) {
	clubID := uuid.New()
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/clubs/"+clubID.String()+"/follow", nil), "id", clubID.String())
	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleFollowClub)).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleFollowClub_UnknownClubReturns404(t *testing.T) {
	clubID := uuid.New()
	userID := uuid.New()
	fs := &fakeStore{followClubErr: store.ErrClubNotFound}
	srv := NewServer(fs, "test-secret", nil)

	token, _ := auth.IssueToken(userID, "test-secret", time.Hour)
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/clubs/"+clubID.String()+"/follow", nil), "id", clubID.String())
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleFollowClub)).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleFollowClub_MalformedClubIDReturns400(t *testing.T) {
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/clubs/not-a-uuid/follow", nil), "id", "not-a-uuid")
	w := httptest.NewRecorder()
	srv.handleFollowClub(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleUnfollowClub_Success(t *testing.T) {
	clubID := uuid.New()
	userID := uuid.New()
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	token, err := auth.IssueToken(userID, "test-secret", time.Hour)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/clubs/"+clubID.String()+"/follow", nil), "id", clubID.String())
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleUnfollowClub)).ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusNoContent, w.Body.String())
	}
	if fs.gotUnfollowClubID != clubID || fs.gotUnfollowUserID != userID {
		t.Errorf("store called with club=%v user=%v, want club=%v user=%v",
			fs.gotUnfollowClubID, fs.gotUnfollowUserID, clubID, userID)
	}
}

func TestHandleUnfollowClub_NeverFollowedIsStillNoContent(t *testing.T) {
	// UnfollowClub is idempotent at the store layer (see
	// internal/store/follows.go) — the handler doesn't add its own
	// "was it followed" branch on top, so unfollowing something never
	// followed still returns 204, not an error.
	clubID := uuid.New()
	userID := uuid.New()
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	token, _ := auth.IssueToken(userID, "test-secret", time.Hour)
	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/clubs/"+clubID.String()+"/follow", nil), "id", clubID.String())
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleUnfollowClub)).ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestHandleUnfollowClub_NoTokenReturns401(t *testing.T) {
	clubID := uuid.New()
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/clubs/"+clubID.String()+"/follow", nil), "id", clubID.String())
	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleUnfollowClub)).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleUnfollowClub_UnknownClubReturns404(t *testing.T) {
	clubID := uuid.New()
	userID := uuid.New()
	fs := &fakeStore{unfollowClubErr: store.ErrClubNotFound}
	srv := NewServer(fs, "test-secret", nil)

	token, _ := auth.IssueToken(userID, "test-secret", time.Hour)
	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/clubs/"+clubID.String()+"/follow", nil), "id", clubID.String())
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleUnfollowClub)).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleUnfollowClub_MalformedClubIDReturns400(t *testing.T) {
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/clubs/not-a-uuid/follow", nil), "id", "not-a-uuid")
	w := httptest.NewRecorder()
	srv.handleUnfollowClub(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
