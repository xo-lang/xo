// Package llvm is the P3 LLVM backend prototype (decision 0040): it lowers
// a checked Xo program in a core subset to textual LLVM IR, which
// internal/build compiles with the system clang together with the C
// runtime in std/llrt (`xo build --backend=llvm`).
//
// The subset: Int (checked arithmetic that faults with the operands and
// the position, with the range analysis of internal/gen removing proven
// safe checks), Bool, Float, Str (literals, interpolation of Int, Float,
// Bool, and Str, `+`, comparison, len), printing with `os.stdio`, structs,
// enums with payloads (Option and Result included), tuples, `match` and
// `if let` with the common pattern forms, `if`, `while`, `for` over ranges
// and lists, functions, recursion, generic functions (monomorphized),
// closures, `List[T]` with push, pop, get, set, index, len, and iteration,
// `?` on Result and Option, `fault`, and `const`. Anything else is
// rejected with an error that names the construct and its position.
//
// Lowering, in short:
//   - every Xo type is an LLVM first class value: Int i64, Bool i1, Float
//     double, Str {ptr, i64}, `()` and capabilities `{}`, a struct a named
//     LLVM struct, a tuple a literal struct, an enum {i64 tag, [N x i64]
//     payload} whose payload is read and written through memory as the
//     variant's struct, a List a pointer to a runtime header, a function
//     value {code ptr, env ptr};
//   - a field whose type contains its own type by value (a recursive
//     struct or enum) is boxed: a pointer to an immutable heap copy;
//   - every binding is an alloca (mem2reg and SROA promote them);
//   - a function that can fail returns its Result enum;
//   - value semantics of lists use a sticky "shared" flag (copy on write,
//     see std/llrt/xo_rt.c): a value copied out of a binding, field, or
//     element marks the list buffers it holds as shared ("retain"), and a
//     mutation of a shared buffer copies it first. Passing an argument to
//     a non `var` parameter is a borrow and marks nothing.
package llvm

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	gogen "github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/ssa"
)

// Options configure generation.
type Options struct {
	// LineRoot is the directory fault locations are made relative to.
	LineRoot string
	// NoRanges turns off the range analysis: every Int operation is
	// checked.
	NoRanges bool
	// NoSSA lowers from the typed AST (the prototype path, bump
	// allocated, never freeing) even when the program lowers to SSA.
	NoSSA bool
}

// Program is the generated IR of one Xo program.
type Program struct {
	IR []byte
	// MainModule is the Xo module whose `main` the program runs.
	MainModule string
	// Funcs counts the function instances emitted.
	Funcs int
	// Path is "ssa" when the program was lowered from internal/ssa with
	// reference counting, "ast" when from the typed AST (the fallback).
	Path string
	// Fallback says why the SSA path was not taken (empty on the SSA
	// path or when Options.NoSSA asked for the AST path).
	Fallback string
}

// Error is a construct the backend does not support, with its position.
type Error struct {
	Pos string
	Msg string
}

func (e *Error) Error() string {
	if e.Pos == "" {
		return e.Msg
	}
	return e.Pos + ": " + e.Msg
}

// Errors is every error of one generation.
type Errors []*Error

func (es Errors) Error() string {
	var b strings.Builder
	for i, e := range es {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(e.Error())
	}
	return b.String()
}

// gen is the state of one program.
type gen struct {
	info    *check.Info
	opts    Options
	prelude *check.Module
	errs    Errors
	errSeen map[string]bool

	typeDefs strings.Builder
	globals  strings.Builder
	funcs    strings.Builder

	types   map[string]*ty
	strs    map[string]string
	locs    map[string]string
	insts   map[string]*inst
	queue   []*inst
	helpers map[string]bool // retain, eq, thunk functions emitted
	safe    map[*check.FuncObj]map[ast.Node]bool
	nclos   int
	nstr    int
}

// Generate lowers the program whose roots are the modules of res: from
// the shared SSA IR with reference counting (decision 0046) when the
// whole program lowers to it, otherwise from the typed AST (the
// prototype path, which never frees), recording why in
// Program.Fallback.
func Generate(res *check.Result, opts Options) (*Program, error) {
	if opts.NoSSA {
		return generateAST(res, opts)
	}
	prog, err := generateViaSSA(res, opts)
	if err == nil {
		return prog, nil
	}
	prog, err2 := generateAST(res, opts)
	if err2 != nil {
		return nil, err2
	}
	prog.Fallback = err.Error()
	return prog, nil
}

