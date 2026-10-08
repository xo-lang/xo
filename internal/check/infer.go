package check

import (
	"github.com/xo-lang/xo/internal/token"
)

// bind sets v to t, recording the change on the trail so a failed
// tentative unification can be undone.
func (c *checker) bind(v *Var, t Type) {
	v.Ref = t
	c.trail = append(c.trail, func() { v.Ref = nil })
}

func (c *checker) bindEff(v *EffVar, s *EffSet) {
	v.Ref = s
	c.trail = append(c.trail, func() { v.Ref = nil })
}

func (c *checker) mark() int { return len(c.trail) }

func (c *checker) undo(m int) {
	for i := len(c.trail) - 1; i >= m; i-- {
		c.trail[i]()
	}
	c.trail = c.trail[:m]
}

// tryUnify unifies a and b, undoing all bindings if it fails.
func (c *checker) tryUnify(a, b Type) bool {
	m := c.mark()
	if c.unify(a, b) {
		return true
	}
	c.undo(m)
	return false
}

// unify makes a and b identical by binding inference variables. Literal
// variables (`lit`) only bind to numeric types of their kind.
func (c *checker) unify(a, b Type) bool {
	a, b = resolve(a), resolve(b)
	if a == b {
		return true
	}
	if _, ok := a.(*BadType); ok {
		return true
	}
	if _, ok := b.(*BadType); ok {
		return true
	}
	if va, ok := a.(*Var); ok {
		return c.bindVar(va, b)
	}
	if vb, ok := b.(*Var); ok {
		return c.bindVar(vb, a)
	}
	switch x := a.(type) {
	case *Named:
		y, ok := b.(*Named)
		if !ok || x.Obj != y.Obj || len(x.Args) != len(y.Args) {
			return false
		}
		for i := range x.Args {
			if !c.unify(x.Args[i], y.Args[i]) {
				return false
			}
		}
		return true
	case *Tuple:
		y, ok := b.(*Tuple)
		if !ok || len(x.Elems) != len(y.Elems) {
			return false
		}
		for i := range x.Elems {
			if !c.unify(x.Elems[i], y.Elems[i]) {
				return false
			}
		}
		return true
	case *Func:
		y, ok := b.(*Func)
		if !ok || len(x.Params) != len(y.Params) {
			return false
		}
		for i := range x.Params {
			if !c.unify(x.Params[i].T, y.Params[i].T) {
				return false
			}
		}
		if !c.unify(x.Result, y.Result) {
			return false
		}
		xe, ye := x.Err, y.Err
		if xe == nil && ye == nil {
			// both cannot fail
		} else if xe == nil {
			if !c.unify(ye, c.named("Never")) {
				return false
			}
		} else if ye == nil {
			if !c.unify(xe, c.named("Never")) {
				return false
			}
		} else if !c.unify(xe, ye) {
			return false
		}
		c.unifyEff(x.Eff, y.Eff)
		return true
	case *ParamType:
		y, ok := b.(*ParamType)
		return ok && x.P == y.P
	case *EffArg:
		y, ok := b.(*EffArg)
		if ok {
			c.unifyEff(x.Set, y.Set)
		}
		return ok
	}
	return false
}

func (c *checker) bindVar(v *Var, t Type) bool {
	if tv, ok := t.(*Var); ok {
		if tv == v {
			return true
		}
		if v.lit != "" && tv.lit == "" {
			c.bind(tv, v)
			return true
		}
		if v.lit != "" && tv.lit != "" && v.lit != tv.lit {
			return false
		}
		c.bind(v, t)
		return true
	}
	if v.lit == "int" && !c.isInt(t) {
		return false
	}
	if v.lit == "float" && !c.isFloat(t) {
		return false
	}
	if occurs(v, t) {
		return false
	}
	c.bind(v, t)
	return true
}

