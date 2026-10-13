package arm64

import (
	"fmt"
	"math"
	"strings"

	"github.com/xo-lang/xo/internal/ssa"
)

// Local optimizations of instruction selection and MIR.
//
//   - Per MIR block memos: constants, list lengths, bounds checks done,
//     and element addresses are computed once per block. They are keyed
//     by vreg, which is sound because a vreg holds one SSA value at a
//     time within a block (list.set results alias their list: same cell,
//     same length; list.pop also aliases its list but changes the
//     length, so it drops the length memos of that list).
//   - Stores of list.set that write back a word loaded from the same
//     address with no store or call in between are dropped (updating one
//     field of a struct element writes one word).
//   - Dead code: instructions without side effects whose definitions are
//     never used are deleted (a field read of a list element loads one
//     word, not the whole element).

type memAt struct {
	base  vreg
	off   int
	epoch int
}

type memo struct {
	blk     *mblock
	consts  map[any]vreg
	lens    map[vreg]vreg
	checked map[[2]vreg]bool
	bases   map[[3]int64]vreg
	loaded  map[vreg]memAt
	epoch   int
	// bounds of Int registers known from the branches that dominate
	// the block
	lo, hi map[vreg]int64
	// lowZero: the number of low bits a branch proved zero
	lowZero map[vreg]int
	// less: x < y holds for the registers (x, y), from a branch or a
	// passed bounds check; the value is x's SSA value (decision 0184)
	less map[[2]vreg]*ssa.Value
	// quo: the quotient register of (dividend register, constant
	// divisor) for a division by a constant (decision 0194), so `x % c`
	// and `x / c` multiply once (inherited: a division that may fault
	// ends its SSA block, so the two are often in successive blocks)
	quo map[[2]int64]vreg
}

// m returns the memo of the current block.
func (s *fstate) m() *memo {
	if s.mm == nil || s.mm.blk != s.cur {
		s.mm = &memo{blk: s.cur, consts: map[any]vreg{}, lens: map[vreg]vreg{}, checked: map[[2]vreg]bool{},
			bases: map[[3]int64]vreg{}, loaded: map[vreg]memAt{}, lo: map[vreg]int64{}, hi: map[vreg]int64{}, lowZero: map[vreg]int{},
			less: map[[2]vreg]*ssa.Value{}, quo: map[[2]int64]vreg{}}
	}
	return s.mm
}

// clobber records a store or a call: loads before it may be stale.
func (s *fstate) clobber() { s.m().epoch++ }

// store adds a memory write.
func (s *fstate) store(text string, uses ...vreg) {
	for i := 1; i < len(uses); i++ {
		uses[i] = s.real(uses[i]) // the address; the value may be xzr
	}
	s.add(&inst{kind: iPlain, text: text, uses: uses})
	s.clobber()
}

// thread retargets branches to blocks that only jump.
func (s *fstate) thread() {
	blocks := append(append([]*mblock(nil), s.hot...), s.cold...)
	final := func(b *mblock) *mblock {
		for i := 0; i < 16; i++ {
			if len(b.insts) != 1 || b.insts[0].kind != iJump || b.insts[0].targets[0] == b || len(b.insts[0].targets) > 1 {
				return b
			}
			b = b.insts[0].targets[0]
		}
		return b
	}
	for _, b := range blocks {
		if t := b.term(); t != nil && (t.kind == iJump || t.kind == iCondBr || t.kind == iSwitch) {
			for i, x := range t.targets {
				t.targets[i] = final(x)
			}
		}
	}
	// drop blocks nothing reaches
	seen := map[*mblock]bool{}
	var walk func(b *mblock)
	walk = func(b *mblock) {
		if seen[b] {
			return
		}
		seen[b] = true
		if t := b.term(); t != nil && (t.kind == iJump || t.kind == iCondBr || t.kind == iSwitch) {
			for _, x := range t.targets {
				walk(x)
			}
		}
	}
	walk(s.hot[0])
	keep := func(bs []*mblock) []*mblock {
		out := bs[:0]
		for _, b := range bs {
			if seen[b] {
				out = append(out, b)
			}
		}
		return out
	}
	s.hot, s.cold = keep(s.hot), keep(s.cold)
}

// dce deletes instructions without side effects whose definitions
// nothing live uses (a register is live when an instruction with side
// effects, a terminator, a call, or a live instruction uses it), and the
// parts of parallel moves into dead registers.
func (s *fstate) dce() {
	blocks := append(append([]*mblock(nil), s.hot...), s.cold...)
	removable := func(in *inst) bool {
		return (in.kind == iPlain && in.fault == nil && !in.keep && len(in.defs) > 0) || in.kind == iPmove
	}
	live := make([]bool, len(s.fp))
	for changed := true; changed; {
		changed = false
		for _, b := range blocks {
			for _, in := range b.insts {
				if in.kind == iPmove {
					for k, d := range in.defs {
						if a := in.srcs[k]; live[d] && a.kind == sReg && !live[a.v] {
							live[a.v] = true
							changed = true
						}
					}
					continue
				}
				keep := !removable(in)
				for _, d := range in.defs {
					keep = keep || live[d]
				}
				if !keep {
					continue
				}
				for _, u := range in.uses {
					if !live[u] {
						live[u] = true
						changed = true
					}
				}
			}
		}
	}
	for _, b := range blocks {
		out := b.insts[:0]
		for _, in := range b.insts {
			if in.kind == iPmove {
				k := 0
				for i, d := range in.defs {
					if live[d] {
						in.defs[k] = d
						in.srcs[k] = in.srcs[i]
						k++
					}
				}
				in.defs, in.srcs = in.defs[:k], in.srcs[:k]
				in.uses = in.uses[:0]
				for _, a := range in.srcs {
					if a.kind == sReg && a.v != 0 {
						in.uses = append(in.uses, a.v)
					}
				}
				if k == 0 {
					continue
				}
			} else if removable(in) {
				dead := true
				for _, d := range in.defs {
					dead = dead && !live[d]
				}
				if dead {
					continue
				}
			}
			out = append(out, in)
		}
		b.insts = out
	}
}

