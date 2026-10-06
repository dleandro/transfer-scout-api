package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultJevBaseURL = "https://api.typesafe.ai/v1/systemone"
	// jevQuestionID is our own key for the one question we ask; TypeSafe
	// does not send it to the model, it only routes the answer back.
	jevQuestionID = "is_transfer_rumour"

	// maxJevBodyBytes keeps state well inside jev-1.13's 32k-token budget
	// for state plus the longest question (roughly 4 bytes per token, so
	// this is ~6k tokens). The opening of a news article is where the
	// player and club are named, so cutting the tail costs the decision
	// little, and a smaller state also suffers less of the context rot the
	// Jev docs warn about.
	maxJevBodyBytes = 24000
)

const jevInstructions = "Does the article in `title` and `body` report a specific football transfer rumour: a named player being linked with a move to a named club?"

const jevCriteriaTrue = "The article names a specific player and the club he is linked with moving to. " +
	"It counts whether the move is early speculation, interest or monitoring, talks, a bid, a medical, " +
	"a deal reported as all but done or agreed, or a pursuit that has collapsed."

const jevCriteriaFalse = "The article is not about a specific player moving to a specific club. This includes: " +
	"match reports, previews and results; a club's transfer plans or strategy with no specific player named; " +
	"a player signing a new contract with his current club, which is not a move; manager and coaching appointments; " +
	"transfer round-ups that mention no specific deal; club finances, ownership or stadium news; " +
	"and anything about a sport other than football."

// Classification is Jev's answer to whether an article is a transfer rumour.
type Classification struct {
	// Probability is the Noul answer: 0 means no, 1 means yes.
	Probability float64
	// Model is the versioned model id that answered, as reported by the
	// response — never the alias we may have asked for.
	Model string
}

// Classifier decides whether an article is a transfer rumour without
// extracting anything from it.
type Classifier interface {
	Classify(ctx context.Context, title, body string) (Classification, error)
}

// JevClassifier calls TypeSafe's System One endpoint with a single Noul
// question. Hand-rolled rather than an SDK: there is no official Go SDK, and
// the request is one POST.
type JevClassifier struct {
	APIKey     string
	Model      string
	BaseURL    string // overridable in tests and local smoke runs
	HTTPClient *http.Client
}

func NewJevClassifier(apiKey, model string) *JevClassifier {
	return &JevClassifier{
		APIKey:     apiKey,
		Model:      model,
		BaseURL:    defaultJevBaseURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type jevRequest struct {
	State     map[string]string      `json:"state"`
	Model     string                 `json:"model"`
	Questions map[string]jevQuestion `json:"questions"`
}

type jevQuestion struct {
	Type         string      `json:"type"`
	Instructions string      `json:"instructions"`
	Criteria     jevCriteria `json:"criteria"`
}

type jevCriteria struct {
	True  string `json:"true"`
	False string `json:"false"`
}

type jevResponse struct {
	Model   string `json:"model"`
	Answers map[string]struct {
		Type string   `json:"type"`
		Noul *float64 `json:"noul"`
	} `json:"answers"`
}

// jevState builds a structured state with named fields, as the State docs
// recommend over one string, so the question can point at `title` and
// `body`. Empty fields are left out rather than sent as "".
func jevState(title, body string) map[string]string {
	state := make(map[string]string, 2)
	if title = strings.TrimSpace(title); title != "" {
		state["title"] = title
	}
	if body = strings.TrimSpace(body); body != "" {
		state["body"] = truncateUTF8(body, maxJevBodyBytes)
	}
	return state
}

// truncateUTF8 cuts s to at most n bytes without splitting a rune.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func (c *JevClassifier) Classify(ctx context.Context, title, body string) (Classification, error) {
	if c.APIKey == "" {
		return Classification{}, fmt.Errorf("jev: no API key configured")
	}
	state := jevState(title, body)
	if len(state) == 0 {
		return Classification{}, fmt.Errorf("jev: article has neither title nor body")
	}

	reqBody, err := json.Marshal(jevRequest{
		State: state,
		Model: c.Model,
		Questions: map[string]jevQuestion{
			jevQuestionID: {
				Type:         "noul",
				Instructions: jevInstructions,
				Criteria:     jevCriteria{True: jevCriteriaTrue, False: jevCriteriaFalse},
			},
		},
	})
	if err != nil {
		return Classification{}, fmt.Errorf("jev: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(reqBody))
	if err != nil {
		return Classification{}, fmt.Errorf("jev: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return Classification{}, fmt.Errorf("jev: call model: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Classification{}, fmt.Errorf("jev: read response: %w", err)
	}

	// No retry here: a 429/529 fails this article like any other
	// extraction failure. pipeline.RunExtract then marks it processed with a NULL
	// extraction (its existing policy for failures), which is never a
	// stored rumour and is findable for a re-run.
	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("jev: model call failed: status %d", resp.StatusCode)
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			msg += " (retry-after " + ra + ")"
		}
		return Classification{}, fmt.Errorf("%s: %s", msg, truncate(string(respBody), 500))
	}

	var apiResp jevResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return Classification{}, fmt.Errorf("jev: decode response: %w (body: %s)", err, truncate(string(respBody), 500))
	}

	answer, ok := apiResp.Answers[jevQuestionID]
	if !ok || answer.Type != "noul" || answer.Noul == nil {
		return Classification{}, fmt.Errorf("jev: no noul answer for %q in response (body: %s)", jevQuestionID, truncate(string(respBody), 500))
	}
	if p := *answer.Noul; p < 0 || p > 1 {
		return Classification{}, fmt.Errorf("jev: noul %v out of range [0,1]", p)
	}
	if apiResp.Model == "" {
		return Classification{}, fmt.Errorf("jev: response has no model id")
	}

	return Classification{Probability: *answer.Noul, Model: apiResp.Model}, nil
}