// unifyEff binds open effect variables of either side to the other.
func (c *checker) unifyEff(a, b *EffSet) {
	if v := a.openVar(); v != nil && b != nil && b.openVar() != v {
		c.bindEff(v, b)
		return
	}
	if v := b.openVar(); v != nil && a != nil && a.openVar() != v {
		c.bindEff(v, a)
	}
}

// assignable reports whether a value of type src may be used where dst is
// expected: identical types (after inference), Never, and the implicit
// conversions of core 2.10 (Some wrapping and interface conversion; error
// conversion is handled at `?`). Function values are subsumed (core 6.2):
// effects are recorded as an obligation at sp.
func (c *checker) assignable(src, dst Type, sp token.Span) bool {
	src, dst = resolve(src), resolve(dst)
	if isBad(src) || isBad(dst) || c.isNever(src) {
		return true
	}
	if sf, ok := src.(*Func); ok {
		if df, ok := dst.(*Func); ok {
			return c.funcAssignable(sf, df, sp)
		}
	}
	if c.tryUnify(src, dst) {
		return true
	}
	if _, ok := dst.(*Var); ok {
		return false
	}
	if elem := c.optionElem(dst); elem != nil && c.optionElem(src) == nil {
		if _, open := src.(*Var); !open && c.assignable(src, elem, sp) {
			return true
		}
	}
	if c.isInterface(dst) {
		if _, open := src.(*Var); open {
			return false
		}
		if c.implements(src, dst.(*Named), sp) {
			return true
		}
	}
	return false
}

// funcAssignable implements subsumption of function values (core 6.2):
// same parameters, a result that is assignable, an error that converts (or
// none), and effects within the expected ones.
func (c *checker) funcAssignable(sf, df *Func, sp token.Span) bool {
	if len(sf.Params) != len(df.Params) {
		return false
	}
	m := c.mark()
	for i := range sf.Params {
		if !c.unify(sf.Params[i].T, df.Params[i].T) {
			c.undo(m)
			return false
		}
	}
	if !c.unify(sf.Result, df.Result) && !c.isNever(sf.Result) {
		c.undo(m)
		return false
	}
	switch {
	case sf.Err == nil:
		if df.Err != nil && isOpenVar(df.Err) {
			c.unify(df.Err, c.named("Never"))
		}
	case df.Err == nil:
		if isOpenVar(sf.Err) {
			c.unify(sf.Err, c.named("Never"))
		} else if !c.isNever(sf.Err) {
			c.undo(m)
			return false
		}
	default:
		if !c.unify(sf.Err, df.Err) && !c.errConverts(sf.Err, df.Err) {
			c.undo(m)
			return false
		}
	}
	if v := df.Eff.openVar(); v != nil {
		if sf.Eff != nil {
			c.bindEff(v, sf.Eff)
		} else {
			c.bindEff(v, &EffSet{})
		}
	} else if sf.Eff != nil && len(sf.Eff.Items) > 0 && sp.File != "" {
		sup := df.Eff
		if sup == nil {
			sup = &EffSet{}
		}
		want := df
		c.obligations = append(c.obligations, effObligation{
			sub: sf.Eff, sup: sup, span: sp,
			what: func(s string) string {
				if s == "" {
					return "the expected function type `" + deep(want).String() + "` is pure"
				}
				return "the expected function type allows only `" + s + "`"
			},
		})
	}
	return true
}

// errConverts implements the error conversion at a `?` site (core 2.6):
// E2 is E, E is Error and E2 implements Display, or E has a `from E2`
// variant. Never converts to anything.
func (c *checker) errConverts(from, to Type) bool {
	from, to = resolve(from), resolve(to)
	if isBad(from) || isBad(to) || c.isNever(from) {
		return true
	}
	if identical(from, to) {
		return true
	}
	if c.isPrelude(to, "Error") {
		return c.implementsName(from, "Display", token.Span{})
	}
	if c.fromVariant(to, from) != nil {
		return true
	}
	return false
}

