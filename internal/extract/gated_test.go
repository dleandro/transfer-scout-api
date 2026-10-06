package extract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dleandro/transfer-scout-api/internal/config"
)

// countingAnthropic is a fake Messages API that counts how often it is paid.
func countingAnthropic(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	body := anthropicToolBody(t, `{"is_transfer_rumour":true,"player_name":"Morten Hjulmand","to_club_name":"Arsenal","status":"talks","summary":"s","confidence":0.8}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func newTestGate(jevURL, anthropicURL string, minProbability float64) *GatedExtractor {
	return &GatedExtractor{
		Classifier:     newTestJev(jevURL),
		Extractor:      newTestExtractor(anthropicURL),
		MinProbability: minProbability,
	}
}

func TestGatedExtractor_BelowThresholdSkipsClaude(t *testing.T) {
	jev := newFakeJev(t, http.StatusOK, jevBody("0.12"), nil)
	claude, claudeCalls := countingAnthropic(t)

	result, err := newTestGate(jev.srv.URL, claude.URL, 0.5).Extract(context.Background(), "Liverpool 2-1 Everton\n\nVan Dijk header wins it.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := claudeCalls.Load(); n != 0 {
		t.Errorf("Anthropic got %d calls, want 0 below the threshold", n)
	}
	if result.IsTransferRumour || result.Usable(0) {
		t.Errorf("result = %+v, want a non-rumour", result)
	}
	if result.JevProbability == nil || *result.JevProbability != 0.12 || result.JevModel != "jev-1.13.0" {
		t.Errorf("gate verdict not recorded: probability=%v model=%q", result.JevProbability, result.JevModel)
	}

	var req struct {
		State map[string]string `json:"state"`
	}
	if err := json.Unmarshal(jev.lastRaw, &req); err != nil {
		t.Fatalf("decode Jev request: %v", err)
	}
	if req.State["title"] != "Liverpool 2-1 Everton" || req.State["body"] != "Van Dijk header wins it." {
		t.Errorf("state = %+v, want the article split into title and body", req.State)
	}
}

func TestGatedExtractor_AtOrAboveThresholdCallsClaude(t *testing.T) {
	for _, noul := range []string{"0.5", "0.97"} {
		t.Run(noul, func(t *testing.T) {
			jev := newFakeJev(t, http.StatusOK, jevBody(noul), nil)
			claude, claudeCalls := countingAnthropic(t)

			result, err := newTestGate(jev.srv.URL, claude.URL, 0.5).Extract(context.Background(), "Arsenal bid for Hjulmand")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if n := claudeCalls.Load(); n != 1 {
				t.Errorf("Anthropic got %d calls, want 1", n)
			}
			if !result.IsTransferRumour || result.PlayerName != "Morten Hjulmand" {
				t.Errorf("Claude's extraction not returned: %+v", result)
			}
			if result.JevProbability == nil || result.JevModel != "jev-1.13.0" {
				t.Errorf("gate verdict not recorded on a passed article: %+v", result)
			}

			raw, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if !strings.Contains(string(raw), `"jev_probability":`) || !strings.Contains(string(raw), `"jev_model":"jev-1.13.0"`) {
				t.Errorf("stored extraction JSON lacks the gate verdict: %s", raw)
			}
		})
	}
}

func TestGatedExtractor_JevErrorNeverReachesClaude(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			jev := newFakeJev(t, status, `{"detail":"no"}`, nil)
			claude, claudeCalls := countingAnthropic(t)

			if _, err := newTestGate(jev.srv.URL, claude.URL, 0.5).Extract(context.Background(), "Arsenal bid for Hjulmand"); err == nil {
				t.Fatal("expected the Jev error to be returned")
			}
			if n := claudeCalls.Load(); n != 0 {
				t.Errorf("Anthropic got %d calls after a Jev error, want 0", n)
			}
		})
	}
}

func TestGatedExtractor_MalformedAnswerIsAnError(t *testing.T) {
	jev := newFakeJev(t, http.StatusOK, `{"model":"jev-1.13.0","answers":{"something_else":{"type":"noul","noul":0.9}}}`, nil)
	claude, claudeCalls := countingAnthropic(t)

	if _, err := newTestGate(jev.srv.URL, claude.URL, 0.5).Extract(context.Background(), "Arsenal bid for Hjulmand"); err == nil {
		t.Fatal("expected an error for a malformed answer")
	}
	if n := claudeCalls.Load(); n != 0 {
		t.Errorf("Anthropic got %d calls, want 0", n)
	}
}

func TestNewFromConfig(t *testing.T) {
	t.Run("no extract key is the stub", func(t *testing.T) {
		e, err := NewFromConfig(config.Config{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := e.(StubExtractor); !ok {
			t.Errorf("got %T, want StubExtractor", e)
		}
	})
	t.Run("extract key without typesafe key refuses", func(t *testing.T) {
		if _, err := NewFromConfig(config.Config{ExtractAPIKey: "sk"}); err == nil {
			t.Fatal("expected a refusal to run Claude ungated")
		}
	})
	t.Run("both keys is the gate", func(t *testing.T) {
		e, err := NewFromConfig(config.Config{ExtractAPIKey: "sk", TypesafeAPIKey: "ts", JevModel: "jev-1.13.0", JevMinProbability: 0.7})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		g, ok := e.(*GatedExtractor)
		if !ok {
			t.Fatalf("got %T, want *GatedExtractor", e)
		}
		if g.MinProbability != 0.7 {
			t.Errorf("MinProbability = %v, want 0.7", g.MinProbability)
		}
		if j := g.Classifier.(*JevClassifier); j.Model != "jev-1.13.0" || j.BaseURL != defaultJevBaseURL {
			t.Errorf("classifier = %+v", j)
		}
	})
}
