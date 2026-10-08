package ssa

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

func (s *fn) callExpr(x *ast.CallExpr) *Value {
	b := s.b
	info := b.info
	if vo := info.Ctor(x); vo != nil {
		return s.ctorCall(vo, x)
	}
	if vo := info.Ctor(x.Fun); vo != nil {
		return s.ctorCall(vo, x)
	}
	if s.isRetCtor(x) {
		return s.retCtor(x)
	}
	if obj := info.TypeCall(x); obj != nil {
		return s.typeCall(obj, x)
	}
	ci := info.CallOf(x)
	if ci == nil {
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok && len(x.Args) == 1 && (sel.Sel.Name == "min" || sel.Sel.Name == "max") {
			a := s.expr(sel.X)
			if a.Type.Kind == KInt || a.Type.Kind == KFloat {
				return s.call(a.Type.Name+"."+sel.Sel.Name, a.Type, "", a, s.valueAs(x.Args[0].Value, a.Type))
			}
		}
		b.unsupported(x, "this call")
		return s.undef(s.natOf(x))
	}
	if ci.Fn == nil {
		return s.valueCall(x)
	}
	if ci.Fn.Module.IsStd {
		return s.stdCall(x, ci)
	}
	return s.declCall(x, ci)
}

// valueCall calls a function value (a closure).
func (s *fn) valueCall(x *ast.CallExpr) *Value {
	fv := s.expr(x.Fun)
	ft := fv.Type
	if ft.Kind != KFunc || len(x.Args) != len(ft.Elems) {
		s.b.unsupported(x, "this call of a function value")
		return s.undef(s.natOf(x))
	}
	args := []*Value{fv}
	for i, a := range x.Args {
		if a.Name != nil {
			s.b.unsupported(a, "a named argument to a function value")
		}
		args = append(args, s.valueAs(a.Value, ft.Elems[i]))
	}
	r := s.add(OpCallValue, ft.Ret, nil, args...)
	return s.callResult(r, x)
}

// closure lowers a function literal: its body becomes an Indirect
// function whose environment holds the captured bindings, read here by
// value (the closure sees their values at creation).
func (s *fn) closure(x *ast.FnLit) *Value {
	b := s.b
	t := s.natOf(x)
	ct, ok := check.Resolve(s.natCheck(x)).(*check.Func)
	if !ok || t.Kind != KFunc {
		b.unsupported(x, "this closure")
		return s.undef(t)
	}
	// captured bindings: used inside, bound outside, in first use order
	var caps []*check.VarObj
	seen := map[*check.VarObj]bool{}
	check.Inspect(x.Body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			if vo := b.info.VarOf(id); vo != nil && !seen[vo] {
				if _, outer := s.varType[vo]; outer {
					seen[vo] = true
					caps = append(caps, vo)
				}
			}
		}
		return true
	})
	var vals []*Value
	var capTs []*Type
	for _, vo := range caps {
		v := s.read(vo)
		vals = append(vals, v)
		capTs = append(capTs, v.Type)
	}
	b.nclos++
	name := s.f.Name + ".fn" + itoa(b.nclos)
	f := b.mod.NewFunc(name, append([]*Type{t}, t.Elems...), []*Type{t.Ret})
	f.Indirect = true
	f.Captures = capTs
	errT := failErr(ct.Err)
	b.queue = append(b.queue, func() {
		c := b.newFn(f, ct.Result, errT)
		c.sub = s.sub
		env := f.Params()[0]
		for i, vo := range caps {
			c.writeVar(vo, c.add(OpCapture, capTs[i], i, env))
		}
		for i, vo := range b.info.ClosureParams(x) {
			if vo != nil && i+1 < len(f.Params()) {
				c.writeVar(vo, f.Params()[i+1])
			}
		}
		v := c.block(x.Body)
		c.ret(v, x.Body)
	})
	return s.add(OpClosure, t, name, vals...)
}

