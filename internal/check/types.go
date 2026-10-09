package check

import (
	"strings"

	"github.com/xo-lang/xo/internal/token"
)

// Type is an xo type. Types are compared structurally with identical or
// unified with inference variables (infer.go).
type Type interface {
	String() string
}

// Named is a nominal type: a struct, enum, interface, newtype, or builtin
// type, with its type arguments.
type Named struct {
	Obj  *TypeObj
	Args []Type
}

// Tuple is `(A, B)`. With no elements it is the unit type `()`.
type Tuple struct {
	Elems []Type
}

// FParam is a parameter of a function type.
type FParam struct {
	Var bool
	T   Type
}

// Func is a function type. Err is nil for a function that cannot fail
// (`T ! Never` is `T`, core 2.6). Eff is the effect set (nil means pure).
type Func struct {
	Params []FParam
	Result Type
	Err    Type
	Eff    *EffSet
}

// TParam is a type or effect parameter of a generic declaration.
type TParam struct {
	Name       string
	Constraint Type // interface, or nil
	IsEffect   bool
	Implicit   bool // bound by a method receiver type (std encoding rule 5)
	// Elided marks an implicit effect parameter created for an effect
	// argument that was left out (core 6.2): the function may perform it
	// without naming it in `uses`.
	Elided bool
	// ElidedAt is the type, written without its effect arguments, that
	// created an Elided parameter; ElidedText is its name.
	ElidedAt   token.Span
	ElidedText string
}

// ParamType is a reference to a type parameter inside a generic body or
// signature.
type ParamType struct {
	P *TParam
}

// Var is an inference variable. Ref is set once it is bound.
type Var struct {
	id  int
	Ref Type
	// lit marks a variable created for an integer or float literal whose
	// type is still open; it defaults to Int or Float.
	lit string
}

// EffArg is an effect set in the type argument list of a type with an
// effect parameter: `Cache[Int, Item, E, net.dial]` (core 6.2, 15).
type EffArg struct {
	Set *EffSet
}

func (t *EffArg) String() string {
	s := t.Set.String()
	if strings.Contains(s, ",") || s == "" {
		return "(" + s + ")"
	}
	return s
}

// BadType is the `<error>` type of a bad expression (core 10.2). It is
// compatible with everything so dependent diagnostics are suppressed.
type BadType struct{}

var (
	unitType = &Tuple{}
	badType  = &BadType{}
)

func (t *Named) String() string {
	if t.Obj == nil {
		return "<nil>"
	}
	if t.Obj.Name == "Option" && t.Obj.Module != nil && t.Obj.Module.IsPrelude && len(t.Args) == 1 {
		s := t.Args[0].String()
		if _, ok := resolve(t.Args[0]).(*Func); ok {
			s = "(" + s + ")"
		}
		return s + "?"
	}
	var b strings.Builder
	b.WriteString(t.Obj.QualName())
	if len(t.Args) > 0 {
		b.WriteByte('[')
		for i, a := range t.Args {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(a.String())
		}
		b.WriteByte(']')
	}
	return b.String()
}

func (t *Tuple) String() string {
	var b strings.Builder
	b.WriteByte('(')
	for i, e := range t.Elems {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(e.String())
	}
	if len(t.Elems) == 1 {
		b.WriteByte(',')
	}
	b.WriteByte(')')
	return b.String()
}

func (t *Func) String() string {
	var b strings.Builder
	b.WriteString("fn(")
	for i, p := range t.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		if p.Var {
			b.WriteString("var ")
		}
		b.WriteString(p.T.String())
	}
	b.WriteByte(')')
	if t.Result != nil && !isUnit(t.Result) {
		b.WriteString(" -> ")
		b.WriteString(t.Result.String())
	}
	if t.Err != nil {
		b.WriteString(" ! ")
		b.WriteString(t.Err.String())
	}
	if s := t.Eff.String(); s != "" {
		b.WriteString(" uses ")
		if strings.Contains(s, ",") {
			b.WriteString("(" + s + ")")
		} else {
			b.WriteString(s)
		}
	}
	return b.String()
}

