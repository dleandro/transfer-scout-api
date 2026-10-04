package extract

import "testing"

func TestResultUsable(t *testing.T) {
	const min = 0.4

	tests := []struct {
		name   string
		result Result
		want   bool
	}{
		{
			name:   "a confident rumour is stored",
			result: Result{IsTransferRumour: true, Confidence: 0.9},
			want:   true,
		},
		{
			name:   "exactly at the threshold is stored",
			result: Result{IsTransferRumour: true, Confidence: min},
			want:   true,
		},
		{
			name:   "a rumour below the threshold is not",
			result: Result{IsTransferRumour: true, Confidence: min - 0.01},
			want:   false,
		},
		{
			// The whole point. Under the old "confidence > 0" gate this was
			// stored as a real rumour; a model rating an ambiguous article
			// 0.99 does not make it a transfer story.
			name:   "a non-rumour is rejected no matter how confident",
			result: Result{IsTransferRumour: false, Confidence: 1},
			want:   false,
		},
		{
			name:   "the zero value is rejected",
			result: Result{},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.Usable(min); got != tt.want {
				t.Errorf("Usable(%v) = %v, want %v", min, got, tt.want)
			}
		})
	}
}

// A zero threshold must still not let non-rumours through: the boolean and
// the threshold guard different things, and one cannot be tuned into doing
// the other's job.
func TestResultUsable_ZeroThresholdStillExcludesNonRumours(t *testing.T) {
	if (Result{IsTransferRumour: false, Confidence: 0}).Usable(0) {
		t.Error("a non-rumour passed a zero threshold")
	}
	if !(Result{IsTransferRumour: true, Confidence: 0}).Usable(0) {
		t.Error("a rumour should pass a zero threshold")
	}
}
