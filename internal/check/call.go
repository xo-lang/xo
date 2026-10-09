package check

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// ---------------------------------------------------------------------------
// Names

func (c *checker) inTestContext() bool {
	return (c.ctx != nil && c.ctx.isTest) || (c.file != nil && c.file.IsTest)
}

// visibleValueNames lists names for XO0601 suggestions.
func (c *checker) visibleValueNames() []string {
	var out []string
	for s := c.sc; s != nil; s = s.parent {
		for n := range s.names {
			out = append(out, n)
		}
	}
	for n := range c.mod.Scope {
		out = append(out, n)
	}
	for n, o := range c.prelude.Scope {
		if _, ok := o.(*FuncObj); ok {
			out = append(out, n)
		}
	}
	for n := range c.file.Imports {
		out = append(out, n)
	}
	out = append(out, "true", "false", "target", "Some", "None", "Ok", "Err")
	sort.Strings(out)
	return out
}

// useVar marks a local as read and records the resolution.
func (c *checker) useVar(id *ast.Ident, v *VarObj) Type {
	v.Used = true
	c.identObj[id] = v
	return v.Type
}

// variantFromHint finds a variant named name in the enum expected by hint
// (core 2.3: the prefix may be omitted when the expected type is known).
func (c *checker) variantFromHint(name string, hint Type) (*VariantObj, *Named) {
	if hint == nil {
		return nil, nil
	}
	h := resolve(hint)
	if w, ok := h.(*retWant); ok {
		h = resolve(w.T)
	}
	n, ok := h.(*Named)
	if !ok {
		return nil, nil
	}
	if n.Obj.Kind != KEnum {
		// an optional of an enum: `Some` wrapping applies
		if el := c.optionElem(n); el != nil {
			return c.variantFromHint(name, el)
		}
		return nil, nil
	}
	if v := n.Obj.variant(name); v != nil {
		return v, n
	}
	return nil, nil
}

// preludeVariant returns Some, None, Ok, or Err.
func (c *checker) preludeVariant(name string) *VariantObj {
	switch name {
	case "Some", "None":
		return c.preludeType("Option").variant(name)
	case "Ok", "Err":
		return c.preludeType("Result").variant(name)
	}
	return nil
}

func (c *checker) ident(x *ast.Ident, hint Type) Type {
	name := x.Name
	if v := c.lookupLocal(name); v != nil {
		return c.useVar(x, v)
	}
	switch name {
	case "true", "false":
		return c.named("Bool")
	case "target":
		return c.named("Target")
	}
	if isUpperCamel(name) {
		if v, _ := c.variantFromHint(name, hint); v != nil {
			return c.construct(v, nil, nil, hint, x.Sp, x)
		}
		if name == "None" {
			return c.construct(c.preludeVariant("None"), nil, nil, hint, x.Sp, x)
		}
		if pv := c.preludeVariant(name); pv != nil {
			c.errorf(diag.WrongKind, x.Sp, "`%s` needs an argument: `%s(x)`", name, name)
			return badType
		}
	}
	if c.inTestContext() && name == "testing" {
		c.errorf(diag.WrongKind, x.Sp, "`testing` is a module, not a value")
		return badType
	}
	if imp := c.file.Imports[name]; imp != nil {
		imp.Used = true
		c.errorf(diag.WrongKind, x.Sp, "`%s` is a module, not a value", name)
		return badType
	}
	switch o := c.lookupModuleObj(c.mod, name).(type) {
	case *FuncObj:
		c.rec.ref[x] = o
		return c.funcValue(o, nil, x.Sp)
	case *ConstObj:
		c.rec.ref[x] = o
		return c.constType(o)
	case *TypeObj:
		d := c.errorf(diag.WrongKind, x.Sp, "`%s` is a type, not a value", name)
		if o.Kind == KStruct && !o.Opaque {
			note(d, token.Span{}, "build a value with a struct literal: `%s{...}`", name)
		}
		return badType
	}
	if name == "result" || name == "old" {
		c.errorf(diag.UnresolvedName, x.Sp, "`%s` is only defined inside an `ensures` clause (core 9.1)", name)
		return badType
	}
	if c.inTestContext() && name == "expect" {
		c.errorf(diag.WrongKind, x.Sp, "`expect` must be called: `expect(x) == y`")
		return badType
	}
	if (name == "os" || name == "expect") && !c.inTestContext() {
		d := c.unresolved(x.Sp, "name", name, c.visibleValueNames())
		note(d, token.Span{}, "`%s` is predeclared only in test blocks (core 9.2); `main` receives `os: Os` as a parameter", name)
		return badType
	}
	if isUpperCamel(name) && hint != nil {
		if n, ok := resolve(hint).(*Named); ok && n.Obj.Kind == KEnum {
			var cands []string
			for _, v := range n.Obj.Variants {
				cands = append(cands, v.Name)
			}
			c.unresolved(x.Sp, "variant of `"+n.Obj.Name+"`", name, cands)
			return badType
		}
	}
	d := c.unresolved(x.Sp, "name", name, c.visibleValueNames())
	c.qualifiedHint(d, name)
	return badType
}

// qualifiedHint notes where a name is declared when it exists in a std
// module that must be named: `FsOp` is `testing.FsOp`.
func (c *checker) qualifiedHint(d *diag.Diagnostic, name string) {
	if d == nil {
		return
	}
	for _, path := range sortedKeys(c.modules) {
		m := c.modules[path]
		if !m.IsStd || m.IsPrelude {
			continue
		}
		if _, ok := m.Scope[name]; ok {
			note(d, token.Span{}, "`%s` is declared in `%s`; refer to it as `%s.%s` (core 2.3, 4.1)", name, m.Path, m.Name, name)
			if m.Name == "testing" && c.inTestContext() {
				fix(d, fmt.Sprintf("qualify as `testing.%s`", name), insertEdit(d.Span.File, token.Pos{Line: d.Span.Line, Col: d.Span.Col}, "testing."))
			}
		}
	}
}

// funcValue is a declared function used as a value: generic parameters are
// instantiated with fresh variables (or targs).
func (c *checker) funcValue(fo *FuncObj, targs []Type, sp token.Span) Type {
	if !fo.sigDone {
		return badType
	}
	sm := c.freshSubst(fo, nil, targs, sp)
	ft := subst(fo.sig(), sm).(*Func)
	if ft.Err != nil && c.isNever(ft.Err) {
		ft.Err = nil
	}
	return ft
}

