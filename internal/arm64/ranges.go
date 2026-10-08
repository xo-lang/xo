package arm64

import (
	"math"
	"math/bits"

	"github.com/xo-lang/xo/internal/ssa"
)

// Value ranges of Int SSA values (decision 0054), for the whole function:
// an interval per value, found by iterating over the blocks in reverse
// postorder until nothing changes; a loop parameter whose interval keeps
// growing is widened to the next threshold (the bounds a remainder or a
// mask gives, then the Int limits), so `seed = (seed * a + c) % 2^31`
// starting at 42 is known to stay in [0, 2^31 - 1].
//
// Every checked operation that would overflow faults, and an unchecked
// one was proved not to overflow, so an operation's interval is its
// exact mathematical interval clipped to Int. Branches do not narrow
// (the memo's bounds do that during selection).
//
// The intervals make the overflow checks of multiplications that cannot
// overflow go away, give the add and subtract checks (safeAdd) better
// bounds, and turn `x % 2^k` and `x / 2^k` of an x known not to be
// negative into a mask and a shift.

type interval struct{ lo, hi int64 }

var fullInterval = interval{math.MinInt64, math.MaxInt64}

func (r interval) union(o interval) interval { return interval{min(r.lo, o.lo), max(r.hi, o.hi)} }

// satAdd and satMul clip the exact result to Int.
func satAdd(a, b int64) int64 {
	s := a + b
	if a > 0 && b > 0 && s < 0 {
		return math.MaxInt64
	}
	if a < 0 && b < 0 && s >= 0 {
		return math.MinInt64
	}
	return s
}

// mulOK returns a * b and whether it fits in an Int.
func mulOK(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	neg := (a < 0) != (b < 0)
	ua, ub := uint64(a), uint64(b)
	if a < 0 {
		ua = -ua
	}
	if b < 0 {
		ub = -ub
	}
	hi, lo := bits.Mul64(ua, ub)
	if hi != 0 {
		return 0, false
	}
	if neg {
		if lo > 1<<63 {
			return 0, false
		}
		return int64(-lo), true
	}
	if lo > math.MaxInt64 {
		return 0, false
	}
	return int64(lo), true
}

func satMul(a, b int64) int64 {
	if p, ok := mulOK(a, b); ok {
		return p
	}
	if (a < 0) != (b < 0) {
		return math.MinInt64
	}
	return math.MaxInt64
}

// computeRanges fills s.ranges for f's Int values (blocks in reverse
// postorder).
func (s *fstate) computeRanges(blocks []*ssa.Block) {
	s.ranges = map[*ssa.Value]interval{}
	// thresholds for widening
	ups := []int64{0, math.MaxInt64}
	downs := []int64{0, math.MinInt64}
	for _, b := range blocks {
		for _, v := range b.Instrs {
			if (v.Op == ssa.OpRem || v.Op == ssa.OpAnd) && v.Type.Kind == ssa.KInt {
				if k, ok := constInt(v.Args[1]); ok && k != math.MinInt64 {
					if k < 0 {
						k = -k
					}
					if v.Op == ssa.OpRem && k > 0 {
						k--
					}
					ups = append(ups, k)
					downs = append(downs, -k)
				}
			}
		}
	}
	widenUp := func(x int64) int64 {
		best := int64(math.MaxInt64)
		for _, t := range ups {
			if t >= x && t < best {
				best = t
			}
		}
		return best
	}
	widenDown := func(x int64) int64 {
		best := int64(math.MinInt64)
		for _, t := range downs {
			if t <= x && t > best {
				best = t
			}
		}
		return best
	}
	grown := map[*ssa.Value]int{}
	get := func(v *ssa.Value) (interval, bool) {
		if n, ok := constInt(v); ok {
			return interval{n, n}, true
		}
		r, ok := s.ranges[v]
		return r, ok
	}
	for round := 0; round < 100; round++ {
		changed := false
		set := func(v *ssa.Value, r interval) {
			old, ok := s.ranges[v]
			if ok && old == r {
				return
			}
			if ok && isParam(v) {
				// a loop parameter that grows: widen after two rounds
				r = r.union(old)
				if r == old {
					return
				}
				grown[v]++
				if grown[v] > 2 {
					if r.hi > old.hi {
						r.hi = widenUp(r.hi)
					}
					if r.lo < old.lo {
						r.lo = widenDown(r.lo)
					}
				}
			}
			s.ranges[v] = r
			changed = true
		}
		for bi, b := range blocks {
			for k, p := range b.Params {
				if p.Type.Kind != ssa.KInt {
					continue
				}
				if bi == 0 {
					set(p, fullInterval)
					continue
				}
				var r interval
				any := false
				for _, e := range b.Preds {
					a, ok := get(e.Args[k])
					if !ok {
						continue // not reached yet: optimistic
					}
					if !any {
						r, any = a, true
					} else {
						r = r.union(a)
					}
				}
				if any {
					set(p, r)
				}
			}
			for _, v := range b.Instrs {
				if v.Type == nil || v.Type.Kind != ssa.KInt || v.Op == ssa.OpConst {
					continue
				}
				if r, ok := s.rangeOf(v, get); ok {
					set(v, r)
				}
			}
		}
		if !changed {
			return
		}
	}
	s.ranges = map[*ssa.Value]interval{} // did not settle: know nothing
}

