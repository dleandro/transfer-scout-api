package models

import "testing"

func TestRumourStatus_IsTerminal(t *testing.T) {
	cases := map[RumourStatus]bool{
		StatusRumoured:  false,
		StatusTalks:     false,
		StatusAdvanced:  false,
		StatusMedical:   false,
		StatusConfirmed: true,
		StatusCollapsed: true,
	}
	for status, want := range cases {
		if got := status.IsTerminal(); got != want {
			t.Errorf("%s.IsTerminal() = %v, want %v", status, got, want)
		}
	}
}

func TestRumourStatus_IsForwardTransition(t *testing.T) {
	tests := []struct {
		from, to RumourStatus
		want     bool
	}{
		{StatusRumoured, StatusTalks, true},
		{StatusTalks, StatusAdvanced, true},
		{StatusRumoured, StatusMedical, true},
		{StatusAdvanced, StatusTalks, false},
		{StatusMedical, StatusRumoured, false},
		{StatusRumoured, StatusRumoured, true},
		{StatusTalks, StatusConfirmed, true},
		{StatusRumoured, StatusCollapsed, true},
		{StatusConfirmed, StatusTalks, false},
	}
	for _, tt := range tests {
		if got := tt.from.IsForwardTransition(tt.to); got != tt.want {
			t.Errorf("%s.IsForwardTransition(%s) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}
