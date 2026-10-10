package ssa

import (
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Interface values on the native backends (decision 0100).
//
// A value of an interface type (other than the prelude's Error, Display,
// and Debug, which are their text, build_show.go, and the capability
// interfaces, which are capabilities) is the struct
//
//	xo.iface[I] {m1: fn(...) -> R1, ..., mk: fn(...) -> Rk, xo.show: fn(Str, Bool) -> Str}
//
// with one function value per method of I in declaration order, each a
// closure over the concrete value whose code calls the concrete type's
// method (adapting its result: `Ok` around a method that cannot fail
// where the interface's can, `from` conversion of its error), and a last
// closure that appends the concrete value's Display text (false) or
// Debug text (true) to its first argument. A call of an interface method
// is a field read and an indirect call; reference counting, sharing
// between tasks, and unwinding need nothing new, since closures already
// have them. A conversion costs one closure environment per method (the
// concrete value copied in by reference); calls cost what a closure call
// costs.

// The hidden fields of an interface value: the Display and Debug text,
// equality, and the concrete type's id.
const (
	ifaceShow = "xo.show"
	ifaceEq   = "xo.eq"
	ifaceTid  = "xo.tid"
)

// isIface reports whether t is the lowered type of an interface value.
func isIface(t *Type) bool { return t.Kind == KStruct && strings.HasPrefix(t.Name, "xo.iface[") }

// ifaceType lowers the interface type r.
func (b *builder) ifaceType(r *check.Named, at ast.Node) *Type {
	m := b.mod
	obj := r.Obj
	t, created := m.Named("xo.iface["+b.typeName(b.implNamed(r))+"]", KStruct)
	if !created {
		return t
	}
	if b.ifaces == nil {
		b.ifaces = map[*Type]*check.Named{}
	}
	b.ifaces[t] = r
	sub := check.NewSubst(obj.TParams, r.Args)
	var fs []Field
	for _, im := range obj.IfaceMeths {
		if len(im.Decl.TypeParams) > 0 {
			b.unsupported(at, "the generic interface method `"+obj.Name+"."+im.Name+"` (in a value of interface type)")
		}
		var ps []*Type
		for _, p := range im.Params {
			if p.Var {
				b.unsupported(at, "the interface method `"+obj.Name+"."+im.Name+"` with a `var` parameter")
			}
			ps = append(ps, b.typ(sub.Apply(p.Type), at))
		}
		ret := b.typ(b.resultOf(sub.Apply(im.Result), failErr(sub.Apply(im.Err))), at)
		fs = append(fs, Field{im.Name, m.FuncType(ps, ret)})
	}
	st, intT := m.Builtin(KStr), m.Builtin(KInt)
	show := m.FuncType([]*Type{st, m.Builtin(KBool)}, st)
	fs = append(fs, Field{ifaceEq, m.FuncType([]*Type{intT, show}, m.Builtin(KBool))})
	fs = append(fs, Field{ifaceTid, intT})
	fs = append(fs, Field{ifaceShow, show})
	t.Fields = fs
	return t
}

// ifaceMeth returns the method of the interface named r with the
// substitution of r's type arguments.
func ifaceMeth(r *check.Named, name string) (*check.FuncObj, *check.Subst) {
	for _, im := range r.Obj.IfaceMeths {
		if im.Name == name {
			return im, check.NewSubst(r.Obj.TParams, r.Args)
		}
	}
	return nil, nil
}

// toIface converts v (checker type from) to the interface value type t.
func (s *fn) toIface(v *Value, from check.Type, t *Type, at ast.Node) *Value {
	b := s.b
	r := b.ifaces[t]
	if r == nil || len(t.Fields) == 0 {
		b.unsupported(at, "converting `"+v.Type.Name+"` to `"+t.Name+"`")
		return s.undef(t)
	}
	from = check.Deep(from)
	var fields []*Value
	if isIface(v.Type) {
		// an interface value converted to another interface: reuse the
		// function values that already have the wanted type, adapt the
		// others; the hidden fields are the concrete value's
		for i, im := range r.Obj.IfaceMeths {
			ft := t.Fields[i].Type
			j := v.Type.FieldIndex(im.Name)
			if j < 0 {
				b.unsupported(at, "converting `"+v.Type.Name+"` to `"+t.Name+"`")
				return s.undef(t)
			}
			if v.Type.Fields[j].Type == ft {
				fields = append(fields, s.add(OpField, ft, j, v))
				continue
			}
			fields = append(fields, s.add(OpClosure, ft, b.ifaceThunk(t, i, from, v.Type, at), v))
		}
		for _, name := range []string{ifaceEq, ifaceTid, ifaceShow} {
			j := v.Type.FieldIndex(name)
			fields = append(fields, s.add(OpField, v.Type.Fields[j].Type, j, v))
		}
		return s.add(OpStruct, t, nil, fields...)
	}
	// one environment holds the value; every function shares it
	n := len(r.Obj.IfaceMeths)
	show := s.add(OpClosure, t.Fields[n+2].Type, b.showThunk(from, v.Type, at), v)
	for i := range r.Obj.IfaceMeths {
		fields = append(fields, s.add(OpRebind, t.Fields[i].Type, b.ifaceThunk(t, i, from, v.Type, at), show))
	}
	fields = append(fields, s.add(OpRebind, t.Fields[n].Type, b.eqThunk(from, v.Type, at), show))
	fields = append(fields, s.cint(int64(b.typeID(from))))
	fields = append(fields, show)
	return s.add(OpStruct, t, nil, fields...)
}

// typeID numbers the concrete types held by interface values (the
// xo.tid field): `==` on two interface values first compares them.
func (b *builder) typeID(ct check.Type) int {
	key := b.implNamed(check.Deep(ct)).String()
	if b.tids == nil {
		b.tids = map[string]int{}
	}
	id, ok := b.tids[key]
	if !ok {
		id = len(b.tids) + 1
		b.tids[key] = id
	}
	return id
}

// eqThunk returns the code of the xo.eq function of an interface value
// holding a value of checker type ct (SSA type vt): given the other
// value's type id and xo.show closure, it reports whether the other holds
// a value of the same type equal to this one. The other's value is read
// out of its show closure's environment, which holds exactly one value
// of type vt when the type ids agree (the layout of this function's own
// environment).
func (b *builder) eqThunk(ct check.Type, vt *Type, at ast.Node) string {
	m := b.mod
	st, boolT, intT := m.Builtin(KStr), m.Builtin(KBool), m.Builtin(KInt)
	show := m.FuncType([]*Type{st, boolT}, st)
	ft := m.FuncType([]*Type{intT, show}, boolT)
	name := "xo.ifeq[" + b.implNamed(ct).String() + "]"
	if f := m.Func(name); f != nil {
		return name
	}
	f := m.NewFunc(name, []*Type{ft, intT, show}, []*Type{boolT})
	f.Indirect = true
	f.Captures = []*Type{vt}
	s := b.newFn(f, nil, nil)
	ps := f.Params()
	same := s.add(OpEq, boolT, nil, ps[1], s.cint(int64(b.typeID(ct))))
	s.ifThen(s.add(OpNot, boolT, nil, same), func() { s.retv(s.bool(false)) })
	if holdsKind(vt, KFunc, map[*Type]bool{}) {
		// a value holding a function equals nothing (std/rt's EqAny:
		// reflect.DeepEqual of functions that are not nil)
		s.retv(s.bool(false))
		s.finishFn()
		return name
	}
	self := s.add(OpCapture, vt, 0, ps[0])
	other := s.add(OpCapture, vt, 0, ps[2])
	if holdsColl(vt, map[*Type]bool{}) {
		s.retv(s.call(b.eqColl(vt), boolT, "", self, other))
	} else {
		s.retv(s.eqValues(self, other))
	}
	s.finishFn()
	return name
}

// ifaceEqual is `l == r` on two interface values of one type.
func (s *fn) ifaceEqual(l, r *Value) *Value {
	t := l.Type
	m := s.b.mod
	ei, ti, si := t.FieldIndex(ifaceEq), t.FieldIndex(ifaceTid), t.FieldIndex(ifaceShow)
	fv := s.add(OpField, t.Fields[ei].Type, ei, l)
	tid := s.add(OpField, t.Fields[ti].Type, ti, r)
	show := s.add(OpField, t.Fields[si].Type, si, r)
	res := s.add(OpCallValue, m.Builtin(KBool), nil, fv, tid, show)
	s.check("")
	return res
}

// holdsKind reports whether a value of t holds a value of kind k.
func holdsKind(t *Type, k Kind, seen map[*Type]bool) bool {
	if t.Kind == k {
		return true
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	for _, f := range t.Fields {
		if holdsKind(f.Type, k, seen) {
			return true
		}
	}
	for _, v := range t.Variants {
		for _, f := range v.Fields {
			if holdsKind(f.Type, k, seen) {
				return true
			}
		}
	}
	if t.Kind != KFunc {
		for _, e := range t.Elems {
			if holdsKind(e, k, seen) {
				return true
			}
		}
	}
	return false
}

// ifaceThunk returns the code of method i of the interface value type t
// for a concrete value of checker type ct (SSA type vt): an Indirect
// function whose environment holds the value.
func (b *builder) ifaceThunk(t *Type, i int, ct check.Type, vt *Type, at ast.Node) string {
	r := b.ifaces[t]
	im := r.Obj.IfaceMeths[i]
	ft := t.Fields[i].Type
	name := t.Name + "." + b.implNamed(ct).String() + "." + im.Name
	if f := b.mod.Func(name); f != nil {
		return name
	}
	f := b.mod.NewFunc(name, append([]*Type{ft}, ft.Elems...), []*Type{ft.Ret})
	f.Indirect = true
	f.Captures = []*Type{vt}
	s := b.newFn(f, nil, nil)
	ps := f.Params()
	self := s.add(OpCapture, vt, 0, ps[0])
	sub := check.NewSubst(r.Obj.TParams, r.Args)
	res := s.methodOn(im.Name, ct, self, ps[1:], ft.Ret, failErr(sub.Apply(im.Err)), at)
	if !s.isDead() {
		s.retv(res)
	}
	s.finishFn()
	return name
}

// showThunk returns the code of the xo.show function of an interface
// value holding a value of checker type ct (SSA type vt).
func (b *builder) showThunk(ct check.Type, vt *Type, at ast.Node) string {
	m := b.mod
	st, boolT := m.Builtin(KStr), m.Builtin(KBool)
	ft := m.FuncType([]*Type{st, boolT}, st)
	name := "xo.show[" + b.implNamed(ct).String() + "]"
	if f := m.Func(name); f != nil {
		return name
	}
	f := m.NewFunc(name, []*Type{ft, st, boolT}, []*Type{st})
	f.Indirect = true
	f.Captures = []*Type{vt}
	s := b.newFn(f, nil, nil)
	ps := f.Params()
	self := s.add(OpCapture, vt, 0, ps[0])
	yes, no := s.branch(ps[2])
	s.cur = yes
	if m := s.methodOf(ct, "debug"); m != nil && !s.displayable(ct) {
		s.retv(s.call("str.concat", st, "", ps[1], s.callMethod(m, ct, self, at)))
	} else {
		s.retv(s.appendDebug(ps[1], self, ct, at))
	}
	s.cur = no
	s.retv(s.appendDisplay(ps[1], self, ct, at))
	s.finishFn()
	return name
}

// ifaceShowCall appends the Display (debug false) or Debug text of the
// interface value v to acc.
func (s *fn) ifaceShowCall(acc, v *Value, debug bool) *Value {
	last := v.Type.FieldIndex(ifaceShow)
	fv := s.add(OpField, v.Type.Fields[last].Type, last, v)
	r := s.add(OpCallValue, s.b.mod.Builtin(KStr), nil, fv, acc, s.bool(debug))
	s.check("")
	return r
}

// methodOn calls the method name of a value recv of checker type ct
// (a concrete type, an interface value, or a value of a prelude
// interface lowered as its text) with args, and adapts the result to
// want, whose error type (when it is a Result) is toErr.
func (s *fn) methodOn(name string, ct check.Type, recv *Value, args []*Value, want *Type, toErr check.Type, at ast.Node) *Value {
	b := s.b
	st := b.mod.Builtin(KStr)
	ct = check.Deep(ct)
	if isIface(recv.Type) {
		j := recv.Type.FieldIndex(name)
		src := b.ifaces[recv.Type]
		if j < 0 || src == nil {
			b.unsupported(at, "the method `"+name+"` of `"+ct.String()+"`")
			return s.undef(want)
		}
		ft := recv.Type.Fields[j].Type
		fv := s.add(OpField, ft, j, recv)
		r := s.add(OpCallValue, ft.Ret, nil, append([]*Value{fv}, args...)...)
		s.check("")
		im, sub := ifaceMeth(src, name)
		return s.adaptResult(r, failErr(sub.Apply(im.Err)), want, toErr, at)
	}
	if m := s.methodOf(ct, name); m != nil {
		n := check.Resolve(ct).(*check.Named)
		targs := append([]check.Type(nil), n.Args...)
		if len(typeParams(m)) != len(targs) || m.RecvVar {
			b.unsupported(at, "the method `"+m.DisplayName()+"` of `"+ct.String()+"` through an interface")
			return s.undef(want)
		}
		callee := b.declare(m, targs, at)
		all := append([]*Value{recv}, args...)
		if len(callee.Params()) != len(all) || len(callee.Results) != 1 {
			b.unsupported(at, "the method `"+m.DisplayName()+"` of `"+ct.String()+"` through an interface")
			return s.undef(want)
		}
		r := s.call(callee.Name, callee.Results[0], "", all...)
		s.check(callee.Name)
		fromErr := failErr(check.NewSubst(typeParams(m), targs).Apply(m.Err))
		if fromErr != nil {
			fromErr = check.Deep(fromErr)
		}
		return s.adaptResult(r, fromErr, want, toErr, at)
	}
	if r := s.inlineMethod(name, recv, args); r != nil {
		return s.adaptResult(r, nil, want, toErr, at)
	}
	if len(args) == 0 {
		switch name {
		case "display":
			return s.adaptResult(s.appendDisplay(s.add(OpConst, st, ""), recv, ct, at), nil, want, toErr, at)
		case "debug":
			return s.adaptResult(s.appendDebug(s.add(OpConst, st, ""), recv, ct, at), nil, want, toErr, at)
		}
	}
	b.unsupported(at, "the method `"+name+"` of `"+ct.String()+"` through an interface")
	return s.undef(want)
}

// adaptResult converts r, the result of a method whose error type is
// fromErr (nil: it cannot fail), to want, the result of the interface's
// method, whose error type is toErr (core 2.4: a method that cannot fail
// implements one that can; its error converts as at a `?`).
func (s *fn) adaptResult(r *Value, fromErr check.Type, want *Type, toErr check.Type, at ast.Node) *Value {
	b := s.b
	m := b.mod
	if r.Type == want || r.Op == OpUndef || s.isDead() {
		return s.coerce(r, want, at)
	}
	okI, errI := want.VariantIndex("Ok"), want.VariantIndex("Err")
	if toErr == nil || want.Kind != KEnum || okI < 0 || errI < 0 {
		b.unsupported(at, "converting `"+r.Type.Name+"` to `"+want.Name+"`")
		return s.undef(want)
	}
	okT := want.Variants[okI].Fields[0].Type
	if fromErr == nil {
		return s.add(OpVariant, want, okI, s.coerce(r, okT, at))
	}
	rOk, rErr := r.Type.VariantIndex("Ok"), r.Type.VariantIndex("Err")
	if r.Type.Kind != KEnum || rOk < 0 || rErr < 0 {
		b.unsupported(at, "converting `"+r.Type.Name+"` to `"+want.Name+"`")
		return s.undef(want)
	}
	join := s.newBlock()
	res := join.AddParam(want)
	isErr := s.add(OpEq, m.Builtin(KBool), nil, s.add(OpTag, m.Builtin(KInt), nil, r), s.cint(int64(rErr)))
	yes, no := s.branch(isErr)
	s.cur = yes
	ev := s.add(OpPayload, r.Type.Variants[rErr].Fields[0].Type, PayloadRef{rErr, 0}, r)
	s.jump(join, s.add(OpVariant, want, errI, s.coerceErr(ev, fromErr, toErr, at)))
	s.cur = no
	ov := s.add(OpPayload, r.Type.Variants[rOk].Fields[0].Type, PayloadRef{rOk, 0}, r)
	s.jump(join, s.add(OpVariant, want, okI, s.coerce(ov, okT, at)))
	s.seal(join)
	s.cur = join
	return res
}

// inlineMethod lowers the prelude methods the builder writes inline
// (stdCall, mapCall) on a value recv, for calls through an interface or
// a type parameter: len and is_empty of List, Str, Map, and Set, and
// contains of Map and Set. It is nil for any other method.
func (s *fn) inlineMethod(name string, recv *Value, args []*Value) *Value {
	b := s.b
	m := b.mod
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	t := recv.Type
	var n *Value
	switch {
	case t.Coll:
		n = s.add(OpField, intT, 4, recv)
	case t.Kind == KList:
		n = s.call("list.len", intT, "", recv)
	case t.Kind == KStr:
		n = s.call("Str.len", intT, "", recv)
	default:
		return nil
	}
	switch {
	case name == "len" && len(args) == 0:
		return n
	case name == "is_empty" && len(args) == 0:
		return s.add(OpEq, boolT, nil, n, s.cint(0))
	case name == "contains" && len(args) == 1 && t.Coll && args[0].Type == mapKey(t):
		k := args[0]
		idx := s.call(b.mapFind(t, nil), intT, "", recv, k, s.call(b.hashFunc(mapKey(t), nil), intT, "", k))
		return s.add(OpGe, boolT, nil, idx, s.cint(0))
	}
	return nil
}