// freshSubst maps the type parameters of fo that are not already in pre to
// explicit type arguments (for its own parameters) or fresh variables.
func (c *checker) freshSubst(fo *FuncObj, pre *substMap, targs []Type, sp token.Span) *substMap {
	sm := &substMap{types: map[*TParam]Type{}, effs: map[*TParam]*EffSet{}}
	if pre != nil {
		for k, v := range pre.types {
			sm.types[k] = v
		}
		for k, v := range pre.effs {
			sm.effs[k] = v
		}
	}
	var own []*TParam
	for _, p := range fo.TParams {
		if !p.Implicit {
			own = append(own, p)
		}
	}
	var typeOwn []*TParam
	for _, p := range own {
		if !p.IsEffect {
			typeOwn = append(typeOwn, p)
		}
	}
	if targs != nil && len(targs) != len(typeOwn) {
		c.errorf(diag.BadArguments, sp, "`%s` takes %d type argument(s), found %d", fo.DisplayName(), len(typeOwn), len(targs))
		targs = nil
	}
	i := 0
	for _, p := range fo.TParams {
		if _, done := sm.types[p]; done {
			continue
		}
		if _, done := sm.effs[p]; done {
			continue
		}
		if p.IsEffect {
			sm.effs[p] = &EffSet{Items: []EffItem{{EVar: &EffVar{}}}}
			continue
		}
		if targs != nil && !p.Implicit {
			sm.types[p] = targs[i]
			i++
			continue
		}
		sm.types[p] = c.newVar()
	}
	return sm
}

// ---------------------------------------------------------------------------
// Selectors

func (c *checker) selector(x *ast.SelectorExpr, hint Type) Type {
	if id, ok := x.X.(*ast.Ident); ok && c.lookupLocal(id.Name) == nil {
		if imp := c.file.Imports[id.Name]; imp != nil {
			imp.Used = true
			return c.moduleMember(imp, x, nil, hint)
		}
		if id.Name == "testing" && c.inTestContext() {
			return c.moduleMember(&Import{Name: "testing", Module: c.modules["std/testing"]}, x, nil, hint)
		}
	}
	if obj, args, ok := c.typeOfExpr(x.X); ok {
		return c.typeMember(obj, args, x, hint, nil)
	}
	bt := c.infer(x.X, nil)
	return c.fieldOf(bt, x, false)
}

func (c *checker) moduleMember(imp *Import, x *ast.SelectorExpr, targs []Type, hint Type) Type {
	if imp.Module == nil {
		return badType
	}
	name := x.Sel.Name
	o := imp.Module.Scope[name]
	if o == nil {
		if c.goSkipped(imp.Module, name, x.Sel.Sp) {
			return badType
		}
		var cands []string
		for n := range imp.Module.Scope {
			cands = append(cands, n)
		}
		sort.Strings(cands)
		c.unresolved(x.Sel.Sp, "member of module `"+imp.Name+"`", name, cands)
		return badType
	}
	if !c.pubIn(o, imp.Module) {
		c.errorf(diag.NotVisible, x.Sel.Sp, "`%s` is not `pub` in module `%s`", name, imp.Module.Path)
	}
	switch ob := o.(type) {
	case *FuncObj:
		c.rec.ref[x] = ob
		return c.funcValue(ob, targs, x.Sp)
	case *ConstObj:
		c.rec.ref[x] = ob
		return c.constType(ob)
	case *TypeObj:
		c.errorf(diag.WrongKind, x.Sp, "`%s.%s` is a type, not a value", imp.Name, name)
	}
	return badType
}

func (c *checker) pubIn(o Object, m *Module) bool {
	if m == c.mod || m.IsStd {
		return true
	}
	switch x := o.(type) {
	case *FuncObj:
		return x.Pub
	case *ConstObj:
		return x.Pub
	case *TypeObj:
		return x.Pub
	}
	return true
}

// typeMember resolves `Type.Variant`, `Type.CONST`, or a static function
// used as a value. call is non nil when the member is called.
func (c *checker) typeMember(obj *TypeObj, targs []Type, x *ast.SelectorExpr, hint Type, call *ast.CallExpr) Type {
	if obj.Kind == KAlias {
		o2, a2 := c.aliasNamed(obj, targs, x.X.Span())
		if o2 == nil {
			return badType
		}
		obj, targs = o2, a2
	}
	name := x.Sel.Name
	if obj.Kind == KEnum {
		if v := obj.variant(name); v != nil {
			return c.construct(v, targs, call, hint, x.Sp, x)
		}
	}
	if fo := obj.Statics[name]; fo != nil {
		c.rec.ref[x] = fo
		if fo.Const {
			if call != nil {
				c.errorf(diag.WrongKind, call.Sp, "`%s.%s` is a constant, not a function", obj.Name, name)
				return badType
			}
			return fo.Result
		}
		if call != nil {
			return c.callStatic(obj, targs, fo, call, hint)
		}
		pre := c.staticSubst(obj, targs, fo)
		return subst(fo.sig(), c.freshSubst(fo, pre, nil, x.Sp))
	}
	var cands []string
	for _, v := range obj.Variants {
		cands = append(cands, v.Name)
	}
	for n := range obj.Statics {
		cands = append(cands, n)
	}
	sort.Strings(cands)
	what := "associated function or constant of `" + obj.QualName() + "`"
	if obj.Kind == KEnum {
		what = "variant of `" + obj.QualName() + "`"
	}
	d := c.unresolved(x.Sel.Sp, what, name, cands)
	if d != nil && obj.Methods[name] != nil {
		note(d, token.Span{}, "`%s` is a method; call it on a value of type `%s`", name, obj.Name)
	}
	if call != nil {
		c.numericConversionHint(d, obj, x, call)
	}
	return badType
}

