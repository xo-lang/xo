package llvm

import (
	"fmt"
	"math"
	"math/bits"

	"github.com/xo-lang/xo/internal/ssa"
)

// Value ranges of Int SSA values (decision 0192), the LLVM twin of the
// arm64 backend's (internal/arm64 ranges.go, decisions 0054 and 0184):
// an interval per value for the whole function, found by iterating over
// the blocks in reverse postorder until nothing changes; a loop
// parameter whose interval keeps growing is widened to the next
// threshold (the bounds a remainder or a mask gives, then the Int
// limits). A midpoint m = a + (b - a) / 2 computed where a < b holds is
// at least a and below b, so a binary search's ends stay in the table.
//
// Every checked operation that would overflow faults, and an unchecked
// one was proved not to overflow (a wrapping one, ssa.Wrap, knows
// nothing), so an operation's interval is its exact mathematical
// interval clipped to Int.
//
// LLVM cannot find these itself: its value tracking does not iterate
// over loop phis, and it knows nothing of the relation a < b that bounds
// a midpoint. The lowering uses them twice:
//
//   - a checked add, subtract, or multiply whose operands' intervals
//     (narrowed by the facts that dominate it: branch comparisons, also
//     through `&&` joins, and indexes taken) prove it cannot overflow is
//     lowered without the check (`nsw`);
//   - the interval of each midpoint is told to LLVM (llvm.assume), so
//     the bounds checks of the table reads inlined into the search
//     (std/native/llrt/unicode.xo's u24) fold away after inlining. Loop
//     parameters are not told: an assume on a loop phi let LLVM compute
//     trip counts in 128 bits and unroll the sieve's inner loop around a
//     division call (1.6 times the instructions; gotchas
//     llvm-loop-phi-ranges.md).

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

