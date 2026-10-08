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
}

// m returns the memo of the current block.
func (s *fstate) m() *memo {
	if s.mm == nil || s.mm.blk != s.cur {
		s.mm = &memo{blk: s.cur, consts: map[any]vreg{}, lens: map[vreg]vreg{}, checked: map[[2]vreg]bool{},
			bases: map[[3]int64]vreg{}, loaded: map[vreg]memAt{}, lo: map[vreg]int64{}, hi: map[vreg]int64{}, lowZero: map[vreg]int{}}
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
		bases: map[[3]int64]vreg{}, loaded: map[vreg]memAt{}, epoch: from.epoch, lo: map[vreg]int64{}, hi: map[vreg]int64{}, lowZero: map[vreg]int{}}
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

// threadConst sends a jump to a block that only tests a register
// straight to the test's outcome when the jumping block set that
// register to a constant (the tag of an Option a pop or get built,
// tested at once by `??` or `if let`).
func (s *fstate) threadConst() {
	blocks := append(append([]*mblock(nil), s.hot...), s.cold...)
	for _, p := range blocks {
		t := p.term()
		if t == nil || t.kind != iJump {
			continue
		}
		j := t.targets[0]
		if len(j.insts) != 1 || j.insts[0].kind != iCondBr {
			continue
		}
		br := j.insts[0]
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
		val, ok := int64(0), false
		for _, in := range p.insts[:len(p.insts)-1] {
			for i, d := range in.defs {
				if d != r {
					continue
				}
				ok = in.kind == iPmove && in.srcs[i].kind == sImm
				if ok {
					val = in.srcs[i].imm
				}
			}
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
	}
}

// threadCset turns `t = cmp ...; cset; r = t; jump J` where J only
// branches on r into a branch on the comparison itself (an inlined
// predicate returns its Bool through the continuation's parameter).
func (s *fstate) threadCset() {
	blocks := append(append([]*mblock(nil), s.hot...), s.cold...)
	for _, p := range blocks {
		t := p.term()
		if t == nil || t.kind != iJump || len(p.insts) < 3 {
			continue
		}
		j := t.targets[0]
		if len(j.insts) != 1 || j.insts[0].kind != iCondBr || j.insts[0].cond != "cbnz" || j == p {
			continue
		}
		r := j.insts[0].uses[0]
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
		if def.kind != iPlain || len(def.defs) != 1 || def.defs[0] != src || def.fault != nil {
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
	}
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
