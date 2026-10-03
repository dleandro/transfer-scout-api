package cluster

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/extract"
	"github.com/dleandro/transfer-scout-api/internal/models"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

// fakeStore is an in-memory Store for tests, re-implementing just enough
// of the real Postgres upsert semantics (case-insensitive get-or-create,
// forward-only status, widening fee range) to exercise Clusterer's
// orchestration logic without a real database.
type fakeStore struct {
	clubs       map[string]uuid.UUID
	players     map[string]uuid.UUID
	rumours     map[string]models.Rumour
	events      []store.RumourEventParams
	reliability map[uuid.UUID]float64 // sourceID -> reliability_score, only tracked once a source appears in an event
	nudges      []nudgeCall
}

type nudgeCall struct {
	rumourID uuid.UUID
	delta    float64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		clubs:       map[string]uuid.UUID{},
		players:     map[string]uuid.UUID{},
		rumours:     map[string]models.Rumour{},
		reliability: map[uuid.UUID]float64{},
	}
}

func (f *fakeStore) GetOrCreateClub(ctx context.Context, name string) (uuid.UUID, error) {
	return getOrCreate(f.clubs, name), nil
}

func (f *fakeStore) GetOrCreatePlayer(ctx context.Context, name string) (uuid.UUID, error) {
	return getOrCreate(f.players, name), nil
}

func getOrCreate(m map[string]uuid.UUID, name string) uuid.UUID {
	key := strings.ToLower(strings.TrimSpace(name))
	if id, ok := m[key]; ok {
		return id
	}
	id := uuid.New()
	m[key] = id
	return id
}

func rumourKey(playerID, toClubID uuid.UUID, window string) string {
	return playerID.String() + "|" + toClubID.String() + "|" + window
}

func (f *fakeStore) UpsertRumour(ctx context.Context, p store.UpsertRumourParams) (models.Rumour, bool, error) {
	key := rumourKey(p.PlayerID, p.ToClubID, p.TransferWindow)
	r := models.Rumour{
		ID:             uuid.New(),
		PlayerID:       p.PlayerID,
		FromClubID:     p.FromClubID,
		ToClubID:       p.ToClubID,
		TransferWindow: p.TransferWindow,
		Status:         p.Status,
		FeeMinEUR:      p.FeeMinEUR,
		FeeMaxEUR:      p.FeeMaxEUR,
		Summary:        p.Summary,
		Confidence:     p.Confidence,
	}

	wasTerminal := false
	if existing, ok := f.rumours[key]; ok {
		r.ID = existing.ID
		wasTerminal = existing.Status.IsTerminal()
		if !existing.Status.IsForwardTransition(p.Status) {
			r.Status = existing.Status
		}
		r.FeeMinEUR = leastPtr(existing.FeeMinEUR, p.FeeMinEUR)
		r.FeeMaxEUR = greatestPtr(existing.FeeMaxEUR, p.FeeMaxEUR)
		if p.FromClubID == nil {
			r.FromClubID = existing.FromClubID
		}
	}

	f.rumours[key] = r
	return r, !wasTerminal && r.Status.IsTerminal(), nil
}

func (f *fakeStore) InsertRumourEvent(ctx context.Context, p store.RumourEventParams) error {
	f.events = append(f.events, p)
	if _, ok := f.reliability[p.SourceID]; !ok {
		f.reliability[p.SourceID] = 50.0
	}
	return nil
}

func (f *fakeStore) NudgeSourceReliability(ctx context.Context, rumourID uuid.UUID, delta float64) error {
	f.nudges = append(f.nudges, nudgeCall{rumourID: rumourID, delta: delta})
	for _, ev := range f.events {
		if ev.RumourID != rumourID {
			continue
		}
		score := f.reliability[ev.SourceID] + delta
		if score < 0 {
			score = 0
		}
		if score > 100 {
			score = 100
		}
		f.reliability[ev.SourceID] = score
	}
	return nil
}

func leastPtr(a, b *float64) *float64 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if *a < *b {
		return a
	}
	return b
}

func greatestPtr(a, b *float64) *float64 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if *a > *b {
		return a
	}
	return b
}

func TestClusterer_Upsert_CreatesNewRumourAndEvent(t *testing.T) {
	fs := newFakeStore()
	c := New(fs, testMinConfidence)

	result := extract.Result{IsTransferRumour: true,
		PlayerName: "Test Player",
		ToClubName: "Test Club",
		Status:     "rumoured",
		Summary:    "Test summary",
		Confidence: 0.7,
	}

	rumourID, err := c.Upsert(context.Background(), uuid.New(), uuid.New(), result, "summer-2026")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rumourID == uuid.Nil {
		t.Fatal("expected a non-nil rumour id")
	}
	if len(fs.events) != 1 {
		t.Fatalf("got %d events, want 1", len(fs.events))
	}
	if len(fs.players) != 1 || len(fs.clubs) != 1 {
		t.Fatalf("expected exactly one player and one club to be created, got %d players, %d clubs", len(fs.players), len(fs.clubs))
	}
}

