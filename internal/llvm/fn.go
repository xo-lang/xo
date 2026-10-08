package llvm

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/token"
)

// fn is the emission state of one LLVM function: a function instance, a
// closure body, or a helper.
type fn struct {
	g    *gen
	sub  *check.Subst
	safe map[ast.Node]bool

	allocas strings.Builder
	body    strings.Builder
	n       int
	term    bool   // the current block has its terminator
	cur     string // label of the current block
	// uniq maps a binding's address to the unshared list it holds, known
	// since the last listMut or push in this straight line region. It is
	// dropped at block merges, user calls, retains, and stores to the
	// binding (see clobber).
	uniq map[string]string

	vars  map[*check.VarObj]*local
	loops []*loopCtx

	res check.Type // declared result
	err check.Type // error type, nil when the function cannot fail
	ret *ty        // LLVM return type
}

// local is a binding: the address of its value.
type local struct {
	ptr string
	t   *ty
}

type loopCtx struct {
	label     string
	brk, cont string
}

// val is an LLVM operand with its lowered type.
type val struct {
	t *ty
	v string
}

func (g *gen) newFn(sub *check.Subst, safe map[ast.Node]bool) *fn {
	return &fn{g: g, sub: sub, safe: safe, vars: map[*check.VarObj]*local{}, cur: "body"}
}

// ---------------------------------------------------------------------------
// Emission primitives

func (f *fn) tmp() string {
	f.n++
	return fmt.Sprintf("%%t%d", f.n)
}

func (f *fn) label(s string) string {
	f.n++
	return fmt.Sprintf("L%d.%s", f.n, s)
}

// emit writes an instruction, opening an unreachable block first when the
// current one is terminated (code after `return`, `break`, or a fault).
func (f *fn) emit(format string, args ...any) {
	if f.term {
		f.startBlock(f.label("dead"))
	}
	f.body.WriteString("  ")
	fmt.Fprintf(&f.body, format, args...)
	f.body.WriteByte('\n')
}

// def emits an instruction with a result and returns its name.
func (f *fn) def(format string, args ...any) string {
	t := f.tmp()
	f.emit("%s = "+format, append([]any{t}, args...)...)
	return t
}

// startBlock starts block l, falling through from the current block.
func (f *fn) startBlock(l string) {
	f.uniq = nil
	f.cont(l)
}

// cont starts block l whose only predecessor is the current block (the
// continuation after a cold fault branch), keeping what is known.
func (f *fn) cont(l string) {
	if !f.term {
		f.body.WriteString("  br label %" + l + "\n")
	}
	f.body.WriteString(l + ":\n")
	f.term = false
	f.cur = l
}

// clobber forgets which list bindings are known unshared: a call may
// copy them, and a retain marks them shared.
func (f *fn) clobber() { f.uniq = nil }

// forget drops what is known about the binding at place (it was
// assigned).
func (f *fn) forget(place string) { delete(f.uniq, place) }

func (f *fn) br(l string) {
	f.emit("br label %%%s", l)
	f.term = true
}

func (f *fn) condBr(c, yes, no string) {
	f.emit("br i1 %s, label %%%s, label %%%s", c, yes, no)
	f.term = true
}

// unlikely branches to fail when c holds, marked cold.
func (f *fn) unlikely(c, fail, ok string) {
	f.emit("br i1 %s, label %%%s, label %%%s, !prof !0", c, fail, ok)
	f.term = true
}

// alloca reserves a stack slot in the entry block.
func (f *fn) alloca(t *ty) string {
	a := f.tmp()
	fmt.Fprintf(&f.allocas, "  %s = alloca %s\n", a, t.ll)
	return a
}

// spill stores a value in a fresh slot and returns its address.
func (f *fn) spill(v val) string {
	a := f.alloca(v.t)
	f.emit("store %s %s, ptr %s", v.t.ll, v.v, a)
	return a
}

