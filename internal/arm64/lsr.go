package arm64

import (
	"fmt"
	"math"

	"github.com/xo-lang/xo/internal/ssa"
)

// Loop strength reduction and the facts of loop indexes (decision 0054).
//
// A pointer induction variable: in a loop whose header H has an Int
// parameter i that every back edge passes as i + c (c a constant), an
// element access xs[i] (list.index or list.set) of a list whose buffer
// cannot move in the loop gets a second parameter of H, p = xs + i *
// size, which the back edges advance by c * size and the entry edges
// compute once. The access then addresses [p, #24 + 8k] instead of
// recomputing xs + i * size (two instructions for 24, 40, or 72 byte
// elements). Bounds checks still compare i.
//
// The buffer cannot move when the list is defined outside the loop, or
// is a parameter of H that every back edge passes back through in place
// updates only (list.set, and parameters of blocks inside the loop that
// receive nothing else); list.push, list.pop, reuse, and calls end the
// chain.
//
// p stays equal to xs + i * size wherever H dominates: i and p are both
// parameters of H, written only by the parallel moves of the edges into
// H, together. So the element address memo (memo.bases) of H's blocks
// starts with p for (xs, i).

type lsrPlan struct {
	iv      *ssa.Value // the Int parameter of the header, or an index defined before the loop
	ivIdx   int        // iv's parameter index, or -1
	list    *ssa.Value // outside the loop, or a parameter of the header
	listIdx int        // its parameter index, or -1
	es      int64
	body    map[*ssa.Block]bool
	p       vreg
}

// lsrLoop is what is known of a loop header.
type lsrLoop struct {
	body             map[*ssa.Block]bool
	latches, entries []*ssa.Edge
	// up, down: Int parameters every back edge passes as i + c with c
	// >= 0 (with c <= 0)
	up, down []int
	// lists: List parameters every back edge passes back updated in
	// place (same buffer, same length)
	lists []int
	plans []*lsrPlan
}

// planLSR finds the loops of f and their pointer induction variables
// (blocks in reverse postorder, idom their immediate dominators).
func (s *fstate) planLSR(blocks []*ssa.Block, idom map[*ssa.Block]*ssa.Block) {
	s.lsr = map[*ssa.Block]*lsrLoop{}
	dominates := func(a, b *ssa.Block) bool {
		for b != nil {
			if a == b {
				return true
			}
			d, ok := idom[b]
			if !ok {
				return false
			}
			b = d
		}
		return false
	}
	reach := map[*ssa.Block]bool{}
	for _, b := range blocks {
		reach[b] = true
	}
	bodies := map[*ssa.Block]map[*ssa.Block]bool{}
	for _, b := range blocks {
		t := b.Term()
		if t == nil {
			continue
		}
		for _, e := range t.Succs {
			h := e.To
			if !dominates(h, b) || h == blocks[0] {
				continue
			}
			body := bodies[h]
			if body == nil {
				body = map[*ssa.Block]bool{h: true}
				bodies[h] = body
			}
			work := []*ssa.Block{b}
			for len(work) > 0 {
				x := work[len(work)-1]
				work = work[:len(work)-1]
				if body[x] || !reach[x] {
					continue
				}
				body[x] = true
				for _, pe := range x.Preds {
					work = append(work, pe.From)
				}
			}
		}
	}
	for _, h := range blocks {
		body := bodies[h]
		if body == nil {
			continue
		}
		lp := &lsrLoop{body: body}
		for _, e := range h.Preds {
			if !reach[e.From] {
				continue
			}
			if body[e.From] {
				lp.latches = append(lp.latches, e)
			} else {
				lp.entries = append(lp.entries, e)
			}
		}
		if len(lp.entries) == 0 {
			continue
		}
		s.lsr[h] = lp
		for k, p := range h.Params {
			switch p.Type.Kind {
			case ssa.KInt:
				up, down := true, true
				for _, e := range lp.latches {
					c, ok := stepOf(p, e.Args[k])
					up = up && ok && c >= 0
					down = down && ok && c <= 0
				}
				if up {
					lp.up = append(lp.up, k)
				}
				if down {
					lp.down = append(lp.down, k)
				}
			case ssa.KList:
				if l, _ := s.stableList(p, h, body, lp.latches); l == p {
					lp.lists = append(lp.lists, k)
				}
			}
		}
		// blocks that run on every iteration
		every := func(b *ssa.Block) bool {
			for _, e := range lp.latches {
				if !dominates(b, e.From) {
					return false
				}
			}
			return true
		}
		ivIdx := map[*ssa.Value]int{}
		for k, iv := range h.Params {
			if iv.Type.Kind == ssa.KInt && ivStep(iv, k, lp.latches) {
				ivIdx[iv] = k
			}
		}
		for _, b := range blocks {
			if !body[b] || !every(b) {
				continue
			}
			for _, v := range b.Instrs {
				if v.Op != ssa.OpCall || len(v.Args) < 2 {
					continue
				}
				if name := v.Aux.(string); name != "list.index" && name != "list.set" {
					continue
				}
				x := v.Args[1]
				k, isIV := ivIdx[x]
				if !isIV {
					// an index the loop does not change: its address
					// is computed once, before the loop
					if x.Op == ssa.OpConst || body[x.Block] {
						continue
					}
					k = -1
				}
				l, li := s.stableList(v.Args[0], h, body, lp.latches)
				if l == nil {
					continue
				}
				es := s.g.esize(l.Type.Elems[0])
				if es == 0 || (isIV && es&(es-1) == 0) {
					continue // xs + i << k is one instruction already
				}
				dup := false
				for _, p := range lp.plans {
					dup = dup || (p.iv == x && p.list == l)
				}
				if !dup {
					lp.plans = append(lp.plans, &lsrPlan{iv: x, ivIdx: k, list: l, listIdx: li, es: es, body: body})
				}
			}
		}
	}
}