// fromVariant returns the `from` variant of enum `to` that wraps `from`.
func (c *checker) fromVariant(to, from Type) *VariantObj {
	n, ok := resolve(to).(*Named)
	if !ok || n.Obj.Kind != KEnum {
		return nil
	}
	s := typeArgsSubst(n.Obj.TParams, n.Args)
	for _, v := range n.Obj.Variants {
		if len(v.Fields) == 1 && v.Fields[0].From {
			if identical(subst(v.Fields[0].Type, s), from) {
				return v
			}
		}
	}
	return nil
}

// implementsName checks t against a prelude interface by name.
func (c *checker) implementsName(t Type, iface string, sp token.Span) bool {
	return c.implements(t, c.named(iface), sp)
}

// implements reports whether t satisfies interface iface (core 2.4, std.md
// 1). Effect requirements of method implementations are recorded as
// obligations at sp.
func (c *checker) implements(t Type, iface *Named, sp token.Span) bool {
	t = resolve(t)
	if isBad(t) || c.isNever(t) {
		return true
	}
	if _, ok := t.(*Var); ok {
		return true // decided later; never cascade
	}
	io := iface.Obj
	if io.Module == c.prelude {
		switch io.Name {
		case "Debug", "Eq", "Hash":
			return c.isEqType(t, io.Name)
		case "Ord":
			return c.isOrd(t)
		case "Json", "OpenApi", "Proto":
			return c.isSerializable(t, io.Name)
		case "SqlRow":
			return c.isSqlRow(t)
		case "Display", "Error":
			if c.isDisplayBuiltin(t) {
				return true
			}
		}
	}
	switch x := t.(type) {
	case *ParamType:
		if x.P.Constraint != nil {
			if cn, ok := resolve(x.P.Constraint).(*Named); ok {
				if cn.Obj == io || (c.isDisplayLike(io) && c.isDisplayLike(cn.Obj)) {
					return true
				}
				// a constraint interface with a superset of methods
				return c.structural(t, iface, sp)
			}
		}
		return false
	case *Named:
		if x.Obj == io {
			return true
		}
		if x.Obj.Kind == KInterface && c.isDisplayLike(io) && c.isDisplayLike(x.Obj) {
			return true
		}
		if x.Obj.Derives[io.Name] && io.Module == c.prelude {
			return true
		}
		if io.Module == c.prelude && (io.Name == "Display" || io.Name == "Error") && x.Obj.Derives["Display"] {
			return true
		}
		return c.structural(t, iface, sp)
	}
	return false
}

func (c *checker) isDisplayLike(o *TypeObj) bool {
	return o.Module == c.prelude && (o.Name == "Display" || o.Name == "Error")
}

// structural checks that t has every method of iface with a matching
// signature. Methods need not be pub (core 2.4).
func (c *checker) structural(t Type, iface *Named, sp token.Span) bool {
	is := typeArgsSubst(iface.Obj.TParams, iface.Args)
	for _, im := range iface.Obj.IfaceMeths {
		m, ms := c.findMethod(t, im.Name)
		if m == nil || m.Static {
			return false
		}
		if len(m.Params) != len(im.Params) {
			return false
		}
		// map the method's own type parameters onto the interface method's
		own := m.TParams[len(m.TParams)-len(m.Decl.TypeParams):]
		iown := im.TParams[len(im.TParams)-len(im.Decl.TypeParams):]
		if len(own) != len(iown) {
			return false
		}
		ms2 := &substMap{types: map[*TParam]Type{}, effs: map[*TParam]*EffSet{}}
		if ms != nil {
			for k, v := range ms.types {
				ms2.types[k] = v
			}
		}
		for i := range own {
			ms2.types[own[i]] = &ParamType{P: iown[i]}
		}
		for i, p := range m.Params {
			if p.Var != im.Params[i].Var || !identical(subst(p.Type, ms2), subst(im.Params[i].Type, is)) {
				return false
			}
		}
		if !identical(subst(m.Result, ms2), subst(im.Result, is)) {
			return false
		}
		if m.Err != nil {
			if im.Err == nil || !(identical(subst(m.Err, ms2), subst(im.Err, is)) || c.errConverts(subst(m.Err, ms2), subst(im.Err, is))) {
				return false
			}
		}
		if sp.File != "" {
			var sub *EffSet
			if m.Declared != nil {
				sub = m.Declared.performed(m.Span, m.DisplayName())
			} else {
				sub = &EffSet{Items: []EffItem{{Fn: m, Span: m.Span}}}
			}
			sup := im.Declared
			if sup == nil {
				sup = &EffSet{}
			}
			mm, ii := m, im
			tt := t
			c.obligations = append(c.obligations, effObligation{sub: sub, sup: sup, span: sp, what: func(s string) string {
				if s == "" {
					return "`" + tt.String() + "` is used as `" + iface.String() + "`, whose method `" + ii.Name + "` is pure, but `" + mm.DisplayName() + "`"
				}
				return "`" + tt.String() + "` is used as `" + iface.String() + "`, whose method `" + ii.Name + "` allows only `" + s + "`, but `" + mm.DisplayName() + "`"
			}})
		}
	}
	return true
}