func (f *fn) load(t *ty, p string) val {
	return val{t, f.def("load %s, ptr %s", t.ll, p)}
}

// faultCall emits a call of a noreturn runtime fault.
func (f *fn) faultCall(format string, args ...any) {
	f.emit("call void "+format, args...)
	f.emit("unreachable")
	f.term = true
}

// ---------------------------------------------------------------------------
// Types of expressions

// t applies the instance's type substitution.
func (f *fn) t(t check.Type) check.Type {
	if t == nil {
		return nil
	}
	if f.sub != nil {
		t = f.sub.Apply(t)
	}
	return check.Deep(t)
}

func (f *fn) ty(t check.Type, at ast.Node) *ty { return f.g.lower(f.t(t), at) }

// typeOf is the type of e in context (after implicit conversion).
func (f *fn) typeOf(e ast.Expr) check.Type {
	if t := f.g.info.TypeOf(e); t != nil {
		return f.t(t)
	}
	return f.t(f.g.info.NaturalType(e))
}

// nat is the type of e before implicit conversion.
func (f *fn) nat(e ast.Expr) check.Type {
	if t := f.g.info.NaturalType(e); t != nil {
		return f.t(t)
	}
	return f.t(f.g.info.TypeOf(e))
}

func (f *fn) unitVal() val { return val{f.g.lower(unitType, nil), "zeroinitializer"} }

// ---------------------------------------------------------------------------
// Destinations of block, if, and match values

type destKind int

const (
	dDiscard destKind = iota
	dSlot             // store into ptr, converted to t
	dRet              // return from the function
)

type dest struct {
	kind destKind
	ptr  string
	t    check.Type
}

// into evaluates e and delivers its value to d.
func (f *fn) into(e ast.Expr, d dest) {
	switch x := e.(type) {
	case *ast.ParenExpr:
		f.into(x.X, d)
		return
	case *ast.Block:
		f.block(x, d)
		return
	case *ast.IfExpr:
		f.ifExpr(x, d)
		return
	case *ast.MatchExpr:
		f.match(x, d)
		return
	case *ast.WhileExpr:
		f.loopWhile(nil, nil, x.Body)
		f.deliverUnit(d)
		return
	}
	// `{}` is an empty Map literal (core 1.6); as an arm or block body
	// whose value is unused it does nothing
	if m, ok := e.(*ast.MapLit); ok && len(m.Entries) == 0 && (d.kind == dDiscard || d.kind == dSlot && check.IsUnit(d.t)) {
		return
	}
	switch d.kind {
	case dDiscard:
		f.expr(e)
	case dSlot:
		nat := f.nat(e)
		if check.IsUnit(d.t) && !check.IsUnit(nat) {
			f.expr(e)
			return
		}
		v := f.ownAs(e, d.t)
		if !f.term {
			f.emit("store %s %s, ptr %s", v.t.ll, v.v, d.ptr)
		}
	case dRet:
		f.retExpr(e)
	}
}

// deliverUnit delivers `()` to d after a statement.
func (f *fn) deliverUnit(d dest) {
	switch d.kind {
	case dSlot:
		if !check.IsUnit(d.t) && !check.IsNever(d.t) {
			v := f.coerce(f.unitVal(), unitType, d.t, nil)
			f.emit("store %s %s, ptr %s", v.t.ll, v.v, d.ptr)
		}
	case dRet:
		f.retValue(f.unitVal(), unitType)
	}
}

// valueInto evaluates an expression needing control flow into a slot of
// its type and returns the value.
func (f *fn) valueInto(e ast.Expr) val {
	t := f.typeOf(e)
	if t == nil {
		t = unitType
	}
	lt := f.ty(t, e)
	slot := f.alloca(lt)
	f.into(e, dest{kind: dSlot, ptr: slot, t: t})
	if f.term {
		return val{lt, "poison"}
	}
	return f.load(lt, slot)
}

