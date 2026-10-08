package arm64

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ssa"
)

// Structural equality: `==` and `!=` on Str, structs, tuples, enums,
// lists, and boxes, as internal/llvm's xo.eq helpers (Float fields
// compare with IEEE equality, so a NaN field makes values unequal; equal
// box pointers short cut, list pointers do not).
//
// A Str compares inline through xo_str_eq. Other types call a generated
// helper that returns 1 or 0 in x0:
//
//   - struct, tuple, enum: the words of both values as integer arguments
//     (floating point words as bits; past x7 on the stack, as any call);
//     the helper stores them in its frame and compares them in memory.
//   - List, Box: the two pointers in x0 and x1.
//
// Inside a helper, x19 and x20 (x23 and x24 for list elements) point at
// the two values' words in memory, 8 bytes per word (a List[Bool]
// element is one byte), and nested fields compare in place: a Str
// through xo_str_eq, a List or Box through its own helper, an enum by
// tag and then the fields of that variant.

// eqAggregate lowers v (a comparison) when its operands are aggregates
// and reports whether it did.
func (s *fstate) eqAggregate(v *ssa.Value) bool {
	g := s.g
	t := v.Args[0].Type
	switch t.Kind {
	case ssa.KStr, ssa.KStruct, ssa.KTuple, ssa.KEnum, ssa.KList, ssa.KBox:
	default:
		return false
	}
	if v.Op != ssa.OpEq && v.Op != ssa.OpNe {
		fail("ordering of `%s`", t.Name)
	}
	var r vreg
	switch t.Kind {
	case ssa.KStr:
		x, y := s.words(v.Args[0]), s.words(v.Args[1])
		xd := s.def1(false, "add $d0, $u0, #16", s.real(x[0]))
		yd := s.def1(false, "add $d0, $u0, #16", s.real(y[0]))
		res := s.rt("xo_str_eq", regs(xd, x[1], yd, y[1]), false)
		r = s.def1(false, "cmp $wu0, #0\ncset $d0, ne", res[0])
	case ssa.KList, ssa.KBox:
		r = s.call(g.eqHelper(t), regs(s.word(v.Args[0]), s.word(v.Args[1])), []bool{false})[0]
	default:
		lay := g.layout(t)
		if len(lay) == 0 {
			r = s.imm(1)
			break
		}
		args := append(s.toInt(s.words(v.Args[0]), lay), s.toInt(s.words(v.Args[1]), lay)...)
		r = s.call(g.eqHelper(t), regs(args...), []bool{false})[0]
	}
	if v.Op == ssa.OpNe {
		r = s.def1(false, "eor $d0, $u0, #1", r)
	}
	s.set(v, r)
	return true
}

// label returns a fresh assembler label for generated helpers.
func (g *gen) helperLabel(hint string) string {
	g.nhelp++
	return g.local(fmt.Sprintf("xo_%s%d", hint, g.nhelp))
}

// eqHelper returns the symbol of the equality helper of t.
func (g *gen) eqHelper(t *ssa.Type) string {
	key := "eq " + t.Name
	if s, ok := g.helped[key]; ok {
		return s
	}
	sym := g.helperLabel("eq")
	g.helped[key] = sym
	yes, no, done := g.helperLabel("eq_yes"), g.helperLabel("eq_no"), g.helperLabel("eq_done")
	var b strings.Builder
	fmt.Fprintf(&b, "// eq %s\n\t.p2align 2\n%s:\n", t.Name, sym)
	frame := 32
	switch t.Kind {
	case ssa.KBox:
		fmt.Fprintf(&b, "\tcmp x0, x1\n\tb.ne %s_cmp\n\tmov x0, #1\n\tret\n%s_cmp:\n", sym, sym)
		b.WriteString("\tstp x29, x30, [sp, #-32]!\n\tmov x29, sp\n\tstp x20, x19, [sp, #16]\n")
		b.WriteString("\tadd x19, x0, #8\n\tadd x20, x1, #8\n")
		g.eqAt(&b, t.Elems[0], "x19", "x20", 0, no, false)
	case ssa.KList:
		frame = 64
		b.WriteString("\tstp x29, x30, [sp, #-64]!\n\tmov x29, sp\n\tstp x20, x19, [sp, #16]\n\tstp x22, x21, [sp, #32]\n\tstp x24, x23, [sp, #48]\n")
		fmt.Fprintf(&b, "\tldr x9, [x0, #8]\n\tldr x10, [x1, #8]\n\tcmp x9, x10\n\tb.ne %s\n", no)
		et := t.Elems[0]
		es := g.esize(et)
		if es == 0 {
			fmt.Fprintf(&b, "\tb %s\n", yes)
			break
		}
		loop := g.helperLabel("eq_loop")
		fmt.Fprintf(&b, "\tmov x21, x9\n\tmov x22, #0\n\tadd x23, x0, #%d\n\tadd x24, x1, #%d\n", listHdr, listHdr)
		fmt.Fprintf(&b, "%s:\n\tcmp x22, x21\n\tb.eq %s\n", loop, yes)
		g.eqAt(&b, et, "x23", "x24", 0, no, et.Kind == ssa.KBool)
		if imm12(es) {
			fmt.Fprintf(&b, "\tadd x23, x23, #%d\n\tadd x24, x24, #%d\n", es, es)
		} else {
			b.WriteString(immText(es, "x9"))
			b.WriteString("\tadd x23, x23, x9\n\tadd x24, x24, x9\n")
		}
		fmt.Fprintf(&b, "\tadd x22, x22, #1\n\tb %s\n", loop)
	case ssa.KStruct, ssa.KTuple, ssa.KEnum:
		n := g.nwords(t)
		frame = align16(32 + 16*n)
		if frame >= 4096 {
			fail("equality of `%s` (too many words)", t.Name)
		}
		fmt.Fprintf(&b, "\tsub sp, sp, #%d\n\tstp x29, x30, [sp]\n\tmov x29, sp\n\tstp x20, x19, [sp, #16]\n", frame)
		for k := 0; k < 2*n; k++ {
			if k < 8 {
				fmt.Fprintf(&b, "\tstr x%d, [sp, #%d]\n", k, 32+8*k)
				continue
			}
			// stack arguments: at the caller's sp, which is sp + frame
			fmt.Fprintf(&b, "\tldr x9, [sp, #%d]\n\tstr x9, [sp, #%d]\n", frame+8*(k-8), 32+8*k)
		}
		fmt.Fprintf(&b, "\tadd x19, sp, #32\n\tadd x20, sp, #%d\n", 32+8*n)
		g.eqAt(&b, t, "x19", "x20", 0, no, false)
	default:
		fail("equality of `%s`", t.Name)
	}
	fmt.Fprintf(&b, "%s:\n\tmov x0, #1\n\tb %s\n%s:\n\tmov x0, #0\n%s:\n", yes, done, no, done)
	if t.Kind == ssa.KList {
		b.WriteString("\tldp x24, x23, [sp, #48]\n\tldp x22, x21, [sp, #32]\n")
	}
	b.WriteString("\tldp x20, x19, [sp, #16]\n")
	if t.Kind == ssa.KStruct || t.Kind == ssa.KTuple || t.Kind == ssa.KEnum {
		fmt.Fprintf(&b, "\tldp x29, x30, [sp]\n\tadd sp, sp, #%d\n\tret\n\n", frame)
	} else {
		fmt.Fprintf(&b, "\tldp x29, x30, [sp], #%d\n\tret\n\n", frame)
	}
	g.helpers.WriteString(b.String())
	return sym
}

