package api

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/models"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

func TestNewRumourView_OmitsFromClubWhenAbsent(t *testing.T) {
	item := store.RumourFeedItem{
		Rumour: models.Rumour{
			ID:       uuid.New(),
			PlayerID: uuid.New(),
			ToClubID: uuid.New(),
			Status:   models.StatusRumoured,
		},
		PlayerName: "Test Player",
		ToClubName: "Test Club",
	}

	v := newRumourView(item)

	if v.FromClub != nil {
		t.Errorf("expected FromClub to be nil when FromClubID is absent, got %+v", v.FromClub)
	}
	if v.Player.Name != "Test Player" || v.ToClub.Name != "Test Club" {
		t.Errorf("player/club names not carried through: %+v", v)
	}
}

func TestNewRumourView_PopulatesFromClubWhenPresent(t *testing.T) {
	fromClubID := uuid.New()
	fromClubName := "Origin Club"
	crest := "https://example.com/crest.png"

	item := store.RumourFeedItem{
		Rumour: models.Rumour{
			ID:         uuid.New(),
			PlayerID:   uuid.New(),
			ToClubID:   uuid.New(),
			FromClubID: &fromClubID,
			Status:     models.StatusTalks,
		},
		PlayerName:    "Test Player",
		ToClubName:    "Test Club",
		FromClubName:  &fromClubName,
		FromClubCrest: &crest,
	}

	v := newRumourView(item)

	if v.FromClub == nil {
		t.Fatal("expected FromClub to be populated when FromClubID is present")
	}
	if v.FromClub.ID != fromClubID || v.FromClub.Name != fromClubName || v.FromClub.CrestURL == nil || *v.FromClub.CrestURL != crest {
		t.Errorf("unexpected from_club view: %+v", v.FromClub)
	}
}

func TestNewRumourView_ExposesToClubCrest(t *testing.T) {
	crest := "https://upload.wikimedia.org/wikipedia/en/5/53/Arsenal_FC.svg"

	item := store.RumourFeedItem{
		Rumour: models.Rumour{
			ID:       uuid.New(),
			PlayerID: uuid.New(),
			ToClubID: uuid.New(),
			Status:   models.StatusRumoured,
		},
		PlayerName:  "Test Player",
		ToClubName:  "Arsenal",
		ToClubCrest: &crest,
	}

	v := newRumourView(item)

	if v.ToClub.CrestURL == nil || *v.ToClub.CrestURL != crest {
		t.Errorf("expected to_club crest_url %q to be exposed, got %+v", crest, v.ToClub.CrestURL)
	}
}

func TestNewRumourView_OmitsToClubCrestWhenAbsent(t *testing.T) {
	item := store.RumourFeedItem{
		Rumour: models.Rumour{
			ID:       uuid.New(),
			PlayerID: uuid.New(),
			ToClubID: uuid.New(),
			Status:   models.StatusRumoured,
		},
		PlayerName: "Test Player",
		ToClubName: "Unmapped Club",
	}

	v := newRumourView(item)

	if v.ToClub.CrestURL != nil {
		t.Errorf("expected nil crest_url for an unmapped club, got %q", *v.ToClub.CrestURL)
	}
}

func TestNewRumourView_CarriesLikeCountAndLikedByMe(t *testing.T) {
	item := store.RumourFeedItem{
		Rumour: models.Rumour{
			ID:       uuid.New(),
			PlayerID: uuid.New(),
			ToClubID: uuid.New(),
			Status:   models.StatusRumoured,
		},
		PlayerName: "Test Player",
		ToClubName: "Test Club",
		LikeCount:  3,
		LikedByMe:  true,
	}

	v := newRumourView(item)

	if v.LikeCount != 3 {
		t.Errorf("LikeCount = %d, want 3", v.LikeCount)
	}
	if !v.LikedByMe {
		t.Error("LikedByMe = false, want true")
	}
}

func TestNewRumourView_LikeCountAndLikedByMeAreUnconditionalInJSON(t *testing.T) {
	item := store.RumourFeedItem{
		Rumour: models.Rumour{
			ID:       uuid.New(),
			PlayerID: uuid.New(),
			ToClubID: uuid.New(),
			Status:   models.StatusRumoured,
		},
		PlayerName: "Test Player",
		ToClubName: "Test Club",
	}

	body, err := json.Marshal(newRumourView(item))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := decoded["like_count"]; !ok {
		t.Error("expected like_count present in JSON even at zero value (unconditional, public)")
	}
	if _, ok := decoded["liked_by_me"]; !ok {
		t.Error("expected liked_by_me present in JSON even at false (unconditional)")
	}
}

func TestNewRumourEventView_CarriesSourceAndArticle(t *testing.T) {
	ev := store.RumourEventItem{
		RumourEvent: models.RumourEvent{
			ID:       uuid.New(),
			SourceID: uuid.New(),
			Status:   models.StatusAdvanced,
		},
		SourceName:   "Test Source",
		ArticleURL:   "https://example.com/article",
		ArticleTitle: "Test Article Title",
	}

	v := newRumourEventView(ev)

	if v.Source.Name != "Test Source" {
		t.Errorf("unexpected source name: %s", v.Source.Name)
	}
	if v.Article.URL != "https://example.com/article" || v.Article.Title != "Test Article Title" {
		t.Errorf("unexpected article view: %+v", v.Article)
	}
	if v.Status != models.StatusAdvanced {
		t.Errorf("unexpected status: %s", v.Status)
	}
}
