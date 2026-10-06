package extract

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/dleandro/transfer-scout-api/internal/config"
)

// labelledArticle is one row of testdata/labelled_articles.json: an article
// text and whether a human says it reports a specific transfer rumour.
type labelledArticle struct {
	Name             string `json:"name"`
	IsTransferRumour bool   `json:"is_transfer_rumour"`
	Text             string `json:"text"`
}

// TestEval_JevGateClassifiesLabelledArticles measures the Jev gate — the
// classifier that decides whether Claude is called at all — against labelled
// examples, instead of asserting a threshold is correct by taste.
//
// It costs real tokens, so it runs only when TYPESAFE_API_KEY is set — the
// rest of the suite stays offline and free. It never calls Claude. Run it
// with:
//
//	TYPESAFE_API_KEY=... go test ./internal/extract/ -run TestEval -v
//
// JEV_MODEL and JEV_MIN_PROBABILITY are honoured, defaulting to what the
// extract job runs with. Every article's probability is logged, followed by
// the confusion matrix at that threshold and a sweep over other thresholds —
// the sweep is what JEV_MIN_PROBABILITY should be chosen from.
//
// It fails on a FALSE POSITIVE at the configured threshold — a non-rumour the
// gate would pass on to Claude and, from there, possibly store — because that
// is the defect this whole mechanism exists to fix. A false negative is
// reported but tolerated: missing one rumour costs a row, admitting one match
// report costs the credibility of every number computed from the table
// afterwards.
//
// The fixtures are written rather than sampled, which is this test's main
// limitation: they are clean examples of each category, so passing here is a
// floor and not proof the gate holds against real scraped prose. Replace them
// with labelled articles from the real corpus when there is one worth
// sampling.
func TestEval_JevGateClassifiesLabelledArticles(t *testing.T) {
	apiKey := os.Getenv("TYPESAFE_API_KEY")
	if apiKey == "" {
		t.Skip("TYPESAFE_API_KEY not set; skipping the evaluation (it calls the real Jev model)")
	}

	raw, err := os.ReadFile("testdata/labelled_articles.json")
	if err != nil {
		t.Fatalf("read labelled set: %v", err)
	}
	var articles []labelledArticle
	if err := json.Unmarshal(raw, &articles); err != nil {
		t.Fatalf("parse labelled set: %v", err)
	}

	model := os.Getenv("JEV_MODEL")
	if model == "" {
		model = config.DefaultJevModel
	}
	threshold := config.DefaultJevMinProbability
	if v := os.Getenv("JEV_MIN_PROBABILITY"); v != "" {
		if threshold, err = strconv.ParseFloat(v, 64); err != nil || threshold < 0 || threshold > 1 {
			t.Fatalf("JEV_MIN_PROBABILITY=%q is not a number in [0,1]", v)
		}
	}
	classifier := NewJevClassifier(apiKey, model)

	type scored struct {
		article     labelledArticle
		probability float64
	}
	var results []scored
	for _, article := range articles {
		// The fixtures are body text with no headline.
		c, err := classifier.Classify(context.Background(), "", article.Text)
		if err != nil {
			t.Fatalf("classify %q: %v", article.Name, err)
		}
		t.Logf("%-45s label=%-5v jev=%.3f (%s)", article.Name, article.IsTransferRumour, c.Probability, c.Model)
		results = append(results, scored{article, c.Probability})
	}

	// confusion counts outcomes at one threshold; passed means "sent on to
	// Claude", the same >= comparison GatedExtractor makes.
	confusion := func(th float64) (tp, tn, fp, fn int) {
		for _, r := range results {
			passed := r.probability >= th
			switch {
			case r.article.IsTransferRumour && passed:
				tp++
			case !r.article.IsTransferRumour && !passed:
				tn++
			case passed:
				fp++
			default:
				fn++
			}
		}
		return
	}

	for _, r := range results {
		if r.probability < threshold {
			if r.article.IsTransferRumour {
				t.Logf("false negative (tolerated): %q is a rumour but Jev gave %.3f < %.2f", r.article.Name, r.probability, threshold)
			}
			continue
		}
		if !r.article.IsTransferRumour {
			t.Errorf("FALSE POSITIVE: %q is not a transfer rumour, but Jev gave %.3f >= %.2f and it would reach Claude",
				r.article.Name, r.probability, threshold)
		}
	}

	tp, tn, fp, fn := confusion(threshold)
	t.Logf("%s at threshold %.2f over %d articles: %d true positive, %d true negative, %d FALSE POSITIVE, %d false negative",
		model, threshold, len(results), tp, tn, fp, fn)

	var sweep strings.Builder
	sweep.WriteString("threshold sweep (passed = sent to Claude):\n  threshold   TP   TN   FP   FN\n")
	for _, th := range []float64{0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9} {
		tp, tn, fp, fn := confusion(th)
		fmt.Fprintf(&sweep, "  %9.2f %4d %4d %4d %4d\n", th, tp, tn, fp, fn)
	}
	t.Log(sweep.String())
}