// inherit starts the memo of the current block from the memo of a block
// that dominates it.
func (s *fstate) inherit(from *memo) {
	m := &memo{blk: s.cur, consts: map[any]vreg{}, lens: map[vreg]vreg{}, checked: map[[2]vreg]bool{},
		bases: map[[3]int64]vreg{}, loaded: map[vreg]memAt{}, epoch: from.epoch, lo: map[vreg]int64{}, hi: map[vreg]int64{}, lowZero: map[vreg]int{},
		less: map[[2]vreg]*ssa.Value{}, quo: map[[2]int64]vreg{}}
	for k, v := range from.less {
		m.less[k] = v
	}
	for k, v := range from.quo {
		m.quo[k] = v
	}
	for k, v := range from.lowZero {
		m.lowZero[k] = v
	}
	for k, v := range from.lo {
		m.lo[k] = v
	}
	for k, v := range from.hi {
		m.hi[k] = v
	}
	for k, v := range from.consts {
		// a constant one instruction makes is not worth a register
		// across blocks (and a fresh one coalesces with a block
		// parameter)
		switch x := k.(type) {
		case int64:
			if x >= -65536 && x < 65536 {
				continue
			}
		case bool:
			continue
		case float64:
			if x == 0 {
				continue
			}
		}
		m.consts[k] = v
	}
	for k, v := range from.lens {
		m.lens[k] = v
	}
	for k, v := range from.checked {
		m.checked[k] = v
	}
	for k, v := range from.bases {
		m.bases[k] = v
	}
	for k, v := range from.loaded {
		m.loaded[k] = v
	}
	s.mm = m
}

// joinFacts adds the facts of memo from (bounds, relations, passed
// checks) to the current memo, for a block that from's block reaches
// it on every path that runs but does not dominate in the control flow
// graph (joinSource). The registers from's block computed (constants,
// lengths, addresses, loads, quotients) are not taken: a path the graph
// has but execution never takes would read them before they are
// written (gotchas/arm64-vreg-read-before-write-break-flag.md).
func (s *fstate) joinFacts(from *memo) {
	m := s.m()
	for k, v := range from.less {
		m.less[k] = v
	}
	for k, v := range from.checked {
		m.checked[k] = v
	}
	for k, v := range from.lo {
		if o, ok := m.lo[k]; !ok || v > o {
			m.lo[k] = v
		}
	}
	for k, v := range from.hi {
		if o, ok := m.hi[k]; !ok || v < o {
			m.hi[k] = v
		}
	}
	for k, v := range from.lowZero {
		if o, ok := m.lowZero[k]; !ok || v > o {
			m.lowZero[k] = v
		}
	}
}

// forgetLen drops what is known about the length of list l (a pop
// changed it).
func (s *fstate) forgetLen(l vreg) {
	m := s.m()
	delete(m.lens, l)
	for k := range m.checked {
		if k[0] == l {
			delete(m.checked, k)
		}
	}
}

// Bounds: a block whose only predecessor ends in `br (x < y)` knows
// x < y on the edge it is reached by, which bounds x and y. A checked
// add or subtract of a constant whose bounds prove it cannot overflow
// is not checked (`n - 1` under `n >= 2`, `i + 1` under `i < n`).

// bounds returns what is known of Int value v.
func (s *fstate) bounds(v *ssa.Value) (int64, int64) { return s.boundsAt(v, boundsDepth) }

// boundsDepth is how many operations deep bounds derives an interval
// from the operands' bounds (decision 0184): the range analysis ignores
// branches, so `(r - 128) * 3` under `r < 2048` learns its bound here.
const boundsDepth = 3

func (s *fstate) boundsAt(v *ssa.Value, d int) (int64, int64) {
	if n, ok := constInt(v); ok {
		return n, n
	}
	r := s.ssaRange(v) // ranges.go
	lo, hi := r.lo, r.hi
	if d > 0 {
		if x, ok := s.derived(v, d-1); ok {
			lo, hi = max(lo, x.lo), min(hi, x.hi)
		}
	}
	ws, ok := s.vals[v]
	if !ok || len(ws) != 1 {
		return lo, hi
	}
	m := s.m()
	if x, ok := m.lo[ws[0]]; ok {
		lo = max(lo, x)
	}
	if x, ok := m.hi[ws[0]]; ok {
		hi = min(hi, x)
	}
	return lo, hi
}

