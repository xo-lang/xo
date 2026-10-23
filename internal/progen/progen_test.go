package progen

import "testing"

// TestProgramDeterministic: one seed is one program, in every run (the
// recorded Go outputs of the selfhost tests key on the text, decision
// 0178 stage 7; a map's order once made it vary).
func TestProgramDeterministic(t *testing.T) {
	for seed := int64(1); seed <= 60; seed++ {
		want := Program(seed)
		for range 5 {
			if got := Program(seed); got != want {
				t.Fatalf("seed %d: two different programs", seed)
			}
		}
	}
}

// TestScaleKnobs: every knob writes a program and its expected output,
// the same one each time, and a larger n writes a larger program.
func TestScaleKnobs(t *testing.T) {
	for _, k := range ScaleKnobs {
		a, b := Scale(k, 100), Scale(k, 200)
		if a != Scale(k, 100) || ScaleWant(k, 100) != ScaleWant(k, 100) {
			t.Fatalf("%s: two different programs for one size", k)
		}
		if len(b) <= len(a) {
			t.Fatalf("%s: n=200 is not larger than n=100", k)
		}
	}
}
