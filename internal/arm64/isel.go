package arm64

import (
	"fmt"
	"math"
	"strings"

	"github.com/xo-lang/xo/internal/ssa"
)

// fstate is the state of one function: instruction selection, then
// register allocation and emission.
type fstate struct {
	g      *gen
	f      *ssa.Func
	fp     []bool // class of each vreg (index 0 is xzr)
	hot    []*mblock
	cold   []*mblock
	nblock int
	cur    *mblock
	vals   map[*ssa.Value][]vreg
	bmap   map[*ssa.Block]*mblock
	nuses  map[*ssa.Value]int
	users  map[*ssa.Value]*ssa.Value // the last user of each value
	// lenParam maps the register of a List block parameter to the
	// register of its length, a parameter too
	lenParam map[vreg]vreg
	// endMemo is the memo at the end of each lowered SSA block
	endMemo map[*ssa.Block]*memo
	lazy    map[*ssa.Value]bool     // compares lowered into the branch that uses them
	out     bool                    // the function uses the $OUT slot
	cancel  map[*ssa.Value]bool     // releases cancelled by a retain (rc.go cancelRC)
	lsr     map[*ssa.Block]*lsrLoop // loop headers: induction variables (lsr.go)
	ranges  map[*ssa.Value]interval // intervals of Int values (ranges.go)
	offs    map[vreg]offset         // registers that are another plus a constant (lsr.go)
	// offBases: the Int values a list index adds a constant to (xs[i]
	// beside xs[i + 1]): their elements are addressed from xs + i * size
	// (elemAddr, decision 0208)
	offBases map[*ssa.Value]bool
	dupOK    map[*mblock]bool // moved latches copied at a branch into them (lsr.go dupLatch)
	// Go mode (goasm.go): the length register of each List parameter,
	// and the parameters a reuse makes unique (the Go wrapper does it)
	goLens map[vreg]vreg
	// register allocation results
	loc     []loc
	order   []*mblock
	ninst   int
	slots   int
	base    int // farBase when the frame is past maxFrame, or 0
	saved   []int
	outArgs int
	frame   int
	stubs   strings.Builder
	nstub   int
	npoll   int // Go mode: polls emitted (goEmitPoll labels)
	mm      *memo
	// landing pads (decision 0085): curPad is the pad of the value being
	// lowered (its calls and fault stubs get it); pads are the SSA blocks
	// landing pads enter; sites are the call site table entries made at
	// emission (stubs after the body)
	curPad    *mblock
	pads      map[*ssa.Block]bool
	sites     []callSite
	stubSites []callSite
	// large `var` parameters passed by reference (byref.go): what the
	// homes hold, the address register of each such parameter, the
	// frame's memory for other by reference arguments (areas by type,
	// memBytes in all), and the call results left in memory
	held     *heldInfo
	refPtr   map[int]vreg
	areas    map[*ssa.Type][]int
	memBytes int
	lazyRes  map[hkey]bool
	stable   map[*ssa.Value]vreg // read only parameters passed by reference: their address
}

// callSite is a call that unwinds to a landing pad: the labels around
// its `bl` and the pad's block.
type callSite struct {
	from, to string
	pad      *mblock
}

func (g *gen) function(f *ssa.Func) {
	s := &fstate{g: g, f: f, fp: []bool{false}, vals: map[*ssa.Value][]vreg{}, bmap: map[*ssa.Block]*mblock{},
		nuses: map[*ssa.Value]int{}, users: map[*ssa.Value]*ssa.Value{}, lenParam: map[vreg]vreg{}, endMemo: map[*ssa.Block]*memo{}, lazy: map[*ssa.Value]bool{},
		goLens: map[vreg]vreg{}}
	s.isel()
	s.dce()
	s.threadConst()
	s.threadCset()
	s.thread()
	if !g.goMode {
		// a jump table's addresses are data in the text section
		s.jumpTables()
	}
	s.dce()
	s.liveDCE()
	s.selectCompares()
	s.hoistConsts()
	s.reuseFlags()
	s.placeLatches()
	if g.goMode {
		s.insertPolls()
	}
	s.regalloc()
	s.emit()
}

func (s *fstate) newVreg(fp bool) vreg {
	s.fp = append(s.fp, fp)
	return vreg(len(s.fp) - 1)
}

func (s *fstate) newVregs(layout []bool) []vreg {
	out := make([]vreg, len(layout))
	for i, fp := range layout {
		out[i] = s.newVreg(fp)
	}
	return out
}

func (s *fstate) newBlock(cold bool) *mblock {
	b := &mblock{id: s.nblock, cold: cold}
	s.nblock++
	if cold {
		s.cold = append(s.cold, b)
	} else {
		s.hot = append(s.hot, b)
	}
	return b
}

func (s *fstate) add(in *inst) *inst {
	if in.kind == iCall && s.curPad != nil {
		in.pad = s.curPad
	}
	s.cur.insts = append(s.cur.insts, in)
	return in
}

// real returns u, or a fresh register holding zero when u is xzr (for
// operand positions where register 31 is not the zero register).
func (s *fstate) real(u vreg) vreg {
	if u != 0 {
		return u
	}
	return s.def1(false, "mov $d0, #0")
}

// plain adds an instruction with a template.
func (s *fstate) plain(text string, defs []vreg, uses ...vreg) *inst {
	for i, u := range uses {
		uses[i] = s.real(u)
	}
	return s.add(&inst{kind: iPlain, text: text, defs: defs, uses: uses})
}

// def1 adds an instruction defining one new register of class fp.
func (s *fstate) def1(fp bool, text string, uses ...vreg) vreg {
	d := s.newVreg(fp)
	s.plain(text, []vreg{d}, uses...)
	return d
}

// check adds a flag setting instruction with a fault side exit.
func (s *fstate) check(text string, uses []vreg, cond, fn string, args ...src) {
	in := s.plain(text, nil, uses...)
	in.fault = &fault{cond: cond, fn: s.g.sym(fn), args: args}
	in.pad = s.curPad
	for _, a := range args {
		if a.kind == sReg && a.v != 0 {
			in.uses = append(in.uses, a.v)
		}
	}
}

// checked adds an instruction defining one register, with a fault side
// exit; the result never shares a register with the operands, which the
// fault stub reads.
//
// recover names how the fault stub gets back an operand whose register
// the result took ("add", "sub", "neg": the operation is undone); with no
// recover, the result is early (never shares a register with an operand).
func (s *fstate) checked(fp bool, text string, uses []vreg, recover string, cond, fn string, args ...src) vreg {
	d := s.newVreg(fp)
	in := s.plain(text, []vreg{d}, uses...)
	if recover == "" || (len(uses) == 2 && uses[0] == uses[1]) {
		in.early = true
	} else {
		in.recover = recover
	}
	in.fault = &fault{cond: cond, fn: s.g.sym(fn), args: args}
	in.pad = s.curPad
	for _, a := range args {
		if a.kind == sReg && a.v != 0 {
			in.uses = append(in.uses, a.v)
		}
	}
	return d
}

// call adds a call of symbol sym (already prefixed) and returns the
// result registers.
func (s *fstate) call(sym string, args []src, res []bool) []vreg {
	if s.g.goMode {
		fail("a call of %s", sym)
	}
	defs := s.newVregs(res)
	in := &inst{kind: iCall, sym: sym, srcs: args, defs: defs}
	for _, a := range args {
		if a.kind == sReg && a.v != 0 {
			in.uses = append(in.uses, a.v)
		}
		if a.kind == sFrame {
			s.out = true
		}
	}
	s.add(in)
	s.clobber()
	return defs
}

func (s *fstate) rt(name string, args []src, res ...bool) []vreg {
	if s.g.goMode {
		fail("runtime call %s", name)
	}
	return s.call(s.g.sym(name), args, res)
}

func regs(vs ...vreg) []src {
	out := make([]src, len(vs))
	for i, v := range vs {
		out[i] = rsrc(v)
	}
	return out
}

// movImm returns the template that puts n in $d0.
func movImm(n int64) string {
	if n >= -65536 && n < 65536 {
		return fmt.Sprintf("mov $d0, #%d", n)
	}
	if int64(int32(n)) == n && n < 0 {
		// fall through to the general sequence
	}
	var lines []string
	u := uint64(n)
	inv := ^u
	// prefer movn when most chunks are all ones
	ones := 0
	for k := 0; k < 4; k++ {
		if (u>>(16*k))&0xffff == 0xffff {
			ones++
		}
	}
	if ones >= 2 {
		first := true
		for k := 0; k < 4; k++ {
			c := (inv >> (16 * k)) & 0xffff
			if first && c != 0 {
				lines = append(lines, fmt.Sprintf("movn $d0, #%d, lsl #%d", c, 16*k))
				first = false
			}
		}
		if first {
			return "mov $d0, #-1"
		}
		for k := 0; k < 4; k++ {
			c := (u >> (16 * k)) & 0xffff
			ci := (inv >> (16 * k)) & 0xffff
			if c != 0xffff && !strings.Contains(lines[0], fmt.Sprintf("#%d, lsl #%d", ci, 16*k)) {
				lines = append(lines, fmt.Sprintf("movk $d0, #%d, lsl #%d", c, 16*k))
			}
		}
		return strings.Join(lines, "\n")
	}
	first := true
	for k := 0; k < 4; k++ {
		c := (u >> (16 * k)) & 0xffff
		if c == 0 {
			continue
		}
		if first {
			lines = append(lines, fmt.Sprintf("movz $d0, #%d, lsl #%d", c, 16*k))
			first = false
		} else {
			lines = append(lines, fmt.Sprintf("movk $d0, #%d, lsl #%d", c, 16*k))
		}
	}
	if first {
		return "mov $d0, #0"
	}
	return strings.Join(lines, "\n")
}

func (s *fstate) imm(n int64) vreg { return s.konst(s.def1(false, movImm(n))) }

// konst marks the instruction that just defined d as a constant (loop
// invariant: hoistConsts may move it out of a loop).
func (s *fstate) konst(d vreg) vreg {
	s.cur.insts[len(s.cur.insts)-1].konst = true
	return d
}

// cimm is imm, shared within the block.
func (s *fstate) cimm(n int64) vreg {
	m := s.m()
	if r, ok := m.consts[n]; ok {
		return r
	}
	r := s.imm(n)
	m.consts[n] = r
	return r
}

// fimm puts the double with bits b in a floating point register.
func (s *fstate) fimm(bits uint64) vreg {
	if bits == 0 {
		return s.konst(s.def1(true, "fmov $d0, xzr"))
	}
	t := s.imm(int64(bits))
	return s.konst(s.def1(true, "fmov $d0, $u0", t))
}

// symAddr puts the address of a symbol in a register.
func (s *fstate) symAddr(sym string) vreg {
	g := s.g
	if g.goMode {
		fail("the address of %s", sym)
	}
	if g.os == Darwin {
		return s.konst(s.def1(false, fmt.Sprintf("adrp $d0, %s@PAGE\nadd $d0, $d0, %s@PAGEOFF", sym, sym)))
	}
	return s.konst(s.def1(false, fmt.Sprintf("adrp $d0, %s\nadd $d0, $d0, :lo12:%s", sym, sym)))
}

// words returns the registers of SSA value v (constants are put in
// registers where they are used).
func (s *fstate) words(v *ssa.Value) []vreg {
	if v.Op == ssa.OpConst {
		m := s.m()
		if r, ok := m.consts[v.Aux]; ok {
			if x, isStr := v.Aux.(string); isStr {
				return []vreg{r, s.cimm(int64(len(x)))}
			}
			return []vreg{r}
		}
		var r vreg
		switch x := v.Aux.(type) {
		case int64:
			r = s.cimm(x)
		case float64:
			r = s.fimm(math.Float64bits(x))
		case bool:
			r = 0
			if x {
				r = s.cimm(1)
			} else {
				r = s.cimm(0)
			}
		case string:
			r = s.symAddr(s.g.strObj(x))
			m.consts[v.Aux] = r
			return []vreg{r, s.cimm(int64(len(x)))}
		default:
			fail("constant %T", v.Aux)
		}
		m.consts[v.Aux] = r
		return []vreg{r}
	}
	if ptr, ok := s.stableOf(v); ok {
		return s.loadWords(ptr, 0, s.g.layout(v.Type))
	}
	if m, ok := s.memOnlyOf(v); ok {
		// held in memory at every use: loaded where it is needed whole
		return s.loadWords(s.held.mems[m].ptr, 0, s.g.layout(v.Type))
	}
	if ws, ok := s.vals[v]; ok {
		return ws
	}
	if s.g.nwords(v.Type) == 0 {
		return nil
	}
	fail("%s in %s used before it is defined", v.Name(), s.f.Name)
	return nil
}

func (s *fstate) word(v *ssa.Value) vreg { return s.real(s.words(v)[0]) }

// constInt reports the value of an Int constant.
func constInt(v *ssa.Value) (int64, bool) {
	if v.Op != ssa.OpConst {
		return 0, false
	}
	n, ok := v.Aux.(int64)
	return n, ok
}

// negConst reports the value of an Int negation of an Int constant
// other than the minimum (a negative literal: `-7` is a negation in the
// SSA).
func negConst(v *ssa.Value) (int64, bool) {
	if v.Op != ssa.OpNeg || v.Type.Kind != ssa.KInt {
		return 0, false
	}
	n, ok := constInt(v.Args[0])
	if !ok || n == math.MinInt64 {
		return 0, false
	}
	return -n, true
}

func imm12(n int64) bool { return n >= 0 && n < 4096 }

// rawImmOff reports whether off is the scaled unsigned immediate of a
// load or store of w bytes (1, 2, 4, or 8): a multiple of w below 4096 *
// w.
func rawImmOff(off, w int64) bool { return w > 0 && off >= 0 && off%w == 0 && off/w < 4096 }

// ---------------------------------------------------------------------------
// Functions and blocks

