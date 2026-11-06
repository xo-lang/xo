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

// TestTierSepcomp: XO_TEST_SEPCOMP=0 sets XO_SEPCOMP=0 unless set.
func TestTierSepcomp(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if k, v, ok := tierSepcomp(env(map[string]string{"XO_TEST_SEPCOMP": "0"})); !ok || k != "XO_SEPCOMP" || v != "0" {
		t.Fatalf("XO_TEST_SEPCOMP=0: %q %q %v", k, v, ok)
	}
	if _, _, ok := tierSepcomp(env(map[string]string{"XO_TEST_SEPCOMP": "0", "XO_SEPCOMP": "1"})); ok {
		t.Fatal("XO_SEPCOMP=1 overridden")
	}
	if _, _, ok := tierSepcomp(env(map[string]string{})); ok {
		t.Fatal("set without XO_TEST_SEPCOMP")
	}
}
