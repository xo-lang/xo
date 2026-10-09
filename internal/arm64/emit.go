package arm64

import (
	"fmt"
	"strconv"
	"strings"
)

// mloc is a machine location: a register, or memory at [base, #off].
type mloc struct {
	reg  int // -1 for memory
	base string
	off  int
	// Go mode (goasm.go): name is the argument's name when base is "FP"
	// (`name+off(FP)`), and size 1 is a byte (a Bool argument)
	name string
	size int
}

type msrcKind int

const (
	mLoc msrcKind = iota
	mImm
	mSym
	mFrame
	mZero
)

type msrc struct {
	kind msrcKind
	l    mloc
	imm  int64
	sym  string
}

type move struct {
	dst mloc
	src msrc
}

func regName(r int, w bool) string {
	if r >= fpBase {
		return "d" + strconv.Itoa(r-fpBase)
	}
	if r == 31 {
		if w {
			return "wzr"
		}
		return "xzr"
	}
	if w {
		return "w" + strconv.Itoa(r)
	}
	return "x" + strconv.Itoa(r)
}

const (
	scratch0 = 16
	scratch1 = 17
	scratch2 = 15
	fscratch = fpBase + 31 // parallel move cycles
)

var gpScratch = []int{scratch0, scratch1, scratch2}
var fpScratch = []int{fpBase + 30, fpBase + 29, fpBase + 31}

// layout of the frame (sp relative, after the prologue), size bytes,
// 16 byte aligned:
//
//	[0, outArgs)             stack arguments of calls (when any)
//	[outOff, outOff+8)       the $OUT slot (length out parameter)
//	[spillBase, ...)         spill slots, 8 bytes each
//	[saveBase, ...)          callee saved registers
//	[rec, rec+16)            the frame record (x29, x30); x29 = sp + rec
//
// The frame record is at the bottom (rec 0) when no call passes stack
// arguments, so one `stp x29, x30, [sp, #-size]!` makes the frame. A
// leaf function (no calls, nothing saved or spilled) has no frame:
// x29 keeps pointing at its caller's record. Incoming stack arguments
// are at [sp, #size + 8k].
type frameInfo struct {
	outArgs, outOff, spillBase, saveBase, rec, size int
	leaf                                            bool
}

func align16(n int) int { return (n + 15) &^ 15 }

func (s *fstate) label(b *mblock) string {
	return s.g.local(fmt.Sprintf("f%d_b%d", s.g.nfunc, b.id))
}

func (s *fstate) vloc(v vreg, fr *frameInfo) (mloc, bool) {
	l := s.loc[v]
	if l.reg >= 0 {
		return mloc{reg: l.reg}, true
	}
	if l.slot < 0 {
		return mloc{}, false
	}
	return mloc{reg: -1, base: "sp", off: fr.spillBase + 8*l.slot}, true
}

