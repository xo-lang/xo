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

func TestAtM(t *testing.T) {
	l := ListOf[int64](10, 20, 30)
	for _, i := range []int64{-1, 0, 2, 3, math.MinInt64, math.MaxInt64} {
		tv, tf := outcome(func() int64 { return l.AtM(i, "x.xo:3:4") })
		gv, gf := outcome(func() int64 { return l.At(i, "x.xo:3:4") })
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