// rpo returns the blocks of f in reverse postorder from the entry.
func rpo(f *ssa.Func) []*ssa.Block {
	seen := map[*ssa.Block]bool{}
	var post []*ssa.Block
	var walk func(b *ssa.Block)
	walk = func(b *ssa.Block) {
		seen[b] = true
		if t := b.Term(); t != nil {
			for i := len(t.Succs) - 1; i >= 0; i-- {
				if to := t.Succs[i].To; !seen[to] {
					walk(to)
				}
			}
		}
		post = append(post, b)
	}
	walk(f.Blocks[0])
	out := make([]*ssa.Block, len(post))
	for i, b := range post {
		out[len(post)-1-i] = b
	}
	return out
}

func (s *fstate) isel() {
	f := s.f
	for _, b := range f.Blocks {
		for _, v := range b.Instrs {
			for _, a := range v.Args {
				s.nuses[a]++
				s.users[a] = v
			}
			if v.Op == ssa.OpCall && len(v.Args) >= 2 && (v.Aux == "list.index" || v.Aux == "list.set" || v.Aux == "list.take") {
				if x := v.Args[1]; x.Op == ssa.OpAdd || x.Op == ssa.OpSub {
					for j := 0; j < 2; j++ {
						if _, ok := constInt(x.Args[1-j]); ok && (j == 0 || x.Op == ssa.OpAdd) {
							if s.offBases == nil {
								s.offBases = map[*ssa.Value]bool{}
							}
							s.offBases[x.Args[j]] = true
							break
						}
					}
				}
			}
			for _, e := range v.Succs {
				for _, a := range e.Args {
					s.nuses[a]++
				}
			}
		}
	}
	// the blocks landing pads enter (decision 0085)
	s.pads = map[*ssa.Block]bool{}
	for _, b := range f.Blocks {
		for _, v := range b.Instrs {
			if v.Pad != nil && !s.g.goMode {
				s.pads[v.Pad.Block.Term().Succs[0].To] = true
			}
		}
	}
	blocks := rpo(f)
	// what only landing pads reach (the cleanup, the unwind exit) is
	// laid out with the cold code too
	cold := map[*ssa.Block]bool{}
	for b := range s.pads {
		cold[b] = true
	}
	for changed := len(cold) > 0; changed; {
		changed = false
		for _, b := range blocks {
			if cold[b] || len(b.Preds) == 0 {
				continue
			}
			all := true
			for _, e := range b.Preds {
				all = all && cold[e.From]
			}
			if all {
				cold[b] = true
				changed = true
			}
		}
	}
	var ptrs map[int]vreg
	if !s.g.goMode {
		ptrs = s.planHeld()
	}
	idom := dominators(blocks)
	s.planLSR(blocks, idom)
	s.computeRanges(blocks, idom)
	for _, b := range blocks {
		s.bmap[b] = &mblock{}
		if b != f.Blocks[0] {
			for _, p := range b.Params {
				s.vals[p] = s.newVregs(s.g.layout(p.Type))
				if p.Type.Kind == ssa.KList {
					// the list's length travels with it (see opt.go)
					s.lenParam[s.vals[p][0]] = s.newVreg(false)
				}
			}
			s.lsrParams(b)
			for _, p := range b.Params {
				if _, ok := s.memOnlyOf(p); ok {
					delete(s.vals, p)
				}
			}
		}
	}
	for i, b := range blocks {
		mb := s.bmap[b]
		mb.id = s.nblock
		s.nblock++
		s.cur = mb
		if cold[b] {
			mb.cold = true
			s.cold = append(s.cold, mb)
		} else {
			s.hot = append(s.hot, mb)
		}
		if s.pads[b] {
			// entered by the unwinder after a fault: nothing computed
			// before it in registers is known to hold (the operation that
			// faulted did not finish)
			s.mm = nil
		}
		if s.pads[b] {
		} else if p, c, truth, ok := s.joinSource(b); ok {
			// b is reached from p through a join that only branches on
			// the Bool p passes (`a && b`, decision 0184): p's facts
			// hold, but p does not dominate b, so the registers it
			// computed come only from b's dominator, as below (gotchas/
			// arm64-vreg-read-before-write-break-flag.md)
			if d := idom[b]; d != nil && s.endMemo[d] != nil {
				s.inherit(s.endMemo[d])
				s.clobber()
			}
			s.joinFacts(s.endMemo[p])
			s.condFacts(c, truth)
		} else if len(b.Preds) == 1 && s.endMemo[b.Preds[0].From] != nil {
			// the only predecessor dominates b: what it computed holds
			s.inherit(s.endMemo[b.Preds[0].From])
			s.branchFacts(b)
		} else if d := idom[b]; d != nil && s.endMemo[d] != nil {
			// so do the facts of the immediate dominator, except what
			// memory held (a path from it may have stored)
			s.inherit(s.endMemo[d])
			s.clobber()
		}
		for _, p := range b.Params {
			if p.Type.Kind != ssa.KList || i == 0 {
				continue
			}
			if c, ok := s.lenParam[s.vals[p][0]]; ok {
				s.m().lens[s.vals[p][0]] = c
			}
		}
		s.lsrHeader(b)
		if i == 0 && s.g.goMode {
			s.goEntry(b)
		} else if i == 0 {
			var defs []vreg
			for i, p := range b.Params {
				if ptr, ok := ptrs[i]; ok {
					defs = append(defs, ptr)
					continue
				}
				ws := s.newVregs(s.g.layout(p.Type))
				s.vals[p] = ws
				defs = append(defs, ws...)
			}
			s.add(&inst{kind: iEntry, defs: defs})
			if ptrs != nil {
				s.byRefEntry(ptrs)
			}
			for _, p := range b.Params {
				if p.Type.Kind == ssa.KList {
					// known for the whole function (dead code removes
					// the load when nothing uses it)
					s.listLen(s.vals[p][0])
				}
			}
		}
		for _, v := range b.Instrs {
			s.instr(v)
		}
	}
}

// edge returns the block a branch along e goes to: the target itself, or
// a new block with the parallel move of e's arguments.
func (s *fstate) edge(e *ssa.Edge) *mblock {
	to := s.bmap[e.To]
	dsts, srcs := s.edgeMoves(e)
	if len(dsts) == 0 {
		return to
	}
	save := s.cur
	b := s.newBlock(false)
	s.cur = b
	s.pmove(dsts, srcs)
	s.add(&inst{kind: iJump, targets: []*mblock{to}})
	s.cur = save
	return b
}

func (s *fstate) edgeMoves(e *ssa.Edge) ([]vreg, []src) {
	var dsts []vreg
	var srcs []src
	for i, p := range e.To.Params {
		if _, ok := s.memOnlyOf(p); ok {
			continue // passed in place
		}
		pw := s.vals[p]
		aw := s.words(e.Args[i])
		for k := range pw {
			if pw[k] == aw[k] {
				continue
			}
			dsts = append(dsts, pw[k])
			srcs = append(srcs, rsrc(aw[k]))
		}
		if p.Type.Kind != ssa.KList {
			continue
		}
		if c, ok := s.lenParam[pw[0]]; ok {
			if n := s.listLen(aw[0]); n != c {
				dsts = append(dsts, c)
				srcs = append(srcs, rsrc(n))
			}
		}
	}
	return s.lsrMoves(e, dsts, srcs)
}

func (s *fstate) pmove(dsts []vreg, srcs []src) {
	if len(dsts) == 0 {
		return
	}
	in := &inst{kind: iPmove, defs: dsts, srcs: srcs}
	for _, a := range srcs {
		if a.kind == sReg && a.v != 0 {
			in.uses = append(in.uses, a.v)
		}
	}
	s.add(in)
}

// ---------------------------------------------------------------------------
// Instructions

var intCond = map[ssa.Op]string{ssa.OpEq: "eq", ssa.OpNe: "ne", ssa.OpLt: "lt", ssa.OpLe: "le", ssa.OpGt: "gt", ssa.OpGe: "ge"}
var uintCond = map[ssa.Op]string{ssa.OpEq: "eq", ssa.OpNe: "ne", ssa.OpLt: "lo", ssa.OpLe: "ls", ssa.OpGt: "hi", ssa.OpGe: "hs"}

// floatCond are the conditions after fcmp for IEEE `==` (false when
// either operand is NaN) and `!=` (unordered or not equal). The ordering
// compares use floatOrder.
var floatCond = map[ssa.Op]string{ssa.OpEq: "eq", ssa.OpNe: "ne"}

// isPosZero reports whether v is the constant +0.0 (fcmp's #0.0).
func isPosZero(v *ssa.Value) bool {
	x, ok := v.Aux.(float64)
	return v.Op == ssa.OpConst && ok && x == 0 && !math.Signbit(x)
}

// notNaNConst reports whether v is a Float constant other than NaN.
func notNaNConst(v *ssa.Value) bool {
	x, ok := v.Aux.(float64)
	return v.Op == ssa.OpConst && ok && !math.IsNaN(x)
}

// floatOrder lowers `<`, `<=`, `>`, `>=` on Floats to the total order of
// std.md 1 (decisions/0053): NaN after every other value and equal to
// NaN, -0.0 equal to 0.0. After `fcmp a, b` the flags are N for less, ZC
// for equal, C for greater, CV for unordered; mi, ls, gt, ge are false
// on NaN, lt, le, hi, hs true.
//
//   - b a constant other than NaN: a < b is mi, a <= b ls, a > b hi
//     (greater or a NaN), a >= b hs.
//   - a a constant other than NaN: a < b is lt (less or b NaN), a <= b
//     le, a > b gt, a >= b ge.
//   - otherwise x < y is `fcmp x, x; fccmp x, y, #0, vc` then lt: when x
//     is a number, the second compare decides and lt holds for less or
//     unordered (y NaN); when x is NaN, the flags are 0000 and lt is
//     false. x >= y is the same flags with ge, a > b and a <= b swap the
//     operands.
func (s *fstate) floatOrder(op ssa.Op, a, b *ssa.Value) (text string, uses []vreg, cond string) {
	if notNaNConst(b) {
		cond = map[ssa.Op]string{ssa.OpLt: "mi", ssa.OpLe: "ls", ssa.OpGt: "hi", ssa.OpGe: "hs"}[op]
		if isPosZero(b) {
			return "fcmp $u0, #0.0", []vreg{s.word(a)}, cond
		}
		return "fcmp $u0, $u1", []vreg{s.word(a), s.word(b)}, cond
	}
	if notNaNConst(a) {
		cond = map[ssa.Op]string{ssa.OpLt: "lt", ssa.OpLe: "le", ssa.OpGt: "gt", ssa.OpGe: "ge"}[op]
		return "fcmp $u0, $u1", []vreg{s.word(a), s.word(b)}, cond
	}
	x, y := a, b
	cond = "lt"
	switch op {
	case ssa.OpGt:
		x, y = b, a
	case ssa.OpLe:
		x, y = b, a
		cond = "ge"
	case ssa.OpGe:
		cond = "ge"
	}
	return "fcmp $u0, $u0\nfccmp $u0, $u1, #0, vc", []vreg{s.word(x), s.word(y)}, cond
}

var swapCond = map[string]string{"eq": "eq", "ne": "ne", "lt": "gt", "gt": "lt", "le": "ge", "ge": "le",
	"lo": "hi", "hi": "lo", "ls": "hs", "hs": "ls"}

var opChar = map[ssa.Op]int64{ssa.OpAdd: '+', ssa.OpSub: '-', ssa.OpMul: '*', ssa.OpDiv: '/', ssa.OpRem: '%'}

func isCompare(op ssa.Op) bool {
	switch op {
	case ssa.OpEq, ssa.OpNe, ssa.OpLt, ssa.OpLe, ssa.OpGt, ssa.OpGe:
		return true
	}
	return false
}

// compare lowers the flag setting part of a comparison and returns the
// condition under which it holds.
func (s *fstate) compare(v *ssa.Value) (text string, uses []vreg, cond string) {
	a, b := v.Args[0], v.Args[1]
	for _, x := range []*ssa.Value{a, b} {
		if s.remTest(x) {
			k, _ := pow2(x.Args[1])
			return fmt.Sprintf("tst $u0, #%d", int64(1)<<k-1), []vreg{s.word(x.Args[0])}, intCond[v.Op]
		}
	}
	switch a.Type.Kind {
	case ssa.KInt, ssa.KBool:
		cm := intCond
		if a.Type.Kind == ssa.KBool {
			cm = uintCond
		}
		cond = cm[v.Op]
		if _, ok := constInt(a); ok {
			if _, ok2 := constInt(b); !ok2 {
				a, b = b, a
				cond = swapCond[cond]
			}
		}
		if n, ok := constInt(b); ok && imm12(n) {
			return fmt.Sprintf("cmp $u0, #%d", n), []vreg{s.word(a)}, cond
		} else if ok && imm12(-n) {
			return fmt.Sprintf("cmn $u0, #%d", -n), []vreg{s.word(a)}, cond
		}
		return "cmp $u0, $u1", []vreg{s.word(a), s.word(b)}, cond
	case ssa.KFloat:
		if v.Op == ssa.OpEq || v.Op == ssa.OpNe {
			cond = floatCond[v.Op]
			if isPosZero(b) {
				return "fcmp $u0, #0.0", []vreg{s.word(a)}, cond
			}
			return "fcmp $u0, $u1", []vreg{s.word(a), s.word(b)}, cond
		}
		return s.floatOrder(v.Op, a, b)
	}
	fail("compare of `%s`", a.Type.Name)
	return
}

func (s *fstate) set(v *ssa.Value, ws ...vreg) { s.vals[v] = ws }