// ---------------------------------------------------------------------------
// Returning

// retExpr returns the value of e from the function (core 2.6: in a
// failing function a T is success, `Err(e)` failure, a Result as is).
func (f *fn) retExpr(e ast.Expr) {
	if call, ok := e.(*ast.CallExpr); ok && f.isRetCtor(call) && f.err != nil {
		id := call.Fun.(*ast.Ident)
		arg := call.Args[0].Value
		if id.Name == "Err" {
			v := f.coerceErr(f.own(arg), f.nat(arg), f.err, arg)
			f.emitRet(f.variant(f.ret, 1, []val{v}, arg))
			return
		}
		v := f.ownAs(arg, f.res)
		f.emitRet(f.variant(f.ret, 0, []val{v}, arg))
		return
	}
	nat := f.nat(e)
	if nat == nil {
		nat = unitType
	}
	if check.IsNever(nat) {
		f.expr(e)
		return
	}
	if check.IsUnit(f.res) && !check.IsUnit(nat) {
		f.expr(e)
		f.retValue(f.unitVal(), unitType)
		return
	}
	if f.err != nil {
		if _, _, ok := f.g.resultParts(nat); ok {
			if _, _, rok := f.g.resultParts(f.res); !rok {
				v := f.ownAs(e, nat)
				f.emitRet(f.coerce(v, nat, f.g.resultOf(f.res, f.err), e))
				return
			}
		}
	}
	f.retValue(f.ownAs(e, nat), nat)
}

// retValue returns v (of type from) as the function's success value.
func (f *fn) retValue(v val, from check.Type) {
	if f.term {
		return
	}
	if check.IsUnit(from) && !check.IsUnit(f.res) && f.g.optionElem(f.res) == nil {
		// the end of a body whose every path returns (or loops forever):
		// the checker proved this point unreachable
		f.emit("unreachable")
		f.term = true
		return
	}
	v = f.coerce(v, from, f.res, nil)
	if f.err != nil {
		v = f.variant(f.ret, 0, []val{v}, nil)
	}
	f.emitRet(v)
}

func (f *fn) emitRet(v val) {
	if f.term {
		return
	}
	f.emit("ret %s %s", f.ret.ll, v.v)
	f.term = true
}

// isRetCtor reports a bare `Err(x)` or `Ok(x)` that the checker typed
// against the enclosing function's Result without recording a
// constructor.
func (f *fn) isRetCtor(call *ast.CallExpr) bool {
	id, ok := call.Fun.(*ast.Ident)
	if !ok || (id.Name != "Err" && id.Name != "Ok") || len(call.Args) != 1 {
		return false
	}
	info := f.g.info
	return info.VarOf(id) == nil && info.Ref(id) == nil && info.Ctor(call) == nil && info.CallOf(call) == nil
}

// ---------------------------------------------------------------------------
// Blocks and statements

func (f *fn) block(b *ast.Block, d dest) {
	if len(b.Stmts) == 0 {
		f.deliverUnit(d)
		return
	}
	for _, s := range b.Stmts[:len(b.Stmts)-1] {
		f.stmt(s)
	}
	last := b.Stmts[len(b.Stmts)-1]
	if es, ok := last.(*ast.ExprStmt); ok {
		f.into(es.X, d)
		return
	}
	f.stmt(last)
	if d.kind == dRet {
		if _, ok := last.(*ast.ReturnStmt); ok {
			return
		}
	}
	f.deliverUnit(d)
}