// numericConversionHint helps with `Float.from(n)` and the like (gap 8 of
// bench/results/real-app-2026-10-08.md): when an associated function that
// does not exist is called on a numeric type with one numeric argument,
// the conversion is a call on the type, `Float(n)` (std.md 2.1). The fix
// comes first so `xo fix` prefers it over a "did you mean" guess. The
// argument is checked here (the failed call does not check its arguments).
func (c *checker) numericConversionHint(d *diag.Diagnostic, obj *TypeObj, x *ast.SelectorExpr, call *ast.CallExpr) {
	if d == nil || obj.Module != c.prelude || !intTypes[obj.Name] && !floatTypes[obj.Name] {
		return
	}
	if len(call.Args) != 1 || call.Args[0].Name != nil {
		return
	}
	if _, ok := unparen(call.Args[0].Value).(*ast.FnLit); ok {
		return
	}
	if !c.isNumeric(c.infer(call.Args[0].Value, nil)) {
		return
	}
	note(d, token.Span{}, "convert with `%s(x)`: a numeric conversion is a call on the type name, from any numeric type (std.md 2.1)", obj.Name)
	d.Fixes = append([]diag.Fix{{Title: fmt.Sprintf("convert with `%s(x)`", obj.Name), Edits: []diag.Edit{replaceEdit(x.Sp, obj.Name)}}}, d.Fixes...)
}

// staticSubst maps the implicit receiver parameters of a static function to
// the type arguments written on the type (or fresh variables).
func (c *checker) staticSubst(obj *TypeObj, targs []Type, fo *FuncObj) *substMap {
	sm := &substMap{types: map[*TParam]Type{}, effs: map[*TParam]*EffSet{}}
	rt, ok := fo.RecvType.(*Named)
	if !ok {
		return sm
	}
	for i, a := range rt.Args {
		p, ok := a.(*ParamType)
		if !ok || !p.P.Implicit {
			continue
		}
		if targs != nil && i < len(targs) {
			sm.types[p.P] = targs[i]
		} else {
			sm.types[p.P] = c.newVar()
		}
	}
	for i, a := range rt.Args {
		ea, ok := a.(*EffArg)
		if !ok || len(ea.Set.Items) != 1 || ea.Set.Items[0].Param == nil {
			continue
		}
		p := ea.Set.Items[0].Param
		if targs != nil && i < len(targs) {
			if ta, ok := targs[i].(*EffArg); ok {
				sm.effs[p] = ta.Set
				continue
			}
		}
		sm.effs[p] = &EffSet{Items: []EffItem{{EVar: &EffVar{}}}}
	}
	return sm
}

func (c *checker) callStatic(obj *TypeObj, targs []Type, fo *FuncObj, call *ast.CallExpr, hint Type) Type {
	if targs != nil && len(targs) != len(obj.TParams) {
		c.errorf(diag.BadArguments, call.Fun.Span(), "`%s` takes %d type argument(s), found %d", obj.QualName(), len(obj.TParams), len(targs))
		targs = nil
	}
	return c.callFunc(fo, c.staticSubst(obj, targs, fo), nil, call, hint, nil)
}

// fieldOf types `x.name` on a value of type bt. In place mode (an
// assignment target) methods are not considered.
func (c *checker) fieldOf(bt Type, x *ast.SelectorExpr, place bool) Type {
	r := resolve(bt)
	name := x.Sel.Name
	if isBad(r) {
		return badType
	}
	if _, ok := r.(*Var); ok {
		c.errorf(diag.CannotInfer, x.X.Span(), "the type of this value must be known before `.%s`; annotate it", name)
		return badType
	}
	if tt, ok := r.(*Tuple); ok && isTupleIndex(name) {
		// tuple field `t.0` (core 3.5)
		if i, err := strconv.Atoi(name); err == nil && i < len(tt.Elems) {
			return tt.Elems[i]
		}
		c.errorf(diag.UnresolvedName, x.Sel.Sp, "tuple `%s` has no field `%s`; its fields are `.0` to `.%d`", deep(r), name, len(tt.Elems)-1)
		return badType
	}
	if n, ok := r.(*Named); ok {
		if n.Obj.Module == c.prelude && n.Obj.Name == "Task" && name == "await" && !place {
			if id, ok := x.X.(*ast.Ident); ok {
				if v := c.lookupLocal(id.Name); v != nil {
					v.awaited = true
				}
			}
			return c.callResult(n.Args[0], n.Args[1])
		}
		if n.Obj.Kind == KStruct && !n.Obj.Opaque {
			if f := n.Obj.field(name); f != nil {
				if !f.Pub && n.Obj.Module != c.mod {
					c.errorf(diag.NotVisible, x.Sel.Sp, "field `%s` of `%s` is not `pub`", name, n.Obj.Name)
				}
				return subst(f.Type, typeArgsSubst(n.Obj.TParams, n.Args))
			}
		}
	}
	if !place {
		if m, sm := c.findMethod(r, name); m != nil {
			if m.RecvVar {
				c.errorf(diag.WrongKind, x.Sp, "method `%s` mutates its receiver and cannot be used as a value", name)
				return badType
			}
			fs := c.freshSubst(m, sm, nil, x.Sp)
			c.rec.ref[x] = m
			c.rec.subst[x] = fs
			ft := subst(m.sig(), fs)
			return ft
		}
	}
	c.noMember(r, x.Sel, place)
	return badType
}

// isTupleIndex reports whether a selector is a tuple field index.
func isTupleIndex(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		if name[i] < '0' || name[i] > '9' {
			return false
		}
	}
	return true
}

// noMember reports XO0601 for a missing field or method.
func (c *checker) noMember(t Type, sel *ast.Ident, place bool) {
	if n, ok := resolve(t).(*Named); ok && c.goSkipped(n.Obj.Module, n.Obj.Name+"."+sel.Name, sel.Sp) {
		return
	}
	var cands []string
	what := "field or method"
	if place {
		what = "field"
	}
	if n, ok := resolve(t).(*Named); ok {
		if n.Obj.Kind == KStruct && !n.Obj.Opaque {
			for _, f := range n.Obj.Fields {
				cands = append(cands, f.Name)
			}
		}
		if !place {
			for m := range n.Obj.Methods {
				cands = append(cands, m)
			}
			for _, m := range n.Obj.IfaceMeths {
				cands = append(cands, m.Name)
			}
		}
	}
	sort.Strings(cands)
	d := c.unresolved(sel.Sp, what+" of `"+deep(t).String()+"`", sel.Name, cands)
	if d != nil && c.optionElem(t) != nil {
		if inner := c.optionElem(t); inner != nil {
			if m, _ := c.findMethod(inner, sel.Name); m != nil {
				note(d, token.Span{}, "the value is optional; unwrap it first with `?`, `??`, `if let`, or `match` (core 2.5)")
			}
		}
	}
	if d != nil {
		if tt, _, ok := c.resultParts(t); ok {
			if m, _ := c.findMethod(tt, sel.Name); m != nil {
				note(d, token.Span{}, "the value is a Result; propagate with `?` first")
				fix(d, "propagate with ?", insertEdit(sel.Sp.File, token.Pos{Line: sel.Sp.Start.Line, Col: sel.Sp.Start.Col - 1, Offset: sel.Sp.Start.Offset - 1}, "?"))
			}
		}
	}
}