func (t *ParamType) String() string { return t.P.Name }

func (t *Var) String() string {
	if t.Ref != nil {
		return t.Ref.String()
	}
	return "_"
}

func (t *BadType) String() string { return "<error>" }

// resolve follows bound inference variables.
func resolve(t Type) Type {
	for {
		v, ok := t.(*Var)
		if !ok || v.Ref == nil {
			return t
		}
		t = v.Ref
	}
}

// deep resolves inference variables everywhere inside t.
func deep(t Type) Type {
	t = resolve(t)
	switch x := t.(type) {
	case *Named:
		if len(x.Args) == 0 {
			return x
		}
		args := make([]Type, len(x.Args))
		for i, a := range x.Args {
			args[i] = deep(a)
		}
		return &Named{Obj: x.Obj, Args: args}
	case *Tuple:
		if len(x.Elems) == 0 {
			return x
		}
		el := make([]Type, len(x.Elems))
		for i, e := range x.Elems {
			el[i] = deep(e)
		}
		return &Tuple{Elems: el}
	case *Func:
		ps := make([]FParam, len(x.Params))
		for i, p := range x.Params {
			ps[i] = FParam{Var: p.Var, T: deep(p.T)}
		}
		var errT Type
		if x.Err != nil {
			errT = deep(x.Err)
			if isNever(errT) {
				errT = nil
			}
		}
		return &Func{Params: ps, Result: deep(x.Result), Err: errT, Eff: x.Eff}
	}
	return t
}

func isBad(t Type) bool {
	_, ok := resolve(t).(*BadType)
	return ok
}

func isUnit(t Type) bool {
	tt, ok := resolve(t).(*Tuple)
	return ok && len(tt.Elems) == 0
}

func isOpenVar(t Type) bool {
	v, ok := resolve(t).(*Var)
	return ok && v.Ref == nil
}

// containsBad reports whether t mentions the error type anywhere.
func containsBad(t Type) bool {
	switch x := resolve(t).(type) {
	case *BadType:
		return true
	case *Named:
		for _, a := range x.Args {
			if containsBad(a) {
				return true
			}
		}
	case *Tuple:
		for _, e := range x.Elems {
			if containsBad(e) {
				return true
			}
		}
	case *Func:
		for _, p := range x.Params {
			if containsBad(p.T) {
				return true
			}
		}
		if x.Result != nil && containsBad(x.Result) {
			return true
		}
		if x.Err != nil && containsBad(x.Err) {
			return true
		}
	}
	return false
}

// subst replaces type parameters using m. Effect parameters inside function
// types are replaced using em.
type substMap struct {
	types map[*TParam]Type
	effs  map[*TParam]*EffSet
}

func (s *substMap) empty() bool { return s == nil || (len(s.types) == 0 && len(s.effs) == 0) }

func subst(t Type, s *substMap) Type {
	if s.empty() || t == nil {
		return t
	}
	switch x := t.(type) {
	case *ParamType:
		if r, ok := s.types[x.P]; ok {
			return r
		}
		return x
	case *EffArg:
		return &EffArg{Set: x.Set.subst(s)}
	case *Var:
		if x.Ref != nil {
			return subst(x.Ref, s)
		}
		return x
	case *Named:
		if len(x.Args) == 0 {
			return x
		}
		args := make([]Type, len(x.Args))
		for i, a := range x.Args {
			args[i] = subst(a, s)
		}
		return &Named{Obj: x.Obj, Args: args}
	case *Tuple:
		if len(x.Elems) == 0 {
			return x
		}
		el := make([]Type, len(x.Elems))
		for i, e := range x.Elems {
			el[i] = subst(e, s)
		}
		return &Tuple{Elems: el}
	case *Func:
		ps := make([]FParam, len(x.Params))
		for i, p := range x.Params {
			ps[i] = FParam{Var: p.Var, T: subst(p.T, s)}
		}
		var errT Type
		if x.Err != nil {
			errT = subst(x.Err, s)
		}
		return &Func{Params: ps, Result: subst(x.Result, s), Err: errT, Eff: x.Eff.subst(s)}
	}
	return t
}