func (s *fstate) emit() {
	g := s.g
	g.nfunc++
	if g.goMode {
		s.goEmit()
		return
	}
	fr := &frameInfo{}
	for _, b := range s.order {
		for _, in := range b.insts {
			if in.kind == iCall {
				n := 0
				// stack arguments, and the result words past the
				// registers, which the callee stores in the same area
				for _, l := range append(s.abiLocs(in.srcs), s.abiLocs(regs(in.defs...))...) {
					if l.reg < 0 {
						n = max(n, l.slot+1)
					}
				}
				if 8*n > fr.outArgs {
					fr.outArgs = 8 * n
				}
			}
		}
	}
	calls := false
	for _, b := range s.order {
		for _, in := range b.insts {
			calls = calls || in.kind == iCall
		}
	}
	fr.leaf = !calls && len(s.saved) == 0 && s.slots == 0 && !s.out
	base := 0
	if fr.outArgs == 0 {
		base = 16 // the frame record comes first
	}
	fr.outOff = base + fr.outArgs
	fr.spillBase = fr.outOff
	if s.out {
		fr.spillBase += 8
	}
	fr.saveBase = fr.spillBase + 8*s.slots
	if fr.outArgs == 0 {
		fr.size = align16(fr.saveBase + 8*len(s.saved))
	} else {
		fr.rec = align16(fr.saveBase + 8*len(s.saved))
		fr.size = fr.rec + 16
	}
	if fr.leaf {
		fr.size = 0
	}
	if fr.size > 4000 {
		fail("frame of %s too large (%d bytes)", s.f.Name, fr.size)
	}
	b := &strings.Builder{}
	defer func() { g.text.WriteString(pairLoads(b.String())) }()
	fmt.Fprintf(b, "// func %s\n", s.f.Name)
	g.funcHeader(b, g.fname(s.f.Name), false)
	pads := false
	for _, mb := range s.order {
		for _, in := range mb.insts {
			pads = pads || in.pad != nil
		}
	}
	s.cfiStart(b, pads)
	s.prologue(b, fr)
	s.cfiFrame(b, fr)
	// blocks reached once branches skip the blocks that only jump
	live := map[*mblock]bool{}
	var walk func(mb *mblock)
	walk = func(mb *mblock) {
		if live[mb] {
			return
		}
		live[mb] = true
		for _, t := range mb.succs {
			walk(s.fwd(t, fr))
			if ti := s.fwd(t, fr).insts; len(ti) == 1 && ti[0].kind == iCondBr {
				for _, t2 := range ti[0].targets {
					walk(s.fwd(t2, fr))
				}
			}
		}
	}
	walk(s.order[0])
	var emitted []*mblock
	for _, mb := range s.order {
		if live[mb] {
			emitted = append(emitted, mb)
		}
	}
	for bi, mb := range emitted {
		var next *mblock
		if bi+1 < len(emitted) {
			next = emitted[bi+1]
		}
		fmt.Fprintf(b, "%s:\n", s.label(mb))
		s.cur = mb
		for k, in := range mb.insts {
			s.emitInst(b, in, fr, next)
			if k == 0 && checkHead(mb) != nil {
				fmt.Fprintf(b, "%s_c:\n", s.label(mb)) // past the check (rotateJump)
			}
		}
	}
	var loop map[string]int
	if g.place.Loop > 2 {
		loop = s.innerLoops(g.place.Outer)
	}
	body := alignLoops(b.String(), g.place.Loop, g.place.LoopFall, g.place.LoopOff, loop)
	b.Reset()
	b.WriteString(body)
	if fr.leaf && s.stubs.Len() > 0 {
		// a leaf function's fault stubs make a frame record before
		// their call (emitInst), so the unwinder can go past them
		b.WriteString("\t.cfi_def_cfa w29, 16\n\t.cfi_offset w30, -8\n\t.cfi_offset w29, -16\n")
	}
	b.WriteString(s.stubs.String())
	b.WriteString("\t.cfi_endproc\n")
	if sites := append(s.sites, s.stubSites...); len(sites) > 0 {
		s.lsda(sites, fr)
	}
	b.WriteString("\n")
}

func subSP(b *strings.Builder, op string, n int) {
	if n >= 4096 {
		fmt.Fprintf(b, "\t%s sp, sp, #%d, lsl #12\n", op, n>>12)
		n &= 4095
	}
	if n > 0 {
		fmt.Fprintf(b, "\t%s sp, sp, #%d\n", op, n)
	}
}

func (s *fstate) prologue(b *strings.Builder, fr *frameInfo) {
	if fr.leaf {
		return
	}
	if fr.rec == 0 && fr.size <= 504 {
		fmt.Fprintf(b, "\tstp x29, x30, [sp, #-%d]!\n\tmov x29, sp\n", fr.size)
	} else {
		subSP(b, "sub", fr.size)
		if fr.rec <= 504 {
			fmt.Fprintf(b, "\tstp x29, x30, [sp, #%d]\n", fr.rec)
		} else {
			fmt.Fprintf(b, "\tstr x29, [sp, #%d]\n\tstr x30, [sp, #%d]\n", fr.rec, fr.rec+8)
		}
		fmt.Fprintf(b, "\tadd x29, sp, #%d\n", fr.rec)
	}
	s.saves(b, fr, "str", "stp")
}

// saves stores or loads the callee saved registers, in pairs when it
// can. A pair puts the higher register first (`stp x20, x19`), as LLVM
// does: on Apple M4 the other order made recursive fib 1.4x slower
// (gotchas/arm64-stp-pair-order.md).
func (s *fstate) saves(b *strings.Builder, fr *frameInfo, one, two string) {
	for k := 0; k < len(s.saved); k++ {
		r, off := s.saved[k], fr.saveBase+8*k
		if k+1 < len(s.saved) && (s.saved[k+1] >= fpBase) == (r >= fpBase) && off <= 496 {
			fmt.Fprintf(b, "\t%s %s, %s, [sp, #%d]\n", two, regName(s.saved[k+1], false), regName(r, false), off)
			k++
			continue
		}
		fmt.Fprintf(b, "\t%s %s, [sp, #%d]\n", one, regName(r, false), off)
	}
}