// typeArgsValue types `f[T]` outside a call: a generic function value.
func (c *checker) typeArgsValue(x *ast.TypeArgsExpr, hint Type) Type {
	targs := c.resolveTypeArgs(x.Types)
	switch f := x.X.(type) {
	case *ast.Ident:
		if fo, ok := c.lookupModuleObj(c.mod, f.Name).(*FuncObj); ok && c.lookupLocal(f.Name) == nil {
			return c.funcValue(fo, targs, x.Sp)
		}
	case *ast.SelectorExpr:
		if id, ok := f.X.(*ast.Ident); ok && c.lookupLocal(id.Name) == nil {
			if imp := c.file.Imports[id.Name]; imp != nil {
				imp.Used = true
				return c.moduleMember(imp, f, targs, hint)
			}
		}
	}
	if _, _, ok := c.typeOfExpr(x); ok {
		c.errorf(diag.WrongKind, x.Sp, "a type is not a value")
		return badType
	}
	c.infer(x.X, nil)
	c.errorf(diag.WrongKind, x.Sp, "type arguments are only allowed on generic functions and types")
	return badType
}

func (c *checker) resolveTypeArgs(ts []ast.Type) []Type {
	env := c.localEnv()
	var out []Type
	for _, t := range ts {
		out = append(out, c.resolveType(t, env))
	}
	return out
}

// ---------------------------------------------------------------------------
// Calls

func (c *checker) call(x *ast.CallExpr, hint Type) Type {
	switch f := x.Fun.(type) {
	case *ast.Ident:
		return c.callIdent(x, f, nil, hint)
	case *ast.SelectorExpr:
		return c.callSelector(x, f, nil, hint)
	case *ast.TypeArgsExpr:
		targs := c.resolveTypeArgs(f.Types)
		switch g := f.X.(type) {
		case *ast.Ident:
			return c.callIdent(x, g, targs, hint)
		case *ast.SelectorExpr:
			return c.callSelector(x, g, targs, hint)
		}
	}
	ft := c.infer(x.Fun, nil)
	return c.callValue(ft, x, hint, "")
}

func (c *checker) callIdent(x *ast.CallExpr, id *ast.Ident, targs []Type, hint Type) Type {
	name := id.Name
	if v := c.lookupLocal(name); v != nil {
		t := c.useVar(id, v)
		if targs != nil {
			c.errorf(diag.WrongKind, x.Fun.Span(), "`%s` is a value and takes no type arguments", name)
		}
		return c.callValue(t, x, hint, name)
	}
	if name == "expect" && c.inTestContext() && c.mod.Scope[name] == nil {
		if len(x.Args) != 1 {
			c.errorf(diag.BadArguments, x.Sp, "`expect` takes one argument")
			for _, a := range x.Args {
				c.infer(a.Value, nil)
			}
			return unitType
		}
		c.check(x.Args[0].Value, c.named("Bool"))
		return unitType
	}
	if name == "old" && c.ctx != nil && c.ctx.ensures {
		if len(x.Args) != 1 {
			c.errorf(diag.BadArguments, x.Sp, "`old` takes one argument")
			return badType
		}
		return c.infer(x.Args[0].Value, hint)
	}
	if isUpperCamel(name) && c.mod.Scope[name] == nil {
		if v, _ := c.variantFromHint(name, hint); v != nil {
			return c.construct(v, targs, x, hint, x.Sp, x)
		}
		if pv := c.preludeVariant(name); pv != nil {
			return c.construct(pv, targs, x, hint, x.Sp, x)
		}
	}
	if imp := c.file.Imports[name]; imp != nil {
		imp.Used = true
		c.errorf(diag.WrongKind, id.Sp, "`%s` is a module and cannot be called", name)
		c.inferArgs(x)
		return badType
	}
	switch o := c.lookupModuleObj(c.mod, name).(type) {
	case *FuncObj:
		return c.callFunc(o, nil, targs, x, hint, nil)
	case *TypeObj:
		return c.typeCall(o, targs, x, hint)
	case *ConstObj:
		return c.callValue(c.constType(o), x, hint, name)
	}
	if name == "spawn" || name == "spawn_background" || name == "go" {
		d := c.errorf(diag.SpawnWithoutScope, id.Sp, "`%s` without a scope handle: tasks are started with `s.spawn(f)` inside `scope s { ... }`, so no task outlives its scope (core 7.1)", name)
		fix(d, "use a scope", insertEdit(id.Sp.File, id.Sp.Start, "s."))
		c.inferArgs(x)
		return badType
	}
	if hint != nil && isUpperCamel(name) {
		if n, ok := resolve(hint).(*Named); ok && n.Obj.Kind == KEnum {
			var cands []string
			for _, v := range n.Obj.Variants {
				cands = append(cands, v.Name)
			}
			c.unresolved(id.Sp, "variant of `"+n.Obj.Name+"`", name, cands)
			c.inferArgs(x)
			return badType
		}
	}
	c.unresolved(id.Sp, "function", name, c.visibleValueNames())
	c.inferArgs(x)
	return badType
}

// inferArgs checks arguments of a call that failed to resolve, so names in
// them are still resolved and marked used.
// inferArgs checks the arguments of a call whose callee could not be typed.
// An error was already reported for the callee, so closure arguments get a
// bad hint: their parameter types are unknowable, and reporting XO0405 for
// them would be a cascade (core 10.2).
func (c *checker) inferArgs(x *ast.CallExpr) {
	for _, a := range x.Args {
		if _, ok := unparen(a.Value).(*ast.FnLit); ok {
			c.infer(a.Value, badType)
			continue
		}
		c.infer(a.Value, nil)
	}
}

