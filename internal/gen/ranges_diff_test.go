package gen_test

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// TestRangeDifferential generates programs of loops, comparisons, and
// arithmetic near the Int bounds, runs them with the range analysis and
// with every operation checked (build.Options.NoRanges), and requires the
// same output: the same values, and the same faults with the same values
// and positions. An elided check that could fail would show up as a
// missing fault or a wrapped value.
//
// XO_RANGE_SEEDS=N runs N more seeds (slow; for exploring).
func TestRangeDifferential(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two programs")
	}
	seeds := []int64{20261008}
	if n, err := strconv.Atoi(os.Getenv("XO_RANGE_SEEDS")); err == nil {
		for i := 1; i <= n; i++ {
			seeds = append(seeds, int64(i))
		}
	}
	for _, seed := range seeds {
		t.Run(fmt.Sprint(seed), func(t *testing.T) { rangeDifferential(t, seed) })
	}
}

func rangeDifferential(t *testing.T, seed int64) {
	r := rand.New(rand.NewSource(seed))
	near := func() int64 {
		switch r.Intn(6) {
		case 0:
			return math.MaxInt64 - r.Int63n(40)
		case 1:
			return math.MinInt64 + r.Int63n(40)
		case 2:
			return r.Int63n(200) - 100
		case 3:
			return (1 << 31) - r.Int63n(5)
		case 4:
			return 3037000499 + r.Int63n(3) - 1
		}
		return r.Int63() >> uint(r.Intn(63))
	}
	lit := func(v int64) string {
		if v == math.MinInt64 {
			return "(-9223372036854775807 - 1)"
		}
		if v < 0 {
			return fmt.Sprintf("(%d)", v)
		}
		return fmt.Sprintf("%d", v)
	}
	ops := []string{"+", "-", "*", "/", "%"}
	op := func() string { return ops[r.Intn(len(ops))] }
	var b strings.Builder
	b.WriteString("fn bsearch(xs: List[Int], x: Int) -> Int {\n  var lo = 0\n  var hi = xs.len()\n  while lo < hi {\n    let mid = (lo + hi) / 2\n    if xs[mid] < x {\n      lo = mid + 1\n    } else {\n      hi = mid\n    }\n  }\n  lo\n}\n\n")
	for n := 0; n < 120; n++ {
		fmt.Fprintf(&b, "test \"t%d\" {\n", n)
		switch n % 6 {
		case 0: // range loop with an accumulator
			lo := r.Int63n(20) - 10
			hi := lo + r.Int63n(15)
			incl := ".."
			if r.Intn(2) == 0 {
				incl = "..="
			}
			fmt.Fprintf(&b, "  var s = %s\n  for i in %s%s%s {\n    s = (i %s %s) %s %s\n  }\n  expect(s) == 0\n",
				lit(near()), lit(lo), incl, lit(hi), op(), lit(near()), op(), lit(r.Int63n(7)+1))
		case 1: // while with a bound and a step
			fmt.Fprintf(&b, "  var x = %s\n  var c = 0\n  while x < %s && c < 50 {\n    x = x + %s\n    c += 1\n  }\n  expect(x %s %s) == 0\n",
				lit(near()), lit(near()), lit(r.Int63n(1<<40)+1), op(), lit(near()))
		case 2: // guarded operations
			k := r.Int63n(3037000500)
			fmt.Fprintf(&b, "  let x = %s\n  var y = 0\n  if x < %d && x > -%d {\n    y = x * x\n  } else {\n    y = x %s %s\n  }\n  expect(y) == 0\n",
				lit(near()), k, k, op(), lit(near()))
		case 3: // index loops around the length
			d := []string{"", " - 1", " + 1"}[r.Intn(3)]
			fmt.Fprintf(&b, "  let xs = [%s, %s, %s]\n  var s = 0\n  for i in 0..xs.len()%s {\n    s = s %s xs[i]\n  }\n  expect(s) == 0\n",
				lit(near()), lit(near()), lit(near()), d, op())
		case 4: // binary search, then arithmetic on the result
			fmt.Fprintf(&b, "  let xs = [1, 4, 9, 16, 25, 36, 49]\n  let i = bsearch(xs, %d)\n  expect(i %s %s) == 0\n",
				r.Int63n(60), op(), lit(near()))
		case 5: // remainders, masks, and constants
			fmt.Fprintf(&b, "  let x = %s\n  let m = x %% %s\n  expect((m %s %s) + (x & %d)) == 0\n",
				lit(near()), lit(r.Int63n(1<<20)+1), op(), lit(near()), r.Int63n(1<<16))
		}
		b.WriteString("}\n\n")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	with, _ := buildAndRunOpts(t, dir, build.Options{Mode: gen.Test})
	without, _ := buildAndRunOpts(t, dir, build.Options{Mode: gen.Test, NoRanges: true})
	ws, wos := dropGoFrames(with.String()), dropGoFrames(without.String())
	if ws != wos {
		t.Fatalf("outputs differ\n--- with range analysis\n%s\n--- every operation checked\n%s", ws, wos)
	}
	if !strings.Contains(ws, "fault: integer overflow") {
		t.Fatalf("the generated cases should include faults:\n%s", ws)
	}
}
