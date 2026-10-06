// Package extract defines the LLM extraction contract used to turn raw
// article text into structured rumour data.
package extract

import (
	"context"
	"errors"
)

const SystemPrompt = `You are a football transfer-rumour extraction assistant.

You will be given the text of a single news article that reports a transfer
rumour: a player being linked with a move to a club. Record its details by
calling the record_rumour tool.

Fill in the player, the club he would leave, the club he is linked with, the
status of the move, the fee range in euros and a one-sentence summary.

Take every name from the article. If the article does not name the player or
the club he is linked with, leave that field empty rather than guess — an
empty field is correct and useful, a fabricated one is not.

confidence is how sure you are that the player, clubs, status and fee you
recorded are right, from 0 to 1.`

type Result struct {
	PlayerName   string   `json:"player_name"`
	FromClubName *string  `json:"from_club_name"`
	ToClubName   string   `json:"to_club_name"`
	Status       string   `json:"status"`
	FeeMinEUR    *float64 `json:"fee_min_eur"`
	FeeMaxEUR    *float64 `json:"fee_max_eur"`
	Summary      string   `json:"summary"`
	Confidence   float64  `json:"confidence"`

	RejectedByJev  bool     `json:"rejected_by_jev,omitempty"`
	JevProbability *float64 `json:"jev_probability,omitempty"`
	JevModel       string   `json:"jev_model,omitempty"`
}

// ErrNotImplemented is returned by StubExtractor. cmd/extract runs against
// this when no API key is configured.
var ErrNotImplemented = errors.New("extract: not implemented (no extractor configured)")

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
