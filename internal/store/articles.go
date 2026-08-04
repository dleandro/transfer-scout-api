package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/dleandro/transfer-scout-api/internal/models"
)

// InsertArticle inserts a new article. If the URL already exists (the
// ingest worker re-polls feeds regularly) it is a no-op: inserted is false
// and id is uuid.Nil.
func (s *Store) InsertArticle(ctx context.Context, a models.Article) (id uuid.UUID, inserted bool, err error) {
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO articles (source_id, url, title, content, published_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (url) DO NOTHING
		RETURNING id`,
		a.SourceID, a.URL, a.Title, a.Content, a.PublishedAt)

	if err := row.Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, false, nil
		}
		return uuid.Nil, false, err
	}
	return id, true, nil
}

// ListUnprocessed returns up to limit articles awaiting LLM extraction,
// oldest first.
func (s *Store) ListUnprocessed(ctx context.Context, limit int) ([]models.Article, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, source_id, url, title, content, published_at, fetched_at, processed, created_at
		FROM articles
		WHERE processed = false
		ORDER BY fetched_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []models.Article
	for rows.Next() {
		var a models.Article
		if err := rows.Scan(&a.ID, &a.SourceID, &a.URL, &a.Title, &a.Content, &a.PublishedAt, &a.FetchedAt, &a.Processed, &a.CreatedAt); err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

// MarkExtracted flags an article as having been through extraction and
// stores the raw extraction result JSON. extractionJSON is nil when
// extraction failed or found nothing usable — the article is still marked
// processed so the worker doesn't retry it forever.
func (s *Store) MarkExtracted(ctx context.Context, id uuid.UUID, extractionJSON []byte) error {
	_, err := s.Pool.Exec(ctx, `UPDATE articles SET processed = true, extraction = $2 WHERE id = $1`, id, extractionJSON)
	return err
}