// funcValue is a named function used as a value: a closure without an
// environment over a wrapper that calls it.
func (s *fn) funcValue(fo *check.FuncObj, at ast.Node) *Value {
	b := s.b
	t := s.natOf(at.(ast.Expr))
	if t.Kind != KFunc || fo.Recv != nil {
		b.unsupported(at, "the function `"+fo.DisplayName()+"` as a value")
		return s.undef(t)
	}
	var targs []check.Type
	var sargs []*Type
	// the type arguments: from the checker (methods), or by matching the
	// signature against the function type the value has here
	bound := map[*check.TParam]check.Type{}
	if ft, ok := check.Resolve(s.natCheck(at.(ast.Expr))).(*check.Func); ok && len(ft.Params) == len(fo.Params) {
		for i, p := range fo.Params {
			matchTParams(p.Type, ft.Params[i].T, bound)
		}
		matchTParams(fo.Result, ft.Result, bound)
	}
	for _, p := range typeParams(fo) {
		ta := b.info.RefSubst(at.(ast.Expr)).Type(p)
		if ta == nil {
			ta = bound[p]
		}
		if ta == nil {
			b.unsupported(at, "the generic function `"+fo.DisplayName()+"` as a value without type arguments")
			return s.undef(t)
		}
		targs = append(targs, s.apply(ta))
		sargs = append(sargs, b.typ(s.apply(ta), at))
	}
	_, key := b.instKey(fo, sargs)
	w := b.thunks[key]
	if w == nil {
		callee := b.declare(fo, targs, at)
		if len(callee.VarParams) > 0 {
			b.unsupported(at, "a function with `var` parameters as a value")
			return s.undef(t)
		}
		w = b.mod.NewFunc(callee.Name+".value", append([]*Type{t}, t.Elems...), []*Type{t.Ret})
		w.Indirect = true
		e := w.Blocks[0]
		r := e.Add(OpCall, callee.Results[0], callee.Name, w.Params()[1:]...)
		e.Add(OpReturn, nil, nil, r)
		b.thunks[key] = w
	}
	return s.add(OpClosure, t, w.Name)
}

// matchTParams binds the type parameters in pattern (a signature type)
// to the corresponding parts of actual.
func matchTParams(pattern, actual check.Type, bound map[*check.TParam]check.Type) {
	if pattern == nil || actual == nil {
		return
	}
	actual = check.Deep(actual)
	switch p := pattern.(type) {
	case *check.ParamType:
		if bound[p.P] == nil {
			bound[p.P] = actual
		}
	case *check.Named:
		if a, ok := check.Resolve(actual).(*check.Named); ok && a.Obj == p.Obj && len(a.Args) == len(p.Args) {
			for i := range p.Args {
				matchTParams(p.Args[i], a.Args[i], bound)
			}
		}
	case *check.Tuple:
		if a, ok := check.Resolve(actual).(*check.Tuple); ok && len(a.Elems) == len(p.Elems) {
			for i := range p.Elems {
				matchTParams(p.Elems[i], a.Elems[i], bound)
			}
		}
	case *check.Func:
		if a, ok := check.Resolve(actual).(*check.Func); ok && len(a.Params) == len(p.Params) {
			for i := range p.Params {
				matchTParams(p.Params[i].T, a.Params[i].T, bound)
			}
			matchTParams(p.Result, a.Result, bound)
		}
	}
}

// ctorCall constructs a variant from call arguments (positional or
// named).
func (s *fn) ctorCall(vo *check.VariantObj, x *ast.CallExpr) *Value {
	t := s.natOf(x)
	tag := t.VariantIndex(vo.Name)
	if t.Kind != KEnum || tag < 0 {
		s.b.unsupported(x, "constructing `"+vo.Name+"` here")
		return s.undef(t)
	}
	fs := t.Variants[tag].Fields
	vals := make([]*Value, len(fs))
	pos := 0
	for _, a := range x.Args {
		i := pos
		if a.Name != nil {
			i = -1
			for j, f := range vo.Fields {
				if f.Name == a.Name.Name {
					i = j
				}
			}
		} else {
			pos++
		}
		if i < 0 || i >= len(fs) {
			s.b.unsupported(a, "this variant argument")
			continue
		}
		vals[i] = s.valueAs(a.Value, fs[i].Type)
	}
	for i := range vals {
		if vals[i] == nil {
			s.b.unsupported(x, "a variant constructed without all its fields")
			vals[i] = s.undef(fs[i].Type)
		}
	}
	return s.add(OpVariant, t, tag, vals...)
}

// isRetCtor reports a bare `Err(x)` or `Ok(x)` that the checker typed
// against the enclosing function's Result without recording a
// constructor.
func (s *fn) isRetCtor(x *ast.CallExpr) bool {
	id, ok := x.Fun.(*ast.Ident)
	if !ok || (id.Name != "Err" && id.Name != "Ok") || len(x.Args) != 1 {
		return false
	}
	info := s.b.info
	return info.VarOf(id) == nil && info.Ref(id) == nil && info.Ctor(x) == nil && info.CallOf(x) == nil
}