func (s *fstate) instr(v *ssa.Value) {
	g := s.g
	if v.Pad != nil && !g.goMode {
		// its calls and fault stubs unwind to the cleanup of its check
		s.curPad = s.bmap[v.Pad.Block.Term().Succs[0].To]
		defer func() { s.curPad = nil }()
	}
	if g.goMode {
		switch v.Op {
		case ssa.OpClosure, ssa.OpRebind, ssa.OpCapture, ssa.OpCallValue, ssa.OpBox, ssa.OpUnbox, ssa.OpFuncAddr:
			fail("op `%s`", v.Op)
		}
	}
	switch v.Op {
	case ssa.OpConst, ssa.OpCap:
		return
	case ssa.OpAdd, ssa.OpSub:
		s.addSub(v)
	case ssa.OpMul:
		if v.Type.Kind == ssa.KFloat {
			s.set(v, s.def1(true, "fmul $d0, $u0, $u1", s.word(v.Args[0]), s.word(v.Args[1])))
			return
		}
		x, y := v.Args[0], v.Args[1]
		if _, ok := constInt(x); ok {
			x, y = y, x
		}
		a, b := s.word(v.Args[0]), s.word(v.Args[1])
		// the low word by shifts: x*2^k, x*(2^k + 1)
		low := ""
		if c, ok := constInt(y); ok {
			if k, ok := pow2(y); ok {
				low = fmt.Sprintf("lsl $d0, $u0, #%d", k)
			} else if k, ok := pow2c(c - 1); ok {
				low = fmt.Sprintf("add $d0, $u0, $u0, lsl #%d", k)
			}
		}
		if low == "" {
			if text, us, ok := s.mulLong(v.Args[0], v.Args[1]); ok {
				if v.Pos == "" || s.safeMul(v) {
					s.set(v, s.def1(false, text, us...))
					return
				}
				if strings.HasPrefix(text, "umull") {
					// the product of two values below 2^32 is below
					// 2^64: an Int when bit 63 is clear
					r := s.def1(false, text, us...)
					s.check("cmp $u0, #0", []vreg{r}, "lt", "xo_fault_overflow", rsrc(a), isrc('*'), rsrc(b), symsrc(g.loc(v.Pos)))
					s.set(v, r)
					return
				}
			}
		}
		mul := func() vreg {
			if low != "" {
				return s.def1(false, low, s.word(x))
			}
			return s.def1(false, "mul $d0, $u0, $u1", a, b)
		}
		if v.Pos == "" || s.safeMul(v) {
			s.set(v, mul())
			return
		}
		r := mul()
		hi := s.def1(false, "smulh $d0, $u0, $u1", a, b)
		s.check("cmp $u0, $u1, asr #63", []vreg{hi, r}, "ne", "xo_fault_overflow", rsrc(a), isrc('*'), rsrc(b), symsrc(g.loc(v.Pos)))
		s.set(v, r)
	case ssa.OpDiv, ssa.OpRem:
		s.divRem(v)
	case ssa.OpAnd, ssa.OpOr, ssa.OpXor:
		ins := map[ssa.Op]string{ssa.OpAnd: "and", ssa.OpOr: "orr", ssa.OpXor: "eor"}[v.Op]
		x, y := v.Args[0], v.Args[1]
		if _, ok := constInt(x); ok {
			x, y = y, x
		}
		if n, ok := constInt(y); ok && onesRun(n) {
			// a logical immediate (decision 0184)
			s.set(v, s.def1(false, fmt.Sprintf("%s $d0, $u0, #%d", ins, n), s.word(x)))
			return
		}
		s.set(v, s.def1(false, ins+" $d0, $u0, $u1", s.word(v.Args[0]), s.word(v.Args[1])))
	case ssa.OpShl, ssa.OpShr:
		s.shift(v)
	case ssa.OpNeg:
		if n, ok := negConst(v); ok {
			// a negative literal (the SSA negates the constant): a
			// constant, which cannot overflow (decision 0194)
			s.set(v, s.cimm(n))
			return
		}
		a := s.word(v.Args[0])
		if v.Type.Kind == ssa.KFloat {
			s.set(v, s.def1(true, "fneg $d0, $u0", a))
			return
		}
		if v.Pos == "" {
			s.set(v, s.def1(false, "neg $d0, $u0", a))
			return
		}
		s.set(v, s.checked(false, "negs $d0, $u0", []vreg{a}, "neg", "vs", "xo_fault_neg", rsrc(a), symsrc(g.loc(v.Pos))))
	case ssa.OpNot:
		a := s.word(v.Args[0])
		if v.Type.Kind == ssa.KBool {
			s.set(v, s.def1(false, "eor $d0, $u0, #1", a))
		} else {
			s.set(v, s.def1(false, "mvn $d0, $u0", a))
		}
	case ssa.OpEq, ssa.OpNe, ssa.OpLt, ssa.OpLe, ssa.OpGt, ssa.OpGe:
		if s.eqAggregate(v) {
			return
		}
		if v.Args[0].Type.Kind == ssa.KUnit {
			r := map[ssa.Op]int64{ssa.OpEq: 1, ssa.OpNe: 0, ssa.OpLt: 0, ssa.OpLe: 1, ssa.OpGt: 0, ssa.OpGe: 1}[v.Op]
			s.set(v, s.imm(r))
			return
		}
		if s.nuses[v] == 1 && s.usedByBranch(v) {
			s.lazy[v] = true
			return
		}
		text, uses, cond := s.compare(v)
		s.set(v, s.def1(false, text+"\ncset $d0, "+cond, uses...))
	case ssa.OpIntToFloat:
		s.set(v, s.def1(true, "scvtf $d0, $u0", s.word(v.Args[0])))
	case ssa.OpIntConv:
		// the same word as another integer type (decision 0081)
		s.set(v, s.word(v.Args[0]))
	case ssa.OpFloatToInt:
		a := s.word(v.Args[0])
		t := s.def1(true, "frintz $d0, $u0", a)
		if v.Pos != "" {
			lo := s.fimm(math.Float64bits(-9223372036854775808.0))
			s.check("fcmp $u0, $u1", []vreg{t, lo}, "lt", "xo_fault_float_int", rsrc(a), symsrc(g.loc(v.Pos)))
			hi := s.fimm(math.Float64bits(9223372036854775808.0))
			s.check("fcmp $u0, $u1", []vreg{t, hi}, "ge", "xo_fault_float_int", rsrc(a), symsrc(g.loc(v.Pos)))
		}
		s.set(v, s.def1(false, "fcvtzs $d0, $u0", t))
	case ssa.OpStruct, ssa.OpTuple:
		var ws []vreg
		for _, x := range v.Args {
			ws = append(ws, s.words(x)...)
		}
		s.set(v, ws...)
	case ssa.OpField:
		if _, ok := s.memOnlyOf(v); ok {
			return // a `var` result left in memory: read where used
		}
		u := v.Args[0]
		off, n := g.fieldWords(u.Type, v.Aux.(int))
		if m, ok := s.memOnlyOf(u); ok {
			// a field of a value held in memory
			s.set(v, s.loadWords(s.held.mems[m].ptr, off, g.layout(v.Type))...)
			return
		}
		if ptr, ok := s.stableOf(u); ok {
			if g.bigByRef(v.Type) {
				// a large field of read only memory: read in place
				// where used, passed on by address
				s.stable[v] = s.addrAt(ptr, 8*off)
				return
			}
			s.set(v, s.loadWords(ptr, off, g.layout(v.Type))...)
			return
		}
		if s.lazyRes[hkey{u, v.Aux.(int)}] {
			fail("%s in %s: a result left in memory read out of place", v.Name(), s.f.Name)
		}
		ws := s.words(u)
		s.set(v, ws[off:off+n]...)
	case ssa.OpSetField:
		off, n := g.fieldWords(v.Type, v.Aux.(int))
		if m, ok := s.held.inPlaceOf(v); ok {
			// the field is written in place
			ptr := s.held.mems[m].ptr
			_, uOnly := s.memOnlyOf(v.Args[0])
			var old []vreg
			if !uOnly {
				old = s.words(v.Args[0])
			}
			s.storeWordsAt(ptr, off, s.words(v.Args[1]))
			if _, ok := s.memOnlyOf(v); ok {
				return
			}
			if uOnly {
				s.set(v, s.loadWords(ptr, 0, g.layout(v.Type))...)
				return
			}
			ws := append([]vreg(nil), old...)
			copy(ws[off:off+n], s.words(v.Args[1]))
			s.set(v, ws...)
			return
		}
		ws := append([]vreg(nil), s.words(v.Args[0])...)
		copy(ws[off:off+n], s.words(v.Args[1]))
		s.set(v, ws...)
	case ssa.OpVariant:
		s.set(v, s.variant(v.Type, v.Aux.(int), v.Args)...)
	case ssa.OpTag:
		if ptr, ok := s.memOf(v.Args[0]); ok {
			// an enum in memory (decision 0153): only its tag is read
			s.set(v, s.loadWords(ptr, 0, []bool{false})...)
			return
		}
		s.set(v, s.words(v.Args[0])[0])
	case ssa.OpPayload:
		r := v.Aux.(ssa.PayloadRef)
		off, n := g.payloadWords(v.Args[0].Type, r.Tag, r.Field)
		if ptr, ok := s.memOf(v.Args[0]); ok {
			// a payload of an enum in memory (decision 0153): read in
			// place; a large one of read only memory stays there
			if _, ro := s.stableOf(v.Args[0]); ro && g.bigByRef(v.Type) {
				s.stable[v] = s.addrAt(ptr, 8*off)
				return
			}
			s.set(v, s.loadWords(ptr, off, g.layout(v.Type))...)
			return
		}
		ws := s.words(v.Args[0])[off : off+n]
		s.set(v, s.fromPayload(ws, g.layout(v.Type))...)
	case ssa.OpCall:
		s.callOp(v)
	case ssa.OpClosure:
		s.closureOp(v)
	case ssa.OpCapture:
		s.captureOp(v)
	case ssa.OpCallValue:
		s.callValueOp(v)
	case ssa.OpBox:
		s.boxOp(v)
	case ssa.OpFuncAddr:
		s.set(v, s.symAddr(g.fname(v.Aux.(string))))
	case ssa.OpRebind:
		// the new code word and the environment word of Args[0]
		s.set(v, s.symAddr(g.fname(v.Aux.(string))), s.words(v.Args[0])[1])
	case ssa.OpUnbox:
		s.unboxOp(v)
	case ssa.OpFault:
		if g.goMode {
			fail("fault with a message")
		}
		ws := s.words(v.Args[0])
		d := s.def1(false, "add $d0, $u0, #16", ws[0])
		s.add(&inst{kind: iCall, sym: g.sym("xo_fault"), srcs: []src{rsrc(d), rsrc(ws[1]), symsrc(g.loc(v.Pos))}, uses: []vreg{d, s.real(ws[1])}, noret: true})
		s.add(&inst{kind: iTrap})
	case ssa.OpRetain:
		x := v.Args[0]
		if g.goMode && x.Type.Heap() {
			fail("reference counting")
		}
		if x.Op == ssa.OpConst || s.cancelRC(v) {
			return // literals are immortal; see cancelRC
		}
		s.retain(x.Type, s.words(x))
	case ssa.OpRelease:
		x := v.Args[0]
		if g.goMode && x.Type.Heap() {
			fail("reference counting")
		}
		if s.cancel[v] {
			return
		}
		s.release(x.Type, s.words(x))
	case ssa.OpReuse:
		if g.goMode {
			s.goReuse(v)
			return
		}
		s.reuse(v)
	case ssa.OpShare:
		x := v.Args[0]
		if g.goMode {
			fail("share")
		}
		if x.Op == ssa.OpConst {
			return // literals are immortal
		}
		s.call(s.g.rcHelper("share", x.Type), regs(s.toInt(s.words(x), s.g.layout(x.Type))...), nil)
	case ssa.OpJump:
		s.endMemo[v.Block] = s.m()
		dsts, srcs := s.edgeMoves(v.Succs[0])
		s.pmove(dsts, srcs)
		s.add(&inst{kind: iJump, targets: []*mblock{s.bmap[v.Succs[0].To]}})
	case ssa.OpBranch:
		s.endMemo[v.Block] = s.m()
		c := v.Args[0]
		if k, ok := c.Aux.(bool); ok && c.Op == ssa.OpConst {
			e := v.Succs[1]
			if k {
				e = v.Succs[0]
			}
			dsts, srcs := s.edgeMoves(e)
			s.pmove(dsts, srcs)
			s.add(&inst{kind: iJump, targets: []*mblock{s.bmap[e.To]}})
			return
		}
		if c.Op == ssa.OpCall && c.Aux == "rt.landing" {
			// only a fault takes it, through a landing pad (decision 0085):
			// a jump to the normal successor, the pad a second target only
			// for liveness and reachability
			s.add(&inst{kind: iJump, targets: []*mblock{s.edge(v.Succs[1]), s.edge(v.Succs[0])}})
			return
		}
		var in *inst
		if s.lazy[c] {
			text, uses, cond := s.compare(c)
			in = &inst{kind: iCondBr, text: text, uses: uses, cond: cond}
			if text == "tst $u0, #1" && (cond == "eq" || cond == "ne") {
				// test bit 0 and branch
				in = &inst{kind: iCondBr, uses: uses, cond: map[string]string{"eq": "tbz", "ne": "tbnz"}[cond]}
			}
		} else {
			in = &inst{kind: iCondBr, cond: "cbnz", uses: []vreg{s.word(c)}}
		}
		in.targets = []*mblock{s.edge(v.Succs[0]), s.edge(v.Succs[1])}
		s.add(in)
	case ssa.OpReturn:
		if g.goMode {
			s.goReturn(v)
			return
		}
		if s.refPtr != nil {
			s.byRefReturn(v)
			return
		}
		var uses []vreg
		for _, x := range v.Args {
			uses = append(uses, s.words(x)...)
		}
		in := &inst{kind: iRet, srcs: regs(uses...)}
		for _, u := range uses {
			if u != 0 {
				in.uses = append(in.uses, u)
			}
		}
		s.add(in)
	case ssa.OpUnreachable:
		s.add(&inst{kind: iTrap})
	default:
		fail("op `%s`", v.Op)
	}
}

func (s *fstate) usedByBranch(v *ssa.Value) bool {
	t := v.Block.Term()
	return t != nil && t.Op == ssa.OpBranch && t.Args[0] == v
}

