// Package extract defines the LLM extraction contract used to turn raw
// article text into structured rumour data.
package extract

import (
	"context"
	"errors"
)

// SystemPrompt is sent to the extraction model together with a single
// article's text. The response shape is pinned by the record_rumour tool
// schema (see anthropic.go), not by asking for JSON in prose.
const SystemPrompt = `You are a football transfer-rumour extraction assistant.

You will be given the text of a single news article. Decide whether it reports
a specific transfer rumour, and if so extract its details, by calling the
record_rumour tool.

A specific transfer rumour names a player and the club he is linked with. It
is still a rumour whether the move is early speculation or all but done.

These are NOT transfer rumours:
- match reports, previews and results
- a club's transfer plans or strategy with no specific player named
- a player signing a new contract with his current club, which is not a move
- manager and coaching appointments
- transfer round-ups that mention no specific deal
- club finances, ownership or stadium news
- anything about a different sport

When the article is not a specific transfer rumour, set is_transfer_rumour to
false and leave player_name, from_club_name and to_club_name empty. Do not
guess a player or a club to fill the fields — an empty answer is correct and
useful, a fabricated one is not.

confidence is about the details you extracted, not about the decision above:
how sure you are that the player, clubs, status and fee are right. Leave it at
0 when is_transfer_rumour is false.`

// Result is the structured output the model returns per article, matching
// the record_rumour tool's input schema.
type Result struct {
	// IsTransferRumour is the model's answer to the only question that
	// decides whether anything is stored. It is deliberately separate from
	// Confidence: a single score could not distinguish "definitely a rumour,
	// unsure of the fee" from "probably not a rumour at all", and the two
	// need opposite handling.
	IsTransferRumour bool     `json:"is_transfer_rumour"`
	PlayerName       string   `json:"player_name"`
	FromClubName     *string  `json:"from_club_name"`
	ToClubName       string   `json:"to_club_name"`
	Status           string   `json:"status"`
	FeeMinEUR        *float64 `json:"fee_min_eur"`
	FeeMaxEUR        *float64 `json:"fee_max_eur"`
	Summary          string   `json:"summary"`
	// Confidence is how sure the model is of the extracted fields, in [0,1].
	// It qualifies the details; it does not decide whether this is a rumour.
	Confidence float64 `json:"confidence"`
}

// Usable reports whether a result should be turned into a rumour.
//
// Both conditions matter and they are not interchangeable. The boolean keeps
// out articles that are not rumours at all — match reports, contract renewals,
// strategy pieces — which is what used to leak through, because the old gate
// was "confidence > 0" and the model happily rated an ambiguous article 0.3.
// minConfidence then keeps out rumours whose details the model does not trust,
// which would otherwise store a real deal with the wrong club or fee.
func (r Result) Usable(minConfidence float64) bool {
	return r.IsTransferRumour && r.Confidence >= minConfidence
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