// testMinConfidence is the threshold these tests run the gate at. Most of
// them are about clustering rather than the gate, so their results sit
// comfortably above it.
const testMinConfidence = 0.4

func TestClusterer_Upsert_RejectsWhatTheGateExcludes(t *testing.T) {
	tests := []struct {
		name   string
		result extract.Result
	}{
		{
			// The case that used to leak: a match report or strategy piece
			// the model rated non-zero. The old gate was confidence > 0, so
			// anything like this was stored as a real rumour.
			name: "not a transfer rumour, however confident",
			result: extract.Result{
				IsTransferRumour: false,
				PlayerName:       "P",
				ToClubName:       "C",
				Status:           "rumoured",
				Confidence:       0.99,
			},
		},
		{
			name: "a rumour whose details the model does not trust",
			result: extract.Result{
				IsTransferRumour: true,
				PlayerName:       "P",
				ToClubName:       "C",
				Status:           "rumoured",
				Confidence:       testMinConfidence - 0.01,
			},
		},
		{
			name:   "an abstention, which names nobody at all",
			result: extract.Result{IsTransferRumour: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := newFakeStore()
			c := New(fs, testMinConfidence)

			rumourID, err := c.Upsert(context.Background(), uuid.New(), uuid.New(), tt.result, "summer-2026")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rumourID != uuid.Nil {
				t.Error("expected no rumour to be created")
			}
			// No club or player row either: a rejected extraction must not
			// leave invented names behind for the next one to cluster onto.
			if len(fs.events) != 0 || len(fs.players) != 0 || len(fs.clubs) != 0 {
				t.Errorf("expected no side effects, got %d events, %d players, %d clubs",
					len(fs.events), len(fs.players), len(fs.clubs))
			}
		})
	}
}

func TestClusterer_Upsert_AcceptsExactlyAtTheThreshold(t *testing.T) {
	fs := newFakeStore()
	c := New(fs, testMinConfidence)

	result := extract.Result{
		IsTransferRumour: true,
		PlayerName:       "P",
		ToClubName:       "C",
		Status:           "rumoured",
		Confidence:       testMinConfidence,
	}
	rumourID, err := c.Upsert(context.Background(), uuid.New(), uuid.New(), result, "summer-2026")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rumourID == uuid.Nil {
		t.Error("a result exactly at the threshold should be stored")
	}
}

func TestClusterer_Upsert_ClustersSamePlayerClubWindowIntoOneRumour(t *testing.T) {
	fs := newFakeStore()
	c := New(fs, testMinConfidence)
	ctx := context.Background()

	result1 := extract.Result{IsTransferRumour: true, PlayerName: "Same Player", ToClubName: "Same Club", Status: "advanced", Confidence: 0.8}
	id1, err := c.Upsert(ctx, uuid.New(), uuid.New(), result1, "summer-2026")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A later report at an *earlier* stage should not roll the status back.
	result2 := extract.Result{IsTransferRumour: true, PlayerName: "same player", ToClubName: "SAME CLUB", Status: "talks", Confidence: 0.5}
	id2, err := c.Upsert(ctx, uuid.New(), uuid.New(), result2, "summer-2026")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id1 != id2 {
		t.Fatalf("expected both extractions to cluster into the same rumour, got %s and %s", id1, id2)
	}
	if len(fs.events) != 2 {
		t.Fatalf("got %d events, want 2 (one per article)", len(fs.events))
	}

	got := fs.rumours[rumourKey(fs.players["same player"], fs.clubs["same club"], "summer-2026")]
	if got.Status != models.StatusAdvanced {
		t.Errorf("status regressed: got %s, want advanced (a later 'talks' report should not roll it back)", got.Status)
	}
}