func (s *fstate) epilogue(b *strings.Builder, fr *frameInfo) {
	if fr.leaf {
		b.WriteString("\tret\n")
		return
	}
	s.saves(b, fr, "ldr", "ldp")
	if fr.rec == 0 && fr.size <= 504 {
		fmt.Fprintf(b, "\tldp x29, x30, [sp], #%d\n\tret\n", fr.size)
		return
	}
	if fr.rec <= 504 {
		fmt.Fprintf(b, "\tldp x29, x30, [sp, #%d]\n", fr.rec)
	} else {
		fmt.Fprintf(b, "\tldr x29, [sp, #%d]\n\tldr x30, [sp, #%d]\n", fr.rec, fr.rec+8)
	}
	subSP(b, "add", fr.size)
	b.WriteString("\tret\n")
}

var invCond = map[string]string{"eq": "ne", "ne": "eq", "lt": "ge", "ge": "lt", "gt": "le", "le": "gt",
	"lo": "hs", "hs": "lo", "hi": "ls", "ls": "hi", "mi": "pl", "pl": "mi", "vs": "vc", "vc": "vs"}

func (s *fstate) msrcOf(a src, fr *frameInfo) msrc {
	switch a.kind {
	case sReg:
		if a.v == 0 {
			return msrc{kind: mZero}
		}
		l, ok := s.vloc(a.v, fr)
		if !ok {
			return msrc{kind: mZero}
		}
		return msrc{kind: mLoc, l: l}
	case sImm:
		return msrc{kind: mImm, imm: a.imm}
	case sSym:
		return msrc{kind: mSym, sym: a.sym}
	}
	return msrc{kind: mFrame}
}

// callMoves returns the moves that put srcs in their ABI locations
// (stack slots at [base, #off0 + 8k]).
func (s *fstate) callMoves(srcs []src, fr *frameInfo, base string, off0 int) []move {
	var ms []move
	for k, l := range s.abiLocs(srcs) {
		dst := mloc{reg: l.reg}
		if l.reg < 0 {
			dst = mloc{reg: -1, base: base, off: off0 + 8*l.slot}
		}
		ms = append(ms, move{dst, s.msrcOf(srcs[k], fr)})
	}
	return ms
}