// ivStep reports whether every back edge passes parameter iv (index k)
// as iv, iv + c, or iv - c.
func ivStep(iv *ssa.Value, k int, latches []*ssa.Edge) bool {
	for _, e := range latches {
		if _, ok := stepOf(iv, e.Args[k]); !ok {
			return false
		}
	}
	return true
}

// stepOf returns c when a is iv + c.
func stepOf(iv, a *ssa.Value) (int64, bool) {
	if a == iv {
		return 0, true
	}
	if (a.Op != ssa.OpAdd && a.Op != ssa.OpSub) || a.Type.Kind != ssa.KInt {
		return 0, false
	}
	x, y := a.Args[0], a.Args[1]
	if a.Op == ssa.OpAdd && y == iv {
		x, y = y, x
	}
	c, ok := constInt(y)
	if x != iv || !ok || c == math.MinInt64 {
		return 0, false
	}
	if a.Op == ssa.OpSub {
		c = -c
	}
	return c, true
}

// stableList returns the list whose buffer xs is in the loop of h
// (body), when that buffer cannot move in the loop: a list defined
// outside the loop, or a parameter of h that every back edge passes
// back updated in place. The index is the parameter's, or -1.
func (s *fstate) stableList(xs *ssa.Value, h *ssa.Block, body map[*ssa.Block]bool, latches []*ssa.Edge) (*ssa.Value, int) {
	l := stripSet(xs)
	if !body[l.Block] {
		return l, -1
	}
	k := -1
	for i, p := range h.Params {
		if p == l {
			k = i
		}
	}
	if k < 0 {
		return nil, 0
	}
	for _, e := range latches {
		if inPlace(e.Args[k], l, h, body, map[*ssa.Value]bool{}) != l {
			return nil, 0
		}
	}
	return l, k
}

func isParam(v *ssa.Value) bool {
	for _, p := range v.Block.Params {
		if p == v {
			return true
		}
	}
	return false
}

// stripSet follows list.set results to the list they update.
func stripSet(v *ssa.Value) *ssa.Value {
	for v.Op == ssa.OpCall && v.Aux == "list.set" {
		v = v.Args[0]
	}
	return v
}

// inPlace returns the value whose buffer v shares within the loop:
// through list.set and through parameters of blocks of the loop (not h)
// that receive one buffer only; nil when unknown.
func inPlace(v, root *ssa.Value, h *ssa.Block, body map[*ssa.Block]bool, seen map[*ssa.Value]bool) *ssa.Value {
	v = stripSet(v)
	if v == root || !body[v.Block] || v.Block == h || !isParam(v) {
		if v == root {
			return root
		}
		return nil
	}
	if seen[v] {
		return root // a cycle through v adds no other buffer
	}
	seen[v] = true
	k := -1
	for i, p := range v.Block.Params {
		if p == v {
			k = i
		}
	}
	for _, e := range v.Block.Preds {
		if inPlace(e.Args[k], root, h, body, seen) != root {
			return nil
		}
	}
	return root
}