func TestClusterer_Upsert_WidensFeeRangeAcrossReports(t *testing.T) {
	fs := newFakeStore()
	c := New(fs, testMinConfidence)
	ctx := context.Background()

	fee1min, fee1max := 20_000_000.0, 30_000_000.0
	result1 := extract.Result{IsTransferRumour: true, PlayerName: "P", ToClubName: "C", Status: "rumoured", Confidence: 0.5, FeeMinEUR: &fee1min, FeeMaxEUR: &fee1max}
	if _, err := c.Upsert(ctx, uuid.New(), uuid.New(), result1, "summer-2026"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fee2min, fee2max := 15_000_000.0, 25_000_000.0
	result2 := extract.Result{IsTransferRumour: true, PlayerName: "P", ToClubName: "C", Status: "talks", Confidence: 0.5, FeeMinEUR: &fee2min, FeeMaxEUR: &fee2max}
	if _, err := c.Upsert(ctx, uuid.New(), uuid.New(), result2, "summer-2026"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := fs.rumours[rumourKey(fs.players["p"], fs.clubs["c"], "summer-2026")]
	if got.FeeMinEUR == nil || *got.FeeMinEUR != 15_000_000 {
		t.Errorf("fee min: got %v, want 15000000 (should widen to the lowest reported)", got.FeeMinEUR)
	}
	if got.FeeMaxEUR == nil || *got.FeeMaxEUR != 30_000_000 {
		t.Errorf("fee max: got %v, want 30000000 (should widen to the highest reported)", got.FeeMaxEUR)
	}
}

func TestClusterer_Upsert_ResolvesFromClub(t *testing.T) {
	fs := newFakeStore()
	c := New(fs, testMinConfidence)

	fromClub := "Origin Club"
	result := extract.Result{IsTransferRumour: true,
		PlayerName:   "P",
		FromClubName: &fromClub,
		ToClubName:   "Destination Club",
		Status:       "rumoured",
		Confidence:   0.5,
	}

	if _, err := c.Upsert(context.Background(), uuid.New(), uuid.New(), result, "summer-2026"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fs.clubs) != 2 {
		t.Fatalf("expected both from-club and to-club to be created, got %d clubs", len(fs.clubs))
	}
}

func TestClusterer_Upsert_ConfirmedNudgesContributingSourcesUp(t *testing.T) {
	fs := newFakeStore()
	c := New(fs, testMinConfidence)
	ctx := context.Background()

	source1, source2 := uuid.New(), uuid.New()

	result1 := extract.Result{IsTransferRumour: true, PlayerName: "P", ToClubName: "C", Status: "talks", Confidence: 0.5}
	if _, err := c.Upsert(ctx, uuid.New(), source1, result1, "summer-2026"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fs.nudges) != 0 {
		t.Fatalf("expected no nudge before resolution, got %d", len(fs.nudges))
	}

	result2 := extract.Result{IsTransferRumour: true, PlayerName: "P", ToClubName: "C", Status: "confirmed", Confidence: 0.9}
	if _, err := c.Upsert(ctx, uuid.New(), source2, result2, "summer-2026"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fs.nudges) != 1 {
		t.Fatalf("expected exactly one nudge on resolution, got %d", len(fs.nudges))
	}
	if fs.nudges[0].delta <= 0 {
		t.Errorf("expected a positive nudge for a confirmed rumour, got delta %v", fs.nudges[0].delta)
	}
	if fs.reliability[source1] <= 50.0 || fs.reliability[source2] <= 50.0 {
		t.Errorf("expected both contributing sources' reliability to increase, got source1=%v source2=%v",
			fs.reliability[source1], fs.reliability[source2])
	}
}

func TestClusterer_Upsert_CollapsedNudgesContributingSourcesDown(t *testing.T) {
	fs := newFakeStore()
	c := New(fs, testMinConfidence)
	ctx := context.Background()

	source := uuid.New()

	result1 := extract.Result{IsTransferRumour: true, PlayerName: "P", ToClubName: "C", Status: "talks", Confidence: 0.5}
	if _, err := c.Upsert(ctx, uuid.New(), source, result1, "summer-2026"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result2 := extract.Result{IsTransferRumour: true, PlayerName: "P", ToClubName: "C", Status: "collapsed", Confidence: 0.9}
	if _, err := c.Upsert(ctx, uuid.New(), source, result2, "summer-2026"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fs.nudges) != 1 || fs.nudges[0].delta >= 0 {
		t.Fatalf("expected exactly one negative nudge for a collapsed rumour, got %+v", fs.nudges)
	}
	if fs.reliability[source] >= 50.0 {
		t.Errorf("expected the contributing source's reliability to decrease, got %v", fs.reliability[source])
	}
}

func TestClusterer_Upsert_OnlyNudgesOnceEvenWithFurtherReportsAfterResolution(t *testing.T) {
	fs := newFakeStore()
	c := New(fs, testMinConfidence)
	ctx := context.Background()

	result1 := extract.Result{IsTransferRumour: true, PlayerName: "P", ToClubName: "C", Status: "confirmed", Confidence: 0.9}
	if _, err := c.Upsert(ctx, uuid.New(), uuid.New(), result1, "summer-2026"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fs.nudges) != 1 {
		t.Fatalf("expected one nudge after the first resolution, got %d", len(fs.nudges))
	}

	// A follow-up article re-reporting the same confirmed deal should not
	// trigger a second nudge.
	result2 := extract.Result{IsTransferRumour: true, PlayerName: "P", ToClubName: "C", Status: "confirmed", Confidence: 0.95}
	if _, err := c.Upsert(ctx, uuid.New(), uuid.New(), result2, "summer-2026"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fs.nudges) != 1 {
		t.Fatalf("expected still only one nudge after a repeat confirmation, got %d", len(fs.nudges))
	}
}
