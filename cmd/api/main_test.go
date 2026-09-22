package main

import (
	"strings"
	"testing"
)

func TestValidateAuthSecret(t *testing.T) {
	cases := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{name: "empty is an error", secret: "", wantErr: true},
		{name: "31 chars is below the floor", secret: strings.Repeat("a", 31), wantErr: true},
		{name: "32 chars is the floor", secret: strings.Repeat("a", 32), wantErr: false},
		{name: "44 chars (production secret length) passes", secret: strings.Repeat("a", 44), wantErr: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAuthSecret(tc.secret)
			if tc.wantErr && err == nil {
				t.Fatalf("validateAuthSecret(%q) = nil, want error", tc.secret)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateAuthSecret(%q) = %v, want nil", tc.secret, err)
			}
		})
	}
}
