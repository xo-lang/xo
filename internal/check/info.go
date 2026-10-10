package check

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/token"
)

// records are resolutions the checker notes while checking bodies, for the
// code generator (internal/gen). They are written next to the checks and
// never change what the checker accepts.
type records struct {
	natType    map[ast.Expr]Type          // type of an expression before implicit conversion
	ref        map[ast.Expr]Object        // functions, constants, statics, and method values named by an expression
	subst      map[ast.Expr]*substMap     // type arguments of a method value
	ctor       map[ast.Expr]*VariantObj   // variant constructed by an identifier, selector, or call
	typeCall   map[*ast.CallExpr]*TypeObj // `T(x)`: numeric conversion or newtype construction
	patType    map[ast.Pattern]Type       // type of the value a pattern is matched against
	patVariant map[*ast.VariantPat]*VariantObj
	patNewtype map[*ast.VariantPat]*TypeObj // `Email(p)`: newtype pattern (decision 0097)
	resultVar  map[*ast.EnsuresClause]*VarObj
	testOs     map[*ast.TestDecl]*VarObj
	testParams map[*ast.TestDecl][]*VarObj
	recvVar    map[*FuncObj]*VarObj
	paramVars  map[*FuncObj][]*VarObj
	invVars    map[*TypeObj][]*VarObj
	closureVar map[*ast.FnLit][]*VarObj
	// names is what each identifier resolved to by name lookup, for the
	// names section of `xo typedump` (dump.go, decision 0141).
	names map[*ast.Ident]nameRef
}

// nameRef is the object an identifier names: *VarObj, *FuncObj,
// *ConstObj, *TypeObj, *TParam, or *Module (an import).
type nameRef struct {
	obj any
	// call marks the callee of a call `T(...)`, where a prelude type
	// name may instead have been a variant of the expected enum.
	call bool
}

// name records that id names obj.
func (r *records) name(id *ast.Ident, obj any) {
	if id != nil && obj != nil {
		r.names[id] = nameRef{obj: obj}
	}
}

func newRecords() *records {
	return &records{
		natType:    map[ast.Expr]Type{},
		ref:        map[ast.Expr]Object{},
		subst:      map[ast.Expr]*substMap{},
		ctor:       map[ast.Expr]*VariantObj{},
		typeCall:   map[*ast.CallExpr]*TypeObj{},
		patType:    map[ast.Pattern]Type{},
		patVariant: map[*ast.VariantPat]*VariantObj{},
		patNewtype: map[*ast.VariantPat]*TypeObj{},
		resultVar:  map[*ast.EnsuresClause]*VarObj{},
		testOs:     map[*ast.TestDecl]*VarObj{},
		testParams: map[*ast.TestDecl][]*VarObj{},
		recvVar:    map[*FuncObj]*VarObj{},
		paramVars:  map[*FuncObj][]*VarObj{},
		invVars:    map[*TypeObj][]*VarObj{},
		closureVar: map[*ast.FnLit][]*VarObj{},
		names:      map[*ast.Ident]nameRef{},
	}
}

// Info gives the code generator read access to what the checker resolved.
// Types it returns are fully resolved (no bound inference variables).
type Info struct {
	c *checker
}

// Info returns the resolution tables of a check run.
func (r *Result) Info() *Info { return &Info{c: r.c} }

// AllModules returns every loaded module (std included) in dependency
// order: a module comes after the modules it imports.
func (i *Info) AllModules() []*Module { return i.c.order }

// Prelude returns the prelude module.
func (i *Info) Prelude() *Module { return i.c.prelude }

// Module returns the loaded module with the given import path.
func (i *Info) Module(path string) *Module { return i.c.modules[path] }

// TypeOf returns the type of e after implicit conversion (its type in
// context), or nil if e was not checked.
func (i *Info) TypeOf(e ast.Expr) Type {
	t, ok := i.c.exprType[e]
	if !ok {
		return nil
	}
	return Deep(t)
}

// NaturalType returns the type e has before any implicit conversion, or
// nil when e was only checked against an expected type (blocks, if, match).
func (i *Info) NaturalType(e ast.Expr) Type {
	t, ok := i.c.rec.natType[e]
	if !ok {
		return nil
	}
	return Deep(t)
}

// VarOf returns the local binding an identifier refers to or declares.
func (i *Info) VarOf(id *ast.Ident) *VarObj { return i.c.identObj[id] }

// Ref returns the function, constant, static, or method an identifier or
// selector names, or nil.
func (i *Info) Ref(e ast.Expr) Object { return i.c.rec.ref[e] }

// RefSubst returns the type arguments of a method used as a value.
func (i *Info) RefSubst(e ast.Expr) *Subst {
	if s := i.c.rec.subst[e]; s != nil {
		return &Subst{s: s}
	}
	return nil
}

// Ctor returns the variant an expression constructs, or nil.
func (i *Info) Ctor(e ast.Expr) *VariantObj { return i.c.rec.ctor[e] }

// TypeCall returns the type of a conversion or newtype construction call.
func (i *Info) TypeCall(call *ast.CallExpr) *TypeObj { return i.c.rec.typeCall[call] }

// Call describes a resolved call of a declared function or a function
// value.
type Call struct {
	Fn    *FuncObj // nil for a call of a function value
	Recv  ast.Expr // receiver expression of a method call
	Subst *Subst
}

