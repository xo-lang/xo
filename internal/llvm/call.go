package llvm

import (
	"fmt"
	"math"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

func (f *fn) call(x *ast.CallExpr) val {
	g := f.g
	info := g.info
	if vo := info.Ctor(x); vo != nil {
		return f.ctorCall(vo, x)
	}
	if vo := info.Ctor(x.Fun); vo != nil {
		return f.ctorCall(vo, x)
	}
	if f.isRetCtor(x) {
		return f.retCtorValue(x)
	}
	if obj := info.TypeCall(x); obj != nil {
		return f.typeCall(obj, x)
	}
	ci := info.CallOf(x)
	if ci == nil {
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
			switch sel.Sel.Name {
			case "min", "max":
				return f.minMax(sel, x)
			}
		}
		g.unsupported(x, "this call")
		return val{f.ty(f.nat(x), x), "poison"}
	}
	if ci.Fn == nil {
		return f.valueCall(x)
	}
	if ci.Fn.Module.IsStd {
		return f.stdCall(x, ci)
	}
	return f.declCall(x, ci)
}

// ctorCall constructs a variant from call arguments (positional or
// named).
func (f *fn) ctorCall(vo *check.VariantObj, x *ast.CallExpr) val {
	return f.ctor(vo, f.nat(x), x.Args, x)
}

// ctor builds variant vo of enum type t from args (nil for a unit
// variant).
func (f *fn) ctor(vo *check.VariantObj, t check.Type, args []*ast.Arg, at ast.Node) val {
	et := f.g.lower(t, at)
	if et.k != kEnum {
		f.g.unsupported(at, "constructing `"+et.name()+"`")
		return val{et, "poison"}
	}
	idx, v := et.variant(vo.Name)
	if v == nil {
		f.g.unsupported(at, "the variant `"+vo.Name+"`")
		return val{et, "poison"}
	}
	exprs := make([]ast.Expr, len(v.fields))
	pos := 0
	for _, a := range args {
		if a.Name == nil {
			if pos < len(exprs) {
				exprs[pos] = a.Value
			}
			pos++
			continue
		}
		for i, fo := range vo.Fields {
			if fo.Name == a.Name.Name {
				exprs[i] = a.Value
			}
		}
	}
	vals := make([]val, len(v.fields))
	for _, a := range args {
		for i, e := range exprs {
			if e == a.Value {
				vals[i] = f.ownAs(e, v.fields[i].ct)
			}
		}
	}
	for i := range vals {
		if vals[i].t == nil {
			f.g.unsupported(at, "a variant constructed without all its fields")
			return val{et, "poison"}
		}
	}
	return f.variant(et, idx, vals, at)
}

// retCtorValue is `Ok(x)` or `Err(x)` outside return position, typed as
// the Result of the call or of the enclosing function.
func (f *fn) retCtorValue(x *ast.CallExpr) val {
	g := f.g
	id := x.Fun.(*ast.Ident)
	t := f.typeOf(x)
	rt, et, ok := g.resultParts(t)
	if !ok {
		if f.err == nil {
			g.unsupported(x, "`"+id.Name+"(...)` here")
			return val{g.lower(t, x), "poison"}
		}
		rt, et, t = f.res, f.err, g.resultOf(f.res, f.err)
	}
	lt := g.lower(t, x)
	arg := x.Args[0].Value
	if id.Name == "Err" {
		return f.variant(lt, 1, []val{f.ownAs(arg, et)}, x)
	}
	return f.variant(lt, 0, []val{f.ownAs(arg, rt)}, x)
}