func (c *checker) callSelector(x *ast.CallExpr, sel *ast.SelectorExpr, targs []Type, hint Type) Type {
	if id, ok := sel.X.(*ast.Ident); ok && c.lookupLocal(id.Name) == nil {
		imp := c.file.Imports[id.Name]
		if imp == nil && id.Name == "testing" && c.inTestContext() {
			imp = &Import{Name: "testing", Module: c.modules["std/testing"]}
		}
		if imp != nil {
			imp.Used = true
			if imp.Module == nil {
				c.inferArgs(x)
				return badType
			}
			o := imp.Module.Scope[sel.Sel.Name]
			if o == nil {
				if c.goSkipped(imp.Module, sel.Sel.Name, sel.Sel.Sp) {
					c.inferArgs(x)
					return badType
				}
				var cands []string
				for n := range imp.Module.Scope {
					cands = append(cands, n)
				}
				sort.Strings(cands)
				c.unresolved(sel.Sel.Sp, "member of module `"+imp.Name+"`", sel.Sel.Name, cands)
				c.inferArgs(x)
				return badType
			}
			if !c.pubIn(o, imp.Module) {
				c.errorf(diag.NotVisible, sel.Sel.Sp, "`%s` is not `pub` in module `%s`", sel.Sel.Name, imp.Module.Path)
			}
			switch ob := o.(type) {
			case *FuncObj:
				return c.callFunc(ob, nil, targs, x, hint, nil)
			case *TypeObj:
				return c.typeCall(ob, targs, x, hint)
			case *ConstObj:
				return c.callValue(c.constType(ob), x, hint, sel.Sel.Name)
			}
			return badType
		}
	}
	if obj, args, ok := c.typeOfExpr(sel.X); ok {
		if targs != nil {
			c.errorf(diag.WrongKind, x.Fun.Span(), "type arguments go on the type: `%s[...].%s(...)`", obj.Name, sel.Sel.Name)
		}
		return c.typeMember(obj, args, sel, hint, x)
	}
	bt := c.infer(sel.X, nil)
	return c.callMethod(bt, sel, targs, x, hint)
}

func (c *checker) callMethod(bt Type, sel *ast.SelectorExpr, targs []Type, x *ast.CallExpr, hint Type) Type {
	r := resolve(bt)
	name := sel.Sel.Name
	if isBad(r) {
		c.inferArgs(x)
		return badType
	}
	if _, ok := r.(*Var); ok {
		c.errorf(diag.CannotInfer, sel.X.Span(), "the type of this value must be known before `.%s(...)`; annotate it", name)
		c.inferArgs(x)
		return badType
	}
	n, isNamed := r.(*Named)
	if isNamed && n.Obj.Module == c.prelude && n.Obj.Name == "Task" && name == "await" {
		d := c.errorf(diag.WrongKind, x.Sp, "`await` is written without parentheses: `t.await` (core 7.1)")
		fix(d, "remove the parentheses", replaceEdit(token.Span{File: x.Sp.File, Start: sel.Sp.End, End: x.Sp.End}, ""))
		return c.fieldOf(bt, sel, false)
	}
	if _, ok := r.(*Tuple); ok && isTupleIndex(name) {
		return c.callValue(c.fieldOf(bt, sel, false), x, hint, name)
	}
	m, _ := c.findMethodAny(r, name)
	if m == nil {
		// a field holding a function
		if isNamed && n.Obj.Kind == KStruct && !n.Obj.Opaque {
			if f := n.Obj.field(name); f != nil {
				ft := c.fieldOf(bt, sel, false)
				return c.callValue(ft, x, hint, name)
			}
		}
		if (name == "min" || name == "max" || name == "clamp") && c.isOrd(r) {
			want := 1
			if name == "clamp" {
				want = 2
			}
			if len(x.Args) != want {
				c.errorf(diag.BadArguments, x.Sp, "`%s` takes %d argument(s), found %d", name, want, len(x.Args))
			}
			for _, a := range x.Args {
				c.check(a.Value, r)
			}
			return r
		}
		if (name == "spawn" || name == "spawn_background") && !c.isPrelude(r, "Scope") {
			c.errorf(diag.SpawnWithoutScope, sel.Sel.Sp, "`%s` is called on `%s`, not on a scope handle; tasks are started with `s.spawn(f)` inside `scope s { ... }` (core 7.1)", name, deep(r))
			c.inferArgs(x)
			return badType
		}
		c.noMember(r, sel.Sel, false)
		c.inferArgs(x)
		return badType
	}
	if m.Static {
		c.errorf(diag.WrongKind, sel.Sel.Sp, "`%s` is an associated function; call it on the type: `%s.%s(...)`", name, m.Recv.Name, name)
		c.inferArgs(x)
		return badType
	}
	if !m.Pub && m.Module != c.mod && !m.Iface {
		c.errorf(diag.NotVisible, sel.Sel.Sp, "method `%s` of `%s` is not `pub` in module `%s`", name, m.Recv.Name, m.Module.Path)
	}
	// bind receiver type parameters
	pre := &substMap{types: map[*TParam]Type{}, effs: map[*TParam]*EffSet{}}
	if m.Iface {
		if isNamed {
			for i, p := range m.Recv.TParams {
				if i < len(n.Args) {
					pre.types[p] = n.Args[i]
				}
			}
		}
	} else {
		for _, p := range m.TParams {
			if p.Implicit {
				if p.IsEffect {
					pre.effs[p] = &EffSet{Items: []EffItem{{EVar: &EffVar{}}}}
				} else {
					pre.types[p] = c.newVar()
				}
			}
		}
		if !c.unify(subst(m.RecvType, pre), r) {
			d := c.errorf(diag.UnresolvedName, sel.Sel.Sp, "method `%s` is not defined on `%s`", name, deep(r))
			note(d, token.Span{}, "`%s` is only defined on `%s`", name, m.RecvType)
			c.inferArgs(x)
			return badType
		}
	}
	if m.RecvVar {
		c.requireMutable(sel.X, diag.VarReceiverNeedVar, fmt.Sprintf("method `%s`", name))
	}
	t := c.callFunc(m, pre, targs, x, hint, sel.X)
	if c.isPrelude(r, "Scope") && (name == "spawn" || name == "spawn_background") {
		c.recordSpawn(sel.X, x, t)
	}
	if isTaskGroup(r) && (name == "spawn" || name == "spawn_background") {
		c.recordGroupSpawn(sel.X, x)
	}
	return t
}

