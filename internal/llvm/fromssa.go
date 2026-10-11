package llvm

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ssa"
)

// Lowering from the shared SSA IR (decision 0045) after reference
// counting (InsertRC, decision 0046). Every SSA value is an LLVM value
// (block parameters are phis); no allocas except scratch slots for enum
// payloads. Memory is reference counted (std/llrt/xo_rt.c, "RC"):
//
//   - a Str is {ptr obj, i64 len}; obj points to {i64 rc, i64 cap, bytes};
//     literals are globals with an immortal count;
//   - a List is a ptr to {i64 rc, i64 len, i64 cap} and the elements;
//   - structs and tuples are LLVM structs, enums {i64 tag, [N x i64]};
//   - a Box[T] (a field of a recursive type) is a ptr to {i64 rc, T};
//   - a function value is {ptr code, ptr env}; env is null or points to
//     {i64 rc, ptr drop, ptr share, captures...}, where drop releases the
//     captures and frees env and share marks the captures shared; code
//     takes the function value and the arguments;
//   - retain and release call generated `xo.dup.T` and `xo.drop.T`
//     helpers (inlined): a count increment, or a decrement that frees at
//     zero after releasing the children; counts shared between threads
//     are negative and updated atomically, literals have an immortal
//     count never written (decision 0058, rcInc and rcDec);
//   - `share` (OpShare) calls `xo.share.T`, which marks every thread
//     local count the value reaches shared;
//   - `reuse` checks the count and copies when it is not 1 (xo_rl_copy
//     updates a shared list held only here in place); list updates then
//     write in place.

// errSSA is a construct the SSA lowering does not handle.
type errSSA struct{ msg string }

func (e *errSSA) Error() string { return e.msg }

// ssaRuntimeDecls declares the RC runtime of std/llrt/xo_rt.c.
const ssaRuntimeDecls = `%xo.str = type { ptr, i64 }
%xo.rlist = type { i64, i64, i64 }

declare ptr @xo_rc_alloc(i64) #2
declare void @xo_rc_free(ptr) #2
declare void @xo_rc_defer(ptr, ptr) #2
declare ptr @xo_rs_concat(ptr, i64, ptr, i64) #2
declare ptr @xo_rs_concat_int(ptr, i64, i64, ptr) #2
declare ptr @xo_rs_concat_float(ptr, i64, double, ptr) #2
declare ptr @xo_rs_of_int(i64, ptr) #2
declare ptr @xo_rs_of_float(double, ptr) #2
declare ptr @xo_rl_new(i64, i64) #2
declare ptr @xo_rl_new_n(i64, i64) #2
declare ptr @xo_rl_copy(ptr, i64, ptr, ptr) #3
declare void @xo_rl_share(ptr, i64, ptr) #2
declare ptr @xo_rl_grow(ptr, i64) #3
declare void @xo_rl_free(ptr, i64, ptr) #2
declare void @xo_print(ptr, i64) #2
declare void @xo_println(ptr, i64) #2
declare void @xo_eprint(ptr, i64) #2
declare void @xo_eprintln(ptr, i64) #2
declare void @xo_fault(ptr, i64, ptr) #0
declare void @xo_fault_overflow(i64, i64, i64, ptr) #0
declare void @xo_fault_shl(i64, i64, ptr) #0
declare void @xo_fault_shift(i64, ptr) #0
declare void @xo_fault_divzero(i64, i64, ptr) #0
declare void @xo_fault_neg(i64, ptr) #0
declare void @xo_fault_index(i64, i64, ptr) #0
declare void @xo_fault_bytes_index(i64, i64, ptr) #0
declare void @xo_fault_abs(i64, ptr) #0
declare void @xo_fault_float_int(double, ptr) #0
declare i32 @xo_str_eq(ptr, i64, ptr, i64) #2
declare i64 @xo_str_cmp(ptr, i64, ptr, i64) #2
declare void @xo_h_free(ptr) #2
declare { i64, i1 } @llvm.sadd.with.overflow.i64(i64, i64)
declare { i64, i1 } @llvm.ssub.with.overflow.i64(i64, i64)
declare { i64, i1 } @llvm.smul.with.overflow.i64(i64, i64)
declare double @llvm.sqrt.f64(double)
declare double @llvm.floor.f64(double)
declare double @llvm.ceil.f64(double)
declare double @llvm.trunc.f64(double)
declare double @llvm.round.f64(double)
declare double @llvm.fabs.f64(double)

declare i32 @xo_personality(...)

; faults unwind (decision 0085): the fault functions and generated code
; may unwind, and every generated function has unwind tables
attributes #0 = { noreturn cold }
attributes #1 = { uwtable }
attributes #2 = { nounwind }
attributes #3 = { nounwind cold noinline }
attributes #4 = { nounwind alwaysinline }
attributes #5 = { "xo-raises" }

!0 = !{!"branch_weights", i32 1, i32 4000}
; TBAA: list headers, list elements, and reference counts never overlap
!1 = !{!"xo tbaa"}
!2 = !{!"xo.list.header", !1, i64 0}
!3 = !{!2, !2, i64 0}
!4 = !{!"xo.list.element", !1, i64 0}
!5 = !{!4, !4, i64 0}
!6 = !{!"xo.rc", !1, i64 0}
!7 = !{!6, !6, i64 0}

`

const (
	tbaaHdr  = ", !tbaa !3"
	tbaaElem = ", !tbaa !5"
	tbaaRC   = ", !tbaa !7"
	// immortal is the count of literals, never written (decision 0058)
	immortal = int64(math.MinInt64)
	// sharedBit is set in every shared (negative, not immortal) count
	sharedBit = int64(1) << 62
)

// sgen is the state of one program lowered from SSA.
type sgen struct {
	m       *ssa.Module
	types   strings.Builder
	globals strings.Builder
	helpers strings.Builder
	funcs   strings.Builder
	named   map[*ssa.Type]bool
	rtDecls strings.Builder // declarations of the rt.* calls used
	strs    map[string]string
	locs    map[string]string
	emitted map[string]bool
	byName  map[string]*ssa.Func
	// byRef marks, per function, the parameters passed by reference
	// (decision 0145); addrTaken the functions used as values, which keep
	// the plain convention
	byRef     map[*ssa.Func][]bool
	addrTaken map[string]bool
}

// byRefMin is the size from which a struct parameter of a direct call
// is passed by reference (decision 0145): a pointer to memory that holds
// the value, written back for a `var` parameter, instead of every field
// as an argument and every field again in the result.
const byRefMin = 256

// byRefOf returns which parameters of f are passed by reference, or nil:
// structs and enums of byRefMin bytes or more (enums since decision 0147:
// their tag and payload reads load from the reference).
func (g *sgen) byRefOf(f *ssa.Func) []bool {
	if br, ok := g.byRef[f]; ok {
		return br
	}
	var br []bool
	if !f.Indirect && !f.Entry && !g.addrTaken[f.Name] && f.Name != "main" {
		for i, p := range f.Params() {
			if (p.Type.Kind == ssa.KStruct || p.Type.Kind == ssa.KEnum) && g.size(p.Type) >= byRefMin {
				if br == nil {
					br = make([]bool, len(f.Params()))
				}
				br[i] = true
			}
		}
	}
	g.byRef[f] = br
	return br
}

// isVarParam reports whether parameter i of f is `var`.
func isVarParam(f *ssa.Func, i int) bool {
	for _, j := range f.VarParams {
		if j == i {
			return true
		}
	}
	return false
}

// keptResults are the indexes of the results of f its LLVM function
// returns: all but those of `var` parameters passed by reference.
func (g *sgen) keptResults(f *ssa.Func) []int {
	br := g.byRefOf(f)
	var out []int
	for k := range f.Results {
		if k > 0 && br != nil && br[f.VarParams[k-1]] {
			continue
		}
		out = append(out, k)
	}
	return out
}

// GenerateSSA lowers a module (after InsertRC) to LLVM IR.
func GenerateSSA(m *ssa.Module) (prog *Program, err error) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*errSSA)
			if !ok {
				panic(r)
			}
			prog, err = nil, e
		}
	}()
	if !m.RC {
		return nil, &errSSA{"the module has no reference counting (run InsertRC)"}
	}
	g := &sgen{m: m, named: map[*ssa.Type]bool{}, strs: map[string]string{}, locs: map[string]string{},
		emitted: map[string]bool{}, byName: map[string]*ssa.Func{},
		byRef: map[*ssa.Func][]bool{}, addrTaken: map[string]bool{}}
	for _, f := range m.Funcs {
		g.byName[f.Name] = f
		for _, b := range f.Blocks {
			for _, v := range b.Instrs {
				switch v.Op {
				case ssa.OpClosure, ssa.OpFuncAddr, ssa.OpRebind:
					if n, ok := v.Aux.(string); ok {
						g.addrTaken[n] = true
					}
				}
			}
		}
	}
	for _, f := range m.Funcs {
		for _, p := range f.Params() {
			g.checkType(p.Type, nil)
		}
	}
	for _, f := range m.Funcs {
		g.fn(f)
	}
	main := g.byName["main"]
	if main == nil {
		return nil, &errSSA{"no main"}
	}
	var args []string
	for _, p := range main.Params() {
		args = append(args, g.ll(p.Type)+" zeroinitializer")
	}
	fmt.Fprintf(&g.funcs, "define void @xo_entry() #1 {\n  call %s %s(%s)\n  ret void\n}\n", g.resLL(main), g.fname(main.Name), strings.Join(args, ", "))
	var out strings.Builder
	out.WriteString("; generated by xo build --backend=llvm (internal/llvm, from internal/ssa with reference counting)\n\n")
	out.WriteString(ssaRuntimeDecls)
	out.WriteString(g.rtDecls.String())
	out.WriteString(g.types.String())
	out.WriteString("\n")
	out.WriteString(g.globals.String())
	out.WriteString("\n")
	out.WriteString(g.helpers.String())
	out.WriteString(g.funcs.String())
	return &Program{IR: []byte(out.String()), Funcs: len(m.Funcs)}, nil
}

func fail(format string, args ...any) { panic(&errSSA{fmt.Sprintf(format, args...)}) }

// checkType rejects types that contain themselves by value (the SSA
// builder boxes recursive fields, so none should reach here).
func (g *sgen) checkType(t *ssa.Type, path map[*ssa.Type]bool) {
	if path == nil {
		path = map[*ssa.Type]bool{}
	}
	if path[t] {
		fail("the SSA lowering does not support the recursive type `%s`", t.Name)
	}
	path[t] = true
	defer delete(path, t)
	switch t.Kind {
	case ssa.KStruct:
		for _, f := range t.Fields {
			g.checkType(f.Type, path)
		}
	case ssa.KEnum:
		for _, v := range t.Variants {
			for _, f := range v.Fields {
				g.checkType(f.Type, path)
			}
		}
	case ssa.KTuple:
		for _, e := range t.Elems {
			g.checkType(e, path)
		}
	}
	// boxes, lists, and function values are on the heap: no recursion by
	// value (an enum holding a List of itself is fine; the element type
	// is checked when it is lowered itself)
}

func (g *sgen) fname(n string) string { return "@" + strconv.Quote("x."+n) }

