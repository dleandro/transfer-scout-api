package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dleandro/transfer-scout-api/internal/store"
)

// TestHandleHealth_FakeStore covers both branches of handleHealth without
// any database at all — a fake whose Ping returns nil (healthy) or an error
// (degraded). The real-pool variants below stay for the case where Ping's
// actual behavior against a live/dead Postgres matters.
func TestHandleHealth_FakeStore(t *testing.T) {
	tests := []struct {
		name     string
		pingErr  error
		wantCode int
		wantBody map[string]string
	}{
		{
			name:     "reachable returns 200 ok",
			pingErr:  nil,
			wantCode: http.StatusOK,
			wantBody: map[string]string{"status": "ok"},
		},
		{
			name:     "unreachable returns 503 degraded",
			pingErr:  errStoreUnavailable,
			wantCode: http.StatusServiceUnavailable,
			wantBody: map[string]string{"status": "degraded", "db": "unreachable"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := NewServer(&fakeStore{pingErr: tt.pingErr}, "test-secret", nil)
			w := httptest.NewRecorder()
			srv.handleHealth(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantCode)
			}
			var body map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			for k, v := range tt.wantBody {
				if body[k] != v {
					t.Errorf("body[%q] = %q, want %q", k, body[k], v)
				}
			}
		})
	}
}

// TestHandleHealth_DBUnreachable_Returns503 doesn't need a real database:
// a pool pointed at an address nothing is listening on fails to ping
// without ever having connected, which is enough to exercise the
// degraded branch of handleHealth.
func TestHandleHealth_DBUnreachable_Returns503(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://u:p@127.0.0.1:1/db?connect_timeout=1")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	srv := NewServer(store.New(pool), "test-secret", nil)
	w := httptest.NewRecorder()
	srv.handleHealth(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "degraded" || body["db"] != "unreachable" {
		t.Errorf("body = %v, want status=degraded db=unreachable", body)
	}
}

// TestHandleHealth_DBReachable_Returns200 needs a real Postgres — skipped
// when DATABASE_URL is unset, same pattern as
// internal/store/integration_test.go.
func TestHandleHealth_DBReachable_Returns200(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test (needs a real Postgres)")
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	srv := NewServer(store.New(pool), "test-secret", nil)
	w := httptest.NewRecorder()
	srv.handleHealth(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf(`body["status"] = %q, want "ok"`, body["status"])
	}
}