// retCtor is a bare `Ok(x)` or `Err(x)`: a Result of the call's type, or
// of the enclosing function's.
func (s *fn) retCtor(x *ast.CallExpr) *Value {
	b := s.b
	id := x.Fun.(*ast.Ident)
	ct := s.checkType(x)
	rt, et, ok := resultParts(ct)
	if !ok {
		if s.errT == nil {
			b.unsupported(x, "`"+id.Name+"(...)` here")
			return s.undef(s.natOf(x))
		}
		rt, et, ct = s.res, s.errT, b.resultOf(s.res, s.errT)
	}
	t := b.typ(ct, x)
	arg := x.Args[0].Value
	if id.Name == "Err" {
		ev := s.coerceErr(s.expr(arg), s.natCheck(arg), et, arg)
		return s.add(OpVariant, t, t.VariantIndex("Err"), ev)
	}
	return s.add(OpVariant, t, t.VariantIndex("Ok"), s.valueAs(arg, b.typ(rt, x)))
}

// resultParts returns T and E of Result[T, E].
func resultParts(t check.Type) (check.Type, check.Type, bool) {
	if n, ok := check.Resolve(check.Deep(t)).(*check.Named); ok && n.Obj.Module != nil && n.Obj.Module.IsPrelude && n.Obj.Name == "Result" && len(n.Args) == 2 {
		return n.Args[0], n.Args[1], true
	}
	return nil, nil, false
}

// optionElem returns T of Option[T].
func optionElem(t check.Type) (check.Type, bool) {
	if n, ok := check.Resolve(check.Deep(t)).(*check.Named); ok && n.Obj.Module != nil && n.Obj.Module.IsPrelude && n.Obj.Name == "Option" && len(n.Args) == 1 {
		return n.Args[0], true
	}
	return nil, false
}

// coerceErr converts an error at a `?` or `Err` site (core 2.6): the same
// type, or wrapped in the target's `from` variant.
func (s *fn) coerceErr(v *Value, from, to check.Type, at ast.Node) *Value {
	b := s.b
	tt := b.typ(to, at)
	if v.Type == tt || v.Op == OpUndef {
		return s.coerce(v, tt, at)
	}
	if vo := b.info.FromVariant(to, from); vo != nil {
		return s.variant(tt, vo.Name, []*Value{v}, at)
	}
	b.unsupported(at, "converting the error `"+v.Type.Name+"` to `"+tt.Name+"`")
	return s.undef(tt)
}

// try is `x?`: on a Result, return its error (converted) from the
// function; on an Option, return None. Otherwise it is the payload.
func (s *fn) try(x *ast.TryExpr) *Value {
	b := s.b
	m := b.mod
	xt := s.natCheck(x.X)
	v := s.expr(x.X)
	if v.Type.Kind != KEnum {
		b.unsupported(x, "`?` on `"+v.Type.Name+"`")
		return s.undef(s.natOf(x))
	}
	failTag, okTag := v.Type.VariantIndex("Err"), v.Type.VariantIndex("Ok")
	_, et, isResult := resultParts(xt)
	if !isResult {
		failTag, okTag = v.Type.VariantIndex("None"), v.Type.VariantIndex("Some")
	}
	if failTag < 0 || okTag < 0 {
		b.unsupported(x, "`?` on `"+v.Type.Name+"`")
		return s.undef(s.natOf(x))
	}
	c := s.add(OpEq, m.Builtin(KBool), nil, s.add(OpTag, m.Builtin(KInt), nil, v), s.cint(int64(failTag)))
	yes, no := s.branch(c)
	s.cur = yes
	want := s.f.Results[0]
	if isResult {
		if s.errT == nil {
			b.unsupported(x, "`?` on a Result in a function that cannot fail")
		} else {
			ev := s.add(OpPayload, v.Type.Variants[failTag].Fields[0].Type, PayloadRef{failTag, 0}, v)
			s.ret(s.add(OpVariant, want, want.VariantIndex("Err"), s.coerceErr(ev, et, s.errT, x)), x)
		}
	} else {
		rt := b.typ(s.res, x)
		if _, ok := optionElem(s.res); !ok {
			b.unsupported(x, "`?` on an Option outside a function returning an Option")
		} else {
			s.ret(s.add(OpVariant, rt, rt.VariantIndex("None")), x)
		}
	}
	s.cur = no
	return s.add(OpPayload, v.Type.Variants[okTag].Fields[0].Type, PayloadRef{okTag, 0}, v)
}