// derived is the interval of the Int operation v from its operands'
// bounds (derived d more levels deep). Every operation is exact (a
// checked one faults outside Int, an unchecked one was proved), as in
// rangeOf; a shift left only counts when it is checked.
func (s *fstate) derived(v *ssa.Value, d int) (interval, bool) {
	if v.Type == nil || v.Type.Kind != ssa.KInt {
		return interval{}, false
	}
	arg := func(i int) interval {
		lo, hi := s.boundsAt(v.Args[i], d)
		return interval{lo, hi}
	}
	switch v.Op {
	case ssa.OpAdd:
		a, b := arg(0), arg(1)
		return interval{satAdd(a.lo, b.lo), satAdd(a.hi, b.hi)}, true
	case ssa.OpSub:
		a, b := arg(0), arg(1)
		if b.lo == math.MinInt64 {
			return interval{}, false
		}
		return interval{satAdd(a.lo, -b.hi), satAdd(a.hi, -b.lo)}, true
	case ssa.OpMul:
		a, b := arg(0), arg(1)
		c0, c1, c2, c3 := satMul(a.lo, b.lo), satMul(a.lo, b.hi), satMul(a.hi, b.lo), satMul(a.hi, b.hi)
		return interval{min(c0, c1, c2, c3), max(c0, c1, c2, c3)}, true
	case ssa.OpDiv:
		k, ok := constInt(v.Args[1])
		if !ok || k <= 0 {
			return interval{}, false
		}
		a := arg(0)
		return interval{a.lo / k, a.hi / k}, true
	case ssa.OpAnd:
		a, b := arg(0), arg(1)
		switch {
		case a.lo >= 0 && b.lo >= 0:
			return interval{0, min(a.hi, b.hi)}, true
		case a.lo >= 0:
			return interval{0, a.hi}, true
		case b.lo >= 0:
			return interval{0, b.hi}, true
		}
	case ssa.OpOr:
		a, b := arg(0), arg(1)
		if a.lo >= 0 && b.lo >= 0 {
			return interval{max(a.lo, b.lo), ones(max(a.hi, b.hi))}, true
		}
	case ssa.OpShr:
		k, ok := constInt(v.Args[1])
		if !ok || k < 0 || k > 63 {
			return interval{}, false
		}
		a := arg(0)
		return interval{a.lo >> k, a.hi >> k}, true
	case ssa.OpShl:
		k, ok := constInt(v.Args[1])
		if !ok || k < 0 || k > 62 || v.Pos == "" {
			return interval{}, false
		}
		a := arg(0)
		return interval{satMul(a.lo, int64(1)<<k), satMul(a.hi, int64(1)<<k)}, true
	case ssa.OpIntConv:
		return arg(0), true
	case ssa.OpCall:
		if v.Aux == "Str.len" && v.Args[0].Op == ssa.OpConst {
			if x, ok := v.Args[0].Aux.(string); ok {
				return interval{int64(len(x)), int64(len(x))}, true
			}
		}
	}
	return interval{}, false
}

// ones is the smallest 2^k - 1 at least x (x >= 0): a bound of x | y
// for x, y in [0, x].
func ones(x int64) int64 {
	r := int64(0)
	for r < x {
		r = r<<1 | 1
	}
	return r
}

// provedIndex reports whether 0 <= i < n is known, n a length in a
// register (decision 0184): from i's bounds and n's constant value k
// (-1 when not constant), or from a fact x < n of the memo where x and
// i are one value plus offsets whose bounds put i at or below x
// (`i + 1` under `i + k < n` with k >= 1).
func (s *fstate) provedIndex(i *ssa.Value, n vreg, k int64) bool {
	lo, hi := s.bounds(i)
	if lo < 0 {
		return false
	}
	if k >= 0 && hi < k {
		return true
	}
	fi := s.addForms(i)
	for key, x := range s.m().less {
		if key[1] != n {
			continue
		}
		for _, b := range s.addForms(x) {
			for _, a := range fi {
				if a.base == b.base && (a.hi <= b.lo || a.off != nil && a.off == b.off) {
					return true
				}
			}
		}
	}
	return false
}

// addForm says a value is base plus an offset in [lo, hi] (the value
// off, when it is one).
type addForm struct {
	base   *ssa.Value
	lo, hi int64
	off    *ssa.Value
}

// addForms returns the ways Int value v is a value plus an offset: v
// plus 0; for v = a + b, a plus b and b plus a; for v = a - c (c
// constant), a plus -c.
func (s *fstate) addForms(v *ssa.Value) []addForm {
	out := []addForm{{v, 0, 0, nil}}
	switch v.Op {
	case ssa.OpAdd:
		alo, ahi := s.bounds(v.Args[0])
		blo, bhi := s.bounds(v.Args[1])
		out = append(out, addForm{v.Args[0], blo, bhi, v.Args[1]}, addForm{v.Args[1], alo, ahi, v.Args[0]})
	case ssa.OpSub:
		if c, ok := constInt(v.Args[1]); ok && c != math.MinInt64 {
			out = append(out, addForm{v.Args[0], -c, -c, nil})
		}
	}
	return out
}

// lessFact records x < y for SSA value x (one register) and register y.
func (s *fstate) lessFact(x *ssa.Value, y vreg) {
	ws, ok := s.vals[x]
	if !ok || len(ws) != 1 || ws[0] == 0 || y == 0 || x.Type.Kind != ssa.KInt {
		return
	}
	s.m().less[[2]vreg{ws[0], y}] = x
}

var negateCmp = map[ssa.Op]ssa.Op{ssa.OpLt: ssa.OpGe, ssa.OpGe: ssa.OpLt, ssa.OpLe: ssa.OpGt, ssa.OpGt: ssa.OpLe, ssa.OpEq: ssa.OpNe, ssa.OpNe: ssa.OpEq}
var swapCmp = map[ssa.Op]ssa.Op{ssa.OpLt: ssa.OpGt, ssa.OpGt: ssa.OpLt, ssa.OpLe: ssa.OpGe, ssa.OpGe: ssa.OpLe, ssa.OpEq: ssa.OpEq, ssa.OpNe: ssa.OpNe}

