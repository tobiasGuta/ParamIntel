package confidence

import "testing"

func TestScoreRewardsReproducibleCandidateAndNegativeControl(t *testing.T) {
	high := Score(3, 3, 0, 3, 2)
	low := Score(3, 3, 3, 3, 2)
	if high < .95 {
		t.Fatalf("high score=%f", high)
	}
	if low >= .30 {
		t.Fatalf("random-control-equivalent behavior should score very low: %f", low)
	}
}

func TestScorePartialReproducibility(t *testing.T) {
	s := Score(2, 3, 0, 3, 1)
	if s <= .50 || s >= .90 {
		t.Fatalf("unexpected partial score=%f", s)
	}
}

func TestLabel(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{0.95, "high"},
		{0.60, "medium"},
		{0.59, "low"},
	}
	for _, tc := range cases {
		if got := Label(tc.score); got != tc.want {
			t.Fatalf("Label(%v)=%q want %q", tc.score, got, tc.want)
		}
	}
}
