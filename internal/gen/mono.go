package gen

// Monomorphization (the default lowering since 2026-10-08; `xo build
// --no-mono` turns it off; bench/results/perf-mono-2026-10-08.md).
//
// With --no-mono a generic Xo function is one Go generic function, and Go
// compiles it once per GC shape with a runtime dictionary; Eq, Ord, and
// constraint interface methods go through rt functions that switch on the
// dynamic type or convert to an interface. By default:
//
//   - A call of a user generic function or method whose type arguments are
//     concrete calls a specialized copy, `<name>__<types>`, generated into
//     the calling package with the type parameters substituted (pg.mono).
//     Copies are deduplicated per package and requested transitively; the
//     generic Go function stays and is the fallback when an instantiation
//     is not concrete, nests deeper than maxSpecDepth (polymorphic
//     recursion), or a function has more than maxSpecsPerFunc copies.
//   - Inside a copy, `==` and `<` on a type parameter are lowered as for
//     the concrete type: Go operators on scalars, and typed XoEqT/XoCmpT
//     methods (generated for every non generic struct and newtype) instead
//     of rt.Eq/rt.Cmp; a constraint method call (`x.area()` with
//     `T: Shape`) calls the concrete method directly instead of converting
//     to the interface.
//   - `xs.map/filter/fold/any/all/find(fn(...) { ... })` with a closure
//     literal that cannot fail and has no `return` or `?` is lowered to a
//     loop with the closure body inlined (no closure, no Result per
//     element). This applies in every function, generic or not.
//   - A non capturing closure literal passed to a user generic function
//     gets its own copy (`<name>__<types>_lit`) that inlines the literal
//     where the copy calls the parameter (lambda specialization).
//   - Int arithmetic and List indexing use typed, inlinable runtime
//     helpers (rt.AddI and friends, List.AtM in std/rt/mono.go), unless
//     the range analysis (ranges.go) proved them safe.
//
// Go forms (whether a function returns rt.Result, takes `.V`, and so on)
// are decided on the generic, unsubstituted types exactly as without
// --mono, so a copy has the same signature form as the generic function and
// every caller agrees with it. Only Go type text (goType) and the choice of
// operation for a type parameter (compare, display, method dispatch) see
// the substitution.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

const (
	// maxSpecDepth bounds chains of copies requested from copies, which
	// cuts off polymorphic recursion (`f[T]` calling `f[(T, T)]`).
	maxSpecDepth = 8
	// maxSpecsPerFunc bounds the copies of one function per package.
	maxSpecsPerFunc = 64
	// maxMangle is the longest type suffix kept readable; longer ones
	// are hashed.
	maxMangle = 60
)

// specReq is one specialized copy of a generic function.
type specReq struct {
	fo    *check.FuncObj
	sub   *check.Subst // type parameters of fo -> concrete types
	name  string       // Go name in the requesting package
	depth int
	// lits are closure literals passed for function typed parameters
	// (by parameter index): the copy inlines their bodies where it calls
	// the parameter (lambda specialization).
	lits map[int]*ast.FnLit
}

// monoState is the specialization state of one package.
type monoState struct {
	byKey   map[string]string // fo + type text -> Go name
	names   map[string]bool
	perFunc map[*check.FuncObj]int
	pending []*specReq
	// sub is the substitution of the copy being generated (nil outside
	// copies); depth is its depth.
	sub   *check.Subst
	depth int
	// generated counts copies emitted, for reports.
	generated int
}

func (pg *pkgGen) monoOn() bool { return !pg.g.opts.NoMono && !pg.inRT && !pg.noMono }

// monoType applies the substitution of the copy being generated.
func (pg *pkgGen) monoType(t check.Type) check.Type {
	if pg.ms.sub == nil || t == nil {
		return t
	}
	return check.Deep(pg.ms.sub.Apply(t))
}

// allTParams returns the non effect type parameters of fo: its own and
// the ones bound by its receiver.
func allTParams(fo *check.FuncObj) []*check.TParam {
	return typeParams(fo.TParams)
}

// isGenericUserFunc reports whether calls of fo can be specialized.
func isGenericUserFunc(fo *check.FuncObj) bool {
	return fo != nil && !fo.Module.IsStd && !fo.Iface && fo.Decl != nil && fo.Decl.Body != nil && len(allTParams(fo)) > 0
}

