package arm64

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ssa"
)

// Loop optimizations (decision 0054): strength reduction and the facts
// of loop indexes (on the SSA, during selection), rotation at a bounds
// check (emission), load pairs (a text peephole), and latch layout (on
// the MIR, before allocation). Value ranges are in ranges.go.
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
	ivIdx   int        // its index
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
	plans    []*lsrPlan
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
				k, ok := ivIdx[x]
				if !ok {
					continue
				}
				l, li := s.stableList(v.Args[0], h, body, lp.latches)
				if l == nil {
					continue
				}
				es := s.g.esize(l.Type.Elems[0])
				if es == 0 || es&(es-1) == 0 {
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
// pointers, and the bounds of its induction variables from their initial
// values (an index that only grows is at least its smallest initial
// value: a step that would overflow faults, or was proved not to).
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
}

// lsrMoves adds the moves of the pointer parameters along e.
func (s *fstate) lsrMoves(e *ssa.Edge, dsts []vreg, srcs []src) ([]vreg, []src) {
	for _, p := range s.plans(e.To) {
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

// offset is a register that holds a + c (exactly: the add was checked or
// proved not to overflow). The relation holds wherever the register is
// used: its definition dominates the use, and a is not written between
// them (a parameter is written only on the edges into its block, which
// dominates the definition).
type offset struct {
	a vreg
	c int64
}

// offsetFacts records that v = a + n (OpAdd) or a - n (OpSub), and v's
// bounds from a's.
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
}

// offsetBase returns the address of element i of list l from the
// address of element a when i = a + c and the memo has a's: one add.
func (s *fstate) offsetBase(l, i vreg, es int64) (vreg, bool) {
	o, ok := s.offs[i]
	if !ok {
		return 0, false
	}
	b, ok := s.m().bases[[3]int64{int64(l), int64(o.a), es}]
	if !ok || o.c >= 4096 || o.c <= -4096 || es >= 4096 {
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

// Loop rotation of bounds checks: a jump back to a block that starts
// with a check (a compare with a fault exit, like the bounds check of a
// loop's xs[i]) does the check itself and branches into the block past
// it, so the loop's back edge is the conditional branch of the check
// (one taken branch and one instruction less per iteration). When the
// check fails the jump goes to the block, which checks again and faults.

// checkHead returns the check block b starts with, or nil.
func checkHead(b *mblock) *inst {
	if len(b.insts) < 2 {
		return nil
	}
	in := b.insts[0]
	if in.kind != iPlain || in.fault == nil || len(in.defs) != 0 || in.recover != "" ||
		!strings.HasPrefix(in.text, "cmp ") || strings.Contains(in.text, "\n") || invCond[in.fault.cond] == "" {
		return nil
	}
	return in
}

// rotateJump emits a jump to t, a block emitted before the current one
// that starts with a check, as the check and a branch past it; false
// when it does not apply.
func (s *fstate) rotateJump(b *strings.Builder, t *mblock, fr *frameInfo) bool {
	in := checkHead(t)
	if in == nil {
		return false
	}
	back := false
	for _, x := range s.order {
		if x == s.cur {
			break
		}
		back = back || x == t
	}
	if !back {
		return false
	}
	s.emitPlain(b, &inst{text: in.text, uses: in.uses}, fr)
	fmt.Fprintf(b, "\tb.%s %s_c\n\tb %s\n", invCond[in.fault.cond], s.label(t), s.label(t))
	return true
}

// pairLoads joins two loads of adjacent words through one base register
// into a load pair: `ldr x11, [x13, #24]` and `ldr x9, [x13, #32]` are
// `ldp x11, x9, [x13, #24]` (one instruction for a struct element's two
// integer fields). Integer registers only: pairs of d registers made
// nbody slower (bench/results/arm64-perf-2026-10-08.md).
func pairLoads(text string) string {
	lines := strings.Split(text, "\n")
	out := lines[:0]
	for i := 0; i < len(lines); i++ {
		if i+1 < len(lines) {
			a := ldrRE.FindStringSubmatch(lines[i])
			c := ldrRE.FindStringSubmatch(lines[i+1])
			if a != nil && c != nil && a[2] == c[2] && a[1] != c[1] && a[1] != a[2] {
				ka, _ := strconv.Atoi(a[3])
				kc, _ := strconv.Atoi(c[3])
				if kc == ka+8 && ka%8 == 0 && ka >= -512 && ka <= 504 {
					out = append(out, fmt.Sprintf("\tldp %s, %s, [%s, #%d]", a[1], c[1], a[2], ka))
					i++
					continue
				}
			}
		}
		out = append(out, lines[i])
	}
	return strings.Join(out, "\n")
}

var ldrRE = regexp.MustCompile(`^\tldr (x[0-9]+), \[(x[0-9]+|sp), #(-?[0-9]+)\]$`)

// placeLatches lays out the latch of a loop right after a block that
// branches either to it or into an inner loop: `for i in .. { if
// flags[i] { inner loop } }` falls through to the latch when the test
// fails (one taken branch per iteration, the back edge, instead of two),
// and the path into the inner loop, whose iterations outweigh the extra
// jumps around it, is the taken branch.
func (s *fstate) placeLatches() {
	succs := func(b *mblock) []*mblock {
		if t := b.term(); t != nil && (t.kind == iJump || t.kind == iCondBr || t.kind == iSwitch) {
			return t.targets
		}
		return nil
	}
	preds := map[*mblock][]*mblock{}
	for _, b := range append(append([]*mblock(nil), s.hot...), s.cold...) {
		for _, t := range succs(b) {
			preds[t] = append(preds[t], b)
		}
	}
	// natural loops of back edges found by depth first search
	bodies := map[*mblock]map[*mblock]bool{}
	state := map[*mblock]int{}
	var dfs func(b *mblock)
	dfs = func(b *mblock) {
		state[b] = 1
		for _, t := range succs(b) {
			switch state[t] {
			case 0:
				dfs(t)
			case 1:
				body := bodies[t]
				if body == nil {
					body = map[*mblock]bool{}
					bodies[t] = body
				}
				work := []*mblock{b}
				body[t] = true
				for len(work) > 0 {
					x := work[len(work)-1]
					work = work[:len(work)-1]
					if !body[x] {
						body[x] = true
						work = append(work, preds[x]...)
					}
				}
			}
		}
		state[b] = 2
	}
	dfs(s.hot[0])
	hot := map[*mblock]bool{}
	for _, b := range s.hot {
		hot[b] = true
	}
	// heavy reports whether x reaches the header of a loop nested in the
	// loop of h without going through h or the latch l
	heavy := func(x, h, l *mblock) bool {
		seen := map[*mblock]bool{h: true, l: true}
		work := []*mblock{x}
		for len(work) > 0 {
			y := work[len(work)-1]
			work = work[:len(work)-1]
			if seen[y] || !bodies[h][y] {
				continue
			}
			seen[y] = true
			if bodies[y] != nil && y != h {
				return true
			}
			work = append(work, succs(y)...)
		}
		return false
	}
	moved := map[*mblock]bool{}
	for i := 0; i < len(s.hot); i++ {
		b := s.hot[i]
		t := b.term()
		if t == nil || t.kind != iCondBr || t.targets[0] == t.targets[1] {
			continue
		}
		for k := 0; k < 2; k++ {
			l, x := t.targets[k], t.targets[1-k]
			// l and the small blocks it jumps through reach the loop's
			// header, or a block that only tests and branches to it
			// (emission tests at the jump)
			var chain []*mblock
			var h *mblock
			for y := l; h == nil && len(chain) < 4; {
				yt := y.term()
				if !hot[y] || moved[y] || y == b || len(y.insts) > 8 || yt == nil || yt.kind != iJump {
					break
				}
				chain = append(chain, y)
				j := yt.targets[0]
				switch {
				case bodies[j] != nil:
					h = j
				case len(j.insts) == 1 && j.insts[0].kind == iCondBr:
					for _, z := range j.insts[0].targets {
						if bodies[z] != nil && bodies[z][j] {
							h = z
						}
					}
					if h == nil {
						y = j
					}
				default:
					y = j
				}
			}
			// b the test of a loop x is in, or before a loop x heads
			// (while): falling into the loop is best
			if h == nil || !bodies[h][b] || !bodies[h][l] || bodies[x] != nil || bodies[b][x] || !heavy(x, h, l) {
				continue
			}
			if i+1 < len(s.hot) && s.hot[i+1] == l {
				break
			}
			// move the chain to right after b
			in := map[*mblock]bool{}
			for _, y := range chain {
				in[y] = true
				moved[y] = true
			}
			out := make([]*mblock, 0, len(s.hot))
			for _, y := range s.hot {
				if in[y] {
					continue
				}
				out = append(out, y)
				if y == b {
					out = append(out, chain...)
				}
			}
			s.hot = out
			i = -1 // positions changed: scan again (each latch moves once)
			break
		}
	}
}
