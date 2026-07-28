// Package extract defines the LLM extraction contract used to turn raw
// article text into structured rumour data. The actual model call is not
// yet implemented — see milestone 1.3.
package extract

import (
	"context"
	"errors"
)

// SystemPrompt is sent to the extraction model together with a single
// article's text. The model must respond with JSON matching Result.
const SystemPrompt = `You are a football transfer-rumour extraction assistant.
Given the text of a single news article about a Premier League transfer
rumour, extract the following as JSON, matching this exact shape:

{
  "player_name": string,
  "from_club_name": string | null,
  "to_club_name": string,
  "status": "rumoured" | "talks" | "advanced" | "medical" | "confirmed" | "collapsed",
  "fee_min_eur": number | null,
  "fee_max_eur": number | null,
  "summary": string,
  "confidence": number
}

confidence is 0.0-1.0: how confident the extraction is that this article is
about a genuine, specific transfer rumour (as opposed to speculation about a
club's transfer strategy in general, a different sport, etc).

If the article is not about a specific player transfer rumour, return
confidence 0 and best-effort values for the other fields.
Respond with JSON only, no surrounding text.`

// Result is the structured output the model returns per article, matching
// the shape described in SystemPrompt.
type Result struct {
	PlayerName   string   `json:"player_name"`
	FromClubName *string  `json:"from_club_name"`
	ToClubName   string   `json:"to_club_name"`
	Status       string   `json:"status"`
	FeeMinEUR    *float64 `json:"fee_min_eur"`
	FeeMaxEUR    *float64 `json:"fee_max_eur"`
	Summary      string   `json:"summary"`
	Confidence   float64  `json:"confidence"`
}

// ErrNotImplemented is returned by StubExtractor. cmd/extract runs against
// this until milestone 1.3 wires up a real model client.
var ErrNotImplemented = errors.New("extract: not implemented (see milestone 1.3)")

// Extractor calls a model to extract structured rumour data from raw
// article text, per the SystemPrompt contract.
type Extractor interface {
	Extract(ctx context.Context, articleText string) (Result, error)
}

// StubExtractor is a placeholder Extractor that always returns
// ErrNotImplemented.
type StubExtractor struct{}

func (StubExtractor) Extract(ctx context.Context, articleText string) (Result, error) {
	return Result{}, ErrNotImplemented
}
