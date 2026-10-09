package ssa

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Contracts (core 9.1), checked where and as the Go backend checks them
// (internal/gen fn.go, decl.go):
//
//   - `requires` at the start of the body: without `else` a false
//     condition faults (`requires failed: <source>`), with it the function
//     returns Err(else value).
//   - `ensures` after the body, against `result` (the success value) and
//     the `old(...)` values taken after the `requires` clauses; a function
//     that can fail checks them only when it succeeds.
//   - A struct invariant after a struct literal (defaults included), after
//     `with`, and after a `var` method of the type (on success only),
//     faulting with `invariant of <Type> failed: <source>` at the literal,
//     the `with`, or the method's name.
//
// A function with `ensures` clauses, or a `var` method of a type with an
// invariant, becomes a wrapper around its body (the function `<name>.body`
// with the same parameters and results): the wrapper checks `requires`,
// takes the `old(...)` values, calls the body, and checks the rest, so
// every way out of the body (return, `?`, the end) is checked once.

// contractWrapped reports whether fo's body is wrapped (ensures, or the
// invariant of a `var` method's receiver).
func (b *builder) contractWrapped(fo *check.FuncObj) (ens []*ast.EnsuresClause, inv bool) {
	if fo.Decl == nil {
		return nil, false
	}
	for _, cl := range fo.Decl.Clauses {
		if c, ok := cl.(*ast.EnsuresClause); ok {
			ens = append(ens, c)
		}
	}
	inv = fo.RecvVar && fo.Recv != nil && fo.Recv.Invariant != nil && !fo.Recv.Module.IsStd
	return ens, inv
}

// contractBody builds f as the wrapper of fo's body (see above); the body
// itself is queued as a function of its own.
func (b *builder) contractBody(fo *check.FuncObj, f *Func, sub *check.Subst, ens []*ast.EnsuresClause, inv bool) {
	var params []*Type
	for _, p := range f.Params() {
		params = append(params, p.Type)
	}
	inner := b.mod.NewFunc(f.Name+".body", params, f.Results)
	inner.VarParams = f.VarParams
	b.queue = append(b.queue, func() { b.bodyOf(fo, inner, sub, false) })

	s := b.newFn(f, nil, nil)
	s.fo = fo
	s.sub = sub
	s.res, s.errT = s.apply(fo.Result), failErr(s.apply(fo.Err))
	vars := s.paramBindings(fo, f)
	for _, cl := range fo.Decl.Clauses {
		if c, ok := cl.(*ast.RequiresClause); ok {
			s.requires(c)
		}
	}
	// old(...) values, in clause order
	s.olds = map[*ast.CallExpr]*Value{}
	for _, c := range ens {
		check.Inspect(c.Cond, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if isOldCall(b, call) && len(call.Args) == 1 {
				s.olds[call] = s.value(call.Args[0].Value)
				return false
			}
			return true
		})
	}
	var args []*Value
	for _, vo := range vars {
		args = append(args, s.read(vo))
	}
	var r, main *Value
	if len(f.Results) == 1 {
		r = s.call(inner.Name, f.Results[0], "", args...)
		s.check(inner.Name)
		main = r
	} else {
		t := b.mod.Tuple(f.Results...)
		r = s.call(inner.Name, t, "", args...)
		s.check(inner.Name)
		main = s.add(OpField, f.Results[0], 0, r)
		for i, vo := range s.varParams {
			s.writeVar(vo, s.add(OpField, f.Results[i+1], i+1, r))
		}
	}
	// the success value, and where the checks run
	join := s.newBlock()
	var success *Value
	if s.errT != nil {
		rt := f.Results[0]
		okI := rt.VariantIndex("Ok")
		isOk := s.add(OpEq, b.mod.Builtin(KBool), nil, s.add(OpTag, b.mod.Builtin(KInt), nil, main), s.cint(int64(okI)))
		yes, no := s.branch(isOk)
		s.cur = no
		s.jump(join)
		s.cur = yes
		success = s.add(OpPayload, rt.Variants[okI].Fields[0].Type, PayloadRef{okI, 0}, main)
	} else {
		success = main
	}
	for _, c := range ens {
		if rv := b.info.ResultVar(c); rv != nil {
			s.writeVar(rv, success)
		}
		s.contractCheck(c.Cond, "ensures failed: ")
	}
	if inv && len(vars) > 0 && !s.isDead() {
		recv := s.read(vars[0])
		s.checkInvariant(recv, s.apply(fo.RecvType), fo.Decl.Name)
	}
	s.olds = nil
	if !s.isDead() {
		s.jump(join)
	}
	s.seal(join)
	s.cur = join
	rets := []*Value{main}
	for _, vo := range s.varParams {
		rets = append(rets, s.read(vo))
	}
	s.add(OpReturn, nil, nil, rets...)
	s.dead()
	s.finishFn()
}

