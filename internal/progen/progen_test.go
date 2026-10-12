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
