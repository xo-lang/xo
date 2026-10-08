package arm64

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ssa"
)

// Reference counting helpers, generated in assembly per program.
//
//   - drop of a Str or List: x0 is the object; decrement its count, or
//     free it at one (a list releases its elements first, through the
//     element callback, in xo_rl_free).
//   - dup and drop of an enum, struct, or tuple: the value's words in x0
//     to x7 (floating point words as bits); they walk the heap leaves.
//   - element callbacks (dupp, dropp) for xo_rl_copy and xo_rl_free: x0
//     points to a list element; they load it and go to dup or drop.
//
// Retaining a Str or List is inlined at the call site.

// rcHelper returns the symbol of the dup or drop helper of t.
func (g *gen) rcHelper(kind string, t *ssa.Type) string {
	key := kind + " " + t.Name
	if s, ok := g.helped[key]; ok {
		return s
	}
	g.nhelp++
	// id names this helper's labels: helpers of the leaves are generated
	// while this one is, and advance g.nhelp
	id := g.nhelp
	sym := g.local(fmt.Sprintf("xo_%s%d", kind, id))
	g.helped[key] = sym
	var b strings.Builder
	fmt.Fprintf(&b, "// %s %s\n\t.p2align 2\n%s:\n", kind, t.Name, sym)
	switch t.Kind {
	case ssa.KFunc:
		g.funcRC(&b, kind, sym)
	case ssa.KBox:
		g.boxRC(&b, kind, sym, t)
	case ssa.KStr, ssa.KList:
		if kind == "dup" {
			b.WriteString("\tldr x1, [x0]\n\tadd x1, x1, #1\n\tstr x1, [x0]\n\tret\n")
			break
		}
		free := g.local(fmt.Sprintf("xo_%s%d_free", kind, id))
		fmt.Fprintf(&b, "\tldr x1, [x0]\n\tcmp x1, #1\n\tb.eq %s\n\tsub x1, x1, #1\n\tstr x1, [x0]\n\tret\n%s:\n", free, free)
		if t.Kind == ssa.KStr {
			fmt.Fprintf(&b, "\tb %s\n", g.sym("xo_rc_free"))
			break
		}
		et := t.Elems[0]
		b.WriteString(immText(g.esize(et), "x1"))
		if et.Heap() {
			b.WriteString(g.addr("x2", g.elemHelper("drop", et)))
		} else {
			b.WriteString("\tmov x2, #0\n")
		}
		fmt.Fprintf(&b, "\tb %s\n", g.sym("xo_rl_free"))
	default:
		// the value's words are saved at [sp, #base + 8k]; words past x7
		// come on the stack (at the caller's sp, sp + frame here), and
		// the leaves' helpers get theirs at [sp, #8k] (out bytes)
		n := g.nwords(t)
		var all []leaf
		if t.Kind == ssa.KEnum {
			for vi, v := range t.Variants {
				for fi, f := range v.Fields {
					off, _ := g.payloadWords(t, vi, fi)
					g.heapLeaves(f.Type, off, &all)
				}
			}
		} else {
			g.heapLeaves(t, 0, &all)
		}
		out := 0
		for _, l := range all {
			out = max(out, align16(8*(g.leafWords(l.t)-8)))
		}
		base := out + 16
		frame := align16(base + 8*n)
		if frame >= 4096 {
			fail("reference counting of `%s` (too many words)", t.Name)
		}
		if out == 0 && frame <= 504 {
			fmt.Fprintf(&b, "\tstp x29, x30, [sp, #-%d]!\n\tmov x29, sp\n", frame)
		} else {
			fmt.Fprintf(&b, "\tsub sp, sp, #%d\n\tstp x29, x30, [sp, #%d]\n\tadd x29, sp, #%d\n", frame, out, out)
		}
		for k := 0; k < n; k++ {
			if k < 8 {
				fmt.Fprintf(&b, "\tstr x%d, [sp, #%d]\n", k, base+8*k)
			} else {
				fmt.Fprintf(&b, "\tldr x9, [sp, #%d]\n\tstr x9, [sp, #%d]\n", frame+8*(k-8), base+8*k)
			}
		}
		done := g.local(fmt.Sprintf("xo_%s%d_done", kind, id))
		if t.Kind == ssa.KEnum {
			for vi, v := range t.Variants {
				var ls []leaf
				for fi, f := range v.Fields {
					off, _ := g.payloadWords(t, vi, fi)
					g.heapLeaves(f.Type, off, &ls)
				}
				if len(ls) == 0 {
					continue
				}
				next := g.local(fmt.Sprintf("xo_%s%d_v%d", kind, id, vi))
				fmt.Fprintf(&b, "\tldr x9, [sp, #%d]\n\tcmp x9, #%d\n\tb.ne %s\n", base, vi, next)
				for _, l := range ls {
					g.leafCall(&b, kind, l, base)
				}
				fmt.Fprintf(&b, "\tb %s\n%s:\n", done, next)
			}
		} else {
			for _, l := range all {
				g.leafCall(&b, kind, l, base)
			}
		}
		fmt.Fprintf(&b, "%s:\n", done)
		if out == 0 && frame <= 504 {
			fmt.Fprintf(&b, "\tldp x29, x30, [sp], #%d\n\tret\n", frame)
		} else {
			fmt.Fprintf(&b, "\tldp x29, x30, [sp, #%d]\n\tadd sp, sp, #%d\n\tret\n", out, frame)
		}
	}
	b.WriteString("\n")
	g.helpers.WriteString(b.String())
	return sym
}