// branchFacts records the bounds the branch into b (its only
// predecessor's terminator) implies.
func (s *fstate) branchFacts(b *ssa.Block) {
	t := b.Preds[0].From.Term()
	if t == nil || t.Op != ssa.OpBranch {
		return
	}
	yes, no := t.Succs[0].To == b, t.Succs[1].To == b
	if yes == no {
		return
	}
	s.condFacts(t.Args[0], yes)
}

// joinSource matches b entered only from a block J that has one
// parameter r, holds nothing but a branch on r, and is reached with an
// r that can take b's edge from one predecessor p only (the others pass
// the constant that takes the other edge): the join of `x && y` or
// `x || y`. Then b runs only after p, and c (what p passes) is truth
// (decision 0184).
func (s *fstate) joinSource(b *ssa.Block) (p *ssa.Block, c *ssa.Value, truth bool, ok bool) {
	if len(b.Preds) != 1 {
		return nil, nil, false, false
	}
	j := b.Preds[0].From
	t := j.Term()
	if t == nil || t.Op != ssa.OpBranch || len(j.Instrs) != 1 || len(j.Params) != 1 || t.Args[0] != j.Params[0] || j == b {
		return nil, nil, false, false
	}
	yes, no := t.Succs[0].To == b, t.Succs[1].To == b
	if yes == no {
		return nil, nil, false, false
	}
	for _, e := range j.Preds {
		a := e.Args[0]
		if k, isK := a.Aux.(bool); isK && a.Op == ssa.OpConst && k != yes {
			continue
		}
		if p != nil {
			return nil, nil, false, false
		}
		p, c = e.From, a
	}
	if p == nil || s.endMemo[p] == nil {
		return nil, nil, false, false
	}
	return p, c, yes, true
}

// condFacts records what Bool c being truth says (a compare of Ints).
func (s *fstate) condFacts(c *ssa.Value, truth bool) {
	if !isCompare(c.Op) || c.Args[0].Type.Kind != ssa.KInt {
		return
	}
	op := c.Op
	if !truth {
		op = negateCmp[op]
	}
	x, y := c.Args[0], c.Args[1]
	if op == ssa.OpEq {
		// x % 2^k == 0: the low k bits of x are zero
		for _, r := range []*ssa.Value{x, y} {
			if s.remTest(r) {
				k, _ := pow2(r.Args[1])
				if ws, ok := s.vals[r.Args[0]]; ok && len(ws) == 1 {
					s.m().lowZero[ws[0]] = k
				}
			}
		}
	}
	switch op {
	case ssa.OpLt:
		if ws, ok := s.vals[y]; ok && len(ws) == 1 {
			s.lessFact(x, ws[0])
		}
	case ssa.OpGt:
		if ws, ok := s.vals[x]; ok && len(ws) == 1 {
			s.lessFact(y, ws[0])
		}
	}
	xlo, xhi := s.bounds(x)
	ylo, yhi := s.bounds(y)
	s.narrow(x, op, ylo, yhi)
	s.narrow(y, swapCmp[op], xlo, xhi)
}

// narrow records that x op y holds, y in [ylo, yhi].
func (s *fstate) narrow(x *ssa.Value, op ssa.Op, ylo, yhi int64) {
	if x.Op == ssa.OpConst {
		return
	}
	ws, ok := s.vals[x]
	if !ok || len(ws) != 1 || ws[0] == 0 {
		return
	}
	lo, hi := s.bounds(x)
	switch op {
	case ssa.OpLt:
		if yhi > math.MinInt64 {
			hi = min(hi, yhi-1)
		}
	case ssa.OpLe:
		hi = min(hi, yhi)
	case ssa.OpGt:
		if ylo < math.MaxInt64 {
			lo = max(lo, ylo+1)
		}
	case ssa.OpGe:
		lo = max(lo, ylo)
	case ssa.OpEq:
		lo, hi = max(lo, ylo), min(hi, yhi)
	}
	m := s.m()
	m.lo[ws[0]], m.hi[ws[0]] = lo, hi
	// x = a + c (exact): a op y - c (decision 0184)
	if x.Op == ssa.OpAdd || x.Op == ssa.OpSub {
		if c, ok := constInt(x.Args[1]); ok && c != math.MinInt64 {
			if x.Op == ssa.OpSub {
				c = -c
			}
			s.narrow(x.Args[0], op, satAdd(ylo, -c), satAdd(yhi, -c))
		}
	}
}

// safeAdd reports whether a + n (or a - n) cannot overflow.
func (s *fstate) safeAdd(a *ssa.Value, n int64, op ssa.Op) bool {
	if op == ssa.OpSub {
		if n == math.MinInt64 {
			return false
		}
		n = -n
	}
	lo, hi := s.bounds(a)
	if n >= 0 {
		return hi <= math.MaxInt64-n
	}
	return lo >= math.MinInt64-n
}