// ll is the LLVM type of t.
func (g *sgen) ll(t *ssa.Type) string {
	switch t.Kind {
	case ssa.KUnit, ssa.KCap:
		return "{}"
	case ssa.KInt:
		return "i64"
	case ssa.KFloat:
		return "double"
	case ssa.KBool:
		return "i1"
	case ssa.KStr:
		return "%xo.str"
	case ssa.KList:
		g.checkType(t, nil)
		return "ptr"
	case ssa.KFunc:
		return "{ ptr, ptr }"
	case ssa.KBox, ssa.KHandle:
		return "ptr"
	case ssa.KTuple:
		var es []string
		for _, e := range t.Elems {
			es = append(es, g.ll(e))
		}
		return "{ " + strings.Join(es, ", ") + " }"
	case ssa.KStruct:
		name := "%" + strconv.Quote("s."+t.Name)
		if !g.named[t] {
			g.named[t] = true
			g.checkType(t, nil)
			var fs []string
			for _, f := range t.Fields {
				fs = append(fs, g.ll(f.Type))
			}
			if len(fs) == 0 {
				fmt.Fprintf(&g.types, "%s = type {}\n", name)
			} else {
				fmt.Fprintf(&g.types, "%s = type { %s }\n", name, strings.Join(fs, ", "))
			}
		}
		return name
	case ssa.KEnum:
		name := "%" + strconv.Quote("e."+t.Name)
		if !g.named[t] {
			g.named[t] = true
			g.checkType(t, nil)
			fmt.Fprintf(&g.types, "%s = type { i64, [%d x i64] }\n", name, g.words(t))
		}
		return name
	}
	fail("the SSA lowering does not support the type `%s`", t.Name)
	return ""
}

// variantLL is the literal struct of the fields of variant i of t.
func (g *sgen) variantLL(t *ssa.Type, i int) string {
	var fs []string
	for _, f := range t.Variants[i].Fields {
		fs = append(fs, g.ll(f.Type))
	}
	return "{ " + strings.Join(fs, ", ") + " }"
}

// layout returns the size and alignment of t (the LLVM data layout of
// arm64 and x86-64).
func (g *sgen) layout(t *ssa.Type) (int64, int64) {
	switch t.Kind {
	case ssa.KUnit, ssa.KCap:
		return 0, 1
	case ssa.KInt, ssa.KFloat, ssa.KList, ssa.KBox, ssa.KHandle:
		return 8, 8
	case ssa.KBool:
		return 1, 1
	case ssa.KStr, ssa.KFunc:
		return 16, 8
	case ssa.KEnum:
		return 8 + 8*g.words(t), 8
	case ssa.KStruct:
		var ts []*ssa.Type
		for _, f := range t.Fields {
			ts = append(ts, f.Type)
		}
		return g.structLayout(ts)
	case ssa.KTuple:
		return g.structLayout(t.Elems)
	}
	return 8, 8
}

func (g *sgen) structLayout(ts []*ssa.Type) (int64, int64) {
	var size, align int64 = 0, 1
	for _, t := range ts {
		s, a := g.layout(t)
		size = alignUp(size, a) + s
		if a > align {
			align = a
		}
	}
	return alignUp(size, align), align
}

// words is the payload size of enum t in 8 byte words.
func (g *sgen) words(t *ssa.Type) int64 {
	var max int64
	for _, v := range t.Variants {
		var ts []*ssa.Type
		for _, f := range v.Fields {
			ts = append(ts, f.Type)
		}
		s, _ := g.structLayout(ts)
		if s > max {
			max = s
		}
	}
	return (max + 7) / 8
}

func (g *sgen) size(t *ssa.Type) int64 {
	s, _ := g.layout(t)
	return s
}

// resLL is the LLVM return type of f: its kept results (keptResults).
func (g *sgen) resLL(f *ssa.Func) string {
	kept := g.keptResults(f)
	if len(kept) == 1 {
		return g.ll(f.Results[kept[0]])
	}
	var es []string
	for _, k := range kept {
		es = append(es, g.ll(f.Results[k]))
	}
	return "{ " + strings.Join(es, ", ") + " }"
}

// fullResLL is the type of all the results of f, as the SSA call has.
func (g *sgen) fullResLL(f *ssa.Func) string {
	if len(f.Results) == 1 {
		return g.ll(f.Results[0])
	}
	var es []string
	for _, r := range f.Results {
		es = append(es, g.ll(r))
	}
	return "{ " + strings.Join(es, ", ") + " }"
}

// strObj returns the global object of a string literal.
func (g *sgen) strObj(s string) string {
	if name, ok := g.strs[s]; ok {
		return name
	}
	name := fmt.Sprintf("@.rs.%d", len(g.strs))
	g.strs[s] = name
	fmt.Fprintf(&g.globals, "%s = private constant { i64, i64, [%d x i8] } { i64 %d, i64 %d, [%d x i8] c\"%s\" }, align 8\n",
		name, len(s), immortal, len(s), len(s), irBytes(s))
	return name
}

func (g *sgen) strConst(s string) string {
	return fmt.Sprintf("{ ptr %s, i64 %d }", g.strObj(s), len(s))
}

// cstr returns a NUL terminated global for a fault location.
func (g *sgen) cstr(s string) string {
	name, ok := g.locs[s]
	if !ok {
		name = fmt.Sprintf("@.loc.%d", len(g.locs)+1)
		g.locs[s] = name
		fmt.Fprintf(&g.globals, "%s = private unnamed_addr constant [%d x i8] c\"%s\\00\"\n", name, len(s)+1, irBytes(s))
	}
	return name
}

// ---------------------------------------------------------------------------
// Reference counting helpers

// rcFn returns the helper that retains ("dup") or releases ("drop") a
// value of type t, emitting it on first use.
func (g *sgen) rcFn(kind string, t *ssa.Type) string {
	name := "@" + strconv.Quote("xo."+kind+"."+t.Name)
	if g.emitted[name] {
		return name
	}
	g.emitted[name] = true
	var b strings.Builder
	n := 0
	tmp := func() string { n++; return fmt.Sprintf("%%t%d", n) }
	lt := g.ll(t)
	attr := "#1"
	switch t.Kind {
	case ssa.KStr, ssa.KList, ssa.KFunc, ssa.KBox, ssa.KHandle:
		if kind != "share" {
			attr = "#4"
		}
	}
	fmt.Fprintf(&b, "define internal void %s(%s %%x) %s {\n", name, lt, attr)
	switch t.Kind {
	case ssa.KStr, ssa.KList:
		obj := "%x"
		if t.Kind == ssa.KStr {
			obj = tmp()
			fmt.Fprintf(&b, "  %s = extractvalue %%xo.str %%x, 0\n", obj)
		}
		switch kind {
		case "dup":
			rcInc(&b, obj, tmp)
		case "drop":
			rcDec(&b, obj, tmp)
			if t.Kind == ssa.KStr {
				fmt.Fprintf(&b, "free:\n  call void @xo_rc_free(ptr %s)\n  br label %%done\n", obj)
			} else {
				et := t.Elems[0]
				fmt.Fprintf(&b, "free:\n  call void @xo_rl_free(ptr %s, i64 %d, ptr %s)\n  br label %%done\n", obj, g.size(et), g.elemFn("drop", et))
			}
		case "share":
			rcMark(&b, obj, tmp)
			if t.Kind == ssa.KList && t.Elems[0].Heap() {
				et := t.Elems[0]
				fmt.Fprintf(&b, "  call void @xo_rl_share(ptr %s, i64 %d, ptr %s)\n", obj, g.size(et), g.elemFn("share", et))
			}
			b.WriteString("  br label %done\n")
		}
		b.WriteString("done:\n  ret void\n")
	case ssa.KHandle:
		// a runtime object (decision 0068): shared from its creation, so
		// marking it does nothing; at zero the runtime frees it
		switch kind {
		case "dup":
			rcInc(&b, "%x", tmp)
		case "drop":
			rcDec(&b, "%x", tmp)
			b.WriteString("free:\n  call void @xo_h_free(ptr %x)\n  br label %done\n")
		case "share":
			b.WriteString("  br label %done\n")
		}
		b.WriteString("done:\n  ret void\n")
	case ssa.KBox:
		switch kind {
		case "dup":
			rcInc(&b, "%x", tmp)
		case "drop":
			rcDec(&b, "%x", tmp)
			// deferred: a long chain of boxes is freed with a worklist
			fmt.Fprintf(&b, "free:\n  call void @xo_rc_defer(ptr %%x, ptr %s)\n  br label %%done\n", g.boxDropFn(t))
		case "share":
			rcMark(&b, "%x", tmp)
			if t.Elems[0].Heap() {
				// the contents through the worklist too
				fmt.Fprintf(&b, "  call void @xo_rc_defer(ptr %%x, ptr %s)\n", g.boxShareFn(t))
			}
			b.WriteString("  br label %done\n")
		}
		b.WriteString("done:\n  ret void\n")
	case ssa.KFunc:
		// the environment: null for a closure without captures
		env := tmp()
		fmt.Fprintf(&b, "  %s = extractvalue { ptr, ptr } %%x, 1\n", env)
		isNull := tmp()
		fmt.Fprintf(&b, "  %s = icmp eq ptr %s, null\n  br i1 %s, label %%done, label %%live\nlive:\n", isNull, env, isNull)
		switch kind {
		case "dup":
			rcInc(&b, env, tmp)
		case "drop":
			rcDec(&b, env, tmp)
			dp, dfn := tmp(), tmp()
			// the drop function frees the environment; deferred so long
			// chains do not recurse on the C stack
			fmt.Fprintf(&b, "free:\n  %s = getelementptr i8, ptr %s, i64 8\n  %s = load ptr, ptr %s\n  call void @xo_rc_defer(ptr %s, ptr %s)\n  br label %%done\n", dp, env, dfn, dp, env, dfn)
		case "share":
			rcMark(&b, env, tmp)
			sp, sfn := tmp(), tmp()
			fmt.Fprintf(&b, "  %s = getelementptr i8, ptr %s, i64 16\n  %s = load ptr, ptr %s\n  call void @xo_rc_defer(ptr %s, ptr %s)\n  br label %%done\n", sp, env, sfn, sp, env, sfn)
		}
		b.WriteString("done:\n  ret void\n")
	case ssa.KStruct, ssa.KTuple:
		var fts []*ssa.Type
		if t.Kind == ssa.KStruct {
			for _, f := range t.Fields {
				fts = append(fts, f.Type)
			}
		} else {
			fts = t.Elems
		}
		for i, ft := range fts {
			if !ft.Heap() {
				continue
			}
			fv := tmp()
			fmt.Fprintf(&b, "  %s = extractvalue %s %%x, %d\n", fv, lt, i)
			fmt.Fprintf(&b, "  call void %s(%s %s)\n", g.rcFn(kind, ft), g.ll(ft), fv)
		}
		b.WriteString("  ret void\n")
	case ssa.KEnum:
		p := tmp()
		fmt.Fprintf(&b, "  %s = alloca %s\n  store %s %%x, ptr %s\n", p, lt, lt, p)
		tag := tmp()
		fmt.Fprintf(&b, "  %s = extractvalue %s %%x, 0\n", tag, lt)
		var cases []string
		var bodies strings.Builder
		for vi, v := range t.Variants {
			heap := false
			for _, f := range v.Fields {
				heap = heap || f.Type.Heap()
			}
			if !heap {
				continue
			}
			cases = append(cases, fmt.Sprintf("i64 %d, label %%v%d", vi, vi))
			fmt.Fprintf(&bodies, "v%d:\n", vi)
			pay := tmp()
			fmt.Fprintf(&bodies, "  %s = getelementptr %s, ptr %s, i32 0, i32 1\n", pay, lt, p)
			vl := g.variantLL(t, vi)
			for fi, f := range v.Fields {
				if !f.Type.Heap() {
					continue
				}
				fp, fv := tmp(), tmp()
				fmt.Fprintf(&bodies, "  %s = getelementptr %s, ptr %s, i32 0, i32 %d\n", fp, vl, pay, fi)
				fmt.Fprintf(&bodies, "  %s = load %s, ptr %s\n", fv, g.ll(f.Type), fp)
				fmt.Fprintf(&bodies, "  call void %s(%s %s)\n", g.rcFn(kind, f.Type), g.ll(f.Type), fv)
			}
			bodies.WriteString("  br label %done\n")
		}
		fmt.Fprintf(&b, "  switch i64 %s, label %%done [ %s ]\n", tag, strings.Join(cases, " "))
		b.WriteString(bodies.String())
		b.WriteString("done:\n  ret void\n")
	default:
		b.WriteString("  ret void\n")
	}
	b.WriteString("}\n\n")
	g.helpers.WriteString(b.String())
	return name
}

