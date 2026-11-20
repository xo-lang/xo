package main

import "testing"

// Decision 0726: the ratio is the median of per-round ratios, not the
// ratio of the medians; a bimodal host (slow and fast phases hitting both
// sides of a round) moves the medians apart but not the per-round ratios.
func TestRoundRatio(t *testing.T) {
	xs := []float64{10, 10, 20, 20, 20}
	gs := []float64{10, 10, 20, 20, 10}
	if got := roundRatio(xs, gs); got != 1 {
		t.Errorf("roundRatio = %v, want 1 (ratio of medians %v)", got, median(xs)/median(gs))
	}
	if got := median([]float64{1, 2, 3, 4}); got != 2.5 {
		t.Errorf("median of an even count = %v, want 2.5", got)
	}
}

func TestMoreRounds(t *testing.T) {
	for _, c := range []struct {
		name   string
		xs, gs []float64
		want   bool
	}{
		{"long Go side", []float64{12, 12, 12}, []float64{11, 11, 11}, false},
		{"short, under 1 s", []float64{2, 2}, []float64{1, 1}, true},
		{"short, Xo past 1 s, Go not", []float64{600, 600}, []float64{1, 1}, true},
		{"short, both past 1 s", rep(2, 600), rep(2, 600), false},
		{"capped", rep(1000, 0.1), rep(1000, 0.1), false},
	} {
		if got := moreRounds(c.xs, c.gs); got != c.want {
			t.Errorf("%s: moreRounds = %v, want %v", c.name, got, c.want)
		}
	}
}

func rep(n int, v float64) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = v
	}
	return s
}
