package api

import (
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