// threadConst sends a jump to a block that only tests a register
// straight to the test's outcome when the jumping block set that
// register to a constant (the tag of an Option a pop or get built,
// tested at once by `??` or `if let`). Blocks that only move registers
// and jump (the joins an inlined return passes its value through) are
// followed to the test, and their moves are copied into the jumping
// block (decision 0184).
func (s *fstate) threadConst() {
	blocks := append(append([]*mblock(nil), s.hot...), s.cold...)
	for _, p := range blocks {
		t := p.term()
		if t == nil || t.kind != iJump {
			continue
		}
		j := t.targets[0]
		chain := moveChain(p, &j)
		if len(j.insts) != 1 || j.insts[0].kind != iCondBr {
			continue
		}
		br := j.insts[0]
		if len(br.uses) == 0 {
			continue // a landing check (decision 0085)
		}
		r := br.uses[0]
		var k int64
		if br.cond != "cbnz" {
			if len(br.uses) != 1 {
				continue
			}
			if _, err := fmt.Sscanf(br.text, "cmp $u0, #%d", &k); err != nil || br.text != fmt.Sprintf("cmp $u0, #%d", k) {
				continue
			}
		}
		val, ok, found := int64(0), false, false
		for c := len(chain) - 1; c >= 0 && !found; c-- {
			for i, d := range chain[c].defs {
				if d != r {
					continue
				}
				switch src := chain[c].srcs[i]; {
				case src.kind == sImm:
					val, ok, found = src.imm, true, true
				case src.kind == sReg && src.v == 0:
					val, ok, found = 0, true, true
				case src.kind == sReg:
					r = src.v
				default:
					found = true
				}
				break
			}
		}
		if !found {
			val, ok = constBefore(p, len(p.insts)-1, r)
		}
		if !ok {
			continue
		}
		var taken bool
		switch br.cond {
		case "cbnz":
			taken = val != 0
		case "eq":
			taken = val == k
		case "ne":
			taken = val != k
		case "lt":
			taken = val < k
		case "le":
			taken = val <= k
		case "gt":
			taken = val > k
		case "ge":
			taken = val >= k
		default:
			continue
		}
		if taken {
			t.targets[0] = br.targets[0]
		} else {
			t.targets[0] = br.targets[1]
		}
		if len(chain) > 0 {
			copyMoves(p, chain)
		}
	}
}

// moveChain follows *j, p's jump target, through blocks that only move
// registers and jump (at most 8), leaving *j at the first other block,
// and returns their moves in order (decision 0184).
func moveChain(p *mblock, j **mblock) []*inst {
	var chain []*inst
	for steps := 0; steps < 8 && *j != p && len((*j).insts) == 2 && (*j).insts[0].kind == iPmove &&
		(*j).insts[1].kind == iJump && len((*j).insts[1].targets) == 1; steps++ {
		chain = append(chain, (*j).insts[0])
		*j = (*j).insts[1].targets[0]
	}
	return chain
}

// throughMoves returns the register whose value r holds after the moves
// of chain, read before them: false when one sets r to anything but a
// register.
func throughMoves(chain []*inst, r vreg) (vreg, bool) {
	for c := len(chain) - 1; c >= 0; c-- {
		for i, d := range chain[c].defs {
			if d != r {
				continue
			}
			if chain[c].srcs[i].kind != sReg || chain[c].srcs[i].v == 0 {
				return 0, false
			}
			r = chain[c].srcs[i].v
			break
		}
	}
	return r, true
}

// copyMoves puts copies of the moves of chain before p's terminator.
func copyMoves(p *mblock, chain []*inst) {
	t := p.insts[len(p.insts)-1]
	var moves []*inst
	for _, mv := range chain {
		c := *mv
		c.defs = append([]vreg(nil), mv.defs...)
		c.srcs = append([]src(nil), mv.srcs...)
		c.uses = append([]vreg(nil), mv.uses...)
		moves = append(moves, &c)
	}
	p.insts = append(append(p.insts[:len(p.insts)-1:len(p.insts)-1], moves...), t)
}

// constBefore returns the constant register r holds before instruction
// end of p, when the last definition of r there is a parallel move of an
// immediate, or of a register that holds one (decision 0184: the true of
// `a || b` is a constant register moved into the join's parameter), or a
// `mov` of a constant.
func constBefore(p *mblock, end int, r vreg) (int64, bool) {
	for k := end - 1; k >= 0; k-- {
		in := p.insts[k]
		for i, d := range in.defs {
			if d != r {
				continue
			}
			if in.kind == iPmove {
				switch {
				case in.srcs[i].kind == sImm:
					return in.srcs[i].imm, true
				case in.srcs[i].kind == sReg && in.srcs[i].v == 0:
					return 0, true // xzr
				case in.srcs[i].kind == sReg:
					return constBefore(p, k, in.srcs[i].v)
				}
				return 0, false
			}
			var n int64
			if in.kind == iPlain && in.konst && len(in.defs) == 1 && strings.HasPrefix(in.text, "mov $d0, #") {
				if _, err := fmt.Sscanf(in.text, "mov $d0, #%d", &n); err == nil && in.text == fmt.Sprintf("mov $d0, #%d", n) {
					return n, true
				}
			}
			return 0, false
		}
	}
	return 0, false
}