// typeCall is a numeric conversion `Float(i)`, `Int(f)`.
func (s *fn) typeCall(obj *check.TypeObj, x *ast.CallExpr) *Value {
	to := s.natOf(x)
	if len(x.Args) != 1 || !obj.Module.IsPrelude {
		s.b.unsupported(x, "the conversion `"+obj.Name+"(...)`")
		return s.undef(to)
	}
	v := s.expr(x.Args[0].Value)
	switch {
	case v.Type == to:
		return v
	case to.Kind == KFloat && v.Type.Kind == KInt:
		return s.add(OpIntToFloat, to, nil, v)
	case to.Kind == KInt && v.Type.Kind == KFloat:
		r := s.add(OpFloatToInt, to, nil, v)
		r.Pos = s.b.pos(x)
		return r
	}
	s.b.unsupported(x, "the conversion `"+obj.Name+"(...)`")
	return s.undef(to)
}

// argExprs matches call arguments to parameters (positional, then
// named); missing ones are nil.
func argExprs(params []*check.ParamInfo, x *ast.CallExpr) []ast.Expr {
	out := make([]ast.Expr, len(params))
	pos := 0
	for _, a := range x.Args {
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

// declCall calls a user function or method. A function with `var`
// parameters returns a tuple (result, updated vars...), and the updated
// values are stored back into the argument places.
func (s *fn) declCall(x *ast.CallExpr, ci *check.Call) *Value {
	b := s.b
	fo := ci.Fn
	var targs []check.Type
	for _, p := range typeParams(fo) {
		t := ci.Subst.Type(p)
		if t == nil {
			b.unsupported(x, "a generic call whose type arguments are not inferred")
			return s.undef(s.natOf(x))
		}
		targs = append(targs, s.apply(t))
	}
	callee := b.declare(fo, targs, x)
	ps := callee.Params()
	var args []*Value
	var places []place
	if fo.Recv != nil {
		if ci.Recv == nil {
			b.unsupported(x, "this method call")
			return s.undef(callee.Results[0])
		}
		if fo.RecvVar {
			p, ok := s.place(ci.Recv)
			if !ok {
				return s.undef(callee.Results[0])
			}
			places = append(places, p)
			args = append(args, p.get())
		} else {
			args = append(args, s.valueAs(ci.Recv, ps[0].Type))
		}
	}
	off := len(args)
	exprs := argExprs(fo.Params, x)
	// evaluate in source order
	vals := make([]*Value, len(fo.Params))
	argPlaces := make([]*place, len(fo.Params))
	for _, a := range x.Args {
		for i, e := range exprs {
			if e != a.Value || vals[i] != nil {
				continue
			}
			if fo.Params[i].Var {
				p, ok := s.place(e)
				if !ok {
					return s.undef(callee.Results[0])
				}
				argPlaces[i] = &p
				vals[i] = p.get()
			} else {
				vals[i] = s.valueAs(e, ps[off+i].Type)
			}
		}
	}
	for i, p := range fo.Params {
		if vals[i] == nil {
			if p.Default == nil {
				b.unsupported(x, "a call with a missing argument")
				vals[i] = s.undef(ps[off+i].Type)
				continue
			}
			vals[i] = s.valueAs(p.Default, ps[off+i].Type)
		}
		if argPlaces[i] != nil {
			places = append(places, *argPlaces[i])
		}
	}
	args = append(args, vals...)
	if len(callee.Results) == 1 {
		r := s.call(callee.Name, callee.Results[0], "", args...)
		return s.callResult(r, x)
	}
	t := b.mod.Tuple(callee.Results...)
	r := s.call(callee.Name, t, "", args...)
	for i, p := range places {
		p.set(s.add(OpField, t.Elems[i+1], i+1, r))
	}
	return s.callResult(s.add(OpField, t.Elems[0], 0, r), x)
}

// callResult ends the block after a call of a function that never
// returns.
func (s *fn) callResult(r *Value, x *ast.CallExpr) *Value {
	if s.isNever(x) {
		s.add(OpUnreachable, nil, nil)
		s.dead()
		return s.undef(r.Type)
	}
	return r
}

// stdCall lowers a prelude function or method: a runtime call, inline
// code, or a terminator (`fault`).
func (s *fn) stdCall(x *ast.CallExpr, ci *check.Call) *Value {
	b := s.b
	m := b.mod
	fo := ci.Fn
	nt := s.natOf(x)
	boolT, intT := m.Builtin(KBool), m.Builtin(KInt)
	if !fo.Module.IsPrelude {
		b.unsupported(x, "the std function `"+fo.Module.Name+"."+fo.DisplayName()+"`")
		return s.undef(nt)
	}
	if fo.Recv == nil {
		if fo.Name == "fault" && len(x.Args) == 1 {
			msg := s.valueAs(x.Args[0].Value, m.Builtin(KStr))
			f := s.add(OpFault, nil, nil, msg)
			f.Pos = b.pos(x)
			s.dead()
			return s.undef(nt)
		}
		b.unsupported(x, "the prelude function `"+fo.Name+"`")
		return s.undef(nt)
	}
	switch fo.Recv.Name {
	case "Stdio":
		switch fo.Name {
		case "print", "println", "eprint", "eprintln":
			r := s.expr(ci.Recv)
			return s.call("Stdio."+fo.Name, m.Builtin(KUnit), "", r, s.valueAs(x.Args[0].Value, m.Builtin(KStr)))
		}
	case "List":
		if fo.RecvVar {
			p, ok := s.place(ci.Recv)
			if !ok || p.t.Kind != KList {
				break
			}
			lt := p.t
			et := lt.Elems[0]
			switch fo.Name {
			case "push":
				l := p.get()
				p.set(s.call("list.push", lt, "", l, s.valueAs(x.Args[0].Value, et)))
				return s.unit()
			case "set":
				l := p.get()
				i := s.valueAs(x.Args[0].Value, intT)
				v := s.valueAs(x.Args[1].Value, et)
				p.set(s.call("list.set", lt, b.posLine(x), l, i, v))
				return s.unit()
			case "pop":
				l := p.get()
				rt := m.Tuple(lt, nt)
				r := s.call("list.pop", rt, "", l)
				p.set(s.add(OpField, lt, 0, r))
				return s.add(OpField, nt, 1, r)
			}
			break
		}
		l := s.expr(ci.Recv)
		if l.Type.Kind != KList {
			break
		}
		switch fo.Name {
		case "len":
			return s.call("list.len", intT, "", l)
		case "is_empty":
			return s.add(OpEq, boolT, nil, s.call("list.len", intT, "", l), s.cint(0))
		case "get":
			return s.call("list.get", nt, "", l, s.valueAs(x.Args[0].Value, intT))
		case "first":
			return s.call("list.get", nt, "", l, s.cint(0))
		case "last":
			// an empty list asks for -1, which list.get answers with None
			n := s.call("list.len", intT, "", l)
			return s.call("list.get", nt, "", l, s.add(OpSub, intT, nil, n, s.cint(1)))
		}
	case "Option", "Result":
		o := s.expr(ci.Recv)
		if o.Type.Kind != KEnum {
			break
		}
		isTag := func(name string) *Value {
			return s.add(OpEq, boolT, nil, s.add(OpTag, intT, nil, o), s.cint(int64(o.Type.VariantIndex(name))))
		}
		switch fo.Recv.Name + "." + fo.Name {
		case "Option.is_some":
			return isTag("Some")
		case "Option.is_none":
			return isTag("None")
		case "Result.is_ok":
			return isTag("Ok")
		case "Result.is_err":
			return isTag("Err")
		case "Result.ok", "Result.err":
			// the Ok or Err payload as an Option
			name := "Ok"
			if fo.Name == "err" {
				name = "Err"
			}
			from := o.Type.VariantIndex(name)
			join := s.newBlock()
			res := join.AddParam(nt)
			yes, no := s.branch(isTag(name))
			s.cur = yes
			pv := s.add(OpPayload, o.Type.Variants[from].Fields[0].Type, PayloadRef{from, 0}, o)
			s.jump(join, s.variant(nt, "Some", []*Value{pv}, x))
			s.cur = no
			s.jump(join, s.variant(nt, "None", nil, x))
			s.seal(join)
			s.cur = join
			return res
		case "Option.or_fault":
			msg := s.valueAs(x.Args[0].Value, m.Builtin(KStr))
			none, some := s.branch(isTag("None"))
			s.cur = none
			f := s.add(OpFault, nil, nil, msg)
			f.Pos = b.posLine(x)
			s.cur = some
			i := o.Type.VariantIndex("Some")
			return s.add(OpPayload, o.Type.Variants[i].Fields[0].Type, PayloadRef{i, 0}, o)
		}
	case "Int", "Float", "Str":
		v := s.expr(ci.Recv)
		var args []*Value
		args = append(args, v)
		for _, a := range x.Args {
			args = append(args, s.expr(a.Value))
		}
		pos := ""
		if fo.Recv.Name == "Int" && fo.Name == "abs" {
			pos = b.posLine(x)
		}
		switch fo.Recv.Name + "." + fo.Name {
		case "Int.abs", "Int.min", "Int.max", "Float.min", "Float.max", "Float.sqrt", "Float.floor",
			"Float.ceil", "Float.trunc", "Float.round", "Float.abs", "Float.is_nan", "Str.len", "Str.is_empty":
			return s.call(fo.Recv.Name+"."+fo.Name, nt, pos, args...)
		}
	}
	b.unsupported(x, "the method `"+fo.DisplayName()+"`")
	return s.undef(nt)
}
