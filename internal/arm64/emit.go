package arm64

import (
	"fmt"
	"regexp"
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
	mMem // the address of byte imm of the frame's memory
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
		if w {
			// the single precision name (`$wd0` of a Float, decision 0195)
			return "s" + strconv.Itoa(r-fpBase)
		}
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
//	[memOff, ...)            memory of by reference arguments (byref.go)
//	[rec, rec+16)            the frame record (x29, x30); x29 = sp + rec
//
// (a far frame puts the memory after its spill slots).
// The frame record is at the bottom (rec 0) when no call passes stack
// arguments, so one `stp x29, x30, [sp, #-size]!` makes the frame. A
// leaf function (no calls, nothing saved or spilled) has no frame:
// x29 keeps pointing at its caller's record. Incoming stack arguments
// are at [sp, #size + 8k].
type frameInfo struct {
	outArgs, outOff, spillBase, saveBase, memOff, rec, size int
	leaf                                                    bool
}

// outArgBytes is the size of the stack argument area of the calls: the
// stack arguments, and the result words past the registers, which the
// callee stores in the same area.
func (s *fstate) outArgBytes() int {
	out := 0
	for _, b := range s.order {
		for _, in := range b.insts {
			if in.kind == iCall {
				n := 0
				for _, l := range append(s.abiLocs(in.srcs), s.abiLocs(regs(in.defs...))...) {
					if l.reg < 0 {
						n = max(n, l.slot+1)
					}
				}
				out = max(out, 8*n)
			}
		}
	}
	return out
}

// frameBound is an upper bound of the frame emit lays out with saved
// callee saved registers.
func (s *fstate) frameBound(saved int) int {
	return s.outArgBytes() + 16 + 8 + 8*s.slots + 8*saved + s.memBytes + 16
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
	fr := &frameInfo{outArgs: s.outArgBytes()}
	calls := false
	for _, b := range s.order {
		for _, in := range b.insts {
			calls = calls || in.kind == iCall
		}
	}
	fr.leaf = !calls && len(s.saved) == 0 && s.slots == 0 && !s.out && s.memBytes == 0
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
	fr.memOff = fr.saveBase + 8*len(s.saved)
	if fr.outArgs == 0 {
		fr.size = align16(fr.memOff + s.memBytes)
	} else {
		fr.rec = align16(fr.memOff + s.memBytes)
		fr.size = fr.rec + 16
	}
	if s.base != 0 {
		// a far frame: the saved registers and the frame record below
		// the spill slots, within reach of sp; the slots past it are
		// addressed through x28 (farSlots)
		fr.outOff = fr.outArgs
		fr.saveBase = fr.outOff
		if s.out {
			fr.saveBase += 8
		}
		fr.rec = align16(fr.saveBase + 8*len(s.saved))
		fr.spillBase = fr.rec + 16
		fr.memOff = fr.spillBase + 8*s.slots
		fr.size = align16(fr.memOff + s.memBytes)
	}
	if fr.leaf {
		fr.size = 0
	}
	if fr.size > maxFrame && s.base == 0 || fr.size >= maxFarFrame || fr.rec+16 > maxFrame {
		fail("frame of %s too large (%d bytes)", s.f.Name, fr.size)
	}
	b := &strings.Builder{}
	lsdaAt := g.lsdas.Len()
	defer func() {
		g.text.WriteString(s.farTests(s.farSlots(pairMem(b.String(), g.lsdas.String()[lsdaAt:]))))
	}()
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

// farTestLimit is the number of instructions past which a function's
// tbz and tbnz may not reach their targets (14 bit offsets: 32 KiB, 8192
// instructions), with a margin for the templates' estimate.
const farTestLimit = 7000

// farCondLimit is the number of instructions past which a function's
// b.cond, cbz, and cbnz may not reach their targets (19 bit offsets: 1
// MiB, 262144 instructions), with the same kind of margin.
const farCondLimit = 240000

// farTests rewrites every tbz and tbnz of a function longer than
// farTestLimit instructions, and every b.cond, cbz, and cbnz of one
// longer than farCondLimit, as the opposite test around an unconditional
// branch, which reaches 128 MiB: `tbnz x9, #63, L` becomes `tbz x9, #63,
// Lf3_far1; b L; Lf3_far1:`. Huge functions (a main with thousands of
// list elements, the self hosted SSA builder's std_call) otherwise fail
// to assemble with "fixup value out of range"
// (gotchas/arm64-tbz-range.md).
func (s *fstate) farTests(text string) string {
	return farBranches(text, farTestLimit, farCondLimit, func(k int) string {
		return s.g.local(fmt.Sprintf("f%d_far%d", s.g.nfunc, k))
	})
}

// farBranches is farTests with its limits and labels given.
func farBranches(text string, testLimit, condLimit int, label func(k int) string) string {
	lines := strings.Split(text, "\n")
	n := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "\t") && !strings.HasPrefix(l, "\t.") {
			n++
		}
	}
	if n < testLimit {
		return text
	}
	var b strings.Builder
	k := 0
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if !strings.HasPrefix(l, "\t") {
			b.WriteString(l)
			continue
		}
		f := strings.Fields(l)
		switch {
		case len(f) == 4 && (f[0] == "tbz" || f[0] == "tbnz"):
			k++
			skip := label(k)
			op := map[string]string{"tbz": "tbnz", "tbnz": "tbz"}[f[0]]
			fmt.Fprintf(&b, "\t%s %s %s %s\n\tb %s\n%s:", op, f[1], f[2], skip, f[3], skip)
			continue
		case n >= condLimit && len(f) == 3 && (f[0] == "cbz" || f[0] == "cbnz"):
			k++
			skip := label(k)
			op := map[string]string{"cbz": "cbnz", "cbnz": "cbz"}[f[0]]
			fmt.Fprintf(&b, "\t%s %s %s\n\tb %s\n%s:", op, f[1], skip, f[2], skip)
			continue
		case n >= condLimit && len(f) == 2 && strings.HasPrefix(f[0], "b.") && invCond[f[0][2:]] != "":
			k++
			skip := label(k)
			fmt.Fprintf(&b, "\tb.%s %s\n\tb %s\n%s:", invCond[f[0][2:]], skip, f[1], skip)
			continue
		}
		b.WriteString(l)
	}
	return b.String()
}