// eqAt compares the values of type t at [ra, #off] and [rb, #off] and
// branches to no when they differ. byte: t is a Bool list element (one
// byte). It may call functions: ra and rb must be callee saved.
func (g *gen) eqAt(b *strings.Builder, t *ssa.Type, ra, rb string, off int, no string, byte bool) {
	switch t.Kind {
	case ssa.KUnit, ssa.KCap:
	case ssa.KInt, ssa.KBool:
		if byte {
			fmt.Fprintf(b, "\tldrb w9, [%s, #%d]\n\tldrb w10, [%s, #%d]\n", ra, off, rb, off)
		} else {
			fmt.Fprintf(b, "\tldr x9, [%s, #%d]\n\tldr x10, [%s, #%d]\n", ra, off, rb, off)
		}
		fmt.Fprintf(b, "\tcmp x9, x10\n\tb.ne %s\n", no)
	case ssa.KFloat:
		// unordered (NaN) sets ne: NaN is unequal to everything
		fmt.Fprintf(b, "\tldr d0, [%s, #%d]\n\tldr d1, [%s, #%d]\n\tfcmp d0, d1\n\tb.ne %s\n", ra, off, rb, off, no)
	case ssa.KStr:
		fmt.Fprintf(b, "\tldr x0, [%s, #%d]\n\tadd x0, x0, #16\n\tldr x1, [%s, #%d]\n", ra, off, ra, off+8)
		fmt.Fprintf(b, "\tldr x2, [%s, #%d]\n\tadd x2, x2, #16\n\tldr x3, [%s, #%d]\n", rb, off, rb, off+8)
		fmt.Fprintf(b, "\tbl %s\n\tcbz w0, %s\n", g.sym("xo_str_eq"), no)
	case ssa.KList, ssa.KBox:
		fmt.Fprintf(b, "\tldr x0, [%s, #%d]\n\tldr x1, [%s, #%d]\n\tbl %s\n\tcbz x0, %s\n", ra, off, rb, off, g.eqHelper(t), no)
	case ssa.KStruct, ssa.KTuple:
		n := len(t.Elems)
		if t.Kind == ssa.KStruct {
			n = len(t.Fields)
		}
		for i := 0; i < n; i++ {
			fo, _ := g.fieldWords(t, i)
			ft := t.Elems
			var et *ssa.Type
			if t.Kind == ssa.KStruct {
				et = t.Fields[i].Type
			} else {
				et = ft[i]
			}
			g.eqAt(b, et, ra, rb, off+8*fo, no, false)
		}
	case ssa.KEnum:
		fmt.Fprintf(b, "\tldr x9, [%s, #%d]\n\tldr x10, [%s, #%d]\n\tcmp x9, x10\n\tb.ne %s\n", ra, off, rb, off, no)
		end := g.helperLabel("eq_tag")
		for vi, vr := range t.Variants {
			words := 0
			for _, f := range vr.Fields {
				words += g.nwords(f.Type)
			}
			if words == 0 {
				continue
			}
			next := g.helperLabel("eq_var")
			// the tag again: comparing a field may have called a function
			fmt.Fprintf(b, "\tldr x9, [%s, #%d]\n\tcmp x9, #%d\n\tb.ne %s\n", ra, off, vi, next)
			for fi, f := range vr.Fields {
				po, _ := g.payloadWords(t, vi, fi)
				g.eqAt(b, f.Type, ra, rb, off+8*po, no, false)
			}
			fmt.Fprintf(b, "\tb %s\n%s:\n", end, next)
		}
		fmt.Fprintf(b, "%s:\n", end)
	default:
		fail("equality of `%s`", t.Name)
	}
}
