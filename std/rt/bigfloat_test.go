package rt

import (
	"math"
	"testing"
)

// TestBigFloatStickyPrec (task #434): an add or sub at precision 0 whose
// operands' exponents are far apart replaces the small operand with a
// stand-in bit (stickyBit), and the result takes the larger operand
// precision, as math/big's. The stand-in kept SetMantExp's 53 bits, so a
// 10 bit sum came out 53 and a 65 bit small operand beside a 10 bit large
// one came out 53, not 65 (eb76da415 to 2dedb948c).
func TestBigFloatStickyPrec(t *testing.T) {
	tiny := math.Ldexp(1, -200)
	cases := []struct {
		name               string
		bigP, smallP, want int64
		smallFirst         bool
	}{
		{"both 10", 10, 10, 10, false},
		{"small 65", 10, 65, 65, false},
		{"small first 65", 10, 65, 65, true},
		{"both 100", 100, 100, 100, false},
	}
	for _, c := range cases {
		b := Big_from_float(nil, 1.5, c.bigP)
		s := Big_from_float(nil, tiny, c.smallP)
		x, y := b, s
		if c.smallFirst {
			x, y = s, b
		}
		for op, r := range map[string]Big_BigFloat{
			"add": x.M_add(nil, y, 0),
			"sub": x.M_sub(nil, y, 0),
		} {
			if r.F_prec != c.want {
				t.Errorf("%s %s: precision %d, want %d", c.name, op, r.F_prec, c.want)
			}
		}
	}
}
