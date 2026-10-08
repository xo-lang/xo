package arm64

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ssa"
)

// Closures (decision 0048): a function value is two integer words, the
// code pointer and the environment. The environment is null when the
// closure captures nothing, else a counted cell
//
//	[0]  rc
//	[8]  drop: the function that releases the captures and frees the cell
//	[16] the captured values' words, 8 bytes each, in Func.Captures order
//
// The code is an Indirect function whose parameter 0 is the closure
// itself, so a call through a value passes the code word in x0, the
// environment in x1, the arguments after them (AAPCS64 like every
// call), and branches with `blr x0`.
//
// Releasing a closure whose count reaches one hands the environment to
// xo_rc_defer with its drop function: a chain of closures capturing
// closures is freed with the runtime's worklist, not the C stack.

const envHdr = 16 // {rc, drop}

// captureWords returns the first word (from the start of the captures)
// and the word count of capture i of f.
func (g *gen) captureWords(f *ssa.Func, i int) (int, int) {
	off := 0
	for k := 0; k < i; k++ {
		off += g.nwords(f.Captures[k])
	}
	return off, g.nwords(f.Captures[i])
}

// closureOp lowers OpClosure: the code address, and a new environment
// holding the captured values (consumed) when there are any.
func (s *fstate) closureOp(v *ssa.Value) {
	g := s.g
	name := v.Aux.(string)
	callee := g.byName[name]
	if callee == nil {
		fail("closure of unknown function `%s`", name)
	}
	code := s.symAddr(g.fname(name))
	if len(v.Args) == 0 {
		s.set(v, code, 0)
		return
	}
	n := 0
	for _, c := range callee.Captures {
		n += g.nwords(c)
	}
	if envHdr+8*n > 32760 {
		fail("closure of `%s` captures too many words", name)
	}
	env := s.rt("xo_rc_alloc", []src{isrc(int64(envHdr + 8*n))}, false)[0]
	s.store("str $u0, [$u1]", s.cimm(1), env)
	s.store("str $u0, [$u1, #8]", s.symAddr(g.envDrop(callee)), env)
	k := 0
	for _, x := range v.Args {
		for _, w := range s.storeWords(x) {
			s.store(fmt.Sprintf("str $u0, [$u1, #%d]", envHdr+8*k), w, env)
			k++
		}
	}
	s.set(v, code, env)
}

// captureOp lowers OpCapture: the words of a captured value, loaded from
// the environment of the closure parameter (a projection: no counting).
func (s *fstate) captureOp(v *ssa.Value) {
	g := s.g
	env := s.real(s.words(v.Args[0])[1])
	off, _ := g.captureWords(s.f, v.Aux.(int))
	var ws []vreg
	for k, fp := range g.layout(v.Type) {
		ws = append(ws, s.def1(fp, fmt.Sprintf("ldr $d0, [$u0, #%d]", envHdr+8*(off+k)), env))
	}
	s.set(v, ws...)
}

// callValueOp lowers OpCallValue: the closure's words (code in x0,
// environment in x1) and the arguments, then `blr x0`.
func (s *fstate) callValueOp(v *ssa.Value) {
	var args []src
	for _, x := range v.Args {
		args = append(args, regs(s.words(x)...)...)
	}
	defs := s.newVregs(s.g.layout(v.Type))
	in := &inst{kind: iCall, srcs: args, defs: defs, indirect: true}
	for _, a := range args {
		if a.v != 0 {
			in.uses = append(in.uses, a.v)
		}
	}
	s.add(in)
	s.clobber()
	s.set(v, defs...)
}

// funcRC writes the body of the dup or drop helper of a function value
// (x0 the code, x1 the environment).
func (g *gen) funcRC(b *strings.Builder, kind, sym string) {
	fmt.Fprintf(b, "\tcbz x1, %s_ret\n\tldr x9, [x1]\n", sym)
	if kind == "dup" {
		fmt.Fprintf(b, "\tadd x9, x9, #1\n\tstr x9, [x1]\n%s_ret:\n\tret\n", sym)
		return
	}
	fmt.Fprintf(b, "\tcmp x9, #1\n\tb.eq %s_free\n\tsub x9, x9, #1\n\tstr x9, [x1]\n%s_ret:\n\tret\n", sym, sym)
	fmt.Fprintf(b, "%s_free:\n\tmov x0, x1\n\tldr x1, [x0, #8]\n\tb %s\n", sym, g.sym("xo_rc_defer"))
}

// envDrop returns the drop function of the environments of f: it
// releases the captures and frees the cell (x0).
func (g *gen) envDrop(f *ssa.Func) string {
	key := "envdrop " + f.Name
	if s, ok := g.helped[key]; ok {
		return s
	}
	sym := g.helperLabel("envdrop")
	g.helped[key] = sym
	var b strings.Builder
	fmt.Fprintf(&b, "// envdrop %s\n\t.p2align 2\n%s:\n", f.Name, sym)
	var ls []leaf
	for i, c := range f.Captures {
		off, _ := g.captureWords(f, i)
		g.heapLeaves(c, off, &ls)
	}
	g.dropLeavesAndFree(&b, ls, envHdr)
	g.helpers.WriteString(b.String())
	return sym
}

// dropLeavesAndFree writes the body of a function that releases the
// leaves ls of the words at [x0, #hdr] and frees the cell x0.
func (g *gen) dropLeavesAndFree(b *strings.Builder, ls []leaf, hdr int) {
	if len(ls) == 0 {
		fmt.Fprintf(b, "\tb %s\n\n", g.sym("xo_rc_free"))
		return
	}
	out := 0
	for _, l := range ls {
		out = max(out, align16(8*(g.leafWords(l.t)-8)))
	}
	frame := out + 32
	fmt.Fprintf(b, "\tsub sp, sp, #%d\n\tstp x29, x30, [sp, #%d]\n\tadd x29, sp, #%d\n\tstr x19, [sp, #%d]\n\tmov x19, x0\n", frame, out, out, out+16)
	for _, l := range ls {
		g.loadCall(b, g.rcHelper("drop", l.t), "x19", hdr+8*l.off, g.leafWords(l.t))
	}
	fmt.Fprintf(b, "\tmov x0, x19\n\tldr x19, [sp, #%d]\n\tldp x29, x30, [sp, #%d]\n\tadd sp, sp, #%d\n\tb %s\n\n", out+16, out, frame, g.sym("xo_rc_free"))
}

// loadCall calls sym with the n words at [base, #off] as integer
// arguments: words past x7 go to the stack arguments at [sp, #8k].
func (g *gen) loadCall(b *strings.Builder, sym, base string, off, n int) {
	for k := 8; k < n; k++ {
		fmt.Fprintf(b, "\tldr x9, [%s, #%d]\n\tstr x9, [sp, #%d]\n", base, off+8*k, 8*(k-8))
	}
	for k := 0; k < n && k < 8; k++ {
		fmt.Fprintf(b, "\tldr x%d, [%s, #%d]\n", k, base, off+8*k)
	}
	fmt.Fprintf(b, "\tbl %s\n", sym)
}
