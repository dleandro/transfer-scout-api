package api

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/models"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

func getClubsThroughRouter(t *testing.T, srv *Server, acceptEncoding string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/clubs", nil)
	if acceptEncoding != "" {
		r.Header.Set("Accept-Encoding", acceptEncoding)
	}
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	return w
}

func TestRouter_ClubsResponseCompression(t *testing.T) {
	fs := &fakeStore{clubs: []store.ClubFeedItem{
		{Club: models.Club{ID: uuid.New(), Name: "Arsenal"}, FollowedByMe: true},
		{Club: models.Club{ID: uuid.New(), Name: "Benfica"}},
	}}
	srv := NewServer(fs, "test-secret", nil)

	identity := getClubsThroughRouter(t, srv, "")
	if got := identity.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("without Accept-Encoding: Content-Encoding = %q, want none", got)
	}
	if !json.Valid(identity.Body.Bytes()) {
		t.Fatalf("identity body is not valid JSON: %q", identity.Body.String())
	}

	compressed := getClubsThroughRouter(t, srv, "gzip")
	if got := compressed.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("with Accept-Encoding gzip: Content-Encoding = %q, want gzip", got)
	}
	if got := compressed.Header().Get("Content-Length"); got != "" {
		t.Errorf("gzip response Content-Length = %q, want none", got)
	}
	zr, err := gzip.NewReader(compressed.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	decompressed, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if !json.Valid(decompressed) {
		t.Fatalf("decompressed body is not valid JSON: %q", decompressed)
	}
	if !bytes.Equal(decompressed, identity.Body.Bytes()) {
		t.Errorf("decompressed body = %q, want %q", decompressed, identity.Body.Bytes())
	}
}

func TestRouter_HealthIsNotCompressed(t *testing.T) {
	srv := NewServer(&fakeStore{}, "test-secret", nil)
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, r)
	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("/health Content-Encoding = %q, want none", got)
	}
}
