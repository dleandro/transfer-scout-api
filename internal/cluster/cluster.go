// Package cluster turns a single article's extraction result into a
// rumour thread + timeline event: resolving player/club names to IDs
// (creating them if needed), upserting the rumour, and appending the
// event.
package cluster

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/extract"
	"github.com/dleandro/transfer-scout-api/internal/models"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

// Store is the subset of store.Store the clusterer needs. Defined here
// (rather than depending on the concrete *store.Store) so Upsert can be
// exercised in tests against a fake, without a real Postgres connection.
type Store interface {
	GetOrCreateClub(ctx context.Context, name string) (uuid.UUID, error)
	GetOrCreatePlayer(ctx context.Context, name string) (uuid.UUID, error)
	UpsertRumour(ctx context.Context, p store.UpsertRumourParams) (models.Rumour, error)
	InsertRumourEvent(ctx context.Context, p store.RumourEventParams) error
}

type Clusterer struct {
	store Store
}

func New(s Store) *Clusterer {
	return &Clusterer{store: s}
}

// Upsert turns a single article's extraction result into a rumour +
// timeline event, returning the rumour's ID. Extractions with confidence
// <= 0 are skipped (per extract.SystemPrompt, that means "not actually
// about a specific transfer rumour") — Upsert returns uuid.Nil, nil in
// that case.
func (c *Clusterer) Upsert(ctx context.Context, articleID, sourceID uuid.UUID, result extract.Result, transferWindow string) (uuid.UUID, error) {
	if result.Confidence <= 0 {
		return uuid.Nil, nil
	}

	playerID, err := c.store.GetOrCreatePlayer(ctx, result.PlayerName)
	if err != nil {
		return uuid.Nil, fmt.Errorf("cluster: get or create player: %w", err)
	}

	toClubID, err := c.store.GetOrCreateClub(ctx, result.ToClubName)
	if err != nil {
		return uuid.Nil, fmt.Errorf("cluster: get or create to-club: %w", err)
	}

	var fromClubID *uuid.UUID
	if result.FromClubName != nil && *result.FromClubName != "" {
		id, err := c.store.GetOrCreateClub(ctx, *result.FromClubName)
		if err != nil {
			return uuid.Nil, fmt.Errorf("cluster: get or create from-club: %w", err)
		}
		fromClubID = &id
	}

	var summary *string
	if result.Summary != "" {
		summary = &result.Summary
	}
	confidence := result.Confidence
	status := models.RumourStatus(result.Status)

	rumour, err := c.store.UpsertRumour(ctx, store.UpsertRumourParams{
		PlayerID:       playerID,
		FromClubID:     fromClubID,
		ToClubID:       toClubID,
		TransferWindow: transferWindow,
		Status:         status,
		FeeMinEUR:      result.FeeMinEUR,
		FeeMaxEUR:      result.FeeMaxEUR,
		Summary:        summary,
		Confidence:     &confidence,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("cluster: upsert rumour: %w", err)
	}

	if err := c.store.InsertRumourEvent(ctx, store.RumourEventParams{
		RumourID:   rumour.ID,
		ArticleID:  articleID,
		SourceID:   sourceID,
		Status:     status,
		FeeMinEUR:  result.FeeMinEUR,
		FeeMaxEUR:  result.FeeMaxEUR,
		Summary:    summary,
		Confidence: &confidence,
	}); err != nil {
		return uuid.Nil, fmt.Errorf("cluster: insert rumour event: %w", err)
	}

	return rumour.ID, nil
}