// findMethodAny finds a method by name on a named type, an interface, or a
// constrained type parameter (receiver applicability is checked by the
// caller).
func (c *checker) findMethodAny(t Type, name string) (*FuncObj, *substMap) {
	switch x := resolve(t).(type) {
	case *Named:
		if x.Obj.Kind == KInterface {
			return c.findMethod(x, name)
		}
		if m := x.Obj.Methods[name]; m != nil {
			return m, nil
		}
	case *ParamType:
		if x.P.Constraint != nil {
			return c.findMethodAny(x.P.Constraint, name)
		}
	}
	return nil, nil
}

// recordSpawn notes a spawned task for the child failure rule (core 7.1).
func (c *checker) recordSpawn(recv ast.Expr, call *ast.CallExpr, taskT Type) {
	n, ok := resolve(taskT).(*Named)
	if !ok || len(n.Args) != 2 {
		return
	}
	errT := n.Args[1]
	var frame *scopeFrame
	if id, ok := recv.(*ast.Ident); ok {
		if v := c.lookupLocal(id.Name); v != nil {
			frame = v.scopeHandle
		}
	}
	if frame != nil {
		frame.spawns = append(frame.spawns, &spawnSite{call: call, errT: errT})
		return
	}
	// A scope handle received from elsewhere: its errors would propagate
	// into a scope this function cannot see, so they must not exist.
	e := resolve(errT)
	if !isBad(e) && !c.isNever(e) && !isOpenVar(e) {
		d := c.errorf(diag.ScopeErrorConvert, call.Sp, "a task spawned on a scope handle from outside this function can fail with `%s`; handle the error inside the task", deep(e))
		note(d, token.Span{}, "child failures propagate out of the scope that owns the handle (core 7.1)")
	}
}

// callFunc checks a call of a declared function or method.
func (c *checker) callFunc(fo *FuncObj, pre *substMap, targs []Type, call *ast.CallExpr, hint Type, recv ast.Expr) Type {
	if !fo.sigDone {
		c.inferArgs(call)
		return badType
	}
	sm := c.freshSubst(fo, pre, targs, call.Fun.Span())
	result := subst(fo.Result, sm)
	var errT Type
	if fo.Err != nil {
		errT = subst(fo.Err, sm)
	}
	if hint != nil {
		h := resolve(hint)
		if w, ok := h.(*retWant); ok {
			h = resolve(w.T)
		}
		if !isOpenVar(h) && !c.isInterface(h) {
			switch {
			case errT == nil:
				c.tryUnify(result, h)
			case !hasOpenVars(errT) && c.isResultType(h):
				c.tryUnify(c.callResult(result, errT), h)
			case !c.isResultType(h):
				c.tryUnify(result, h)
			}
		}
	}
	params := make([]*ParamInfo, len(fo.Params))
	for i, p := range fo.Params {
		params[i] = &ParamInfo{Name: p.Name, Type: subst(p.Type, sm), Var: p.Var, Default: p.Default, Span: p.Span}
	}
	ci := &callInfo{fn: fo, recvExpr: recv, sm: sm}
	c.callInfo[call] = ci
	c.checkArgs(params, call, fo.DisplayName(), ci)
	// constraints
	for _, p := range fo.TParams {
		if p.Constraint == nil || p.IsEffect {
			continue
		}
		at := resolve(sm.types[p])
		if isOpenVar(at) || isBad(at) {
			continue
		}
		cn, ok := resolve(subst(p.Constraint, sm)).(*Named)
		if !ok {
			continue
		}
		if !c.implements(at, cn, call.Sp) {
			d := c.errorf(diag.NotImplemented, call.Sp, "`%s` does not implement `%s`, required by `%s` of `%s`", deep(at), cn, p.Name, fo.DisplayName())
			if d != nil && cn.Obj.Module == c.prelude && (cn.Obj.Name == "Json" || cn.Obj.Name == "Ord" || cn.Obj.Name == "Display" || cn.Obj.Name == "SqlRow") {
				note(d, token.Span{}, "add `derive %s` to the type declaration (core 2.9)", cn.Obj.Name)
			}
		}
	}
	for _, p := range fo.TParams {
		if p.IsEffect || p.Implicit {
			continue
		}
		if v, ok := sm.types[p].(*Var); ok && v.Ref == nil && (fo.Err == nil || !mentions(fo.Err, p)) {
			c.typeArgVars = append(c.typeArgVars, typeArgVar{v: v, param: p.Name, fn: fo.DisplayName(), span: call.Fun.Span()})
		}
	}
	// an error type that nothing determined: the call cannot fail
	if errT != nil {
		if v, ok := resolve(errT).(*Var); ok {
			c.bind(v, c.named("Never"))
		}
	}
	for _, s := range sm.effs {
		for _, it := range s.Items {
			if it.EVar != nil && it.EVar.Ref == nil {
				c.bindEff(it.EVar, &EffSet{})
			}
		}
	}
	// effects
	if fo.Declared != nil {
		c.perform(fo.Declared.subst(sm).performed(call.Sp, fo.DisplayName()))
	} else {
		c.perform(&EffSet{Items: []EffItem{{Fn: fo, Span: call.Sp, Via: fo.DisplayName(), Sub: sm}}})
	}
	return c.callResult(result, errT)
}

