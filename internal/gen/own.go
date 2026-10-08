package gen

// List ownership (std/rt/list.go, bench/results/perf-list-2026-10-08.md).
//
// A List writes its elements in place when its backing array's block is
// not marked shared. The runtime marks blocks it hands out twice; the
// generated code marks the blocks a value holds wherever it copies the
// value out of a place into something that outlives the read ("own"):
//
//   - Every read of a binding, field, or element (Ident, SelectorExpr,
//     IndexExpr) whose type holds lists is wrapped in an own mark, unless
//     the expression that reads it asked for a borrow (borrow, below).
//     So a new kind of use is safe by default: forgetting a borrow costs a
//     copy on the next write, never value semantics.
//   - Results of std calls that hold lists are marked (std code may hand
//     out a value it also keeps: Map.get, Once.get, Mutex.with, dbg),
//     except the producers that return fresh lists (freshStd).
//   - A closure marks the lists of every binding it captures; a `var`
//     place passed through a temporary (place, rmw) is marked; a value
//     converted to an interface is marked (its type no longer says it
//     holds lists).
//
// A borrow is a read whose copy dies before the place can be written
// again: the base of a field or element read, the receiver of a read only
// List method, an operand of a comparison or `+`, a loop subject the body
// does not write, an argument of a user function call without `var`
// parameters. The reader checks that the other operands evaluated while
// the borrowed copy is alive do not write the place's root binding
// (mutates); `var` parameters may alias each other, so a write through
// one counts as a write through all.

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// holdsList reports whether values of t may hold a List inline (not
// through a Map, Set, enum payload, interface, Box, or handle, whose
// contents are never written in place). Type parameters may.
func (g *gen) holdsList(t check.Type) bool {
	return g.holdsList1(t, map[*check.TypeObj]bool{})
}

func (g *gen) holdsList1(t check.Type, seen map[*check.TypeObj]bool) bool {
	switch x := check.Resolve(t).(type) {
	case *check.ParamType:
		return true
	case *check.Tuple:
		for _, e := range x.Elems {
			if g.holdsList1(e, seen) {
				return true
			}
		}
	case *check.Named:
		o := x.Obj
		if o.Module.IsPrelude {
			switch o.Name {
			case "List":
				return true
			case "Option", "Result":
				for _, a := range x.Args {
					if g.holdsList1(a, seen) {
						return true
					}
				}
				return false
			}
		}
		if seen[o] {
			return false
		}
		switch o.Kind {
		case check.KStruct:
			seen[o] = true
			defer delete(seen, o)
			s := check.NewSubst(o.TParams, x.Args)
			for _, fo := range o.Fields {
				if g.isBoxedField(o, fo) {
					continue
				}
				if g.holdsList1(s.Apply(fo.Type), seen) {
					return true
				}
			}
		case check.KNewtype:
			seen[o] = true
			defer delete(seen, o)
			return g.holdsList1(check.NewSubst(o.TParams, x.Args).Apply(o.Target), seen)
		}
	}
	return false
}

// typeHoldsList is holdsList for a type as seen from the code being
// generated (the substitution of a specialized copy applied).
func (f *fnGen) typeHoldsList(t check.Type) bool {
	if t == nil {
		return false
	}
	return f.g.holdsList(f.pg.monoType(t))
}

// markStmts are statements that mark the lists held by the addressable Go
// expression addr of type t.
func (pg *pkgGen) markStmts(addr string, t check.Type) []string {
	t = pg.monoType(t)
	if !pg.g.holdsList(t) {
		return nil
	}
	switch x := check.Resolve(t).(type) {
	case *check.ParamType:
		return []string{fmt.Sprintf("%sMarkP(&%s)", pg.rt(), addr)}
	case *check.Tuple:
		var out []string
		for i, e := range x.Elems {
			out = append(out, pg.markStmts(fmt.Sprintf("%s.F%d", addr, i), e)...)
		}
		return out
	case *check.Named:
		if x.Obj.Module.IsPrelude {
			switch x.Obj.Name {
			case "List":
				return []string{addr + ".XoMark()"}
			case "Option":
				in := pg.markStmts(addr+".V", x.Args[0])
				return wrapIf(addr+".Some", in, nil)
			case "Result":
				return wrapIf(addr+".Ok", pg.markStmts(addr+".V", x.Args[0]), pg.markStmts(addr+".E", x.Args[1]))
			}
		}
		// a struct or newtype: its generated XoMark (markMethod)
		return []string{addr + ".XoMark()"}
	}
	return nil
}

func wrapIf(cond string, then, els []string) []string {
	if len(then) == 0 && len(els) == 0 {
		return nil
	}
	out := []string{"if " + cond + " {"}
	out = append(out, then...)
	if len(els) > 0 {
		out = append(out, "} else {")
		out = append(out, els...)
	}
	return append(out, "}")
}