// generateViaSSA builds SSA, inserts reference counting, verifies, and
// lowers.
func generateViaSSA(res *check.Result, opts Options) (*Program, error) {
	m, err := ssa.BuildOpts(res, ssa.Options{LineRoot: opts.LineRoot})
	if err != nil {
		return nil, err
	}
	if err := m.Verify(); err != nil {
		return nil, err
	}
	m.InsertRC()
	if err := m.Verify(); err != nil {
		return nil, err
	}
	prog, err := GenerateSSA(m)
	if err != nil {
		return nil, err
	}
	for _, mod := range res.Modules {
		if fo, ok := mod.Scope["main"].(*check.FuncObj); ok && fo.Recv == nil {
			prog.MainModule = mod.Path
			break
		}
	}
	return prog, nil
}

// generateAST lowers from the typed AST.
func generateAST(res *check.Result, opts Options) (*Program, error) {
	info := res.Info()
	g := &gen{info: info, opts: opts, prelude: info.Prelude(), errSeen: map[string]bool{},
		types: map[string]*ty{}, strs: map[string]string{}, locs: map[string]string{}, insts: map[string]*inst{},
		helpers: map[string]bool{}, safe: map[*check.FuncObj]map[ast.Node]bool{}}
	var mainFo *check.FuncObj
	var mainMod *check.Module
	for _, m := range res.Modules {
		if fo, ok := m.Scope["main"].(*check.FuncObj); ok && fo.Recv == nil {
			mainFo, mainMod = fo, m
			break
		}
	}
	if mainFo == nil {
		return nil, Errors{{Msg: "no `main` function in the root module"}}
	}
	if mainFo.Err != nil {
		g.unsupported(mainFo.Decl, "`main` that returns an error (declare `fn main(os: Os)` and handle errors with match)")
	}
	if len(mainFo.TParams) > 0 {
		g.unsupported(mainFo.Decl, "generic `main`")
	}
	main := g.instance(mainFo, nil, mainFo.Decl)
	for len(g.queue) > 0 {
		in := g.queue[0]
		g.queue = g.queue[1:]
		g.emitInstance(in)
	}
	if len(g.errs) > 0 {
		return nil, g.errs
	}
	// the entry point called by the C runtime
	var args []string
	for _, p := range main.sig.params {
		args = append(args, p.ll+" zeroinitializer")
	}
	fmt.Fprintf(&g.funcs, "define void @xo_entry() #1 {\n  call %s @%s(%s)\n  ret void\n}\n", main.sig.ret.ll, quoteName(main.name), strings.Join(args, ", "))
	var out strings.Builder
	out.WriteString("; generated by xo build --backend=llvm (internal/llvm)\n\n")
	out.WriteString(runtimeDecls)
	out.WriteString(g.typeDefs.String())
	out.WriteString("\n")
	out.WriteString(g.globals.String())
	out.WriteString("\n")
	out.WriteString(g.funcs.String())
	return &Program{IR: []byte(out.String()), MainModule: mainMod.Path, Funcs: len(g.insts), Path: "ast"}, nil
}

// runtimeDecls declares the C runtime (std/llrt/xo_rt.c) and intrinsics.
const runtimeDecls = `%xo.str = type { ptr, i64 }
%xo.fn = type { ptr, ptr }
%xo.list = type { i64, i64, i64 }

@xo_list_empty = external global %xo.list
@.str.empty = private unnamed_addr constant [1 x i8] zeroinitializer

declare ptr @xo_alloc(i64) #2
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
declare void @xo_fault_abs(i64, ptr) #0
declare void @xo_fault_float_int(double, ptr) #0
declare void @xo_fault_unreachable(ptr) #0
declare ptr @xo_sb_new() #2
declare void @xo_sb_str(ptr, ptr, i64) #2
declare void @xo_sb_int(ptr, i64) #2
declare void @xo_sb_float(ptr, double) #2
declare void @xo_sb_bool(ptr, i32) #2
declare ptr @xo_sb_done(ptr, ptr) #2
declare ptr @xo_str_concat(ptr, i64, ptr, i64) #2
declare i32 @xo_str_eq(ptr, i64, ptr, i64) #2
declare i64 @xo_str_cmp(ptr, i64, ptr, i64) #2
declare ptr @xo_list_new(i64, i64) #2
declare ptr @xo_list_unshare(ptr, i64, ptr) #3
declare ptr @xo_list_grow(ptr, i64, ptr) #3
declare { i64, i1 } @llvm.sadd.with.overflow.i64(i64, i64)
declare { i64, i1 } @llvm.ssub.with.overflow.i64(i64, i64)
declare { i64, i1 } @llvm.smul.with.overflow.i64(i64, i64)
declare double @llvm.sqrt.f64(double)
declare double @llvm.floor.f64(double)
declare double @llvm.ceil.f64(double)
declare double @llvm.trunc.f64(double)
declare double @llvm.round.f64(double)
declare double @llvm.fabs.f64(double)

attributes #0 = { noreturn cold nounwind }
attributes #1 = { nounwind }
attributes #2 = { nounwind }
attributes #3 = { nounwind cold }

!0 = !{!"branch_weights", i32 1, i32 4000}
; TBAA: list headers and list elements never overlap
!1 = !{!"xo tbaa"}
!2 = !{!"xo.list.header", !1, i64 0}
!3 = !{!2, !2, i64 0}
!4 = !{!"xo.list.element", !1, i64 0}
!5 = !{!4, !4, i64 0}

`