// lsrParams adds the pointer parameters of header b.
func (s *fstate) lsrParams(b *ssa.Block) {
	for _, p := range s.plans(b) {
		p.p = s.newVreg(false)
	}
}

func (s *fstate) plans(b *ssa.Block) []*lsrPlan {
	if lp := s.lsr[b]; lp != nil {
		return lp.plans
	}
	return nil
}

// lsrHeader starts the memo of loop header b: the address memo with its
// pointers, the bounds of its induction variables from their initial
// values, and the lengths of the lists it updates in place.
func (s *fstate) lsrHeader(b *ssa.Block) {
	lp := s.lsr[b]
	if lp == nil {
		return
	}
	m := s.m()
	for _, p := range lp.plans {
		l, i := s.vals[p.list][0], s.vals[p.iv][0]
		m.bases[[3]int64{int64(l), int64(i), p.es}] = p.p
	}
	// what holds of argument a at the end of the block e leaves
	at := func(e *ssa.Edge, a *ssa.Value) (*memo, vreg) {
		em := s.endMemo[e.From]
		ws, ok := s.vals[a]
		if em == nil || !ok || len(ws) != 1 || ws[0] == 0 {
			return nil, 0
		}
		return em, ws[0]
	}
	for _, k := range lp.up {
		lo := int64(math.MaxInt64)
		for _, e := range lp.entries {
			x, ok := constInt(e.Args[k])
			if !ok {
				em, w := at(e, e.Args[k])
				if em == nil {
					lo = math.MinInt64
					break
				}
				if x, ok = em.lo[w]; !ok {
					lo = math.MinInt64
					break
				}
			}
			lo = min(lo, x)
		}
		if lo > math.MinInt64 {
			m.lo[s.vals[b.Params[k]][0]] = lo
		}
	}
	for _, k := range lp.down {
		hi := int64(math.MinInt64)
		for _, e := range lp.entries {
			x, ok := constInt(e.Args[k])
			if !ok {
				em, w := at(e, e.Args[k])
				if em == nil {
					hi = math.MaxInt64
					break
				}
				if x, ok = em.hi[w]; !ok {
					hi = math.MaxInt64
					break
				}
			}
			hi = max(hi, x)
		}
		if hi < math.MaxInt64 {
			m.hi[s.vals[b.Params[k]][0]] = hi
		}
	}
	// a list updated in place keeps the length it entered with
	for _, k := range lp.lists {
		var n vreg
		for _, e := range lp.entries {
			em, w := at(e, e.Args[k])
			var x vreg
			if em != nil {
				x = em.lens[w]
			}
			if x == 0 || (n != 0 && x != n) {
				n = 0
				break
			}
			n = x
		}
		if n != 0 {
			m.lens[s.vals[b.Params[k]][0]] = n
		}
	}
}

// lsrMoves adds the moves of the pointer parameters along e.
func (s *fstate) lsrMoves(e *ssa.Edge, dsts []vreg, srcs []src) ([]vreg, []src) {
	for _, p := range s.plans(e.To) {
		a := p.iv
		if p.ivIdx >= 0 {
			a = e.Args[p.ivIdx]
		}
		if p.body[e.From] {
			if p.ivIdx < 0 {
				continue // loop invariant
			}
			c, _ := stepOf(p.iv, a)
			if c == 0 {
				continue
			}
			n := c * p.es
			var d vreg
			switch {
			case imm12(n):
				d = s.def1(false, fmt.Sprintf("add $d0, $u0, #%d", n), p.p)
			case imm12(-n):
				d = s.def1(false, fmt.Sprintf("sub $d0, $u0, #%d", -n), p.p)
			default:
				d = s.def1(false, "add $d0, $u0, $u1", p.p, s.cimm(n))
			}
			dsts = append(dsts, p.p)
			srcs = append(srcs, rsrc(d))
			continue
		}
		var l vreg
		if p.listIdx >= 0 {
			l = s.words(e.Args[p.listIdx])[0]
		} else {
			l = s.vals[p.list][0]
		}
		dsts = append(dsts, p.p)
		srcs = append(srcs, rsrc(s.elemBase(s.real(l), s.word(a), p.es)))
	}
	return dsts, srcs
}