func (s *fstate) emitInst(b *strings.Builder, in *inst, fr *frameInfo, next *mblock) {
	switch in.kind {
	case iPlain:
		s.emitPlain(b, in, fr)
	case iPmove:
		var ms []move
		for k, d := range in.defs {
			l, ok := s.vloc(d, fr)
			if !ok {
				continue
			}
			ms = append(ms, move{l, s.msrcOf(in.srcs[k], fr)})
		}
		s.parallel(b, ms, fr)
	case iCall:
		if s.g.goMode {
			s.goEmitCall(b, in, fr)
			return
		}
		s.parallel(b, s.callMoves(in.srcs, fr, "sp", 0), fr)
		var csTo string
		if in.pad != nil {
			from, to := s.callSiteLabels()
			fmt.Fprintf(b, "%s:\n", from)
			s.sites = append(s.sites, callSite{from, to, s.padOf(in)})
			csTo = to
		}
		if in.indirect {
			b.WriteString("\tblr x0\n")
		} else {
			fmt.Fprintf(b, "\tbl %s\n", in.sym)
		}
		if csTo != "" {
			fmt.Fprintf(b, "%s:\n", csTo)
		}
		if in.noret {
			return
		}
		var ms []move
		for k, l := range s.abiLocs(regs(in.defs...)) {
			src := mloc{reg: l.reg}
			if l.reg < 0 {
				// a result word past the registers: the callee stored it
				// in the stack argument area
				src = mloc{reg: -1, base: "sp", off: 8 * l.slot}
			}
			if d, ok := s.vloc(in.defs[k], fr); ok {
				ms = append(ms, move{d, msrc{kind: mLoc, l: src}})
			}
		}
		s.parallel(b, ms, fr)
	case iEntry:
		if s.g.goMode {
			var ms []move
			for k, d := range in.defs {
				if dl, ok := s.vloc(d, fr); ok {
					ms = append(ms, move{dl, msrc{kind: mLoc, l: in.gargs[k]}})
				}
			}
			s.parallel(b, ms, fr)
			return
		}
		var ms []move
		for k, l := range s.abiLocs(regs(in.defs...)) {
			src := mloc{reg: l.reg}
			if l.reg < 0 {
				src = mloc{reg: -1, base: "sp", off: fr.size + 8*l.slot}
			}
			if d, ok := s.vloc(in.defs[k], fr); ok {
				ms = append(ms, move{d, msrc{kind: mLoc, l: src}})
			}
		}
		s.parallel(b, ms, fr)
	case iRet:
		if s.g.goMode {
			var ms []move
			for k, a := range in.srcs {
				ms = append(ms, move{in.gargs[k], s.msrcOf(a, fr)})
			}
			s.parallel(b, ms, fr)
			b.WriteString("\tret\n")
			return
		}
		var ms []move
		for k, l := range s.abiLocs(in.srcs) {
			dst := mloc{reg: l.reg}
			if l.reg < 0 {
				// past the registers: into the caller's stack argument
				// area (its arguments were all read at entry)
				dst = mloc{reg: -1, base: "sp", off: fr.size + 8*l.slot}
			}
			ms = append(ms, move{dst, s.msrcOf(in.srcs[k], fr)})
		}
		s.parallel(b, ms, fr)
		s.epilogue(b, fr)
	case iJump:
		t := s.fwd(in.targets[0], fr)
		if t == next {
			return
		}
		if s.rotateJump(b, t, fr) {
			return
		}
		if ti := t.insts; len(ti) == 1 && ti[0].kind == iCondBr && t != s.cur {
			// a loop header that only tests: test here (one taken branch
			// per iteration instead of two)
			s.emitInst(b, ti[0], fr, next)
			return
		}
		if t != next {
			fmt.Fprintf(b, "\tb %s\n", s.label(t))
		}
	case iCondBr:
		t, f := s.fwd(in.targets[0], fr), s.fwd(in.targets[1], fr)
		if t == f {
			if t != next {
				fmt.Fprintf(b, "\tb %s\n", s.label(t))
			}
			return
		}
		// fall: the target the branch falls through to; a moved latch
		// is copied after the branch instead (lsr.go dupLatch)
		fall := next
		if l := s.dupLatch(t, f, next); l != nil {
			fall = l
			defer s.emitLatch(b, l, fr, next)
		}
		if in.cond == "cbnz" {
			r := s.useReg(b, in.uses[0], 0, fr)
			if t == fall {
				fmt.Fprintf(b, "\tcbz %s, %s\n", r, s.label(f))
				return
			}
			fmt.Fprintf(b, "\tcbnz %s, %s\n", r, s.label(t))
		} else if in.cond == "tbz" || in.cond == "tbnz" {
			r := s.useReg(b, in.uses[0], 0, fr)
			c := in.cond
			if t == fall {
				c = map[string]string{"tbz": "tbnz", "tbnz": "tbz"}[c]
				fmt.Fprintf(b, "\t%s %s, #%d, %s\n", c, r, in.bit, s.label(f))
				return
			}
			fmt.Fprintf(b, "\t%s %s, #%d, %s\n", c, r, in.bit, s.label(t))
		} else {
			if in.text != "" { // empty: the flags of the predecessor's compare (reuseFlags)
				s.emitPlain(b, &inst{text: in.text, uses: in.uses}, fr)
			}
			if t == fall {
				fmt.Fprintf(b, "\tb.%s %s\n", invCond[in.cond], s.label(f))
				return
			}
			fmt.Fprintf(b, "\tb.%s %s\n", in.cond, s.label(t))
		}
		if f != fall {
			fmt.Fprintf(b, "\tb %s\n", s.label(f))
		}
	case iSwitch:
		s.emitSwitch(b, in, fr)
	case iTrap:
		b.WriteString("\tbrk #1\n")
	}
}

// useReg returns the register name of use v, reloading a spilled one
// into scratch register k of its class.
func (s *fstate) useReg(b *strings.Builder, v vreg, k int, fr *frameInfo) string {
	if v == 0 {
		return "xzr"
	}
	l, _ := s.vloc(v, fr)
	if l.reg >= 0 {
		return regName(l.reg, false)
	}
	r := gpScratch[k]
	if s.fp[v] {
		r = fpScratch[k]
	}
	fmt.Fprintf(b, "\tldr %s, [%s, #%d]\n", regName(r, false), l.base, l.off)
	return regName(r, false)
}