func (f *fn) stmt(s ast.Stmt) {
	g := f.g
	switch x := s.(type) {
	case *ast.ExprStmt:
		f.into(x.X, dest{kind: dDiscard})
	case *ast.LetStmt:
		f.letStmt(x.Pattern, x.Value)
	case *ast.VarStmt:
		v := g.info.VarOf(x.Name)
		f.bindNew(v, x.Value)
	case *ast.ConstDecl:
		v := g.info.VarOf(x.Name)
		if v == nil {
			return
		}
		f.bindNew(v, x.Value)
	case *ast.AssignStmt:
		f.assign(x)
	case *ast.ReturnStmt:
		if x.Value == nil {
			f.retValue(f.unitVal(), unitType)
			return
		}
		f.retExpr(x.Value)
	case *ast.BreakStmt:
		l := f.findLoop(x.Label, x)
		if l != nil {
			f.br(l.brk)
		}
	case *ast.ContinueStmt:
		l := f.findLoop(x.Label, x)
		if l != nil {
			f.br(l.cont)
		}
	case *ast.WhileStmt:
		f.loopWhile(x.Label, x.Cond, x.Body)
	case *ast.ForStmt:
		f.forStmt(x)
	case *ast.DeferStmt:
		g.unsupported(x, "`defer`")
	default:
		g.unsupported(s, fmt.Sprintf("the statement %T", s))
	}
}

// bindNew declares v and stores the value of e (converted to v's type).
func (f *fn) bindNew(v *check.VarObj, e ast.Expr) {
	if v == nil {
		f.expr(e)
		return
	}
	vt := f.t(v.Type)
	val := f.ownAs(e, vt)
	l := f.declare(v, e)
	if !f.term {
		f.emit("store %s %s, ptr %s", l.t.ll, val.v, l.ptr)
	}
}

// declare gives v a stack slot.
func (f *fn) declare(v *check.VarObj, at ast.Node) *local {
	if l, ok := f.vars[v]; ok {
		return l
	}
	t := f.ty(v.Type, at)
	l := &local{ptr: f.alloca(t), t: t}
	f.vars[v] = l
	return l
}

func (f *fn) letStmt(p ast.Pattern, value ast.Expr) {
	if bp, ok := p.(*ast.BindPat); ok {
		f.bindNew(f.g.info.VarOf(bp.Name), value)
		return
	}
	if _, ok := p.(*ast.WildcardPat); ok {
		f.into(value, dest{kind: dDiscard})
		return
	}
	pt := f.g.info.PatType(p)
	if pt == nil {
		pt = f.typeOf(value)
	}
	pt = f.t(pt)
	v := f.ownAs(value, pt)
	if f.term {
		return
	}
	ptr := f.spill(v)
	fail := f.label("letfail")
	ok := f.label("let")
	f.pattern(p, ptr, v.t, fail, nil)
	f.br(ok)
	f.startBlock(fail)
	f.faultCall("@xo_fault_unreachable(ptr %s)", f.g.cstr(f.g.loc(p)))
	f.startBlock(ok)
}

func (f *fn) findLoop(label *ast.Ident, at ast.Node) *loopCtx {
	for i := len(f.loops) - 1; i >= 0; i-- {
		if label == nil || f.loops[i].label == label.Name {
			return f.loops[i]
		}
	}
	f.g.unsupported(at, "a `break` or `continue` outside a loop")
	return nil
}

func labelName(id *ast.Ident) string {
	if id == nil {
		return ""
	}
	return id.Name
}

// loopWhile emits `while cond { body }` (cond nil: an infinite loop).
func (f *fn) loopWhile(label *ast.Ident, cond ast.Expr, body *ast.Block) {
	head := f.label("while")
	bodyL := f.label("body")
	exit := f.label("wend")
	f.startBlock(head)
	if cond != nil {
		c := f.val(cond)
		f.condBr(c.v, bodyL, exit)
	} else {
		f.br(bodyL)
	}
	f.startBlock(bodyL)
	f.loops = append(f.loops, &loopCtx{label: labelName(label), brk: exit, cont: head})
	f.block(body, dest{kind: dDiscard})
	f.loops = f.loops[:len(f.loops)-1]
	if !f.term {
		f.br(head)
	}
	f.startBlock(exit)
}

