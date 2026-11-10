package main

import "testing"

// TestVetCoalesceOperand covers XO0542: a literal default followed by an
// operator that binds tighter than `??` (the owner's word count of
// 2026-11-10, `m.get(w) ?? 0 + 1`). Parenthesized forms and a default
// that is not a literal do not report.
func TestVetCoalesceOperand(t *testing.T) {
	src := `fn main(os: Os) {
  let m: Map[Str, Int] = {"a": 5}
  let a = m.get("w") ?? 0 + 1
  let b = m.get("w") ?? 1 * 2
  let c = m.get("w") ?? 0 - 1
  let d = (m.get("w") ?? 0) + 1
  let e = m.get("w") ?? (0 + 1)
  let k = 2
  let g = m.get("w") ?? k + 1
  os.stdio.println("${a} ${b} ${c} ${d} ${e} ${g}")
}
`
	got := vetCodes(t, src)
	want := []int{3, 4, 5}
	if len(got["XO0542"]) != len(want) || got["XO0542"][0] != 3 || got["XO0542"][1] != 4 || got["XO0542"][2] != 5 {
		t.Errorf("XO0542 at %v, want %v: %v", got["XO0542"], want, got)
	}
}
