package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/store"
)

var reclassifyCutoff = time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)

// reclassifyFixture inserts rumours dated 2020-01-01 (before the cutoff)
// and one dated 2020-01-03 (after it), linked to articles via
// rumour_events, plus a comment and a like on the first rumour.
type reclassifyFixture struct {
	twoArticles, noArticles, afterCutoff uuid.UUID
	articleA, articleB                   uuid.UUID
}

func newReclassifyFixture(t *testing.T, s *store.Store) reclassifyFixture {
	t.Helper()
	ctx := context.Background()
	var f reclassifyFixture

	var playerID, clubID, sourceID uuid.UUID
	mustScan := func(dst *uuid.UUID, q string, args ...any) {
		t.Helper()
		if err := s.Pool.QueryRow(ctx, q, args...).Scan(dst); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustScan(&playerID, `INSERT INTO players (name) VALUES ($1) RETURNING id`, uniqueName("Reclassify Player"))
	mustScan(&clubID, `INSERT INTO clubs (name) VALUES ($1) RETURNING id`, uniqueName("Reclassify Club"))
	mustScan(&sourceID, `INSERT INTO sources (name) VALUES ($1) RETURNING id`, uniqueName("Reclassify Source"))

	insertRumour := func(dst *uuid.UUID, createdAt string) {
		mustScan(dst, `INSERT INTO rumours (player_id, to_club_id, transfer_window, status, created_at, updated_at)
			VALUES ($1, $2, $3, 'rumoured', $4, $4) RETURNING id`, playerID, clubID, uniqueName("reclassify-window"), createdAt)
	}
	insertRumour(&f.twoArticles, "2020-01-01T00:00:00.000Z")
	insertRumour(&f.noArticles, "2020-01-01T00:00:00.000Z")
	insertRumour(&f.afterCutoff, "2020-01-03T00:00:00.000Z")

	insertArticle := func(dst *uuid.UUID, title, createdAt string) {
		mustScan(dst, `INSERT INTO articles (source_id, url, title, content, fetched_at, created_at)
			VALUES ($1, $2, $3, 'body', $4, $4) RETURNING id`, sourceID, "https://example.test/"+uuid.NewString(), title, createdAt)
	}
	insertArticle(&f.articleA, "first article", "2020-01-01T00:00:00.000Z")
	insertArticle(&f.articleB, "second article", "2020-01-01T00:00:00.000Z")

	for i, link := range []struct{ rumour, article uuid.UUID }{
		{f.twoArticles, f.articleA}, {f.twoArticles, f.articleB}, {f.afterCutoff, f.articleA},
	} {
		if _, err := s.Pool.Exec(ctx, `INSERT INTO rumour_events (rumour_id, article_id, source_id, status, created_at)
			VALUES ($1, $2, $3, 'rumoured', $4)`, link.rumour, link.article, sourceID,
			time.Date(2020, 1, 1, 0, i, 0, 0, time.UTC)); err != nil {
			t.Fatalf("insert rumour_event: %v", err)
		}
	}

	user, err := s.UpsertUser(ctx, "google-sub-"+uuid.NewString(), "reclassify@example.com", "Reclassify", "")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO comments (rumour_id, user_id, body) VALUES ($1, $2, 'hi')`, f.twoArticles, user.ID); err != nil {
		t.Fatalf("insert comment: %v", err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO likes (rumour_id, user_id) VALUES ($1, $2)`, f.twoArticles, user.ID); err != nil {
		t.Fatalf("insert like: %v", err)
	}
	return f
}

func TestIntegration_ListRumoursForReclassify_GroupsArticlesAndRespectsCutoff(t *testing.T) {
	s := newTestStore(t)
	f := newReclassifyFixture(t, s)

	rumours, err := s.ListRumoursForReclassify(context.Background(), reclassifyCutoff)
	if err != nil {
		t.Fatalf("ListRumoursForReclassify: %v", err)
	}
	byID := map[uuid.UUID]store.ReclassifyRumour{}
	for _, r := range rumours {
		if _, dup := byID[r.ID]; dup {
			t.Fatalf("rumour %s returned twice; articles must be grouped under one row", r.ID)
		}
		byID[r.ID] = r
	}

	two, ok := byID[f.twoArticles]
	if !ok {
		t.Fatal("rumour created before the cutoff is missing")
	}
	if len(two.Articles) != 2 || two.Articles[0].ID != f.articleA || two.Articles[1].ID != f.articleB {
		t.Errorf("articles = %+v, want first then second article", two.Articles)
	}
	if two.Articles[0].Title != "first article" || two.Articles[0].Content == nil || *two.Articles[0].Content != "body" {
		t.Errorf("article fields not populated: %+v", two.Articles[0])
	}
	if none, ok := byID[f.noArticles]; !ok || len(none.Articles) != 0 {
		t.Errorf("rumour with no events: present=%v articles=%+v, want present with no articles", ok, none.Articles)
	}
	if _, ok := byID[f.afterCutoff]; ok {
		t.Error("rumour created after the cutoff was returned")
	}
}

func TestIntegration_DeleteRumours_CascadesAndCountsInOneTransaction(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	f := newReclassifyFixture(t, s)
	ids := []uuid.UUID{f.twoArticles, f.noArticles}
	want := store.RumourDeletion{Rumours: 2, Events: 2, Comments: 1, Likes: 1}

	dry, err := s.CountRumourDeletion(ctx, ids)
	if err != nil {
		t.Fatalf("CountRumourDeletion: %v", err)
	}
	if dry != want {
		t.Errorf("dry-run count = %+v, want %+v", dry, want)
	}

	got, err := s.DeleteRumours(ctx, ids)
	if err != nil {
		t.Fatalf("DeleteRumours: %v", err)
	}
	if got != want {
		t.Errorf("deleted = %+v, want %+v", got, want)
	}

	var rumours, events, comments, likes, articles, kept int
	if err := s.Pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM rumours WHERE id = ANY($1)),
			(SELECT count(*) FROM rumour_events WHERE rumour_id = ANY($1)),
			(SELECT count(*) FROM comments WHERE rumour_id = ANY($1)),
			(SELECT count(*) FROM likes WHERE rumour_id = ANY($1)),
			(SELECT count(*) FROM articles WHERE id = ANY($2)),
			(SELECT count(*) FROM rumour_events WHERE rumour_id = $3)`,
		ids, []uuid.UUID{f.articleA, f.articleB}, f.afterCutoff).Scan(&rumours, &events, &comments, &likes, &articles, &kept); err != nil {
		t.Fatalf("count leftovers: %v", err)
	}
	if rumours+events+comments+likes != 0 {
		t.Errorf("left behind: %d rumours, %d events, %d comments, %d likes", rumours, events, comments, likes)
	}
	if articles != 2 {
		t.Errorf("%d of 2 articles survive, want both: articles are never deleted", articles)
	}
	if kept != 1 {
		t.Errorf("the rumour not in ids lost its event (%d left, want 1)", kept)
	}
}
