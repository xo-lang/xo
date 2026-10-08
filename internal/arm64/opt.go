package arm64

import (
	"math"

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
}

// m returns the memo of the current block.
func (s *fstate) m() *memo {
	if s.mm == nil || s.mm.blk != s.cur {
		s.mm = &memo{blk: s.cur, consts: map[any]vreg{}, lens: map[vreg]vreg{}, checked: map[[2]vreg]bool{},
			bases: map[[3]int64]vreg{}, loaded: map[vreg]memAt{}, lo: map[vreg]int64{}, hi: map[vreg]int64{}}
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
			if len(b.insts) != 1 || b.insts[0].kind != iJump || b.insts[0].targets[0] == b {
				return b
			}
			b = b.insts[0].targets[0]
		}
		return b
	}
	for _, b := range blocks {
		if t := b.term(); t != nil && (t.kind == iJump || t.kind == iCondBr) {
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
		if t := b.term(); t != nil && (t.kind == iJump || t.kind == iCondBr) {
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
		return (in.kind == iPlain && in.fault == nil && len(in.defs) > 0) || in.kind == iPmove
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
		bases: map[[3]int64]vreg{}, loaded: map[vreg]memAt{}, epoch: from.epoch, lo: map[vreg]int64{}, hi: map[vreg]int64{}}
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
func (s *fstate) bounds(v *ssa.Value) (int64, int64) {
	if n, ok := constInt(v); ok {
		return n, n
	}
	lo, hi := int64(math.MinInt64), int64(math.MaxInt64)
	ws, ok := s.vals[v]
	if !ok || len(ws) != 1 {
		return lo, hi
	}
	m := s.m()
	if x, ok := m.lo[ws[0]]; ok {
		lo = x
	}
	if x, ok := m.hi[ws[0]]; ok {
		hi = x
	}
	return lo, hi
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
	c := t.Args[0]
	if yes == no || !isCompare(c.Op) || c.Args[0].Type.Kind != ssa.KInt {
		return
	}
	op := c.Op
	if no {
		op = negateCmp[op]
	}
	x, y := c.Args[0], c.Args[1]
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