// paramBindings binds fo's receiver and parameters to f's parameters and
// records the `var` parameters; it returns the bindings in order.
func (s *fn) paramBindings(fo *check.FuncObj, f *Func) []*check.VarObj {
	b := s.b
	var vars []*check.VarObj
	if fo.Recv != nil {
		vars = append(vars, b.info.RecvVar(fo))
	}
	vars = append(vars, b.info.ParamVars(fo)...)
	for i, vo := range vars {
		if vo != nil && i < len(f.Params()) {
			s.writeVar(vo, f.Params()[i])
		}
	}
	for _, i := range f.VarParams {
		s.varParams = append(s.varParams, vars[i])
	}
	return vars
}

// isOldCall reports whether call is `old(x)` in an ensures clause.
func isOldCall(b *builder, call *ast.CallExpr) bool {
	id, ok := call.Fun.(*ast.Ident)
	return ok && id.Name == "old" && b.info.VarOf(id) == nil
}

// contractCheck faults with prefix and cond's source text when cond is
// false, with the values of the names in cond on the next line (as std/rt's
// Require and Ensure print them).
func (s *fn) contractCheck(cond ast.Expr, prefix string) {
	b := s.b
	boolT := b.mod.Builtin(KBool)
	ok := s.valueAs(cond, boolT)
	fail, good := s.branch(s.add(OpNot, boolT, nil, ok))
	s.cur = fail
	s.faultDetail(cond)
	s.fault(s.add(OpConst, b.mod.Builtin(KStr), prefix+b.sourceText(cond)), b.pos(cond))
	s.cur = good
}

// faultDetail hands the runtime the line printed under a failed contract:
// each name cond reads (outside calls) and its Debug text.
func (s *fn) faultDetail(cond ast.Expr) {
	b := s.b
	st := b.mod.Builtin(KStr)
	var acc *Value
	seen := map[string]bool{}
	check.Inspect(cond, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			return false
		case *ast.Ident:
			vo := b.info.VarOf(x)
			if vo == nil || seen[x.Name] {
				return true
			}
			if _, known := s.varType[vo]; !known {
				return true
			}
			seen[x.Name] = true
			if acc == nil {
				acc = s.add(OpConst, st, " ")
			}
			acc = s.call("str.concat", st, "", acc, s.add(OpConst, st, "  "+x.Name+" = "))
			acc = s.appendDebug(acc, s.read(vo), s.checkType(x), x)
		}
		return true
	})
	if acc != nil {
		s.rt("fault_detail", nil, acc)
	}
}

// checkInvariant faults at `at` when the struct value v (checker type ct)
// breaks its type's invariant.
func (s *fn) checkInvariant(v *Value, ct check.Type, at ast.Node) {
	b := s.b
	n, ok := check.Resolve(check.Deep(ct)).(*check.Named)
	if !ok || n.Obj.Invariant == nil || v.Type.Kind != KStruct {
		return
	}
	boolT := b.mod.Builtin(KBool)
	inv := b.invariantFunc(n, v.Type, at)
	holds := s.call(inv, boolT, "", v)
	s.check(inv)
	fail, good := s.branch(s.add(OpNot, boolT, nil, holds))
	s.cur = fail
	s.fault(s.add(OpConst, b.mod.Builtin(KStr), "invariant of "+n.Obj.Name+" failed: "+b.sourceText(n.Obj.Invariant)), b.pos(at))
	s.cur = good
}

// invariantFunc returns the function that evaluates the invariant of the
// struct type n (lowered to t) on a value: Bool.
func (b *builder) invariantFunc(n *check.Named, t *Type, at ast.Node) string {
	name := "xo.invariant[" + t.Name + "]"
	if b.mod.Func(name) != nil {
		return name
	}
	boolT := b.mod.Builtin(KBool)
	f := b.mod.NewFunc(name, []*Type{t}, []*Type{boolT})
	s := b.newFn(f, nil, nil)
	if len(n.Obj.TParams) > 0 {
		s.sub = check.NewSubst(n.Obj.TParams, n.Args)
	}
	v := f.Params()[0]
	for i, vo := range b.info.InvariantVars(n.Obj) {
		if vo == nil || i >= len(t.Fields) {
			continue
		}
		s.writeVar(vo, s.unbox(s.add(OpField, t.Fields[i].Type, i, v)))
	}
	s.retv(s.valueAs(n.Obj.Invariant, boolT))
	s.finishFn()
	return name
}
