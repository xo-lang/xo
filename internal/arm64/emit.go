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

// layout of the frame (sp relative, after the prologue):
//
//	[0, outArgs)             stack arguments of calls
//	[outOff, outOff+8)       the $OUT slot (length out parameter)
//	[spillBase, ...)         spill slots, 8 bytes each
//	[saveBase, ...)          callee saved registers
//	frame                    (16 byte aligned), then the saved x29, x30
type frameInfo struct {
	outArgs, outOff, spillBase, saveBase, size int
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
	fr := &frameInfo{}
	for _, b := range s.order {
		for _, in := range b.insts {
			if in.kind == iCall {
				n := 0
				for _, l := range s.abiLocs(in.srcs) {
					if l.reg < 0 {
						n = l.slot + 1
					}
				}
				if 8*n > fr.outArgs {
					fr.outArgs = 8 * n
				}
			}
		}
	}
	fr.outOff = fr.outArgs
	fr.spillBase = fr.outOff
	if s.out {
		fr.spillBase += 8
	}
	fr.saveBase = fr.spillBase + 8*s.slots
	fr.size = align16(fr.saveBase + 8*len(s.saved))
	if fr.size+16 > 32000 {
		fail("frame of %s too large (%d bytes)", s.f.Name, fr.size)
	}
	b := &g.text
	fmt.Fprintf(b, "// func %s\n", s.f.Name)
	g.funcHeader(b, g.fname(s.f.Name), false)
	b.WriteString("\tstp x29, x30, [sp, #-16]!\n\tmov x29, sp\n")
	if fr.size > 0 {
		subSP(b, "sub", fr.size)
	}
	for k, r := range s.saved {
		fmt.Fprintf(b, "\tstr %s, [sp, #%d]\n", regName(r, false), fr.saveBase+8*k)
	}
	for bi, mb := range s.order {
		var next *mblock
		if bi+1 < len(s.order) {
			next = s.order[bi+1]
		}
		fmt.Fprintf(b, "%s:\n", s.label(mb))
		for _, in := range mb.insts {
			s.emitInst(b, in, fr, next)
		}
	}
	b.WriteString(s.stubs.String())
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

func (s *fstate) epilogue(b *strings.Builder, fr *frameInfo) {
	for k, r := range s.saved {
		fmt.Fprintf(b, "\tldr %s, [sp, #%d]\n", regName(r, false), fr.saveBase+8*k)
	}
	b.WriteString("\tmov sp, x29\n\tldp x29, x30, [sp], #16\n\tret\n")
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
		s.parallel(b, s.callMoves(in.srcs, fr, "sp", 0), fr)
		fmt.Fprintf(b, "\tbl %s\n", in.sym)
		if in.noret {
			return
		}
		var ms []move
		for k, l := range s.abiLocs(regs(in.defs...)) {
			if l.reg < 0 {
				fail("a call of %s returns more than 8 words of a class", in.sym)
			}
			if d, ok := s.vloc(in.defs[k], fr); ok {
				ms = append(ms, move{d, msrc{kind: mLoc, l: mloc{reg: l.reg}}})
			}
		}
		s.parallel(b, ms, fr)
	case iEntry:
		var ms []move
		for k, l := range s.abiLocs(regs(in.defs...)) {
			src := mloc{reg: l.reg}
			if l.reg < 0 {
				src = mloc{reg: -1, base: "x29", off: 16 + 8*l.slot}
			}
			if d, ok := s.vloc(in.defs[k], fr); ok {
				ms = append(ms, move{d, msrc{kind: mLoc, l: src}})
			}
		}
		s.parallel(b, ms, fr)
	case iRet:
		var ms []move
		for k, l := range s.abiLocs(in.srcs) {
			if l.reg < 0 {
				fail("%s returns more than 8 words of a class", s.f.Name)
			}
			ms = append(ms, move{mloc{reg: l.reg}, s.msrcOf(in.srcs[k], fr)})
		}
		s.parallel(b, ms, fr)
		s.epilogue(b, fr)
	case iJump:
		if in.targets[0] != next {
			fmt.Fprintf(b, "\tb %s\n", s.label(in.targets[0]))
		}
	case iCondBr:
		t, f := in.targets[0], in.targets[1]
		if t == f {
			if t != next {
				fmt.Fprintf(b, "\tb %s\n", s.label(t))
			}
			return
		}
		if in.cond == "cbnz" {
			r := s.useReg(b, in.uses[0], 0, fr)
			if t == next {
				fmt.Fprintf(b, "\tcbz %s, %s\n", r, s.label(f))
				return
			}
			fmt.Fprintf(b, "\tcbnz %s, %s\n", r, s.label(t))
		} else {
			s.emitPlain(b, &inst{text: in.text, uses: in.uses}, fr)
			if t == next {
				fmt.Fprintf(b, "\tb.%s %s\n", invCond[in.cond], s.label(f))
				return
			}
			fmt.Fprintf(b, "\tb.%s %s\n", in.cond, s.label(t))
		}
		if f != next {
			fmt.Fprintf(b, "\tb %s\n", s.label(f))
		}
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
	if in.fault != nil {
		s.nstub++
		l := s.g.local(fmt.Sprintf("f%d_fault%d", s.g.nfunc, s.nstub))
		fmt.Fprintf(b, "\tb.%s %s\n", in.fault.cond, l)
		fmt.Fprintf(&s.stubs, "%s:\n", l)
		s.parallel(&s.stubs, s.callMoves(in.fault.args, fr, "sp", 0), fr)
		fmt.Fprintf(&s.stubs, "\tbl %s\n\tbrk #1\n", in.fault.fn)
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

func memRef(l mloc) string { return fmt.Sprintf("[%s, #%d]", l.base, l.off) }

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
	if src.kind != mLoc {
		if dst.reg >= 0 {
			put(dst.reg)
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