// typeCall is a numeric conversion `Float(i)`, `Int(f)`.
func (f *fn) typeCall(obj *check.TypeObj, x *ast.CallExpr) val {
	g := f.g
	if len(x.Args) != 1 || !obj.Module.IsPrelude {
		g.unsupported(x, "the conversion `"+obj.Name+"(...)`")
		return val{f.ty(f.nat(x), x), "poison"}
	}
	v := f.val(x.Args[0].Value)
	to := f.ty(f.nat(x), x)
	switch {
	case to.k == v.t.k:
		return val{to, v.v}
	case to.k == kFloat && v.t.k == kInt:
		return val{to, f.def("sitofp i64 %s to double", v.v)}
	case to.k == kInt && v.t.k == kFloat:
		// truncates toward zero; NaN and out of range fault
		t := f.def("call double @llvm.trunc.f64(double %s)", v.v)
		lo := f.def("fcmp oge double %s, 0x%016X", t, math.Float64bits(-9223372036854775808.0))
		hi := f.def("fcmp olt double %s, 0x%016X", t, math.Float64bits(9223372036854775808.0))
		in := f.def("and i1 %s, %s", lo, hi)
		bad := f.def("xor i1 %s, true", in)
		fail, ok := f.label("fifault"), f.label("fiok")
		f.unlikely(bad, fail, ok)
		f.startBlock(fail)
		f.faultCall("@xo_fault_float_int(double %s, ptr %s)", v.v, g.cstr(g.loc(x)))
		f.startBlock(ok)
		return val{to, f.def("fptosi double %s to i64", t)}
	}
	g.unsupported(x, "the conversion `"+obj.Name+"(...)` from `"+v.t.name()+"`")
	return val{to, "poison"}
}

// minMax is `a.min(b)`, `a.max(b)` on Int and Float.
func (f *fn) minMax(sel *ast.SelectorExpr, x *ast.CallExpr) val {
	a := f.val(sel.X)
	if len(x.Args) != 1 || (a.t.k != kInt && a.t.k != kFloat) {
		f.g.unsupported(x, "`."+sel.Sel.Name+"` here")
		return val{a.t, "poison"}
	}
	b := f.valAs(x.Args[0].Value, a.t.t)
	var c string
	if a.t.k == kInt {
		c = f.def("icmp slt i64 %s, %s", a.v, b.v)
	} else {
		c = f.def("fcmp olt double %s, %s", a.v, b.v)
	}
	if sel.Sel.Name == "min" {
		return val{a.t, f.def("select i1 %s, %s %s, %s %s", c, a.t.ll, a.v, a.t.ll, b.v)}
	}
	return val{a.t, f.def("select i1 %s, %s %s, %s %s", c, a.t.ll, b.v, a.t.ll, a.v)}
}

// args evaluates the arguments of a call in source order against the
// parameters (pointers for `var` parameters; defaults for missing ones).
// A by value argument is borrowed (not retained) unless the call also
// has a `var` parameter or receiver (anyVar): then the callee could
// mutate, through the `var` one, the list the borrowed one shares.
func (f *fn) args(x *ast.CallExpr, params []*check.ParamInfo, ptypes []*ty, sub *check.Subst, anyVar bool) []string {
	exprs := argExprs(params, x)
	for _, p := range params {
		anyVar = anyVar || p.Var
	}
	out := make([]string, len(params))
	done := make([]bool, len(params))
	eval := func(i int) {
		e := exprs[i]
		if params[i].Var {
			p, _, _ := f.place(e)
			out[i] = "ptr " + p
		} else {
			// a borrow: the callee cannot mutate it, and retains what it
			// keeps
			var v val
			if anyVar {
				v = f.ownAs(e, ptypes[i].t)
			} else {
				v = f.valAs(e, ptypes[i].t)
			}
			out[i] = v.t.ll + " " + v.v
		}
		done[i] = true
	}
	for _, a := range x.Args {
		for i, e := range exprs {
			if e == a.Value && !done[i] {
				eval(i)
			}
		}
	}
	for i, p := range params {
		if exprs[i] == nil {
			if p.Default == nil {
				f.g.unsupported(x, "a call with a missing argument")
				out[i] = ptypes[i].ll + " poison"
				continue
			}
			pt := p.Type
			if sub != nil {
				pt = sub.Apply(pt)
			}
			v := f.valAs(p.Default, f.t(pt))
			out[i] = v.t.ll + " " + v.v
		}
	}
	return out
}