// CallOf returns the resolution of a call, or nil.
func (i *Info) CallOf(call *ast.CallExpr) *Call {
	ci := i.c.callInfo[call]
	if ci == nil {
		return nil
	}
	out := &Call{Fn: ci.fn, Recv: ci.recvExpr}
	if ci.sm != nil {
		out.Subst = &Subst{s: ci.sm}
	}
	return out
}

// PatType returns the type of the value a pattern matches.
func (i *Info) PatType(p ast.Pattern) Type {
	t, ok := i.c.rec.patType[p]
	if !ok {
		return nil
	}
	return Deep(t)
}

// PatVariant returns the variant a variant pattern names.
func (i *Info) PatVariant(p *ast.VariantPat) *VariantObj { return i.c.rec.patVariant[p] }

// PatNewtype returns the newtype a pattern `Email(p)` unwraps (decision
// 0097), or nil when the pattern is a variant pattern.
func (i *Info) PatNewtype(p *ast.VariantPat) *TypeObj { return i.c.rec.patNewtype[p] }

// ResultVar returns the `result` binding of an ensures clause.
func (i *Info) ResultVar(e *ast.EnsuresClause) *VarObj { return i.c.rec.resultVar[e] }

// TestOs returns the predeclared `os` of a test block.
func (i *Info) TestOs(t *ast.TestDecl) *VarObj { return i.c.rec.testOs[t] }

// TestParams returns the bindings of a property test's `for` parameters.
func (i *Info) TestParams(t *ast.TestDecl) []*VarObj { return i.c.rec.testParams[t] }

// RecvVar returns the receiver binding of a method body.
func (i *Info) RecvVar(f *FuncObj) *VarObj { return i.c.rec.recvVar[f] }

// ParamVars returns the parameter bindings of a function body.
func (i *Info) ParamVars(f *FuncObj) []*VarObj { return i.c.rec.paramVars[f] }

// InvariantVars returns the field bindings used by a struct invariant.
func (i *Info) InvariantVars(t *TypeObj) []*VarObj { return i.c.rec.invVars[t] }

// ClosureParams returns the parameter bindings of a closure.
func (i *Info) ClosureParams(f *ast.FnLit) []*VarObj { return i.c.rec.closureVar[f] }

// FuncOf returns the function object declared by fd in module m.
func (i *Info) FuncOf(m *Module, fd *ast.FnDecl) *FuncObj {
	saved := i.c.mod
	i.c.mod = m
	defer func() { i.c.mod = saved }()
	return i.c.findFuncObj(fd)
}

// FromVariant returns the `from` variant of enum `to` that wraps `from`.
func (i *Info) FromVariant(to, from Type) *VariantObj { return i.c.fromVariant(to, from) }

// Implements reports whether t satisfies the interface iface.
func (i *Info) Implements(t Type, iface *Named) bool {
	return i.c.implements(t, iface, token.Span{})
}

// FindMethod finds a method on t and the substitution of its receiver
// type parameters.
func (i *Info) FindMethod(t Type, name string) (*FuncObj, *Subst) {
	m, s := i.c.findMethod(t, name)
	if m == nil {
		return nil, nil
	}
	return m, &Subst{s: s}
}

// Subst is a substitution of type parameters.
type Subst struct {
	s *substMap
}

// Type returns what p is replaced by, resolved, or nil.
func (s *Subst) Type(p *TParam) Type {
	if s == nil || s.s == nil {
		return nil
	}
	t, ok := s.s.types[p]
	if !ok {
		return nil
	}
	return Deep(t)
}

// Apply substitutes t.
func (s *Subst) Apply(t Type) Type {
	if s == nil {
		return t
	}
	return Subst1(t, s.s)
}

// Subst1 applies a substitution without dropping `Never` error types (the
// generic form of a signature).
func Subst1(t Type, s *substMap) Type { return subst(t, s) }

// NewSubst builds a substitution from parameters to types.
func NewSubst(params []*TParam, args []Type) *Subst {
	return &Subst{s: typeArgsSubst(params, args)}
}

// Deep resolves every inference variable inside t. A function type whose
// error type is Never gets a nil error.
func Deep(t Type) Type {
	if t == nil {
		return nil
	}
	return deep(t)
}

// Resolve follows bound inference variables at the top of t.
func Resolve(t Type) Type { return resolve(t) }

// IsNever reports whether t is the prelude type Never.
func IsNever(t Type) bool { return t != nil && isNever(t) }

// IsUnit reports whether t is `()`.
func IsUnit(t Type) bool { return t != nil && isUnit(t) }

// Awaited reports whether a task handle bound to v is awaited somewhere
// (core 7.1: its error is then consumed by the await).
func (v *VarObj) Awaited() bool { return v.awaited }

// LitKind reports "int" or "float" for an open literal variable.
func (t *Var) LitKind() string { return t.lit }

// Decl returns the declaration of a type: *ast.StructDecl, *ast.EnumDecl,
// *ast.InterfaceDecl, or *ast.TypeDecl.
func (t *TypeObj) Decl() ast.Node {
	switch {
	case t.structDecl != nil:
		return t.structDecl
	case t.enumDecl != nil:
		return t.enumDecl
	case t.ifaceDecl != nil:
		return t.ifaceDecl
	case t.typeDecl != nil:
		return t.typeDecl
	}
	return nil
}