// threadCset turns `t = cmp ...; cset; r = t; jump J` where J only
// branches on r into a branch on the comparison itself (an inlined
// predicate returns its Bool through the continuation's parameter).
func (s *fstate) threadCset() {
	blocks := append(append([]*mblock(nil), s.hot...), s.cold...)
	nuses := useCounts(blocks)
	for _, p := range blocks {
		t := p.term()
		if t == nil || t.kind != iJump || len(p.insts) < 3 || len(t.targets) > 1 {
			continue
		}
		j := t.targets[0]
		chain := moveChain(p, &j)
		if len(j.insts) != 1 || j.insts[0].kind != iCondBr || j.insts[0].cond != "cbnz" || j == p {
			continue
		}
		r, traced := throughMoves(chain, j.insts[0].uses[0])
		if !traced {
			continue
		}
		mv := p.insts[len(p.insts)-2]
		if mv.kind != iPmove {
			continue
		}
		var src vreg
		for i, d := range mv.defs {
			if d == r && mv.srcs[i].kind == sReg {
				src = mv.srcs[i].v
			}
		}
		if src == 0 {
			continue
		}
		def := p.insts[len(p.insts)-3]
		if def.kind != iPlain || len(def.defs) != 1 || def.defs[0] != src || def.fault != nil || def.keep {
			continue
		}
		i := strings.LastIndex(def.text, "\ncset $d0, ")
		if i < 0 {
			continue
		}
		cond := def.text[i+len("\ncset $d0, "):]
		if _, ok := invCond[cond]; !ok {
			continue
		}
		p.insts[len(p.insts)-1] = &inst{kind: iCondBr, text: def.text[:i], uses: append([]vreg(nil), def.uses...),
			cond: cond, targets: append([]*mblock(nil), j.insts[0].targets...)}
		if len(chain) > 0 {
			copyMoves(p, chain)
		} else if nuses[r] == 1 {
			// only J's branch reads r, and p no longer reaches it: the
			// move (and then the cset, by dce) is dead (decision 0184)
			dropMove(p, len(p.insts)-2, r)
		}
	}
}

// useCounts counts the uses of each register in blocks.
func useCounts(blocks []*mblock) map[vreg]int {
	n := map[vreg]int{}
	for _, b := range blocks {
		for _, in := range b.insts {
			for _, u := range in.uses {
				n[u]++
			}
		}
	}
	return n
}

// dropMove removes the move into r from the parallel move p.insts[k]
// (the whole instruction when nothing is left).
func dropMove(p *mblock, k int, r vreg) {
	mv := p.insts[k]
	var defs []vreg
	var srcs []src
	var uses []vreg
	for i, d := range mv.defs {
		if d == r {
			continue
		}
		defs = append(defs, d)
		srcs = append(srcs, mv.srcs[i])
		if mv.srcs[i].kind == sReg && mv.srcs[i].v != 0 {
			uses = append(uses, mv.srcs[i].v)
		}
	}
	if len(defs) == 0 {
		p.insts = append(p.insts[:k], p.insts[k+1:]...)
		return
	}
	mv.defs, mv.srcs, mv.uses = defs, srcs, uses
}

// hoistConsts moves the instructions that make constants out of loops,
// into the loop's preheader (the header's one predecessor outside the
// loop, when it ends in a jump), innermost loops first. A constant that
// feeds a parallel move, a call, or a return stays: made in place it
// goes straight into the block parameter or argument register
// (gotchas/arm64-cheap-constants-across-blocks.md).
func (s *fstate) hoistConsts() {
	blocks := append(append([]*mblock(nil), s.hot...), s.cold...)
	succs := func(b *mblock) []*mblock {
		if t := b.term(); t != nil && (t.kind == iJump || t.kind == iCondBr || t.kind == iSwitch) {
			return t.targets
		}
		return nil
	}
	preds := map[*mblock][]*mblock{}
	for _, b := range blocks {
		for _, t := range succs(b) {
			preds[t] = append(preds[t], b)
		}
	}
	moved := map[vreg]bool{}
	for _, b := range blocks {
		for _, in := range b.insts {
			// a constant a move, call, or return takes is made straight
			// into its register in place
			if in.kind == iPmove || in.kind == iCall || in.kind == iRet {
				for _, a := range in.srcs {
					if a.kind == sReg {
						moved[a.v] = true
					}
				}
			}
		}
	}
	// natural loops of back edges found by depth first search
	type loop struct {
		head *mblock
		body map[*mblock]bool
	}
	var loops []loop
	state := map[*mblock]int{}
	var dfs func(b *mblock)
	dfs = func(b *mblock) {
		state[b] = 1
		for _, t := range succs(b) {
			switch state[t] {
			case 0:
				dfs(t)
			case 1:
				body := map[*mblock]bool{t: true}
				work := []*mblock{b}
				for len(work) > 0 {
					x := work[len(work)-1]
					work = work[:len(work)-1]
					if !body[x] {
						body[x] = true
						work = append(work, preds[x]...)
					}
				}
				loops = append(loops, loop{t, body})
			}
		}
		state[b] = 2
	}
	dfs(s.hot[0])
	// innermost first: smaller bodies first
	for i := 0; i < len(loops); i++ {
		for j := i + 1; j < len(loops); j++ {
			if len(loops[j].body) < len(loops[i].body) {
				loops[i], loops[j] = loops[j], loops[i]
			}
		}
	}
	for _, l := range loops {
		var pre *mblock
		for _, p := range preds[l.head] {
			if l.body[p] {
				continue
			}
			if pre != nil {
				pre = nil
				break
			}
			pre = p
		}
		if pre == nil || pre.term() == nil || pre.term().kind != iJump {
			continue
		}
		// across a call a constant needs a callee saved register, and a
		// spilled one would be reloaded from the stack: leave loops that
		// call on their hot path alone (rematerializing spilled
		// constants would lift this)
		calls := false
		for b := range l.body {
			for _, in := range b.insts {
				calls = calls || (in.kind == iCall && !b.cold)
			}
		}
		if calls {
			continue
		}
		var hoisted []*inst
		for _, b := range blocks {
			if !l.body[b] {
				continue
			}
			out := b.insts[:0]
			for _, in := range b.insts {
				if in.konst && !moved[in.defs[0]] {
					hoisted = append(hoisted, in)
					continue
				}
				out = append(out, in)
			}
			b.insts = out
		}
		if len(hoisted) == 0 {
			continue
		}
		n := len(pre.insts)
		pre.insts = append(append(append([]*inst(nil), pre.insts[:n-1]...), hoisted...), pre.insts[n-1])
	}
}

