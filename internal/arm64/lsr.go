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
	iv      *ssa.Value // the Int parameter of the header
	ivIdx   int
	list    *ssa.Value // outside the loop, or a parameter of the header
	listIdx int        // its parameter index, or -1
	es      int64
	body    map[*ssa.Block]bool
	p       vreg
}

// planLSR finds the pointer induction variables of f (blocks in reverse
// postorder, idom their immediate dominators).
func (s *fstate) planLSR(blocks []*ssa.Block, idom map[*ssa.Block]*ssa.Block) {
	s.lsr = map[*ssa.Block][]*lsrPlan{}
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
		var latches, entries []*ssa.Edge
		for _, e := range h.Preds {
			if !reach[e.From] {
				continue
			}
			if body[e.From] {
				latches = append(latches, e)
			} else {
				entries = append(entries, e)
			}
		}
		if len(entries) == 0 {
			continue
		}
		// blocks that run on every iteration
		every := func(b *ssa.Block) bool {
			for _, e := range latches {
				if !dominates(b, e.From) {
					return false
				}
			}
			return true
		}
		for k, iv := range h.Params {
			if iv.Type.Kind != ssa.KInt || !ivStep(iv, k, latches) {
				continue
			}
			for _, b := range blocks {
				if !body[b] || !every(b) {
					continue
				}
				for _, v := range b.Instrs {
					if v.Op != ssa.OpCall || len(v.Args) < 2 || v.Args[1] != iv {
						continue
					}
					name := v.Aux.(string)
					if name != "list.index" && name != "list.set" {
						continue
					}
					l, li := s.stableList(v.Args[0], h, body, latches)
					if l == nil {
						continue
					}
					es := s.g.esize(l.Type.Elems[0])
					if es == 0 || es&(es-1) == 0 {
						continue // xs + i << k is one instruction already
					}
					dup := false
					for _, p := range s.lsr[h] {
						dup = dup || (p.iv == iv && p.list == l)
					}
					if !dup {
						s.lsr[h] = append(s.lsr[h], &lsrPlan{iv: iv, ivIdx: k, list: l, listIdx: li, es: es, body: body})
					}
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
	for _, p := range s.lsr[b] {
		p.p = s.newVreg(false)
	}
}

// lsrSeed starts the address memo of header b with its pointers.
func (s *fstate) lsrSeed(b *ssa.Block) {
	m := s.m()
	for _, p := range s.lsr[b] {
		l, i := s.vals[p.list][0], s.vals[p.iv][0]
		m.bases[[3]int64{int64(l), int64(i), p.es}] = p.p
	}
}

// lsrMoves adds the moves of the pointer parameters along e.
func (s *fstate) lsrMoves(e *ssa.Edge, dsts []vreg, srcs []src) ([]vreg, []src) {
	for _, p := range s.lsr[e.To] {
		a := e.Args[p.ivIdx]
		if p.body[e.From] {
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
