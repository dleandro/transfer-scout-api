package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// recordRumourTool is the schema the model must fill in. Pinning the shape
// here rather than describing it in prose means a missing or mistyped field
// is the API's problem, not something that silently becomes a Go zero value
// after json.Unmarshal.
const recordRumourTool = "record_rumour"

type anthropicRequest struct {
	Model      string              `json:"model"`
	MaxTokens  int                 `json:"max_tokens"`
	System     string              `json:"system"`
	Messages   []anthropicMessage  `json:"messages"`
	Tools      []anthropicTool     `json:"tools"`
	ToolChoice anthropicToolChoice `json:"tool_choice"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// Type "tool" forces this specific tool, so the model cannot answer with
// prose instead of a structured result.
type anthropicToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

func rumourToolSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"player_name": map[string]any{
				"type":        "string",
				"description": "The player being linked with a move, as named in the article. Empty if the article names none.",
			},
			"from_club_name": map[string]any{
				"type":        []string{"string", "null"},
				"description": "The club the player would leave, if named.",
			},
			"to_club_name": map[string]any{
				"type":        "string",
				"description": "The club the player is linked with, as named in the article. Empty if the article names none.",
			},
			"status": map[string]any{
				"type": "string",
				"enum": []string{"rumoured", "talks", "advanced", "medical", "confirmed", "collapsed"},
			},
			"fee_min_eur": map[string]any{"type": []string{"number", "null"}},
			"fee_max_eur": map[string]any{"type": []string{"number", "null"}},
			"summary": map[string]any{
				"type":        "string",
				"description": "One sentence describing the rumour.",
			},
			"confidence": map[string]any{
				"type":        "number",
				"description": "0-1: how sure you are of the extracted fields.",
			},
		},
		"required": []string{"player_name", "to_club_name", "status", "confidence"},
	}
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
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
		Tools: []anthropicTool{{
			Name:        recordRumourTool,
			Description: "Record the details of the transfer rumour this article reports.",
			InputSchema: rumourToolSchema(),
		}},
		ToolChoice: anthropicToolChoice{Type: "tool", Name: recordRumourTool},
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

	// tool_choice forces record_rumour, so a response without a tool_use
	// block means the call did not do what was asked — worth an error rather
	// than falling back to parsing whatever prose came back.
	var input json.RawMessage
	for _, block := range apiResp.Content {
		if block.Type == "tool_use" && block.Name == recordRumourTool {
			input = block.Input
			break
		}
	}
	if input == nil {
		return Result{}, fmt.Errorf("extract: no %s tool_use block in response", recordRumourTool)
	}

	var result Result
	if err := json.Unmarshal(input, &result); err != nil {
		return Result{}, fmt.Errorf("extract: parse tool input: %w (raw: %s)", err, truncate(string(input), 500))
	}

	if err := validateResult(result); err != nil {
		return Result{}, fmt.Errorf("extract: invalid model output: %w", err)
	}

	return result, nil
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
	if r.Confidence < 0 || r.Confidence > 1 {
		return fmt.Errorf("confidence %v out of range [0,1]", r.Confidence)
	}
	if !validStatuses[r.Status] {
		return fmt.Errorf("unknown status %q", r.Status)
	}
	return nil
}