// The count tests of decision 0058 (std/llrt/xo_rt.c, "Reference
// counts"): a positive count is thread local, a negative count -n is
// shared (atomic), and the immortal count of literals (math.MinInt64)
// has bit 62 clear where every shared count has it set. The count is
// read with an unordered atomic load (another thread may update a shared
// count at the same time); thread local counts are written with plain
// stores. Each writes the code of one helper from its entry, ending in
// `br label %done` (and rcDec in branches to %free, which the caller
// writes).

// rcInc writes the increment of the count at obj.
func rcInc(b *strings.Builder, obj string, tmp func() string) {
	c, slow, c1, sb, sh := tmp(), tmp(), tmp(), tmp(), tmp()
	fmt.Fprintf(b, "  %s = load atomic i64, ptr %s unordered, align 8%s\n", c, obj, tbaaRC)
	fmt.Fprintf(b, "  %s = icmp slt i64 %s, 1\n  br i1 %s, label %%shared, label %%inc, !prof !0\n", slow, c, slow)
	fmt.Fprintf(b, "inc:\n  %s = add i64 %s, 1\n  store i64 %s, ptr %s%s\n  br label %%done\n", c1, c, c1, obj, tbaaRC)
	fmt.Fprintf(b, "shared:\n  %s = and i64 %s, %d\n  %s = icmp ne i64 %s, 0\n  br i1 %s, label %%atomic, label %%done\n", sb, c, sharedBit, sh, sb, sh)
	fmt.Fprintf(b, "atomic:\n  atomicrmw add ptr %s, i64 -1 monotonic, align 8\n  br label %%done\n", obj)
}

// rcDec writes the decrement of the count at obj, branching to %free
// when it was the last reference.
func rcDec(b *strings.Builder, obj string, tmp func() string) {
	c, slow, c1, one, sb, sh, old, last := tmp(), tmp(), tmp(), tmp(), tmp(), tmp(), tmp(), tmp()
	fmt.Fprintf(b, "  %s = load atomic i64, ptr %s unordered, align 8%s\n", c, obj, tbaaRC)
	fmt.Fprintf(b, "  %s = icmp slt i64 %s, 2\n  br i1 %s, label %%slow, label %%dec, !prof !0\n", slow, c, slow)
	fmt.Fprintf(b, "dec:\n  %s = sub i64 %s, 1\n  store i64 %s, ptr %s%s\n  br label %%done\n", c1, c, c1, obj, tbaaRC)
	fmt.Fprintf(b, "slow:\n  %s = icmp eq i64 %s, 1\n  br i1 %s, label %%free, label %%shared\n", one, c, one)
	fmt.Fprintf(b, "shared:\n  %s = and i64 %s, %d\n  %s = icmp ne i64 %s, 0\n  br i1 %s, label %%atomic, label %%done\n", sb, c, sharedBit, sh, sb, sh)
	fmt.Fprintf(b, "atomic:\n  %s = atomicrmw add ptr %s, i64 1 acq_rel, align 8\n  %s = icmp eq i64 %s, -1\n  br i1 %s, label %%free, label %%done\n", old, obj, last, old, last)
}

// rcMark writes the marking of the count at obj as shared; the code
// after it (in block %mark) runs when the count was thread local, and
// must mark the children and end in `br label %done`.
func rcMark(b *strings.Builder, obj string, tmp func() string) {
	c, loc, n := tmp(), tmp(), tmp()
	fmt.Fprintf(b, "  %s = load atomic i64, ptr %s unordered, align 8%s\n", c, obj, tbaaRC)
	fmt.Fprintf(b, "  %s = icmp sgt i64 %s, 0\n  br i1 %s, label %%mark, label %%done\n", loc, c, loc)
	fmt.Fprintf(b, "mark:\n  %s = sub i64 0, %s\n  store i64 %s, ptr %s%s\n", n, c, n, obj, tbaaRC)
}

// elemFn returns the runtime callback that retains or releases a list
// element through a pointer, or null when elements are not on the heap.
func (g *sgen) elemFn(kind string, t *ssa.Type) string {
	if !t.Heap() {
		return "null"
	}
	name := "@" + strconv.Quote("xo."+kind+"p."+t.Name)
	if g.emitted[name] {
		return name
	}
	g.emitted[name] = true
	inner := g.rcFn(kind, t)
	lt := g.ll(t)
	fmt.Fprintf(&g.helpers, "define internal void %s(ptr %%p) #1 {\n  %%x = load %s, ptr %%p\n  call void %s(%s %%x)\n  ret void\n}\n\n", name, lt, inner, lt)
	return name
}

// boxLL is the LLVM type of the cell of a Box[T]: {i64 rc, T}.
func (g *sgen) boxLL(t *ssa.Type) string {
	return "{ i64, " + g.ll(t.Elems[0]) + " }"
}

// boxDropFn returns the function that releases the contents of a box
// whose count reached zero and frees it.
func (g *sgen) boxDropFn(t *ssa.Type) string {
	name := "@" + strconv.Quote("xo.boxdrop."+t.Name)
	if g.emitted[name] {
		return name
	}
	g.emitted[name] = true
	et := t.Elems[0]
	var b strings.Builder
	fmt.Fprintf(&b, "define internal void %s(ptr %%p) #1 {\n", name)
	if et.Heap() {
		fmt.Fprintf(&b, "  %%vp = getelementptr %s, ptr %%p, i32 0, i32 1\n", g.boxLL(t))
		fmt.Fprintf(&b, "  %%v = load %s, ptr %%vp\n", g.ll(et))
		fmt.Fprintf(&b, "  call void %s(%s %%v)\n", g.rcFn("drop", et), g.ll(et))
	}
	b.WriteString("  call void @xo_rc_free(ptr %p)\n  ret void\n}\n\n")
	g.helpers.WriteString(b.String())
	return name
}