// findMethod finds method name on t (any visibility) and returns the
// substitution of its receiver type parameters.
func (c *checker) findMethod(t Type, name string) (*FuncObj, *substMap) {
	switch x := resolve(t).(type) {
	case *Named:
		if x.Obj.Kind == KInterface {
			for _, m := range x.Obj.IfaceMeths {
				if m.Name == name {
					return m, typeArgsSubst(x.Obj.TParams, x.Args)
				}
			}
			return nil, nil
		}
		m := x.Obj.Methods[name]
		if m == nil {
			if x.Obj.Kind == KNewtype || x.Obj.Kind == KAlias {
				return nil, nil
			}
			return nil, nil
		}
		s, ok := c.matchRecv(m, x)
		if !ok {
			return nil, nil
		}
		return m, s
	case *ParamType:
		if x.P.Constraint != nil {
			return c.findMethod(x.P.Constraint, name)
		}
	}
	return nil, nil
}

// matchRecv binds the implicit receiver parameters of m against the
// receiver type t without inference variables.
func (c *checker) matchRecv(m *FuncObj, t *Named) (*substMap, bool) {
	s := &substMap{types: map[*TParam]Type{}, effs: map[*TParam]*EffSet{}}
	rt, ok := m.RecvType.(*Named)
	if !ok {
		return s, true
	}
	if len(rt.Args) != len(t.Args) {
		return s, true
	}
	for i, a := range rt.Args {
		if p, ok := a.(*ParamType); ok && p.P.Implicit {
			s.types[p.P] = t.Args[i]
			continue
		}
		if ea, ok := a.(*EffArg); ok {
			if ta, ok := t.Args[i].(*EffArg); ok && len(ea.Set.Items) == 1 && ea.Set.Items[0].Param != nil {
				s.effs[ea.Set.Items[0].Param] = ta.Set
			}
			continue
		}
		if !identical(a, t.Args[i]) && !isOpenVar(t.Args[i]) {
			return nil, false
		}
	}
	return s, true
}

// isEqType decides the builtin Eq, Hash, and Debug interfaces: every type
// except function types (Debug: every type).
func (c *checker) isEqType(t Type, name string) bool {
	if name == "Debug" {
		return true
	}
	switch x := resolve(t).(type) {
	case *Func:
		return false
	case *Tuple:
		for _, e := range x.Elems {
			if !c.isEqType(e, name) {
				return false
			}
		}
	case *Named:
		for _, a := range x.Args {
			if !c.isEqType(a, name) {
				return false
			}
		}
	}
	return true
}

var ordBuiltins = map[string]bool{"Int": true, "Int8": true, "Int16": true, "Int32": true, "Int64": true, "UInt8": true, "UInt16": true, "UInt32": true, "UInt64": true, "Float": true, "Float32": true, "Rune": true, "Str": true, "Duration": true, "Time": true, "Bool": true, "Bytes": true}

