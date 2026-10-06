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

	"github.com/dleandro/transfer-scout-api/internal/extract"
	"github.com/dleandro/transfer-scout-api/internal/models"
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

// countingExtractor returns a non-rumour for every article and counts calls,
// so a test can tell an article was re-extracted.
type countingExtractor struct{ calls int }

func (e *countingExtractor) Extract(ctx context.Context, text string) (extract.Result, error) {
	e.calls++
	return extract.Result{IsTransferRumour: false}, nil
}

// rejectingClusterer behaves like the real gate on a non-rumour.
type rejectingClusterer struct{}

func (rejectingClusterer) Upsert(ctx context.Context, articleID, sourceID uuid.UUID, result extract.Result, transferWindow string) (uuid.UUID, error) {
	return uuid.Nil, nil
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
		Clusterer:      rejectingClusterer{},
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
