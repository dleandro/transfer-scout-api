package extract

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

// labelledArticle is one row of testdata/labelled_articles.json: an article
// text and whether a human says it reports a specific transfer rumour.
type labelledArticle struct {
	Name             string `json:"name"`
	IsTransferRumour bool   `json:"is_transfer_rumour"`
	Text             string `json:"text"`
}

// TestEval_ClassifiesLabelledArticles measures the gate against labelled
// examples instead of asserting a threshold is correct by taste.
//
// It costs real tokens, so it runs only when EXTRACT_API_KEY is set — the
// rest of the suite stays offline and free. Run it with:
//
//	EXTRACT_API_KEY=... go test ./internal/extract/ -run TestEval -v
//
// It fails on a FALSE POSITIVE — a non-rumour the gate would store — because
// that is the defect this whole mechanism exists to fix, and one is enough to
// pollute the corpus. A false negative is reported but tolerated: missing one
// rumour costs a row, admitting one match report costs the credibility of
// every number computed from the table afterwards.
//
// The fixtures are written rather than sampled, which is this test's main
// limitation: they are clean examples of each category, so passing here is a
// floor and not proof the gate holds against real scraped prose. Replace them
// with labelled articles from the real corpus when there is one worth
// sampling.
func TestEval_ClassifiesLabelledArticles(t *testing.T) {
	apiKey := os.Getenv("EXTRACT_API_KEY")
	if apiKey == "" {
		t.Skip("EXTRACT_API_KEY not set; skipping the evaluation (it calls the real model)")
	}

	raw, err := os.ReadFile("testdata/labelled_articles.json")
	if err != nil {
		t.Fatalf("read labelled set: %v", err)
	}
	var articles []labelledArticle
	if err := json.Unmarshal(raw, &articles); err != nil {
		t.Fatalf("parse labelled set: %v", err)
	}

	model := os.Getenv("EXTRACT_MODEL")
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	extractor := NewAnthropicExtractor(apiKey, model)

	const minConfidence = 0.4
	var truePos, trueNeg, falsePos, falseNeg int

	for _, article := range articles {
		t.Run(article.Name, func(t *testing.T) {
			result, err := extractor.Extract(context.Background(), article.Text)
			if err != nil {
				t.Fatalf("extract: %v", err)
			}

			stored := result.Usable(minConfidence)
			switch {
			case article.IsTransferRumour && stored:
				truePos++
			case !article.IsTransferRumour && !stored:
				trueNeg++
			case !article.IsTransferRumour && stored:
				falsePos++
				t.Errorf("FALSE POSITIVE: not a transfer rumour, but the gate would store it as %q to %q (confidence %.2f)",
					result.PlayerName, result.ToClubName, result.Confidence)
			default:
				falseNeg++
				t.Logf("false negative (tolerated): a real rumour was dropped (is_transfer_rumour=%v, confidence %.2f)",
					result.IsTransferRumour, result.Confidence)
			}
		})
	}

	t.Logf("labelled set: %d articles — %d true positive, %d true negative, %d FALSE POSITIVE, %d false negative",
		len(articles), truePos, trueNeg, falsePos, falseNeg)
}
