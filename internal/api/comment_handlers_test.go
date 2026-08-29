package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/auth"
	"github.com/dleandro/transfer-scout-api/internal/models"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

func TestValidateCommentBody(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{name: "valid body passes through trimmed", body: "  hello  ", want: "hello"},
		{name: "empty is an error", body: "", wantErr: true},
		{name: "whitespace-only is an error", body: "   ", wantErr: true},
		{name: "exactly 2000 chars is valid", body: repeatChar("a", 2000), want: repeatChar("a", 2000)},
		{name: "2001 chars is an error", body: repeatChar("a", 2001), wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateCommentBody(tc.body)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got none (value %q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func repeatChar(s string, n int) string {
	return string(bytes.Repeat([]byte(s), n))
}

func TestHandleCreateComment_Success(t *testing.T) {
	rumourID := uuid.New()
	userID := uuid.New()
	created := &models.Comment{
		ID:       uuid.New(),
		RumourID: rumourID,
		Author:   models.Author{ID: userID, DisplayName: "Alice"},
		Body:     "great signing",
	}
	fs := &fakeStore{createdComment: created}
	srv := NewServer(fs, "test-secret", nil)

	token, err := auth.IssueToken(userID, "test-secret", time.Hour)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"body": "great signing"})
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/rumours/"+rumourID.String()+"/comments", bytes.NewReader(body)), "id", rumourID.String())
	req.Header.Set("Authorization", "Bearer "+token)

	// Route through the real middleware chain so RequireAuth actually
	// populates the context, rather than calling the handler directly.
	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleCreateComment)).ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	if fs.gotCommentUserID != userID {
		t.Errorf("store called with user id %v, want %v", fs.gotCommentUserID, userID)
	}
	if fs.gotCommentBody != "great signing" {
		t.Errorf("store called with body %q, want %q", fs.gotCommentBody, "great signing")
	}
}

func TestHandleCreateComment_NoTokenReturns401(t *testing.T) {
	rumourID := uuid.New()
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	body, _ := json.Marshal(map[string]string{"body": "hi"})
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/rumours/"+rumourID.String()+"/comments", bytes.NewReader(body)), "id", rumourID.String())

	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleCreateComment)).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleCreateComment_EmptyBodyReturns400(t *testing.T) {
	rumourID := uuid.New()
	userID := uuid.New()
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	token, _ := auth.IssueToken(userID, "test-secret", time.Hour)
	body, _ := json.Marshal(map[string]string{"body": "   "})
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/rumours/"+rumourID.String()+"/comments", bytes.NewReader(body)), "id", rumourID.String())
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleCreateComment)).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateComment_RumourNotFoundReturns404(t *testing.T) {
	rumourID := uuid.New()
	userID := uuid.New()
	fs := &fakeStore{createCommentErr: store.ErrRumourNotFound}
	srv := NewServer(fs, "test-secret", nil)

	token, _ := auth.IssueToken(userID, "test-secret", time.Hour)
	body, _ := json.Marshal(map[string]string{"body": "hello"})
	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/rumours/"+rumourID.String()+"/comments", bytes.NewReader(body)), "id", rumourID.String())
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	auth.RequireAuth("test-secret")(http.HandlerFunc(srv.handleCreateComment)).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleListComments_Success(t *testing.T) {
	rumourID := uuid.New()
	fs := &fakeStore{
		rumourExists:    true,
		comments:        []models.Comment{{ID: uuid.New(), RumourID: rumourID, Body: "hi"}},
		commentsHasMore: true,
	}
	srv := NewServer(fs, "test-secret", nil)

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/rumours/"+rumourID.String()+"/comments", nil), "id", rumourID.String())
	w := httptest.NewRecorder()
	srv.handleListComments(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if fs.gotID != rumourID {
		t.Errorf("store called with rumour id %v, want %v", fs.gotID, rumourID)
	}
}

func TestHandleListComments_RumourNotFoundReturns404(t *testing.T) {
	rumourID := uuid.New()
	fs := &fakeStore{rumourExists: false}
	srv := NewServer(fs, "test-secret", nil)

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/rumours/"+rumourID.String()+"/comments", nil), "id", rumourID.String())
	w := httptest.NewRecorder()
	srv.handleListComments(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleListComments_MalformedRumourIDReturns400(t *testing.T) {
	fs := &fakeStore{}
	srv := NewServer(fs, "test-secret", nil)

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/rumours/not-a-uuid/comments", nil), "id", "not-a-uuid")
	w := httptest.NewRecorder()
	srv.handleListComments(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
