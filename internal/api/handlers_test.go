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
	"github.com/dleandro/transfer-scout-api/internal/store"
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

func TestParseUUIDParam(t *testing.T) {
	validID := uuid.New()

	t.Run("absent param returns nil, nil", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/rumours", nil)
		got, err := parseUUIDParam(r, "club_id")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("invalid UUID is an error", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/rumours?club_id=not-a-uuid", nil)
		if _, err := parseUUIDParam(r, "club_id"); err == nil {
			t.Fatal("expected an error, got none")
		}
	})

	t.Run("valid UUID is parsed", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/rumours?club_id="+validID.String(), nil)
		got, err := parseUUIDParam(r, "club_id")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || *got != validID {
			t.Errorf("got %v, want %v", got, validID)
		}
	})
}

func TestHandleListRumours_FilterParamsReachTheStore(t *testing.T) {
	clubID := uuid.New()
	playerID := uuid.New()

	t.Run("club_id and player_id are parsed and passed through", func(t *testing.T) {
		fs := &fakeStore{}
		srv := NewServer(fs, "test-secret", nil)

		w := httptest.NewRecorder()
		srv.handleListRumours(w, httptest.NewRequest(http.MethodGet,
			"/api/v1/rumours?club_id="+clubID.String()+"&player_id="+playerID.String(), nil))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if fs.gotFilter.ClubID == nil || *fs.gotFilter.ClubID != clubID {
			t.Errorf("gotFilter.ClubID = %v, want %v", fs.gotFilter.ClubID, clubID)
		}
		if fs.gotFilter.PlayerID == nil || *fs.gotFilter.PlayerID != playerID {
			t.Errorf("gotFilter.PlayerID = %v, want %v", fs.gotFilter.PlayerID, playerID)
		}
	})

	t.Run("absent filter params stay nil (no regression)", func(t *testing.T) {
		fs := &fakeStore{}
		srv := NewServer(fs, "test-secret", nil)

		w := httptest.NewRecorder()
		srv.handleListRumours(w, httptest.NewRequest(http.MethodGet, "/api/v1/rumours", nil))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if fs.gotFilter.ClubID != nil || fs.gotFilter.PlayerID != nil {
			t.Errorf("gotFilter = %+v, want both nil", fs.gotFilter)
		}
	})

	t.Run("malformed club_id returns 400", func(t *testing.T) {
		srv := NewServer(&fakeStore{}, "test-secret", nil)
		w := httptest.NewRecorder()
		srv.handleListRumours(w, httptest.NewRequest(http.MethodGet, "/api/v1/rumours?club_id=not-a-uuid", nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("malformed player_id returns 400", func(t *testing.T) {
		srv := NewServer(&fakeStore{}, "test-secret", nil)
		w := httptest.NewRecorder()
		srv.handleListRumours(w, httptest.NewRequest(http.MethodGet, "/api/v1/rumours?player_id=not-a-uuid", nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})
}

func TestHandleListClubs(t *testing.T) {
	t.Run("returns clubs from the store", func(t *testing.T) {
		fs := &fakeStore{clubs: []models.Club{{ID: uuid.New(), Name: "Arsenal"}}}
		srv := NewServer(fs, "test-secret", nil)

		w := httptest.NewRecorder()
		srv.handleListClubs(w, httptest.NewRequest(http.MethodGet, "/api/v1/clubs", nil))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		var body struct {
			Clubs []models.Club `json:"clubs"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(body.Clubs) != 1 || body.Clubs[0].Name != "Arsenal" {
			t.Errorf("body = %+v, want one club named Arsenal", body)
		}
	})

	t.Run("store error returns 500", func(t *testing.T) {
		srv := NewServer(&fakeStore{clubsErr: errStoreUnavailable}, "test-secret", nil)
		w := httptest.NewRecorder()
		srv.handleListClubs(w, httptest.NewRequest(http.MethodGet, "/api/v1/clubs", nil))
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

func TestHandleListPlayers(t *testing.T) {
	t.Run("returns players from the store", func(t *testing.T) {
		fs := &fakeStore{players: []models.Player{{ID: uuid.New(), Name: "Test Player"}}}
		srv := NewServer(fs, "test-secret", nil)

		w := httptest.NewRecorder()
		srv.handleListPlayers(w, httptest.NewRequest(http.MethodGet, "/api/v1/players", nil))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		var body struct {
			Players []models.Player `json:"players"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(body.Players) != 1 || body.Players[0].Name != "Test Player" {
			t.Errorf("body = %+v, want one player named Test Player", body)
		}
	})

	t.Run("store error returns 500", func(t *testing.T) {
		srv := NewServer(&fakeStore{playersErr: errStoreUnavailable}, "test-secret", nil)
		w := httptest.NewRecorder()
		srv.handleListPlayers(w, httptest.NewRequest(http.MethodGet, "/api/v1/players", nil))
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

func TestHandleListRumours_EmptyResultIsEmptyArrayNotNull(t *testing.T) {
	// The handler builds views via make([]rumourView, len(items)), which is
	// a non-nil empty slice even when items is nil — so an empty page
	// renders as `[]`, not `null` (unlike a bare nil-slice marshal).
	fs := &fakeStore{rumours: nil, hasMore: false}
	srv := NewServer(fs, "test-secret", nil)

	w := httptest.NewRecorder()
	srv.handleListRumours(w, httptest.NewRequest(http.MethodGet, "/api/v1/rumours", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Body.String(); got != `{"has_more":false,"rumours":[]}`+"\n" {
		t.Errorf("body = %q, want an empty rumours array", got)
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
		rumour: &store.RumourFeedItem{
			Rumour:     models.Rumour{ID: id, Status: models.StatusTalks},
			PlayerName: "Test Player",
			ToClubName: "Test Club",
		},
		events: []store.RumourEventItem{{
			RumourEvent:  models.RumourEvent{ID: uuid.New(), RumourID: id},
			SourceName:   "Test Source",
			ArticleURL:   "https://example.com/article",
			ArticleTitle: "Test Article",
		}},
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
	// rumourDetailView embeds rumourView (id, player, to_club, ... are
	// top-level fields), plus a sibling "events" array.
	var body struct {
		ID     uuid.UUID `json:"id"`
		Events []struct {
			ID uuid.UUID `json:"id"`
		} `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.ID != id || len(body.Events) != 1 {
		t.Errorf("body = %+v, want id=%v and 1 event", body, id)
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
