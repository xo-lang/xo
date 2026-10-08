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
		b.unsupported(x, "calls of function values (closures)")
		return s.undef(s.natOf(x))
	}
	if ci.Fn.Module.IsStd {
		return s.stdCall(x, ci)
	}
	return s.declCall(x, ci)
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
	callee := b.declare(fo, x)
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
				p.set(s.call("list.set", lt, b.pos(x), l, i, v))
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
		}
	case "Option":
		o := s.expr(ci.Recv)
		switch fo.Name {
		case "is_some", "is_none":
			name := "Some"
			if fo.Name == "is_none" {
				name = "None"
			}
			tag := o.Type.VariantIndex(name)
			return s.add(OpEq, boolT, nil, s.add(OpTag, intT, nil, o), s.cint(int64(tag)))
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
			pos = b.pos(x)
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