// addOK returns a + b and whether it fits in an Int.
func addOK(a, b int64) (int64, bool) {
	s := a + b
	return s, !(a > 0 && b > 0 && s < 0) && !(a < 0 && b < 0 && s >= 0)
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

// maxStrLen bounds the length of a Str or Bytes value (decision 0184):
// bytes in memory, an address space of 2^48.
const maxStrLen = 1 << 48

// ssaConst reports the value of an Int constant.
func ssaConst(v *ssa.Value) (int64, bool) {
	if v.Op != ssa.OpConst {
		return 0, false
	}
	n, ok := v.Aux.(int64)
	return n, ok
}

// rangeInfo is the result of the analysis of one function.
type rangeInfo struct {
	ranges map[*ssa.Value]interval
	mids   map[*ssa.Value]bool
	// safe marks the checked adds, subtracts, and multiplies that cannot
	// overflow
	safe map[*ssa.Value]bool
	// facts holds what is known on entry to each block
	facts map[*ssa.Block]*factList
}

// A fact holds where a block is entered: x op y (a comparison of Ints
// on the edge of a branch that dominates it), or, when op is 0, x is an
// index below lim that a list or Bytes access took (a passed bounds
// check, or one the builder proved).
type fact struct {
	x, y *ssa.Value
	op   ssa.Op
	lim  int64
}

// factList is a persistent list of facts: a block's facts are its own
// followed by its immediate dominator's.
type factList struct {
	f    fact
	next *factList
}

// maxFactWalk bounds how many facts narrowing an operand looks at (the
// nearest ones), so the analysis stays linear in big functions.
const maxFactWalk = 32

// indexFact returns the fact of a value that indexes a list or Bytes.
func indexFact(v *ssa.Value) (fact, bool) {
	if v.Op != ssa.OpCall {
		return fact{}, false
	}
	switch v.Aux {
	case "bytes.index":
		return fact{x: v.Args[1], lim: maxStrLen}, true
	case "list.index", "list.take":
		return fact{x: v.Args[1], lim: math.MaxInt64}, true
	}
	return fact{}, false
}

// negated is the comparison that holds when op does not.
func negated(op ssa.Op) ssa.Op {
	switch op {
	case ssa.OpLt:
		return ssa.OpGe
	case ssa.OpLe:
		return ssa.OpGt
	case ssa.OpGt:
		return ssa.OpLe
	case ssa.OpGe:
		return ssa.OpLt
	case ssa.OpEq:
		return ssa.OpNe
	}
	return ssa.OpEq
}

// swapped is op with its operands exchanged.
func swapped(op ssa.Op) ssa.Op {
	switch op {
	case ssa.OpLt:
		return ssa.OpGt
	case ssa.OpLe:
		return ssa.OpGe
	case ssa.OpGt:
		return ssa.OpLt
	case ssa.OpGe:
		return ssa.OpLe
	}
	return op
}

// entryFacts returns the facts on entry to block b: the comparison of
// the branch whose edge is its only way in, then base (the facts on
// leaving its immediate dominator). A branch on a Bool parameter of its
// block (the join of `a && b` or `a || b`) whose edge only one
// predecessor of that block can take (the others pass the constant
// that leaves by the other edge) gives that predecessor's facts on
// leaving it, and the comparison it passes (decision 0184's
// joinSource); exit holds the facts on leaving the blocks before b.
func entryFacts(b *ssa.Block, base *factList, exit map[*ssa.Block]*factList) *factList {
	if len(b.Preds) != 1 {
		return base
	}
	j := b.Preds[0].From
	t := j.Term()
	if t == nil || t.Op != ssa.OpBranch {
		return base
	}
	yes, no := t.Succs[0].To == b, t.Succs[1].To == b
	if yes == no {
		return base
	}
	c := t.Args[0]
	if c.Op == ssa.OpParam {
		k := -1
		for i, p := range j.Params {
			if p == c {
				k = i
			}
		}
		var src *ssa.Edge
		n := 0
		for _, e := range j.Preds {
			if k < 0 {
				break
			}
			if x := e.Args[k]; x.Op == ssa.OpConst && x.Aux == no {
				continue // leaves by the other edge
			}
			src = e
			n++
		}
		if n != 1 {
			return base
		}
		base, c = exit[src.From], src.Args[k]
	}
	switch c.Op {
	case ssa.OpLt, ssa.OpLe, ssa.OpGt, ssa.OpGe, ssa.OpEq, ssa.OpNe:
	default:
		return base
	}
	if at := c.Args[0].Type; at.Kind != ssa.KInt || at.U64() {
		return base
	}
	op := c.Op
	if no {
		op = negated(op)
	}
	return &factList{fact{x: c.Args[0], y: c.Args[1], op: op}, base}
}

// narrow returns the interval of x where the facts fs hold.
func (ri *rangeInfo) narrow(x *ssa.Value, fs *factList) (interval, bool) {
	r, ok := ri.get(x)
	if !ok {
		return r, false
	}
	n := r
	k := 0
	for l := fs; l != nil && k < maxFactWalk; l = l.next {
		k++
		f := l.f
		if f.op == 0 {
			if f.x == x {
				n.lo, n.hi = max(n.lo, 0), min(n.hi, f.lim-1)
			}
			continue
		}
		op, other := f.op, f.y
		switch x {
		case f.x:
		case f.y:
			op, other = swapped(op), f.x
		default:
			continue
		}
		o := ri.of(other)
		switch op {
		case ssa.OpLt:
			if o.hi > math.MinInt64 {
				n.hi = min(n.hi, o.hi-1)
			}
		case ssa.OpLe:
			n.hi = min(n.hi, o.hi)
		case ssa.OpGt:
			if o.lo < math.MaxInt64 {
				n.lo = max(n.lo, o.lo+1)
			}
		case ssa.OpGe:
			n.lo = max(n.lo, o.lo)
		case ssa.OpEq:
			n.lo, n.hi = max(n.lo, o.lo), min(n.hi, o.hi)
		}
	}
	if n.lo > n.hi {
		// the facts contradict: code no run reaches
		return r, true
	}
	return n, true
}

// get returns the interval of v (constants are their own).
func (ri *rangeInfo) get(v *ssa.Value) (interval, bool) {
	if n, ok := ssaConst(v); ok {
		return interval{n, n}, true
	}
	r, ok := ri.ranges[v]
	return r, ok
}

// of returns the interval of v, the full one when unknown.
func (ri *rangeInfo) of(v *ssa.Value) interval {
	if r, ok := ri.get(v); ok {
		return r
	}
	return fullInterval
}

// computeRanges analyses f, whose reachable blocks are blocks (reverse
// postorder).
func computeRanges(f *ssa.Func, blocks []*ssa.Block) *rangeInfo {
	ri := &rangeInfo{ranges: map[*ssa.Value]interval{}, mids: map[*ssa.Value]bool{},
		safe: map[*ssa.Value]bool{}, facts: map[*ssa.Block]*factList{}}
	order := map[*ssa.Block]int{}
	for i, b := range blocks {
		order[b] = i
	}
	idom := idoms(blocks, order)
	// the facts on leaving each block: its entry facts and the indexes
	// it took
	exit := map[*ssa.Block]*factList{}
	for _, b := range blocks {
		fs := entryFacts(b, exit[idom[b]], exit)
		ri.facts[b] = fs
		for _, v := range b.Instrs {
			if f, ok := indexFact(v); ok {
				fs = &factList{f, fs}
			}
		}
		exit[b] = fs
	}
	mids := map[*ssa.Value][2]*ssa.Value{}
	ups := []int64{0, math.MaxInt64}
	downs := []int64{0, math.MinInt64}
	for _, b := range blocks {
		for _, v := range b.Instrs {
			if lo, hi, ok := midOf(v); ok {
				for d := b; d != nil; d = idom[d] {
					if lessOnEntry(d, lo, hi) {
						mids[v] = [2]*ssa.Value{lo, hi}
						ri.mids[v] = true
						break
					}
				}
			}
			if (v.Op == ssa.OpRem || v.Op == ssa.OpAnd) && v.Type.Kind == ssa.KInt {
				if k, ok := ssaConst(v.Args[1]); ok && k != math.MinInt64 {
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
	for round := 0; round < 100; round++ {
		changed := false
		set := func(v *ssa.Value, r interval) {
			old, ok := ri.ranges[v]
			if ok && old == r {
				return
			}
			if ok && v.Op == ssa.OpParam {
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
			ri.ranges[v] = r
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
					a, ok := ri.get(e.Args[k])
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
			fs := ri.facts[b]
			for _, v := range b.Instrs {
				if v.Type != nil && v.Type.Kind == ssa.KInt && v.Op != ssa.OpConst {
					r, ok := ri.rangeOf(v, fs)
					if ab, isMid := mids[v]; ok && isMid {
						// a <= m < b (decision 0184)
						ra, ok1 := ri.get(ab[0])
						rb, ok2 := ri.get(ab[1])
						if ok1 && ok2 && rb.hi > math.MinInt64 {
							r.lo, r.hi = max(r.lo, ra.lo), min(r.hi, rb.hi-1)
						}
					}
					if ok {
						set(v, r)
					}
				}
				if f, ok := indexFact(v); ok {
					fs = &factList{f, fs}
				}
			}
		}
		if !changed {
			ri.findSafe(blocks)
			return ri
		}
	}
	// did not settle: know nothing
	ri.ranges = map[*ssa.Value]interval{}
	return ri
}

// findSafe marks the checked adds, subtracts, and multiplies whose
// operands' intervals, where they are computed, prove they cannot
// overflow.
func (ri *rangeInfo) findSafe(blocks []*ssa.Block) {
	for _, b := range blocks {
		fs := ri.facts[b]
		for _, v := range b.Instrs {
			switch v.Op {
			case ssa.OpAdd, ssa.OpSub, ssa.OpMul:
				if v.Pos != "" && v.Type.Kind == ssa.KInt && ri.noOverflow(v, fs) {
					ri.safe[v] = true
				}
			}
			if f, ok := indexFact(v); ok {
				fs = &factList{f, fs}
			}
		}
	}
}

// idoms returns the immediate dominator of each block of blocks (reverse
// postorder; the entry's is nil), by Cooper, Harvey, and Kennedy's
// iteration.
func idoms(blocks []*ssa.Block, order map[*ssa.Block]int) map[*ssa.Block]*ssa.Block {
	idom := map[*ssa.Block]*ssa.Block{}
	if len(blocks) == 0 {
		return idom
	}
	entry := blocks[0]
	idom[entry] = entry
	for changed := true; changed; {
		changed = false
		for _, b := range blocks[1:] {
			var nd *ssa.Block
			for _, e := range b.Preds {
				p := e.From
				if _, ok := idom[p]; !ok {
					continue
				}
				if nd == nil {
					nd = p
					continue
				}
				x, y := p, nd
				for x != y {
					for order[x] > order[y] {
						x = idom[x]
					}
					for order[y] > order[x] {
						y = idom[y]
					}
				}
				nd = x
			}
			if nd != nil && idom[b] != nd {
				idom[b] = nd
				changed = true
			}
		}
	}
	idom[entry] = nil
	return idom
}

// midOf matches v = a + (b - a) / 2 (or `>> 1`, either operand order of
// the add).
func midOf(v *ssa.Value) (a, b *ssa.Value, ok bool) {
	if v.Op != ssa.OpAdd || v.Type == nil || v.Type.Kind != ssa.KInt {
		return nil, nil, false
	}
	for k := 0; k < 2; k++ {
		x, h := v.Args[k], v.Args[1-k]
		half := false
		if len(h.Args) == 2 {
			if n, isK := ssaConst(h.Args[1]); isK {
				half = h.Op == ssa.OpDiv && n == 2 || h.Op == ssa.OpShr && n == 1
			}
		}
		if !half {
			continue
		}
		if d := h.Args[0]; d.Op == ssa.OpSub && d.Args[1] == x {
			return x, d.Args[0], true
		}
	}
	return nil, nil, false
}

// lessOnEntry reports whether block d is entered only by the edge of a
// branch on which a < b holds.
func lessOnEntry(d *ssa.Block, a, b *ssa.Value) bool {
	if len(d.Preds) != 1 {
		return false
	}
	t := d.Preds[0].From.Term()
	if t == nil || t.Op != ssa.OpBranch {
		return false
	}
	yes, no := t.Succs[0].To == d, t.Succs[1].To == d
	if yes == no {
		return false
	}
	c := t.Args[0]
	if len(c.Args) != 2 {
		return false
	}
	x, y := c.Args[0], c.Args[1]
	switch {
	case yes && c.Op == ssa.OpLt, no && c.Op == ssa.OpGe:
		return x == a && y == b
	case yes && c.Op == ssa.OpGt, no && c.Op == ssa.OpLe:
		return x == b && y == a
	}
	return false
}

// rangeOf computes the interval of v from its operands' (false while an
// operand has none yet).
func (ri *rangeInfo) rangeOf(v *ssa.Value, fs *factList) (interval, bool) {
	arg := func(i int) (interval, bool) { return ri.narrow(v.Args[i], fs) }
	switch v.Op {
	case ssa.OpAdd, ssa.OpSub, ssa.OpMul:
		if v.Aux == ssa.Wrap {
			return fullInterval, true
		}
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
		k, isK := ssaConst(v.Args[1])
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
		k, isK := ssaConst(v.Args[1])
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
		k, isK := ssaConst(v.Args[1])
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
			return fullInterval, true
		}
		return interval{-a.hi, -a.lo}, true
	case ssa.OpCall:
		switch v.Aux {
		case "list.len":
			return interval{0, math.MaxInt64}, true
		case "Str.len":
			return interval{0, maxStrLen}, true
		case "bytes.index":
			return interval{0, 255}, true
		}
	case ssa.OpIntConv:
		return arg(0)
	}
	return fullInterval, true
}

// noOverflow reports whether the Int add, subtract, or multiply v cannot
// overflow, by its operands' intervals.
func (ri *rangeInfo) noOverflow(v *ssa.Value, fs *factList) bool {
	a, ok1 := ri.narrow(v.Args[0], fs)
	b, ok2 := ri.narrow(v.Args[1], fs)
	if !ok1 || !ok2 {
		return false
	}
	switch v.Op {
	case ssa.OpAdd:
		_, ok1 := addOK(a.lo, b.lo)
		_, ok2 := addOK(a.hi, b.hi)
		return ok1 && ok2
	case ssa.OpSub:
		if b.lo == math.MinInt64 {
			return false
		}
		_, ok1 := addOK(a.lo, -b.hi)
		_, ok2 := addOK(a.hi, -b.lo)
		return ok1 && ok2
	case ssa.OpMul:
		for _, x := range []int64{a.lo, a.hi} {
			for _, y := range []int64{b.lo, b.hi} {
				if _, ok := mulOK(x, y); !ok {
					return false
				}
			}
		}
		return true
	}
	return false
}

// assume tells LLVM that the Int value x lies in r (nothing for the full
// interval).
func (s *sfn) assume(x string, r interval) {
	var c string
	switch {
	case r.lo == math.MinInt64 && r.hi == math.MaxInt64:
		return
	case r.lo == math.MinInt64:
		c = s.def("icmp sle i64 %s, %d", x, r.hi)
	case r.hi == math.MaxInt64:
		c = s.def("icmp sge i64 %s, %d", x, r.lo)
	case r.lo == 0:
		c = s.def("icmp ule i64 %s, %d", x, r.hi)
	default:
		d := s.def("sub i64 %s, %d", x, r.lo)
		c = s.def("icmp ule i64 %s, %s", d, fmt.Sprint(int64(uint64(r.hi)-uint64(r.lo))))
	}
	s.emit("call void @llvm.assume(i1 %s)", c)
}