func (s *fstate) addSub(v *ssa.Value) {
	g := s.g
	ins := map[ssa.Op]string{ssa.OpAdd: "add", ssa.OpSub: "sub"}[v.Op]
	if v.Type.Kind == ssa.KFloat {
		s.set(v, s.def1(true, "f"+ins+" $d0, $u0, $u1", s.word(v.Args[0]), s.word(v.Args[1])))
		return
	}
	a, b := v.Args[0], v.Args[1]
	// faults name the operands in source order: c + x swapped to x + c
	// still reports c + x
	swapped := false
	if _, ok := constInt(a); ok && v.Op == ssa.OpAdd {
		if _, ok2 := constInt(b); !ok2 {
			a, b = b, a
			swapped = true
		}
	}
	if n, ok := constInt(b); ok {
		defer s.offsetFacts(v, a, n)
	}
	checked := v.Pos != ""
	if n, ok := constInt(b); ok && checked && s.safeAdd(a, n, v.Op) {
		checked = false // the bounds of a prove it
	}
	flag := ""
	if checked {
		flag = "s"
	}
	if n, ok := constInt(b); ok && (imm12(n) || imm12(-n)) {
		op := ins
		if n < 0 {
			op = map[string]string{"add": "sub", "sub": "add"}[ins]
			n = -n
		}
		text := fmt.Sprintf("%s%s $d0, $u0, #%d", op, flag, n)
		av := s.word(a)
		if !checked {
			s.set(v, s.def1(false, text, av))
			return
		}
		l, r := rsrc(av), isrc(mustConst(b))
		if swapped {
			l, r = r, l
		}
		s.set(v, s.checked(false, text, []vreg{av}, ins, "vs", "xo_fault_overflow", l, isrc(opChar[v.Op]), r, symsrc(g.loc(v.Pos))))
		return
	}
	if checked {
		alo, ahi := s.bounds(a)
		blo, bhi := s.bounds(b)
		if s.safeAddRange(v.Op, alo, ahi, blo, bhi) {
			checked = false
			flag = ""
		}
	}
	av, bv := s.word(a), s.word(b)
	text := ins + flag + " $d0, $u0, $u1"
	if !checked {
		s.set(v, s.def1(false, text, av, bv))
		return
	}
	l, r := rsrc(av), rsrc(bv)
	if swapped {
		l, r = r, l
	}
	s.set(v, s.checked(false, text, []vreg{av, bv}, ins, "vs", "xo_fault_overflow", l, isrc(opChar[v.Op]), r, symsrc(g.loc(v.Pos))))
}

func mustConst(v *ssa.Value) int64 {
	n, _ := constInt(v)
	return n
}

// pow2 reports k when v is the Int constant 2^k, k >= 1.
func pow2(v *ssa.Value) (int, bool) {
	n, ok := constInt(v)
	if !ok {
		return 0, false
	}
	return pow2c(n)
}

// pow2c reports k when n is 2^k, k >= 1.
func pow2c(n int64) (int, bool) {
	if n < 2 || n&(n-1) != 0 {
		return 0, false
	}
	k := 0
	for int64(1)<<k != n {
		k++
	}
	return k, true
}

// remTest reports whether v is `x % 2^k` used only by `== 0` or `!= 0`
// (lowered as a bit test of x).
func (s *fstate) remTest(v *ssa.Value) bool {
	if v.Op != ssa.OpRem || v.Type.Kind != ssa.KInt || s.nuses[v] != 1 {
		return false
	}
	if _, ok := pow2(v.Args[1]); !ok {
		return false
	}
	u := s.users[v]
	if u == nil || (u.Op != ssa.OpEq && u.Op != ssa.OpNe) {
		return false
	}
	o := u.Args[0]
	if o == v {
		o = u.Args[1]
	}
	n, ok := constInt(o)
	return ok && n == 0
}

func (s *fstate) divRem(v *ssa.Value) {
	g := s.g
	if k, ok := pow2(v.Args[1]); ok && v.Type.Kind == ssa.KInt {
		if s.remTest(v) {
			return // lowered by the comparison
		}
		// round toward zero: add 2^k - 1 to negative dividends
		a := s.word(v.Args[0])
		if lo, _ := s.bounds(v.Args[0]); lo >= 0 {
			// not negative: a mask or a shift
			if v.Op == ssa.OpDiv {
				s.set(v, s.def1(false, fmt.Sprintf("asr $d0, $u0, #%d", k), a))
			} else {
				s.set(v, s.def1(false, fmt.Sprintf("and $d0, $u0, #%d", int64(1)<<k-1), a))
			}
			return
		}
		if s.m().lowZero[a] >= k {
			// a branch proved a % 2^k == 0: exact
			if v.Op == ssa.OpDiv {
				s.set(v, s.def1(false, fmt.Sprintf("asr $d0, $u0, #%d", k), a))
			} else {
				s.set(v, s.cimm(0))
			}
			return
		}
		var t vreg
		if k == 1 {
			t = s.def1(false, "add $d0, $u0, $u0, lsr #63", a)
		} else {
			sg := s.def1(false, "asr $d0, $u0, #63", a)
			t = s.def1(false, fmt.Sprintf("add $d0, $u0, $u1, lsr #%d", 64-k), a, sg)
		}
		if v.Op == ssa.OpDiv {
			s.set(v, s.def1(false, fmt.Sprintf("asr $d0, $u0, #%d", k), t))
			return
		}
		m := s.def1(false, fmt.Sprintf("and $d0, $u0, #%d", -(int64(1)<<k)), t)
		s.set(v, s.def1(false, "sub $d0, $u0, $u1", a, m))
		return
	}
	if v.Type.Kind == ssa.KFloat {
		a, b := s.word(v.Args[0]), s.word(v.Args[1])
		if v.Op == ssa.OpDiv {
			s.set(v, s.def1(true, "fdiv $d0, $u0, $u1", a, b))
		} else {
			s.set(v, s.rt("fmod", regs(a, b), true)...)
		}
		return
	}
	k, isConst := constInt(v.Args[1])
	if !isConst {
		k, isConst = negConst(v.Args[1])
	}
	if isConst && v.Type.Kind == ssa.KInt && k != 0 && k != 1 && k != -1 && k != math.MinInt64 {
		s.divConst(v, k)
		return
	}
	a, b := s.word(v.Args[0]), s.word(v.Args[1])
	if v.Pos != "" && !(isConst && k != 0) {
		s.check("cmp $u0, #0", []vreg{b}, "eq", "xo_fault_divzero", rsrc(a), isrc(opChar[v.Op]), symsrc(g.loc(v.Pos)))
	}
	if v.Op == ssa.OpDiv {
		if v.Pos != "" && !(isConst && k != -1) {
			mn := s.imm(math.MinInt64)
			s.check("cmp $u0, $u1\nccmn $u2, #1, #0, eq", []vreg{a, mn, b}, "eq", "xo_fault_overflow", rsrc(a), isrc('/'), rsrc(b), symsrc(g.loc(v.Pos)))
		}
		s.set(v, s.def1(false, "sdiv $d0, $u0, $u1", a, b))
		return
	}
	// a - (a / b) * b; arm64's sdiv of the minimum by -1 is the minimum,
	// so `x % -1` is 0 as Xo requires
	q := s.def1(false, "sdiv $d0, $u0, $u1", a, b)
	s.set(v, s.def1(false, "msub $d0, $u0, $u1, $u2", q, b, a))
}

// divConst lowers v = x / d or x % d for an Int constant d other than 0,
// 1, -1, the minimum, and the positive powers of two (decision 0194):
// neither can fault. The quotient of x by |d| is a multiply high and
// shifts (sdivQuot); x / d negates it for d < 0, and x % d (whose sign
// is x's, so x % d == x % |d|) is x - q * |d|.
func (s *fstate) divConst(v *ssa.Value, d int64) {
	a := s.word(v.Args[0])
	ad := d
	if d < 0 {
		ad = -d
	}
	q := s.sdivQuot(v.Args[0], a, ad)
	if v.Op == ssa.OpDiv {
		if d < 0 {
			q = s.def1(false, "neg $d0, $u0", q)
		}
		s.set(v, q)
		return
	}
	s.set(v, s.def1(false, "msub $d0, $u0, $u1, $u2", q, s.cimm(ad), a))
}

// sdivQuot returns a register holding x / d rounded toward zero (x the
// SSA value in register a, 2 <= d < 2^63), shared with the blocks the
// block dominates (memo.quo): for a power of two 2^k the rounding add
// and a shift (as divRem), else Granlund and Montgomery's multiply high
// by sdivMagic's M, plus x when M is negative as an Int, an arithmetic
// shift, and 1 when the result is negative (left out when x's bounds
// prove it is not).
func (s *fstate) sdivQuot(x *ssa.Value, a vreg, d int64) vreg {
	m := s.m()
	key := [2]int64{int64(a), d}
	if q, ok := m.quo[key]; ok {
		return q
	}
	lo, _ := s.bounds(x)
	var q vreg
	if k, ok := pow2c(d); ok {
		switch {
		case lo >= 0 || m.lowZero[a] >= k:
			q = s.def1(false, fmt.Sprintf("asr $d0, $u0, #%d", k), a)
		case k == 1:
			t := s.def1(false, "add $d0, $u0, $u0, lsr #63", a)
			q = s.def1(false, "asr $d0, $u0, #1", t)
		default:
			sg := s.def1(false, "asr $d0, $u0, #63", a)
			t := s.def1(false, fmt.Sprintf("add $d0, $u0, $u1, lsr #%d", 64-k), a, sg)
			q = s.def1(false, fmt.Sprintf("asr $d0, $u0, #%d", k), t)
		}
	} else {
		mg, sh := sdivMagic(d)
		q = s.def1(false, "smulh $d0, $u0, $u1", a, s.cimm(mg))
		if mg < 0 {
			q = s.def1(false, "add $d0, $u0, $u1", q, a)
		}
		if sh > 0 {
			q = s.def1(false, fmt.Sprintf("asr $d0, $u0, #%d", sh), q)
		}
		if lo < 0 {
			q = s.def1(false, "add $d0, $u0, $u0, lsr #63", q)
		}
	}
	m.quo[key] = q
	return q
}

// sdivMagic returns the multiplier M (as an Int's bits) and the shift
// s of a signed division by d (3 <= d < 2^63, not a power of two), by
// Hacker's Delight 10-1 (Granlund and Montgomery): for the least p >= 64
// with e * anc < 2^p, where M = ceil(2^p / d), e = M * d - 2^p, and anc
// is the largest Int with anc mod d == d - 1, floor(x * M / 2^p) is
// floor(x / d) for every Int x, so x / d rounded toward zero is that
// plus 1 for a negative x; s = p - 64. M < 2^64. The arithmetic stays
// in Ints below 2^63 (no 128 bit products) so that selfhost/arm64
// sdiv_magic, its mirror, computes the same.
func sdivMagic(d int64) (int64, int) {
	t := (math.MaxInt64%d + 1) % d // 2^63 mod d
	anc := math.MaxInt64 - t       // 2^62 <= anc < 2^63
	// q1, r1: 2^p / anc and 2^p mod anc, from p = 63; big once q1 >=
	// 2^63, when e < 2^63 <= q1 settles the test
	q1, r1 := int64(1), math.MaxInt64-anc+1
	if anc == 1<<62 {
		q1, r1 = 2, 0
	}
	big := false
	// q2, r2: 2^p / d (mod 2^64) and 2^p mod d, from p = 0
	var q2 uint64
	r2 := int64(1)
	for p := 1; ; p++ {
		q2 <<= 1
		if r2 >= d-r2 {
			r2 -= d - r2
			q2 |= 1
		} else {
			r2 += r2
		}
		if p < 64 {
			continue
		}
		if !big {
			if q1 >= 1<<62 {
				big = true
			} else {
				q1 += q1
				if r1 >= anc-r1 {
					r1 -= anc - r1
					q1++
				} else {
					r1 += r1
				}
			}
		}
		// e * anc < 2^p = q1 * anc + r1
		if e := d - r2; big || e < q1 || (e == q1 && r1 > 0) {
			return int64(q2 + 1), p - 64
		}
	}
}

func (s *fstate) shift(v *ssa.Value) {
	g := s.g
	a := s.word(v.Args[0])
	if k, ok := constInt(v.Args[1]); ok && k >= 0 && k < 64 {
		if v.Op == ssa.OpShr {
			s.set(v, s.def1(false, fmt.Sprintf("asr $d0, $u0, #%d", k), a))
			return
		}
		r := s.def1(false, fmt.Sprintf("lsl $d0, $u0, #%d", k), a)
		if v.Pos != "" && !s.safeShl(v.Args[0], k) {
			back := s.def1(false, fmt.Sprintf("asr $d0, $u0, #%d", k), r)
			s.check("cmp $u0, $u1", []vreg{back, a}, "ne", "xo_fault_shl", rsrc(a), isrc(k), symsrc(g.loc(v.Pos)))
		}
		s.set(v, r)
		return
	}
	b := s.word(v.Args[1])
	klo, khi := s.bounds(v.Args[1])
	if v.Pos != "" && (klo < 0 || khi > 63) {
		s.check("cmp $u0, #64", []vreg{b}, "hs", "xo_fault_shift", rsrc(b), symsrc(g.loc(v.Pos)))
	}
	if v.Op == ssa.OpShr {
		s.set(v, s.def1(false, "asr $d0, $u0, $u1", a, b))
		return
	}
	r := s.def1(false, "lsl $d0, $u0, $u1", a, b)
	if v.Pos != "" && !(klo >= 0 && khi <= 63 && s.safeShl(v.Args[0], khi)) {
		back := s.def1(false, "asr $d0, $u0, $u1", r, b)
		s.check("cmp $u0, $u1", []vreg{back, a}, "ne", "xo_fault_shl", rsrc(a), rsrc(b), symsrc(g.loc(v.Pos)))
	}
	s.set(v, r)
}