// ownCode is code (a Go expression of Xo type t) wrapped so that it marks
// the lists the value holds and evaluates to the value.
func (f *fnGen) ownCode(code string, t check.Type) string {
	if !f.typeHoldsList(t) {
		return code
	}
	t = f.pg.monoType(t)
	if f.g.isPrelude(t, "List") {
		return "(" + code + ").Own()"
	}
	if containsParam(t) {
		return fmt.Sprintf("%sOwn(%s)", f.rt(), code)
	}
	gt := f.goType(t)
	stmts := f.pg.markStmts("v", t)
	return fmt.Sprintf("func(v %s) %s { %s; return v }(%s)", gt, gt, strings.Join(stmts, "; "), code)
}

// markMethod emits `func (v *T) XoMark()` for a struct or newtype whose
// fields hold lists, so MarkP and markStmts reach them.
func (pg *pkgGen) markMethod(self string, fields []fieldRef, types []check.Type) {
	var body []string
	for i, fr := range fields {
		if types[i] == nil {
			continue
		}
		body = append(body, pg.markStmts(fr.code, types[i])...)
	}
	if len(body) == 0 {
		return
	}
	pg.p("// XoMark marks the lists held by the fields (see rt.List).")
	pg.p("func (v *%s) XoMark() {", self)
	for _, l := range body {
		pg.p("\t%s", l)
	}
	pg.p("}\n")
}

// ---------------------------------------------------------------------------
// Borrows

// borrow is val(e) without an own mark on e itself (a borrow, see the top
// of this file). The caller has checked that the place cannot be written
// while the copy is alive (borrowOK).
func (f *fnGen) borrow(e ast.Expr) string {
	f.noteBorrow(e)
	return f.val(e)
}

// borrowAs is valAs(e, want) as a borrow.
func (f *fnGen) borrowAs(e ast.Expr, want check.Type) string {
	f.noteBorrow(e)
	return f.valAs(e, want)
}

func (f *fnGen) noteBorrow(e ast.Expr) {
	e = unparenExpr(e)
	switch e.(type) {
	case *ast.Ident, *ast.SelectorExpr, *ast.IndexExpr:
		if f.borrowed == nil {
			f.borrowed = map[ast.Expr]bool{}
		}
		f.borrowed[e] = true
	}
}

// ownRead finishes the read of a place: code unchanged for a borrow,
// else wrapped in an own mark when the value holds lists.
func (f *fnGen) ownRead(e ast.Expr, code string) string {
	if f.borrowed[e] {
		delete(f.borrowed, e)
		return code
	}
	return f.ownCode(code, f.nat(e))
}

// borrowOK reports whether e may be borrowed while the nodes in others
// are evaluated (or run, for a callee): none of them writes the binding
// e's place is rooted at.
func (f *fnGen) borrowOK(e ast.Expr, others ...ast.Node) bool {
	root := f.rootVar(e)
	if root == nil {
		return true
	}
	for _, n := range others {
		if n != nil && f.mutates(n, root) {
			return false
		}
	}
	return true
}

func unparenExpr(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// rootVar is the binding a place expression is rooted at, or nil.
func (f *fnGen) rootVar(e ast.Expr) *check.VarObj {
	switch x := unparenExpr(e).(type) {
	case *ast.Ident:
		return f.info.VarOf(x)
	case *ast.SelectorExpr:
		return f.rootVar(x.X)
	case *ast.IndexExpr:
		return f.rootVar(x.X)
	}
	return nil
}

// varAlias reports a `var` parameter or receiver: it may be the same
// caller binding as another one.
func varAlias(v *check.VarObj) bool {
	return v != nil && v.Mutable && (v.Kind == check.VParam || v.Kind == check.VRecv)
}

// mutates reports whether evaluating n may write binding v: an assignment
// to a place rooted at v, or v (or a place in it) passed as a `var`
// receiver or argument (of a call whose callee is unknown, any argument).
// For a `var` parameter v, a write through any `var` parameter counts.
func (f *fnGen) mutates(n ast.Node, v *check.VarObj) bool {
	hit := func(e ast.Expr) bool {
		r := f.rootVar(e)
		return r != nil && (r == v || varAlias(v) && varAlias(r))
	}
	found := false
	inspectNode(n, func(n ast.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *ast.AssignStmt:
			if hit(x.Target) {
				found = true
			}
		case *ast.CallExpr:
			ci := f.info.CallOf(x)
			if ci == nil || ci.Fn == nil {
				for _, a := range x.Args {
					if hit(a.Value) {
						found = true
					}
				}
				return !found
			}
			if ci.Fn.RecvVar && ci.Recv != nil && hit(ci.Recv) {
				found = true
			}
			for i, e := range argExprs(ci.Fn.Params, x) {
				if e != nil && ci.Fn.Params[i].Var && hit(e) {
					found = true
				}
			}
		}
		return !found
	})
	return found
}

