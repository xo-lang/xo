package arm64

import (
	"strconv"
	"strings"
	"testing"
)

// TestFarBranches checks the rewrite of the tests and conditional
// branches of huge functions (farTests): tbz and tbnz past the test
// limit, b.cond, cbz, and cbnz past the condition limit (1 MiB of code:
// the self hosted SSA builder's std_call, built with --backend=arm64,
// failed to assemble with `b.hs Lf852_fault1` out of range), each as the
// opposite test around an unconditional branch; a shorter function is
// unchanged.
func TestFarBranches(t *testing.T) {
	body := "\ttbnz x9, #63, La\n\tb.hs Lf1_fault1\n\tcbz x3, Lb\n\tb.ne Lc\n\tb Ld\nLa:\n\tret"
	label := func(k int) string { return "Lfar" + strconv.Itoa(k) }
	if got := farBranches(body, 100, 200, label); got != body {
		t.Fatalf("a short function changed:\n%s", got)
	}
	pad := strings.Repeat("\n\tnop", 150)
	got := farBranches(body+pad, 100, 200, label)
	if !strings.Contains(got, "\ttbz x9, #63, Lfar1\n\tb La\nLfar1:") {
		t.Errorf("tbnz past the test limit not rewritten:\n%s", got[:200])
	}
	if !strings.Contains(got, "\tb.hs Lf1_fault1\n") || !strings.Contains(got, "\tcbz x3, Lb\n") {
		t.Errorf("a conditional branch under the condition limit was rewritten:\n%s", got[:200])
	}
	pad = strings.Repeat("\n\tnop", 250)
	got = farBranches(body+pad, 100, 200, label)
	for _, want := range []string{
		"\ttbz x9, #63, Lfar1\n\tb La\nLfar1:",
		"\tb.lo Lfar2\n\tb Lf1_fault1\nLfar2:",
		"\tcbnz x3, Lfar3\n\tb Lb\nLfar3:",
		"\tb.eq Lfar4\n\tb Lc\nLfar4:",
		"\tb Ld\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got[:300])
		}
	}
}
