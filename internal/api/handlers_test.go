package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseIntParam(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		def     int
		want    int
		wantErr bool
	}{
		{name: "absent param falls back to default", query: "", def: 50, want: 50},
		{name: "valid integer is parsed", query: "limit=25", def: 50, want: 25},
		{name: "zero is a valid integer (range clamping is the caller's job)", query: "limit=0", def: 50, want: 0},
		{name: "negative is a valid integer (range clamping is the caller's job)", query: "limit=-5", def: 50, want: -5},
		{name: "non-integer is an error", query: "limit=abc", def: 50, wantErr: true},
		{name: "explicit empty value is treated the same as absent", query: "limit=", def: 50, want: 50},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/rumours?"+tc.query, nil)
			got, err := parseIntParam(r, "limit", tc.def)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got none (value %d)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}