// maxFrame is the largest frame whose every spill slot and saved
// register, and 4 KiB of incoming stack arguments past them, is within
// reach of a scaled 12 bit `ldr`/`str` offset from sp (32760 bytes).
// A larger frame keeps x28 free (regalloc) and addresses what is past
// that reach through it (farSlots). Frames past one page are probed
// (probeSP).
const maxFrame = 28672

// farBase is the register of the addresses of a far frame's slots.
const farBase = 28

// maxFarFrame bounds a frame by what `add x28, sp, #hi, lsl #12` and
// `add x28, x28, #lo` reach (16 MiB); the default stack is 8 MiB.
const maxFarFrame = 1 << 24

// farSlots rewrites the sp relative loads and stores of a far frame
// whose offset is past the instruction's reach (spill slots, incoming
// stack arguments) to go through x28: `ldr x9, [sp, #40968]` is `add
// x28, sp, #10, lsl #12` and `ldr x9, [x28, #8]`. x28 holds nothing
// else in such a function (regalloc keeps it free and the prologue
// saves it); its own save is within reach.
func (s *fstate) farSlots(text string) string {
	if s.base == 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		m := spRefRE.FindStringSubmatch(l)
		if m == nil {
			b.WriteString(l)
			continue
		}
		off, _ := strconv.Atoi(m[3])
		scale, pair := memScale(m[1], m[2])
		reach := 4095 * scale
		if pair {
			reach = 63 * scale
		}
		if off <= reach && off%scale == 0 {
			b.WriteString(l)
			continue
		}
		hi, lo := off&^4095, off&4095
		fmt.Fprintf(&b, "\tadd x28, sp, #%d, lsl #12\n", hi>>12)
		if lo > reach || lo%scale != 0 {
			fmt.Fprintf(&b, "\tadd x28, x28, #%d\n", lo)
			lo = 0
		}
		fmt.Fprintf(&b, "\t%s %s, [x28, #%d]", m[1], m[2], lo)
	}
	return b.String()
}

var spRefRE = regexp.MustCompile(`^\t(ldr|str|ldp|stp|ldrb|strb|ldrh|strh|ldrsw|ldrsb|ldrsh) ([^\[]+), \[sp, #([0-9]+)\]$`)

// memScale returns the access size of a load or store of regs (the
// scale of its offset), and whether it is a pair.
func memScale(op, regs string) (int, bool) {
	switch op {
	case "ldrb", "strb", "ldrsb":
		return 1, false
	case "ldrh", "strh", "ldrsh":
		return 2, false
	case "ldrsw":
		return 4, false
	}
	size := 8
	switch regs[0] {
	case 'w', 's':
		size = 4
	case 'q':
		size = 16
	case 'h':
		size = 2
	case 'b':
		size = 1
	}
	return size, op == "ldp" || op == "stp"
}