// typeArgsSubst maps the type parameters of obj to args.
func typeArgsSubst(params []*TParam, args []Type) *substMap {
	if len(params) == 0 {
		return nil
	}
	s := &substMap{types: map[*TParam]Type{}, effs: map[*TParam]*EffSet{}}
	for i, p := range params {
		if i < len(args) {
			if ea, ok := args[i].(*EffArg); ok {
				s.effs[p] = ea.Set
				continue
			}
			s.types[p] = args[i]
		}
	}
	return s
}

// identical reports structural equality after resolving variables. Open
// variables are only identical to themselves.
func identical(a, b Type) bool {
	a, b = resolve(a), resolve(b)
	if a == b {
		return true
	}
	switch x := a.(type) {
	case *BadType:
		return true
	case *Named:
		y, ok := b.(*Named)
		if !ok || x.Obj != y.Obj || len(x.Args) != len(y.Args) {
			_, bad := b.(*BadType)
			return bad
		}
		for i := range x.Args {
			if !identical(x.Args[i], y.Args[i]) {
				return false
			}
		}
		return true
	case *Tuple:
		y, ok := b.(*Tuple)
		if !ok || len(x.Elems) != len(y.Elems) {
			_, bad := b.(*BadType)
			return bad
		}
		for i := range x.Elems {
			if !identical(x.Elems[i], y.Elems[i]) {
				return false
			}
		}
		return true
	case *Func:
		y, ok := b.(*Func)
		if !ok || len(x.Params) != len(y.Params) {
			_, bad := b.(*BadType)
			return bad
		}
		for i := range x.Params {
			if x.Params[i].Var != y.Params[i].Var || !identical(x.Params[i].T, y.Params[i].T) {
				return false
			}
		}
		if !identical(x.Result, y.Result) {
			return false
		}
		if (x.Err == nil) != (y.Err == nil) {
			return false
		}
		return x.Err == nil || identical(x.Err, y.Err)
	case *ParamType:
		y, ok := b.(*ParamType)
		if ok {
			return x.P == y.P
		}
	case *EffArg:
		_, ok := b.(*EffArg)
		return ok
	}
	_, bad := b.(*BadType)
	return bad
}

// occurs reports whether v appears in t.
func occurs(v *Var, t Type) bool {
	switch x := resolve(t).(type) {
	case *Var:
		return x == v
	case *Named:
		for _, a := range x.Args {
			if occurs(v, a) {
				return true
			}
		}
	case *Tuple:
		for _, e := range x.Elems {
			if occurs(v, e) {
				return true
			}
		}
	case *Func:
		for _, p := range x.Params {
			if occurs(v, p.T) {
				return true
			}
		}
		if occurs(v, x.Result) {
			return true
		}
		if x.Err != nil && occurs(v, x.Err) {
			return true
		}
	}
	return false
}

// hasOpenVars reports whether t still contains unbound inference variables.
func hasOpenVars(t Type) bool {
	switch x := resolve(t).(type) {
	case *Var:
		return true
	case *Named:
		for _, a := range x.Args {
			if hasOpenVars(a) {
				return true
			}
		}
	case *Tuple:
		for _, e := range x.Elems {
			if hasOpenVars(e) {
				return true
			}
		}
	case *Func:
		for _, p := range x.Params {
			if hasOpenVars(p.T) {
				return true
			}
		}
		if hasOpenVars(x.Result) {
			return true
		}
	}
	return false
}