// bytesLoad loads byte i (SSA value iv, register i) of the Bytes
// object in register obj. The data address (16 bytes in) is computed
// once per block, and so is data + j for i = j + c (0 < c < 4096), so
// t[k], t[k + 1], t[k + 2] are one add and three loads (decision 0184).
func (s *fstate) bytesLoad(obj vreg, iv *ssa.Value, i vreg) vreg {
	m := s.m()
	obj = s.real(obj)
	key := [3]int64{int64(obj), 0, -16}
	d, ok := m.bases[key]
	if !ok {
		d = s.def1(false, "add $d0, $u0, #16", obj)
		m.bases[key] = d
	}
	if iv.Op == ssa.OpAdd {
		if c, isK := constInt(iv.Args[1]); isK && c > 0 && c < 4096 {
			if ws, ok := s.vals[iv.Args[0]]; ok && len(ws) == 1 && ws[0] != 0 {
				jkey := [3]int64{int64(d), int64(ws[0]), -17}
				t, ok := m.bases[jkey]
				if !ok {
					t = s.def1(false, "add $d0, $u0, $u1", d, ws[0])
					m.bases[jkey] = t
				}
				return s.def1(false, fmt.Sprintf("ldrb $wd0, [$u0, #%d]", c), t)
			}
		}
	}
	return s.def1(false, "ldrb $wd0, [$u0, $u1]", d, i)
}

// onesRun reports whether n > 0 is a value every logical instruction
// takes as an immediate: one run of ones (0b0..01..10..0, decision
// 0184), or the repetition of an element of 2 to 32 bits that is one
// run of ones, rotated (0x00FF00FF00FF00FF, 0x3030303030303030; decision
// 0193 step 3).
func onesRun(n int64) bool {
	if n <= 0 {
		return false
	}
	// the smallest element whose repetition is n
	e := uint(64)
	for e > 2 {
		h := e / 2
		m := int64(1)<<h - 1
		if n&m != (n>>h)&m {
			break
		}
		e = h
	}
	if e == 64 {
		return run1(n)
	}
	m := int64(1)<<e - 1
	x := n & m
	return run1(x) || run1(^x&m)
}

// run1 reports whether x > 0 is one run of ones.
func run1(x int64) bool {
	if x <= 0 {
		return false
	}
	for x&1 == 0 {
		x >>= 1
	}
	return x == math.MaxInt64 || x&(x+1) == 0
}

// safeShl reports whether a << k cannot overflow for every k in [0,
// khi] (khi <= 63): a's bounds fit in 63 - khi bits and a sign
// (decision 0184).
func (s *fstate) safeShl(a *ssa.Value, khi int64) bool {
	if khi == 0 {
		return true
	}
	lo, hi := s.bounds(a)
	lim := int64(1) << (63 - khi)
	return lo >= -lim && hi <= lim-1
}

// variant builds the words of variant tag of enum t.
func (s *fstate) variant(t *ssa.Type, tag int, args []*ssa.Value) []vreg {
	ws := make([]vreg, s.g.nwords(t))
	ws[0] = s.imm(int64(tag))
	k := 1
	for _, a := range args {
		aw := s.words(a)
		for i, fp := range s.g.layout(a.Type) {
			if fp {
				ws[k] = s.def1(false, "fmov $d0, $u0", aw[i])
			} else {
				ws[k] = aw[i]
			}
			k++
		}
	}
	return ws
}

// fromPayload converts payload words (integer registers) to a value of
// the given layout.
func (s *fstate) fromPayload(ws []vreg, layout []bool) []vreg {
	out := make([]vreg, len(ws))
	for i, fp := range layout {
		if fp {
			out[i] = s.def1(true, "fmov $d0, $u0", s.real(ws[i]))
		} else {
			out[i] = ws[i]
		}
	}
	return out
}