// envLL is the LLVM type of the environment of Indirect function f:
// {i64 rc, ptr drop, ptr share, captures...}.
func (g *sgen) envLL(f *ssa.Func) string {
	parts := []string{"i64", "ptr", "ptr"}
	for _, c := range f.Captures {
		parts = append(parts, g.ll(c))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

// envDropFn returns the function that releases the captures of an
// environment of f and frees it (called when its count reaches zero).
func (g *sgen) envDropFn(f *ssa.Func) string {
	name := "@" + strconv.Quote("xo.envdrop."+f.Name)
	if g.emitted[name] {
		return name
	}
	g.emitted[name] = true
	var b strings.Builder
	el := g.envLL(f)
	fmt.Fprintf(&b, "define internal void %s(ptr %%e) #1 {\n", name)
	for i, c := range f.Captures {
		if !c.Heap() {
			continue
		}
		fmt.Fprintf(&b, "  %%p%d = getelementptr %s, ptr %%e, i32 0, i32 %d\n", i, el, i+envFields)
		fmt.Fprintf(&b, "  %%c%d = load %s, ptr %%p%d\n", i, g.ll(c), i)
		fmt.Fprintf(&b, "  call void %s(%s %%c%d)\n", g.rcFn("drop", c), g.ll(c), i)
	}
	b.WriteString("  call void @xo_rc_free(ptr %e)\n  ret void\n}\n\n")
	g.helpers.WriteString(b.String())
	return name
}

// envFields is the number of header fields of an environment (rc,
// drop, share) before the captures.
const envFields = 3

// envShareFn returns the function that marks the captures of an
// environment of f shared (decision 0058; the environment itself was
// just marked).
func (g *sgen) envShareFn(f *ssa.Func) string {
	name := "@" + strconv.Quote("xo.envshare."+f.Name)
	if g.emitted[name] {
		return name
	}
	g.emitted[name] = true
	var b strings.Builder
	el := g.envLL(f)
	fmt.Fprintf(&b, "define internal void %s(ptr %%e) #1 {\n", name)
	for i, c := range f.Captures {
		if !c.Heap() {
			continue
		}
		fmt.Fprintf(&b, "  %%p%d = getelementptr %s, ptr %%e, i32 0, i32 %d\n", i, el, i+envFields)
		fmt.Fprintf(&b, "  %%c%d = load %s, ptr %%p%d\n", i, g.ll(c), i)
		fmt.Fprintf(&b, "  call void %s(%s %%c%d)\n", g.rcFn("share", c), g.ll(c), i)
	}
	b.WriteString("  ret void\n}\n\n")
	g.helpers.WriteString(b.String())
	return name
}

// boxShareFn returns the function that marks the contents of a box of
// type t shared (the box itself was just marked).
func (g *sgen) boxShareFn(t *ssa.Type) string {
	name := "@" + strconv.Quote("xo.boxshare."+t.Name)
	if g.emitted[name] {
		return name
	}
	g.emitted[name] = true
	et := t.Elems[0]
	var b strings.Builder
	fmt.Fprintf(&b, "define internal void %s(ptr %%p) #1 {\n", name)
	fmt.Fprintf(&b, "  %%vp = getelementptr %s, ptr %%p, i32 0, i32 1\n", g.boxLL(t))
	fmt.Fprintf(&b, "  %%v = load %s, ptr %%vp\n", g.ll(et))
	fmt.Fprintf(&b, "  call void %s(%s %%v)\n", g.rcFn("share", et), g.ll(et))
	b.WriteString("  ret void\n}\n\n")
	g.helpers.WriteString(b.String())
	return name
}

// eqFn returns the helper that compares two values of type t
// structurally (`==` on strings, structs, tuples, enums, and lists),
// emitting it on first use.
func (g *sgen) eqFn(t *ssa.Type) string {
	name := "@" + strconv.Quote("xo.eq."+t.Name)
	if g.emitted[name] {
		return name
	}
	g.emitted[name] = true
	w := &strings.Builder{} // the current block's text
	n := 0
	tmp := func() string { n++; return fmt.Sprintf("%%t%d", n) }
	emit := func(format string, args ...any) { w.WriteString("  " + fmt.Sprintf(format, args...) + "\n") }
	def := func(format string, args ...any) string {
		r := tmp()
		emit("%s = "+format, append([]any{r}, args...)...)
		return r
	}
	// eq compares x and y of type ft
	eq := func(ft *ssa.Type, x, y string) string {
		switch ft.Kind {
		case ssa.KUnit, ssa.KCap:
			return "true"
		case ssa.KInt:
			return def("icmp eq i64 %s, %s", x, y)
		case ssa.KBool:
			return def("icmp eq i1 %s, %s", x, y)
		case ssa.KFloat:
			return def("fcmp oeq double %s, %s", x, y)
		}
		return def("call i1 %s(%s %s, %s %s)", g.eqFn(ft), g.ll(ft), x, g.ll(ft), y)
	}
	lt := g.ll(t)
	var allocas strings.Builder
	switch t.Kind {
	case ssa.KBox:
		// the same cell, or equal contents; a cell holding a Float is
		// always compared (NaN is not equal to itself, decisions/0053)
		et := t.Elems[0]
		if et.HasFloat() {
			emit("br label %%cmp")
		} else {
			same := def("icmp eq ptr %%a, %%b")
			emit("br i1 %s, label %%yes, label %%cmp", same)
		}
		w.WriteString("cmp:\n")
		pa := def("getelementptr %s, ptr %%a, i32 0, i32 1", g.boxLL(t))
		pb := def("getelementptr %s, ptr %%b, i32 0, i32 1", g.boxLL(t))
		x := def("load %s, ptr %s", g.ll(et), pa)
		y := def("load %s, ptr %s", g.ll(et), pb)
		emit("ret i1 %s", eq(et, x, y))
		w.WriteString("yes:\n  ret i1 true\n")
	case ssa.KStr:
		var parts [2][2]string
		for i, x := range []string{"%a", "%b"} {
			obj := def("extractvalue %%xo.str %s, 0", x)
			parts[i][0] = def("getelementptr i8, ptr %s, i64 16", obj)
			parts[i][1] = def("extractvalue %%xo.str %s, 1", x)
		}
		r := def("call i32 @xo_str_eq(ptr %s, i64 %s, ptr %s, i64 %s)", parts[0][0], parts[0][1], parts[1][0], parts[1][1])
		emit("ret i1 %s", def("icmp ne i32 %s, 0", r))
	case ssa.KStruct, ssa.KTuple:
		var fts []*ssa.Type
		if t.Kind == ssa.KStruct {
			for _, f := range t.Fields {
				fts = append(fts, f.Type)
			}
		} else {
			fts = t.Elems
		}
		acc := "true"
		for i, ft := range fts {
			if ft.Kind == ssa.KUnit || ft.Kind == ssa.KCap {
				continue
			}
			x := def("extractvalue %s %%a, %d", lt, i)
			y := def("extractvalue %s %%b, %d", lt, i)
			acc = def("and i1 %s, %s", acc, eq(ft, x, y))
		}
		emit("ret i1 %s", acc)
	case ssa.KEnum:
		ta := def("extractvalue %s %%a, 0", lt)
		tb := def("extractvalue %s %%b, 0", lt)
		same := def("icmp eq i64 %s, %s", ta, tb)
		emit("br i1 %s, label %%tags, label %%no", same)
		w.WriteString("tags:\n")
		pa, pb := tmp(), tmp()
		fmt.Fprintf(&allocas, "  %s = alloca %s\n  %s = alloca %s\n", pa, lt, pb, lt)
		emit("store %s %%a, ptr %s", lt, pa)
		emit("store %s %%b, ptr %s", lt, pb)
		head := w
		var cases []string
		arms := &strings.Builder{}
		for vi, vr := range t.Variants {
			if len(vr.Fields) == 0 {
				continue
			}
			cases = append(cases, fmt.Sprintf("i64 %d, label %%v%d", vi, vi))
			w = arms
			fmt.Fprintf(w, "v%d:\n", vi)
			vl := g.variantLL(t, vi)
			payA := def("getelementptr %s, ptr %s, i32 0, i32 1", lt, pa)
			payB := def("getelementptr %s, ptr %s, i32 0, i32 1", lt, pb)
			acc := "true"
			for fi, f := range vr.Fields {
				if f.Type.Kind == ssa.KUnit || f.Type.Kind == ssa.KCap {
					continue
				}
				fa := def("getelementptr %s, ptr %s, i32 0, i32 %d", vl, payA, fi)
				fb := def("getelementptr %s, ptr %s, i32 0, i32 %d", vl, payB, fi)
				x := def("load %s, ptr %s", g.ll(f.Type), fa)
				y := def("load %s, ptr %s", g.ll(f.Type), fb)
				acc = def("and i1 %s, %s", acc, eq(f.Type, x, y))
			}
			emit("ret i1 %s", acc)
		}
		w = head
		emit("switch i64 %s, label %%yes [ %s ]", ta, strings.Join(cases, " "))
		w.WriteString(arms.String())
		w.WriteString("yes:\n  ret i1 true\nno:\n  ret i1 false\n")
	case ssa.KList:
		et := t.Elems[0]
		var lens [2]string
		for i, x := range []string{"%a", "%b"} {
			p := def("getelementptr %%xo.rlist, ptr %s, i32 0, i32 1", x)
			lens[i] = def("load i64, ptr %s%s", p, tbaaHdr)
		}
		same := def("icmp eq i64 %s, %s", lens[0], lens[1])
		emit("br i1 %s, label %%loop, label %%no", same)
		w.WriteString("loop:\n")
		emit("%%i = phi i64 [ 0, %%entry ], [ %%i1, %%next ]")
		done := def("icmp eq i64 %%i, %s", lens[0])
		emit("br i1 %s, label %%yes, label %%body", done)
		w.WriteString("body:\n")
		e := "true"
		if et.Kind != ssa.KUnit && et.Kind != ssa.KCap {
			da := def("getelementptr %%xo.rlist, ptr %%a, i64 1")
			db := def("getelementptr %%xo.rlist, ptr %%b, i64 1")
			xa := def("getelementptr %s, ptr %s, i64 %%i", g.ll(et), da)
			xb := def("getelementptr %s, ptr %s, i64 %%i", g.ll(et), db)
			x := def("load %s, ptr %s%s", g.ll(et), xa, tbaaElem)
			y := def("load %s, ptr %s%s", g.ll(et), xb, tbaaElem)
			e = eq(et, x, y)
		}
		emit("br i1 %s, label %%next, label %%no", e)
		w.WriteString("next:\n")
		emit("%%i1 = add i64 %%i, 1")
		emit("br label %%loop")
		w.WriteString("yes:\n  ret i1 true\nno:\n  ret i1 false\n")
	default:
		fail("the SSA lowering does not compare `%s`", t.Name)
	}
	g.helpers.WriteString(fmt.Sprintf("define internal i1 %s(%s %%a, %s %%b) #1 {\nentry:\n%s%s}\n\n", name, lt, lt, allocas.String(), w.String()))
	return name
}

// ---------------------------------------------------------------------------
// Functions

type sfn struct {
	g       *sgen
	f       *ssa.Func
	allocas strings.Builder
	body    *strings.Builder
	label   string
	label0  string // the SSA block the current LLVM block belongs to
	names   map[*ssa.Value]string
	n       int
	nlabel  int
	end     map[*ssa.Block]string
	text    map[*ssa.Block]*strings.Builder
	cancel  map[*ssa.Value]bool // releases a retain cancelled (ssa.CancelledRelease)
	// landing pads (decision 0085): the value being lowered lands on
	// padTo (an SSA block) when it faults; its calls that can raise are
	// invokes unwinding to pad (made at the first one)
	padTo *ssa.Block
	pad   string
	pads  strings.Builder
	npad  int
	// home is, per LLVM struct type, the memory that carries a value of
	// it passed by reference: the function's own `var` parameter of the
	// type, or an alloca (decision 0145); spare are more allocas for a
	// call that passes several
	home  map[string]string
	spare map[string][]string
	// addr maps values that sit in memory nothing writes while the
	// function runs to that memory (decision 0147): parameters passed by
	// reference that are not `var` (the caller's slot), fields and
	// payloads of such values, and the contents of boxes and list
	// elements reached from a borrowed parameter (alive and never updated
	// in place while borrowed). Tag, payload, and field reads load from
	// it, and a call passes it to a parameter by reference that is not
	// `var` instead of copying the value into a slot.
	addr map[*ssa.Value]string
	// held is what the memory of each `var` parameter passed by
	// reference holds at each instruction (held.go, decision 0148);
	// lazy marks the values never materialized, read from that memory
	// where they are used
	held       *heldInfo
	stableMemo map[*ssa.Value]bool
	lazy       map[*ssa.Value]int
	uses       map[*ssa.Value][]*ssa.Value
}

// isAgg reports a struct, enum, or tuple type.
func isAgg(t *ssa.Type) bool {
	return t.Kind == ssa.KStruct || t.Kind == ssa.KEnum || t.Kind == ssa.KTuple
}

// rootBorrowed reports whether v is reached from a borrowed parameter or
// a constant through fields, payloads, box contents, and list elements:
// its containers stay alive, and are not updated in place, for the whole
// call (ssa rc.go projKind).
func (s *sfn) rootBorrowed(v *ssa.Value) bool {
	for {
		switch v.Op {
		case ssa.OpConst:
			return true
		case ssa.OpParam:
			if v.Block != s.f.Blocks[0] {
				return false
			}
			for i, p := range s.f.Params() {
				if p == v {
					return i < len(s.f.Borrowed) && s.f.Borrowed[i]
				}
			}
			return false
		case ssa.OpPayload, ssa.OpUnbox, ssa.OpField:
			v = v.Args[0]
		case ssa.OpCall:
			if n, _ := v.Aux.(string); n != "list.index" && n != "list.get" {
				return false
			}
			v = v.Args[0]
		default:
			return false
		}
	}
}

// loadRef reads a value of type t from memory p (decision 0145). One
// aggregate load: a load field by field measured no faster (the unwind
// paths still use every field of the value loaded at entry).
func (s *sfn) loadRef(t *ssa.Type, p string) string {
	return s.def("load %s, ptr %s", s.g.ll(t), p)
}

// storeRef writes value x of type t to memory p (loadRef).
func (s *sfn) storeRef(t *ssa.Type, x, p string) {
	s.emit("store %s %s, ptr %s", s.g.ll(t), x, p)
}

// slot returns memory for an argument of LLVM type lt passed by
// reference; used counts the slots of the type the call already took.
func (s *sfn) slot(lt string, used map[string]int) string {
	n := used[lt]
	used[lt]++
	if n == 0 {
		if h, ok := s.home[lt]; ok {
			return h
		}
		h := s.alloca(lt)
		s.home[lt] = h
		return h
	}
	for len(s.spare[lt]) < n {
		s.spare[lt] = append(s.spare[lt], s.alloca(lt))
	}
	return s.spare[lt][n-1]
}

// raise emits call text c (`T @f(args)`) of a function that can raise a
// fault: an invoke when the value being lowered has a landing pad.
// res names the result ("" for none).
func (s *sfn) raise(res, c string) {
	if s.padTo == nil {
		if res == "" {
			s.emit("call %s", c)
		} else {
			s.emit("%s = call %s", res, c)
		}
		return
	}
	if s.pad == "" {
		s.npad++
		s.pad = fmt.Sprintf("%s.lp%d", s.label0, s.npad)
		fmt.Fprintf(&s.pads, "%s:\n  %%%s.x = landingpad { ptr, i32 } cleanup\n  br label %%%s\n", s.pad, s.pad, s.padTo.Name())
	}
	cont := s.newLabel("cont")
	if res == "" {
		s.emit("invoke %s to label %%%s unwind label %%%s", c, cont, s.pad)
	} else {
		s.emit("%s = invoke %s to label %%%s unwind label %%%s", res, c, cont, s.pad)
	}
	s.start(cont)
}

func (s *sfn) tmp() string {
	s.n++
	return fmt.Sprintf("%%t%d", s.n)
}

func (s *sfn) emit(format string, args ...any) {
	s.body.WriteString("  " + fmt.Sprintf(format, args...) + "\n")
}

func (s *sfn) def(format string, args ...any) string {
	t := s.tmp()
	s.emit("%s = "+format, append([]any{t}, args...)...)
	return t
}

func (s *sfn) newLabel(hint string) string {
	s.nlabel++
	return fmt.Sprintf("%s.%s%d", s.label0, hint, s.nlabel)
}

func (s *sfn) start(l string) {
	s.body.WriteString(l + ":\n")
	s.label = l
}

func (s *sfn) alloca(t string) string {
	a := s.tmp()
	fmt.Fprintf(&s.allocas, "  %s = alloca %s, align 8\n", a, t)
	return a
}

func vname(v *ssa.Value) string { return fmt.Sprintf("%%v%d", v.ID) }

// op returns the operand text of v.
func (s *sfn) op(v *ssa.Value) string {
	switch v.Type.Kind {
	case ssa.KUnit, ssa.KCap:
		return "zeroinitializer"
	}
	if m, ok := s.lazy[v]; ok {
		// held in memory at every use (decision 0148)
		mem := s.held.mems[m]
		return s.def("load %s, ptr %s", mem.lt, mem.ptr)
	}
	if n, ok := s.names[v]; ok {
		return n
	}
	if v.Op == ssa.OpConst {
		switch x := v.Aux.(type) {
		case int64:
			return strconv.FormatInt(x, 10)
		case float64:
			return fmt.Sprintf("0x%016X", math.Float64bits(x))
		case bool:
			return strconv.FormatBool(x)
		case string:
			return s.g.strConst(x)
		}
	}
	return vname(v)
}

func (s *sfn) typed(v *ssa.Value) string { return s.g.ll(v.Type) + " " + s.op(v) }

func (g *sgen) fn(f *ssa.Func) {
	s := &sfn{g: g, f: f, end: map[*ssa.Block]string{}, text: map[*ssa.Block]*strings.Builder{}, names: map[*ssa.Value]string{},
		home: map[string]string{}, spare: map[string][]string{}, addr: map[*ssa.Value]string{},
		stableMemo: map[*ssa.Value]bool{}, lazy: map[*ssa.Value]int{}}
	var ps []string
	var loads strings.Builder
	s.body = &loads
	br := g.byRefOf(f)
	var mems []heldMem
	for i, p := range f.Params() {
		if br != nil && br[i] {
			lt := g.ll(p.Type)
			ptr := fmt.Sprintf("%%p%d", p.ID)
			if isVarParam(f, i) {
				ps = append(ps, "ptr noalias "+ptr)
				if _, ok := s.home[lt]; !ok {
					s.home[lt] = ptr
					mems = append(mems, heldMem{ptr: ptr, lt: lt, param: p})
				}
			} else {
				ps = append(ps, "ptr noalias readonly "+ptr)
				s.addr[p] = ptr
			}
			continue
		}
		ps = append(ps, g.ll(p.Type)+" "+vname(p))
	}
	s.computeHeld(mems)
	for i, p := range f.Params() {
		if br != nil && br[i] {
			if m, ok := s.memOnly(p); ok {
				s.lazy[p] = m
				continue
			}
			s.alias(p, s.loadRef(p.Type, fmt.Sprintf("%%p%d", p.ID)))
		}
	}
	// lowered in reverse postorder, so a value is lowered (and its alias
	// known) before every use: a block may come before the blocks that
	// dominate it in f.Blocks (generated code builds a join first)
	for _, b := range reversePostorder(f) {
		s.body = &strings.Builder{}
		s.text[b] = s.body
		s.label0 = b.Name()
		s.label = b.Name()
		for _, p := range b.Params {
			if m := s.heldParam(b, p); m >= 0 {
				// passed in place by every edge: no phi (decision 0148)
				if _, ok := s.memOnly(p); ok {
					s.lazy[p] = m
				} else {
					mem := s.held.mems[m]
					s.alias(p, s.def("load %s, ptr %s", mem.lt, mem.ptr))
				}
			}
		}
		for _, v := range b.Instrs {
			s.instr(v)
		}
	}
	var out strings.Builder
	pers := ""
	if s.npad > 0 {
		pers = " personality ptr @xo_personality"
	}
	fmt.Fprintf(&out, "define internal %s %s(%s) #1%s {\nentry:\n%s%s  br label %%b0\n", g.resLL(f), g.fname(f.Name), strings.Join(ps, ", "), pers, s.allocas.String(), loads.String())
	for _, b := range f.Blocks {
		fmt.Fprintf(&out, "%s:\n", b.Name())
		if b != f.Blocks[0] {
			for i, p := range b.Params {
				if p.Type.Kind == ssa.KUnit || p.Type.Kind == ssa.KCap || s.heldParam(b, p) >= 0 {
					continue
				}
				var in []string
				for _, e := range b.Preds {
					in = append(in, fmt.Sprintf("[ %s, %%%s ]", s.op(e.Args[i]), s.end[e.From]))
				}
				fmt.Fprintf(&out, "  %s = phi %s %s\n", vname(p), g.ll(p.Type), strings.Join(in, ", "))
			}
		}
		out.WriteString(s.text[b].String())
	}
	out.WriteString(s.pads.String())
	out.WriteString("}\n\n")
	g.funcs.WriteString(out.String())
}

// reversePostorder lists the blocks of f reachable from its entry in
// reverse postorder (a block after the blocks that dominate it).
func reversePostorder(f *ssa.Func) []*ssa.Block {
	seen := map[*ssa.Block]bool{}
	var post []*ssa.Block
	var walk func(b *ssa.Block)
	walk = func(b *ssa.Block) {
		seen[b] = true
		if t := b.Term(); t != nil {
			for _, e := range t.Succs {
				if !seen[e.To] {
					walk(e.To)
				}
			}
		}
		post = append(post, b)
	}
	walk(f.Blocks[0])
	out := make([]*ssa.Block, 0, len(post))
	for i := len(post) - 1; i >= 0; i-- {
		out = append(out, post[i])
	}
	// blocks the walk did not reach keep their place at the end
	for _, b := range f.Blocks {
		if !seen[b] {
			out = append(out, b)
		}
	}
	return out
}

// faultIf branches to a fault call when cond holds.
func (s *sfn) faultIf(cond, call string, args ...any) {
	bad, ok := s.newLabel("fault"), s.newLabel("ok")
	s.emit("br i1 %s, label %%%s, label %%%s, !prof !0", cond, bad, ok)
	s.start(bad)
	s.raise("", "void "+fmt.Sprintf(call, args...))
	s.emit("unreachable")
	s.start(ok)
}

func (s *sfn) loc(v *ssa.Value) string { return s.g.cstr(v.Pos) }

// listData returns the address of element i of list l.
func (s *sfn) elemPtr(et *ssa.Type, l, i string) string {
	data := s.def("getelementptr %%xo.rlist, ptr %s, i64 1", l)
	return s.def("getelementptr %s, ptr %s, i64 %s", s.g.ll(et), data, i)
}

func (s *sfn) listLen(l string) string {
	p := s.def("getelementptr %%xo.rlist, ptr %s, i32 0, i32 1", l)
	return s.def("load i64, ptr %s%s", p, tbaaHdr)
}

// variant builds variant tag of enum t from field operands.
func (s *sfn) variant(t *ssa.Type, tag int, fields []string) string {
	g := s.g
	lt := g.ll(t)
	if g.words(t) == 0 {
		return s.def("insertvalue %s zeroinitializer, i64 %d, 0", lt, tag)
	}
	a := s.alloca(lt)
	s.emit("store %s zeroinitializer, ptr %s", lt, a)
	s.emit("store i64 %d, ptr %s", tag, a)
	if len(fields) > 0 {
		vl := g.variantLL(t, tag)
		pay := s.def("getelementptr %s, ptr %s, i32 0, i32 1", lt, a)
		for i, fv := range fields {
			fp := s.def("getelementptr %s, ptr %s, i32 0, i32 %d", vl, pay, i)
			s.emit("store %s %s, ptr %s", g.ll(t.Variants[tag].Fields[i].Type), fv, fp)
		}
	}
	return s.def("load %s, ptr %s", lt, a)
}

// payload loads field fi of variant tag of enum value x.
func (s *sfn) payload(t *ssa.Type, x string, tag, fi int) string {
	g := s.g
	lt := g.ll(t)
	a := s.alloca(lt)
	s.emit("store %s %s, ptr %s", lt, x, a)
	pay := s.def("getelementptr %s, ptr %s, i32 0, i32 1", lt, a)
	fp := s.def("getelementptr %s, ptr %s, i32 0, i32 %d", g.variantLL(t, tag), pay, fi)
	return s.def("load %s, ptr %s", g.ll(t.Variants[tag].Fields[fi].Type), fp)
}

// set defines v as the result of an expression (by naming the result).
func (s *sfn) set(v *ssa.Value, format string, args ...any) {
	s.emit("%s = "+format, append([]any{vname(v)}, args...)...)
}

// alias makes operand x the value of v (no instruction).
func (s *sfn) alias(v *ssa.Value, x string) {
	s.names[v] = x
}

var cmpInt = map[ssa.Op]string{ssa.OpEq: "eq", ssa.OpNe: "ne", ssa.OpLt: "slt", ssa.OpLe: "sle", ssa.OpGt: "sgt", ssa.OpGe: "sge"}
var cmpBool = map[ssa.Op]string{ssa.OpEq: "eq", ssa.OpNe: "ne", ssa.OpLt: "ult", ssa.OpLe: "ule", ssa.OpGt: "ugt", ssa.OpGe: "uge"}
var cmpFloat = map[ssa.Op]string{ssa.OpEq: "oeq", ssa.OpNe: "une", ssa.OpLt: "olt", ssa.OpLe: "ole", ssa.OpGt: "ogt", ssa.OpGe: "oge"}
var opChar = map[ssa.Op]int64{ssa.OpAdd: '+', ssa.OpSub: '-', ssa.OpMul: '*', ssa.OpDiv: '/', ssa.OpRem: '%'}

// floatOrder emits the parts of an ordering compare of two Floats and
// returns the instruction (without its result name) that combines them.
// `<`, `<=`, `>`, `>=` follow the total order of std.md 1 (decisions/0053):
// NaN after every other value and equal to NaN, -0.0 equal to 0.0. The
// plain IEEE predicates (olt and friends) are false on NaN.
func (s *sfn) floatOrder(op ssa.Op, a, b string) string {
	switch op {
	case ssa.OpLt: // a < b, or b is NaN and a is not
		x := s.def("fcmp ult double %s, %s", a, b)
		y := s.def("fcmp ord double %s, %s", a, a)
		return fmt.Sprintf("and i1 %s, %s", x, y)
	case ssa.OpGt: // a > b, or a is NaN and b is not
		x := s.def("fcmp ugt double %s, %s", a, b)
		y := s.def("fcmp ord double %s, %s", b, b)
		return fmt.Sprintf("and i1 %s, %s", x, y)
	case ssa.OpLe: // a <= b, or b is NaN
		x := s.def("fcmp ole double %s, %s", a, b)
		y := s.def("fcmp uno double %s, %s", b, b)
		return fmt.Sprintf("or i1 %s, %s", x, y)
	case ssa.OpGe: // a >= b, or a is NaN
		x := s.def("fcmp oge double %s, %s", a, b)
		y := s.def("fcmp uno double %s, %s", a, a)
		return fmt.Sprintf("or i1 %s, %s", x, y)
	}
	fail("float order of op %v", op)
	return ""
}

func (s *sfn) instr(v *ssa.Value) {
	if v.Pad != nil {
		// a fault lands on the cleanup of its check (decision 0085)
		s.padTo, s.pad = v.Pad.Block.Term().Succs[0].To, ""
		defer func() { s.padTo, s.pad = nil, "" }()
	}
	g := s.g
	a := func(i int) string { return s.op(v.Args[i]) }
	switch v.Op {
	case ssa.OpConst, ssa.OpCap:
		return
	case ssa.OpAdd, ssa.OpSub, ssa.OpMul:
		ins := map[ssa.Op]string{ssa.OpAdd: "add", ssa.OpSub: "sub", ssa.OpMul: "mul"}[v.Op]
		if v.Type.Kind == ssa.KFloat {
			s.set(v, "f%s double %s, %s", ins, a(0), a(1))
			return
		}
		if v.Pos == "" {
			if v.Aux == ssa.Wrap {
				s.set(v, "%s i64 %s, %s", ins, a(0), a(1))
				return
			}
			s.set(v, "%s nsw i64 %s, %s", ins, a(0), a(1))
			return
		}
		r := s.def("call { i64, i1 } @llvm.s%s.with.overflow.i64(i64 %s, i64 %s)", ins, a(0), a(1))
		ov := s.def("extractvalue { i64, i1 } %s, 1", r)
		s.faultIf(ov, "@xo_fault_overflow(i64 %s, i64 %d, i64 %s, ptr %s)", a(0), opChar[v.Op], a(1), s.loc(v))
		s.set(v, "extractvalue { i64, i1 } %s, 0", r)
	case ssa.OpDiv, ssa.OpRem:
		if v.Type.Kind == ssa.KFloat {
			s.set(v, "%s double %s, %s", map[ssa.Op]string{ssa.OpDiv: "fdiv", ssa.OpRem: "frem"}[v.Op], a(0), a(1))
			return
		}
		k, isConst := constInt(a(1))
		if v.Pos != "" && !(isConst && k != 0) {
			z := s.def("icmp eq i64 %s, 0", a(1))
			s.faultIf(z, "@xo_fault_divzero(i64 %s, i64 %d, ptr %s)", a(0), opChar[v.Op], s.loc(v))
		}
		if v.Op == ssa.OpDiv {
			if v.Pos != "" && !(isConst && k != -1) {
				m1 := s.def("icmp eq i64 %s, -1", a(1))
				mn := s.def("icmp eq i64 %s, %d", a(0), int64(math.MinInt64))
				both := s.def("and i1 %s, %s", m1, mn)
				s.faultIf(both, "@xo_fault_overflow(i64 %s, i64 %d, i64 %s, ptr %s)", a(0), opChar[v.Op], a(1), s.loc(v))
			}
			s.set(v, "sdiv i64 %s, %s", a(0), a(1))
			return
		}
		if isConst && k != -1 {
			s.set(v, "srem i64 %s, %s", a(0), a(1))
			return
		}
		// Xo's `x % -1` is 0; LLVM's srem of the minimum by -1 is undefined
		m1 := s.def("icmp eq i64 %s, -1", a(1))
		d := s.def("select i1 %s, i64 1, i64 %s", m1, a(1))
		s.set(v, "srem i64 %s, %s", a(0), d)
	case ssa.OpAnd, ssa.OpOr, ssa.OpXor:
		ins := map[ssa.Op]string{ssa.OpAnd: "and", ssa.OpOr: "or", ssa.OpXor: "xor"}[v.Op]
		s.set(v, "%s %s %s, %s", ins, g.ll(v.Type), a(0), a(1))
	case ssa.OpShl, ssa.OpShr:
		if v.Pos != "" {
			bad := s.def("icmp uge i64 %s, 64", a(1))
			s.faultIf(bad, "@xo_fault_shift(i64 %s, ptr %s)", a(1), s.loc(v))
		}
		if v.Op == ssa.OpShr {
			s.set(v, "ashr i64 %s, %s", a(0), a(1))
			return
		}
		r := s.def("shl i64 %s, %s", a(0), a(1))
		if v.Pos != "" {
			back := s.def("ashr i64 %s, %s", r, a(1))
			lost := s.def("icmp ne i64 %s, %s", back, a(0))
			s.faultIf(lost, "@xo_fault_shl(i64 %s, i64 %s, ptr %s)", a(0), a(1), s.loc(v))
		}
		s.alias(v, r)
	case ssa.OpNeg:
		if v.Type.Kind == ssa.KFloat {
			s.set(v, "fneg double %s", a(0))
			return
		}
		if v.Pos != "" {
			bad := s.def("icmp eq i64 %s, %d", a(0), int64(math.MinInt64))
			s.faultIf(bad, "@xo_fault_neg(i64 %s, ptr %s)", a(0), s.loc(v))
		}
		s.set(v, "sub nsw i64 0, %s", a(0))
	case ssa.OpNot:
		if v.Type.Kind == ssa.KBool {
			s.set(v, "xor i1 %s, true", a(0))
		} else {
			s.set(v, "xor i64 %s, -1", a(0))
		}
	case ssa.OpEq, ssa.OpNe, ssa.OpLt, ssa.OpLe, ssa.OpGt, ssa.OpGe:
		at := v.Args[0].Type
		switch at.Kind {
		case ssa.KInt:
			s.set(v, "icmp %s i64 %s, %s", cmpInt[v.Op], a(0), a(1))
		case ssa.KBool:
			s.set(v, "icmp %s i1 %s, %s", cmpBool[v.Op], a(0), a(1))
		case ssa.KFloat:
			if v.Op == ssa.OpEq || v.Op == ssa.OpNe {
				s.set(v, "fcmp %s double %s, %s", cmpFloat[v.Op], a(0), a(1))
				break
			}
			s.set(v, "%s", s.floatOrder(v.Op, a(0), a(1)))
		case ssa.KUnit:
			r := map[ssa.Op]string{ssa.OpEq: "true", ssa.OpNe: "false", ssa.OpLt: "false", ssa.OpLe: "true", ssa.OpGt: "false", ssa.OpGe: "true"}[v.Op]
			s.set(v, "and i1 %s, true", r)
		case ssa.KStr, ssa.KStruct, ssa.KTuple, ssa.KEnum, ssa.KList:
			if v.Op != ssa.OpEq && v.Op != ssa.OpNe {
				fail("the SSA lowering does not order `%s`", at.Name)
			}
			eq := s.def("call i1 %s(%s, %s)", g.eqFn(at), s.typed(v.Args[0]), s.typed(v.Args[1]))
			if v.Op == ssa.OpNe {
				s.set(v, "xor i1 %s, true", eq)
			} else {
				s.alias(v, eq)
			}
		default:
			fail("the SSA lowering does not compare `%s`", at.Name)
		}
	case ssa.OpIntToFloat:
		s.set(v, "sitofp i64 %s to double", a(0))
	case ssa.OpIntConv:
		s.alias(v, a(0))
	case ssa.OpFloatToInt:
		t := s.def("call double @llvm.trunc.f64(double %s)", a(0))
		if v.Pos != "" {
			lo := s.def("fcmp oge double %s, 0x%016X", t, math.Float64bits(-9223372036854775808.0))
			hi := s.def("fcmp olt double %s, 0x%016X", t, math.Float64bits(9223372036854775808.0))
			in := s.def("and i1 %s, %s", lo, hi)
			bad := s.def("xor i1 %s, true", in)
			s.faultIf(bad, "@xo_fault_float_int(double %s, ptr %s)", a(0), s.loc(v))
		}
		s.set(v, "fptosi double %s to i64", t)
	case ssa.OpStruct, ssa.OpTuple:
		if v.Type.Kind == ssa.KUnit {
			return
		}
		lt := g.ll(v.Type)
		cur := "undef"
		if len(v.Args) == 0 {
			cur = "zeroinitializer"
		}
		for i, x := range v.Args {
			if i == len(v.Args)-1 {
				s.set(v, "insertvalue %s %s, %s, %d", lt, cur, s.typed(x), i)
				return
			}
			cur = s.def("insertvalue %s %s, %s, %d", lt, cur, s.typed(x), i)
		}
		s.alias(v, cur)
	case ssa.OpField:
		if v.Type.Kind == ssa.KUnit || v.Type.Kind == ssa.KCap {
			return
		}
		if m, ok := s.memOnly(v); ok && v.Args[0].Op == ssa.OpCall {
			// a `var` result the call wrote back, held at every use
			s.lazy[v] = m
			return
		}
		if p, ok := s.addr[v.Args[0]]; ok {
			fp := s.def("getelementptr %s, ptr %s, i32 0, i32 %d", g.ll(v.Args[0].Type), p, v.Aux.(int))
			if isAgg(v.Type) {
				s.addr[v] = fp
			}
			s.set(v, "load %s, ptr %s", g.ll(v.Type), fp)
			return
		}
		if m := s.heldAt(v, v.Args[0]); m >= 0 {
			mem := s.held.mems[m]
			fp := s.def("getelementptr %s, ptr %s, i32 0, i32 %d", mem.lt, mem.ptr, v.Aux.(int))
			s.set(v, "load %s, ptr %s", g.ll(v.Type), fp)
			return
		}
		s.set(v, "extractvalue %s %s, %d", g.ll(v.Args[0].Type), a(0), v.Aux.(int))
	case ssa.OpSetField:
		if s.held != nil {
			if m, ok := s.held.inPlace[v]; ok {
				// the memory holds the operand: write the field there
				mem := s.held.mems[m]
				x := s.typed(v.Args[1])
				fp := s.def("getelementptr %s, ptr %s, i32 0, i32 %d", mem.lt, mem.ptr, v.Aux.(int))
				s.emit("store %s, ptr %s", x, fp)
				if _, ok := s.memOnly(v); ok {
					s.lazy[v] = m
				} else {
					s.set(v, "load %s, ptr %s", mem.lt, mem.ptr)
				}
				return
			}
		}
		s.set(v, "insertvalue %s %s, %s, %d", g.ll(v.Type), a(0), s.typed(v.Args[1]), v.Aux.(int))
	case ssa.OpVariant:
		var fs []string
		for i := range v.Args {
			fs = append(fs, a(i))
		}
		s.alias(v, s.variant(v.Type, v.Aux.(int), fs))
	case ssa.OpTag:
		if p, ok := s.addr[v.Args[0]]; ok {
			s.set(v, "load i64, ptr %s", p)
			return
		}
		s.set(v, "extractvalue %s %s, 0", g.ll(v.Args[0].Type), a(0))
	case ssa.OpPayload:
		r := v.Aux.(ssa.PayloadRef)
		if v.Type.Kind == ssa.KUnit || v.Type.Kind == ssa.KCap {
			return
		}
		if p, ok := s.addr[v.Args[0]]; ok {
			t := v.Args[0].Type
			pay := s.def("getelementptr %s, ptr %s, i32 0, i32 1", g.ll(t), p)
			fp := s.def("getelementptr %s, ptr %s, i32 0, i32 %d", g.variantLL(t, r.Tag), pay, r.Field)
			if isAgg(v.Type) {
				s.addr[v] = fp
			}
			s.set(v, "load %s, ptr %s", g.ll(v.Type), fp)
			return
		}
		s.alias(v, s.payload(v.Args[0].Type, a(0), r.Tag, r.Field))
	case ssa.OpCall:
		s.call(v)
	case ssa.OpClosure:
		callee := g.byName[v.Aux.(string)]
		env := "null"
		if len(v.Args) > 0 {
			el := g.envLL(callee)
			in := g.m.Builtin(ssa.KInt)
			size, _ := g.structLayout(append([]*ssa.Type{in, in, in}, callee.Captures...))
			env = s.def("call ptr @xo_rc_alloc(i64 %d)", size)
			s.emit("store i64 1, ptr %s%s", env, tbaaRC)
			dp := s.def("getelementptr %s, ptr %s, i32 0, i32 1", el, env)
			s.emit("store ptr %s, ptr %s", g.envDropFn(callee), dp)
			sp := s.def("getelementptr %s, ptr %s, i32 0, i32 2", el, env)
			s.emit("store ptr %s, ptr %s", g.envShareFn(callee), sp)
			for i, x := range v.Args {
				if x.Type.Kind == ssa.KUnit || x.Type.Kind == ssa.KCap {
					continue
				}
				p := s.def("getelementptr %s, ptr %s, i32 0, i32 %d", el, env, i+envFields)
				s.emit("store %s, ptr %s", s.typed(x), p)
			}
		}
		t0 := s.def("insertvalue { ptr, ptr } undef, ptr %s, 0", g.fname(callee.Name))
		s.set(v, "insertvalue { ptr, ptr } %s, ptr %s, 1", t0, env)
	case ssa.OpFuncAddr:
		s.set(v, "ptrtoint ptr %s to i64", g.fname(v.Aux.(string)))
	case ssa.OpRebind:
		s.set(v, "insertvalue { ptr, ptr } %s, ptr %s, 0", a(0), g.fname(v.Aux.(string)))
	case ssa.OpBox:
		et := v.Args[0].Type
		size, _ := g.structLayout([]*ssa.Type{g.m.Builtin(ssa.KInt), et})
		p := s.def("call ptr @xo_rc_alloc(i64 %d)", size)
		s.emit("store i64 1, ptr %s%s", p, tbaaRC)
		if et.Kind != ssa.KUnit && et.Kind != ssa.KCap {
			vp := s.def("getelementptr %s, ptr %s, i32 0, i32 1", g.boxLL(v.Type), p)
			s.emit("store %s, ptr %s", s.typed(v.Args[0]), vp)
		}
		s.alias(v, p)
	case ssa.OpUnbox:
		if v.Type.Kind == ssa.KUnit || v.Type.Kind == ssa.KCap {
			return
		}
		vp := s.def("getelementptr %s, ptr %s, i32 0, i32 1", g.boxLL(v.Args[0].Type), a(0))
		if isAgg(v.Type) && s.rootBorrowed(v.Args[0]) {
			s.addr[v] = vp
		}
		s.set(v, "load %s, ptr %s", g.ll(v.Type), vp)
	case ssa.OpCapture:
		if v.Type.Kind == ssa.KUnit || v.Type.Kind == ssa.KCap {
			return
		}
		env := s.def("extractvalue { ptr, ptr } %s, 1", a(0))
		p := s.def("getelementptr %s, ptr %s, i32 0, i32 %d", g.envLL(s.f), env, v.Aux.(int)+envFields)
		s.set(v, "load %s, ptr %s", g.ll(v.Type), p)
	case ssa.OpCallValue:
		code := s.def("extractvalue { ptr, ptr } %s, 0", a(0))
		var args []string
		for _, x := range v.Args {
			args = append(args, s.typed(x))
		}
		c := fmt.Sprintf("%s %s(%s)", g.ll(v.Type), code, strings.Join(args, ", "))
		if v.Type.Kind == ssa.KUnit {
			s.raise("", c)
			return
		}
		s.raise(vname(v), c)
	case ssa.OpFault:
		m := a(0)
		obj := s.def("extractvalue %%xo.str %s, 0", m)
		n := s.def("extractvalue %%xo.str %s, 1", m)
		data := s.def("getelementptr i8, ptr %s, i64 16", obj)
		s.emit("call void @xo_fault(ptr %s, i64 %s, ptr %s)", data, n, s.loc(v))
		s.emit("unreachable")
	case ssa.OpRetain:
		x := v.Args[0]
		if x.Op == ssa.OpConst {
			return // literals are immortal
		}
		if r := ssa.CancelledRelease(v); r != nil {
			// a retain and a release of the same reference: neither
			// (LLVM folded the pair while counts were plain; the shared
			// paths of decision 0058 keep it from seeing through)
			if s.cancel == nil {
				s.cancel = map[*ssa.Value]bool{}
			}
			s.cancel[r] = true
			return
		}
		s.emit("call void %s(%s)", g.rcFn("dup", x.Type), s.typed(x))
	case ssa.OpRelease:
		x := v.Args[0]
		if s.cancel[v] {
			return
		}
		s.emit("call void %s(%s)", g.rcFn("drop", x.Type), s.typed(x))
	case ssa.OpShare:
		x := v.Args[0]
		if x.Op == ssa.OpConst {
			return // literals are immortal
		}
		s.emit("call void %s(%s)", g.rcFn("share", x.Type), s.typed(x))
	case ssa.OpReuse:
		x := v.Args[0]
		if x.Type.Kind != ssa.KList {
			fail("reuse of `%s`", x.Type.Name)
		}
		et := x.Type.Elems[0]
		c := s.def("load atomic i64, ptr %s unordered, align 8%s", a(0), tbaaRC)
		shared := s.def("icmp ne i64 %s, 1", c)
		from := s.label
		slow, done := s.newLabel("copy"), s.newLabel("unique")
		s.emit("br i1 %s, label %%%s, label %%%s, !prof !0", shared, slow, done)
		s.start(slow)
		// a copy, or the same list when it is shared but only held here
		cp := s.def("call ptr @xo_rl_copy(ptr %s, i64 %d, ptr %s, ptr %s)", a(0), g.size(et), g.elemFn("dup", et), g.elemFn("drop", et))
		s.emit("br label %%%s", done)
		s.start(done)
		s.set(v, "phi ptr [ %s, %%%s ], [ %s, %%%s ]", a(0), from, cp, slow)
	case ssa.OpJump:
		s.emit("br label %%%s", v.Succs[0].To.Name())
		s.end[v.Block] = s.label
	case ssa.OpBranch:
		s.emit("br i1 %s, label %%%s, label %%%s", a(0), v.Succs[0].To.Name(), v.Succs[1].To.Name())
		s.end[v.Block] = s.label
	case ssa.OpReturn:
		rt := g.resLL(s.f)
		// the `var` parameters passed by reference are written back
		if br := g.byRefOf(s.f); br != nil {
			// every value is read before any is stored: one held in a
			// memory is loaded before another write back overwrites it
			params := s.f.Params()
			var xs, ptrs []string
			var ts []*ssa.Type
			for j, vi := range s.f.VarParams {
				if br[vi] {
					ptr := fmt.Sprintf("%%p%d", params[vi].ID)
					if m := s.heldAt(v, v.Args[1+j]); m >= 0 && s.held.mems[m].ptr == ptr {
						continue // already there (decision 0148)
					}
					xs, ptrs, ts = append(xs, s.op(v.Args[1+j])), append(ptrs, ptr), append(ts, params[vi].Type)
				}
			}
			for i := range xs {
				s.storeRef(ts[i], xs[i], ptrs[i])
			}
		}
		kept := g.keptResults(s.f)
		if len(kept) == 1 {
			s.emit("ret %s", s.typed(v.Args[kept[0]]))
			return
		}
		cur := "undef"
		for i, k := range kept {
			cur = s.def("insertvalue %s %s, %s, %d", rt, cur, s.typed(v.Args[k]), i)
		}
		s.emit("ret %s %s", rt, cur)
	case ssa.OpUnreachable:
		s.emit("unreachable")
	default:
		fail("the SSA lowering does not support `%s`", v.Op)
	}
}

// strParts returns the object, data pointer, and length of a Str.
func (s *sfn) strParts(x string) (obj, data, n string) {
	obj = s.def("extractvalue %%xo.str %s, 0", x)
	data = s.def("getelementptr i8, ptr %s, i64 16", obj)
	n = s.def("extractvalue %%xo.str %s, 1", x)
	return
}

func (s *sfn) mkStr(v *ssa.Value, obj, n string) {
	t := s.def("insertvalue %%xo.str undef, ptr %s, 0", obj)
	s.set(v, "insertvalue %%xo.str %s, i64 %s, 1", t, n)
}

func (s *sfn) call(v *ssa.Value) {
	g := s.g
	name := v.Aux.(string)
	a := func(i int) string { return s.op(v.Args[i]) }
	if callee := g.byName[name]; callee != nil {
		br := g.byRefOf(callee)
		var args []string
		slots := make([]string, len(v.Args))
		used := map[string]int{}
		// every operand is read before anything is stored: a value held
		// in a memory is loaded before another argument overwrites it
		type store struct {
			t    *ssa.Type
			x, p string
		}
		var stores []store
		for i, x := range v.Args {
			if p, ok := s.addr[x]; ok && br != nil && br[i] && !isVarParam(callee, i) {
				// in memory nothing writes during the call: passed as it is
				args = append(args, "ptr "+p)
				continue
			}
			if br != nil && br[i] {
				lt := g.ll(x.Type)
				slots[i] = s.slot(lt, used)
				args = append(args, "ptr "+slots[i])
				if m := s.heldAt(v, x); m >= 0 && s.held.mems[m].ptr == slots[i] {
					continue // already in its slot (decision 0148)
				}
				stores = append(stores, store{x.Type, s.op(x), slots[i]})
				continue
			}
			args = append(args, s.typed(x))
		}
		for _, st := range stores {
			s.storeRef(st.t, st.x, st.p)
		}
		c := fmt.Sprintf("%s %s(%s)", g.resLL(callee), g.fname(name), strings.Join(args, ", "))
		kept := g.keptResults(callee)
		if len(kept) == len(callee.Results) {
			if v.Type.Kind == ssa.KUnit {
				s.raise("", c)
				return
			}
			s.raise(vname(v), c)
			return
		}
		// rebuild the results the SSA call has: the `var` parameters
		// passed by reference are read back from their memory
		r := s.tmp()
		s.raise(r, c)
		full := g.fullResLL(callee)
		cur := "undef"
		ki := 0
		for k, rt := range callee.Results {
			var x string
			if k > 0 && br[callee.VarParams[k-1]] {
				if s.lazyResult(v, k) {
					x = "undef" // read from memory where it is used (decision 0148)
				} else {
					x = s.loadRef(rt, slots[callee.VarParams[k-1]])
				}
			} else {
				switch {
				case rt.Kind == ssa.KUnit || rt.Kind == ssa.KCap:
					x = "zeroinitializer"
				case len(kept) == 1:
					x = r
				default:
					x = s.def("extractvalue %s %s, %d", g.resLL(callee), r, ki)
				}
				ki++
			}
			cur = s.def("insertvalue %s %s, %s %s, %d", full, cur, g.ll(rt), x, k)
		}
		s.alias(v, cur)
		return
	}
	if name == "rt.landing" {
		// a check only a fault takes: never true here; the landing pads
		// enter its first successor (decision 0085)
		s.alias(v, "false")
		return
	}
	if strings.HasPrefix(name, "rt.") {
		s.rtCall(v)
		return
	}
	switch name {
	case "list.new":
		et := v.Type.Elems[0]
		if len(v.Args) == 1 {
			// with room for a number of elements
			s.set(v, "call ptr @xo_rl_new_n(i64 %s, i64 %d)", a(0), g.size(et))
			break
		}
		s.set(v, "call ptr @xo_rl_new(i64 4, i64 %d)", g.size(et))
	case "list.len":
		s.alias(v, s.listLen(a(0)))
	case "list.index":
		et := v.Type
		if v.Pos != "" {
			n := s.listLen(a(0))
			bad := s.def("icmp uge i64 %s, %s", a(1), n)
			s.faultIf(bad, "@xo_fault_index(i64 %s, i64 %s, ptr %s)", a(1), n, s.loc(v))
		}
		p := s.elemPtr(et, a(0), a(1))
		if et.Kind == ssa.KUnit {
			return
		}
		if isAgg(et) && s.rootBorrowed(v.Args[0]) {
			s.addr[v] = p
		}
		s.set(v, "load %s, ptr %s%s", g.ll(et), p, tbaaElem)
	case "list.get":
		ot := v.Type
		et := v.Args[0].Type.Elems[0]
		some, none := ot.VariantIndex("Some"), ot.VariantIndex("None")
		n := s.listLen(a(0))
		in := s.def("icmp ult i64 %s, %s", a(1), n)
		yes, no, end := s.newLabel("get"), s.newLabel("none"), s.newLabel("gend")
		s.emit("br i1 %s, label %%%s, label %%%s", in, yes, no)
		s.start(yes)
		p := s.elemPtr(et, a(0), a(1))
		ev := "zeroinitializer"
		if et.Kind != ssa.KUnit {
			ev = s.def("load %s, ptr %s%s", g.ll(et), p, tbaaElem)
		}
		sv := s.variant(ot, some, []string{ev})
		yesEnd := s.label
		s.emit("br label %%%s", end)
		s.start(no)
		nv := s.variant(ot, none, nil)
		noEnd := s.label
		s.emit("br label %%%s", end)
		s.start(end)
		s.set(v, "phi %s [ %s, %%%s ], [ %s, %%%s ]", g.ll(ot), sv, yesEnd, nv, noEnd)
	case "list.set":
		et := v.Args[2].Type
		l := a(0)
		if v.Pos != "" {
			n := s.listLen(l)
			bad := s.def("icmp uge i64 %s, %s", a(1), n)
			s.faultIf(bad, "@xo_fault_index(i64 %s, i64 %s, ptr %s)", a(1), n, s.loc(v))
		}
		p := s.elemPtr(et, l, a(1))
		if et.Heap() {
			old := s.def("load %s, ptr %s%s", g.ll(et), p, tbaaElem)
			s.emit("call void %s(%s %s)", g.rcFn("drop", et), g.ll(et), old)
		}
		if et.Kind != ssa.KUnit {
			s.emit("store %s, ptr %s%s", s.typed(v.Args[2]), p, tbaaElem)
		}
		s.alias(v, l)
	case "list.push":
		et := v.Args[1].Type
		l := a(0)
		n := s.listLen(l)
		cp := s.def("getelementptr %%xo.rlist, ptr %s, i32 0, i32 2", l)
		c := s.def("load i64, ptr %s%s", cp, tbaaHdr)
		full := s.def("icmp eq i64 %s, %s", n, c)
		from := s.label
		grow, room := s.newLabel("grow"), s.newLabel("room")
		s.emit("br i1 %s, label %%%s, label %%%s, !prof !0", full, grow, room)
		s.start(grow)
		l2 := s.def("call ptr @xo_rl_grow(ptr %s, i64 %d)", l, g.size(et))
		s.emit("br label %%%s", room)
		s.start(room)
		lm := s.def("phi ptr [ %s, %%%s ], [ %s, %%%s ]", l, from, l2, grow)
		if et.Kind != ssa.KUnit {
			p := s.elemPtr(et, lm, n)
			s.emit("store %s, ptr %s%s", s.typed(v.Args[1]), p, tbaaElem)
		}
		n1 := s.def("add nuw nsw i64 %s, 1", n)
		lp := s.def("getelementptr %%xo.rlist, ptr %s, i32 0, i32 1", lm)
		s.emit("store i64 %s, ptr %s%s", n1, lp, tbaaHdr)
		s.alias(v, lm)
	case "list.pop":
		lt := v.Args[0].Type
		et := lt.Elems[0]
		ot := v.Type.Elems[1]
		some, none := ot.VariantIndex("Some"), ot.VariantIndex("None")
		l := a(0)
		n := s.listLen(l)
		c := s.def("icmp sgt i64 %s, 0", n)
		yes, no, end := s.newLabel("pop"), s.newLabel("empty"), s.newLabel("pend")
		s.emit("br i1 %s, label %%%s, label %%%s", c, yes, no)
		s.start(yes)
		n1 := s.def("sub i64 %s, 1", n)
		lp := s.def("getelementptr %%xo.rlist, ptr %s, i32 0, i32 1", l)
		s.emit("store i64 %s, ptr %s%s", n1, lp, tbaaHdr)
		ev := "zeroinitializer"
		if et.Kind != ssa.KUnit {
			p := s.elemPtr(et, l, n1)
			ev = s.def("load %s, ptr %s%s", g.ll(et), p, tbaaElem)
		}
		sv := s.variant(ot, some, []string{ev})
		yesEnd := s.label
		s.emit("br label %%%s", end)
		s.start(no)
		nv := s.variant(ot, none, nil)
		noEnd := s.label
		s.emit("br label %%%s", end)
		s.start(end)
		o := s.def("phi %s [ %s, %%%s ], [ %s, %%%s ]", g.ll(ot), sv, yesEnd, nv, noEnd)
		rt := g.ll(v.Type)
		t := s.def("insertvalue %s undef, ptr %s, 0", rt, l)
		s.set(v, "insertvalue %s %s, %s %s, 1", rt, t, g.ll(ot), o)
	case "str.concat":
		ao, _, an := s.strParts(a(0))
		bo, _, bn := s.strParts(a(1))
		obj := s.def("call ptr @xo_rs_concat(ptr %s, i64 %s, ptr %s, i64 %s)", ao, an, bo, bn)
		n := s.def("add i64 %s, %s", an, bn)
		s.mkStr(v, obj, n)
	case "str.concat_int":
		ao, _, an := s.strParts(a(0))
		slot := s.alloca("i64")
		obj := s.def("call ptr @xo_rs_concat_int(ptr %s, i64 %s, i64 %s, ptr %s)", ao, an, a(1), slot)
		n := s.def("load i64, ptr %s", slot)
		s.mkStr(v, obj, n)
	case "str.concat_float":
		ao, _, an := s.strParts(a(0))
		slot := s.alloca("i64")
		obj := s.def("call ptr @xo_rs_concat_float(ptr %s, i64 %s, double %s, ptr %s)", ao, an, a(1), slot)
		n := s.def("load i64, ptr %s", slot)
		s.mkStr(v, obj, n)
	case "str.eq":
		_, ad, an := s.strParts(a(0))
		_, bd, bn := s.strParts(a(1))
		r := s.def("call i32 @xo_str_eq(ptr %s, i64 %s, ptr %s, i64 %s)", ad, an, bd, bn)
		s.set(v, "icmp ne i32 %s, 0", r)
	case "str.cmp":
		_, ad, an := s.strParts(a(0))
		_, bd, bn := s.strParts(a(1))
		s.set(v, "call i64 @xo_str_cmp(ptr %s, i64 %s, ptr %s, i64 %s)", ad, an, bd, bn)
	case "str.of_int", "str.of_float":
		slot := s.alloca("i64")
		fn, at := "@xo_rs_of_int", "i64"
		if name == "str.of_float" {
			fn, at = "@xo_rs_of_float", "double"
		}
		obj := s.def("call ptr %s(%s %s, ptr %s)", fn, at, a(0), slot)
		n := s.def("load i64, ptr %s", slot)
		s.mkStr(v, obj, n)
	case "str.of_bool":
		s.set(v, "select i1 %s, %%xo.str %s, %%xo.str %s", a(0), g.strConst("true"), g.strConst("false"))
	case "Stdio.print", "Stdio.println", "Stdio.eprint", "Stdio.eprintln":
		_, d, n := s.strParts(a(1))
		fn := "@xo_" + strings.TrimPrefix(name, "Stdio.")
		s.emit("call void %s(ptr %s, i64 %s)", fn, d, n)
	case "Int.min", "Int.max", "Float.min", "Float.max":
		lt := g.ll(v.Type)
		var c string
		if v.Type.Kind == ssa.KInt {
			c = s.def("icmp slt i64 %s, %s", a(0), a(1))
		} else {
			// the total order, the receiver on a tie (std.md 1,
			// decisions/0053): min is `b < a ? b : a`, max `a < b ? b : a`
			if strings.HasSuffix(name, ".min") {
				c = s.def("%s", s.floatOrder(ssa.OpLt, a(1), a(0)))
				s.set(v, "select i1 %s, double %s, double %s", c, a(1), a(0))
			} else {
				c = s.def("%s", s.floatOrder(ssa.OpLt, a(0), a(1)))
				s.set(v, "select i1 %s, double %s, double %s", c, a(1), a(0))
			}
			break
		}
		if strings.HasSuffix(name, ".min") {
			s.set(v, "select i1 %s, %s %s, %s %s", c, lt, a(0), lt, a(1))
		} else {
			s.set(v, "select i1 %s, %s %s, %s %s", c, lt, a(1), lt, a(0))
		}
	case "Int.abs":
		if v.Pos != "" {
			bad := s.def("icmp eq i64 %s, %d", a(0), int64(math.MinInt64))
			s.faultIf(bad, "@xo_fault_abs(i64 %s, ptr %s)", a(0), s.loc(v))
		}
		neg := s.def("icmp slt i64 %s, 0", a(0))
		m := s.def("sub nsw i64 0, %s", a(0))
		s.set(v, "select i1 %s, i64 %s, i64 %s", neg, m, a(0))
	case "Float.sqrt", "Float.floor", "Float.ceil", "Float.trunc", "Float.round", "Float.abs":
		intr := strings.TrimPrefix(name, "Float.")
		if intr == "abs" {
			intr = "fabs"
		}
		s.set(v, "call double @llvm.%s.f64(double %s)", intr, a(0))
	case "Float.is_nan":
		s.set(v, "fcmp uno double %s, %s", a(0), a(0))
	case "bytes.index":
		// b[i] on Bytes (decision 0081)
		_, d, n := s.strParts(a(0))
		if v.Pos != "" {
			bad := s.def("icmp uge i64 %s, %s", a(1), n)
			s.faultIf(bad, "@xo_fault_bytes_index(i64 %s, i64 %s, ptr %s)", a(1), n, s.loc(v))
		}
		p := s.def("getelementptr i8, ptr %s, i64 %s", d, a(1))
		c := s.def("load i8, ptr %s", p)
		s.set(v, "zext i8 %s to i64", c)
	case "Str.len":
		s.set(v, "extractvalue %%xo.str %s, 1", a(0))
	case "Str.is_empty":
		n := s.def("extractvalue %%xo.str %s, 1", a(0))
		s.set(v, "icmp eq i64 %s, 0", n)
	default:
		fail("the SSA lowering does not support the runtime call `%s`", name)
	}
}

// rtCall lowers a call of the task runtime (`rt.name` is the C function
// xo_name, decision 0068): an Int is an i64, a Bool an i64 0 or 1, a
// List or handle a pointer, a Str its bytes and length; the result is
// void or one of those.
func (s *sfn) rtCall(v *ssa.Value) {
	g := s.g
	sym := "@xo_" + strings.TrimPrefix(v.Aux.(string), "rt.")
	// `rt.rs_*` functions take their Str operands as the string object
	// (the first is consumed, decision 0074); the others its bytes
	obj := strings.HasPrefix(v.Aux.(string), "rt.rs_")
	var params, args []string
	for _, x := range v.Args {
		switch x.Type.Kind {
		case ssa.KInt:
			params = append(params, "i64")
			args = append(args, "i64 "+s.op(x))
		case ssa.KFloat:
			params = append(params, "double")
			args = append(args, "double "+s.op(x))
		case ssa.KBool:
			params = append(params, "i64")
			args = append(args, "i64 "+s.def("zext i1 %s to i64", s.op(x)))
		case ssa.KList, ssa.KHandle:
			params = append(params, "ptr")
			args = append(args, "ptr "+s.op(x))
		case ssa.KStr:
			o, d, n := s.strParts(s.op(x))
			if obj {
				d = o
			}
			params = append(params, "ptr", "i64")
			args = append(args, "ptr "+d, "i64 "+n)
		default:
			fail("the runtime call `%s` takes a `%s`", v.Aux, x.Type.Name)
		}
	}
	ret := "void"
	slot := ""
	switch v.Type.Kind {
	case ssa.KUnit:
	case ssa.KInt, ssa.KBool:
		ret = "i64"
	case ssa.KFloat:
		ret = "double"
	case ssa.KList, ssa.KHandle:
		ret = "ptr"
	case ssa.KStr:
		// the string object; its length through a pointer
		ret = "ptr"
		slot = s.alloca("i64")
		params = append(params, "ptr")
		args = append(args, "ptr "+slot)
	default:
		fail("the runtime call `%s` returns a `%s`", v.Aux, v.Type.Name)
	}
	if !g.emitted[sym] && !strings.Contains(ssaRuntimeDecls, " "+sym+"(") {
		g.emitted[sym] = true
		attr := "#2"
		if ssa.RuntimeRaises(v.Aux.(string)) {
			attr = "#5" // it raises faults: not nounwind (decision 0085)
		}
		fmt.Fprintf(&g.rtDecls, "declare %s %s(%s) %s\n", ret, sym, strings.Join(params, ", "), attr)
	}
	c := fmt.Sprintf("%s %s(%s)", ret, sym, strings.Join(args, ", "))
	switch v.Type.Kind {
	case ssa.KUnit:
		s.raise("", c)
	case ssa.KBool:
		r := s.tmp()
		s.raise(r, c)
		s.set(v, "icmp ne i64 %s, 0", r)
	case ssa.KStr:
		o := s.tmp()
		s.raise(o, c)
		n := s.def("load i64, ptr %s", slot)
		s.mkStr(v, o, n)
	default:
		s.raise(vname(v), c)
	}
}