// emitPlain expands a template.
func (s *fstate) emitPlain(b *strings.Builder, in *inst, fr *frameInfo) {
	t := in.text
	// reload the spilled uses the template names
	useReg := map[int]int{}
	nscratch := [2]int{}
	for i := 0; i < len(t); i++ {
		if t[i] != '$' {
			continue
		}
		j := i + 1
		if j < len(t) && t[j] == 'w' {
			j++
		}
		if j+1 < len(t) && t[j] == 'u' {
			k := int(t[j+1] - '0')
			if _, done := useReg[k]; done {
				continue
			}
			v := in.uses[k]
			if v == 0 {
				useReg[k] = 31
				continue
			}
			l, _ := s.vloc(v, fr)
			if l.reg >= 0 {
				useReg[k] = l.reg
				continue
			}
			cls := 0
			pool := gpScratch
			if s.fp[v] {
				cls, pool = 1, fpScratch
			}
			r := pool[nscratch[cls]]
			nscratch[cls]++
			fmt.Fprintf(b, "\tldr %s, [%s, #%d]\n", regName(r, false), l.base, l.off)
			useReg[k] = r
		}
	}
	defReg := map[int]int{}
	var stores []string
	for k, d := range in.defs {
		l, ok := s.vloc(d, fr)
		if ok && l.reg >= 0 {
			defReg[k] = l.reg
			continue
		}
		r := gpScratch[0]
		if s.fp[d] {
			r = fpScratch[0]
		}
		defReg[k] = r
		if ok {
			stores = append(stores, fmt.Sprintf("\tstr %s, [%s, #%d]\n", regName(r, false), l.base, l.off))
		}
	}
	if t == "mov $d0, $u0" && defReg[0] == useReg[0] && len(stores) == 0 {
		return // a copy into the same register
	}
	var out strings.Builder
	out.WriteByte('\t')
	for i := 0; i < len(t); i++ {
		c := t[i]
		if c == '\n' {
			out.WriteString("\n\t")
			continue
		}
		if c != '$' {
			out.WriteByte(c)
			continue
		}
		if strings.HasPrefix(t[i:], "$OUT") {
			out.WriteString(strconv.Itoa(fr.outOff))
			i += 3
			continue
		}
		j := i + 1
		w := false
		if t[j] == 'w' {
			w = true
			j++
		}
		k := int(t[j+1] - '0')
		if t[j] == 'u' {
			out.WriteString(regName(useReg[k], w))
		} else {
			out.WriteString(regName(defReg[k], w))
		}
		i = j + 1
	}
	out.WriteByte('\n')
	b.WriteString(out.String())
	if in.fault != nil && s.g.goMode {
		s.goFaultStub(b, in, fr)
	} else if in.fault != nil {
		s.nstub++
		l := s.g.local(fmt.Sprintf("f%d_fault%d", s.g.nfunc, s.nstub))
		fmt.Fprintf(b, "\tb.%s %s\n", in.fault.cond, l)
		fmt.Fprintf(&s.stubs, "%s:\n", l)
		if fr.leaf {
			// a frame record for the unwinder (the call overwrites x30)
			s.stubs.WriteString("\tstp x29, x30, [sp, #-16]!\n\tmov x29, sp\n")
		}
		ms := s.callMoves(in.fault.args, fr, "sp", 0)
		s.recoverOperand(&s.stubs, in, ms, fr)
		s.parallel(&s.stubs, ms, fr)
		if in.pad != nil {
			from, to := s.callSiteLabels()
			s.stubSites = append(s.stubSites, callSite{from, to, s.padOf(in)})
			fmt.Fprintf(&s.stubs, "%s:\n\tbl %s\n%s:\n\tbrk #1\n", from, in.fault.fn, to)
		} else {
			fmt.Fprintf(&s.stubs, "\tbl %s\n\tbrk #1\n", in.fault.fn)
		}
	}
	for _, st := range stores {
		b.WriteString(st)
	}
}

// ---------------------------------------------------------------------------
// Parallel moves

func sameLoc(a, b mloc) bool {
	if a.reg >= 0 || b.reg >= 0 {
		return a.reg == b.reg
	}
	return a.base == b.base && a.off == b.off
}