// toInt converts value words to integer registers (floats as bits), the
// convention of the reference counting helpers.
func (s *fstate) toInt(ws []vreg, layout []bool) []vreg {
	out := make([]vreg, len(ws))
	for i, fp := range layout {
		if fp {
			out[i] = s.def1(false, "fmov $d0, $u0", ws[i])
		} else {
			out[i] = ws[i]
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Reference counting

// inc retains the counted cell p inline: a thread local count (bit 63
// clear) goes up by one; a shared count (bits 63 and 62 set) is updated
// atomically in a cold block; the immortal count is left alone
// (decision 0058).
func (s *fstate) inc(p vreg) {
	p = s.real(p)
	c := s.def1(false, "ldr $d0, [$u0]", p)
	before := s.m()
	shared := s.newBlock(true)
	fast := s.newBlock(false)
	cont := s.newBlock(false)
	s.add(&inst{kind: iCondBr, cond: "tbnz", bit: 63, uses: []vreg{c}, targets: []*mblock{shared, fast}})
	s.cur = fast
	c1 := s.def1(false, "add $d0, $u0, #1", c)
	s.store("str $u0, [$u1]", c1, p)
	s.add(&inst{kind: iJump, targets: []*mblock{cont}})
	s.cur = shared
	atomic := s.newBlock(true)
	s.add(&inst{kind: iCondBr, cond: "tbz", bit: 62, uses: []vreg{c}, targets: []*mblock{cont, atomic}})
	s.cur = atomic
	// x16 may hold a reloaded p: x17 and w15 are free scratch
	s.add(&inst{kind: iPlain, text: s.g.atomicInc("$u0", "x17", "w15"), uses: []vreg{p}})
	s.add(&inst{kind: iJump, targets: []*mblock{cont}})
	s.cur = cont
	s.inherit(before)
	s.clobber()
}

func (s *fstate) retain(t *ssa.Type, ws []vreg) {
	switch t.Kind {
	case ssa.KStr, ssa.KList, ssa.KBox, ssa.KHandle:
		s.inc(s.real(ws[0]))
	case ssa.KFunc:
		s.retainFunc(ws)
	default:
		if !t.Heap() {
			return
		}
		s.call(s.g.rcHelper("dup", t), regs(s.toInt(ws, s.g.layout(t))...), nil)
	}
}

func (s *fstate) release(t *ssa.Type, ws []vreg) {
	if !t.Heap() {
		return
	}
	switch t.Kind {
	case ssa.KStr, ssa.KList, ssa.KHandle:
		s.call(s.g.rcHelper("drop", t), regs(ws[0]), nil)
	case ssa.KBox:
		s.releaseBox(t, s.real(ws[0]))
	case ssa.KFunc:
		s.releaseFunc(ws)
	default:
		s.call(s.g.rcHelper("drop", t), regs(s.toInt(ws, s.g.layout(t))...), nil)
	}
}

// reuse makes a list unique: the same list when its count is 1,
// otherwise a copy (and the original loses a reference).
func (s *fstate) reuse(v *ssa.Value) {
	x := v.Args[0]
	if x.Type.Kind != ssa.KList {
		fail("reuse of `%s`", x.Type.Name)
	}
	et := x.Type.Elems[0]
	l := s.word(x)
	r := s.newVreg(false)
	c := s.def1(false, "ldr $d0, [$u0]", l)
	before := s.m()
	slow := s.newBlock(true)
	fast := s.newBlock(false)
	cont := s.newBlock(false)
	s.add(&inst{kind: iCondBr, text: "cmp $u0, #1", uses: []vreg{c}, cond: "ne", targets: []*mblock{slow, fast}})
	s.cur = fast
	s.pmove([]vreg{r}, regs(l))
	s.add(&inst{kind: iJump, targets: []*mblock{cont}})
	s.cur = slow
	// a copy, or the same list when it is shared but only held here
	cp := s.rt("xo_rl_copy", []src{rsrc(l), isrc(s.g.esize(et)), s.elemFn("dup", et), s.elemFn("drop", et)}, false)
	s.pmove([]vreg{r}, regs(cp[0]))
	s.add(&inst{kind: iJump, targets: []*mblock{cont}})
	s.cur = cont
	s.inherit(before)
	s.clobber()
	s.set(v, r)
}

// elemFn is the runtime callback that retains or releases a list element
// through a pointer (zero when elements are not on the heap).
func (s *fstate) elemFn(kind string, t *ssa.Type) src {
	if !t.Heap() {
		return isrc(0)
	}
	return symsrc(s.g.elemHelper(kind, t))
}

// ---------------------------------------------------------------------------
// Lists

const listHdr = 24 // {rc, len, cap}

// elemBase returns a register b such that element i of list l starts at
// b + listHdr.
func (s *fstate) elemBase(l, i vreg, es int64) vreg {
	m := s.m()
	key := [3]int64{int64(l), int64(i), es}
	if b, ok := m.bases[key]; ok {
		return b
	}
	b, ok := s.offsetBase(l, i, es)
	if !ok {
		b = s.elemBase1(l, i, es)
	}
	m.bases[key] = b
	return b
}

func (s *fstate) elemBase1(l, i vreg, es int64) vreg {
	switch {
	case es == 0:
		return l
	case es == 1:
		return s.def1(false, "add $d0, $u0, $u1", l, i)
	case es&(es-1) == 0:
		k := 0
		for int64(1)<<k != es {
			k++
		}
		return s.def1(false, fmt.Sprintf("add $d0, $u0, $u1, lsl #%d", k), l, i)
	}
	for sh, k := range map[int]int64{1: 3, 2: 5, 3: 9} {
		if es == 8*k {
			// i*(2^sh + 1), then *8 in the address add
			t := s.def1(false, fmt.Sprintf("add $d0, $u0, $u0, lsl #%d", sh), i)
			return s.def1(false, "add $d0, $u0, $u1, lsl #3", l, t)
		}
	}
	n := s.cimm(es)
	return s.def1(false, "madd $d0, $u0, $u1, $u2", i, n, l)
}

// elemAt is the address of a list element: [base, #off] (idx zero,
// off counting the list header), or [base, idx, lsl #sh] for an element
// of one word (base the list's data address, its first element).
type elemAt struct {
	base, idx vreg
	off       int
	sh        int
}

// at returns the element address b + listHdr.
func (s *fstate) at(b vreg) elemAt { return elemAt{base: b, off: s.hdr()} }

// regs returns the registers of the address.
func (a elemAt) regs() []vreg {
	if a.idx != 0 {
		return []vreg{a.base, a.idx}
	}
	return []vreg{a.base}
}

// operand returns the memory operand of word k, its registers $u<u> on.
func (a elemAt) operand(k, u int) string {
	if a.idx != 0 {
		if a.sh == 0 {
			return fmt.Sprintf("[$u%d, $u%d]", u, u+1)
		}
		return fmt.Sprintf("[$u%d, $u%d, lsl #%d]", u, u+1, a.sh)
	}
	return fmt.Sprintf("[$u%d, #%d]", u, a.off+8*k)
}

// mem returns what loaded records for word k.
func (a elemAt) mem(k, epoch int) memAt {
	if a.idx != 0 {
		return memAt{a.base, a.idx, 0, epoch}
	}
	return memAt{a.base, 0, a.off + 8*k, epoch}
}

// elemAddr returns the address of element i (SSA value iv) of list l
// (decision 0208): [l, #24 + k * size] for a constant index k, [b, #24 +
// c * size] for i = a + c (b the address of element a), [data, i, lsl
// #3] for a one word element (data the list's data address, once per
// block, or once per loop when the loop cannot move the buffer: lsr.go),
// else [b, #24] with b = l + i * size.
func (s *fstate) elemAddr(l, i vreg, iv *ssa.Value, et *ssa.Type) elemAt {
	g := s.g
	es := g.esize(et)
	if es == 0 || g.goMode {
		return s.at(s.elemBase(l, i, es))
	}
	if b, ok := s.m().bases[[3]int64{int64(l), int64(i), es}]; ok {
		return s.at(b)
	}
	nw := int64(len(g.layout(et)))
	fits := func(off int64) bool {
		if et.Kind == ssa.KBool {
			return off >= 0 && off <= 4095
		}
		return off >= 0 && off%8 == 0 && off+8*(nw-1) <= 32760
	}
	if iv != nil {
		if k, ok := constInt(iv); ok && k >= 0 && k < 4096 {
			if off := listHdr + k*es; fits(off) {
				return elemAt{base: l, off: int(off)}
			}
		}
	}
	if o, ok := s.offs[i]; ok && o.c > -4096 && o.c < 4096 && es < 4096 {
		if off := listHdr + o.c*es; fits(off) {
			return elemAt{base: s.elemBase(l, o.a, es), off: int(off)}
		}
	}
	if iv != nil && s.offBases[iv] {
		return s.at(s.elemBase(l, i, es))
	}
	if et.Kind == ssa.KBool {
		return elemAt{base: s.dataAddr(l), idx: i}
	}
	if es == 8 {
		return elemAt{base: s.dataAddr(l), idx: i, sh: 3}
	}
	return s.at(s.elemBase(l, i, es))
}

// dataAddr returns the address of the first element of list l, once per
// block.
func (s *fstate) dataAddr(l vreg) vreg {
	m := s.m()
	key := [3]int64{int64(l), 0, -24}
	if d, ok := m.bases[key]; ok {
		return d
	}
	d := s.def1(false, fmt.Sprintf("add $d0, $u0, #%d", listHdr), l)
	m.bases[key] = d
	return d
}

// loadElem loads the element words at a (as integer registers when
// asInt).
func (s *fstate) loadElem(a elemAt, et *ssa.Type, asInt bool) []vreg {
	m := s.m()
	if et.Kind == ssa.KBool {
		d := s.def1(false, "ldrb $wd0, "+a.operand(0, 0), a.regs()...)
		m.loaded[d] = a.mem(0, m.epoch)
		return []vreg{d}
	}
	var out []vreg
	for k, fp := range s.g.layout(et) {
		d := s.def1(fp && !asInt, "ldr $d0, "+a.operand(k, 0), a.regs()...)
		m.loaded[d] = a.mem(k, m.epoch)
		out = append(out, d)
	}
	return out
}

// storeWords returns the words of a value to store: a zero constant is
// xzr.
func (s *fstate) storeWords(v *ssa.Value) []vreg {
	if v.Op == ssa.OpConst {
		switch x := v.Aux.(type) {
		case int64:
			if x == 0 {
				return []vreg{0}
			}
		case bool:
			if !x {
				return []vreg{0}
			}
		}
	}
	return s.words(v)
}

func (s *fstate) storeElem(a elemAt, et *ssa.Type, ws []vreg) {
	m := s.m()
	same := func(w vreg, k int) bool {
		at, ok := m.loaded[w]
		return ok && at == a.mem(k, m.epoch)
	}
	if et.Kind == ssa.KBool {
		if !same(ws[0], 0) {
			s.store("strb $wu0, "+a.operand(0, 1), append([]vreg{ws[0]}, a.regs()...)...)
		}
		return
	}
	var skip []bool
	for k := range s.g.layout(et) {
		skip = append(skip, same(ws[k], k))
	}
	for k := range s.g.layout(et) {
		if !skip[k] {
			s.store("str $u0, "+a.operand(k, 1), append([]vreg{ws[k]}, a.regs()...)...)
		}
	}
}

// storeHole writes the placeholder list.take leaves in the element at
// a (decision 0169): the words reference counting reads, so that
// retain, release, and share of it do nothing. A Str, List, Box, or
// handle points to the empty string's object (immortal), a function
// value's environment is null, and an enum's tag is past its variants.
func (s *fstate) storeHole(a elemAt, et *ssa.Type) {
	g := s.g
	var ls []leaf
	g.heapLeaves(et, 0, &ls)
	var obj vreg
	for _, l := range ls {
		var w vreg
		off := l.off
		switch l.t.Kind {
		case ssa.KFunc:
			off++ // the environment
		case ssa.KEnum:
			w = s.cimm(int64(len(l.t.Variants)))
		default:
			if obj == 0 {
				obj = s.symAddr(g.strObj(""))
			}
			w = obj
		}
		s.store("str $u0, "+a.operand(off, 1), append([]vreg{w}, a.regs()...)...)
	}
}

func (s *fstate) listLen(l vreg) vreg {
	m := s.m()
	if n, ok := m.lens[l]; ok {
		return n
	}
	if s.g.goMode {
		// a Go slice's length is a parameter word, not in memory
		if n, ok := s.goLens[l]; ok {
			return n
		}
		fail("the length of a list not passed in")
	}
	n := s.def1(false, "ldr $d0, [$u0, #8]", l)
	m.lens[l] = n
	if a, ok := m.lenAt[l]; ok {
		s.sameFacts(a, n)
	}
	return n
}

// sameFacts gives register n the facts of register a, which holds the
// same value (decision 0208).
func (s *fstate) sameFacts(a, n vreg) {
	m := s.m()
	if x, ok := m.lo[a]; ok {
		m.lo[n] = x
	}
	if x, ok := m.hi[a]; ok {
		m.hi[n] = x
	}
	if x, ok := m.lowZero[a]; ok {
		m.lowZero[n] = x
	}
	var ks [][2]vreg
	for k := range m.less {
		if k[1] == a {
			ks = append(ks, k)
		}
	}
	for _, k := range ks {
		m.less[[2]vreg{k[0], n}] = m.less[k]
	}
}

func (s *fstate) boundsCheck(v *ssa.Value, l, i vreg) {
	if v.Pos == "" {
		return
	}
	m := s.m()
	if m.checked[[2]vreg{l, i}] {
		return
	}
	m.checked[[2]vreg{l, i}] = true
	n := s.listLen(l)
	if !s.provedIndex(v.Args[1], n, -1) {
		s.check("cmp $u0, $u1", []vreg{i, n}, "hs", "xo_fault_index", rsrc(i), rsrc(n), symsrc(s.g.loc(v.Pos)))
	}
	s.indexFacts(i)
	s.lessFact(v.Args[1], n)
}

// option builds Option words on two paths: in the current block when
// cond holds (some fills the payload), else none.
//
// extra registers are set on both paths too: from the sources some
// returns, and from none.
func (s *fstate) option(ot *ssa.Type, text string, uses []vreg, cond string, extra []vreg, some func() ([]vreg, []src), none []src) []vreg {
	g := s.g
	si, ni := ot.VariantIndex("Some"), ot.VariantIndex("None")
	ws := s.newVregs(make([]bool, g.nwords(ot)))
	before := s.m()
	yes, no, join := s.newBlock(false), s.newBlock(false), s.newBlock(false)
	s.add(&inst{kind: iCondBr, text: text, uses: uses, cond: cond, targets: []*mblock{yes, no}})
	s.cur = yes
	pay, yesExtra := some()
	srcs := []src{isrc(int64(si))}
	for k := 1; k < len(ws); k++ {
		if k-1 < len(pay) {
			srcs = append(srcs, rsrc(pay[k-1]))
		} else {
			srcs = append(srcs, rsrc(0))
		}
	}
	s.pmoveSrcs(append(append([]vreg(nil), ws...), extra...), append(srcs, yesExtra...))
	s.add(&inst{kind: iJump, targets: []*mblock{join}})
	s.cur = no
	srcs = []src{isrc(int64(ni))}
	for k := 1; k < len(ws); k++ {
		srcs = append(srcs, rsrc(0))
	}
	s.pmoveSrcs(append(append([]vreg(nil), ws...), extra...), append(srcs, none...))
	s.add(&inst{kind: iJump, targets: []*mblock{join}})
	s.cur = join
	s.inherit(before)
	s.clobber()
	return ws
}

// pmoveSrcs is a parallel move whose sources may be immediates.
func (s *fstate) pmoveSrcs(dsts []vreg, srcs []src) {
	in := &inst{kind: iPmove, defs: dsts, srcs: srcs}
	for _, a := range srcs {
		if a.kind == sReg && a.v != 0 {
			in.uses = append(in.uses, a.v)
		}
	}
	s.add(in)
}

// ---------------------------------------------------------------------------
// Calls

func (s *fstate) callOp(v *ssa.Value) {
	g := s.g
	name := v.Aux.(string)
	if name == "rt.landing" {
		// a check only a fault takes: no code (the branch on it, decision
		// 0085)
		s.set(v, 0)
		return
	}
	if callee := g.byName[name]; callee != nil && g.goMode {
		s.goCall(v, callee)
		return
	}
	if callee := g.byName[name]; callee != nil && g.byRefOf(callee) != nil {
		s.byRefCall(v, callee)
		return
	}
	if callee := g.byName[name]; callee != nil {
		var args []src
		for _, x := range v.Args {
			args = append(args, regs(s.words(x)...)...)
		}
		var res []bool
		for _, r := range callee.Results {
			res = append(res, g.layout(r)...)
		}
		s.set(v, s.call(g.fname(name), args, res)...)
		return
	}
	a := func(i int) []vreg { return s.words(v.Args[i]) }
	if strings.HasPrefix(name, "rt.") {
		s.rtCall(v)
		return
	}
	switch name {
	case "list.new":
		if len(v.Args) == 1 {
			// with room for a number of elements
			s.set(v, s.rt("xo_rl_new_n", []src{rsrc(s.word(v.Args[0])), isrc(g.esize(v.Type.Elems[0]))}, false)...)
			break
		}
		// the shared empty list, immortal: the first push allocates
		// (xo_rl_grow); mirrors selfhost/arm64 calls.xo
		s.set(v, s.rt("xo_rl_empty", nil, false)...)
	case "list.len":
		s.set(v, s.listLen(s.word(v.Args[0])))
	case "list.index":
		l, i := s.word(v.Args[0]), s.word(v.Args[1])
		s.boundsCheck(v, l, i)
		et := v.Type
		base := s.elemAddr(l, i, v.Args[1], et)
		s.set(v, s.loadElem(base, et, false)...)
	case "list.take":
		// element i moves out (decision 0169); its slot gets a
		// placeholder that releasing ignores until list.set stores back
		l, i := s.word(v.Args[0]), s.word(v.Args[1])
		s.boundsCheck(v, l, i)
		et := v.Type.Elems[1]
		base := s.elemAddr(l, i, v.Args[1], et)
		e := s.loadElem(base, et, false)
		s.storeHole(base, et)
		s.set(v, append([]vreg{l}, e...)...)
	case "list.get":
		l, i := s.word(v.Args[0]), s.word(v.Args[1])
		et := v.Args[0].Type.Elems[0]
		n := s.listLen(l)
		s.set(v, s.option(v.Type, "cmp $u0, $u1", []vreg{i, n}, "lo", nil, func() ([]vreg, []src) {
			return s.loadElem(s.elemAddr(l, i, v.Args[1], et), et, true), nil
		}, nil)...)
	case "list.set":
		l, i := s.word(v.Args[0]), s.word(v.Args[1])
		et := v.Args[2].Type
		x := s.storeWords(v.Args[2])
		s.boundsCheck(v, l, i)
		base := s.elemAddr(l, i, v.Args[1], et)
		if et.Heap() {
			old := s.loadElem(base, et, false)
			s.release(et, old)
		}
		s.storeElem(base, et, x)
		s.set(v, l)
	case "list.push":
		l := s.word(v.Args[0])
		et := v.Args[1].Type
		x := s.storeWords(v.Args[1])
		n := s.listLen(l)
		c := s.def1(false, "ldr $d0, [$u0, #16]", l)
		lm := s.newVreg(false)
		before := s.m()
		grow, fast, room := s.newBlock(true), s.newBlock(false), s.newBlock(false)
		s.add(&inst{kind: iCondBr, text: "cmp $u0, $u1", uses: []vreg{n, c}, cond: "eq", targets: []*mblock{grow, fast}})
		s.cur = fast
		s.pmove([]vreg{lm}, regs(l))
		s.add(&inst{kind: iJump, targets: []*mblock{room}})
		s.cur = grow
		l2 := s.rt("xo_rl_grow", []src{rsrc(l), isrc(g.esize(et))}, false)
		s.pmove([]vreg{lm}, regs(l2[0]))
		s.add(&inst{kind: iJump, targets: []*mblock{room}})
		s.cur = room
		s.inherit(before)
		s.clobber()
		if g.esize(et) > 0 {
			s.storeElem(s.at(s.elemBase(lm, n, g.esize(et))), et, x)
		}
		n1 := s.def1(false, "add $d0, $u0, #1", n)
		s.store("str $u0, [$u1, #8]", n1, lm)
		s.m().lens[lm] = n1
		s.set(v, lm)
	case "list.pop":
		l := s.word(v.Args[0])
		et := v.Args[0].Type.Elems[0]
		ot := v.Type.Elems[1]
		n := s.listLen(l)
		// a new register for the shorter list (what is known about l,
		// its length, must not reach blocks it dominates), and its length
		l2, nl := s.newVreg(false), s.newVreg(false)
		o := s.option(ot, "cmp $u0, #0", []vreg{n}, "gt", []vreg{l2, nl}, func() ([]vreg, []src) {
			n1 := s.def1(false, "sub $d0, $u0, #1", n)
			s.store("str $u0, [$u1, #8]", n1, l)
			return s.loadElem(s.at(s.elemBase(l, n1, g.esize(et))), et, true), regs(l, n1)
		}, regs(l, n))
		s.m().lens[l2] = nl
		s.set(v, append([]vreg{l2}, o...)...)
	case "str.concat":
		x, y := a(0), a(1)
		n := s.def1(false, "add $d0, $u0, $u1", x[1], y[1])
		obj := s.rt("xo_rs_concat", regs(x[0], x[1], y[0], y[1]), false)
		s.set(v, obj[0], n)
	case "str.concat_int", "str.concat_float":
		x, y := a(0), a(1)
		fn := "xo_rs_concat_int"
		if name == "str.concat_float" {
			fn = "xo_rs_concat_float"
		}
		obj := s.rt(fn, []src{rsrc(x[0]), rsrc(x[1]), rsrc(y[0]), framesrc()}, false)
		n := s.def1(false, "ldr $d0, [sp, #$OUT]")
		s.set(v, obj[0], n)
	case "str.of_int", "str.of_float":
		fn := "xo_rs_of_int"
		if name == "str.of_float" {
			fn = "xo_rs_of_float"
		}
		obj := s.rt(fn, []src{rsrc(a(0)[0]), framesrc()}, false)
		n := s.def1(false, "ldr $d0, [sp, #$OUT]")
		s.set(v, obj[0], n)
	case "str.of_bool":
		b := s.word(v.Args[0])
		t, f := s.symAddr(g.strObj("true")), s.symAddr(g.strObj("false"))
		obj := s.def1(false, "cmp $u0, #0\ncsel $d0, $u1, $u2, ne", b, t, f)
		four, five := s.imm(4), s.imm(5)
		n := s.def1(false, "cmp $u0, #0\ncsel $d0, $u1, $u2, ne", b, four, five)
		s.set(v, obj, n)
	case "str.eq", "str.cmp":
		x, y := a(0), a(1)
		xd := s.def1(false, "add $d0, $u0, #16", s.real(x[0]))
		yd := s.def1(false, "add $d0, $u0, #16", s.real(y[0]))
		if name == "str.cmp" {
			s.set(v, s.rt("xo_str_cmp", regs(xd, x[1], yd, y[1]), false)...)
			return
		}
		r := s.rt("xo_str_eq", regs(xd, x[1], yd, y[1]), false)
		s.set(v, s.def1(false, "cmp $wu0, #0\ncset $d0, ne", r[0]))
	case "Stdio.print", "Stdio.println", "Stdio.eprint", "Stdio.eprintln":
		x := a(1)
		d := s.def1(false, "add $d0, $u0, #16", s.real(x[0]))
		s.rt("xo_"+strings.TrimPrefix(name, "Stdio."), regs(d, x[1]))
		s.set(v)
	case "Int.min", "Int.max":
		c := map[string]string{"Int.min": "lt", "Int.max": "gt"}[name]
		s.set(v, s.def1(false, "cmp $u0, $u1\ncsel $d0, $u0, $u1, "+c, s.word(v.Args[0]), s.word(v.Args[1])))
	case "Float.min", "Float.max":
		// the total order, the receiver on a tie (std.md 1,
		// decisions/0053): min is `b < a ? b : a`, max `a < b ? b : a`
		x, y := v.Args[0], v.Args[1]
		if name == "Float.min" {
			x, y = y, x
		}
		text, uses, cond := s.floatOrder(ssa.OpLt, x, y)
		ra, rb := s.word(v.Args[0]), s.word(v.Args[1])
		n := len(uses)
		uses = append(uses, ra, rb)
		s.set(v, s.def1(true, fmt.Sprintf("%s\nfcsel $d0, $u%d, $u%d, %s", text, n+1, n, cond), uses...))
	case "Int.abs":
		x := s.word(v.Args[0])
		if v.Pos != "" {
			mn := s.imm(math.MinInt64)
			s.check("cmp $u0, $u1", []vreg{x, mn}, "eq", "xo_fault_abs", rsrc(x), symsrc(g.loc(v.Pos)))
		}
		s.set(v, s.def1(false, "cmp $u0, #0\ncneg $d0, $u0, lt", x))
	case "Float.sqrt", "Float.floor", "Float.ceil", "Float.trunc", "Float.round", "Float.abs":
		ins := map[string]string{"Float.sqrt": "fsqrt", "Float.floor": "frintm", "Float.ceil": "frintp",
			"Float.trunc": "frintz", "Float.round": "frinta", "Float.abs": "fabs"}[name]
		s.set(v, s.def1(true, ins+" $d0, $u0", s.word(v.Args[0])))
	case "Float.is_nan":
		s.set(v, s.def1(false, "fcmp $u0, $u0\ncset $d0, vs", s.word(v.Args[0])))
	case "bytes.index":
		// b[i] on Bytes (decision 0081): the data is 16 bytes into the
		// string object
		x, i := a(0), s.word(v.Args[1])
		k := int64(-1)
		if c, ok := v.Args[0].Aux.(string); ok && v.Args[0].Op == ssa.OpConst {
			k = int64(len(c))
		}
		if v.Pos != "" && !s.provedIndex(v.Args[1], x[1], k) {
			s.check("cmp $u0, $u1", []vreg{i, x[1]}, "hs", "xo_fault_bytes_index", rsrc(i), rsrc(x[1]), symsrc(g.loc(v.Pos)))
		}
		if v.Pos != "" {
			// what the check (or the proof) says holds after it
			s.indexFacts(i)
			s.lessFact(v.Args[1], x[1])
		}
		s.set(v, s.bytesLoad(x[0], v.Args[1], i))
	case "Str.len":
		s.set(v, a(0)[1])
	case "Str.is_empty":
		s.set(v, s.def1(false, "cmp $u0, #0\ncset $d0, eq", s.real(a(0)[1])))
	default:
		fail("runtime call `%s`", name)
	}
}

// dominators returns the immediate dominator of each block (blocks in
// reverse postorder; Cooper, Harvey, Kennedy 2001).
func dominators(blocks []*ssa.Block) map[*ssa.Block]*ssa.Block {
	num := map[*ssa.Block]int{}
	for i, b := range blocks {
		num[b] = i
	}
	idom := map[*ssa.Block]*ssa.Block{blocks[0]: blocks[0]}
	intersect := func(a, b *ssa.Block) *ssa.Block {
		for a != b {
			for num[a] > num[b] {
				a = idom[a]
			}
			for num[b] > num[a] {
				b = idom[b]
			}
		}
		return a
	}
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
				} else {
					nd = intersect(p, nd)
				}
			}
			if nd != nil && idom[b] != nd {
				idom[b] = nd
				changed = true
			}
		}
	}
	delete(idom, blocks[0])
	return idom
}

