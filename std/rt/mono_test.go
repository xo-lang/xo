package rt

import (
	"math"
	"math/rand"
	"testing"
)

// outcome runs f and returns its result or the fault message.
func outcome(f func() int64) (v int64, fault string) {
	defer func() {
		if r := recover(); r != nil {
			if x, ok := r.(*arithFault); ok {
				r = x.fault()
			}
			fe, ok := r.(*FaultErr)
			if !ok {
				panic(r)
			}
			fault = fe.Msg + " at " + fe.Loc
		}
	}()
	return f(), ""
}

// TestTypedIntArith checks that the typed Int helpers emitted by
// `xo build --mono` agree with the generic checked operations: same
// results, same faults, same messages.
func TestTypedIntArith(t *testing.T) {
	t.Parallel() // local values only
	edge := []int64{0, 1, -1, 2, -2, 3, 7, -7, 1 << 31, -(1 << 31), 1<<31 - 1, -(1 << 31) - 1,
		1 << 32, 1<<32 + 1, -(1 << 32), 3037000499, 3037000500, -3037000500,
		math.MaxInt64, math.MinInt64, math.MaxInt64 - 1, math.MinInt64 + 1, math.MaxInt32, math.MinInt32}
	r := rand.New(rand.NewSource(1))
	vals := append([]int64{}, edge...)
	for i := 0; i < 200; i++ {
		x, y := r.Int63(), r.Int63()
		vals = append(vals, x-y, int64(int32(x)), int64(int32(y))-int64(int32(x>>32)), r.Int63n(1<<33)-(1<<32))
	}
	ops := []struct {
		name  string
		typed func(a, b int64, at string) int64
		gen   func(a, b int64, at string) int64
	}{
		{"+", AddI, AddS[int64]},
		{"-", SubI, SubS[int64]},
		{"*", MulI, MulS[int64]},
		{"/", DivI, DivS[int64]},
		{"%", RemI, RemS[int64]},
	}
	for _, op := range ops {
		for _, a := range vals {
			for _, b := range vals {
				tv, tf := outcome(func() int64 { return op.typed(a, b, "x.xo:1:2") })
				gv, gf := outcome(func() int64 { return op.gen(a, b, "x.xo:1:2") })
				if tv != gv || tf != gf {
					t.Fatalf("%d %s %d: typed (%d, %q), generic (%d, %q)", a, op.name, b, tv, tf, gv, gf)
				}
			}
		}
	}
}

// TestConstIntArith checks the constant operand forms against the generic
// checked operations on every value of TestTypedIntArith's kind and every
// constant the generator gives each form.
func TestConstIntArith(t *testing.T) {
	t.Parallel() // local values only
	ks := []int64{1, 2, 3, 7, 1000000, 1103515245, 1 << 31, 1<<32 + 1, 3037000499, 3037000500, math.MaxInt64 / 2, math.MaxInt64 - 1, math.MaxInt64}
	var all []int64
	for _, k := range ks {
		all = append(all, k, -k)
	}
	all = append(all, math.MinInt64, math.MinInt64+1)
	r := rand.New(rand.NewSource(2))
	vals := []int64{0, 1, -1, 2, -2, math.MaxInt64, math.MinInt64, math.MaxInt64 - 1, math.MinInt64 + 1, 1 << 31, -(1 << 31), 1 << 32, 3037000499, -3037000500}
	for _, k := range all {
		if k != 0 {
			vals = append(vals, math.MaxInt64/k, math.MaxInt64/k+1, math.MaxInt64/k-1, math.MinInt64/k, math.MinInt64/k+1, math.MinInt64/k-1, k, -k, math.MaxInt64-k, math.MinInt64-k)
		}
	}
	for i := 0; i < 300; i++ {
		vals = append(vals, r.Int63()-r.Int63(), r.Int63n(1<<34)-(1<<33))
	}
	mulBounds := func(k int64) (int64, int64) {
		a, b := int64(math.MaxInt64)/k, int64(math.MinInt64)/k
		return min(a, b), max(a, b)
	}
	const at = "x.xo:5:6"
	for _, k := range all {
		for _, a := range vals {
			check := func(name string, got, want func() int64) {
				gv, gf := outcome(got)
				wv, wf := outcome(want)
				if gv != wv || gf != wf {
					t.Fatalf("%s a=%d k=%d: got (%d, %q), want (%d, %q)", name, a, k, gv, gf, wv, wf)
				}
			}
			if k > 0 {
				check("AddIUp", func() int64 { return AddIUp(a, k, at) }, func() int64 { return AddS(a, k, at) })
				check("SubIDown", func() int64 { return SubIDown(a, k, at) }, func() int64 { return SubS(a, k, at) })
			}
			if k < 0 {
				check("AddIDown", func() int64 { return AddIDown(a, k, at) }, func() int64 { return AddS(a, k, at) })
				check("SubIUp", func() int64 { return SubIUp(a, k, at) }, func() int64 { return SubS(a, k, at) })
			}
			if k != -1 && k != 0 && k != 1 {
				lo, hi := mulBounds(k)
				check("MulIK", func() int64 { return MulIK(a, k, lo, hi, at) }, func() int64 { return MulS(a, k, at) })
				check("MulKI", func() int64 { return MulKI(k, a, lo, hi, at) }, func() int64 { return MulS(k, a, at) })
			}
		}
	}
}

func TestAtM(t *testing.T) {
	l := ListOf[int64](10, 20, 30)
	for _, i := range []int64{-1, 0, 2, 3, math.MinInt64, math.MaxInt64} {
		tv, tf := outcome(func() int64 { return List_AtM(l, i, "x.xo:3:4") })
		gv, gf := outcome(func() int64 { return List_At(l, i, "x.xo:3:4") })
		if tv != gv || tf != gf {
			t.Fatalf("index %d: AtM (%d, %q), At (%d, %q)", i, tv, tf, gv, gf)
		}
	}
}

func TestCmpO(t *testing.T) {
	for _, c := range [][2]int64{{1, 2}, {2, 1}, {3, 3}, {math.MinInt64, math.MaxInt64}} {
		if CmpO(c[0], c[1]) != Cmp(c[0], c[1]) {
			t.Fatalf("CmpO(%d, %d)", c[0], c[1])
		}
	}
	for _, c := range [][2]string{{"a", "b"}, {"b", "a"}, {"é", "e"}, {"", ""}} {
		if CmpO(c[0], c[1]) != Cmp(c[0], c[1]) {
			t.Fatalf("CmpO(%q, %q)", c[0], c[1])
		}
	}
}