// probePage is the stride of the stack probes: the smallest guard page
// (4 KiB on Linux; a fiber stack has one guard page, std/llrt xo_rt.c).
const probePage = 4096

// probeUnroll is the most pages probeSP probes without a loop.
const probeUnroll = 8

// probeSP lowers sp by n, touching every page on the way when n is more
// than one page, so a large frame cannot step over the guard page below
// a fiber stack into another allocation (gotchas/arm64-large-frame.md).
// Past probeUnroll pages the probes are a loop counting in x16 (free in
// a prologue), with loop the label of its head.
func probeSP(b *strings.Builder, n int, loop string) {
	if pages := (n - 1) / probePage; pages > probeUnroll {
		fmt.Fprintf(b, "\tmov x16, #%d\n%s:\n\tsub sp, sp, #%d, lsl #12\n\tstr xzr, [sp]\n\tsubs x16, x16, #1\n\tb.ne %s\n", pages, loop, probePage>>12, loop)
		n -= pages * probePage
	}
	for n > probePage {
		fmt.Fprintf(b, "\tsub sp, sp, #%d, lsl #12\n\tstr xzr, [sp]\n", probePage>>12)
		n -= probePage
	}
	subSP(b, "sub", n)
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
		probeSP(b, fr.size, s.g.local(fmt.Sprintf("f%d_probe", s.g.nfunc)))
		if fr.rec <= 504 {
			fmt.Fprintf(b, "\tstp x29, x30, [sp, #%d]\n", fr.rec)
		} else {
			fmt.Fprintf(b, "\tstr x29, [sp, #%d]\n\tstr x30, [sp, #%d]\n", fr.rec, fr.rec+8)
		}
		if fr.rec >= 4096 {
			// add takes a 12 bit immediate, optionally shifted by 12
			fmt.Fprintf(b, "\tadd x29, sp, #%d, lsl #12\n", fr.rec>>12)
			if lo := fr.rec & 4095; lo > 0 {
				fmt.Fprintf(b, "\tadd x29, x29, #%d\n", lo)
			}
		} else {
			fmt.Fprintf(b, "\tadd x29, sp, #%d\n", fr.rec)
		}
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
	case sMem:
		return msrc{kind: mMem, imm: a.imm}
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
		} else if in.svc {
			b.WriteString("\tmov x8, x6\n\tsvc #0\n")
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
			// both ways go to one block, but the compare stays: the
			// block may test these flags without a compare of its own
			// (reuseFlags)
			if in.text != "" && in.cond != "cbnz" && in.cond != "tbz" && in.cond != "tbnz" {
				s.emitPlain(b, &inst{text: in.text, uses: in.uses}, fr)
			}
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
		if in.early {
			// the definition is written while the uses are read: a scratch
			// register no reloaded use took (decision 0200's cas templates)
			cls, pool := 0, gpScratch
			if s.fp[d] {
				cls, pool = 1, fpScratch
			}
			if nscratch[cls] < len(pool) {
				r = pool[nscratch[cls]]
				nscratch[cls]++
			} else if in.keep {
				panic("arm64: no scratch register for the spilled result of `" + t + "`")
			}
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

// addSP materializes sp + off in reg. add's immediate has 12 bits
// (optionally shifted by 12), so an offset past 4095 takes two adds: the
// $OUT slot of a frame whose calls pass more than 4 KiB of stack
// arguments (a large struct by value) is that far (gotchas/arm64-add-
// immediate-range.md).
func addSP(b *strings.Builder, reg string, off int) {
	if off <= 4095 {
		fmt.Fprintf(b, "\tadd %s, sp, #%d\n", reg, off)
		return
	}
	fmt.Fprintf(b, "\tadd %s, sp, #%d, lsl #12\n", reg, off>>12)
	if lo := off & 4095; lo != 0 {
		fmt.Fprintf(b, "\tadd %s, %s, #%d\n", reg, reg, lo)
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
			addSP(b, regName(r, false), fr.outOff)
		case mMem:
			addSP(b, regName(r, false), fr.memOff+int(src.imm))
		}
	}
	if g.goMode && (src.kind == mSym || src.kind == mFrame || src.kind == mMem) {
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
