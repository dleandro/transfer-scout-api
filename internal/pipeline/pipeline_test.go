package pipeline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/dleandro/transfer-scout-api/internal/cluster"
	"github.com/dleandro/transfer-scout-api/internal/extract"
	"github.com/dleandro/transfer-scout-api/internal/models"
	"github.com/dleandro/transfer-scout-api/internal/store"
)

// fakeStore is an in-memory ExtractStore mirroring the real query: the
// oldest unprocessed articles first, up to limit. MarkExtracted fails for
// IDs in unmarkable, leaving them queued exactly as a failed UPDATE would.
type fakeStore struct {
	articles   []models.Article
	unmarkable map[uuid.UUID]bool
	listCalls  int
}

func newFakeStore(n int) *fakeStore {
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	f := &fakeStore{unmarkable: map[uuid.UUID]bool{}}
	for i := range n {
		f.articles = append(f.articles, models.Article{
			ID:        uuid.New(),
			SourceID:  uuid.New(),
			URL:       fmt.Sprintf("https://example.com/%d", i),
			Title:     fmt.Sprintf("Article %d", i),
			FetchedAt: base.Add(time.Duration(i) * time.Minute),
		})
	}
	return f
}

func (f *fakeStore) ListUnprocessed(ctx context.Context, limit int) ([]models.Article, error) {
	f.listCalls++
	if f.listCalls > 1000 {
		return nil, errors.New("fakeStore: ListUnprocessed called over 1000 times, the drain loop is not terminating")
	}
	var out []models.Article
	for _, a := range f.articles {
		if !a.Processed && len(out) < limit {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeStore) MarkExtracted(ctx context.Context, id uuid.UUID, extractionJSON []byte) error {
	if f.unmarkable[id] {
		return errors.New("fakeStore: update failed")
	}
	for i := range f.articles {
		if f.articles[i].ID == id {
			f.articles[i].Processed = true
		}
	}
	return nil
}

func (f *fakeStore) queued() int {
	n := 0
	for _, a := range f.articles {
		if !a.Processed {
			n++
		}
	}
	return n
}

type countingExtractor struct{ calls int }

func (e *countingExtractor) Extract(ctx context.Context, text string) (extract.Result, error) {
	e.calls++
	return extract.Result{RejectedByJev: true}, nil
}

type scriptedExtractor struct{ byTitle map[string]extract.Result }

func (e scriptedExtractor) Extract(ctx context.Context, text string) (extract.Result, error) {
	title, _, _ := strings.Cut(text, "\n\n")
	return e.byTitle[title], nil
}

type clusterStore struct{ players, clubs, rumours, events int }

func (s *clusterStore) GetOrCreateClub(ctx context.Context, name string) (uuid.UUID, error) {
	s.clubs++
	return uuid.New(), nil
}

func (s *clusterStore) GetOrCreatePlayer(ctx context.Context, name string) (uuid.UUID, error) {
	s.players++
	return uuid.New(), nil
}

func (s *clusterStore) UpsertRumour(ctx context.Context, p store.UpsertRumourParams) (models.Rumour, bool, error) {
	s.rumours++
	return models.Rumour{ID: uuid.New(), Status: p.Status}, false, nil
}

func (s *clusterStore) InsertRumourEvent(ctx context.Context, p store.RumourEventParams) error {
	s.events++
	return nil
}

func (s *clusterStore) NudgeSourceReliability(ctx context.Context, rumourID uuid.UUID, delta float64) error {
	return nil
}

type fakePoller struct {
	err   error
	calls int
}

func (p *fakePoller) PollOnce(ctx context.Context) error {
	p.calls++
	return p.err
}

func deps(s *fakeStore, e extract.Extractor, maxArticles int) ExtractDeps {
	return ExtractDeps{
		Store:          s,
		Extractor:      e,
		Clusterer:      cluster.New(&clusterStore{}),
		TransferWindow: "summer-2026",
		MaxArticles:    maxArticles,
	}
}

// captureLogs routes slog's default logger into a buffer for the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestRunExtract_DrainsAQueueLargerThanOneBatch(t *testing.T) {
	s := newFakeStore(2*batchSize + 20)
	e := &countingExtractor{}

	st, err := RunExtract(context.Background(), deps(s, e, 500))
	if err != nil {
		t.Fatalf("RunExtract: %v", err)
	}
	if got := s.queued(); got != 0 {
		t.Errorf("%d articles still queued, want the queue drained", got)
	}
	if st.Total != 2*batchSize+20 || st.Batches != 3 || st.Rejected != st.Total {
		t.Errorf("stats = %+v, want total=%d batches=3 and every article rejected", st, 2*batchSize+20)
	}
	if st.Capped {
		t.Error("Capped = true for a queue under the cap")
	}
}

func TestRunExtract_StopsAtTheCapAndWarns(t *testing.T) {
	logs := captureLogs(t)
	s := newFakeStore(2 * batchSize)
	e := &countingExtractor{}

	st, err := RunExtract(context.Background(), deps(s, e, 70))
	if err != nil {
		t.Fatalf("RunExtract: %v", err)
	}
	if st.Total != 70 || e.calls != 70 {
		t.Errorf("total=%d extractor calls=%d, want both 70", st.Total, e.calls)
	}
	if got := s.queued(); got != 2*batchSize-70 {
		t.Errorf("%d articles still queued, want %d left for the next run", got, 2*batchSize-70)
	}
	if !st.Capped {
		t.Error("Capped = false, want true when the cap stops a non-empty queue")
	}
	if !strings.Contains(logs.String(), "level=WARN msg=\"extract: stopped at the per-run cap") {
		t.Errorf("no cap warning logged; logs:\n%s", logs)
	}
}

func TestRunExtract_NoCapWarningWhenTheCapExactlyDrainsTheQueue(t *testing.T) {
	logs := captureLogs(t)
	s := newFakeStore(batchSize)

	st, err := RunExtract(context.Background(), deps(s, &countingExtractor{}, batchSize))
	if err != nil {
		t.Fatalf("RunExtract: %v", err)
	}
	if st.Capped || strings.Contains(logs.String(), "per-run cap") {
		t.Errorf("Capped=%v with an empty queue left; logs:\n%s", st.Capped, logs)
	}
}

// An article whose MarkExtracted fails stays at the head of the queue. The
// run must neither loop on it forever nor pay to extract it again, and must
// still drain the articles behind it.
func TestRunExtract_TerminatesWhenAnArticleCannotBeMarked(t *testing.T) {
	s := newFakeStore(batchSize + 10)
	stuck := s.articles[0].ID
	s.unmarkable[stuck] = true
	e := &countingExtractor{}

	st, err := RunExtract(context.Background(), deps(s, e, 500))
	if err != nil {
		t.Fatalf("RunExtract: %v", err)
	}
	if e.calls != batchSize+10 {
		t.Errorf("extractor called %d times, want %d (each article once)", e.calls, batchSize+10)
	}
	if got := s.queued(); got != 1 {
		t.Errorf("%d articles still queued, want only the unmarkable one", got)
	}
	if st.Unmarked != 1 {
		t.Errorf("Unmarked = %d, want 1", st.Unmarked)
	}
}

// A full batch of unmarkable articles comes back unchanged from every
// ListUnprocessed, so "fewer than a full batch" never ends the loop; the
// run has to notice it is making no progress.
func TestRunExtract_StopsWhenAFullBatchIsUnmarkable(t *testing.T) {
	logs := captureLogs(t)
	s := newFakeStore(batchSize)
	for _, a := range s.articles {
		s.unmarkable[a.ID] = true
	}
	e := &countingExtractor{}

	st, err := RunExtract(context.Background(), deps(s, e, 500))
	if err != nil {
		t.Fatalf("RunExtract: %v", err)
	}
	if e.calls != batchSize || s.listCalls != 2 {
		t.Errorf("extractor calls=%d list calls=%d, want %d and 2", e.calls, s.listCalls, batchSize)
	}
	if st.Unmarked != batchSize {
		t.Errorf("Unmarked = %d, want %d", st.Unmarked, batchSize)
	}
	if !strings.Contains(logs.String(), "level=WARN msg=\"extract: articles could not be marked processed") {
		t.Errorf("no warning about unmarkable articles; logs:\n%s", logs)
	}
}

func TestRunExtract_ReturnsAnErrorWhenInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := newFakeStore(5)

	if _, err := RunExtract(ctx, deps(s, &countingExtractor{}, 500)); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunExtract error = %v, want context.Canceled", err)
	}
	if got := s.queued(); got != 5 {
		t.Errorf("%d articles queued, want all 5 untouched", got)
	}
}