// declCall calls a user function or method.
func (f *fn) declCall(x *ast.CallExpr, ci *check.Call) val {
	g := f.g
	fo := ci.Fn
	if fo.Iface {
		g.unsupported(x, "interface method calls")
		return val{f.ty(f.nat(x), x), "poison"}
	}
	var targs []check.Type
	for _, p := range typeParams(fo) {
		t := ci.Subst.Type(p)
		if t == nil {
			g.unsupported(x, "a generic call whose type arguments are not inferred")
			return val{f.ty(f.nat(x), x), "poison"}
		}
		targs = append(targs, f.t(t))
	}
	in := g.instance(fo, targs, x)
	var all []string
	if fo.Recv != nil {
		if ci.Recv == nil {
			g.unsupported(x, "this method call")
			return val{in.sig.ret, "poison"}
		}
		if fo.RecvVar {
			p, _, _ := f.place(ci.Recv)
			all = append(all, "ptr "+p)
		} else {
			v := f.valAs(ci.Recv, in.sig.recv.t)
			all = append(all, v.t.ll+" "+v.v)
		}
	}
	all = append(all, f.args(x, fo.Params, in.sig.params, in.sub, fo.RecvVar)...)
	if f.term {
		return val{in.sig.ret, "poison"}
	}
	f.clobber()
	r := f.def("call %s @%s(%s)", in.sig.ret.ll, quoteName(in.name), strings.Join(all, ", "))
	return f.callResult(val{in.sig.ret, r}, x)
}

// callResult gives a call's value its natural type (a function that
// cannot fail returns its result; one that can returns its Result).
func (f *fn) callResult(v val, x *ast.CallExpr) val {
	nt := f.ty(f.nat(x), x)
	if check.IsNever(f.nat(x)) {
		f.emit("unreachable")
		f.term = true
		return val{nt, "poison"}
	}
	if nt.key != v.t.key && nt.ll == v.t.ll {
		return val{nt, v.v}
	}
	return v
}

// valueCall calls a function value: {code, env}.
func (f *fn) valueCall(x *ast.CallExpr) val {
	g := f.g
	fv := f.val(x.Fun)
	ft := fv.t.fn
	if fv.t.k != kFn || ft == nil {
		g.unsupported(x, "a call of this value")
		return val{f.ty(f.nat(x), x), "poison"}
	}
	var err check.Type
	if ft.Err != nil && !check.IsNever(ft.Err) {
		err = ft.Err
	}
	rt := g.retType(ft.Result, err, x)
	code := f.def("extractvalue %%xo.fn %s, 0", fv.v)
	env := f.def("extractvalue %%xo.fn %s, 1", fv.v)
	all := []string{"ptr " + env}
	anyVar := false
	for _, p := range ft.Params {
		anyVar = anyVar || p.Var
	}
	for i, a := range x.Args {
		if i >= len(ft.Params) {
			break
		}
		if ft.Params[i].Var {
			p, _, _ := f.place(a.Value)
			all = append(all, "ptr "+p)
			continue
		}
		var v val
		if anyVar {
			v = f.ownAs(a.Value, ft.Params[i].T)
		} else {
			v = f.valAs(a.Value, ft.Params[i].T)
		}
		all = append(all, v.t.ll+" "+v.v)
	}
	if f.term {
		return val{rt, "poison"}
	}
	f.clobber()
	r := f.def("call %s %s(%s)", rt.ll, code, strings.Join(all, ", "))
	return f.callResult(val{rt, r}, x)
}

// funcValue is a declared function used as a value: a thunk that ignores
// the environment.
func (f *fn) funcValue(fo *check.FuncObj, t check.Type, at ast.Node) val {
	g := f.g
	ft := g.lower(t, at)
	if len(typeParams(fo)) > 0 || fo.Module.IsStd || fo.Recv != nil {
		g.unsupported(at, "using `"+fo.DisplayName()+"` as a value")
		return val{ft, "poison"}
	}
	in := g.instance(fo, nil, at)
	name := "thunk:" + in.name
	if !g.helpers[name] {
		g.helpers[name] = true
		h := g.newFn(nil, nil)
		names := []string{"%env"}
		var call []string
		for i, p := range in.sig.params {
			n := fmt.Sprintf("%%a%d", i)
			ll := p.ll
			if in.sig.vars[i] {
				ll = "ptr"
			}
			names = append(names, ll+" "+n)
			call = append(call, ll+" "+n)
		}
		r := h.def("call %s @%s(%s)", in.sig.ret.ll, quoteName(in.name), strings.Join(call, ", "))
		h.emit("ret %s %s", in.sig.ret.ll, r)
		h.term = true
		names[0] = "ptr %env"
		g.writeFunc(name, in.sig.ret.ll, strings.Join(names, ", "), h)
	}
	return val{ft, fmt.Sprintf("{ ptr @%s, ptr null }", quoteName(name))}
}