// inlineRT lowers the runtime intrinsics that are an instruction or two
// (decision 0193 step 3: the word arithmetic of the number text in
// std/native/llrt/num.xo): rt.int_wrap with a constant op (0 +, 1 -, 2
// *, 3 << and 4 >> logical by the amount mod 64; decision 0195: 5 rotate
// right mod 64, 6 the low 32 bits rotated right mod 32 and zero
// extended, 7 the bytes reversed), rt.u64_mul_hi, the bits of a Float
// both ways, and rt.bytes_word (an unchecked load of 8 bytes);
// the scan intrinsics of decision 0201: rt.popcount (cnt and addv),
// rt.ctz (rbit and clz), rt.clz, and rt.bytes_eq16 and rt.bytes_high16
// (16 bytes in v30 compared, narrowed by shrn to 4 bits a byte; v30 and
// v31 are scratch registers of the emitter), and the string writes of
// Bytes.to_hex: rt.rs_put_word (an unchecked store of 8 bytes into a
// new string with room) and rt.rs_set_len (the same object with a new
// length). It reports whether v was one.
func (s *fstate) inlineRT(v *ssa.Value) bool {
	switch v.Aux.(string) {
	case "rt.int_wrap":
		op, ok := constInt(v.Args[0])
		if !ok || op < 0 || op > 7 {
			return false
		}
		a := s.word(v.Args[1])
		if op == 7 {
			s.set(v, s.def1(false, "rev $d0, $u0", a))
			return true
		}
		if op == 5 || op == 6 {
			if k, ok := constInt(v.Args[2]); ok {
				if op == 5 {
					s.set(v, s.def1(false, fmt.Sprintf("ror $d0, $u0, #%d", k&63), a))
				} else {
					s.set(v, s.def1(false, fmt.Sprintf("ror $wd0, $wu0, #%d", k&31), a))
				}
				return true
			}
			if op == 5 {
				s.set(v, s.def1(false, "ror $d0, $u0, $u1", a, s.word(v.Args[2])))
			} else {
				s.set(v, s.def1(false, "ror $wd0, $wu0, $wu1", a, s.word(v.Args[2])))
			}
			return true
		}
		if op == 2 {
			if text, us, ok := s.mulLong(v.Args[1], v.Args[2]); ok {
				s.set(v, s.def1(false, text, us...))
				return true
			}
		}
		if op == 0 && s.fusedAdd(v) {
			return true
		}
		ins := [...]string{"add", "sub", "mul", "lsl", "lsr"}[op]
		if k, ok := constInt(v.Args[2]); ok && op <= 1 && imm12(k) {
			// a constant addend is the immediate (decision 0207 stage 3)
			s.set(v, s.def1(false, fmt.Sprintf("%s $d0, $u0, #%d", ins, k), a))
			return true
		}
		if k, ok := constInt(v.Args[2]); ok && op >= 3 {
			s.set(v, s.def1(false, fmt.Sprintf("%s $d0, $u0, #%d", ins, k&63), a))
			return true
		}
		s.set(v, s.def1(false, ins+" $d0, $u0, $u1", a, s.word(v.Args[2])))
	case "rt.u64_mul_hi":
		s.set(v, s.def1(false, "umulh $d0, $u0, $u1", s.word(v.Args[0]), s.word(v.Args[1])))
	case "rt.u64_to_float":
		// decision 0195: the conversions of UInt64 and Float32
		s.set(v, s.def1(true, "ucvtf $d0, $u0", s.word(v.Args[0])))
	case "rt.f32_round":
		s.set(v, s.def1(true, "fcvt $wd0, $u0\nfcvt $d0, $wd0", s.word(v.Args[0])))
	case "rt.i64_to_f32":
		s.set(v, s.def1(true, "scvtf $wd0, $u0\nfcvt $d0, $wd0", s.word(v.Args[0])))
	case "rt.float_bits":
		s.set(v, s.def1(false, "fmov $d0, $u0", s.word(v.Args[0])))
	case "rt.float_from_bits":
		s.set(v, s.def1(true, "fmov $d0, $u0", s.word(v.Args[0])))
	case "rt.bytes_word":
		ws := s.words(v.Args[0])
		d := s.def1(false, "add $d0, $u0, #16", s.real(ws[0]))
		s.set(v, s.def1(false, "ldr $d0, [$u0, $u1]", d, s.word(v.Args[1])))
	case "rt.sys6":
		// a call whose `bl` is `svc #0` (decision 0206): the arguments in
		// x0 to x5 and the number in x6 (moved to x8), the result in x0
		in := &inst{kind: iCall, svc: true, defs: s.newVregs([]bool{false})}
		for _, k := range []int{1, 2, 3, 4, 5, 6, 0} {
			r := s.word(v.Args[k])
			in.srcs = append(in.srcs, rsrc(r))
			in.uses = append(in.uses, r)
		}
		s.add(in)
		s.clobber()
		s.set(v, in.defs...)
	case "rt.sys_restorer":
		s.g.restorerRef = true
		s.set(v, s.symAddr(s.g.local("xo_sigreturn")))
	case "rt.call_c2":
		// a C ABI call of an address (decision 0206 stage 2): the
		// arguments in x0 and x1, the address in x2 (`blr x2`)
		in := &inst{kind: iCall, cptr: true, defs: s.newVregs([]bool{false})}
		for _, k := range []int{1, 2, 0} {
			r := s.word(v.Args[k])
			in.srcs = append(in.srcs, rsrc(r))
			in.uses = append(in.uses, r)
		}
		s.add(in)
		s.clobber()
		s.set(v, in.defs...)
	case "rt.sys_words":
		s.g.wordsRef = true
		s.set(v, s.symAddr(s.g.local("xo_sys_words")))
	case "rt.popcount":
		s.set(v, s.def1(false, "fmov d30, $u0\ncnt v30.8b, v30.8b\naddv b30, v30.8b\nfmov $d0, d30", s.word(v.Args[0])))
	case "rt.ctz":
		s.set(v, s.def1(false, "rbit $d0, $u0\nclz $d0, $d0", s.word(v.Args[0])))
	case "rt.clz":
		s.set(v, s.def1(false, "clz $d0, $u0", s.word(v.Args[0])))
	case "rt.bytes_eq16", "rt.bytes_high16":
		ws := s.words(v.Args[0])
		d := s.def1(false, "add $d0, $u0, #16", s.real(ws[0]))
		uses := []vreg{d, s.word(v.Args[1])}
		cmp := "cmlt v30.16b, v30.16b, #0"
		if v.Aux.(string) == "rt.bytes_eq16" {
			if c, ok := constInt(v.Args[2]); ok {
				cmp = fmt.Sprintf("movi v31.16b, #%d\ncmeq v30.16b, v30.16b, v31.16b", c&255)
			} else {
				cmp = "dup v31.16b, $wu2\ncmeq v30.16b, v30.16b, v31.16b"
				uses = append(uses, s.word(v.Args[2]))
			}
		}
		s.set(v, s.def1(false, "ldr q30, [$u0, $u1]\n"+cmp+"\nshrn v30.8b, v30.8h, #4\nfmov $d0, d30", uses...))
	case "rt.rs_put_word":
		ws := s.words(v.Args[0])
		d := s.def1(false, "add $d0, $u0, #16", s.real(ws[0]))
		s.plain("str $u2, [$u0, $u1]", nil, d, s.word(v.Args[1]), s.word(v.Args[2]))
		s.set(v, ws...)
	case "rt.rs_set_len":
		s.set(v, s.words(v.Args[0])[0], s.word(v.Args[1]))
	case "rt.slot_load", "rt.slot_store", "rt.slot_swap", "rt.slot_cas", "rt.slot_add", "rt.slot_addr", "rt.fence", "rt.spin_hint":
		s.atomic(v)
	default:
		return s.inlineRaw(v)
	}
	return true
}

// inlineRaw lowers the raw memory intrinsics (decision 0207): loads and
// stores of a constant width, memcpy, memmove, memset, and memcmp of the
// platform library, a string with its length set, and an object's
// address both ways. It reports whether v was one.
func (s *fstate) inlineRaw(v *ssa.Value) bool {
	switch v.Aux.(string) {
	case "rt.mem_load":
		w, _ := constInt(v.Args[2])
		if off, ok := constInt(v.Args[1]); ok && rawImmOff(off, w) {
			// a constant offset is the instruction's scaled immediate
			// (decision 0207 stage 3: the heap record's words)
			ins := map[int64]string{1: "ldrb $wd0, [$u0, #%d]", 2: "ldrh $wd0, [$u0, #%d]", 4: "ldr $wd0, [$u0, #%d]"}[w]
			if ins == "" {
				ins = "ldr $d0, [$u0, #%d]"
			}
			s.set(v, s.def1(false, fmt.Sprintf(ins, off), s.real(s.word(v.Args[0]))))
			break
		}
		ins := map[int64]string{1: "ldrb $wd0, [$u0, $u1]", 2: "ldrh $wd0, [$u0, $u1]", 4: "ldr $wd0, [$u0, $u1]"}[w]
		if ins == "" {
			ins = "ldr $d0, [$u0, $u1]"
		}
		s.set(v, s.def1(false, ins, s.real(s.word(v.Args[0])), s.real(s.word(v.Args[1]))))
	case "rt.mem_store":
		w, _ := constInt(v.Args[3])
		if off, ok := constInt(v.Args[1]); ok && rawImmOff(off, w) {
			ins := map[int64]string{1: "strb $wu0, [$u1, #%d]", 2: "strh $wu0, [$u1, #%d]", 4: "str $wu0, [$u1, #%d]"}[w]
			if ins == "" {
				ins = "str $u0, [$u1, #%d]"
			}
			s.store(fmt.Sprintf(ins, off), s.real(s.word(v.Args[2])), s.word(v.Args[0]))
			s.set(v)
			break
		}
		ins := map[int64]string{1: "strb $wu0, [$u1, $u2]", 2: "strh $wu0, [$u1, $u2]", 4: "str $wu0, [$u1, $u2]"}[w]
		if ins == "" {
			ins = "str $u0, [$u1, $u2]"
		}
		s.store(ins, s.real(s.word(v.Args[2])), s.word(v.Args[0]), s.word(v.Args[1]))
		s.set(v)
	case "rt.mem_copy", "rt.mem_move", "rt.mem_set":
		fn := map[string]string{"rt.mem_copy": "memcpy", "rt.mem_move": "memmove", "rt.mem_set": "memset"}[v.Aux.(string)]
		s.rt(fn, regs(s.word(v.Args[0]), s.word(v.Args[1]), s.word(v.Args[2])))
		s.clobber()
		s.set(v)
	case "rt.prefetch":
		// decision 0207 stage 3: the heap's free list pops
		s.store("prfm pldl1keep, [$u0]", s.real(s.word(v.Args[0])))
		s.set(v)
	case "rt.mem_cmp":
		r := s.rt("memcmp", regs(s.word(v.Args[0]), s.word(v.Args[1]), s.word(v.Args[2])), false)
		s.set(v, s.def1(false, "sxtw $d0, $wu0", r[0]))
	case "rt.obj_addr":
		s.set(v, s.words(v.Args[0])[0])
	case "rt.obj_at":
		a := s.real(s.word(v.Args[0]))
		s.retain(v.Type, []vreg{a})
		s.set(v, a)
	default:
		return false
	}
	return true
}