func TestRun_ExtractsAfterAFailedIngestAndReportsTheFailure(t *testing.T) {
	ingestErr := errors.New("every feed unreachable")
	p := &fakePoller{err: ingestErr}
	s := newFakeStore(batchSize + 5)

	err := Run(context.Background(), p, deps(s, &countingExtractor{}, 500))
	if !errors.Is(err, ingestErr) {
		t.Fatalf("Run error = %v, want it to carry the ingest failure", err)
	}
	if p.calls != 1 {
		t.Errorf("PollOnce called %d times, want 1", p.calls)
	}
	if got := s.queued(); got != 0 {
		t.Errorf("%d articles still queued, want extract to have drained them after the failed ingest", got)
	}
}

func TestRun_SucceedsWhenBothStagesDo(t *testing.T) {
	s := newFakeStore(3)

	if err := Run(context.Background(), &fakePoller{}, deps(s, &countingExtractor{}, 500)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := s.queued(); got != 0 {
		t.Errorf("%d articles still queued, want 0", got)
	}
}

func TestRunExtract_JevAloneDecidesAndIncompleteExtractionsAreCountedApart(t *testing.T) {
	logs := captureLogs(t)
	s := newFakeStore(3)
	passed := 0.9
	rejected := 0.1
	low := s.articles[0].Title
	incomplete := s.articles[1].Title
	notRumour := s.articles[2].Title
	e := scriptedExtractor{byTitle: map[string]extract.Result{
		low:        {PlayerName: "P", ToClubName: "C", Status: "rumoured", Confidence: 0.05, JevProbability: &passed},
		incomplete: {PlayerName: "P", Status: "rumoured", Confidence: 0.9, JevProbability: &passed},
		notRumour:  {RejectedByJev: true, JevProbability: &rejected, JevModel: "jev-1.13.0"},
	}}
	cs := &clusterStore{}
	d := deps(s, e, 500)
	d.Clusterer = cluster.New(cs)
	d.JevMinProbability = 0.5

	st, err := RunExtract(context.Background(), d)
	if err != nil {
		t.Fatalf("RunExtract: %v", err)
	}
	if st.Clustered != 1 || st.Incomplete != 1 || st.Rejected != 1 || st.Failed != 0 || st.Extracted != 3 {
		t.Errorf("stats = %+v, want clustered=1 incomplete=1 rejected=1 failed=0 extracted=3", st)
	}
	if cs.rumours != 1 || cs.events != 1 {
		t.Errorf("store got %d rumours and %d events, want only the low-confidence article stored", cs.rumours, cs.events)
	}

	out := logs.String()
	for _, want := range []string{
		`msg="extract: extraction incomplete, not stored" article_id=` + s.articles[1].ID.String(),
		`msg="extract: extraction rejected by the gate" article_id=` + s.articles[2].ID.String(),
		"jev_probability=0.1 jev_model=jev-1.13.0",
		"extracted=3 clustered=1 rejected=1 incomplete=1 failed=0 total=3",
		"jev_min_probability=0.5",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("logs lack %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "min_confidence") {
		t.Errorf("logs still mention min_confidence:\n%s", out)
	}
}