// forStmt emits `for pat in iter { body }` over a range or a List.
func (f *fn) forStmt(x *ast.ForStmt) {
	g := f.g
	label := labelName(x.Label)
	if r, ok := unparen(x.Iter).(*ast.RangeExpr); ok {
		if r.Lo == nil || r.Hi == nil || !g.isPrelude(f.nat(r.Lo), "Int") {
			g.unsupported(r, "this range (only `a..b` and `a..=b` over Int)")
			return
		}
		lo := f.val(r.Lo)
		hi := f.val(r.Hi)
		intT := lo.t
		ip := f.alloca(intT)
		f.emit("store i64 %s, ptr %s", lo.v, ip)
		head := f.label("for")
		body := f.label("body")
		cont := f.label("next")
		exit := f.label("fend")
		f.startBlock(head)
		cur := f.def("load i64, ptr %s", ip)
		op := "slt"
		if r.Inclusive {
			op = "sle"
		}
		c := f.def("icmp %s i64 %s, %s", op, cur, hi.v)
		f.condBr(c, body, exit)
		f.startBlock(body)
		f.bindLoopVar(x.Pattern, val{intT, cur})
		f.loops = append(f.loops, &loopCtx{label: label, brk: exit, cont: cont})
		f.block(x.Body, dest{kind: dDiscard})
		f.loops = f.loops[:len(f.loops)-1]
		f.startBlock(cont)
		cur2 := f.def("load i64, ptr %s", ip)
		if r.Inclusive {
			// cur == hi ends the loop before cur + 1 could overflow
			last := f.def("icmp eq i64 %s, %s", cur2, hi.v)
			inc := f.label("inc")
			f.condBr(last, exit, inc)
			f.startBlock(inc)
		}
		nx := f.def("add nsw i64 %s, 1", cur2)
		f.emit("store i64 %s, ptr %s", nx, ip)
		f.br(head)
		f.startBlock(exit)
		return
	}
	// for (i, x) in xs.indexed()
	iter := x.Iter
	indexed := false
	if call, ok := unparen(iter).(*ast.CallExpr); ok {
		if ci := g.info.CallOf(call); ci != nil && ci.Fn != nil && ci.Fn.Module.IsPrelude && ci.Fn.Name == "indexed" && ci.Recv != nil && g.isPrelude(f.typeOf(ci.Recv), "List") {
			iter = ci.Recv
			indexed = true
		}
	}
	it := f.typeOf(iter)
	if !g.isPrelude(it, "List") {
		g.unsupported(x.Iter, "iteration over `"+fmt.Sprint(it)+"` (only ranges and List)")
		return
	}
	lv := f.val(iter)
	// a binding the body mutates must not change the list being iterated
	if root := f.rootVar(iter); root != nil && f.mutates(x.Body, root) {
		f.retain(lv)
	}
	et := lv.t.elem
	n := f.listLen(lv.v)
	data := f.listData(lv.v)
	intT := g.lower(g.preludeType("Int"), x)
	ip := f.alloca(intT)
	f.emit("store i64 0, ptr %s", ip)
	head := f.label("for")
	body := f.label("body")
	cont := f.label("next")
	exit := f.label("fend")
	f.startBlock(head)
	cur := f.def("load i64, ptr %s", ip)
	c := f.def("icmp slt i64 %s, %s", cur, n)
	f.condBr(c, body, exit)
	f.startBlock(body)
	ep := f.def("getelementptr %s, ptr %s, i64 %s", et.ll, data, cur)
	if indexed {
		tt := g.lower(&check.Tuple{Elems: []check.Type{intT.t, et.t}}, x)
		tp := f.alloca(tt)
		f.emit("store i64 %s, ptr %s", cur, f.def("getelementptr %s, ptr %s, i32 0, i32 0", tt.ll, tp))
		ev := f.load(et, ep)
		f.emit("store %s %s, ptr %s", et.ll, ev.v, f.def("getelementptr %s, ptr %s, i32 0, i32 1", tt.ll, tp))
		f.bindLoopPat(x.Pattern, tp, tt)
	} else {
		f.bindLoopPat(x.Pattern, ep, et)
	}
	f.loops = append(f.loops, &loopCtx{label: label, brk: exit, cont: cont})
	f.block(x.Body, dest{kind: dDiscard})
	f.loops = f.loops[:len(f.loops)-1]
	f.startBlock(cont)
	nx := f.def("add nsw i64 %s, 1", cur)
	f.emit("store i64 %s, ptr %s", nx, ip)
	f.br(head)
	f.startBlock(exit)
}