// specFor returns the Go name of the copy of fo for the type arguments in
// sub (as seen from the code being generated), requesting it if needed.
// ok is false when the call must use the generic function.
func (f *fnGen) specFor(fo *check.FuncObj, sub *check.Subst) (string, bool) {
	return f.specForLits(fo, sub, nil)
}

// specForLits is specFor for a copy that also inlines the closure
// literals lits.
func (f *fnGen) specForLits(fo *check.FuncObj, sub *check.Subst, lits map[int]*ast.FnLit) (string, bool) {
	pg := f.pg
	if !pg.monoOn() || !isGenericUserFunc(fo) || sub == nil {
		return "", false
	}
	if pg.ms.depth+1 > maxSpecDepth {
		return "", false
	}
	args := make([]check.Type, len(fo.TParams))
	var texts []string
	for i, p := range fo.TParams {
		t := sub.Type(p)
		if p.IsEffect {
			args[i] = t
			continue
		}
		if t == nil {
			return "", false
		}
		t = pg.monoType(t)
		if containsParam(t) {
			return "", false
		}
		args[i] = t
		texts = append(texts, pg.goType(t))
	}
	key := fmt.Sprintf("%p|%s", fo, strings.Join(texts, ","))
	litTag := ""
	if len(lits) > 0 {
		for i := range fo.Params {
			if lit := lits[i]; lit != nil {
				key += fmt.Sprintf("|%d=%p", i, lit)
			}
		}
		litTag = "_lit"
	}
	if name, ok := pg.ms.byKey[key]; ok {
		return name, true
	}
	if pg.ms.perFunc[fo] >= maxSpecsPerFunc {
		return "", false
	}
	base := funcGoName(fo)
	if fo.Recv != nil {
		base = fo.Recv.Name + "_M_" + fo.Name
	}
	if fo.Module != pg.m {
		base = pkgIdent(fo.Module.Path) + "_" + base
	}
	name := base + "__" + mangle(texts) + litTag
	for i := 2; pg.ms.names[name]; i++ {
		name = fmt.Sprintf("%s__%s%s%d", base, mangle(texts), litTag, i)
	}
	pg.ms.names[name] = true
	pg.ms.byKey[key] = name
	pg.ms.perFunc[fo]++
	pg.ms.pending = append(pg.ms.pending, &specReq{fo: fo, sub: check.NewSubst(fo.TParams, args), name: name, depth: pg.ms.depth + 1, lits: lits})
	return name, true
}

// litArgs returns the closure literals of a call of a user generic
// function that its copy can inline: literals that inlinableLit accepts,
// capture no local binding, match the parameter's function type, and whose
// expressions all have concrete types (the copy generates the body with
// its own substitution, which does not cover the caller's type
// parameters). Only parameters that are not `var` qualify.
func (f *fnGen) litArgs(fo *check.FuncObj, sub *check.Subst, x *ast.CallExpr) map[int]*ast.FnLit {
	if !f.pg.monoOn() || !isGenericUserFunc(fo) || sub == nil {
		return nil
	}
	var out map[int]*ast.FnLit
	for i, e := range argExprs(fo.Params, x) {
		if e == nil || fo.Params[i].Var {
			continue
		}
		lit, ft := f.inlinableLit(e)
		if lit == nil || f.captures(lit) || containsParam(ft) || !f.litConcrete(lit) {
			continue
		}
		want, ok := check.Resolve(sub.Apply(fo.Params[i].Type)).(*check.Func)
		if !ok || f.pg.funcType(want) != f.pg.funcType(ft) {
			continue
		}
		if out == nil {
			out = map[int]*ast.FnLit{}
		}
		out[i] = lit
	}
	return out
}

// litConcrete reports whether no expression or binding inside lit has a
// type that mentions a type parameter.
func (f *fnGen) litConcrete(lit *ast.FnLit) bool {
	ok := true
	inspectNode(lit.Body, func(n ast.Node) bool {
		if !ok {
			return false
		}
		switch x := n.(type) {
		case *ast.Ident:
			if v := f.info.VarOf(x); v != nil && containsParam(check.Deep(v.Type)) {
				ok = false
			}
		}
		if e, isExpr := n.(ast.Expr); isExpr {
			if containsParam(f.info.TypeOf(e)) || containsParam(f.info.NaturalType(e)) {
				ok = false
			}
		}
		return ok
	})
	for _, v := range f.info.ClosureParams(lit) {
		if containsParam(check.Deep(v.Type)) {
			ok = false
		}
	}
	return ok
}

