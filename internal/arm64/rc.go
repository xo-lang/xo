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
// Retaining a Str, List, Box, or function value is inlined at the call
// site and in the helpers; releasing a Box or function value is inlined
// too (decrement, and only a count of one calls xo_rc_defer with the
// drop function).
//
// Counts shared between threads (decision 0058; std/llrt/xo_rt.c,
// "Reference counts"): a positive count is thread local and updated
// with plain loads and stores; a negative count -n is shared and
// updated atomically (LSE `stadd` and `ldaddal` on Darwin, whose arm64
// machines all have them; exclusive load and store loops on Linux,
// which also runs on ARMv8.0); the immortal count of literals is
// negative with bit 62 clear and never written. A retain tests bit 63
// (one more predicted branch than a plain count), a release compares
// with one as before (`b.le` instead of `b.eq`). `share` helpers mark a
// value shared: they negate every thread local count they reach and stop
// at counts already negative.

// atomicInc returns the code that adds a reference to the shared count
// at [obj] (a relaxed atomic decrement of the negative count), with
// scratch registers t (64 bit) and st (32 bit, Linux only).
func (g *gen) atomicInc(obj, t, st string) string {
	if g.os == Darwin {
		return fmt.Sprintf("mov %s, #-1\n\tstadd %s, [%s]", t, t, obj)
	}
	return fmt.Sprintf("1:\n\tldxr %s, [%s]\n\tsub %s, %s, #1\n\tstxr %s, %s, [%s]\n\tcbnz %s, 1b", t, obj, t, t, st, t, obj, st)
}

// atomicDec returns the code that drops a reference from the shared
// count at [obj] with acquire and release order, and leaves the flags eq
// when it was the last reference (the count went from -1 to 0).
func (g *gen) atomicDec(obj, t, st string) string {
	if g.os == Darwin {
		return fmt.Sprintf("mov %s, #1\n\tldaddal %s, %s, [%s]\n\tcmn %s, #1", t, t, t, obj, t)
	}
	return fmt.Sprintf("1:\n\tldaxr %s, [%s]\n\tadd %s, %s, #1\n\tstlxr %s, %s, [%s]\n\tcbnz %s, 1b\n\tcmp %s, #0", t, obj, t, t, st, t, obj, st, t)
}

// incText returns the code that retains the object at register obj
// (t and st are scratch, see atomicInc). The labels start with lbl; the
// code falls through when done, or returns when ret is set.
func (g *gen) incText(obj, t, st, lbl string, ret bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\tldr %s, [%s]\n", t, obj)
	if ret {
		fmt.Fprintf(&b, "\ttbnz %s, #63, %s_sh\n\tadd %s, %s, #1\n\tstr %s, [%s]\n\tret\n", t, lbl, t, t, t, obj)
		fmt.Fprintf(&b, "%s_sh:\n\ttbz %s, #62, %s_ret\n\t%s\n%s_ret:\n\tret\n", lbl, t, lbl, g.atomicInc(obj, t, st), lbl)
		return b.String()
	}
	fmt.Fprintf(&b, "\ttbz %s, #63, %s_loc\n\ttbz %s, #62, %s_inc\n\t%s\n\tb %s_inc\n", t, lbl, t, lbl, g.atomicInc(obj, t, st), lbl)
	fmt.Fprintf(&b, "%s_loc:\n\tadd %s, %s, #1\n\tstr %s, [%s]\n%s_inc:\n", lbl, t, t, t, obj, lbl)
	return b.String()
}

// decText returns the code that releases the object at register obj
// (t and st are scratch): it branches to free when that was the last
// reference, else to done (or returns, when done is empty).
func (g *gen) decText(obj, t, st, lbl, free, done string) string {
	var b strings.Builder
	end := "ret"
	if done != "" {
		end = "b " + done
	}
	fmt.Fprintf(&b, "\tldr %s, [%s]\n\tcmp %s, #1\n\tb.le %s_slow\n\tsub %s, %s, #1\n\tstr %s, [%s]\n\t%s\n", t, obj, t, lbl, t, t, t, obj, end)
	fmt.Fprintf(&b, "%s_slow:\n\tb.eq %s\n\ttbz %s, #62, %s_kept\n\t%s\n\tb.eq %s\n%s_kept:\n\t%s\n", lbl, free, t, lbl, g.atomicDec(obj, t, st), free, lbl, end)
	return b.String()
}