// selectCompares turns a compare that only picks which of two other
// compares decides a branch into straight line code: `if a.key != b.key {
// a.key < b.key } else { a.id < b.id }` feeding a branch becomes both
// compares with cset, a csel on the first compare, and one tbnz. The
// branch on the first compare is often unpredictable (equal keys in a
// sort); the two compares cost a few ALU operations.
func (s *fstate) selectCompares() {
	blocks := append(append([]*mblock(nil), s.hot...), s.cold...)
	npred := map[*mblock]int{}
	for _, b := range blocks {
		if t := b.term(); t != nil && (t.kind == iJump || t.kind == iCondBr || t.kind == iSwitch) {
			for _, x := range t.targets {
				npred[x]++
			}
		}
	}
	flagTest := func(in *inst) bool {
		return in != nil && in.kind == iCondBr && in.text != "" && in.cond != "cbnz" && in.cond != "tbz" && in.cond != "tbnz" && invCond[in.cond] != ""
	}
	// shift renumbers the uses of a template by k
	shift := func(t string, k int) string {
		var out strings.Builder
		for i := 0; i < len(t); i++ {
			out.WriteByte(t[i])
			if t[i] == '$' && i+2 < len(t) && t[i+1] == 'u' && t[i+2] >= '0' && t[i+2] <= '9' {
				out.WriteByte('u')
				out.WriteByte(byte('0' + int(t[i+2]-'0') + k))
				i += 2
			}
		}
		return out.String()
	}
	changed := false
	for _, p := range s.hot {
		pt := p.term()
		if !flagTest(pt) || (pt.cond != "eq" && pt.cond != "ne") || strings.Contains(pt.text, "\n") {
			continue
		}
		e, n := pt.targets[0], pt.targets[1] // e: taken when the first compare is equal
		if pt.cond == "ne" {
			e, n = n, e
		}
		if e == n || e == p || n == p || len(e.insts) != 1 || len(n.insts) != 1 || npred[e] != 1 || npred[n] != 1 {
			continue
		}
		et, nt := e.insts[0], n.insts[0]
		if !flagTest(et) || !flagTest(nt) || !oneCompare(et.text) || !oneCompare(nt.text) {
			continue
		}
		t, f := et.targets[0], et.targets[1]
		if t == f {
			continue
		}
		ec := et.cond
		nc := nt.cond
		switch {
		case nt.targets[0] == t && nt.targets[1] == f:
		case nt.targets[0] == f && nt.targets[1] == t:
			nc = invCond[nc]
		default:
			continue
		}
		if len(et.uses)+len(pt.uses) > 9 || len(nt.uses)+len(pt.uses)+1 > 9 {
			continue
		}
		insts := p.insts[:len(p.insts)-1]
		a := s.newVreg(false)
		insts = append(insts, &inst{kind: iPlain, text: et.text + "\ncset $d0, " + ec, defs: []vreg{a}, uses: append([]vreg(nil), et.uses...)})
		r := s.newVreg(false)
		same := nt.text == pt.text && len(nt.uses) == len(pt.uses)
		for i := 0; same && i < len(nt.uses); i++ {
			same = nt.uses[i] == pt.uses[i]
		}
		if same {
			// the other compare is the first one: its flags pick too
			insts = append(insts, &inst{kind: iPlain, text: shift(pt.text, 1) + "\ncset $d0, " + nc + "\ncsel $d0, $u0, $d0, eq",
				defs: []vreg{r}, uses: append([]vreg{a}, pt.uses...), early: true})
		} else {
			b := s.newVreg(false)
			insts = append(insts, &inst{kind: iPlain, text: nt.text + "\ncset $d0, " + nc, defs: []vreg{b}, uses: append([]vreg(nil), nt.uses...)})
			insts = append(insts, &inst{kind: iPlain, text: shift(pt.text, 2) + "\ncsel $d0, $u0, $u1, eq",
				defs: []vreg{r}, uses: append([]vreg{a, b}, pt.uses...)})
		}
		insts = append(insts, &inst{kind: iCondBr, cond: "tbnz", uses: []vreg{r}, targets: []*mblock{t, f}})
		p.insts = insts
		changed = true
	}
	if changed {
		s.thread()
	}
}

// oneCompare reports whether a branch's flag setting text is one
// compare: one instruction, or a Float ordering compare (fcmp then
// fccmp, floatOrder) that only sets the flags.
func oneCompare(text string) bool {
	if !strings.Contains(text, "\n") {
		return true
	}
	for _, l := range strings.Split(text, "\n") {
		if !strings.HasPrefix(l, "fcmp ") && !strings.HasPrefix(l, "fccmp ") {
			return false
		}
	}
	return true
}

