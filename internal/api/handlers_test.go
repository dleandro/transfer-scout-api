package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/models"
)

func TestParseIntParam(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		def     int
		want    int
		wantErr bool
	}{
		{name: "absent param falls back to default", query: "", def: 50, want: 50},
		{name: "valid integer is parsed", query: "limit=25", def: 50, want: 25},
		{name: "zero is a valid integer (range clamping is the caller's job)", query: "limit=0", def: 50, want: 0},
		{name: "negative is a valid integer (range clamping is the caller's job)", query: "limit=-5", def: 50, want: -5},
		{name: "non-integer is an error", query: "limit=abc", def: 50, wantErr: true},
		{name: "explicit empty value is treated the same as absent", query: "limit=", def: 50, want: 50},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/rumours?"+tc.query, nil)
			got, err := parseIntParam(r, "limit", tc.def)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got none (value %d)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestHandleListRumours_EmptyResultIsNilSliceNotEmptyArray(t *testing.T) {
	// Pins the documented quirk (see transfer-scout-web's CLAUDE.md): Go's
	// json.Marshal renders a nil []models.Rumour as JSON `null`, not `[]`.
	// The handler doesn't normalize this — API clients are expected to
	// handle it, same as the existing web client does.
	fs := &fakeStore{rumours: nil, hasMore: false}
	srv := NewServer(fs, "test-secret", nil)

	w := httptest.NewRecorder()
	srv.handleListRumours(w, httptest.NewRequest(http.MethodGet, "/api/v1/rumours", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Body.String(); got != `{"has_more":false,"rumours":null}`+"\n" {
		t.Errorf("body = %q, want a literal null rumours field", got)
	}
}

func TestHandleListRumours_PaginationClampingReachesTheStore(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		wantLimit  int
		wantOffset int
	}{
		{name: "defaults when absent", query: "", wantLimit: defaultRumoursLimit, wantOffset: 0},
		{name: "limit above max is clamped down", query: "limit=500", wantLimit: maxRumoursLimit, wantOffset: 0},
		{name: "limit below 1 is clamped up", query: "limit=0", wantLimit: 1, wantOffset: 0},
		{name: "negative offset is clamped to 0", query: "offset=-10", wantLimit: defaultRumoursLimit, wantOffset: 0},
		{name: "in-range values pass through unchanged", query: "limit=25&offset=10", wantLimit: 25, wantOffset: 10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := &fakeStore{}
			srv := NewServer(fs, "test-secret", nil)

			w := httptest.NewRecorder()
			srv.handleListRumours(w, httptest.NewRequest(http.MethodGet, "/api/v1/rumours?"+tc.query, nil))

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}
			if fs.gotLimit != tc.wantLimit || fs.gotOffset != tc.wantOffset {
				t.Errorf("store called with limit=%d offset=%d, want limit=%d offset=%d",
					fs.gotLimit, fs.gotOffset, tc.wantLimit, tc.wantOffset)
			}
		})
	}
}

func TestHandleListRumours_StoreErrorReturns500(t *testing.T) {
	fs := &fakeStore{listErr: errStoreUnavailable}
	srv := NewServer(fs, "test-secret", nil)

	w := httptest.NewRecorder()
	srv.handleListRumours(w, httptest.NewRequest(http.MethodGet, "/api/v1/rumours", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// withURLParam simulates chi's routing having already extracted a path
// parameter, without spinning up a full router.
func withURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestHandleGetRumour_Success(t *testing.T) {
	id := uuid.New()
	fs := &fakeStore{
		rumour: &models.Rumour{ID: id, Status: models.StatusTalks},
		events: []models.RumourEvent{{ID: uuid.New(), RumourID: id}},
	}
	srv := NewServer(fs, "test-secret", nil)

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/rumours/"+id.String(), nil), "id", id.String())
	w := httptest.NewRecorder()
	srv.handleGetRumour(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if fs.gotID != id {
		t.Errorf("store called with id=%v, want %v", fs.gotID, id)
	}
	var body struct {
		Rumour models.Rumour        `json:"rumour"`
		Events []models.RumourEvent `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Rumour.ID != id || len(body.Events) != 1 {
		t.Errorf("body = %+v, want rumour.id=%v and 1 event", body, id)
	}
}

func TestHandleGetRumour_MalformedUUIDReturns400(t *testing.T) {
	srv := NewServer(&fakeStore{}, "test-secret", nil)

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/rumours/not-a-uuid", nil), "id", "not-a-uuid")
	w := httptest.NewRecorder()
	srv.handleGetRumour(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleGetRumour_NotFoundReturns404(t *testing.T) {
	id := uuid.New()
	fs := &fakeStore{getErr: errStoreUnavailable}
	srv := NewServer(fs, "test-secret", nil)

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/rumours/"+id.String(), nil), "id", id.String())
	w := httptest.NewRecorder()
	srv.handleGetRumour(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
