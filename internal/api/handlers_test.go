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
	leagueID := uuid.New()

	t.Run("league_id is parsed and passed through", func(t *testing.T) {
		fs := &fakeStore{}
		srv := NewServer(fs, "test-secret", nil)

		w := httptest.NewRecorder()
		srv.handleListRumours(w, httptest.NewRequest(http.MethodGet, "/api/v1/rumours?league_id="+leagueID.String(), nil))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if fs.gotFilter.LeagueID == nil || *fs.gotFilter.LeagueID != leagueID {
			t.Errorf("gotFilter.LeagueID = %v, want %v", fs.gotFilter.LeagueID, leagueID)
		}
	})

	t.Run("malformed league_id returns 400", func(t *testing.T) {
		srv := NewServer(&fakeStore{}, "test-secret", nil)
		w := httptest.NewRecorder()
		srv.handleListRumours(w, httptest.NewRequest(http.MethodGet, "/api/v1/rumours?league_id=not-a-uuid", nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

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
		if fs.gotFilter.ClubID != nil || fs.gotFilter.PlayerID != nil || fs.gotFilter.LeagueID != nil {
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

	t.Run("following=true reaches the store filter", func(t *testing.T) {
		fs := &fakeStore{}
		srv := NewServer(fs, "test-secret", nil)

		w := httptest.NewRecorder()
		srv.handleListRumours(w, httptest.NewRequest(http.MethodGet, "/api/v1/rumours?following=true", nil))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if !fs.gotFilter.Following {
			t.Error("gotFilter.Following = false, want true")
		}
	})

	t.Run("absent following stays false (no regression)", func(t *testing.T) {
		fs := &fakeStore{}
		srv := NewServer(fs, "test-secret", nil)

		w := httptest.NewRecorder()
		srv.handleListRumours(w, httptest.NewRequest(http.MethodGet, "/api/v1/rumours", nil))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if fs.gotFilter.Following {
			t.Error("gotFilter.Following = true, want false when the query param is absent")
		}
	})
}

func TestHandleListClubs(t *testing.T) {
	t.Run("returns clubs from the store, including followed_by_me", func(t *testing.T) {
		fs := &fakeStore{clubs: []store.ClubFeedItem{
			{Club: models.Club{ID: uuid.New(), Name: "Arsenal"}, FollowedByMe: true},
		}}
		srv := NewServer(fs, "test-secret", nil)

		w := httptest.NewRecorder()
		srv.handleListClubs(w, httptest.NewRequest(http.MethodGet, "/api/v1/clubs", nil))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		var body struct {
			Clubs []store.ClubFeedItem `json:"clubs"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(body.Clubs) != 1 || body.Clubs[0].Name != "Arsenal" || !body.Clubs[0].FollowedByMe {
			t.Errorf("body = %+v, want one club named Arsenal with followed_by_me=true", body)
		}
	})

	t.Run("anonymous caller passes a nil viewerID through to the store", func(t *testing.T) {
		fs := &fakeStore{}
		srv := NewServer(fs, "test-secret", nil)

		w := httptest.NewRecorder()
		srv.handleListClubs(w, httptest.NewRequest(http.MethodGet, "/api/v1/clubs", nil))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if fs.gotClubsViewerID != nil {
			t.Errorf("gotClubsViewerID = %v, want nil for an anonymous caller", fs.gotClubsViewerID)
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

func TestHandleListLeagues(t *testing.T) {
	t.Run("returns leagues from the store", func(t *testing.T) {
		fs := &fakeStore{leagues: []models.League{{ID: uuid.New(), Name: "Premier League"}}}
		srv := NewServer(fs, "test-secret", nil)

		w := httptest.NewRecorder()
		srv.handleListLeagues(w, httptest.NewRequest(http.MethodGet, "/api/v1/leagues", nil))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		var body struct {
			Leagues []models.League `json:"leagues"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(body.Leagues) != 1 || body.Leagues[0].Name != "Premier League" {
			t.Errorf("body = %+v, want one league named Premier League", body)
		}
	})

	t.Run("store error returns 500", func(t *testing.T) {
		srv := NewServer(&fakeStore{leaguesErr: errStoreUnavailable}, "test-secret", nil)
		w := httptest.NewRecorder()
		srv.handleListLeagues(w, httptest.NewRequest(http.MethodGet, "/api/v1/leagues", nil))
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}

func TestHandleListClubs_LeagueIDFilter(t *testing.T) {
	leagueID := uuid.New()

	t.Run("league_id is parsed and passed through", func(t *testing.T) {
		fs := &fakeStore{}
		srv := NewServer(fs, "test-secret", nil)

		w := httptest.NewRecorder()
		srv.handleListClubs(w, httptest.NewRequest(http.MethodGet, "/api/v1/clubs?league_id="+leagueID.String(), nil))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if fs.gotClubsLeagueID == nil || *fs.gotClubsLeagueID != leagueID {
			t.Errorf("gotClubsLeagueID = %v, want %v", fs.gotClubsLeagueID, leagueID)
		}
	})

	t.Run("absent league_id stays nil (no regression)", func(t *testing.T) {
		fs := &fakeStore{}
		srv := NewServer(fs, "test-secret", nil)

		w := httptest.NewRecorder()
		srv.handleListClubs(w, httptest.NewRequest(http.MethodGet, "/api/v1/clubs", nil))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		if fs.gotClubsLeagueID != nil {
			t.Errorf("gotClubsLeagueID = %v, want nil", fs.gotClubsLeagueID)
		}
	})

	t.Run("malformed league_id returns 400", func(t *testing.T) {
		srv := NewServer(&fakeStore{}, "test-secret", nil)
		w := httptest.NewRecorder()
		srv.handleListClubs(w, httptest.NewRequest(http.MethodGet, "/api/v1/clubs?league_id=not-a-uuid", nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
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

func TestHandleListRumours_NonIntegerPaginationReturns400(t *testing.T) {
	// The handler's own error branch (not just parseIntParam in isolation):
	// a present-but-unparseable limit/offset is a client error, so the
	// store is never consulted.
	cases := []struct {
		name  string
		query string
	}{
		{name: "non-integer limit", query: "limit=abc"},
		{name: "non-integer offset", query: "offset=xyz"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := &fakeStore{}
			srv := NewServer(fs, "test-secret", nil)

			w := httptest.NewRecorder()
			srv.handleListRumours(w, httptest.NewRequest(http.MethodGet, "/api/v1/rumours?"+tc.query, nil))

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
			if fs.gotLimit != 0 || fs.gotOffset != 0 {
				t.Errorf("store was consulted (limit=%d offset=%d), want no call on a bad request",
					fs.gotLimit, fs.gotOffset)
			}
		})
	}
}

func TestHandleListRumours_PopulatedListRendersViewShape(t *testing.T) {
	// The empty-array test covers the nil-slice path; this covers the
	// handler actually mapping each item through newRumourView and
	// reporting has_more from the store.
	fromClubID := uuid.New()
	fromClubName := "Selling FC"
	fs := &fakeStore{
		hasMore: true,
		rumours: []store.RumourFeedItem{{
			Rumour:       models.Rumour{ID: uuid.New(), Status: models.StatusTalks, TransferWindow: "summer-2026", FromClubID: &fromClubID},
			PlayerName:   "Test Player",
			ToClubName:   "Buying FC",
			FromClubName: &fromClubName,
		}},
	}
	srv := NewServer(fs, "test-secret", nil)

	w := httptest.NewRecorder()
	srv.handleListRumours(w, httptest.NewRequest(http.MethodGet, "/api/v1/rumours", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body struct {
		HasMore bool `json:"has_more"`
		Rumours []struct {
			Player   struct{ Name string } `json:"player"`
			ToClub   struct{ Name string } `json:"to_club"`
			FromClub *struct {
				Name string `json:"name"`
			} `json:"from_club"`
		} `json:"rumours"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !body.HasMore {
		t.Errorf("has_more = false, want true (passed through from the store)")
	}
	if len(body.Rumours) != 1 {
		t.Fatalf("got %d rumours, want 1", len(body.Rumours))
	}
	r := body.Rumours[0]
	if r.Player.Name != "Test Player" || r.ToClub.Name != "Buying FC" {
		t.Errorf("view = %+v, want player/to_club names mapped through newRumourView", r)
	}
	if r.FromClub == nil || r.FromClub.Name != "Selling FC" {
		t.Errorf("from_club = %+v, want the optional from-club mapped", r.FromClub)
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