// reuseFlags drops the compare of a block that only branches when its one
// predecessor ended with the same compare of the same registers: the
// flags still hold (`a.key != b.key` then `a.key < b.key`).
func (s *fstate) reuseFlags() {
	blocks := append(append([]*mblock(nil), s.hot...), s.cold...)
	preds := map[*mblock][]*mblock{}
	for _, b := range blocks {
		if t := b.term(); t != nil && (t.kind == iJump || t.kind == iCondBr || t.kind == iSwitch) {
			for _, x := range t.targets {
				if len(preds[x]) == 0 || preds[x][len(preds[x])-1] != b {
					preds[x] = append(preds[x], b)
				}
			}
		}
	}
	flags := func(in *inst) bool {
		return in != nil && in.kind == iCondBr && in.text != "" && in.cond != "cbnz" && in.cond != "tbz" && in.cond != "tbnz"
	}
	for _, x := range blocks {
		if len(x.insts) != 1 || !flags(x.insts[0]) || len(preds[x]) != 1 || preds[x][0] == x {
			continue
		}
		in, p := x.insts[0], preds[x][0].term()
		if !flags(p) || p.text != in.text || len(p.uses) != len(in.uses) {
			continue
		}
		same := true
		for i := range in.uses {
			same = same && in.uses[i] == p.uses[i]
		}
		if same {
			in.text, in.uses = "", nil
		}
	}
}

// safeAddRange reports whether a + b (op OpAdd) or a - b (OpSub) cannot
// overflow for a in [alo, ahi] and b in [blo, bhi] (decision 0184).
func (s *fstate) safeAddRange(op ssa.Op, alo, ahi, blo, bhi int64) bool {
	if op == ssa.OpSub {
		if blo == math.MinInt64 {
			return false
		}
		blo, bhi = -bhi, -blo
	}
	if bhi > 0 && ahi > math.MaxInt64-bhi {
		return false
	}
	if blo < 0 && alo < math.MinInt64-blo {
		return false
	}
	return true
}

// liveDCE deletes the instructions without side effects, and the moves
// of parallel moves, whose definitions are dead where they are written
// (decision 0184). Unlike dce, which keeps a register every use of
// anywhere, it is flow sensitive: a Bool that a threaded join no longer
// reads on this path (the `mov x0, #1` and `cset` left by threadConst
// and threadCset) goes. One backward pass over each block from the
// registers live out of it; a register live into a landing pad counts
// as live everywhere. It runs before selectCompares and reuseFlags,
// which rely on compares in other blocks.
func (s *fstate) liveDCE() {
	order := append(append([]*mblock(nil), s.hot...), s.cold...)
	n := len(s.fp)
	idx := map[*mblock]int{}
	for k, b := range order {
		idx[b] = k
	}
	succs := make([][]int, len(order))
	gen := make([]bitset, len(order))
	kill := make([]bitset, len(order))
	liveIn := make([]bitset, len(order))
	var pads []int
	for k, b := range order {
		for _, t := range s.succs(b) {
			if j, ok := idx[t]; ok {
				succs[k] = append(succs[k], j)
			}
		}
		gen[k], kill[k], liveIn[k] = newBitset(n), newBitset(n), newBitset(n)
		for _, in := range b.insts {
			if in.pad != nil {
				j, ok := idx[s.padOf(in)]
				if !ok {
					return
				}
				pads = append(pads, j)
			}
			for _, u := range in.uses {
				if u != 0 && !kill[k].has(u) {
					gen[k].set(u)
				}
			}
			for _, d := range in.defs {
				if d != 0 {
					kill[k].set(d)
				}
			}
		}
	}
	out := newBitset(n)
	for changed := true; changed; {
		changed = false
		for k := len(order) - 1; k >= 0; k-- {
			for w := range out {
				out[w] = 0
			}
			for _, j := range succs[k] {
				for w, x := range liveIn[j] {
					out[w] |= x
				}
			}
			for w := range out {
				nv := gen[k][w] | (out[w] &^ kill[k][w])
				if nv != liveIn[k][w] {
					liveIn[k][w] = nv
					changed = true
				}
			}
		}
	}
	padLive := newBitset(n)
	for _, j := range pads {
		for w, x := range liveIn[j] {
			padLive[w] |= x
		}
	}
	live := newBitset(n)
	dead := func(d vreg) bool { return d != 0 && !live.has(d) && !padLive.has(d) }
	for k, b := range order {
		for w := range live {
			live[w] = 0
		}
		for _, j := range succs[k] {
			for w, x := range liveIn[j] {
				live[w] |= x
			}
		}
		keep := make([]bool, len(b.insts))
		for i := len(b.insts) - 1; i >= 0; i-- {
			in := b.insts[i]
			keep[i] = true
			switch {
			case in.kind == iPmove:
				m := 0
				for k2, d := range in.defs {
					if !dead(d) {
						in.defs[m], in.srcs[m] = d, in.srcs[k2]
						m++
					}
				}
				in.defs, in.srcs = in.defs[:m], in.srcs[:m]
				in.uses = in.uses[:0]
				for _, a := range in.srcs {
					if a.kind == sReg && a.v != 0 {
						in.uses = append(in.uses, a.v)
					}
				}
				keep[i] = m > 0
			case in.kind == iPlain && in.fault == nil && !in.keep && len(in.defs) > 0:
				all := true
				for _, d := range in.defs {
					all = all && dead(d)
				}
				keep[i] = !all
			}
			if !keep[i] {
				continue
			}
			for _, d := range in.defs {
				if d != 0 {
					live.clear(d)
				}
			}
			for _, u := range in.uses {
				if u != 0 {
					live.set(u)
				}
			}
		}
		kept := b.insts[:0]
		for i, in := range b.insts {
			if keep[i] {
				kept = append(kept, in)
			}
		}
		b.insts = kept
	}
}
