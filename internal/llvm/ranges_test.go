package llvm_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestRangeFacts checks the IR of the golden range_facts (decision
// 0192): the checks value ranges prove are gone, the ones they do not
// prove stay, midpoints (and nothing else) are told to LLVM, and calls
// of inlinable functions are alwaysinline.
func TestRangeFacts(t *testing.T) {
	dir := filepath.Join("testdata", "golden", "range_facts")
	ir, err := xotool.LLVMIR(dir)
	if err != nil {
		t.Fatal(err)
	}
	def := func(name string) string {
		// `define ... @<name>(` to the closing brace
		m := regexp.MustCompile(`(?ms)^define [^\n]*@"?` + regexp.QuoteMeta(name) + `"?\(.*?^\}$`).Find(ir)
		if m == nil {
			t.Fatalf("no definition of %s", name)
		}
		return string(m)
	}
	for _, c := range []struct {
		fn                string
		checks            int  // calls of llvm.s*.with.overflow
		assume, inlineU24 bool // an llvm.assume; the call of u24 marked #6
	}{
		// the midpoint and the ends of the search: no check
		{"x.search", 0, true, true},
		// after b[i]: i + 1 to i + 3 cannot overflow; the sums of bytes
		// neither
		{"x.quad", 0, false, false},
		// i < n bounds i + 1, the remainder h * 31; h * 31 + i is not bounded
		{"x.mix", 1, false, false},
		// x in (-10, 0): x * x - x needs no check; x - 1 for x == -100 neither
		{"x.negs", 0, false, false},
		// x > 0 says nothing of k
		{"x.bump", 1, false, false},
	} {
		d := def(c.fn)
		if n := strings.Count(d, ".with.overflow.i64("); n != c.checks {
			t.Errorf("%s: %d overflow checks, want %d\n%s", c.fn, n, c.checks, d)
		}
		if got := strings.Contains(d, "@llvm.assume("); got != c.assume {
			t.Errorf("%s: llvm.assume %v, want %v\n%s", c.fn, got, c.assume, d)
		}
		if got := strings.Contains(d, `@"x.u24"(`) && strings.Contains(d, ") #6"); got != c.inlineU24 {
			t.Errorf("%s: alwaysinline call of u24 %v, want %v\n%s", c.fn, got, c.inlineU24, d)
		}
	}
}