// closure builds a closure value: the captured bindings are copied into a
// heap environment (core 3.3: capture by value).
func (f *fn) closure(x *ast.FnLit) val {
	g := f.g
	t := f.ty(f.nat(x), x)
	ft, ok := check.Resolve(t.t).(*check.Func)
	if !ok {
		g.unsupported(x, "this closure")
		return val{t, "poison"}
	}
	// captured bindings: used inside, bound outside
	var caps []*check.VarObj
	seen := map[*check.VarObj]bool{}
	check.Inspect(x.Body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			if v := g.info.VarOf(id); v != nil && !seen[v] {
				if _, outer := f.vars[v]; outer {
					seen[v] = true
					caps = append(caps, v)
				}
			}
		}
		return true
	})
	var capTys []*ty
	var lls []string
	for _, v := range caps {
		l := f.vars[v]
		capTys = append(capTys, l.t)
		lls = append(lls, l.t.ll)
	}
	envLL := "{ " + strings.Join(lls, ", ") + " }"
	if len(lls) == 0 {
		envLL = "{}"
	}
	env := "null"
	if len(caps) > 0 {
		var sz []*ty
		var boxed []bool
		for _, ct := range capTys {
			sz = append(sz, ct)
			boxed = append(boxed, false)
		}
		size, _ := layout(sz, boxed)
		env = f.def("call ptr @xo_alloc(i64 %d)", max(size, 1))
		for i, v := range caps {
			l := f.vars[v]
			cv := f.load(l.t, l.ptr)
			f.retain(cv)
			p := f.def("getelementptr %s, ptr %s, i32 0, i32 %d", envLL, env, i)
			f.emit("store %s %s, ptr %s", cv.t.ll, cv.v, p)
		}
	}
	// the closure body
	g.nclos++
	name := fmt.Sprintf("closure.%d", g.nclos)
	c := g.newFn(f.sub, f.safe)
	c.res = ft.Result
	if c.res == nil {
		c.res = unitType
	}
	if ft.Err != nil && !check.IsNever(ft.Err) {
		c.err = ft.Err
	}
	c.ret = g.retType(c.res, c.err, x)
	for i, v := range caps {
		p := c.def("getelementptr %s, ptr %%env, i32 0, i32 %d", envLL, i)
		cv := c.load(capTys[i], p)
		c.vars[v] = &local{ptr: c.spill(cv), t: capTys[i]}
	}
	params := []string{"ptr %env"}
	pvs := g.info.ClosureParams(x)
	for i, p := range ft.Params {
		pt := g.lower(p.T, x)
		pn := fmt.Sprintf("%%a%d", i)
		if p.Var {
			params = append(params, "ptr "+pn)
			if i < len(pvs) && pvs[i] != nil {
				c.vars[pvs[i]] = &local{ptr: pn, t: pt}
			}
			continue
		}
		params = append(params, pt.ll+" "+pn)
		if i < len(pvs) && pvs[i] != nil {
			c.vars[pvs[i]] = &local{ptr: c.spill(val{pt, pn}), t: pt}
		}
	}
	c.block(x.Body, dest{kind: dRet})
	g.writeFunc(name, c.ret.ll, strings.Join(params, ", "), c)
	a := f.def("insertvalue %%xo.fn undef, ptr @%s, 0", quoteName(name))
	return val{t, f.def("insertvalue %%xo.fn %s, ptr %s, 1", a, env)}
}

// ---------------------------------------------------------------------------
// Prelude functions and methods

