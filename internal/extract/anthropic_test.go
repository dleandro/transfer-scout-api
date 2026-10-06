package extract

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, statusCode int, responseBody string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// anthropicToolBody is what the API returns for a forced tool call: a
// tool_use content block whose input is the filled-in schema.
func anthropicToolBody(t *testing.T, input string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"content": []map[string]any{
			{"type": "tool_use", "name": recordRumourTool, "input": json.RawMessage(input)},
		},
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(body)
}

func newTestExtractor(baseURL string) *AnthropicExtractor {
	e := NewAnthropicExtractor("test-key", "test-model")
	e.BaseURL = baseURL
	return e
}

func TestAnthropicExtractor_Extract_Success(t *testing.T) {
	input := `{"player_name":"Test Player","from_club_name":"Club A","to_club_name":"Club B","status":"talks","fee_min_eur":10000000,"fee_max_eur":15000000,"summary":"Test summary","confidence":0.8}`
	srv := newTestServer(t, http.StatusOK, anthropicToolBody(t, input))

	result, err := newTestExtractor(srv.URL).Extract(context.Background(), "some article text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PlayerName != "Test Player" || result.ToClubName != "Club B" || result.Status != "talks" {
		t.Errorf("unexpected result: %+v", result)
	}
	if result.Confidence != 0.8 {
		t.Errorf("unexpected confidence: %v", result.Confidence)
	}
	if result.FeeMinEUR == nil || *result.FeeMinEUR != 10000000 {
		t.Errorf("unexpected fee_min_eur: %v", result.FeeMinEUR)
	}
}

func TestAnthropicExtractor_Extract_EmptyNamesReturnedWithoutError(t *testing.T) {
	input := `{"player_name":"","from_club_name":null,"to_club_name":"","status":"rumoured","fee_min_eur":null,"fee_max_eur":null,"summary":"s","confidence":0.2}`
	srv := newTestServer(t, http.StatusOK, anthropicToolBody(t, input))

	result, err := newTestExtractor(srv.URL).Extract(context.Background(), "text")
	if err != nil {
		t.Fatalf("empty names must reach the clusterer as an incomplete extraction, not fail: %v", err)
	}
	if result.PlayerName != "" || result.ToClubName != "" || result.Confidence != 0.2 {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestAnthropicExtractor_Extract_TextResponseRejected(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"content": []map[string]any{{"type": "text", "text": `{"player_name":"P","to_club_name":"C","status":"rumoured","summary":"s","confidence":0.5}`}},
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	srv := newTestServer(t, http.StatusOK, string(body))

	if _, err := newTestExtractor(srv.URL).Extract(context.Background(), "text"); err == nil {
		t.Fatal("expected an error when the response carries no tool_use block")
	}
}

func TestAnthropicExtractor_Extract_SendsForcedToolSchema(t *testing.T) {
	var got anthropicRequest
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		if raw, err = io.ReadAll(r.Body); err != nil {
			t.Errorf("read request: %v", err)
		}
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(anthropicToolBody(t, `{"player_name":"P","to_club_name":"C","status":"rumoured","confidence":0.5}`)))
	}))
	t.Cleanup(srv.Close)

	if _, err := newTestExtractor(srv.URL).Extract(context.Background(), "text"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Tools) != 1 || got.Tools[0].Name != recordRumourTool {
		t.Fatalf("expected the %s tool to be offered, got %+v", recordRumourTool, got.Tools)
	}
	if got.ToolChoice.Type != "tool" || got.ToolChoice.Name != recordRumourTool {
		t.Errorf("expected tool_choice to force %s, got %+v", recordRumourTool, got.ToolChoice)
	}
	required, ok := got.Tools[0].InputSchema["required"].([]any)
	if !ok {
		t.Fatalf("expected required fields in the schema, got %v", got.Tools[0].InputSchema["required"])
	}
	for _, field := range []string{"player_name", "to_club_name", "status", "confidence"} {
		if !slices.Contains(required, any(field)) {
			t.Errorf("%s must be required, got %v", field, required)
		}
	}
	if strings.Contains(string(raw), "is_transfer_rumour") {
		t.Errorf("the request must not ask Claude to classify the article, got %s", raw)
	}
}

func TestAnthropicExtractor_Extract_InvalidStatusRejected(t *testing.T) {
	input := `{"player_name":"P","to_club_name":"C","status":"not-a-real-status","summary":"s","confidence":0.5}`
	srv := newTestServer(t, http.StatusOK, anthropicToolBody(t, input))

	if _, err := newTestExtractor(srv.URL).Extract(context.Background(), "text"); err == nil {
		t.Fatal("expected an error for an invalid status, got nil")
	}
}

func TestAnthropicExtractor_Extract_ConfidenceOutOfRangeRejected(t *testing.T) {
	input := `{"player_name":"P","to_club_name":"C","status":"rumoured","summary":"s","confidence":1.5}`
	srv := newTestServer(t, http.StatusOK, anthropicToolBody(t, input))

	if _, err := newTestExtractor(srv.URL).Extract(context.Background(), "text"); err == nil {
		t.Fatal("expected an error for out-of-range confidence, got nil")
	}
}

func TestAnthropicExtractor_Extract_APIErrorSurfaced(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"error": map[string]string{"type": "invalid_request_error", "message": "bad request"},
	})
	srv := newTestServer(t, http.StatusBadRequest, string(body))

	_, err := newTestExtractor(srv.URL).Extract(context.Background(), "text")
	if err == nil {
		t.Fatal("expected an error for a non-200 response, got nil")
	}
}

func TestAnthropicExtractor_Extract_MalformedToolInputRejected(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, anthropicToolBody(t, `"not an object at all"`))

	if _, err := newTestExtractor(srv.URL).Extract(context.Background(), "text"); err == nil {
		t.Fatal("expected an error for malformed model output, got nil")
	}
}

func TestAnthropicExtractor_Extract_NoAPIKey(t *testing.T) {
	e := NewAnthropicExtractor("", "test-model")

	if _, err := e.Extract(context.Background(), "text"); err == nil {
		t.Fatal("expected an error when no API key is configured")
	}
}

func TestStubExtractor_ReturnsNotImplemented(t *testing.T) {
	_, err := (StubExtractor{}).Extract(context.Background(), "text")
	if err != ErrNotImplemented {
		t.Fatalf("got error %v, want ErrNotImplemented", err)
	}
}