// checkArgs matches arguments to parameters (positional first, then named,
// defaults for the rest) and checks them. Closure arguments are checked
// after the others so their parameter types are known.
func (c *checker) checkArgs(params []*ParamInfo, call *ast.CallExpr, fname string, ci *callInfo) {
	assigned := make([]ast.Expr, len(params))
	seenNamed := false
	pos := 0
	ok := true
	for _, a := range call.Args {
		if a.Name == nil {
			if seenNamed {
				c.errorf(diag.BadArguments, a.Sp, "positional arguments must come before named ones (core 3.2)")
				ok = false
				continue
			}
			if pos >= len(params) {
				c.errorf(diag.BadArguments, a.Sp, "too many arguments to `%s`: it takes %d", fname, len(params))
				c.infer(a.Value, nil)
				ok = false
				continue
			}
			assigned[pos] = a.Value
			pos++
			continue
		}
		seenNamed = true
		idx := -1
		for i, p := range params {
			if p.Name == a.Name.Name {
				idx = i
			}
		}
		if idx < 0 {
			var cands []string
			for _, p := range params {
				cands = append(cands, p.Name)
			}
			d := c.errorf(diag.BadArguments, a.Name.Sp, "`%s` has no parameter named `%s`", fname, a.Name.Name)
			if s := nearest(a.Name.Name, cands); len(s) > 0 && d != nil {
				note(d, token.Span{}, "did you mean `%s`?", s[0])
				fix(d, fmt.Sprintf("replace with `%s`", s[0]), replaceEdit(a.Name.Sp, s[0]))
			}
			c.infer(a.Value, nil)
			ok = false
			continue
		}
		if assigned[idx] != nil {
			c.errorf(diag.BadArguments, a.Name.Sp, "parameter `%s` is given twice", a.Name.Name)
			c.infer(a.Value, nil)
			ok = false
			continue
		}
		assigned[idx] = a.Value
	}
	var missing []string
	for i, p := range params {
		if assigned[i] == nil && p.Default == nil {
			missing = append(missing, p.Name)
		}
	}
	if len(missing) > 0 && ok {
		c.errorf(diag.BadArguments, call.Sp, "missing argument(s) `%s` in call to `%s`", strings.Join(missing, "`, `"), fname)
	}
	for pass := 0; pass < 2; pass++ {
		for i, e := range assigned {
			if e == nil {
				continue
			}
			_, isLit := e.(*ast.FnLit)
			if isLit != (pass == 1) {
				continue
			}
			c.check(e, params[i].Type)
			if params[i].Var {
				c.requireMutable(e, diag.AssignImmutable, fmt.Sprintf("passing to `var` parameter `%s`", params[i].Name))
			}
			if ci != nil {
				ci.params = append(ci.params, &argParam{arg: e, isVar: params[i].Var})
			}
		}
	}
}

// callValue calls a value of function type.
func (c *checker) callValue(ft Type, call *ast.CallExpr, hint Type, name string) Type {
	r := resolve(ft)
	if isBad(r) {
		c.inferArgs(call)
		return badType
	}
	f, ok := r.(*Func)
	if !ok {
		if _, open := r.(*Var); open {
			c.errorf(diag.CannotInfer, call.Fun.Span(), "the type of this function value must be known here")
		} else {
			c.errorf(diag.WrongKind, call.Fun.Span(), "`%s` is not a function (its type is `%s`)", exprText(call.Fun, name), deep(ft))
		}
		c.inferArgs(call)
		return badType
	}
	params := make([]*ParamInfo, len(f.Params))
	for i, p := range f.Params {
		params[i] = &ParamInfo{Name: fmt.Sprintf("#%d", i+1), Type: p.T, Var: p.Var}
	}
	for _, a := range call.Args {
		if a.Name != nil {
			c.errorf(diag.BadArguments, a.Name.Sp, "a function value takes positional arguments only")
			a.Name = nil
		}
	}
	ci := &callInfo{}
	c.callInfo[call] = ci
	if name == "" {
		name = "function value"
	}
	c.checkArgs(params, call, name, ci)
	via := name
	if f.Eff != nil {
		c.perform(f.Eff.performed(call.Sp, via))
	}
	return c.callResult(f.Result, f.Err)
}

func exprText(e ast.Expr, name string) string {
	if name != "" {
		return name
	}
	if n := calleeName(e); n != "" {
		return n
	}
	return "this value"
}

// typeCall types `T(x)`: numeric conversions (core 2.1) and newtype
// construction (core 2.8).
func (c *checker) typeCall(obj *TypeObj, targs []Type, call *ast.CallExpr, hint Type) Type {
	if obj.Kind == KAlias {
		o2, a2 := c.aliasNamed(obj, targs, call.Fun.Span())
		if o2 == nil {
			c.inferArgs(call)
			return badType
		}
		obj, targs = o2, a2
	}
	c.rec.typeCall[call] = obj
	if obj.Module == c.prelude && (intTypes[obj.Name] || floatTypes[obj.Name]) {
		if len(call.Args) != 1 || call.Args[0].Name != nil {
			c.errorf(diag.BadArguments, call.Sp, "conversion `%s(x)` takes one positional argument", obj.Name)
			c.inferArgs(call)
			return c.named(obj.Name)
		}
		at := c.infer(call.Args[0].Value, nil)
		if intTypes[obj.Name] && c.isPrelude(at, "Rune") {
			// a Rune's code point (core 2.1, decision 0079)
			return c.named(obj.Name)
		}
		if !isBad(at) && !c.isNumeric(at) && !isOpenVar(at) {
			d := c.errorf(diag.BadConversion, call.Args[0].Value.Span(), "cannot convert `%s` to `%s`; numeric conversions take a number", deep(at), obj.Name)
			switch {
			case c.isPrelude(at, "Str"):
				note(d, token.Span{}, "parse text with `%s.parse(s)`, which returns `%s?` (core 2.1)", obj.Name, obj.Name)
			case c.isPrelude(at, "Rune"):
				note(d, token.Span{}, "a Rune converts to an integer type, its code point: `Float(Int(r))` (core 2.1)")
			}
		}
		return c.named(obj.Name)
	}
	if obj.Module == c.prelude && obj.Name == "Rune" {
		// `Rune(n)`: the character with code point n; faults when n is not
		// a Unicode scalar value (core 2.1, decision 0079)
		if len(call.Args) != 1 || call.Args[0].Name != nil {
			c.errorf(diag.BadArguments, call.Sp, "conversion `Rune(n)` takes one positional argument")
			c.inferArgs(call)
			return c.named("Rune")
		}
		at := c.infer(call.Args[0].Value, nil)
		if !isBad(at) && !c.isInt(at) && !c.isPrelude(at, "Rune") && !isOpenVar(at) {
			d := c.errorf(diag.BadConversion, call.Args[0].Value.Span(), "cannot convert `%s` to `Rune`; a Rune is made from an integer code point", deep(at))
			switch {
			case c.isPrelude(at, "Str"):
				note(d, token.Span{}, "the characters of a Str are `s.runes()` (std.md 2.2)")
			case c.isFloat(at):
				note(d, token.Span{}, "convert to an integer first: `Rune(Int(x))` (core 2.1)")
			}
		}
		return c.named("Rune")
	}
	if obj.Kind == KNewtype {
		nt, ok := c.instantiate(obj, targs, call.Fun.Span())
		if !ok {
			c.inferArgs(call)
			return badType
		}
		if hint != nil {
			c.tryUnify(nt, hint)
		}
		if len(call.Args) != 1 || call.Args[0].Name != nil {
			c.errorf(diag.BadArguments, call.Sp, "newtype `%s(x)` takes one positional argument", obj.Name)
			c.inferArgs(call)
			return nt
		}
		c.check(call.Args[0].Value, subst(obj.Target, typeArgsSubst(obj.TParams, nt.Args)))
		c.ctorCall(call)
		return nt
	}
	d := c.errorf(diag.WrongKind, call.Fun.Span(), "`%s` is a type and cannot be called", obj.QualName())
	if d != nil {
		switch {
		case obj.Kind == KStruct && !obj.Opaque:
			note(d, token.Span{}, "build a struct with a literal: `%s{field: value}` (core 2.2)", obj.Name)
		case obj.Kind == KEnum:
			note(d, token.Span{}, "construct a variant: `%s.Variant(...)` (core 2.3)", obj.Name)
		case obj.Module == c.prelude && obj.Name == "Str":
			note(d, token.Span{}, "convert to text with interpolation: `\"${x}\"` (core 1.6)")
		}
	}
	c.inferArgs(call)
	return badType
}