// bindLoopVar binds a range loop's pattern to the Int v.
func (f *fn) bindLoopVar(p ast.Pattern, v val) {
	switch x := p.(type) {
	case *ast.WildcardPat:
		return
	case *ast.BindPat:
		l := f.declare(f.g.info.VarOf(x.Name), x)
		f.emit("store i64 %s, ptr %s", v.v, l.ptr)
		return
	}
	f.bindLoopPat(p, f.spill(v), v.t)
}

// bindLoopPat binds an irrefutable loop pattern to the value at ptr.
func (f *fn) bindLoopPat(p ast.Pattern, ptr string, t *ty) {
	if _, ok := p.(*ast.WildcardPat); ok {
		return
	}
	fail := f.label("forfail")
	ok := f.label("forbind")
	f.pattern(p, ptr, t, fail, nil)
	f.br(ok)
	f.startBlock(fail)
	f.faultCall("@xo_fault_unreachable(ptr %s)", f.g.cstr(f.g.loc(p)))
	f.startBlock(ok)
}

func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// rootVar returns the binding at the root of a place expression.
func (f *fn) rootVar(e ast.Expr) *check.VarObj {
	switch x := unparen(e).(type) {
	case *ast.Ident:
		return f.g.info.VarOf(x)
	case *ast.SelectorExpr:
		return f.rootVar(x.X)
	case *ast.IndexExpr:
		return f.rootVar(x.X)
	}
	return nil
}

// mutates reports whether n may change binding v: an assignment to a
// place rooted at v, or v (or a place in it) passed as a `var` receiver
// or argument.
func (f *fn) mutates(n ast.Node, v *check.VarObj) bool {
	if !v.Mutable && v.Kind != check.VParam {
		return false
	}
	found := false
	check.Inspect(n, func(n ast.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *ast.AssignStmt:
			if f.rootVar(x.Target) == v {
				found = true
			}
		case *ast.CallExpr:
			ci := f.g.info.CallOf(x)
			if ci == nil {
				// a call of a function value with a `var` parameter
				for _, a := range x.Args {
					if f.rootVar(a.Value) == v {
						found = true
					}
				}
				return true
			}
			if ci.Fn != nil && ci.Fn.RecvVar && ci.Recv != nil && f.rootVar(ci.Recv) == v {
				found = true
			}
			if ci.Fn != nil {
				for i, e := range argExprs(ci.Fn.Params, x) {
					if e != nil && ci.Fn.Params[i].Var && f.rootVar(e) == v {
						found = true
					}
				}
			}
		}
		return true
	})
	return found
}

// ---------------------------------------------------------------------------
// Places (assignment targets, `var` receivers and arguments)

// TBAA access tags (see runtimeDecls): list header fields and list
// elements. Other memory is untagged (may alias anything).
const (
	tagHdr  = ", !tbaa !3"
	tagElem = ", !tbaa !5"
)

// listLen loads the length of list l.
func (f *fn) listLen(l string) string { return f.def("load i64, ptr %s"+tagHdr, l) }

// listData returns the address of the first element of list l.
func (f *fn) listData(l string) string {
	return f.def("getelementptr %%xo.list, ptr %s, i64 1", l)
}