// Long multiplies (decision 0208): the product of two values of 32 bits
// (masked by 0xFFFFFFFF, or `(x << 32) >> 32`, or known in range) is
// umull (smull) of their low halves, and a wrapping add of it umaddl
// (smaddl); a wrapping add of a wrapping shift left by a constant is an
// add with a shifted operand. The mask or the extension, used only
// there, is then dead.

// half32 returns the register whose low 32 bits are v, zero extended
// (sign extended when signed), and whether that took off a mask or an
// extension.
func (s *fstate) half32(v *ssa.Value, signed bool) (vreg, bool, bool) {
	if v.Type.Kind != ssa.KInt {
		return 0, false, false
	}
	if !signed && v.Op == ssa.OpAnd {
		for j := 0; j < 2; j++ {
			if k, ok := constInt(v.Args[j]); ok && k == 0xFFFFFFFF {
				return s.word(v.Args[1-j]), true, true
			}
		}
	}
	if signed && v.Op == ssa.OpShr && v.Args[0].Op == ssa.OpShl {
		k1, ok1 := constInt(v.Args[1])
		k2, ok2 := constInt(v.Args[0].Args[1])
		if ok1 && ok2 && k1 == 32 && k2 == 32 {
			return s.word(v.Args[0].Args[0]), true, true
		}
	}
	lo, hi := s.bounds(v)
	if signed && lo >= math.MinInt32 && hi <= math.MaxInt32 || !signed && lo >= 0 && hi <= math.MaxUint32 {
		return s.word(v), false, true
	}
	return 0, false, false
}

// mulLong returns umull or smull of x and y, when both are 32 bit values
// and one is masked or extended for it.
func (s *fstate) mulLong(x, y *ssa.Value) (string, []vreg, bool) {
	if s.g.goMode {
		return "", nil, false
	}
	for _, signed := range []bool{false, true} {
		a, ea, ok1 := s.half32(x, signed)
		if !ok1 {
			continue
		}
		b, eb, ok2 := s.half32(y, signed)
		if ok2 && (ea || eb) {
			if signed {
				return "smull $d0, $wu0, $wu1", []vreg{a, b}, true
			}
			return "umull $d0, $wu0, $wu1", []vreg{a, b}, true
		}
	}
	return "", nil, false
}

// wrapOp returns the operation of an rt.int_wrap call with a constant
// operation.
func wrapOp(v *ssa.Value) (int64, bool) {
	if v.Op != ssa.OpCall || v.Aux != "rt.int_wrap" {
		return 0, false
	}
	return constInt(v.Args[0])
}

// fusedAdd lowers rt.int_wrap(0, a, b) when a or b, used only there, is
// a long multiply (umaddl, smaddl) or a shift left by a constant (an
// add with a shifted operand).
func (s *fstate) fusedAdd(v *ssa.Value) bool {
	if s.g.goMode {
		return false
	}
	for j := 1; j <= 2; j++ {
		x, o := v.Args[j], v.Args[3-j]
		op, ok := wrapOp(x)
		if !ok || s.nuses[x] != 1 {
			continue
		}
		switch op {
		case 2:
			if text, us, ok := s.mulLong(x.Args[1], x.Args[2]); ok {
				text = text[:1] + "maddl $d0, $wu0, $wu1, $u2"
				s.set(v, s.def1(false, text, append(us, s.word(o))...))
				return true
			}
		case 3:
			if k, ok := constInt(x.Args[2]); ok && k&63 != 0 {
				s.set(v, s.def1(false, fmt.Sprintf("add $d0, $u0, $u1, lsl #%d", k&63), s.word(o), s.word(x.Args[1])))
				return true
			}
		}
	}
	return false
}

// ordArg returns the memory order of argument k of v (decision 0200):
// its constant 0 relaxed, 1 acquire, 2 release, 3 acq_rel, 4 seq_cst; a
// value that is not one of them is seq_cst.
func ordArg(v *ssa.Value, k int) int {
	if k >= len(v.Args) {
		return 4
	}
	o, ok := constInt(v.Args[k])
	if !ok || o < 0 || o > 4 {
		return 4
	}
	return int(o)
}

// cellAddr returns the address of the cell (container, index) of an
// atomic: the list's element, or an Atomic's value (the same layout: a
// header of three words, then words).
//
// The address form (decision 0207): a container that is an Int is an
// address, and the cell is word i from it.
func (s *fstate) cellAddr(v *ssa.Value) vreg {
	l := s.word(v.Args[0])
	if v.Args[0].Type.Kind == ssa.KInt {
		if k, ok := constInt(v.Args[1]); ok && k >= 0 && 8*k < 4096 {
			return s.def1(false, fmt.Sprintf("add $d0, $u0, #%d", 8*k), s.real(l))
		}
		return s.def1(false, "add $d0, $u0, $u1, lsl #3", s.real(l), s.word(v.Args[1]))
	}
	if k, ok := constInt(v.Args[1]); ok && k >= 0 && listHdr+8*k < 4096 {
		return s.def1(false, fmt.Sprintf("add $d0, $u0, #%d", listHdr+8*k), l)
	}
	b := s.def1(false, "add $d0, $u0, $u1, lsl #3", l, s.word(v.Args[1]))
	return s.def1(false, fmt.Sprintf("add $d0, $u0, #%d", listHdr), b)
}

// atomicDef adds an atomic instruction defining one register: kept when
// its result is dead and never folded into a branch (keep); early, since
// a template may write its result before it reads every operand.
func (s *fstate) atomicDef(text string, uses ...vreg) vreg {
	d := s.newVreg(false)
	in := s.plain(text, []vreg{d}, uses...)
	in.early = true
	in.keep = true
	s.clobber()
	return d
}

// atomicResult sets v, an Int or a Bool (a Bool cell holds 0 or 1), to
// register r.
func (s *fstate) atomicResult(v *ssa.Value, r vreg) {
	switch v.Type.Kind {
	case ssa.KInt, ssa.KBool:
		s.set(v, r)
	default:
		fail("the atomic `%s` returns a `%s`", v.Aux.(string), v.Type.Name)
	}
}

// atomic lowers the atomics and fences of decision 0200 inline: on
// darwin the LSE instructions every Apple arm64 CPU has (ldar, stlr,
// swp, cas, ldadd with the order's a and l; dmb, yield); on linux, which
// also runs on ARMv8.0, loads, stores, and fences inline and the read
// modify writes as calls of libgcc's outline atomics
// (__aarch64_cas8_acq_rel and friends), as the C runtime's
// -moutline-atomics objects do (selfhost/macho elflink.xo writes them
// when it links without cc).
func (s *fstate) atomic(v *ssa.Value) {
	name := v.Aux.(string)
	switch name {
	case "rt.spin_hint":
		s.store("yield")
		s.set(v)
		return
	case "rt.fence":
		switch ordArg(v, 0) {
		case 0:
		case 1:
			s.store("dmb ishld")
		default:
			s.store("dmb ish")
		}
		s.set(v)
		return
	case "rt.slot_addr":
		s.set(v, s.cellAddr(v))
		return
	}
	a := s.cellAddr(v)
	// the order of a read modify write: none, acquire, release, both
	ord := ordArg(v, len(v.Args)-1)
	sfx := [...]string{"", "a", "l", "al", "al"}[ord]
	model := [...]string{"relax", "acq", "rel", "acq_rel", "acq_rel"}[ord]
	switch name {
	case "rt.slot_load":
		ins := "ldar"
		if ord == 0 {
			ins = "ldr"
		}
		s.atomicResult(v, s.atomicDef(ins+" $d0, [$u0]", a))
	case "rt.slot_store":
		ins := "stlr"
		if ord == 0 {
			ins = "str"
		}
		s.store(ins+" $u0, [$u1]", s.word(v.Args[2]), a)
		s.set(v)
	case "rt.slot_swap", "rt.slot_add":
		val := s.word(v.Args[2])
		op := "ldadd"
		if name == "rt.slot_swap" {
			op = "swp"
		}
		if s.g.os == Darwin {
			s.atomicResult(v, s.atomicDef(op+sfx+" $u0, $d0, [$u1]", val, a))
		} else {
			r := s.rt("__aarch64_"+op+"8_"+model, regs(val, a), false)
			s.atomicResult(v, r[0])
		}
	default:
		// rt.slot_cas: the old value seen, compared with the expected one
		old := s.word(v.Args[2])
		nw := s.word(v.Args[3])
		if s.g.os == Darwin {
			s.set(v, s.atomicDef("mov $d0, $u0\ncas"+sfx+" $d0, $u1, [$u2]\ncmp $d0, $u0\ncset $d0, eq", old, nw, a))
		} else {
			r := s.rt("__aarch64_cas8_"+model, regs(old, nw, a), false)
			s.set(v, s.def1(false, "cmp $u0, $u1\ncset $d0, eq", r[0], old))
		}
	}
}

// rtCall lowers a call of the task runtime (`rt.name` is the C function
// xo_name, decision 0068): an Int, Bool, List, or handle is one word, a
// Str its bytes and length; the result is nothing or one word.
func (s *fstate) rtCall(v *ssa.Value) {
	if strings.HasPrefix(v.Aux.(string), "rt.pc.") {
		s.platformCall(v)
		return
	}
	if s.inlineRT(v) {
		return
	}
	var args []src
	// `rt.rs_*` functions take their Str operands as the string object
	// (the first is consumed, decision 0074); the others its bytes
	obj := strings.HasPrefix(v.Aux.(string), "rt.rs_")
	for _, x := range v.Args {
		ws := s.words(x)
		switch x.Type.Kind {
		case ssa.KInt, ssa.KBool, ssa.KList, ssa.KHandle, ssa.KFloat:
			args = append(args, rsrc(ws[0]))
		case ssa.KStr:
			if obj {
				args = append(args, rsrc(ws[0]), rsrc(ws[1]))
				break
			}
			d := s.def1(false, "add $d0, $u0, #16", s.real(ws[0]))
			args = append(args, rsrc(d), rsrc(ws[1]))
		default:
			fail("the runtime call `%s` takes a `%s`", v.Aux, x.Type.Name)
		}
	}
	name := "xo_" + strings.TrimPrefix(v.Aux.(string), "rt.")
	switch v.Type.Kind {
	case ssa.KUnit:
		s.rt(name, args)
		s.set(v)
	case ssa.KInt, ssa.KBool, ssa.KList, ssa.KHandle:
		s.set(v, s.rt(name, args, false)...)
	case ssa.KFloat:
		s.set(v, s.rt(name, args, true)...)
	case ssa.KStr:
		// the string object; its length in the frame's out slot
		o := s.rt(name, append(args, framesrc()), false)
		n := s.def1(false, "ldr $d0, [sp, #$OUT]")
		s.set(v, o[0], n)
	default:
		fail("the runtime call `%s` returns a `%s`", v.Aux, v.Type.Name)
	}
}

// platformCall lowers a call of a platform library function (decision
// 0213, std/native/sys_<os>.xo): a call of the bare C symbol with the C
// ABI, an Int or Int32 argument one word, a Float a double, Bytes its
// data address; an Int32 result sign extended from 32 bits, a UInt32 one
// zero extended. Apple arm64 passes the variadic arguments on the stack.
// A call for another OS (an arm of a `match` on `target.os` the passes
// keep) is a trap.
func (s *fstate) platformCall(v *ssa.Value) {
	os, sym, nfixed := ssa.PlatformParts(v.Aux.(string))
	if os != "darwin" || s.g.os != Darwin {
		s.add(&inst{kind: iPlain, text: "brk #1"})
		s.clobber()
		switch v.Type.Kind {
		case ssa.KInt:
			s.set(v, 0)
		case ssa.KFloat:
			s.set(v, s.konst(s.def1(true, "fmov $d0, xzr")))
		default:
			s.set(v)
		}
		return
	}
	in := &inst{kind: iCall, sym: s.g.sym(sym)}
	for _, x := range v.Args {
		ws := s.words(x)
		switch x.Type.Kind {
		case ssa.KInt, ssa.KFloat:
			in.srcs = append(in.srcs, rsrc(ws[0]))
			if ws[0] != 0 {
				in.uses = append(in.uses, ws[0])
			}
		case ssa.KStr:
			d := s.def1(false, "add $d0, $u0, #16", s.real(ws[0]))
			in.srcs = append(in.srcs, rsrc(d))
			in.uses = append(in.uses, d)
		default:
			fail("the platform call `%s` takes a `%s`", sym, x.Type.Name)
		}
	}
	switch v.Type.Kind {
	case ssa.KUnit:
	case ssa.KInt:
		in.defs = s.newVregs([]bool{false})
	case ssa.KFloat:
		in.defs = s.newVregs([]bool{true})
	default:
		fail("the platform call `%s` returns a `%s`", sym, v.Type.Name)
	}
	if s.g.os == Darwin && nfixed > 0 {
		in.vfix = nfixed
	}
	s.add(in)
	s.clobber()
	if v.Type.Kind == ssa.KInt && v.Type.Bits == 32 {
		ext := "sxtw $d0, $wu0"
		if v.Type.Unsigned {
			ext = "mov $wd0, $wu0"
		}
		s.set(v, s.def1(false, ext, in.defs[0]))
		return
	}
	s.set(v, in.defs...)
}
