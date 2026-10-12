package arm64

import (
	"math"
	"math/bits"
	"math/rand"
	"testing"
)

// smulh is arm64's signed multiply high.
func smulh(a, b int64) int64 {
	hi, _ := bits.Mul64(uint64(a), uint64(b))
	h := int64(hi)
	if a < 0 {
		h -= b
	}
	if b < 0 {
		h -= a
	}
	return h
}

// magicQuot runs the instructions sdivQuot emits for a divisor d that is
// not a power of two (the sign fixup always on).
func magicQuot(x, d int64) int64 {
	mg, sh := sdivMagic(d)
	q := smulh(x, mg)
	if mg < 0 {
		q += x
	}
	q >>= sh
	return q + int64(uint64(q)>>63)
}

// TestSdivMagic checks the multiply high lowering of a division by a
// constant (decision 0194) against Go's division over edge dividends and
// random ones, for small divisors, the divisors of number text, random
// ones, and those near 2^62 and 2^63.
func TestSdivMagic(t *testing.T) {
	known := map[int64][2]int64{
		3:          {0x5555555555555556, 0},
		7:          {0x4924924924924925, 1},
		10:         {0x6666666666666667, 2},
		1000000000: {0x112e0be826d694b3, 26},
	}
	for d, w := range known {
		mg, sh := sdivMagic(d)
		if mg != w[0] || int64(sh) != w[1] {
			t.Errorf("sdivMagic(%d) = %#x, %d; want %#x, %d", d, mg, sh, w[0], w[1])
		}
	}
	rng := rand.New(rand.NewSource(20261012))
	var ds []int64
	for d := int64(3); d < 2000; d++ {
		ds = append(ds, d)
	}
	for _, d := range []int64{60, 100, 1000, 3600, 1e6, 1e9, 1e18, math.MaxInt64, math.MaxInt64 - 1, 1<<62 + 1, 1<<62 - 1, 1<<62 + 3, 1<<61 + 1, 1<<32 + 1, 641, 6700417} {
		ds = append(ds, d)
	}
	for i := 0; i < 2000; i++ {
		ds = append(ds, rng.Int63n(math.MaxInt64-2)+3)
		ds = append(ds, rng.Int63n(1<<uint(rng.Intn(60)+2))+3)
	}
	xs := []int64{0, 1, -1, 2, -2, math.MaxInt64, math.MinInt64, math.MaxInt64 - 1, math.MinInt64 + 1, 1 << 62, -(1 << 62)}
	for _, d := range ds {
		if d&(d-1) == 0 {
			continue
		}
		cand := append([]int64{d, -d, d - 1, -(d - 1), d + 1, -(d + 1), 2*d - 1, -(2*d - 1)}, xs...)
		for i := 0; i < 64; i++ {
			cand = append(cand, int64(rng.Uint64()), int64(rng.Uint64()>>uint(rng.Intn(64))))
		}
		// multiples of d and their neighbours, up to the extremes
		k := math.MaxInt64 / d
		for _, m := range []int64{k, k - 1, -k, -(k - 1)} {
			for _, o := range []int64{-1, 0, 1} {
				cand = append(cand, m*d+o)
			}
		}
		for _, x := range cand {
			if got, want := magicQuot(x, d), x/d; got != want {
				t.Fatalf("%d / %d: magic gives %d, want %d", x, d, got, want)
			}
		}
	}
}
