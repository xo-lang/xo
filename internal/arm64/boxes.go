package arm64

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ssa"
)

// Boxes (decision 0048): a Box[T], the field of a recursive type, is one
// integer word, a pointer to an immutable counted cell
//
//	[0] rc
//	[8] the words of the T value, 8 bytes each
//
// `box` copies a value (consumed) into a new cell; `unbox` loads its
// words (a projection: no counting). A box whose count reaches one is
// released through xo_rc_defer with a drop function that releases the
// contents and frees the cell, so a long chain of boxes is freed with
// the runtime's worklist, not the C stack. Equality compares contents
// (eq.go; equal pointers short cut).

const boxHdr = 8 // {rc}

// boxOp lowers OpBox.
func (s *fstate) boxOp(v *ssa.Value) {
	x := v.Args[0]
	n := s.g.nwords(x.Type)
	if boxHdr+8*n > 32760 {
		fail("box of `%s` (too many words)", x.Type.Name)
	}
	p := s.rt("xo_rc_alloc", []src{isrc(int64(boxHdr + 8*n))}, false)[0]
	s.store("str $u0, [$u1]", s.cimm(1), p)
	for k, w := range s.storeWords(x) {
		s.store(fmt.Sprintf("str $u0, [$u1, #%d]", boxHdr+8*k), w, p)
	}
	s.set(v, p)
}

// unboxOp lowers OpUnbox.
func (s *fstate) unboxOp(v *ssa.Value) {
	p := s.word(v.Args[0])
	var ws []vreg
	for k, fp := range s.g.layout(v.Type) {
		ws = append(ws, s.def1(fp, fmt.Sprintf("ldr $d0, [$u0, #%d]", boxHdr+8*k), p))
	}
	s.set(v, ws...)
}

// boxRC writes the body of the dup or drop helper of a box (x0).
func (g *gen) boxRC(b *strings.Builder, kind, sym string, t *ssa.Type) {
	b.WriteString("\tldr x9, [x0]\n")
	if kind == "dup" {
		b.WriteString("\tadd x9, x9, #1\n\tstr x9, [x0]\n\tret\n")
		return
	}
	fmt.Fprintf(b, "\tcmp x9, #1\n\tb.eq %s_free\n\tsub x9, x9, #1\n\tstr x9, [x0]\n\tret\n", sym)
	fmt.Fprintf(b, "%s_free:\n", sym)
	b.WriteString(g.addr("x1", g.boxDrop(t)))
	fmt.Fprintf(b, "\tb %s\n", g.sym("xo_rc_defer"))
}

// boxDrop returns the function that releases the contents of a box of
// type t and frees the cell (x0).
func (g *gen) boxDrop(t *ssa.Type) string {
	key := "boxdrop " + t.Name
	if s, ok := g.helped[key]; ok {
		return s
	}
	sym := g.helperLabel("boxdrop")
	g.helped[key] = sym
	var b strings.Builder
	fmt.Fprintf(&b, "// boxdrop %s\n\t.p2align 2\n%s:\n", t.Name, sym)
	var ls []leaf
	g.heapLeaves(t.Elems[0], 0, &ls)
	g.dropLeavesAndFree(&b, ls, boxHdr)
	g.helpers.WriteString(b.String())
	return sym
}

// releaseBox releases the box p of type t inline: its count goes down by
// one, and at one the cell goes to xo_rc_defer with its boxdrop
// function (out of line, cold).
func (s *fstate) releaseBox(t *ssa.Type, p vreg) {
	before := s.m()
	cont := s.decOrDefer(p, func() src { return symsrc(s.g.boxDrop(t)) })
	s.cur = cont
	s.inherit(before)
	s.clobber()
}
