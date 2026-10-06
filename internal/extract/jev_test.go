package extract

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dleandro/transfer-scout-api/internal/config"
)

const jevOKBody = `{"model":"jev-1.13.0","answers":{"is_transfer_rumour":{"type":"noul","noul":%s}},"usage":{"input_tokens":300,"output_tokens":20}}`

func jevBody(noul string) string { return strings.Replace(jevOKBody, "%s", noul, 1) }

// fakeJev serves body with status and records the last request it saw.
type fakeJev struct {
	srv     *httptest.Server
	calls   atomic.Int32
	lastReq *http.Request
	lastRaw []byte
}

func newFakeJev(t *testing.T, status int, body string, headers map[string]string) *fakeJev {
	t.Helper()
	f := &fakeJev{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		f.lastReq = r
		f.lastRaw, _ = io.ReadAll(r.Body)
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func newTestJev(baseURL string) *JevClassifier {
	c := NewJevClassifier("ts-test-key", config.DefaultJevModel)
	c.BaseURL = baseURL
	return c
}

func TestJevClassifier_RequestShape(t *testing.T) {
	f := newFakeJev(t, http.StatusOK, jevBody("0.91"), nil)

	got, err := newTestJev(f.srv.URL).Classify(context.Background(), "Arsenal bid for Hjulmand", "Arsenal have lodged a bid.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Probability != 0.91 || got.Model != "jev-1.13.0" {
		t.Errorf("unexpected classification: %+v", got)
	}

	if f.lastReq.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", f.lastReq.Method)
	}
	if auth := f.lastReq.Header.Get("Authorization"); auth != "Bearer ts-test-key" {
		t.Errorf("Authorization = %q, want Bearer ts-test-key", auth)
	}

	var req struct {
		State     map[string]string `json:"state"`
		Model     string            `json:"model"`
		Questions map[string]struct {
			Type         string `json:"type"`
			Instructions string `json:"instructions"`
			Criteria     struct {
				True  string `json:"true"`
				False string `json:"false"`
			} `json:"criteria"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(f.lastRaw, &req); err != nil {
		t.Fatalf("decode request: %v (%s)", err, f.lastRaw)
	}
	if req.Model != "jev-1.13.0" {
		t.Errorf("model = %q, want the pinned jev-1.13.0", req.Model)
	}
	if req.State["title"] != "Arsenal bid for Hjulmand" || req.State["body"] != "Arsenal have lodged a bid." {
		t.Errorf("state = %+v, want title and body as separate fields", req.State)
	}
	q, ok := req.Questions[jevQuestionID]
	if !ok || len(req.Questions) != 1 {
		t.Fatalf("questions = %+v, want exactly %q", req.Questions, jevQuestionID)
	}
	if q.Type != "noul" || q.Instructions == "" {
		t.Errorf("question = %+v, want a noul with instructions", q)
	}
	for _, boundary := range []string{"match reports", "new contract", "manager", "all but done", "collapsed"} {
		if !strings.Contains(q.Criteria.True+q.Criteria.False, boundary) {
			t.Errorf("criteria do not mention boundary case %q", boundary)
		}
	}
}

func TestJevClassifier_TruncatesLongBody(t *testing.T) {
	f := newFakeJev(t, http.StatusOK, jevBody("0.1"), nil)

	long := strings.Repeat("é", maxJevBodyBytes) // 2 bytes per rune, so twice the cap
	if _, err := newTestJev(f.srv.URL).Classify(context.Background(), "t", long); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var req struct {
		State map[string]string `json:"state"`
	}
	if err := json.Unmarshal(f.lastRaw, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	body := req.State["body"]
	if len(body) > maxJevBodyBytes || len(body) < maxJevBodyBytes-1 {
		t.Errorf("body is %d bytes, want just under %d", len(body), maxJevBodyBytes)
	}
	if strings.ContainsRune(body, '\uFFFD') {
		t.Error("truncation split a rune")
	}
}

func TestJevClassifier_Errors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		headers map[string]string
		wantErr string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"detail":"invalid key"}`, nil, "status 401"},
		{"validation", http.StatusUnprocessableEntity, `{"detail":"bad question"}`, nil, "bad question"},
		{"rate limited", http.StatusTooManyRequests, `{"detail":"slow down"}`, map[string]string{"Retry-After": "3"}, "retry-after 3"},
		{"server error", http.StatusInternalServerError, `oops`, nil, "status 500"},
		{"overloaded", 529, `{}`, nil, "status 529"},
		{"not json", http.StatusOK, `not json`, nil, "decode response"},
		{"missing answer", http.StatusOK, `{"model":"jev-1.13.0","answers":{}}`, nil, "no noul answer"},
		{"wrong answer type", http.StatusOK, `{"model":"jev-1.13.0","answers":{"is_transfer_rumour":{"type":"score","noul":0.9}}}`, nil, "no noul answer"},
		{"noul missing", http.StatusOK, `{"model":"jev-1.13.0","answers":{"is_transfer_rumour":{"type":"noul"}}}`, nil, "no noul answer"},
		{"noul out of range", http.StatusOK, jevBody("1.5"), nil, "out of range"},
		{"no model id", http.StatusOK, `{"answers":{"is_transfer_rumour":{"type":"noul","noul":0.9}}}`, nil, "no model id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeJev(t, tc.status, tc.body, tc.headers)
			_, err := newTestJev(f.srv.URL).Classify(context.Background(), "title", "body")
			if err == nil {
				t.Fatalf("expected an error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestJevClassifier_EmptyArticleIsAnErrorWithoutACall(t *testing.T) {
	f := newFakeJev(t, http.StatusOK, jevBody("0.9"), nil)
	if _, err := newTestJev(f.srv.URL).Classify(context.Background(), "  ", ""); err == nil {
		t.Fatal("expected an error for an empty article")
	}
	if n := f.calls.Load(); n != 0 {
		t.Errorf("Jev got %d calls, want 0", n)
	}
}