// leafWords is the number of words an rc helper of leaf type t takes
// (a Str helper takes only the object).
func (g *gen) leafWords(t *ssa.Type) int {
	if t.Kind == ssa.KStr {
		return 1
	}
	return g.nwords(t)
}

// leafCall loads leaf l of the value saved at [sp, #base] and retains or
// releases it (words past x7 go to the stack arguments at [sp]).
func (g *gen) leafCall(b *strings.Builder, kind string, l leaf, base int) {
	if kind == "dup" && (l.t.Kind == ssa.KStr || l.t.Kind == ssa.KList) {
		fmt.Fprintf(b, "\tldr x9, [sp, #%d]\n\tldr x10, [x9]\n\tadd x10, x10, #1\n\tstr x10, [x9]\n", base+8*l.off)
		return
	}
	n := g.leafWords(l.t)
	for k := 8; k < n; k++ {
		fmt.Fprintf(b, "\tldr x9, [sp, #%d]\n\tstr x9, [sp, #%d]\n", base+8*(l.off+k), 8*(k-8))
	}
	for k := 0; k < n && k < 8; k++ {
		fmt.Fprintf(b, "\tldr x%d, [sp, #%d]\n", k, base+8*(l.off+k))
	}
	fmt.Fprintf(b, "\tbl %s\n", g.rcHelper(kind, l.t))
}

// elemHelper returns the element callback (dupp or dropp) of t.
func (g *gen) elemHelper(kind string, t *ssa.Type) string {
	key := kind + "p " + t.Name
	if s, ok := g.helped[key]; ok {
		return s
	}
	g.nhelp++
	sym := g.local(fmt.Sprintf("xo_%sp%d", kind, g.nhelp))
	g.helped[key] = sym
	inner := g.rcHelper(kind, t)
	var b strings.Builder
	fmt.Fprintf(&b, "// %sp %s\n\t.p2align 2\n%s:\n", kind, t.Name, sym)
	n := g.leafWords(t)
	if n > 8 {
		// the words past x7 go on the stack: no tail call
		out := align16(8 * (n - 8))
		fmt.Fprintf(&b, "\tsub sp, sp, #%d\n\tstp x29, x30, [sp, #%d]\n\tadd x29, sp, #%d\n", out+16, out, out)
		for k := 8; k < n; k++ {
			fmt.Fprintf(&b, "\tldr x9, [x0, #%d]\n\tstr x9, [sp, #%d]\n", 8*k, 8*(k-8))
		}
		for k := 7; k >= 0; k-- {
			fmt.Fprintf(&b, "\tldr x%d, [x0, #%d]\n", k, 8*k)
		}
		fmt.Fprintf(&b, "\tbl %s\n\tldp x29, x30, [sp, #%d]\n\tadd sp, sp, #%d\n\tret\n\n", inner, out, out+16)
		g.helpers.WriteString(b.String())
		return sym
	}
	for k := n - 1; k >= 0; k-- {
		fmt.Fprintf(&b, "\tldr x%d, [x0, #%d]\n", k, 8*k)
	}
	fmt.Fprintf(&b, "\tb %s\n\n", inner)
	g.helpers.WriteString(b.String())
	return sym
}

// cancelRC reports whether retain v can be dropped together with a
// release later in its block: v retains the one heap field of the variant
// its block is known to hold, and the release drops that enum value with
// nothing in between but register aliases (`if let Some(w) = xs.pop()`
// retains w and releases the Option). The release is marked in s.cancel.
func (s *fstate) cancelRC(v *ssa.Value) bool {
	x := v.Args[0]
	if x.Op != ssa.OpPayload || (x.Type.Kind != ssa.KStr && x.Type.Kind != ssa.KList) {
		return false
	}
	ref := x.Aux.(ssa.PayloadRef)
	e := x.Args[0]
	if e.Type.Kind != ssa.KEnum || len(e.Type.Variants[ref.Tag].Fields) != 1 {
		return false
	}
	// the block runs only when e has the variant: its one predecessor
	// branches on tag(e) == Tag
	b := v.Block
	if len(b.Preds) != 1 {
		return false
	}
	br := b.Preds[0].From.Term()
	if br == nil || br.Op != ssa.OpBranch || br.Succs[0].To == br.Succs[1].To || br.Succs[0].To != b {
		return false
	}
	c := br.Args[0]
	if c.Op != ssa.OpEq || c.Args[0].Op != ssa.OpTag || !sameRC(c.Args[0].Args[0], e) {
		return false
	}
	if k, ok := constInt(c.Args[1]); !ok || k != int64(ref.Tag) {
		return false
	}
	i := 0
	for i < len(b.Instrs) && b.Instrs[i] != v {
		i++
	}
	for _, w := range b.Instrs[i+1:] {
		switch w.Op {
		case ssa.OpField, ssa.OpTag, ssa.OpPayload, ssa.OpConst, ssa.OpTuple, ssa.OpStruct:
			continue
		case ssa.OpRelease:
			if sameRC(w.Args[0], e) {
				if s.cancel == nil {
					s.cancel = map[*ssa.Value]bool{}
				}
				s.cancel[w] = true
				return true
			}
		}
		return false
	}
	return false
}

// sameRC reports whether a and b are the same value read through the
// same chain of field projections.
func sameRC(a, b *ssa.Value) bool {
	for a.Op == ssa.OpField && b.Op == ssa.OpField {
		if a.Aux.(int) != b.Aux.(int) {
			return false
		}
		a, b = a.Args[0], b.Args[0]
	}
	return a == b
}