func (c *checker) isOrd(t Type) bool {
	switch x := resolve(t).(type) {
	case *Named:
		if x.Obj.Module == c.prelude && ordBuiltins[x.Obj.Name] {
			return true
		}
		if x.Obj.Derives["Ord"] {
			return true
		}
		if x.Obj.Module == c.prelude && (x.Obj.Name == "Option" || x.Obj.Name == "List") {
			return c.isOrd(x.Args[0])
		}
	case *Tuple:
		for _, e := range x.Elems {
			if !c.isOrd(e) {
				return false
			}
		}
		return true
	case *ParamType:
		if x.P.Constraint != nil {
			if cn, ok := resolve(x.P.Constraint).(*Named); ok {
				return cn.Obj.Module == c.prelude && cn.Obj.Name == "Ord"
			}
		}
	}
	return false
}

var jsonBuiltins = map[string]bool{"Int": true, "Int8": true, "Int16": true, "Int32": true, "Int64": true, "UInt8": true, "UInt16": true, "UInt32": true, "UInt64": true, "Float": true, "Float32": true, "Bool": true, "Str": true, "Duration": true, "Time": true, "Bytes": true}

// isSerializable decides Json (std.md 5) and OpenApi, Proto (derive only).
func (c *checker) isSerializable(t Type, name string) bool {
	switch x := resolve(t).(type) {
	case *Named:
		if x.Obj.Derives[name] {
			return true
		}
		if name != "Json" || x.Obj.Module != c.prelude {
			return false
		}
		if jsonBuiltins[x.Obj.Name] {
			return true
		}
		switch x.Obj.Name {
		case "List", "Set", "Option":
			return c.isSerializable(x.Args[0], name)
		case "Map":
			return c.isPrelude(x.Args[0], "Str") && c.isSerializable(x.Args[1], name)
		}
	case *ParamType:
		if x.P.Constraint != nil {
			if cn, ok := resolve(x.P.Constraint).(*Named); ok {
				return cn.Obj.Module == c.prelude && cn.Obj.Name == name
			}
		}
	case *Tuple:
		return len(x.Elems) == 0
	}
	return false
}

// isSqlRow decides SqlRow (std.md 10.4): only `derive SqlRow` provides it.
func (c *checker) isSqlRow(t Type) bool {
	switch x := resolve(t).(type) {
	case *Named:
		return x.Obj.Derives["SqlRow"]
	case *ParamType:
		if x.P.Constraint != nil {
			if cn, ok := resolve(x.P.Constraint).(*Named); ok {
				return cn.Obj.Module == c.prelude && cn.Obj.Name == "SqlRow"
			}
		}
	}
	return false
}

// sqlColumnTypes are the field types `derive SqlRow` decodes (alone or in
// an Option): the value types of std/sql rows.
var sqlColumnTypes = map[string]bool{"Int": true, "Float": true, "Str": true, "Bool": true, "Time": true}

// isSqlColumn reports whether a field of type t can be decoded from a column.
func (c *checker) isSqlColumn(t Type) bool {
	n, ok := resolve(t).(*Named)
	if !ok || n.Obj.Module != c.prelude {
		return false
	}
	if n.Obj.Name == "Option" && len(n.Args) == 1 {
		e, ok := resolve(n.Args[0]).(*Named)
		return ok && e.Obj.Module == c.prelude && sqlColumnTypes[e.Obj.Name]
	}
	return sqlColumnTypes[n.Obj.Name]
}

var displayBuiltins = map[string]bool{"Int": true, "Int8": true, "Int16": true, "Int32": true, "Int64": true, "UInt8": true, "UInt16": true, "UInt32": true, "UInt64": true, "Float": true, "Float32": true, "Bool": true, "Rune": true, "Str": true, "Duration": true, "Time": true, "Secret": true}

func (c *checker) isDisplayBuiltin(t Type) bool {
	n, ok := resolve(t).(*Named)
	return ok && n.Obj.Module == c.prelude && displayBuiltins[n.Obj.Name]
}