// parallel emits moves that all read their sources before any writes
// its destination: moves whose destination no other pending move reads
// go first; a cycle is broken by saving one destination in a scratch
// register. Moves from constants come last.
func (s *fstate) parallel(b *strings.Builder, moves []move, fr *frameInfo) {
	var pending, consts []move
	for _, m := range moves {
		if m.src.kind != mLoc {
			consts = append(consts, m)
			continue
		}
		if sameLoc(m.dst, m.src.l) {
			continue
		}
		pending = append(pending, m)
	}
	for len(pending) > 0 {
		progress := false
		for i, m := range pending {
			read := false
			for j, o := range pending {
				if j != i && sameLoc(o.src.l, m.dst) {
					read = true
					break
				}
			}
			if !read {
				s.move(b, m.dst, m.src, fr)
				pending = append(pending[:i], pending[i+1:]...)
				progress = true
				break
			}
		}
		if progress {
			continue
		}
		m := pending[0]
		tmp := mloc{reg: scratch0}
		if m.dst.reg >= fpBase {
			tmp = mloc{reg: fscratch}
		}
		s.move(b, tmp, msrc{kind: mLoc, l: m.dst}, fr)
		for j := range pending {
			if sameLoc(pending[j].src.l, m.dst) {
				pending[j].src.l = tmp
			}
		}
	}
	for _, m := range consts {
		s.move(b, m.dst, m.src, fr)
	}
}

func memRef(l mloc) string {
	if l.base == "FP" {
		return fmt.Sprintf("@%s+%d(FP)", l.name, l.off)
	}
	return fmt.Sprintf("[%s, #%d]", l.base, l.off)
}

func (s *fstate) move(b *strings.Builder, dst mloc, src msrc, fr *frameInfo) {
	g := s.g
	// a source that is not a location goes through a register first
	put := func(r int) {
		switch src.kind {
		case mZero:
			if r >= fpBase {
				fmt.Fprintf(b, "\tfmov %s, xzr\n", regName(r, false))
			} else {
				fmt.Fprintf(b, "\tmov %s, xzr\n", regName(r, false))
			}
		case mImm:
			if r >= fpBase {
				b.WriteString(immText(src.imm, "x17"))
				fmt.Fprintf(b, "\tfmov %s, x17\n", regName(r, false))
				return
			}
			b.WriteString(immText(src.imm, regName(r, false)))
		case mSym:
			b.WriteString(g.addr(regName(r, false), src.sym))
		case mFrame:
			fmt.Fprintf(b, "\tadd %s, sp, #%d\n", regName(r, false), fr.outOff)
		}
	}
	if g.goMode && (src.kind == mSym || src.kind == mFrame) {
		fail("a move of a symbol or frame address")
	}
	if src.kind != mLoc {
		if dst.reg >= 0 {
			put(dst.reg)
			return
		}
		if dst.size == 1 {
			if src.kind == mZero {
				fmt.Fprintf(b, "\tstrb wzr, %s\n", memRef(dst))
				return
			}
			put(scratch1)
			fmt.Fprintf(b, "\tstrb w17, %s\n", memRef(dst))
			return
		}
		if src.kind == mZero {
			fmt.Fprintf(b, "\tstr xzr, %s\n", memRef(dst))
			return
		}
		put(scratch1)
		fmt.Fprintf(b, "\tstr x17, %s\n", memRef(dst))
		return
	}
	sl := src.l
	if sl.size == 1 || dst.size == 1 {
		// a Bool argument or result of Go mode: one byte in memory
		switch {
		case dst.reg >= 0 && sl.reg < 0 && dst.reg < fpBase:
			fmt.Fprintf(b, "\tldrb %s, %s\n", regName(dst.reg, true), memRef(sl))
		case sl.reg >= 0 && dst.reg < 0 && sl.reg < fpBase:
			fmt.Fprintf(b, "\tstrb %s, %s\n", regName(sl.reg, true), memRef(dst))
		case sl.reg < 0 && dst.reg < 0:
			fmt.Fprintf(b, "\tldrb w17, %s\n\tstrb w17, %s\n", memRef(sl), memRef(dst))
		default:
			fail("a byte move between registers of different classes")
		}
		return
	}
	switch {
	case dst.reg >= 0 && sl.reg >= 0:
		dfp, sfp := dst.reg >= fpBase, sl.reg >= fpBase
		switch {
		case dfp && sfp:
			fmt.Fprintf(b, "\tfmov %s, %s\n", regName(dst.reg, false), regName(sl.reg, false))
		case !dfp && !sfp:
			fmt.Fprintf(b, "\tmov %s, %s\n", regName(dst.reg, false), regName(sl.reg, false))
		default:
			fmt.Fprintf(b, "\tfmov %s, %s\n", regName(dst.reg, false), regName(sl.reg, false))
		}
	case dst.reg >= 0:
		fmt.Fprintf(b, "\tldr %s, %s\n", regName(dst.reg, false), memRef(sl))
	case sl.reg >= 0:
		fmt.Fprintf(b, "\tstr %s, %s\n", regName(sl.reg, false), memRef(dst))
	default:
		fmt.Fprintf(b, "\tldr x17, %s\n\tstr x17, %s\n", memRef(sl), memRef(dst))
	}
}