// ---------------------------------------------------------------------------
// Errors and positions

func (g *gen) linePath(file string) string {
	if g.opts.LineRoot != "" {
		if abs, err := filepath.Abs(file); err == nil {
			if rel, err := filepath.Rel(g.opts.LineRoot, abs); err == nil && !strings.HasPrefix(rel, "..") {
				return filepath.ToSlash(rel)
			}
		}
	}
	return filepath.ToSlash(filepath.Clean(file))
}

// loc is the `file:line:col` of a node, as the Go backend prints it.
func (g *gen) loc(n ast.Node) string {
	sp := n.Span()
	return fmt.Sprintf("%s:%d:%d", g.linePath(sp.File), sp.Start.Line, sp.Start.Col)
}

// lineLoc is `file:line` (faults the Go backend locates from the stack).
func (g *gen) lineLoc(n ast.Node) string {
	sp := n.Span()
	return fmt.Sprintf("%s:%d", g.linePath(sp.File), sp.Start.Line)
}

// unsupported records a construct outside the subset.
func (g *gen) unsupported(n ast.Node, what string) {
	pos := ""
	if n != nil {
		pos = g.loc(n)
	}
	msg := "the LLVM backend does not support " + what
	k := pos + msg
	if g.errSeen[k] {
		return
	}
	g.errSeen[k] = true
	g.errs = append(g.errs, &Error{Pos: pos, Msg: msg})
}

// ---------------------------------------------------------------------------
// Globals

// quoteName quotes an LLVM global or type name.
func quoteName(s string) string { return strconv.Quote(s) }

// irBytes encodes s as the body of an LLVM c"..." string.
func irBytes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x20 && c < 0x7f && c != '"' && c != '\\' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "\\%02X", c)
		}
	}
	return b.String()
}

// strConst returns the constant %xo.str operand of a string literal.
func (g *gen) strConst(s string) string {
	if s == "" {
		return "{ ptr @.str.empty, i64 0 }"
	}
	name, ok := g.strs[s]
	if !ok {
		g.nstr++
		name = fmt.Sprintf("@.str.%d", g.nstr)
		g.strs[s] = name
		fmt.Fprintf(&g.globals, "%s = private unnamed_addr constant [%d x i8] c\"%s\"\n", name, len(s), irBytes(s))
	}
	return fmt.Sprintf("{ ptr %s, i64 %d }", name, len(s))
}

// strPtr returns the global holding the bytes of s (for runtime calls).
func (g *gen) strPtr(s string) string {
	if s == "" {
		return "@.str.empty"
	}
	g.strConst(s)
	return g.strs[s]
}

// cstr returns a NUL terminated global for a fault location.
func (g *gen) cstr(s string) string {
	name, ok := g.locs[s]
	if !ok {
		name = fmt.Sprintf("@.loc.%d", len(g.locs)+1)
		g.locs[s] = name
		fmt.Fprintf(&g.globals, "%s = private unnamed_addr constant [%d x i8] c\"%s\\00\"\n", name, len(s)+1, irBytes(s))
	}
	return name
}

// ---------------------------------------------------------------------------
// Function instances

// inst is one emitted function: a declared function or method with
// concrete type arguments.
type inst struct {
	fo   *check.FuncObj
	sub  *check.Subst
	name string
	sig  *sig
	at   ast.Node
}

// sig is the lowered signature of an instance.
type sig struct {
	recv    *ty
	recvVar bool
	params  []*ty
	vars    []bool
	ret     *ty
	res     check.Type // declared result, substituted (unit when omitted)
	err     check.Type // error type, nil when the function cannot fail
}

// typeParams are the non effect type parameters of fo.
func typeParams(fo *check.FuncObj) []*check.TParam {
	var out []*check.TParam
	for _, p := range fo.TParams {
		if !p.IsEffect {
			out = append(out, p)
		}
	}
	return out
}

func (g *gen) funcName(fo *check.FuncObj) string {
	n := fo.Module.Path + "." + fo.Name
	if fo.Recv != nil {
		n = fo.Module.Path + "." + fo.Recv.Name + "." + fo.Name
	}
	return n
}