// listMut returns the list stored at place, unshared first when it is
// shared (copy on write), storing the copy back.
func (f *fn) listMut(place string, et *ty, at ast.Node) string {
	if l, ok := f.uniq[place]; ok && !f.term {
		return l
	}
	l := f.def("load ptr, ptr %s", place)
	shp := f.def("getelementptr %%xo.list, ptr %s, i32 0, i32 2", l)
	sh := f.def("load i64, ptr %s"+tagHdr, shp)
	c := f.def("icmp ne i64 %s, 0", sh)
	from := f.cur
	slow, done := f.label("unshare"), f.label("unique")
	f.unlikely(c, slow, done)
	f.startBlock(slow)
	l2 := f.def("call ptr @xo_list_unshare(ptr %s, i64 %d, ptr %s)", l, et.size, f.g.retainFn(et, at))
	f.emit("store ptr %s, ptr %s", l2, place)
	f.br(done)
	known := f.uniq
	f.startBlock(done)
	f.uniq = known
	lm := f.def("phi ptr [ %s, %%%s ], [ %s, %%%s ]", l, from, l2, slow)
	f.know(place, lm)
	return lm
}

func (f *fn) know(place, l string) {
	if f.uniq == nil {
		f.uniq = map[string]string{}
	}
	f.uniq[place] = l
}

// listPush appends a slot to the list stored at place and returns its
// address (the caller stores the element there).
func (f *fn) listPush(place string, et *ty, at ast.Node) string {
	l := f.def("load ptr, ptr %s", place)
	n := f.listLen(l)
	capp := f.def("getelementptr %%xo.list, ptr %s, i32 0, i32 1", l)
	cp := f.def("load i64, ptr %s"+tagHdr, capp)
	shp := f.def("getelementptr %%xo.list, ptr %s, i32 0, i32 2", l)
	sh := f.def("load i64, ptr %s"+tagHdr, shp)
	full := f.def("icmp eq i64 %s, %s", n, cp)
	shared := f.def("icmp ne i64 %s, 0", sh)
	bad := f.def("or i1 %s, %s", full, shared)
	from := f.cur
	slow, done := f.label("grow"), f.label("room")
	f.unlikely(bad, slow, done)
	f.startBlock(slow)
	l2 := f.def("call ptr @xo_list_grow(ptr %s, i64 %d, ptr %s)", l, et.size, f.g.retainFn(et, at))
	f.emit("store ptr %s, ptr %s", l2, place)
	f.br(done)
	known := f.uniq
	f.startBlock(done)
	f.uniq = known
	lm := f.def("phi ptr [ %s, %%%s ], [ %s, %%%s ]", l, from, l2, slow)
	f.know(place, lm)
	slot := f.def("getelementptr %s, ptr %s, i64 %s", et.ll, f.listData(lm), n)
	n1 := f.def("add nuw nsw i64 %s, 1", n)
	f.emit("store i64 %s, ptr %s"+tagHdr, n1, lm)
	return slot
}

