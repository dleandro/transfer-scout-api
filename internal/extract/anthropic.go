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
)

const (
	defaultAnthropicBaseURL = "https://api.anthropic.com/v1/messages"
	anthropicVersion        = "2023-06-01"
	maxResponseTokens       = 1024
)

// AnthropicExtractor calls the Anthropic Messages API to extract structured
// rumour data from article text, per the SystemPrompt contract.
type AnthropicExtractor struct {
	APIKey     string
	Model      string
	BaseURL    string // overridable in tests
	HTTPClient *http.Client
}

func NewAnthropicExtractor(apiKey, model string) *AnthropicExtractor {
	return &AnthropicExtractor{
		APIKey:     apiKey,
		Model:      model,
		BaseURL:    defaultAnthropicBaseURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (e *AnthropicExtractor) Extract(ctx context.Context, articleText string) (Result, error) {
	if e.APIKey == "" {
		return Result{}, fmt.Errorf("extract: no API key configured")
	}

	reqBody, err := json.Marshal(anthropicRequest{
		Model:     e.Model,
		MaxTokens: maxResponseTokens,
		System:    SystemPrompt,
		Messages: []anthropicMessage{
			{Role: "user", Content: articleText},
		},
	})
	if err != nil {
		return Result{}, fmt.Errorf("extract: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL, bytes.NewReader(reqBody))
	if err != nil {
		return Result{}, fmt.Errorf("extract: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", e.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	client := e.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("extract: call model: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, fmt.Errorf("extract: read response: %w", err)
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return Result{}, fmt.Errorf("extract: decode response: %w (body: %s)", err, truncate(string(respBody), 500))
	}

	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("status %d", resp.StatusCode)
		if apiResp.Error != nil {
			msg = apiResp.Error.Message
		}
		return Result{}, fmt.Errorf("extract: model call failed: %s", msg)
	}

	if len(apiResp.Content) == 0 {
		return Result{}, fmt.Errorf("extract: empty response content")
	}

	raw := stripCodeFence(strings.TrimSpace(apiResp.Content[0].Text))

	var result Result
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return Result{}, fmt.Errorf("extract: parse model JSON: %w (raw: %s)", err, truncate(raw, 500))
	}

	if err := validateResult(result); err != nil {
		return Result{}, fmt.Errorf("extract: invalid model output: %w", err)
	}

	return result, nil
}

func stripCodeFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

var validStatuses = map[string]bool{
	"rumoured":  true,
	"talks":     true,
	"advanced":  true,
	"medical":   true,
	"confirmed": true,
	"collapsed": true,
}

func validateResult(r Result) error {
	if r.PlayerName == "" {
		return fmt.Errorf("player_name is empty")
	}
	if r.ToClubName == "" {
		return fmt.Errorf("to_club_name is empty")
	}
	if !validStatuses[r.Status] {
		return fmt.Errorf("unknown status %q", r.Status)
	}
	if r.Confidence < 0 || r.Confidence > 1 {
		return fmt.Errorf("confidence %v out of range [0,1]", r.Confidence)
	}
	return nil
}