// indexFacts records what a passed bounds check `i <u len` proves about
// i: 0 <= i <= MaxInt64 - 1 (a length is at most MaxInt64), so `i + 1`
// and `i - 1` after it cannot overflow.
func (s *fstate) indexFacts(i vreg) {
	m := s.m()
	lo, ok := m.lo[i]
	if !ok || lo < 0 {
		lo = 0
	}
	hi, ok := m.hi[i]
	if !ok || hi > math.MaxInt64-1 {
		hi = math.MaxInt64 - 1
	}
	m.lo[i], m.hi[i] = lo, hi
}

// ltFact records x < y (signed) when the branch into the current block
// proved x op y.
func (s *fstate) ltFact(x *ssa.Value, op ssa.Op, y *ssa.Value) {
	switch op {
	case ssa.OpGt:
		x, y = y, x
	case ssa.OpLt:
	default:
		return
	}
	xw, ok1 := s.vals[x]
	yw, ok2 := s.vals[y]
	if !ok1 || !ok2 || len(xw) != 1 || len(yw) != 1 || xw[0] == 0 || yw[0] == 0 {
		return
	}
	s.m().lt[[2]vreg{xw[0], yw[0]}] = true
}

// provenIndex reports whether 0 <= i < len(l) is known: i >= 0 from its
// bounds and i < n from a branch, n the register of l's length.
func (s *fstate) provenIndex(l, i vreg) bool {
	m := s.m()
	n, ok := m.lens[l]
	if !ok {
		return false
	}
	lo, ok := m.lo[i]
	return ok && lo >= 0 && m.lt[[2]vreg{i, n}]
}

// offset is a register that holds a + c (exactly: the add was checked or
// proved not to overflow). The relation holds wherever the register is
// used: its definition dominates the use, and a is not written between
// them (a parameter is written only on the edges into its block, which
// dominates the definition).
type offset struct {
	a vreg
	c int64
}

// offsetFacts records that v = a + n (OpAdd) or a - n (OpSub), and what
// follows from a's facts: v's bounds, and v < y when a < y and n <= 0.
func (s *fstate) offsetFacts(v, a *ssa.Value, n int64) {
	if v.Type.Kind != ssa.KInt {
		return
	}
	if v.Op == ssa.OpSub {
		if n == math.MinInt64 {
			return
		}
		n = -n
	}
	aw, ok1 := s.vals[a]
	dw, ok2 := s.vals[v]
	if !ok1 || !ok2 || len(aw) != 1 || len(dw) != 1 || aw[0] == 0 || dw[0] == 0 || aw[0] == dw[0] {
		return
	}
	ar, d := aw[0], dw[0]
	if s.offs == nil {
		s.offs = map[vreg]offset{}
	}
	s.offs[d] = offset{ar, n}
	m := s.m()
	if lo, ok := m.lo[ar]; ok && (n >= 0 || lo >= math.MinInt64-n) {
		if n <= 0 || lo <= math.MaxInt64-n {
			m.lo[d] = lo + n
		}
	}
	if hi, ok := m.hi[ar]; ok && (n <= 0 || hi <= math.MaxInt64-n) {
		if n >= 0 || hi >= math.MinInt64-n {
			m.hi[d] = hi + n
		}
	}
	if n <= 0 {
		for k := range m.lt {
			if k[0] == ar {
				m.lt[[2]vreg{d, k[1]}] = true
			}
		}
	}
}

// offsetBase returns the address of element i of list l from the
// address of element a when i = a + c and the memo has a's: one add.
func (s *fstate) offsetBase(l, i vreg, es int64) (vreg, bool) {
	o, ok := s.offs[i]
	if !ok {
		return 0, false
	}
	b, ok := s.m().bases[[3]int64{int64(l), int64(o.a), es}]
	if !ok || o.c > math.MaxInt64/4096 || o.c < -math.MaxInt64/4096 {
		return 0, false
	}
	n := o.c * es
	switch {
	case n == 0:
		return b, true
	case imm12(n):
		return s.def1(false, fmt.Sprintf("add $d0, $u0, #%d", n), b), true
	case imm12(-n):
		return s.def1(false, fmt.Sprintf("sub $d0, $u0, #%d", -n), b), true
	}
	return 0, false
}
