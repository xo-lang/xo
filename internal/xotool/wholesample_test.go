package xotool

import "testing"

// TestWholeSample: the whole-program sample reads the amd64 sample's
// names, comments and blank lines skipped.
func TestWholeSample(t *testing.T) {
	s := wholeSample()
	if !s["arith"] || !s["overflow_add"] {
		t.Fatalf("whole sample lacks arith or overflow_add: %d names", len(s))
	}
	for n := range s {
		if n == "" || n[0] == '#' {
			t.Fatalf("whole sample holds %q", n)
		}
	}
	if !hasEnv([]string{"A=1", "XO_SEPCOMP=1"}, "XO_SEPCOMP") || hasEnv([]string{"XO_SEPCOMP_BG=0"}, "XO_SEPCOMP") {
		t.Fatal("hasEnv")
	}
}