// place returns the address of a place expression, making list buffers
// on the path unshared so the write does not reach other values, and the
// TBAA tag of that memory ("" or tagElem).
func (f *fn) place(e ast.Expr) (string, *ty, string) {
	g := f.g
	switch x := unparen(e).(type) {
	case *ast.Ident:
		v := g.info.VarOf(x)
		if v == nil {
			break
		}
		l, ok := f.vars[v]
		if !ok {
			g.unsupported(x, "a binding outside the function (`"+x.Name+"`)")
			return f.alloca(f.ty(v.Type, x)), f.ty(v.Type, x), ""
		}
		return l.ptr, l.t, ""
	case *ast.SelectorExpr:
		bp, bt, tag := f.place(x.X)
		switch bt.k {
		case kStruct, kTuple:
			i := bt.fieldIndex(x.Sel.Name)
			if i < 0 {
				break
			}
			fl := bt.fields[i]
			p := f.def("getelementptr %s, ptr %s, i32 0, i32 %d", bt.ll, bp, i)
			ft := g.fieldTy(fl, x)
			if fl.boxed {
				// copy on write: the box may be shared with other values
				old := f.def("load ptr, ptr %s", p)
				v := f.load(ft, old)
				f.retain(v)
				nb := f.def("call ptr @xo_alloc(i64 %d)", max(ft.size, 1))
				f.emit("store %s %s, ptr %s", ft.ll, v.v, nb)
				f.emit("store ptr %s, ptr %s", nb, p)
				return nb, ft, ""
			}
			return p, ft, tag
		}
	case *ast.IndexExpr:
		if _, ok := x.Index.(*ast.RangeExpr); ok {
			break
		}
		bp, bt, _ := f.place(x.X)
		if bt.k != kList {
			break
		}
		idx := f.val(x.Index)
		l := f.listMut(bp, bt.elem, x)
		f.boundsCheck(l, idx.v, x)
		return f.def("getelementptr %s, ptr %s, i64 %s", bt.elem.ll, f.listData(l), idx.v), bt.elem, tagElem
	}
	g.unsupported(e, "this assignment target or `var` argument")
	t := f.ty(f.typeOf(e), e)
	return f.alloca(t), t, ""
}

// boundsCheck faults unless 0 <= i < len(l), unless proven.
func (f *fn) boundsCheck(l, i string, at *ast.IndexExpr) {
	if f.safe[at] {
		return
	}
	n := f.listLen(l)
	bad := f.def("icmp uge i64 %s, %s", i, n)
	fail := f.label("oob")
	ok := f.label("inb")
	f.unlikely(bad, fail, ok)
	known := f.uniq
	f.startBlock(fail)
	f.faultCall("@xo_fault_index(i64 %s, i64 %s, ptr %s)", i, n, f.g.cstr(f.g.loc(at)))
	f.uniq = known
	f.cont(ok)
}

func (f *fn) assign(x *ast.AssignStmt) {
	g := f.g
	if x.Op == token.Assign {
		if ix, ok := unparen(x.Target).(*ast.IndexExpr); ok && !g.isPrelude(f.typeOf(ix.X), "List") {
			g.unsupported(x, "assignment through `m[k]` (only List elements)")
			return
		}
		tt := f.typeOf(x.Target)
		v := f.ownAs(x.Value, tt)
		if f.term {
			return
		}
		p, pt, tag := f.place(x.Target)
		f.emit("store %s %s, ptr %s"+tag, pt.ll, v.v, p)
		f.forget(p)
		return
	}
	op := map[token.Kind]string{token.PlusAssign: "+", token.MinusAssign: "-", token.StarAssign: "*", token.SlashAssign: "/", token.PercentAssign: "%"}[x.Op]
	if op == "" {
		g.unsupported(x, "this compound assignment")
		return
	}
	tt := f.typeOf(x.Target)
	r := f.valAs(x.Value, tt)
	if f.term {
		return
	}
	p, pt, tag := f.place(x.Target)
	old := val{pt, f.def("load %s, ptr %s"+tag, pt.ll, p)}
	nv := f.arith(op, old, r, x, tt)
	if !f.term {
		f.emit("store %s %s, ptr %s"+tag, pt.ll, nv.v, p)
		f.forget(p)
	}
}

// argExprs matches call arguments to parameters: positional first, then
// named. Missing ones are nil (their default is used).
func argExprs(params []*check.ParamInfo, call *ast.CallExpr) []ast.Expr {
	out := make([]ast.Expr, len(params))
	pos := 0
	for _, a := range call.Args {
		if a.Name == nil {
			if pos < len(out) {
				out[pos] = a.Value
			}
			pos++
			continue
		}
		for i, p := range params {
			if p.Name == a.Name.Name {
				out[i] = a.Value
			}
		}
	}
	return out
}
