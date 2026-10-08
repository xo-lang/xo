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
	offs    map[vreg]offset         // registers that are another plus a constant (lsr.go)
	// register allocation results
	loc     []loc
	order   []*mblock
	ninst   int
	slots   int
	saved   []int
	outArgs int
	frame   int
	stubs   strings.Builder
	nstub   int
	mm      *memo
}

func (g *gen) function(f *ssa.Func) {
	s := &fstate{g: g, f: f, fp: []bool{false}, vals: map[*ssa.Value][]vreg{}, bmap: map[*ssa.Block]*mblock{},
		nuses: map[*ssa.Value]int{}, users: map[*ssa.Value]*ssa.Value{}, lenParam: map[vreg]vreg{}, endMemo: map[*ssa.Block]*memo{}, lazy: map[*ssa.Value]bool{}}
	s.isel()
	s.dce()
	s.threadConst()
	s.threadCset()
	s.thread()
	s.jumpTables()
	s.dce()
	s.selectCompares()
	s.hoistConsts()
	s.reuseFlags()
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

func imm12(n int64) bool { return n >= 0 && n < 4096 }

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
			for _, e := range v.Succs {
				for _, a := range e.Args {
					s.nuses[a]++
				}
			}
		}
	}
	blocks := rpo(f)
	idom := dominators(blocks)
	s.planLSR(blocks, idom)
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
		}
	}
	for i, b := range blocks {
		mb := s.bmap[b]
		mb.id = s.nblock
		s.nblock++
		s.hot = append(s.hot, mb)
		s.cur = mb
		if len(b.Preds) == 1 && s.endMemo[b.Preds[0].From] != nil {
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
		if i == 0 {
			var defs []vreg
			for _, p := range b.Params {
				ws := s.newVregs(s.g.layout(p.Type))
				s.vals[p] = ws
				defs = append(defs, ws...)
			}
			s.add(&inst{kind: iEntry, defs: defs})
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

// floatCond are the conditions after fcmp that are false when either
// operand is NaN (ordered), except ne (unordered or not equal).
var floatCond = map[ssa.Op]string{ssa.OpEq: "eq", ssa.OpNe: "ne", ssa.OpLt: "mi", ssa.OpLe: "ls", ssa.OpGt: "gt", ssa.OpGe: "ge"}

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
		cond = floatCond[v.Op]
		if b.Op == ssa.OpConst && b.Aux == float64(0) && !math.Signbit(b.Aux.(float64)) {
			return "fcmp $u0, #0.0", []vreg{s.word(a)}, cond
		}
		return "fcmp $u0, $u1", []vreg{s.word(a), s.word(b)}, cond
	}
	fail("compare of `%s`", a.Type.Name)
	return
}

func (s *fstate) set(v *ssa.Value, ws ...vreg) { s.vals[v] = ws }

func (s *fstate) instr(v *ssa.Value) {
	g := s.g
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
		mul := func() vreg {
			if low != "" {
				return s.def1(false, low, s.word(x))
			}
			return s.def1(false, "mul $d0, $u0, $u1", a, b)
		}
		if v.Pos == "" {
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
		s.set(v, s.def1(false, ins+" $d0, $u0, $u1", s.word(v.Args[0]), s.word(v.Args[1])))
	case ssa.OpShl, ssa.OpShr:
		s.shift(v)
	case ssa.OpNeg:
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
		off, n := g.fieldWords(v.Args[0].Type, v.Aux.(int))
		ws := s.words(v.Args[0])
		s.set(v, ws[off:off+n]...)
	case ssa.OpSetField:
		off, n := g.fieldWords(v.Type, v.Aux.(int))
		ws := append([]vreg(nil), s.words(v.Args[0])...)
		copy(ws[off:off+n], s.words(v.Args[1]))
		s.set(v, ws...)
	case ssa.OpVariant:
		s.set(v, s.variant(v.Type, v.Aux.(int), v.Args)...)
	case ssa.OpTag:
		s.set(v, s.words(v.Args[0])[0])
	case ssa.OpPayload:
		r := v.Aux.(ssa.PayloadRef)
		off, n := g.payloadWords(v.Args[0].Type, r.Tag, r.Field)
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
	case ssa.OpUnbox:
		s.unboxOp(v)
	case ssa.OpFault:
		ws := s.words(v.Args[0])
		d := s.def1(false, "add $d0, $u0, #16", ws[0])
		s.add(&inst{kind: iCall, sym: g.sym("xo_fault"), srcs: []src{rsrc(d), rsrc(ws[1]), symsrc(g.loc(v.Pos))}, uses: []vreg{d, s.real(ws[1])}, noret: true})
		s.add(&inst{kind: iTrap})
	case ssa.OpRetain:
		x := v.Args[0]
		if x.Op == ssa.OpConst || s.cancelRC(v) {
			return // literals are immortal; see cancelRC
		}
		s.retain(x.Type, s.words(x))
	case ssa.OpRelease:
		x := v.Args[0]
		if s.cancel[v] {
			return
		}
		s.release(x.Type, s.words(x))
	case ssa.OpReuse:
		s.reuse(v)
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
	if _, ok := constInt(a); ok && v.Op == ssa.OpAdd {
		if _, ok2 := constInt(b); !ok2 {
			a, b = b, a
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
		s.set(v, s.checked(false, text, []vreg{av}, ins, "vs", "xo_fault_overflow", rsrc(av), isrc(opChar[v.Op]), isrc(mustConst(b)), symsrc(g.loc(v.Pos))))
		return
	}
	av, bv := s.word(a), s.word(b)
	text := ins + flag + " $d0, $u0, $u1"
	if !checked {
		s.set(v, s.def1(false, text, av, bv))
		return
	}
	s.set(v, s.checked(false, text, []vreg{av, bv}, ins, "vs", "xo_fault_overflow", rsrc(av), isrc(opChar[v.Op]), rsrc(bv), symsrc(g.loc(v.Pos))))
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

func (s *fstate) shift(v *ssa.Value) {
	g := s.g
	a := s.word(v.Args[0])
	if k, ok := constInt(v.Args[1]); ok && k >= 0 && k < 64 {
		if v.Op == ssa.OpShr {
			s.set(v, s.def1(false, fmt.Sprintf("asr $d0, $u0, #%d", k), a))
			return
		}
		r := s.def1(false, fmt.Sprintf("lsl $d0, $u0, #%d", k), a)
		if v.Pos != "" {
			back := s.def1(false, fmt.Sprintf("asr $d0, $u0, #%d", k), r)
			s.check("cmp $u0, $u1", []vreg{back, a}, "ne", "xo_fault_shl", rsrc(a), isrc(k), symsrc(g.loc(v.Pos)))
		}
		s.set(v, r)
		return
	}
	b := s.word(v.Args[1])
	if v.Pos != "" {
		s.check("cmp $u0, #64", []vreg{b}, "hs", "xo_fault_shift", rsrc(b), symsrc(g.loc(v.Pos)))
	}
	if v.Op == ssa.OpShr {
		s.set(v, s.def1(false, "asr $d0, $u0, $u1", a, b))
		return
	}
	r := s.def1(false, "lsl $d0, $u0, $u1", a, b)
	if v.Pos != "" {
		back := s.def1(false, "asr $d0, $u0, $u1", r, b)
		s.check("cmp $u0, $u1", []vreg{back, a}, "ne", "xo_fault_shl", rsrc(a), rsrc(b), symsrc(g.loc(v.Pos)))
	}
	s.set(v, r)
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

func (s *fstate) inc(p vreg) {
	c := s.def1(false, "ldr $d0, [$u0]", p)
	c1 := s.def1(false, "add $d0, $u0, #1", c)
	s.store("str $u0, [$u1]", c1, p)
}

func (s *fstate) retain(t *ssa.Type, ws []vreg) {
	switch t.Kind {
	case ssa.KStr, ssa.KList, ssa.KBox:
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
	case ssa.KStr, ssa.KList:
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
	cp := s.rt("xo_rl_copy", []src{rsrc(l), isrc(s.g.esize(et)), s.elemFn("dup", et)}, false)
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

// loadElem loads the element words at base (as integer registers when
// asInt).
func (s *fstate) loadElem(base vreg, et *ssa.Type, asInt bool) []vreg {
	m := s.m()
	if et.Kind == ssa.KBool {
		d := s.def1(false, fmt.Sprintf("ldrb $wd0, [$u0, #%d]", listHdr), base)
		m.loaded[d] = memAt{base, listHdr, m.epoch}
		return []vreg{d}
	}
	var out []vreg
	for k, fp := range s.g.layout(et) {
		d := s.def1(fp && !asInt, fmt.Sprintf("ldr $d0, [$u0, #%d]", listHdr+8*k), base)
		m.loaded[d] = memAt{base, listHdr + 8*k, m.epoch}
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

func (s *fstate) storeElem(base vreg, et *ssa.Type, ws []vreg) {
	m := s.m()
	same := func(w vreg, off int) bool {
		at, ok := m.loaded[w]
		return ok && at == memAt{base, off, m.epoch}
	}
	if et.Kind == ssa.KBool {
		if !same(ws[0], listHdr) {
			s.store(fmt.Sprintf("strb $wu0, [$u1, #%d]", listHdr), ws[0], base)
		}
		return
	}
	var skip []bool
	for k := range s.g.layout(et) {
		skip = append(skip, same(ws[k], listHdr+8*k))
	}
	for k := range s.g.layout(et) {
		if !skip[k] {
			s.store(fmt.Sprintf("str $u0, [$u1, #%d]", listHdr+8*k), ws[k], base)
		}
	}
}

func (s *fstate) listLen(l vreg) vreg {
	m := s.m()
	if n, ok := m.lens[l]; ok {
		return n
	}
	n := s.def1(false, "ldr $d0, [$u0, #8]", l)
	m.lens[l] = n
	return n
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
	if s.provenIndex(l, i) {
		s.indexFacts(i)
		return
	}
	n := s.listLen(l)
	s.check("cmp $u0, $u1", []vreg{i, n}, "hs", "xo_fault_index", rsrc(i), rsrc(n), symsrc(s.g.loc(v.Pos)))
	s.indexFacts(i)
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
	switch name {
	case "list.new":
		s.set(v, s.rt("xo_rl_new", []src{isrc(4), isrc(g.esize(v.Type.Elems[0]))}, false)...)
	case "list.len":
		s.set(v, s.listLen(s.word(v.Args[0])))
	case "list.index":
		l, i := s.word(v.Args[0]), s.word(v.Args[1])
		s.boundsCheck(v, l, i)
		et := v.Type
		base := s.elemBase(l, i, g.esize(et))
		s.set(v, s.loadElem(base, et, false)...)
	case "list.get":
		l, i := s.word(v.Args[0]), s.word(v.Args[1])
		et := v.Args[0].Type.Elems[0]
		n := s.listLen(l)
		s.set(v, s.option(v.Type, "cmp $u0, $u1", []vreg{i, n}, "lo", nil, func() ([]vreg, []src) {
			return s.loadElem(s.elemBase(l, i, g.esize(et)), et, true), nil
		}, nil)...)
	case "list.set":
		l, i := s.word(v.Args[0]), s.word(v.Args[1])
		et := v.Args[2].Type
		x := s.storeWords(v.Args[2])
		s.boundsCheck(v, l, i)
		base := s.elemBase(l, i, g.esize(et))
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
			s.storeElem(s.elemBase(lm, n, g.esize(et)), et, x)
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
			return s.loadElem(s.elemBase(l, n1, g.esize(et)), et, true), regs(l, n1)
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
	case "Float.min":
		s.set(v, s.def1(true, "fcmp $u0, $u1\nfcsel $d0, $u0, $u1, mi", s.word(v.Args[0]), s.word(v.Args[1])))
	case "Float.max":
		s.set(v, s.def1(true, "fcmp $u0, $u1\nfcsel $d0, $u1, $u0, mi", s.word(v.Args[0]), s.word(v.Args[1])))
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