// immText is the assembly that puts n in register r.
func immText(n int64, r string) string {
	return strings.ReplaceAll("\t"+strings.ReplaceAll(movImm(n), "\n", "\n\t"), "$d0", r) + "\n"
}

// recoverOperand recomputes, in a fault stub, an operand of a checked
// add, sub, or neg whose register the result took (the operation
// wraps, so undoing it gives the operand back), and points its move at
// the recomputed value (x15).
func (s *fstate) recoverOperand(b *strings.Builder, in *inst, ms []move, fr *frameInfo) {
	if in.recover == "" {
		return
	}
	dl, ok := s.vloc(in.defs[0], fr)
	if !ok || dl.reg < 0 {
		return
	}
	d := regName(dl.reg, false)
	args := in.fault.args
	clobbered := func(k int) bool {
		if k >= len(args) || args[k].kind != sReg || args[k].v == 0 {
			return false
		}
		l, _ := s.vloc(args[k].v, fr)
		return l.reg == dl.reg
	}
	// other puts operand k in a register
	other := func(k int) string {
		a := args[k]
		if a.kind == sImm {
			b.WriteString(immText(a.imm, "x17"))
			return "x17"
		}
		l, _ := s.vloc(a.v, fr)
		if l.reg >= 0 {
			return regName(l.reg, false)
		}
		fmt.Fprintf(b, "\tldr x17, %s\n", memRef(l))
		return "x17"
	}
	for _, k := range []int{0, 2} {
		if !clobbered(k) {
			continue
		}
		switch {
		case in.recover == "neg":
			fmt.Fprintf(b, "\tneg x15, %s\n", d)
		case in.recover == "add":
			fmt.Fprintf(b, "\tsub x15, %s, %s\n", d, other(2-k))
		case in.recover == "sub" && k == 0:
			fmt.Fprintf(b, "\tadd x15, %s, %s\n", d, other(2))
		case in.recover == "sub":
			fmt.Fprintf(b, "\tsub x15, %s, %s\n", other(0), d)
		}
		ms[k].src = msrc{kind: mLoc, l: mloc{reg: scratch2}}
	}
}

// padOf is the landing pad block of in, past the blocks that only jump
// (thread() may have dropped them).
func (s *fstate) padOf(in *inst) *mblock {
	b := in.pad
	for i := 0; i < 16; i++ {
		if len(b.insts) != 1 || b.insts[0].kind != iJump || b.insts[0].targets[0] == b || len(b.insts[0].targets) > 1 {
			return b
		}
		b = b.insts[0].targets[0]
	}
	return b
}

// callSiteLabels returns the labels around the `bl` of a call that
// unwinds to a landing pad (decision 0085).
func (s *fstate) callSiteLabels() (string, string) {
	s.g.ncs++
	n := s.g.ncs
	return s.g.local(fmt.Sprintf("f%d_cs%da", s.g.nfunc, n)), s.g.local(fmt.Sprintf("f%d_cs%db", s.g.nfunc, n))
}

// cfiStart opens the unwind information of the function (decision 0085):
// its call frame information, and with landing pads the personality
// routine and the call site table (lsda).
func (s *fstate) cfiStart(b *strings.Builder, pads bool) {
	g := s.g
	fmt.Fprintf(b, "%s:\n", s.funcBegin())
	b.WriteString("\t.cfi_startproc\n")
	if !pads {
		return
	}
	if g.os == Darwin {
		fmt.Fprintf(b, "\t.cfi_personality 155, %s\n\t.cfi_lsda 16, %s\n", g.sym("xo_personality"), s.lsdaLabel())
	} else {
		g.personalityRef = true
		fmt.Fprintf(b, "\t.cfi_personality 155, DW.ref.xo_personality\n\t.cfi_lsda 27, %s\n", s.lsdaLabel())
	}
}