// captures reports whether closure lit uses a local binding declared
// outside it.
func (f *fnGen) captures(lit *ast.FnLit) bool {
	sp := lit.Span()
	found := false
	inspectNode(lit.Body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || found {
			return !found
		}
		v := f.info.VarOf(id)
		if v == nil {
			return true
		}
		vs := v.Span
		if vs.File != sp.File || vs.Start.Offset < sp.Start.Offset || vs.Start.Offset >= sp.End.Offset {
			found = true
		}
		return !found
	})
	return found
}

// litCall inlines the body of the closure literal bound to a parameter of
// a copy (lambda specialization) at a call of that parameter.
func (f *fnGen) litCall(x *ast.CallExpr) (string, bool) {
	id, ok := x.Fun.(*ast.Ident)
	if !ok || len(f.litParams) == 0 {
		return "", false
	}
	v := f.info.VarOf(id)
	lit := f.litParams[v]
	if lit == nil {
		return "", false
	}
	ft, ok := check.Resolve(f.nat(lit)).(*check.Func)
	if !ok || len(x.Args) != len(ft.Params) {
		return "", false
	}
	for _, a := range x.Args {
		if a.Name != nil {
			return "", false
		}
	}
	codes := f.operands(len(x.Args), func(i int) string { return f.valAs(x.Args[i].Value, ft.Params[i].T) })
	for i, c := range codes {
		codes[i] = f.spill(c)
	}
	r := f.tmp("r")
	f.line("var %s %s", r, f.goType(ft.Result))
	f.inlineLit(lit, ft, codes, dest{kind: dAssign, name: r, typ: ft.Result})
	// the copy's body sees the parameter's declared type: a call of
	// `fn(T) -> U ! E` is a Result even when E is Never here
	if pt, ok := check.Resolve(f.exprType(x.Fun)).(*check.Func); ok && pt.Err != nil {
		return fmt.Sprintf("%sOk[%s, %s](%s)", f.rt(), f.goType(pt.Result), f.errGo(pt.Err), r), true
	}
	return r, true
}

// mangle turns Go type texts into an identifier suffix.
func mangle(texts []string) string {
	s := strings.Join(texts, "_")
	var b strings.Builder
	under := false
	for _, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			under = false
			continue
		}
		if !under {
			b.WriteByte('_')
			under = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if len(out) > maxMangle {
		h := sha256.Sum256([]byte(s))
		out = out[:maxMangle/2] + "_" + hex.EncodeToString(h[:6])
	}
	if out == "" {
		out = "unit"
	}
	return out
}