func (f *fn) stdCall(x *ast.CallExpr, ci *check.Call) val {
	g := f.g
	fo := ci.Fn
	nt := f.ty(f.nat(x), x)
	bt := func() *ty { return g.lower(g.preludeType("Bool"), x) }
	intT := func() *ty { return g.lower(g.preludeType("Int"), x) }
	if !fo.Module.IsPrelude {
		g.unsupported(x, "the std function `"+fo.Module.Name+"."+fo.DisplayName()+"`")
		return val{nt, "poison"}
	}
	if fo.Recv == nil {
		switch fo.Name {
		case "fault":
			m := f.val(x.Args[0].Value)
			p, n := f.strParts(m)
			f.faultCall("@xo_fault(ptr %s, i64 %s, ptr %s)", p, n, g.cstr(g.loc(x)))
			return val{nt, "poison"}
		}
		g.unsupported(x, "the prelude function `"+fo.Name+"`")
		return val{nt, "poison"}
	}
	recvName := fo.Recv.Name
	switch recvName {
	case "Stdio":
		switch fo.Name {
		case "print", "println", "eprint", "eprintln":
			f.val(ci.Recv)
			s := f.val(x.Args[0].Value)
			p, n := f.strParts(s)
			f.emit("call void @xo_%s(ptr %s, i64 %s)", fo.Name, p, n)
			return f.unitVal()
		}
	case "List":
		return f.listMethod(x, ci, nt)
	case "Option", "Result":
		rv := f.val(ci.Recv)
		tag := f.tagOf(rv)
		switch fo.Name {
		case "is_some", "is_err":
			return val{bt(), f.def("icmp eq i64 %s, 1", tag)}
		case "is_none", "is_ok":
			return val{bt(), f.def("icmp eq i64 %s, 0", tag)}
		case "ok", "err":
			// Result -> Option of the Ok (tag 0) or Err (tag 1) payload
			want := int64(0)
			if fo.Name == "err" {
				want = 1
			}
			p := f.spill(rv)
			slot := f.alloca(nt)
			yes, no, end := f.label("has"), f.label("not"), f.label("oend")
			c := f.def("icmp eq i64 %s, %d", tag, want)
			f.condBr(c, yes, no)
			f.startBlock(yes)
			pv := f.payload(p, rv.t, int(want), 0, x)
			sv := f.variant(nt, 1, []val{pv}, x)
			f.emit("store %s %s, ptr %s", nt.ll, sv.v, slot)
			f.br(end)
			f.startBlock(no)
			nv := f.variant(nt, 0, nil, x)
			f.emit("store %s %s, ptr %s", nt.ll, nv.v, slot)
			f.br(end)
			f.startBlock(end)
			return f.load(nt, slot)
		case "or_fault":
			if recvName != "Option" {
				break
			}
			msg := f.val(x.Args[0].Value)
			p := f.spill(rv)
			c := f.def("icmp eq i64 %s, 0", tag)
			fail, ok := f.label("orfault"), f.label("orok")
			f.unlikely(c, fail, ok)
			f.startBlock(fail)
			mp, mn := f.strParts(msg)
			f.faultCall("@xo_fault(ptr %s, i64 %s, ptr %s)", mp, mn, g.cstr(g.lineLoc(x)))
			f.startBlock(ok)
			return f.payload(p, rv.t, 1, 0, x)
		}
	case "Int":
		switch fo.Name {
		case "abs":
			v := f.val(ci.Recv)
			bad := f.def("icmp eq i64 %s, %d", v.v, int64(math.MinInt64))
			fail, ok := f.label("absov"), f.label("absok")
			f.unlikely(bad, fail, ok)
			f.startBlock(fail)
			// the Go runtime locates this fault from the stack: no column
			f.faultCall("@xo_fault_abs(i64 %s, ptr %s)", v.v, g.cstr(g.lineLoc(x)))
			f.startBlock(ok)
			neg := f.def("icmp slt i64 %s, 0", v.v)
			m := f.def("sub nsw i64 0, %s", v.v)
			return val{v.t, f.def("select i1 %s, i64 %s, i64 %s", neg, m, v.v)}
		}
	case "Float":
		intr := map[string]string{"sqrt": "sqrt", "floor": "floor", "ceil": "ceil", "trunc": "trunc", "round": "round", "abs": "fabs"}[fo.Name]
		v := f.val(ci.Recv)
		if intr != "" {
			return val{v.t, f.def("call double @llvm.%s.f64(double %s)", intr, v.v)}
		}
		if fo.Name == "is_nan" {
			return val{bt(), f.def("fcmp uno double %s, %s", v.v, v.v)}
		}
	case "Str":
		v := f.val(ci.Recv)
		switch fo.Name {
		case "len":
			return val{intT(), f.def("extractvalue %%xo.str %s, 1", v.v)}
		case "is_empty":
			n := f.def("extractvalue %%xo.str %s, 1", v.v)
			return val{bt(), f.def("icmp eq i64 %s, 0", n)}
		}
	}
	g.unsupported(x, "the method `"+fo.DisplayName()+"`")
	return val{nt, "poison"}
}

