package rt

import (
	"math"
	"testing"
)

// convResult runs f and returns its value or its fault text.
func convResult[T Integer](f func() T) (v T, fault string) {
	defer func() {
		if r := recover(); r != nil {
			fault = AsFault(r).Error()
		}
	}()
	return f(), ""
}

// checkConv compares the non-generic helper with rt.Conv for x.
func checkConv[To Integer, From Integer](t *testing.T, x From, helper func() To) {
	t.Helper()
	wv, wf := convResult(func() To { return Conv[To, From](x, "m.xo:1:2") })
	gv, gf := convResult(helper)
	if wv != gv || wf != gf {
		var z To
		t.Errorf("%s(%v): helper (%v, %q), Conv (%v, %q)", typeName(z), x, gv, gf, wv, wf)
	}
}

// TestConvHelpersMatchConv: every helper in conv.go gives rt.Conv's value
// or fault text (message and location) at and around each target's
// bounds, from signed and unsigned sources.
func TestConvHelpersMatchConv(t *testing.T) {
	signed := []int64{math.MinInt64, math.MinInt32 - 1, math.MinInt32, math.MinInt16 - 1, math.MinInt16, -129, -128, -1, 0, 1, 127, 128, 255, 256, math.MaxInt16, math.MaxInt16 + 1, math.MaxUint16, math.MaxUint16 + 1, math.MaxInt32, math.MaxInt32 + 1, math.MaxUint32, math.MaxUint32 + 1, math.MaxInt64}
	for _, x := range signed {
		checkConv(t, x, func() int8 { return ConvS_int8(x, "m.xo:1:2") })
		checkConv(t, x, func() int16 { return ConvS_int16(x, "m.xo:1:2") })
		checkConv(t, x, func() int32 { return ConvS_int32(x, "m.xo:1:2") })
		checkConv(t, x, func() uint8 { return ConvS_uint8(x, "m.xo:1:2") })
		checkConv(t, x, func() uint16 { return ConvS_uint16(x, "m.xo:1:2") })
		checkConv(t, x, func() uint32 { return ConvS_uint32(x, "m.xo:1:2") })
		checkConv(t, x, func() uint64 { return ConvS_uint64(x, "m.xo:1:2") })
		if x >= math.MinInt32 && x <= math.MaxInt32 {
			x32 := int32(x)
			checkConv(t, x32, func() uint8 { return ConvS_uint8(int64(x32), "m.xo:1:2") })
		}
	}
	unsigned := []uint64{0, 1, 127, 128, 255, 256, math.MaxInt16, math.MaxInt16 + 1, math.MaxUint16, math.MaxUint16 + 1, math.MaxInt32, math.MaxInt32 + 1, math.MaxUint32, math.MaxUint32 + 1, math.MaxInt64, math.MaxInt64 + 1, math.MaxUint64}
	for _, x := range unsigned {
		checkConv(t, x, func() int8 { return ConvU_int8(x, "m.xo:1:2") })
		checkConv(t, x, func() int16 { return ConvU_int16(x, "m.xo:1:2") })
		checkConv(t, x, func() int32 { return ConvU_int32(x, "m.xo:1:2") })
		checkConv(t, x, func() int64 { return ConvU_int64(x, "m.xo:1:2") })
		checkConv(t, x, func() uint8 { return ConvU_uint8(x, "m.xo:1:2") })
		checkConv(t, x, func() uint16 { return ConvU_uint16(x, "m.xo:1:2") })
		checkConv(t, x, func() uint32 { return ConvU_uint32(x, "m.xo:1:2") })
	}
}

var convSink uint8

// BenchmarkConvGeneric and BenchmarkConvHelper: rt.Conv against the
// inlined helper (2026-10-21 on the Mac: 3.1 and 0.47 ns, median of 5).
func BenchmarkConvGeneric(b *testing.B) {
	var s uint8
	for i := 0; i < b.N; i++ {
		s += Conv[uint8, int64](int64(i&127), "m.xo:1:2")
	}
	convSink = s
}

func BenchmarkConvHelper(b *testing.B) {
	var s uint8
	for i := 0; i < b.N; i++ {
		s += ConvS_uint8(int64(i&127), "m.xo:1:2")
	}
	convSink = s
}