// instance returns the instance of fo for the type arguments targs (one
// per non effect type parameter), queueing it for emission.
func (g *gen) instance(fo *check.FuncObj, targs []check.Type, at ast.Node) *inst {
	tps := typeParams(fo)
	name := g.funcName(fo)
	if len(tps) > 0 {
		var ks []string
		for _, t := range targs {
			ks = append(ks, g.key(t))
		}
		name += "[" + strings.Join(ks, ",") + "]"
	}
	if in, ok := g.insts[name]; ok {
		return in
	}
	in := &inst{fo: fo, name: name, at: at}
	if len(tps) > 0 {
		in.sub = check.NewSubst(tps, targs)
	}
	g.insts[name] = in
	s := &sig{}
	apply := func(t check.Type) check.Type {
		if t == nil {
			return nil
		}
		return check.Deep(in.sub.Apply(t))
	}
	if fo.Recv != nil {
		s.recv = g.lower(apply(fo.RecvType), at)
		s.recvVar = fo.RecvVar
	}
	for _, p := range fo.Params {
		s.params = append(s.params, g.lower(apply(p.Type), at))
		s.vars = append(s.vars, p.Var)
	}
	s.res = apply(fo.Result)
	if s.res == nil {
		s.res = unitType
	}
	s.err = apply(fo.Err)
	if s.err != nil && check.IsNever(s.err) {
		s.err = nil
	}
	s.ret = g.retType(s.res, s.err, at)
	in.sig = s
	g.queue = append(g.queue, in)
	return in
}

// retType is the lowered return type of a function with result res and
// error err (nil: cannot fail).
func (g *gen) retType(res, err check.Type, at ast.Node) *ty {
	if err == nil {
		return g.lower(res, at)
	}
	return g.lower(g.resultOf(res, err), at)
}

// params renders the LLVM parameter list of a signature, with names.
func (s *sig) paramList(names []string) string {
	var ps []string
	i := 0
	if s.recv != nil {
		if s.recvVar {
			ps = append(ps, "ptr "+names[i])
		} else {
			ps = append(ps, s.recv.ll+" "+names[i])
		}
		i++
	}
	for k, p := range s.params {
		if s.vars[k] {
			ps = append(ps, "ptr "+names[i])
		} else {
			ps = append(ps, p.ll+" "+names[i])
		}
		i++
	}
	return strings.Join(ps, ", ")
}

// safeOps returns the range analysis of fo's body (gen.SafeIntOps).
func (g *gen) safeOps(fo *check.FuncObj) map[ast.Node]bool {
	if g.opts.NoRanges {
		return nil
	}
	s, ok := g.safe[fo]
	if !ok {
		s = gogen.SafeIntOps(g.info, fo)
		g.safe[fo] = s
	}
	return s
}

// emitInstance emits the body of a function instance.
func (g *gen) emitInstance(in *inst) {
	fo := in.fo
	fd := fo.Decl
	if fd == nil || fd.Body == nil {
		g.unsupported(in.at, "a call of `"+fo.DisplayName()+"` (no body)")
		return
	}
	for _, cl := range fd.Clauses {
		switch cl.(type) {
		case *ast.RequiresClause, *ast.EnsuresClause:
			g.unsupported(cl, "contracts (`requires`, `ensures`)")
		}
	}
	f := g.newFn(in.sub, g.safeOps(fo))
	f.res, f.err, f.ret = in.sig.res, in.sig.err, in.sig.ret
	var names []string
	n := 0
	param := func(v *check.VarObj, t *ty, isVar bool) {
		n++
		pn := fmt.Sprintf("%%p%d", n)
		names = append(names, pn)
		if v == nil {
			return
		}
		if isVar {
			f.vars[v] = &local{ptr: pn, t: t}
			return
		}
		a := f.alloca(t)
		f.emit("store %s %s, ptr %s", t.ll, pn, a)
		f.vars[v] = &local{ptr: a, t: t}
	}
	if fo.Recv != nil {
		param(g.info.RecvVar(fo), in.sig.recv, in.sig.recvVar)
	}
	pvs := g.info.ParamVars(fo)
	for i, t := range in.sig.params {
		var v *check.VarObj
		if i < len(pvs) {
			v = pvs[i]
		}
		param(v, t, in.sig.vars[i])
	}
	f.block(fd.Body, dest{kind: dRet})
	g.writeFunc(in.name, in.sig.ret.ll, in.sig.paramList(names), f)
}

// writeFunc writes a finished function.
func (g *gen) writeFunc(name, ret, params string, f *fn) {
	if !f.term {
		f.emit("unreachable")
		f.term = true
	}
	fmt.Fprintf(&g.funcs, "define internal %s @%s(%s) #1 {\nentry:\n%s  br label %%body\nbody:\n%s}\n\n",
		ret, quoteName(name), params, f.allocas.String(), f.body.String())
}
