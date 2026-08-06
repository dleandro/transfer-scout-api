// Package models defines the core Transfer Scout domain entities, shared by
// the API, ingest, and extract binaries.
package models

import (
	"time"

	"github.com/google/uuid"
)

// RumourStatus mirrors the Postgres rumour_status enum. Transitions are
// roughly one-way: Rumoured -> Talks -> Advanced -> Medical -> Confirmed | Collapsed.
type RumourStatus string

const (
	StatusRumoured  RumourStatus = "rumoured"
	StatusTalks     RumourStatus = "talks"
	StatusAdvanced  RumourStatus = "advanced"
	StatusMedical   RumourStatus = "medical"
	StatusConfirmed RumourStatus = "confirmed"
	StatusCollapsed RumourStatus = "collapsed"
)

// IsTerminal reports whether the status is one that resolves a rumour
// (and, later, any game predictions attached to it).
func (s RumourStatus) IsTerminal() bool {
	return s == StatusConfirmed || s == StatusCollapsed
}

// statusRank orders non-terminal statuses so callers can tell whether an
// incoming status represents forward progress.
var statusRank = map[RumourStatus]int{
	StatusRumoured: 0,
	StatusTalks:    1,
	StatusAdvanced: 2,
	StatusMedical:  3,
}

// IsForwardTransition reports whether moving from s to next is forward
// progress (or a jump straight to a terminal state), rather than a
// regression that callers should ignore.
func (s RumourStatus) IsForwardTransition(next RumourStatus) bool {
	if s.IsTerminal() {
		return false
	}
	if next.IsTerminal() {
		return true
	}
	nextRank, ok := statusRank[next]
	if !ok {
		return false
	}
	curRank, ok := statusRank[s]
	if !ok {
		return false
	}
	return nextRank >= curRank
}

type Club struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	ShortName *string   `json:"short_name,omitempty"`
	CrestURL  *string   `json:"crest_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Player struct {
	ID            uuid.UUID  `json:"id"`
	Name          string     `json:"name"`
	CurrentClubID *uuid.UUID `json:"current_club_id,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type Source struct {
	ID               uuid.UUID `json:"id"`
	Name             string    `json:"name"`
	FeedURL          *string   `json:"feed_url,omitempty"`
	ReliabilityScore float64   `json:"reliability_score"`
	CreatedAt        time.Time `json:"created_at"`
}

type Article struct {
	ID          uuid.UUID  `json:"id"`
	SourceID    uuid.UUID  `json:"source_id"`
	URL         string     `json:"url"`
	Title       string     `json:"title"`
	Content     *string    `json:"content,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	FetchedAt   time.Time  `json:"fetched_at"`
	Processed   bool       `json:"processed"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Rumour struct {
	ID             uuid.UUID    `json:"id"`
	PlayerID       uuid.UUID    `json:"player_id"`
	FromClubID     *uuid.UUID   `json:"from_club_id,omitempty"`
	ToClubID       uuid.UUID    `json:"to_club_id"`
	TransferWindow string       `json:"transfer_window"`
	Status         RumourStatus `json:"status"`
	FeeMinEUR      *float64     `json:"fee_min_eur,omitempty"`
	FeeMaxEUR      *float64     `json:"fee_max_eur,omitempty"`
	Summary        *string      `json:"summary,omitempty"`
	Confidence     *float64     `json:"confidence,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// User authenticates via Google OAuth only (see internal/auth). Email is
// not treated as a unique identity key — GoogleSub is.
type User struct {
	ID          uuid.UUID `json:"id"`
	GoogleSub   string    `json:"-"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	AvatarURL   *string   `json:"avatar_url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Author is the public projection of a User used in comment/reaction
// responses — deliberately excludes Email.
type Author struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
	AvatarURL   *string   `json:"avatar_url,omitempty"`
}

type Comment struct {
	ID        uuid.UUID `json:"id"`
	RumourID  uuid.UUID `json:"rumour_id"`
	Author    Author    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type RumourEvent struct {
	ID         uuid.UUID    `json:"id"`
	RumourID   uuid.UUID    `json:"rumour_id"`
	ArticleID  uuid.UUID    `json:"article_id"`
	SourceID   uuid.UUID    `json:"source_id"`
	Status     RumourStatus `json:"status"`
	FeeMinEUR  *float64     `json:"fee_min_eur,omitempty"`
	FeeMaxEUR  *float64     `json:"fee_max_eur,omitempty"`
	Summary    *string      `json:"summary,omitempty"`
	Confidence *float64     `json:"confidence,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
}
