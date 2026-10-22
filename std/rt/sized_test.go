package rt

import (
	"math"
	"testing"
)

// run calls f and returns its value and the fault message ("" for none).
func runFault[T any](f func() T) (v T, msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = AsFault(r).Msg
		}
	}()
	return f(), ""
}

type sizedOps[T Integer] struct {
	add, sub, mul, div, rem      func(a, b T, at string) T
	shl, shr                     func(a T, n int64, at string) T
	gadd, gsub, gmul, gdiv, grem func(a, b T, at string) T
}

func signedOps[T Signed](add, sub, mul, div, rem func(a, b T, at string) T, shl, shr func(a T, n int64, at string) T) sizedOps[T] {
	return sizedOps[T]{add, sub, mul, div, rem, shl, shr, AddS[T], SubS[T], MulS[T], DivS[T], RemS[T]}
}

func unsignedOps[T Unsigned](add, sub, mul, div, rem func(a, b T, at string) T, shl, shr func(a T, n int64, at string) T) sizedOps[T] {
	return sizedOps[T]{add, sub, mul, div, rem, shl, shr, AddU[T], SubU[T], MulU[T], DivU[T], RemU[T]}
}

func checkSized[T Integer](t *testing.T, name string, ops sizedOps[T], vals []T) {
	t.Helper()
	type bin struct {
		op   string
		h, g func(a, b T, at string) T
	}
	bins := []bin{{"+", ops.add, ops.gadd}, {"-", ops.sub, ops.gsub}, {"*", ops.mul, ops.gmul},
		{"/", ops.div, ops.gdiv}, {"%", ops.rem, ops.grem}}
	for _, a := range vals {
		for _, b := range vals {
			for _, o := range bins {
				hv, hm := runFault(func() T { return o.h(a, b, "m.xo:1:2") })
				gv, gm := runFault(func() T { return o.g(a, b, "m.xo:1:2") })
				if hv != gv || hm != gm {
					t.Errorf("%s %d %s %d: helper (%d, %q), generic (%d, %q)", name, a, o.op, b, hv, hm, gv, gm)
				}
			}
		}
		for _, n := range []int64{-1, 0, 1, 3, 7, 8, 15, 16, 31, 32, 63, 64, 100} {
			hv, hm := runFault(func() T { return ops.shl(a, n, "m.xo:1:2") })
			gv, gm := runFault(func() T { return Shl(a, n, "m.xo:1:2") })
			if hv != gv || hm != gm {
				t.Errorf("%s %d << %d: helper (%d, %q), generic (%d, %q)", name, a, n, hv, hm, gv, gm)
			}
			hv, hm = runFault(func() T { return ops.shr(a, n, "m.xo:1:2") })
			gv, gm = runFault(func() T { return Shr(a, n, "m.xo:1:2") })
			if hv != gv || hm != gm {
				t.Errorf("%s %d >> %d: helper (%d, %q), generic (%d, %q)", name, a, n, hv, hm, gv, gm)
			}
		}
	}
}

// TestSizedHelpersMatchGeneric: the non-generic helpers (sized.go) give
// the generic functions' values and fault text at every edge value.
func TestSizedHelpersMatchGeneric(t *testing.T) {
	checkSized(t, "int8", signedOps[int8](Add_int8, Sub_int8, Mul_int8, Div_int8, Rem_int8, Shl_int8, Shr_int8),
		[]int8{math.MinInt8, math.MinInt8 + 1, -64, -2, -1, 0, 1, 2, 7, 11, 63, 64, math.MaxInt8 - 1, math.MaxInt8})
	checkSized(t, "int16", signedOps[int16](Add_int16, Sub_int16, Mul_int16, Div_int16, Rem_int16, Shl_int16, Shr_int16),
		[]int16{math.MinInt16, math.MinInt16 + 1, -181, -2, -1, 0, 1, 2, 7, 181, 255, 256, math.MaxInt16 - 1, math.MaxInt16})
	checkSized(t, "int32", signedOps[int32](Add_int32, Sub_int32, Mul_int32, Div_int32, Rem_int32, Shl_int32, Shr_int32),
		[]int32{math.MinInt32, math.MinInt32 + 1, -46341, -2, -1, 0, 1, 2, 7, 46340, 46341, 65536, math.MaxInt32 - 1, math.MaxInt32})
	checkSized(t, "uint8", unsignedOps[uint8](Add_uint8, Sub_uint8, Mul_uint8, Div_uint8, Rem_uint8, Shl_uint8, Shr_uint8),
		[]uint8{0, 1, 2, 7, 15, 16, 127, 128, 200, 254, 255})
	checkSized(t, "uint16", unsignedOps[uint16](Add_uint16, Sub_uint16, Mul_uint16, Div_uint16, Rem_uint16, Shl_uint16, Shr_uint16),
		[]uint16{0, 1, 2, 7, 255, 256, 257, 32767, 32768, 65534, 65535})
	checkSized(t, "uint32", unsignedOps[uint32](Add_uint32, Sub_uint32, Mul_uint32, Div_uint32, Rem_uint32, Shl_uint32, Shr_uint32),
		[]uint32{0, 1, 2, 7, 65535, 65536, 65537, 1 << 31, 1<<31 + 1, math.MaxUint32 - 1, math.MaxUint32})
	checkSized(t, "uint64", unsignedOps[uint64](Add_uint64, Sub_uint64, Mul_uint64, Div_uint64, Rem_uint64, Shl_uint64, Shr_uint64),
		[]uint64{0, 1, 2, 7, 1 << 32, 1<<32 + 1, math.MaxUint32, 1 << 63, 1<<63 + 1, math.MaxUint64 - 1, math.MaxUint64})
}

var sizedSink uint16

func BenchmarkSizedGeneric(b *testing.B) {
	var s uint16
	for i := 0; i < b.N; i++ {
		s = RemU(MulU(s, 13, "m.xo:1:2"), 251, "m.xo:1:2") + uint16(i&7)
	}
	sizedSink = s
}

func BenchmarkSizedHelper(b *testing.B) {
	var s uint16
	for i := 0; i < b.N; i++ {
		s = Rem_uint16(Mul_uint16(s, 13, "m.xo:1:2"), 251, "m.xo:1:2") + uint16(i&7)
	}
	sizedSink = s
}