func (f *fn) listMethod(x *ast.CallExpr, ci *check.Call, nt *ty) val {
	g := f.g
	fo := ci.Fn
	bt := g.lower(g.preludeType("Bool"), x)
	if fo.RecvVar {
		p, lt, _ := f.place(ci.Recv)
		if lt.k != kList {
			g.unsupported(x, "this List method")
			return val{nt, "poison"}
		}
		et := lt.elem
		switch fo.Name {
		case "push":
			v := f.ownAs(x.Args[0].Value, et.t)
			slot := f.listPush(p, et, x)
			f.emit("store %s %s, ptr %s"+tagElem, et.ll, v.v, slot)
			return f.unitVal()
		case "set":
			i := f.val(x.Args[0].Value)
			v := f.ownAs(x.Args[1].Value, et.t)
			l := f.listMut(p, et, x)
			n := f.listLen(l)
			bad := f.def("icmp uge i64 %s, %s", i.v, n)
			fail, ok := f.label("oob"), f.label("inb")
			f.unlikely(bad, fail, ok)
			f.startBlock(fail)
			f.faultCall("@xo_fault_index(i64 %s, i64 %s, ptr %s)", i.v, n, g.cstr(g.lineLoc(x)))
			f.startBlock(ok)
			ep := f.def("getelementptr %s, ptr %s, i64 %s", et.ll, f.listData(l), i.v)
			f.emit("store %s %s, ptr %s"+tagElem, et.ll, v.v, ep)
			return f.unitVal()
		case "pop":
			l := f.def("load ptr, ptr %s", p)
			n := f.listLen(l)
			slot := f.alloca(nt)
			yes, no, end := f.label("pop"), f.label("empty"), f.label("pend")
			c := f.def("icmp sgt i64 %s, 0", n)
			f.condBr(c, yes, no)
			f.startBlock(yes)
			ul := f.listMut(p, et, x)
			n1 := f.def("sub i64 %s, 1", n)
			f.emit("store i64 %s, ptr %s"+tagHdr, n1, ul)
			ep := f.def("getelementptr %s, ptr %s, i64 %s", et.ll, f.listData(ul), n1)
			ev := val{et, f.def("load %s, ptr %s"+tagElem, et.ll, ep)}
			sv := f.variant(nt, 1, []val{ev}, x)
			f.emit("store %s %s, ptr %s", nt.ll, sv.v, slot)
			f.br(end)
			f.startBlock(no)
			nv := f.variant(nt, 0, nil, x)
			f.emit("store %s %s, ptr %s", nt.ll, nv.v, slot)
			f.br(end)
			f.startBlock(end)
			return f.load(nt, slot)
		}
		g.unsupported(x, "the method `List."+fo.Name+"`")
		return val{nt, "poison"}
	}
	lv := f.val(ci.Recv)
	if lv.t.k != kList {
		g.unsupported(x, "this List method")
		return val{nt, "poison"}
	}
	et := lv.t.elem
	switch fo.Name {
	case "len":
		return val{nt, f.listLen(lv.v)}
	case "is_empty":
		n := f.listLen(lv.v)
		return val{bt, f.def("icmp eq i64 %s, 0", n)}
	case "get", "first", "last":
		n := f.listLen(lv.v)
		var i string
		switch fo.Name {
		case "get":
			i = f.val(x.Args[0].Value).v
		case "first":
			i = "0"
		default:
			i = f.def("sub i64 %s, 1", n)
		}
		slot := f.alloca(nt)
		yes, no, end := f.label("get"), f.label("none"), f.label("gend")
		c := f.def("icmp ult i64 %s, %s", i, n)
		f.condBr(c, yes, no)
		f.startBlock(yes)
		ep := f.def("getelementptr %s, ptr %s, i64 %s", et.ll, f.listData(lv.v), i)
		ev := val{et, f.def("load %s, ptr %s"+tagElem, et.ll, ep)}
		sv := f.variant(nt, 1, []val{ev}, x)
		f.emit("store %s %s, ptr %s", nt.ll, sv.v, slot)
		f.br(end)
		f.startBlock(no)
		nv := f.variant(nt, 0, nil, x)
		f.emit("store %s %s, ptr %s", nt.ll, nv.v, slot)
		f.br(end)
		f.startBlock(end)
		return f.load(nt, slot)
	}
	g.unsupported(x, "the method `List."+fo.Name+"`")
	return val{nt, "poison"}
}