// drainSpecs generates the requested copies of every package until no
// request is left.
func (g *gen) drainSpecs() {
	for {
		progress := false
		for _, m := range g.info.AllModules() {
			pg := g.pkgs[m]
			if pg == nil {
				continue
			}
			for len(pg.ms.pending) > 0 {
				req := pg.ms.pending[0]
				pg.ms.pending = pg.ms.pending[1:]
				pg.genFuncWith(req.fo, req)
				pg.ms.generated++
				progress = true
			}
		}
		if !progress {
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Constraint methods

// devirt returns the concrete method a constraint interface method call
// on a type parameter resolves to inside a copy, or nil to dispatch
// through the interface. The concrete method must have the same Go
// signature as the interface method.
func (f *fnGen) devirt(fo *check.FuncObj, recv ast.Expr) (*check.FuncObj, *check.Subst) {
	pg := f.pg
	if pg.ms.sub == nil || !fo.Iface || recv == nil || !methodIsGoMethod(fo) {
		return nil, nil
	}
	pt, ok := check.Resolve(f.typeOf(recv)).(*check.ParamType)
	if !ok {
		return nil, nil
	}
	ct := pg.monoType(pt)
	n := f.g.named(ct)
	if n == nil || n.Obj.Module.IsStd || n.Obj.Kind == check.KInterface {
		return nil, nil
	}
	m, ms := f.g.info.FindMethod(ct, fo.Name)
	if m == nil || m.Iface || m.RecvVar || m.Module.IsStd || len(typeParams(ownTParams(m))) > 0 {
		return nil, nil
	}
	iface := f.g.named(pg.monoType(pt.P.Constraint))
	var isub *check.Subst
	if iface != nil {
		isub = check.NewSubst(iface.Obj.TParams, iface.Args)
	}
	if pg.methodSig(m, ms) != pg.methodSig(fo, isub) {
		return nil, nil
	}
	return m, ms
}

// ---------------------------------------------------------------------------
// Typed comparison (XoEqT, XoCmpT)

// hasTypedEq reports whether values of t have generated XoEqT and, with
// ord, XoCmpT methods: non generic user structs and newtypes under --mono.
func (pg *pkgGen) hasTypedEq(t check.Type, ord bool) bool {
	if !pg.monoOn() {
		return false
	}
	n := pg.g.named(t)
	if n == nil || n.Obj.Module.IsStd || len(n.Obj.TParams) > 0 {
		return false
	}
	if n.Obj.Kind != check.KStruct && n.Obj.Kind != check.KNewtype {
		return false
	}
	return !ord || n.Obj.Derives["Ord"]
}

// fieldEq is a typed equality of two field values of type t.
func (pg *pkgGen) fieldEq(a, b string, t check.Type) string {
	switch {
	case pg.g.isScalar(t):
		return "(" + a + " == " + b + ")"
	case pg.hasTypedEq(t, false):
		return a + ".XoEqT(" + b + ")"
	}
	return fmt.Sprintf("%sEq(%s, %s)", pg.rt(), a, b)
}

// fieldCmp is a typed three way comparison of two field values of type t.
func (pg *pkgGen) fieldCmp(a, b string, t check.Type) string {
	r := pg.rt()
	switch {
	case pg.g.isFloat(t):
		return fmt.Sprintf("%sFloatCmp(float64(%s), float64(%s))", r, a, b)
	case pg.g.isPrelude(t, "Bool"):
		return fmt.Sprintf("%sCmp(%s, %s)", r, a, b)
	case pg.g.isScalar(t):
		return fmt.Sprintf("%sCmpO(%s, %s)", r, a, b)
	case pg.hasTypedEq(t, true):
		return a + ".XoCmpT(" + b + ")"
	}
	return fmt.Sprintf("%sCmp(%s, %s)", r, a, b)
}

// genTypedEq emits XoEqT and, for `derive Ord`, XoCmpT on a non generic
// struct or newtype: the same results as XoEq and XoCmp without boxing.
func (pg *pkgGen) genTypedEq(t *check.TypeObj, self string, fields func(recv string) []fieldRef) {
	if !pg.monoOn() || len(t.TParams) > 0 {
		return
	}
	types := pg.fieldTypes(t)
	fs, os := fields("v"), fields("o")
	pg.p("func (v %s) XoEqT(o %s) bool {", self, self)
	for i := range fs {
		pg.p("\tif !%s {\n\t\treturn false\n\t}", pg.fieldEq(fs[i].code, os[i].code, types[i]))
	}
	pg.p("\treturn true\n}\n")
	if !t.Derives["Ord"] {
		return
	}
	pg.p("func (v %s) XoCmpT(o %s) int {", self, self)
	for i := range fs {
		pg.p("\tif c := %s; c != 0 {\n\t\treturn c\n\t}", pg.fieldCmp(fs[i].code, os[i].code, types[i]))
	}
	pg.p("\treturn 0\n}\n")
}

// fieldTypes lists the field types of a struct, or the target of a
// newtype, in the order genDerived lists them.
func (pg *pkgGen) fieldTypes(t *check.TypeObj) []check.Type {
	if t.Kind == check.KNewtype {
		return []check.Type{t.Target}
	}
	var out []check.Type
	for _, f := range t.Fields {
		out = append(out, f.Type)
	}
	return out
}

// ---------------------------------------------------------------------------
// Higher order prelude methods with closure literals

// inlinableLit returns the closure literal e if its body can be inlined
// where it is called: it cannot fail, has no `var` parameters, and has no
// `return` or `?` of its own.
func (f *fnGen) inlinableLit(e ast.Expr) (*ast.FnLit, *check.Func) {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			break
		}
		e = p.X
	}
	lit, ok := e.(*ast.FnLit)
	if !ok {
		return nil, nil
	}
	ft, ok := check.Resolve(f.nat(lit)).(*check.Func)
	if !ok || ft.Err != nil && !check.IsNever(ft.Err) {
		return nil, nil
	}
	for _, p := range ft.Params {
		if p.Var {
			return nil, nil
		}
	}
	if len(f.info.ClosureParams(lit)) != len(ft.Params) {
		return nil, nil
	}
	ok = true
	inspectNode(lit.Body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.ReturnStmt, *ast.TryExpr:
			ok = false
		case *ast.FnLit:
			return false // its own frame
		}
		return ok
	})
	if !ok {
		return nil, nil
	}
	return lit, ft
}

