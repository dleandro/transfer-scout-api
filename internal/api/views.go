package api

import (
	"time"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/models"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

type playerView struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type clubView struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	CrestURL *string   `json:"crest_url,omitempty"`
}

type rumourView struct {
	ID             uuid.UUID           `json:"id"`
	Player         playerView          `json:"player"`
	FromClub       *clubView           `json:"from_club,omitempty"`
	ToClub         clubView            `json:"to_club"`
	TransferWindow string              `json:"transfer_window"`
	Status         models.RumourStatus `json:"status"`
	FeeMinEUR      *float64            `json:"fee_min_eur,omitempty"`
	FeeMaxEUR      *float64            `json:"fee_max_eur,omitempty"`
	Summary        *string             `json:"summary,omitempty"`
	Confidence     *float64            `json:"confidence,omitempty"`
	// Credibility is the average reliability_score (0-100) across this
	// rumour's contributing sources — see store.RumourFeedItem.
	Credibility *float64  `json:"credibility,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func newRumourView(item store.RumourFeedItem) rumourView {
	v := rumourView{
		ID:             item.ID,
		Player:         playerView{ID: item.PlayerID, Name: item.PlayerName},
		ToClub:         clubView{ID: item.ToClubID, Name: item.ToClubName, CrestURL: item.ToClubCrest},
		TransferWindow: item.TransferWindow,
		Status:         item.Status,
		FeeMinEUR:      item.FeeMinEUR,
		FeeMaxEUR:      item.FeeMaxEUR,
		Summary:        item.Summary,
		Confidence:     item.Confidence,
		Credibility:    item.Credibility,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
	if item.FromClubID != nil {
		name := ""
		if item.FromClubName != nil {
			name = *item.FromClubName
		}
		v.FromClub = &clubView{ID: *item.FromClubID, Name: name, CrestURL: item.FromClubCrest}
	}
	return v
}

type sourceView struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type articleView struct {
	ID    uuid.UUID `json:"id"`
	URL   string    `json:"url"`
	Title string    `json:"title"`
}

type rumourEventView struct {
	ID         uuid.UUID           `json:"id"`
	Status     models.RumourStatus `json:"status"`
	FeeMinEUR  *float64            `json:"fee_min_eur,omitempty"`
	FeeMaxEUR  *float64            `json:"fee_max_eur,omitempty"`
	Summary    *string             `json:"summary,omitempty"`
	Confidence *float64            `json:"confidence,omitempty"`
	CreatedAt  time.Time           `json:"created_at"`
	Source     sourceView          `json:"source"`
	Article    articleView         `json:"article"`
}

func newRumourEventView(ev store.RumourEventItem) rumourEventView {
	return rumourEventView{
		ID:         ev.ID,
		Status:     ev.Status,
		FeeMinEUR:  ev.FeeMinEUR,
		FeeMaxEUR:  ev.FeeMaxEUR,
		Summary:    ev.Summary,
		Confidence: ev.Confidence,
		CreatedAt:  ev.CreatedAt,
		Source:     sourceView{ID: ev.SourceID, Name: ev.SourceName},
		Article:    articleView{ID: ev.ArticleID, URL: ev.ArticleURL, Title: ev.ArticleTitle},
	}
}

// rumourDetailView is a rumourView plus its full event timeline.
type rumourDetailView struct {
	rumourView
	Events []rumourEventView `json:"events"`
}