func (s *fstate) funcBegin() string { return s.g.local(fmt.Sprintf("f%d_begin", s.g.nfunc)) }

// lsdaLabel names the function's call site table. On Mach-O it is a
// symbol, not an assembler local: the linker splits sections at symbols
// and finds each table's start that way (a local label past the first
// table pointed into code, decision 0085), as clang names them.
func (s *fstate) lsdaLabel() string {
	if s.g.os == Darwin {
		return fmt.Sprintf("GCC_except_table_xo%d", s.g.nfunc)
	}
	return s.g.local(fmt.Sprintf("f%d_lsda", s.g.nfunc))
}

// cfiFrame describes the frame the prologue made: the canonical frame
// address from x29, and where the frame record and the callee saved
// registers are.
func (s *fstate) cfiFrame(b *strings.Builder, fr *frameInfo) {
	if fr.leaf {
		return
	}
	fmt.Fprintf(b, "\t.cfi_def_cfa w29, %d\n", fr.size-fr.rec)
	if fr.size-fr.rec == 16 {
		fmt.Fprintf(b, "\t.cfi_offset w30, %d\n\t.cfi_offset w29, %d\n", fr.rec+8-fr.size, fr.rec-fr.size)
	} else {
		// the frame record is not at the top of the frame: the Darwin
		// assembler's compact unwind encoding (frame mode) ignores the
		// CFA offset and would assume 16; with the frame pointer's rule
		// first it keeps the DWARF rules (gotchas/arm64-compact-unwind-cfa.md)
		fmt.Fprintf(b, "\t.cfi_offset w29, %d\n\t.cfi_offset w30, %d\n", fr.rec-fr.size, fr.rec+8-fr.size)
	}
	at := func(r, off int) {
		name := regName(r, false)
		if r < fpBase {
			name = "w" + name[1:]
		}
		fmt.Fprintf(b, "\t.cfi_offset %s, %d\n", name, off-fr.size)
	}
	// as saves() stores them: a pair puts the higher register first
	for k := 0; k < len(s.saved); k++ {
		r, off := s.saved[k], fr.saveBase+8*k
		if k+1 < len(s.saved) && (s.saved[k+1] >= fpBase) == (r >= fpBase) && off <= 496 {
			at(s.saved[k+1], off)
			at(r, off+8)
			k++
			continue
		}
		at(r, off)
	}
}

// lsda emits the call site table of the function: GCC's format for
// cleanups, offsets from the function's start (decision 0085).
func (s *fstate) lsda(sites []callSite, fr *frameInfo) {
	g := s.g
	b := &g.lsdas
	if g.os == Darwin {
		b.WriteString("\t.section __TEXT,__gcc_except_tab\n")
	} else {
		b.WriteString("\t.section .gcc_except_table,\"a\",@progbits\n")
	}
	fb := s.funcBegin()
	beg, end := s.g.local(fmt.Sprintf("f%d_cst", g.nfunc)), s.g.local(fmt.Sprintf("f%d_cste", g.nfunc))
	fmt.Fprintf(b, "\t.p2align 2\n%s:\n\t.byte 255\n\t.byte 255\n\t.byte 1\n\t.uleb128 %s-%s\n%s:\n", s.lsdaLabel(), end, beg, beg)
	for _, c := range sites {
		pad := s.label(s.fwd(c.pad, fr))
		fmt.Fprintf(b, "\t.uleb128 %s-%s\n\t.uleb128 %s-%s\n\t.uleb128 %s-%s\n\t.byte 0\n", c.from, fb, c.to, c.from, pad, fb)
	}
	fmt.Fprintf(b, "%s:\n", end)
}

// fwd follows blocks that do nothing but jump (their parallel moves are
// all between the same locations after allocation).
func (s *fstate) fwd(b *mblock, fr *frameInfo) *mblock {
	for i := 0; i < 8; i++ {
		t := b.term()
		if t == nil || t.kind != iJump || t.targets[0] == b || len(t.targets) > 1 {
			return b
		}
		for _, in := range b.insts[:len(b.insts)-1] {
			if in.kind != iPmove {
				return b
			}
			for k, d := range in.defs {
				dl, ok := s.vloc(d, fr)
				src := s.msrcOf(in.srcs[k], fr)
				if ok && !(src.kind == mLoc && sameLoc(dl, src.l)) {
					return b
				}
			}
		}
		b = t.targets[0]
	}
	return b
}