// borrowRecv lists the read only List methods whose receiver is borrowed:
// the runtime marks whatever it copies out of the receiver (std/rt/list.go).
var borrowRecv = map[string]bool{
	"len": true, "is_empty": true, "get": true, "first": true, "last": true,
	"contains": true, "index_of": true, "indexed": true, "map": true,
	"filter": true, "fold": true, "find": true, "any": true, "all": true,
	"sort": true, "sort_by": true, "reverse": true, "to_set": true, "join": true,
}

// freshStd lists std methods whose results hold only fresh lists or lists
// the runtime marked (std/rt markElems), so the result is not marked.
var freshStd = map[string]map[string]bool{
	"List":  {"map": true, "filter": true, "sort": true, "sort_by": true, "reverse": true, "indexed": true},
	"Map":   {"keys": true, "values": true, "entries": true},
	"Set":   {"to_list": true},
	"Range": {"to_list": true},
	"Str":   {"split": true, "lines": true, "runes": true},
	"Buf":   {"freeze": true},
}

// stdResultFresh reports whether the result of std function fo needs no
// own mark.
func stdResultFresh(fo *check.FuncObj) bool {
	if fo.Recv == nil || !fo.Recv.Module.IsPrelude {
		return false
	}
	return freshStd[fo.Recv.Name][fo.Name]
}

// ownStdResult marks the lists held by the result of a std call (code of
// kind kind), unless fo returns fresh lists.
func (f *fnGen) ownStdResult(fo *check.FuncObj, sub *check.Subst, code string, kind callKind) (string, callKind) {
	if kind == ckVoid || stdResultFresh(fo) {
		return code, kind
	}
	rt := fo.Result
	if sub != nil {
		rt = sub.Apply(rt)
	}
	if returnsResult(fo) {
		et := fo.Err
		if sub != nil {
			et = sub.Apply(et)
		}
		if !check.IsNever(check.Deep(et)) {
			// the Go value is rt.Result[T, E]
			if !f.typeHoldsList(rt) && !f.typeHoldsList(et) {
				return code, kind
			}
			return f.ownResult(code, rt, et), kind
		}
	}
	return f.ownCode(code, rt), kind
}

// ownResult is ownCode for a Go rt.Result[T, E] value.
func (f *fnGen) ownResult(code string, t, e check.Type) string {
	t, e = f.pg.monoType(t), f.pg.monoType(e)
	if containsParam(t) || containsParam(e) {
		return fmt.Sprintf("%sOwn(%s)", f.rt(), code)
	}
	gt := fmt.Sprintf("%sResult[%s, %s]", f.rt(), f.goType(t), f.goType(e))
	stmts := wrapIf("v.Ok", f.pg.markStmts("v.V", t), f.pg.markStmts("v.E", e))
	return fmt.Sprintf("func(v %s) %s { %s; return v }(%s)", gt, gt, strings.Join(stmts, "; "), code)
}

// inlinePush lowers `p.push(x)` on a List place that is a binding or a
// field path to the inline push form (rt/list.go), which keeps a local
// list in registers.
func (f *fnGen) inlinePush(x *ast.CallExpr, ci *check.Call) (string, bool) {
	fo := ci.Fn
	if fo.Name != "push" || fo.Recv == nil || !fo.Recv.Module.IsPrelude || fo.Recv.Name != "List" || ci.Recv == nil || len(x.Args) != 1 || x.Args[0].Name != nil {
		return "", false
	}
	lt := f.g.named(f.typeOf(ci.Recv))
	if lt == nil || len(lt.Args) != 1 {
		return "", false
	}
	if !f.fieldPath(ci.Recv) {
		// an element or a box on the path: the general call with its
		// write back
		return "", false
	}
	r := f.readPlace(f.resolvePlace(ci.Recv))
	v := f.valAs(x.Args[0].Value, lt.Args[0])
	if !trivialCode(v) {
		t := f.tmp("pv")
		f.line("%s := %s", t, v)
		v = t
	}
	f.line("if %s.CanPush() {", r)
	f.line("	%s = %s.PushFast(%s)", r, r, v)
	f.line("} else {")
	f.line("	%s = %s.PushSlow(%s)", r, r, v)
	f.line("}")
	return f.rt() + "Unit{}", true
}

// fieldPath reports a place made of a binding and struct or tuple fields
// only (resolving it emits nothing).
func (f *fnGen) fieldPath(e ast.Expr) bool {
	switch x := unparenExpr(e).(type) {
	case *ast.Ident:
		return f.info.VarOf(x) != nil
	case *ast.SelectorExpr:
		return !f.g.boxedFieldNamed(f.typeOf(x.X), x.Sel.Name) && f.fieldPath(x.X)
	}
	return false
}
