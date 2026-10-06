package extract

import (
	"context"
	"fmt"
	"strings"
)

// GatedExtractor asks a cheap Classifier whether an article is a transfer
// rumour before paying the Extractor (Claude) to extract it. Most fetched
// articles are not rumours, and the classifier answers that one question at a
// fraction of the cost; Claude is still needed for the fields, which a
// classifier cannot generate.
type GatedExtractor struct {
	Classifier Classifier
	Extractor  Extractor
	// MinProbability is the classifier probability an article must reach
	// to be sent to Extractor.
	MinProbability float64
}

// Extract implements Extractor. A classifier error is returned as-is — it
// never falls through to Extractor, because an unguarded call is exactly the
// spend the gate exists to prevent. A failed article is not a stored rumour;
// pipeline.RunExtract records it with a NULL extraction.
func (g *GatedExtractor) Extract(ctx context.Context, articleText string) (Result, error) {
	title, body := splitArticleText(articleText)
	c, err := g.Classifier.Classify(ctx, title, body)
	if err != nil {
		return Result{}, fmt.Errorf("gate: %w", err)
	}
	probability := c.Probability

	if probability < g.MinProbability {
		return Result{IsTransferRumour: false, JevProbability: &probability, JevModel: c.Model}, nil
	}

	result, err := g.Extractor.Extract(ctx, articleText)
	if err != nil {
		return Result{}, err
	}
	result.JevProbability = &probability
	result.JevModel = c.Model
	return result, nil
}

// splitArticleText undoes internal/pipeline's extractOne, which joins an article's
// title and content with a blank line; Jev gets them as separate state
// fields. Text with no blank line is a title-only article.
func splitArticleText(text string) (title, body string) {
	title, body, _ = strings.Cut(text, "\n\n")
	return title, body
}
