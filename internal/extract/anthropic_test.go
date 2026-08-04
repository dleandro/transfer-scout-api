package extract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func anthropicSuccessBody(t *testing.T, text string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"content": []map[string]string{{"type": "text", "text": text}},
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
	resultJSON := `{"player_name":"Test Player","from_club_name":"Club A","to_club_name":"Club B","status":"talks","fee_min_eur":10000000,"fee_max_eur":15000000,"summary":"Test summary","confidence":0.8}`
	srv := newTestServer(t, http.StatusOK, anthropicSuccessBody(t, resultJSON))

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

func TestAnthropicExtractor_Extract_StripsCodeFence(t *testing.T) {
	resultJSON := "```json\n{\"player_name\":\"P\",\"to_club_name\":\"C\",\"status\":\"rumoured\",\"summary\":\"s\",\"confidence\":0.1}\n```"
	srv := newTestServer(t, http.StatusOK, anthropicSuccessBody(t, resultJSON))

	result, err := newTestExtractor(srv.URL).Extract(context.Background(), "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PlayerName != "P" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestAnthropicExtractor_Extract_InvalidStatusRejected(t *testing.T) {
	resultJSON := `{"player_name":"P","to_club_name":"C","status":"not-a-real-status","summary":"s","confidence":0.5}`
	srv := newTestServer(t, http.StatusOK, anthropicSuccessBody(t, resultJSON))

	if _, err := newTestExtractor(srv.URL).Extract(context.Background(), "text"); err == nil {
		t.Fatal("expected an error for an invalid status, got nil")
	}
}

func TestAnthropicExtractor_Extract_ConfidenceOutOfRangeRejected(t *testing.T) {
	resultJSON := `{"player_name":"P","to_club_name":"C","status":"rumoured","summary":"s","confidence":1.5}`
	srv := newTestServer(t, http.StatusOK, anthropicSuccessBody(t, resultJSON))

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

func TestAnthropicExtractor_Extract_MalformedJSONRejected(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, anthropicSuccessBody(t, "not json at all"))

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