// inlineLit generates the body of lit with its parameters bound to the Go
// values args, delivering the value to d.
func (f *fnGen) inlineLit(lit *ast.FnLit, ft *check.Func, args []string, d dest) {
	f.line("{")
	f.ind++
	for i, pv := range f.info.ClosureParams(lit) {
		name := f.declName(pv)
		f.line("var %s %s = %s", name, f.goType(ft.Params[i].T), args[i])
		f.line("_ = %s", name)
	}
	f.at(lit.Body)
	f.block(lit.Body, d)
	f.ind--
	f.line("}")
}

// mapView reports a call `m.values()` or `m.keys()` on a Map: the map
// expression and the runtime function that iterates that view in place.
func (f *fnGen) mapView(e ast.Expr) (ast.Expr, string) {
	c, ok := unparen(e).(*ast.CallExpr)
	if !ok || len(c.Args) != 0 {
		return nil, ""
	}
	ci := f.info.CallOf(c)
	if ci == nil || ci.Recv == nil {
		return nil, ""
	}
	switch {
	case isStdMethod(ci.Fn, "prelude", "Map", "values"):
		return ci.Recv, "Map_values_seq"
	case isStdMethod(ci.Fn, "prelude", "Map", "keys"):
		return ci.Recv, "Map_keys_seq"
	}
	return nil, ""
}

// toLowerRecv reports a call `s.to_lower()` on a Str: its receiver.
func (f *fnGen) toLowerRecv(e ast.Expr) ast.Expr {
	c, ok := unparen(e).(*ast.CallExpr)
	if !ok || len(c.Args) != 0 {
		return nil
	}
	if ci := f.info.CallOf(c); ci != nil && ci.Recv != nil && isStdMethod(ci.Fn, "prelude", "Str", "to_lower") {
		return ci.Recv
	}
	return nil
}