func (c *checker) ctorCall(call *ast.CallExpr) {
	if c.ctorCalls == nil {
		c.ctorCalls = map[*ast.CallExpr]bool{}
	}
	c.ctorCalls[call] = true
}

func (c *checker) isConstructorCall(x *ast.CallExpr) bool { return c.ctorCalls[x] }

// construct builds an enum variant (core 2.3). call is nil for a unit
// variant used as a value.
func (c *checker) construct(v *VariantObj, targs []Type, call *ast.CallExpr, hint Type, sp token.Span, e ast.Expr) Type {
	enum := v.Enum
	if e != nil {
		c.rec.ctor[e] = v
	}
	nt, ok := c.instantiate(enum, targs, sp)
	if !ok {
		if call != nil {
			c.inferArgs(call)
		}
		return badType
	}
	if hint != nil {
		h := resolve(hint)
		if w, ok := h.(*retWant); ok {
			h = resolve(w.T)
		}
		if el := c.optionElem(h); el != nil && !(enum.Module == c.prelude && enum.Name == "Option") {
			h = resolve(el)
		}
		c.tryUnify(nt, h)
	}
	s := typeArgsSubst(enum.TParams, nt.Args)
	if call == nil {
		if len(v.Fields) > 0 {
			c.errorf(diag.BadArguments, sp, "variant `%s` has fields; construct it with arguments: `%s(...)`", v.Name, v.Name)
		}
		return nt
	}
	c.ctorCall(call)
	if v.Unit || len(v.Fields) == 0 {
		if len(call.Args) > 0 {
			c.errorf(diag.BadArguments, call.Sp, "variant `%s` has no fields", v.Name)
			c.inferArgs(call)
		}
		return nt
	}
	params := make([]*ParamInfo, len(v.Fields))
	for i, f := range v.Fields {
		name := f.Name
		if f.From {
			name = ""
		}
		params[i] = &ParamInfo{Name: name, Type: subst(f.Type, s)}
	}
	if len(v.Fields) > 1 {
		for _, a := range call.Args {
			if a.Name == nil {
				var names []string
				for _, f := range v.Fields {
					names = append(names, f.Name+": ...")
				}
				c.errorf(diag.BadArguments, a.Sp, "variant `%s` has %d fields; construct it with names: `%s(%s)` (core 2.3)", v.Name, len(v.Fields), v.Name, strings.Join(names, ", "))
				c.inferArgs(call)
				return nt
			}
		}
	}
	c.checkArgs(params, call, v.Name, nil)
	return nt
}

// isExpectCall reports whether call is the predeclared `expect(x)` of a
// test (core 9.2).
func (c *checker) isExpectCall(call *ast.CallExpr) bool {
	id, ok := call.Fun.(*ast.Ident)
	return ok && id.Name == "expect" && c.inTestContext() && c.lookupLocal("expect") == nil && c.mod.Scope["expect"] == nil
}

// expectCompare types `expect(a) op b` as the comparison `a op b`.
func (c *checker) expectCompare(x *ast.BinaryExpr, call *ast.CallExpr) Type {
	if len(call.Args) != 1 {
		c.errorf(diag.BadArguments, call.Sp, "`expect` takes one argument")
		c.inferArgs(call)
		c.infer(x.Y, nil)
		return unitType
	}
	at := c.infer(call.Args[0].Value, nil)
	c.exprType[call] = c.named("Bool")
	switch x.Op {
	case token.Eq, token.NotEq, token.Lt, token.LtEq, token.Gt, token.GtEq:
	default:
		c.errorf(diag.BadOperand, x.Sp, "`expect(x)` must be followed by a comparison: ==, !=, <, <=, >, >=")
		c.infer(x.Y, nil)
		return unitType
	}
	var yt Type
	if isOpenVar(at) {
		yt = c.infer(x.Y, nil)
	} else {
		yt = c.infer(x.Y, at)
	}
	c.binaryType(x.Op, at, yt, x.Sp, call.Args[0].Value, x.Y)
	return unitType
}

// mentions reports whether type t refers to type parameter p.
func mentions(t Type, p *TParam) bool {
	switch x := t.(type) {
	case *ParamType:
		return x.P == p
	case *Named:
		for _, a := range x.Args {
			if mentions(a, p) {
				return true
			}
		}
	case *Tuple:
		for _, e := range x.Elems {
			if mentions(e, p) {
				return true
			}
		}
	case *Func:
		for _, q := range x.Params {
			if mentions(q.T, p) {
				return true
			}
		}
		return mentions(x.Result, p) || x.Err != nil && mentions(x.Err, p)
	}
	return false
}
