package store

import "testing"

func TestCrestURLFor(t *testing.T) {
	tests := []struct {
		name  string
		input string
		// want is the expected URL; "" means crestURLFor should return nil.
		want string
	}{
		{"exact canonical name", "Arsenal", "https://upload.wikimedia.org/wikipedia/en/5/53/Arsenal_FC.svg"},
		{"case-insensitive match", "chelsea", "https://upload.wikimedia.org/wikipedia/en/c/cc/Chelsea_FC.svg"},
		{"surrounding whitespace trimmed", "  Liverpool  ", "https://upload.wikimedia.org/wikipedia/en/0/0c/Liverpool_FC.svg"},
		{"multi-word name with ampersand", "Brighton & Hove Albion", "https://upload.wikimedia.org/wikipedia/en/d/d0/Brighton_and_Hove_Albion_FC_crest.svg"},
		{"unmapped club is nil", "Real Madrid", ""},
		{"empty string is nil", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := crestURLFor(tt.input)
			if tt.want == "" {
				if got != nil {
					t.Errorf("crestURLFor(%q) = %q, want nil", tt.input, *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("crestURLFor(%q) = nil, want %q", tt.input, tt.want)
			}
			if *got != tt.want {
				t.Errorf("crestURLFor(%q) = %q, want %q", tt.input, *got, tt.want)
			}
		})
	}
}