// rangeOf computes the interval of v from its operands' (false while an
// operand has none yet).
func (s *fstate) rangeOf(v *ssa.Value, get func(*ssa.Value) (interval, bool)) (interval, bool) {
	arg := func(i int) (interval, bool) { return get(v.Args[i]) }
	switch v.Op {
	case ssa.OpAdd, ssa.OpSub, ssa.OpMul:
		a, ok1 := arg(0)
		b, ok2 := arg(1)
		if !ok1 || !ok2 {
			return interval{}, false
		}
		switch v.Op {
		case ssa.OpAdd:
			return interval{satAdd(a.lo, b.lo), satAdd(a.hi, b.hi)}, true
		case ssa.OpSub:
			if b.lo == math.MinInt64 {
				return fullInterval, true
			}
			return interval{satAdd(a.lo, -b.hi), satAdd(a.hi, -b.lo)}, true
		}
		c := []int64{satMul(a.lo, b.lo), satMul(a.lo, b.hi), satMul(a.hi, b.lo), satMul(a.hi, b.hi)}
		return interval{min(c[0], c[1], c[2], c[3]), max(c[0], c[1], c[2], c[3])}, true
	case ssa.OpDiv:
		a, ok := arg(0)
		k, isK := constInt(v.Args[1])
		if !ok {
			return interval{}, false
		}
		if !isK || k <= 0 {
			return fullInterval, true
		}
		return interval{a.lo / k, a.hi / k}, true
	case ssa.OpRem:
		a, ok := arg(0)
		if !ok {
			return interval{}, false
		}
		k, isK := constInt(v.Args[1])
		if !isK || k == 0 || k == math.MinInt64 {
			return fullInterval, true
		}
		if k < 0 {
			k = -k
		}
		m := k - 1
		switch {
		case a.lo >= 0:
			return interval{0, min(a.hi, m)}, true
		case a.hi <= 0:
			return interval{max(a.lo, -m), 0}, true
		}
		return interval{max(a.lo, -m), min(a.hi, m)}, true
	case ssa.OpAnd:
		a, ok1 := arg(0)
		b, ok2 := arg(1)
		if !ok1 || !ok2 {
			return interval{}, false
		}
		switch {
		case a.lo >= 0 && b.lo >= 0:
			return interval{0, min(a.hi, b.hi)}, true
		case a.lo >= 0:
			return interval{0, a.hi}, true
		case b.lo >= 0:
			return interval{0, b.hi}, true
		}
		return fullInterval, true
	case ssa.OpShr:
		a, ok := arg(0)
		k, isK := constInt(v.Args[1])
		if !ok {
			return interval{}, false
		}
		if !isK || k < 0 || k > 63 {
			return fullInterval, true
		}
		return interval{a.lo >> k, a.hi >> k}, true
	case ssa.OpNeg:
		a, ok := arg(0)
		if !ok {
			return interval{}, false
		}
		if a.lo == math.MinInt64 {
			return interval{-a.hi, math.MaxInt64}, true
		}
		return interval{-a.hi, -a.lo}, true
	case ssa.OpCall:
		switch v.Aux {
		case "list.len", "Str.len":
			return interval{0, math.MaxInt64}, true
		}
	}
	return fullInterval, true
}

// ssaRange returns the interval of Int value v.
func (s *fstate) ssaRange(v *ssa.Value) interval {
	if r, ok := s.ranges[v]; ok {
		return r
	}
	return fullInterval
}

// safeMul reports whether the multiplication v cannot overflow.
func (s *fstate) safeMul(v *ssa.Value) bool {
	alo, ahi := s.bounds(v.Args[0])
	blo, bhi := s.bounds(v.Args[1])
	for _, x := range []int64{alo, ahi} {
		for _, y := range []int64{blo, bhi} {
			if _, ok := mulOK(x, y); !ok {
				return false
			}
		}
	}
	return true
}
