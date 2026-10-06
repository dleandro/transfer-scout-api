package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ReclassifyArticle is one article that put a rumour on its timeline.
type ReclassifyArticle struct {
	ID      uuid.UUID
	Title   string
	Content *string
}

// ReclassifyRumour is a rumour together with every article linked to it
// through rumour_events — the evidence cmd/reclassify re-judges.
type ReclassifyRumour struct {
	ID         uuid.UUID
	PlayerName string
	ToClubName string
	CreatedAt  time.Time
	// Articles is empty for a rumour with no rumour_events at all.
	Articles []ReclassifyArticle
}

// ListRumoursForReclassify returns every rumour created before
// createdBefore with the articles linked to it, oldest rumour first. A
// rumour with no rumour_events is still returned, with no articles, so the
// caller sees it rather than having it silently disappear from the report.
func (s *Store) ListRumoursForReclassify(ctx context.Context, createdBefore time.Time) ([]ReclassifyRumour, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT r.id, p.name, c.name, r.created_at, a.id, a.title, a.content
		FROM rumours r
		JOIN players p ON p.id = r.player_id
		JOIN clubs c ON c.id = r.to_club_id
		LEFT JOIN rumour_events e ON e.rumour_id = r.id
		LEFT JOIN articles a ON a.id = e.article_id
		WHERE r.created_at < $1
		ORDER BY r.created_at, r.id, e.created_at, a.id`, createdBefore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rumours []ReclassifyRumour
	for rows.Next() {
		var (
			r         ReclassifyRumour
			articleID *uuid.UUID
			title     *string
			content   *string
		)
		if err := rows.Scan(&r.ID, &r.PlayerName, &r.ToClubName, &r.CreatedAt, &articleID, &title, &content); err != nil {
			return nil, err
		}
		if n := len(rumours); n == 0 || rumours[n-1].ID != r.ID {
			rumours = append(rumours, r)
		}
		if articleID != nil {
			last := &rumours[len(rumours)-1]
			last.Articles = append(last.Articles, ReclassifyArticle{ID: *articleID, Title: *title, Content: content})
		}
	}
	return rumours, rows.Err()
}

// RumourDeletion counts what deleting a set of rumours removes. Events,
// comments and likes go with their rumour through ON DELETE CASCADE;
// articles are never deleted.
type RumourDeletion struct {
	Rumours  int64
	Events   int64
	Comments int64
	Likes    int64
}

const countRumourDependents = `
	SELECT
		(SELECT count(*) FROM rumours       WHERE id        = ANY($1)),
		(SELECT count(*) FROM rumour_events WHERE rumour_id = ANY($1)),
		(SELECT count(*) FROM comments      WHERE rumour_id = ANY($1)),
		(SELECT count(*) FROM likes         WHERE rumour_id = ANY($1))`

// CountRumourDeletion reports what DeleteRumours would remove for ids,
// without removing anything.
func (s *Store) CountRumourDeletion(ctx context.Context, ids []uuid.UUID) (RumourDeletion, error) {
	var d RumourDeletion
	err := s.Pool.QueryRow(ctx, countRumourDependents, ids).Scan(&d.Rumours, &d.Events, &d.Comments, &d.Likes)
	return d, err
}

// DeleteRumours deletes ids and, by cascade, their events, comments and
// likes, in one transaction, and returns what went. Counting happens inside
// the same transaction so the report matches the delete exactly.
func (s *Store) DeleteRumours(ctx context.Context, ids []uuid.UUID) (RumourDeletion, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return RumourDeletion{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	var d RumourDeletion
	if err := tx.QueryRow(ctx, countRumourDependents, ids).Scan(&d.Rumours, &d.Events, &d.Comments, &d.Likes); err != nil {
		return RumourDeletion{}, fmt.Errorf("count dependents: %w", err)
	}
	tag, err := tx.Exec(ctx, `DELETE FROM rumours WHERE id = ANY($1)`, ids)
	if err != nil {
		return RumourDeletion{}, fmt.Errorf("delete rumours: %w", err)
	}
	if tag.RowsAffected() != d.Rumours {
		return RumourDeletion{}, fmt.Errorf("deleted %d rumours, counted %d", tag.RowsAffected(), d.Rumours)
	}
	if err := tx.Commit(ctx); err != nil {
		return RumourDeletion{}, err
	}
	return d, nil
}