// hofInline lowers `xs.map/filter/fold/any/all/find(<closure literal>)`
// to a loop with the closure body inlined. ok is false when the call does
// not qualify.
func (f *fnGen) hofInline(x *ast.CallExpr, ci *check.Call) (string, bool) {
	fo := ci.Fn
	if !f.pg.monoOn() || fo == nil || ci.Recv == nil || !fo.Module.IsStd || fo.Recv == nil || !fo.Recv.Module.IsPrelude || fo.Recv.Name != "List" {
		return "", false
	}
	switch fo.Name {
	case "map", "filter", "fold", "any", "all", "find":
	default:
		return "", false
	}
	sub := ci.Subst
	if fo.Err == nil || sub == nil || !check.IsNever(check.Deep(sub.Apply(fo.Err))) {
		return "", false
	}
	exprs := argExprs(fo.Params, x)
	for _, e := range exprs {
		if e == nil {
			return "", false
		}
	}
	fi := len(exprs) - 1
	lit, ft := f.inlinableLit(exprs[fi])
	if lit == nil {
		return "", false
	}
	want, ok := check.Resolve(sub.Apply(fo.Params[fi].Type)).(*check.Func)
	if !ok || f.pg.funcType(want) != f.pg.funcType(ft) {
		return "", false
	}
	lt := f.g.named(f.typeOf(ci.Recv))
	if lt == nil || len(lt.Args) == 0 {
		return "", false
	}
	elemT := lt.Args[0]
	et := f.goType(elemT)
	r := f.rt()
	var src, init string
	// `m.values()` or `m.keys()` as the receiver: the loop walks the map
	// in place instead of building the list (seq names the runtime
	// iterator, which marks the elements it yields as the list would)
	recvE := ci.Recv
	mapE, seq := f.mapView(ci.Recv)
	if mapE != nil {
		recvE = mapE
	}
	// the receiver is only read by the loop: a borrow (own.go); the
	// inlined body cannot write it unless it names its binding
	recv := func() string {
		if f.borrowOK(recvE, x) {
			return f.borrow(recvE)
		}
		return f.val(recvE)
	}
	if fo.Name == "fold" {
		codes := f.operands(2, func(i int) string {
			if i == 0 {
				return recv()
			}
			return f.valAs(exprs[0], sub.Apply(fo.Params[0].Type))
		})
		src, init = codes[0], codes[1]
	} else {
		src = recv()
	}
	sv := f.tmp("src")
	f.line("%s := %s", sv, src)
	e := f.tmp("e")
	header := fmt.Sprintf("_, %s := range %sList_Items(%s)", e, f.rt(), sv)
	length := fmt.Sprintf("len(%sList_Items(%s))", f.rt(), sv)
	if mapE != nil {
		header = fmt.Sprintf("%s := range %s%s(%s)", e, r, seq, sv)
		length = fmt.Sprintf("%sMap_Len(%s)", f.rt(), sv)
	}
	loop := func(body func()) {
		f.line("for %s {", header)
		f.ind++
		body()
		f.ind--
		f.line("}")
	}
	switch fo.Name {
	case "map":
		ut := f.goType(want.Result)
		out := f.tmp("out")
		f.line("%s := make([]%s, 0, %s)", out, ut, length)
		loop(func() {
			v := f.tmp("r")
			f.line("var %s %s", v, ut)
			f.inlineLit(lit, ft, []string{e}, dest{kind: dAssign, name: v, typ: want.Result})
			f.line("%s = append(%s, %s)", out, out, v)
		})
		return fmt.Sprintf("%sListFromSlice(%s)", r, out), true
	case "filter":
		out := f.tmp("out")
		f.line("var %s []%s", out, et)
		loop(func() {
			v := f.tmp("r")
			f.line("var %s bool", v)
			f.inlineLit(lit, ft, []string{e}, dest{kind: dAssign, name: v, typ: want.Result})
			f.line("if %s {", v)
			// the element stays in the source list too
			for _, l := range f.pg.markStmts(e, elemT) {
				f.line("\t%s", l)
			}
			f.line("\t%s = append(%s, %s)", out, out, e)
			f.line("}")
		})
		return fmt.Sprintf("%sListFromSlice(%s)", r, out), true
	case "fold":
		acc := f.tmp("acc")
		f.line("var %s %s = %s", acc, f.goType(want.Result), init)
		loop(func() {
			f.inlineLit(lit, ft, []string{acc, e}, dest{kind: dAssign, name: acc, typ: want.Result})
		})
		return acc, true
	case "any", "all":
		res := f.tmp("res")
		hit, start := "true", "false"
		cond := "%s"
		if fo.Name == "all" {
			hit, start, cond = "false", "true", "!%s"
		}
		f.line("%s := %s", res, start)
		loop(func() {
			v := f.tmp("r")
			f.line("var %s bool", v)
			f.inlineLit(lit, ft, []string{e}, dest{kind: dAssign, name: v, typ: want.Result})
			f.line("if "+cond+" {", v)
			f.line("\t%s = %s", res, hit)
			f.line("\tbreak")
			f.line("}")
		})
		return res, true
	case "find":
		res := f.tmp("res")
		f.line("%s := %sNone[%s]()", res, r, et)
		loop(func() {
			v := f.tmp("r")
			f.line("var %s bool", v)
			f.inlineLit(lit, ft, []string{e}, dest{kind: dAssign, name: v, typ: want.Result})
			f.line("if %s {", v)
			f.line("\t%s = %sSome[%s](%s)", res, r, et, e)
			f.line("\tbreak")
			f.line("}")
		})
		return res, true
	}
	return "", false
}