// markText returns the code that marks the object at register obj
// shared: it branches to done when the count is not thread local (the
// object is shared or immortal already), else negates it and falls
// through (to the code that marks the children).
func markText(obj, t, done string) string {
	return fmt.Sprintf("\tldr %s, [%s]\n\tcmp %s, #0\n\tb.le %s\n\tneg %s, %s\n\tstr %s, [%s]\n", t, obj, t, done, t, t, t, obj)
}

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
			b.WriteString(g.incText("x0", "x1", "w2", sym, true))
			break
		}
		if kind == "share" {
			ret := g.local(fmt.Sprintf("xo_%s%d_ret", kind, id))
			b.WriteString(markText("x0", "x1", ret))
			if et := t.Elems; t.Kind == ssa.KList && et[0].Heap() {
				b.WriteString(immText(g.esize(et[0]), "x1"))
				b.WriteString(g.addr("x2", g.elemHelper("share", et[0])))
				fmt.Fprintf(&b, "\tb %s\n", g.sym("xo_rl_share"))
			}
			fmt.Fprintf(&b, "%s:\n\tret\n", ret)
			break
		}
		free := g.local(fmt.Sprintf("xo_%s%d_free", kind, id))
		b.WriteString(g.decText("x0", "x1", "w2", sym, free, ""))
		fmt.Fprintf(&b, "%s:\n", free)
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
				for li, l := range ls {
					g.leafCall(&b, kind, l, base, fmt.Sprintf("%s_v%dl%d", sym, vi, li))
				}
				fmt.Fprintf(&b, "\tb %s\n%s:\n", done, next)
			}
		} else {
			for li, l := range all {
				g.leafCall(&b, kind, l, base, fmt.Sprintf("%s_l%d", sym, li))
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
// releases it (words past x7 go to the stack arguments at [sp]). Box and
// function value leaves are counted inline; lbl prefixes the labels of
// the inline code.
func (g *gen) leafCall(b *strings.Builder, kind string, l leaf, base int, lbl string) {
	if kind == "dup" && (l.t.Kind == ssa.KStr || l.t.Kind == ssa.KList || l.t.Kind == ssa.KBox) {
		fmt.Fprintf(b, "\tldr x9, [sp, #%d]\n", base+8*l.off)
		b.WriteString(g.incText("x9", "x10", "w11", lbl, false))
		return
	}
	switch {
	case kind == "share":
		// out of line (helpers of the leaf types)
	case l.t.Kind == ssa.KFunc:
		// the environment word; null: no captures
		fmt.Fprintf(b, "\tldr x0, [sp, #%d]\n\tcbz x0, %s_done\n", base+8*(l.off+1), lbl)
		if kind == "dup" {
			b.WriteString(g.incText("x0", "x9", "w10", lbl, false))
			fmt.Fprintf(b, "%s_done:\n", lbl)
			return
		}
		b.WriteString(g.decText("x0", "x9", "w10", lbl, lbl+"_free", lbl+"_done"))
		fmt.Fprintf(b, "%s_free:\n\tldr x1, [x0, #8]\n\tbl %s\n%s_done:\n", lbl, g.sym("xo_rc_defer"), lbl)
		return
	case l.t.Kind == ssa.KBox:
		fmt.Fprintf(b, "\tldr x0, [sp, #%d]\n", base+8*l.off)
		b.WriteString(g.decText("x0", "x9", "w10", lbl, lbl+"_free", lbl+"_done"))
		fmt.Fprintf(b, "%s_free:\n", lbl)
		b.WriteString(g.addr("x1", g.boxDrop(l.t)))
		fmt.Fprintf(b, "\tbl %s\n%s_done:\n", g.sym("xo_rc_defer"), lbl)
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
