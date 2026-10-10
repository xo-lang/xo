package ssa

import (
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Display, Debug, and values of interface type on the native backends
// (decision 0074).
//
//   - `${x}` of any type appends x's Display text: Str, numbers, Bool,
//     Duration, Time inline or through the runtime; a type with a
//     `display` method calls it (a std type's goes to its native
//     implementation); anything else, `derive Display` included, appends
//     its Debug text (core 2.9).
//   - Debug text comes from functions generated per checker type
//     (xo.debug[T]: (Str, T) -> Str, the text appended), with the Go
//     backend's format: `User{id: 1, name: "a"}`, `NotFound(id: 42)`,
//     `[1, 2]`, `{"a": 1}`, `#{1}`, `(1, "a")`, `Some(1)`, `b"x"`.
//   - A value of the prelude interfaces Error, Display, or Debug (an
//     error returned by `main ! Error`, a log field of Map[Str, Debug]) is
//     its text: the struct xo.dyn {text: Str, raw: Bool, dbg: Str}, made
//     where a concrete value is converted to the interface. raw marks a
//     number or Bool (a JSON log record writes it bare). Their methods
//     return the text. dbg is the value's Debug text when it differs
//     from text (a Str is quoted, a type with a `display` method shows
//     its fields), else empty: the Debug text of the interface value.

// dynType is the SSA type of a value of interface type.
func (b *builder) dynType() *Type {
	m := b.mod
	t, created := m.Named("xo.dyn", KStruct)
	if created {
		t.Fields = []Field{{"text", m.Builtin(KStr)}, {"raw", m.Builtin(KBool)}, {"dbg", m.Builtin(KStr)}}
	}
	return t
}

func isDyn(t *Type) bool { return t.Kind == KStruct && t.Name == "xo.dyn" }

// dynIface reports whether the interface obj is lowered as xo.dyn.
func dynIface(obj *check.TypeObj) bool {
	if obj.Module == nil || !obj.Module.IsPrelude {
		return false
	}
	switch obj.Name {
	case "Error", "Display", "Debug":
		return true
	}
	return false
}

// toDyn converts v, of checker type ct, to a value of the interface
// iface (its Display text, or for Debug a type's Debug text when it has
// no Display).
func (s *fn) toDyn(v *Value, ct check.Type, iface string, at ast.Node) *Value {
	b := s.b
	if isDyn(v.Type) {
		return v
	}
	st := b.mod.Builtin(KStr)
	acc := s.add(OpConst, st, "")
	ct = check.Deep(ct)
	if iface == "Debug" && !s.displayable(ct) {
		if m := s.methodOf(ct, "debug"); m != nil {
			acc = s.call("str.concat", st, "", acc, s.callMethod(m, ct, v, at))
		} else {
			acc = s.appendDebug(acc, v, ct, at)
		}
	} else {
		acc = s.appendDisplay(acc, v, ct, at)
	}
	raw := false
	switch v.Type.Kind {
	case KInt, KFloat, KBool:
		raw = !isPreludeNamed(ct, "Duration", "Time")
	}
	dbg := s.add(OpConst, st, "")
	if isPreludeNamed(ct, "Str", "Rune") || isIface(v.Type) || s.methodOf(ct, "display") != nil {
		dbg = s.appendDebug(dbg, v, ct, at)
	}
	return s.add(OpStruct, b.dynType(), nil, acc, s.bool(raw), dbg)
}

// dynDebug appends the Debug text of the interface value v (xo.dyn) to
// acc: its dbg text, or its text when dbg is empty.
func (s *fn) dynDebug(acc, v *Value) *Value {
	m := s.b.mod
	st := m.Builtin(KStr)
	dbg := s.add(OpField, st, 2, v)
	join := s.newBlock()
	res := join.AddParam(st)
	yes, no := s.branch(s.add(OpEq, m.Builtin(KBool), nil, s.call("Str.len", m.Builtin(KInt), "", dbg), s.cint(0)))
	s.cur = yes
	s.jump(join, s.call("str.concat", st, "", acc, s.add(OpField, st, 0, v)))
	s.cur = no
	s.jump(join, s.call("str.concat", st, "", acc, dbg))
	s.seal(join)
	s.cur = join
	return res
}

// displayable reports whether values of ct have Display text of their
// own (not their Debug text).
func (s *fn) displayable(ct check.Type) bool {
	switch r := check.Resolve(check.Deep(ct)).(type) {
	case *check.Named:
		if r.Obj.Module != nil && r.Obj.Module.IsPrelude {
			switch r.Obj.Name {
			case "Int", "Int64", "Float", "Bool", "Str", "Duration", "Time":
				return true
			}
			if r.Obj.Kind == check.KInterface {
				return true
			}
		}
		if r.Obj.Kind == check.KInterface {
			return true
		}
		return s.methodOf(r, "display") != nil
	}
	return false
}

// methodOf returns the method name of the named type ct (declared on
// it, or a std type's native implementation), or nil.
func (s *fn) methodOf(ct check.Type, name string) *check.FuncObj {
	n, ok := check.Resolve(check.Deep(ct)).(*check.Named)
	if !ok {
		return nil
	}
	m := n.Obj.Methods[name]
	if m == nil {
		if n.Obj.Module != nil && n.Obj.Module.IsStd {
			// a method the native implementation adds to a std type
			// (json.DecodeErr's display)
			if nm := s.b.nativeMod(n.Obj.Module.Path); nm != nil {
				if f, ok := nm.Scope[strings.ToLower(n.Obj.Name)+"_"+name].(*check.FuncObj); ok && f.Recv == nil {
					return f
				}
			}
		}
		return nil
	}
	if m.Module != nil && m.Module.IsStd {
		return s.b.implOf(m)
	}
	return m
}

// callMethod calls method m (from methodOf) of the named type ct on v,
// with no other arguments.
func (s *fn) callMethod(m *check.FuncObj, ct check.Type, v *Value, at ast.Node) *Value {
	b := s.b
	n := check.Resolve(check.Deep(ct)).(*check.Named)
	targs := append([]check.Type(nil), n.Args...)
	if len(typeParams(m)) != len(targs) {
		b.unsupported(at, "the method `"+m.DisplayName()+"` of `"+ct.String()+"`")
		return s.undef(b.typ(m.Result, at))
	}
	callee := b.declare(m, targs, at)
	r := s.call(callee.Name, callee.Results[0], "", v)
	s.check(callee.Name)
	return r
}

// appendDisplay is acc + the Display text of v (checker type ct).
func (s *fn) appendDisplay(acc, v *Value, ct check.Type, at ast.Node) *Value {
	b := s.b
	m := b.mod
	st := m.Builtin(KStr)
	ct = check.Deep(ct)
	if isDyn(v.Type) {
		return s.call("str.concat", st, "", acc, s.add(OpField, st, 0, v))
	}
	if isIface(v.Type) {
		return s.ifaceShowCall(acc, v, false)
	}
	switch {
	case isPreludeNamed(ct, "Secret"):
		return s.call("str.concat", st, "", acc, s.add(OpConst, st, "<redacted>"))
	case isPreludeNamed(ct, "Duration"):
		return s.rt("rs_append_duration", st, acc, v)
	case isPreludeNamed(ct, "Time"):
		return s.rt("rs_append_time", st, acc, v)
	case isPreludeNamed(ct, "Bytes"):
		return s.appendDebug(acc, v, ct, at)
	case isPreludeNamed(ct, "UInt64"):
		return s.rt("rs_append_u64", st, acc, v)
	case isPreludeNamed(ct, "Rune"):
		return s.rt("rs_append_rune", st, acc, v)
	}
	switch v.Type.Kind {
	case KStr:
		return s.call("str.concat", st, "", acc, v)
	case KInt, KFloat:
		return s.call("str.concat_"+lowerKind(v.Type), st, "", acc, v)
	case KBool:
		return s.call("str.concat", st, "", acc, s.call("str.of_bool", st, "", v))
	}
	if md := s.methodOf(ct, "display"); md != nil {
		return s.call("str.concat", st, "", acc, s.callMethod(md, ct, v, at))
	}
	return s.appendDebug(acc, v, ct, at)
}

// isBuiltinKind reports a Str, Int, Float, or Bool.
func isBuiltinKind(t *Type) bool {
	switch t.Kind {
	case KStr, KInt, KFloat, KBool:
		return true
	}
	return false
}

func lowerKind(t *Type) string {
	if t.Kind == KFloat {
		return "float"
	}
	return "int"
}

// appendDebug is acc + the Debug text of v (checker type ct).
func (s *fn) appendDebug(acc, v *Value, ct check.Type, at ast.Node) *Value {
	st := s.b.mod.Builtin(KStr)
	return s.call(s.b.debugFunc(check.Deep(ct), v.Type, at), st, "", acc, v)
}

// debugFunc returns xo.debug[ct](acc: Str, v: T) -> Str: acc and the
// Debug text of v (T is t, ct lowered).
func (b *builder) debugFunc(ct check.Type, t *Type, at ast.Node) string {
	m := b.mod
	st, intT, boolT := m.Builtin(KStr), m.Builtin(KInt), m.Builtin(KBool)
	return b.genFunc("xo.debug["+ct.String()+"]", []*Type{st, t}, []*Type{st}, func(s *fn, ps []*Value) {
		acc, v := ps[0], ps[1]
		lit := func(a *Value, text string) *Value {
			return s.call("str.concat", st, "", a, s.add(OpConst, st, text))
		}
		sub := func(a, x *Value, xt check.Type) *Value {
			x = s.unbox(x)
			return s.call(b.debugFunc(check.Deep(xt), x.Type, at), st, "", a, x)
		}
		if isDyn(t) {
			s.retv(s.dynDebug(acc, v))
			return
		}
		if isIface(t) {
			s.retv(s.ifaceShowCall(acc, v, true))
			return
		}
		if okT, et, ok := resultParts(ct); ok && check.IsNever(et) {
			// Result[T, Never] is lowered as its T (decision 0074)
			s.retv(lit(sub(lit(acc, "Ok("), v, okT), ")"))
			return
		}
		switch {
		case isPreludeNamed(ct, "Secret"):
			s.retv(lit(acc, "<redacted>"))
			return
		case isPreludeNamed(ct, "Buf"):
			// `Buf[1, 2]` (std/rt's Buf.XoDebug)
			if n, ok := check.Resolve(ct).(*check.Named); ok && len(n.Args) == 1 {
				s.retv(sub(lit(acc, "Buf"), v, b.preludeType("List", n.Args[0])))
				return
			}
		case isPreludeNamed(ct, "Duration"):
			s.retv(s.rt("rs_append_duration", st, acc, v))
			return
		case isPreludeNamed(ct, "Time"):
			s.retv(s.rt("rs_append_time", st, acc, v))
			return
		case isPreludeNamed(ct, "Bytes"):
			s.retv(s.rt("rs_append_quote", st, lit(acc, "b"), v))
			return
		case isPreludeNamed(ct, "UInt64"):
			s.retv(s.rt("rs_append_u64", st, acc, v))
			return
		case isPreludeNamed(ct, "Rune"):
			s.retv(s.rt("rs_append_rune_quote", st, acc, v))
			return
		}
		switch t.Kind {
		case KStr:
			s.retv(s.rt("rs_append_quote", st, acc, v))
			return
		case KInt, KFloat:
			s.retv(s.call("str.concat_"+lowerKind(t), st, "", acc, v))
			return
		case KBool:
			s.retv(s.call("str.concat", st, "", acc, s.call("str.of_bool", st, "", v)))
			return
		case KUnit:
			s.retv(lit(acc, "()"))
			return
		case KFunc:
			s.retv(lit(acc, "<fn>"))
			return
		case KCap:
			s.retv(lit(acc, "<"+lowerName(t.Name)+">"))
			return
		case KHandle:
			s.retv(lit(acc, "<"+lowerName(t.Name)+">"))
			return
		case KBox:
			s.retv(sub(acc, v, ct))
			return
		case KTuple:
			tt, ok := check.Resolve(ct).(*check.Tuple)
			if !ok || len(tt.Elems) != len(t.Elems) {
				break
			}
			a := lit(acc, "(")
			for i, et := range t.Elems {
				if i > 0 {
					a = lit(a, ", ")
				}
				a = sub(a, s.add(OpField, et, i, v), tt.Elems[i])
			}
			s.retv(lit(a, ")"))
			return
		}
		n, ok := check.Resolve(ct).(*check.Named)
		if !ok {
			b.unsupported(at, "the Debug text of `"+ct.String()+"`")
			s.retv(acc)
			return
		}
		obj := n.Obj
		if obj.Module != nil && obj.Module.IsPrelude {
			switch obj.Name {
			case "List":
				et := t.Elems[0]
				av := local("acc")
				s.writeVar(av, lit(acc, "["))
				s.forUpTo(s.call("list.len", intT, "", v), func(i *Value) {
					s.ifThen(s.add(OpGt, boolT, nil, i, s.cint(0)), func() { s.writeVar(av, lit(s.read(av), ", ")) })
					s.writeVar(av, sub(s.read(av), s.call("list.index", et, "", v, i), n.Args[0]))
				})
				s.retv(lit(s.read(av), "]"))
				return
			case "Map", "Set":
				K, V := mapKey(t), mapVal(t)
				keys := s.add(OpField, t.Fields[0].Type, 0, v)
				vals := s.add(OpField, t.Fields[1].Type, 1, v)
				live := s.add(OpField, t.Fields[2].Type, 2, v)
				av, first := local("acc"), local("first")
				open := "{"
				if obj.Name == "Set" {
					open = "#{"
				}
				s.writeVar(av, lit(acc, open))
				s.writeVar(first, s.bool(true))
				s.forUpTo(s.call("list.len", intT, "", keys), func(i *Value) {
					s.ifThen(s.call("list.index", boolT, "", live, i), func() {
						s.ifThen(s.add(OpNot, boolT, nil, s.read(first)), func() { s.writeVar(av, lit(s.read(av), ", ")) })
						s.writeVar(first, s.bool(false))
						a := sub(s.read(av), s.call("list.index", K, "", keys, i), n.Args[0])
						if obj.Name == "Map" {
							a = lit(a, ": ")
							a = sub(a, s.call("list.index", V, "", vals, i), n.Args[1])
						}
						s.writeVar(av, a)
					})
				})
				s.retv(lit(s.read(av), "}"))
				return
			}
		}
		sb := check.NewSubst(obj.TParams, n.Args)
		switch {
		case t.Kind == KStruct && obj.Kind == check.KNewtype && len(t.Fields) == 1:
			// `Email("a@b")` (std/rt's Debug of a newtype)
			a := lit(acc, obj.Name+"(")
			a = sub(a, s.add(OpField, t.Fields[0].Type, 0, v), sb.Apply(obj.Target))
			s.retv(lit(a, ")"))
			return
		case t.Kind == KStruct && obj.Kind == check.KStruct:
			if obj.Opaque {
				s.retv(lit(acc, "<"+lowerName(obj.Name)+">"))
				return
			}
			a := lit(acc, obj.Name+"{")
			for i, f := range obj.Fields {
				if i > 0 {
					a = lit(a, ", ")
				}
				a = lit(a, f.Name+": ")
				fi := t.FieldIndex(f.Name)
				if fi < 0 {
					b.unsupported(at, "the Debug text of `"+ct.String()+"`")
					continue
				}
				a = sub(a, s.add(OpField, t.Fields[fi].Type, fi, v), sb.Apply(f.Type))
			}
			s.retv(lit(a, "}"))
			return
		case t.Kind == KEnum && obj.Kind == check.KEnum:
			tag := s.add(OpTag, intT, nil, v)
			for _, vo := range obj.Variants {
				vi := t.VariantIndex(vo.Name)
				if vi < 0 {
					continue
				}
				s.ifThen(s.add(OpEq, boolT, nil, tag, s.cint(int64(vi))), func() {
					if len(vo.Fields) == 0 {
						s.retv(lit(acc, vo.Name))
						return
					}
					a := lit(acc, vo.Name+"(")
					for fi, f := range vo.Fields {
						sep := ""
						if fi > 0 {
							sep = ", "
						}
						// Option and Result print as Some(x), Ok(x), Err(e) (std/rt option.go)
						if !f.From && f.Name != "" && !isPreludeNamed(ct, "Option", "Result") {
							sep += f.Name + ": "
						}
						if sep != "" {
							a = lit(a, sep)
						}
						ft := t.Variants[vi].Fields[fi].Type
						a = sub(a, s.add(OpPayload, ft, PayloadRef{vi, fi}, v), sb.Apply(f.Type))
					}
					s.retv(lit(a, ")"))
				})
			}
			s.retv(acc)
			return
		}
		b.unsupported(at, "the Debug text of `"+ct.String()+"`")
		s.retv(acc)
	})
}

// lowerName is a type name in lower case for handles: `<chan>`.
func lowerName(n string) string {
	for i := 0; i < len(n); i++ {
		if n[i] == '[' {
			n = n[:i]
			break
		}
	}
	out := []byte(n)
	for i, c := range out {
		if c >= 'A' && c <= 'Z' {
			out[i] = c + 32
		}
	}
	return string(out)
}

// ifaceCall lowers a call of an interface method: on a value of a type
// parameter constrained by an interface (the method of the concrete
// type, adapted to the interface's signature), on an interface value (a
// call through its function value, build_iface.go), or on a value of
// Error, Display, or Debug (its text).
func (s *fn) ifaceCall(x *ast.CallExpr, ci *check.Call) *Value {
	b := s.b
	fo := ci.Fn
	nt := s.natOf(x)
	if ci.Recv == nil {
		b.unsupported(x, "the interface method `"+fo.DisplayName()+"`")
		return s.undef(nt)
	}
	ct := s.natCheck(ci.Recv)
	v := s.expr(ci.Recv)
	st := b.mod.Builtin(KStr)
	if isDyn(v.Type) && len(x.Args) == 0 {
		if fo.Name == "debug" {
			return s.dynDebug(s.add(OpConst, st, ""), v)
		}
		if fo.Name == "display" {
			return s.add(OpField, st, 0, v)
		}
		b.unsupported(x, "the interface method `"+fo.DisplayName()+"` of an interface value")
		return s.undef(nt)
	}
	// the arguments, against the interface method's parameters
	var pts []*Type
	if isIface(v.Type) {
		j := v.Type.FieldIndex(fo.Name)
		if j < 0 {
			b.unsupported(x, "the interface method `"+fo.DisplayName()+"` of `"+ct.String()+"`")
			return s.undef(nt)
		}
		pts = v.Type.Fields[j].Type.Elems
	} else {
		sub := ifaceSubst(ci, fo)
		for _, p := range fo.Params {
			pts = append(pts, b.typ(s.apply(sub.Apply(p.Type)), x))
		}
	}
	exprs := argExprs(fo.Params, x)
	var args []*Value
	for i, e := range exprs {
		if e == nil || fo.Params[i].Var || i >= len(pts) {
			b.unsupported(x, "a call of the interface method `"+fo.DisplayName()+"` with a missing or `var` argument")
			return s.undef(nt)
		}
		args = append(args, s.valueAs(e, pts[i]))
	}
	var toErr check.Type
	if fo.Err != nil {
		if _, et, ok := resultParts(s.natCheck(x)); ok {
			toErr = check.Deep(et)
		}
	}
	return s.callResult(s.methodOn(fo.Name, ct, v, args, nt, failErr(toErr), x), x)
}

// ifaceSubst maps the type parameters of the interface declaring the
// method fo to the type arguments of the call ci.
func ifaceSubst(ci *check.Call, fo *check.FuncObj) *check.Subst {
	var args []check.Type
	for _, tp := range fo.Recv.TParams {
		a := ci.Subst.Type(tp)
		if a == nil {
			a = &check.ParamType{P: tp}
		}
		args = append(args, a)
	}
	return check.NewSubst(fo.Recv.TParams, args)
}
