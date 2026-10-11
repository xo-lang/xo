package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// protectedNames cannot be redefined at top level (core 1.4): predeclared
// values and functions, the builtin types of core 2.1, and the prelude
// interfaces including the capability interfaces (std.md 1, 3).
var protectedNames = map[string]bool{
	"true": true, "false": true, "self": true, "target": true, "fault": true, "dbg": true,
	"Ok": true, "Err": true, "Some": true, "None": true,
	"Bool": true, "Int": true, "Int8": true, "Int16": true, "Int32": true, "Int64": true,
	"UInt8": true, "UInt16": true, "UInt32": true, "UInt64": true, "Float": true, "Float32": true,
	"Rune": true, "Str": true, "Bytes": true, "Duration": true, "Time": true, "List": true,
	"Map": true, "Set": true, "Buf": true, "Chan": true, "Mutex": true, "Atomic": true,
	"Secret": true, "Option": true, "Result": true, "Never": true, "StrBuilder": true, "BytesBuilder": true,
	"Display": true, "Error": true, "Debug": true, "Eq": true, "Hash": true, "Ord": true,
	"Json": true, "OpenApi": true, "Proto": true, "SqlRow": true,
	"Os": true, "Net": true, "Fs": true, "Clock": true, "Rand": true, "Env": true, "Proc": true, "Stdio": true,
}

// typeEnv is the context for resolving type expressions.
type typeEnv struct {
	file   *File
	mod    *Module
	params map[string]*TParam
	// implicit collects implicit receiver parameters (std encoding rule 5)
	// when non nil.
	implicit *[]*TParam
	quiet    bool
	// argParam is the type parameter whose argument is being resolved,
	// so an implicit receiver parameter inherits its kind and constraint.
	argParam *TParam
	// elided collects the implicit effect parameters created for effect
	// arguments left out of a type (core 6.2, decision 0033): in a
	// function signature they become effect variables of the function, in
	// a struct or enum field effect parameters of the type. When nil (and
	// implicit is nil) an omitted effect argument is inferred.
	elided *[]*TParam
}

func (e *typeEnv) with(ps []*TParam) *typeEnv {
	n := *e
	n.params = map[string]*TParam{}
	for k, v := range e.params {
		n.params[k] = v
	}
	for _, p := range ps {
		n.params[p.Name] = p
	}
	n.implicit = nil
	n.elided = nil
	return &n
}

func (c *checker) define(m *Module, name string, obj Object, sp token.Span, f *File) {
	if name == "" {
		return
	}
	if !m.IsStd && protectedNames[name] {
		c.errorf(diag.Duplicate, sp, "`%s` is predeclared and cannot be redefined at top level (core 1.4)", name)
		return
	}
	if prev, ok := m.Scope[name]; ok {
		d := c.errorf(diag.Duplicate, sp, "`%s` is already declared in this module", name)
		note(d, objSpan(prev), "previous declaration")
		return
	}
	m.Scope[name] = obj
}

func objSpan(o Object) token.Span {
	switch x := o.(type) {
	case *TypeObj:
		return x.Span
	case *FuncObj:
		return x.Span
	case *ConstObj:
		if x.Decl != nil {
			return x.Decl.Name.Sp
		}
	case *VarObj:
		return x.Span
	}
	return token.Span{}
}

func (c *checker) newTParams(tps []*ast.TypeParam) []*TParam {
	var out []*TParam
	for _, tp := range tps {
		p := &TParam{Name: tp.Name.Name, IsEffect: tp.Effects, Span: tp.Name.Sp}
		if tp.Effects {
			c.checkName(tp.Name, "effect variable")
		} else {
			c.checkName(tp.Name, "type parameter")
		}
		out = append(out, p)
	}
	return out
}

// declare creates objects for every top level declaration of m.
func (c *checker) declare(m *Module) {
	c.mod = m
	for _, f := range m.Files {
		c.file = f
		for _, u := range f.AST.Uses {
			if u.Alias != nil {
				c.checkName(u.Alias, "module")
			}
		}
		for _, d := range f.AST.Decls {
			switch x := d.(type) {
			case *ast.FnDecl:
				fo := &FuncObj{Name: x.Name.Name, Module: m, File: f, Pub: x.Pub || m.IsStd, Decl: x, Span: x.Name.Sp}
				fo.Moves = x.Doc != nil && strings.Contains(x.Doc.Text, "@moves")
				if x.Recv != nil {
					continue // attached to the receiver type in resolveSigs
				}
				c.checkName(x.Name, "function")
				c.define(m, x.Name.Name, fo, x.Name.Sp, f)
			case *ast.StructDecl:
				t := &TypeObj{Name: x.Name.Name, Kind: KStruct, Module: m, Pub: x.Pub || m.IsStd, Span: x.Name.Sp, structDecl: x}
				t.TParams = c.newTParams(x.TypeParams)
				t.Opaque = m.IsStd && len(x.Fields) == 0
				c.checkName(x.Name, "type")
				c.define(m, t.Name, t, x.Name.Sp, f)
				t.Methods, t.Statics = map[string]*FuncObj{}, map[string]*FuncObj{}
				t.Invariant = x.Invariant
			case *ast.EnumDecl:
				t := &TypeObj{Name: x.Name.Name, Kind: KEnum, Module: m, Pub: x.Pub || m.IsStd, Span: x.Name.Sp, enumDecl: x}
				t.TParams = c.newTParams(x.TypeParams)
				c.checkName(x.Name, "type")
				c.define(m, t.Name, t, x.Name.Sp, f)
				t.Methods, t.Statics = map[string]*FuncObj{}, map[string]*FuncObj{}
			case *ast.InterfaceDecl:
				t := &TypeObj{Name: x.Name.Name, Kind: KInterface, Module: m, Pub: x.Pub || m.IsStd, Span: x.Name.Sp, ifaceDecl: x}
				t.TParams = c.newTParams(x.TypeParams)
				c.checkName(x.Name, "interface")
				c.define(m, t.Name, t, x.Name.Sp, f)
				t.Methods, t.Statics = map[string]*FuncObj{}, map[string]*FuncObj{}
			case *ast.TypeDecl:
				k := KAlias
				if x.Newtype {
					k = KNewtype
				}
				t := &TypeObj{Name: x.Name.Name, Kind: k, Module: m, Pub: x.Pub || m.IsStd, Span: x.Name.Sp, typeDecl: x}
				t.TParams = c.newTParams(x.TypeParams)
				c.checkName(x.Name, "type")
				c.define(m, t.Name, t, x.Name.Sp, f)
				t.Methods, t.Statics = map[string]*FuncObj{}, map[string]*FuncObj{}
			case *ast.ConstDecl:
				co := &ConstObj{Name: x.Name.Name, Module: m, File: f, Pub: x.Pub || m.IsStd, Decl: x}
				c.checkName(x.Name, "constant")
				c.define(m, co.Name, co, x.Name.Sp, f)
			}
		}
	}
}

// resolveTypes resolves the bodies of type declarations of m.
func (c *checker) resolveTypes(m *Module) {
	c.mod = m
	names := sortedKeys(m.Scope)
	for _, n := range names {
		if t, ok := m.Scope[n].(*TypeObj); ok {
			c.resolveTypeObj(t)
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (c *checker) declFile(t *TypeObj) *File {
	var n ast.Node
	switch {
	case t.structDecl != nil:
		n = t.structDecl
	case t.enumDecl != nil:
		n = t.enumDecl
	case t.ifaceDecl != nil:
		n = t.ifaceDecl
	case t.typeDecl != nil:
		n = t.typeDecl
	}
	if n == nil {
		return nil
	}
	for _, f := range t.Module.Files {
		if f.Path == n.Span().File {
			return f
		}
	}
	return nil
}

func (c *checker) resolveTypeObj(t *TypeObj) {
	if t.resolved || t.resolving {
		return
	}
	t.resolving = true
	defer func() { t.resolving, t.resolved = false, true }()
	savedMod, savedFile := c.mod, c.file
	c.mod, c.file = t.Module, c.declFile(t)
	defer func() { c.mod, c.file = savedMod, savedFile }()
	env := (&typeEnv{file: c.file, mod: t.Module}).with(t.TParams)
	var elided []*TParam
	env.elided = &elided
	defer func() { t.TParams = append(t.TParams, elided...) }()
	switch {
	case t.structDecl != nil:
		c.resolveConstraints(t.structDecl.TypeParams, t.TParams, env)
		seen := map[string]bool{}
		for _, f := range t.structDecl.Fields {
			c.checkName(f.Name, "field")
			if seen[f.Name.Name] {
				c.errorf(diag.Duplicate, f.Name.Sp, "field `%s` is declared twice", f.Name.Name)
				continue
			}
			seen[f.Name.Name] = true
			t.Fields = append(t.Fields, &FieldObj{Name: f.Name.Name, Type: c.resolveType(f.Type, env), Pub: f.Pub || t.Module.IsStd, Default: f.Default, Span: f.Name.Sp})
		}
		t.Derives = c.derives(t.structDecl.Derive)
	case t.enumDecl != nil:
		c.resolveConstraints(t.enumDecl.TypeParams, t.TParams, env)
		seen := map[string]bool{}
		for i, v := range t.enumDecl.Variants {
			c.checkName(v.Name, "variant")
			if seen[v.Name.Name] {
				c.errorf(diag.Duplicate, v.Name.Sp, "variant `%s` is declared twice", v.Name.Name)
				continue
			}
			seen[v.Name.Name] = true
			vo := &VariantObj{Name: v.Name.Name, Enum: t, Index: i, Span: v.Name.Sp, Unit: v.Fields == nil}
			fseen := map[string]bool{}
			for _, vf := range v.Fields {
				fo := &VFieldObj{From: vf.From, Type: c.resolveType(vf.Type, env)}
				if vf.Name != nil {
					fo.Name = vf.Name.Name
					fo.Span = vf.Name.Sp
					c.checkName(vf.Name, "field")
					if fseen[fo.Name] {
						c.errorf(diag.Duplicate, vf.Name.Sp, "field `%s` is declared twice", fo.Name)
					}
					fseen[fo.Name] = true
				}
				if vf.From && len(v.Fields) != 1 {
					c.errorf(diag.BadArguments, vf.Sp, "a `from` variant has exactly one field (core 2.6)")
				}
				vo.Fields = append(vo.Fields, fo)
			}
			t.Variants = append(t.Variants, vo)
		}
		t.Derives = c.derives(t.enumDecl.Derive)
	case t.ifaceDecl != nil:
		c.resolveConstraints(t.ifaceDecl.TypeParams, t.TParams, env)
		seen := map[string]bool{}
		for _, md := range t.ifaceDecl.Methods {
			fo := &FuncObj{Name: md.Name.Name, Module: t.Module, File: c.file, Pub: true, Decl: md, Span: md.Name.Sp, Iface: true, Recv: t}
			c.checkName(md.Name, "method")
			if seen[fo.Name] {
				c.errorf(diag.Duplicate, md.Name.Sp, "method `%s` is declared twice", fo.Name)
				continue
			}
			seen[fo.Name] = true
			if len(md.Params) == 0 || !md.Params[0].Self {
				if !t.Module.IsStd {
					c.errorf(diag.BadArguments, md.Name.Sp, "interface method `%s` must take `self` first (core 2.4)", fo.Name)
				}
			}
			c.resolveSig(fo, env, true)
			t.IfaceMeths = append(t.IfaceMeths, fo)
		}
	case t.typeDecl != nil:
		c.resolveConstraints(t.typeDecl.TypeParams, t.TParams, env)
		t.Target = c.resolveType(t.typeDecl.Type, env)
	}
}

func (c *checker) resolveConstraints(tps []*ast.TypeParam, ps []*TParam, env *typeEnv) {
	for i, tp := range tps {
		if tp.Constraint != nil && i < len(ps) {
			ct := c.resolveType(tp.Constraint, env)
			if !isBad(ct) && !c.isInterface(ct) {
				c.errorf(diag.WrongKind, tp.Constraint.Span(), "constraint `%s` is not an interface (core 2.7)", ct)
				ct = nil
			}
			ps[i].Constraint = ct
		}
	}
}

var derivable = map[string]bool{"Display": true, "Ord": true, "Json": true, "OpenApi": true, "Proto": true, "Debug": true, "Eq": true, "Hash": true, "SqlRow": true}

func (c *checker) derives(ids []*ast.Ident) map[string]bool {
	out := map[string]bool{}
	for _, id := range ids {
		if !derivable[id.Name] {
			c.unresolved(id.Sp, "derivable interface", id.Name, sortedKeys(derivable))
			continue
		}
		out[id.Name] = true
	}
	return out
}

// aliasTarget returns the resolved target of an alias, guarding cycles.
func (c *checker) aliasTarget(t *TypeObj, sp token.Span) Type {
	if t.resolving {
		c.errorf(diag.WrongKind, sp, "type alias `%s` refers to itself", t.Name)
		return badType
	}
	c.resolveTypeObj(t)
	if t.Target == nil {
		return badType
	}
	return t.Target
}

// resolveSigs resolves function signatures and attaches methods.
func (c *checker) resolveSigs(m *Module) {
	c.mod = m
	for _, f := range m.Files {
		c.file = f
		for _, d := range f.AST.Decls {
			fd, ok := d.(*ast.FnDecl)
			if !ok {
				continue
			}
			var fo *FuncObj
			if fd.Recv == nil {
				fo, _ = m.Scope[fd.Name.Name].(*FuncObj)
				if fo == nil || fo.Decl != fd {
					fo = &FuncObj{Name: fd.Name.Name, Module: m, File: f, Pub: fd.Pub || m.IsStd, Decl: fd, Span: fd.Name.Sp}
				}
			} else {
				fo = &FuncObj{Name: fd.Name.Name, Module: m, File: f, Pub: fd.Pub || m.IsStd, Decl: fd, Span: fd.Name.Sp}
				fo.Moves = fd.Doc != nil && strings.Contains(fd.Doc.Text, "@moves")
			}
			env := &typeEnv{file: f, mod: m, params: map[string]*TParam{}}
			if fd.Recv != nil {
				if !c.attachMethod(fo, fd, env) {
					continue
				}
			}
			c.resolveSig(fo, env, false)
		}
	}
}

func (c *checker) attachMethod(fo *FuncObj, fd *ast.FnDecl, env *typeEnv) bool {
	var implicit []*TParam
	renv := *env
	renv.implicit = &implicit
	rt := c.resolveType(fd.Recv.Type, &renv)
	fo.RecvVar = fd.Recv.Var
	fo.Static = fd.Recv.Name.Name == "static" && c.mod.IsStd
	n, ok := resolve(rt).(*Named)
	if !ok {
		if !isBad(rt) {
			c.errorf(diag.WrongKind, fd.Recv.Type.Span(), "a method receiver must be a named type, found `%s`", rt)
		}
		return false
	}
	if n.Obj.Kind == KInterface {
		c.errorf(diag.WrongKind, fd.Recv.Type.Span(), "methods cannot be declared on interface `%s`", n.Obj.Name)
		return false
	}
	if n.Obj.Module != c.mod {
		c.errorf(diag.ForeignMethod, fd.Name.Sp, "method `%s` is declared on `%s` from another module; methods may be declared only in the module that declares the type (core 3.4)", fd.Name.Name, n.Obj.QualName())
		return false
	}
	if !fo.Static {
		c.checkName(fd.Recv.Name, "receiver")
	}
	c.checkName(fd.Name, "method")
	fo.Recv = n.Obj
	fo.RecvType = rt
	fo.TParams = append(fo.TParams, implicit...)
	for _, p := range implicit {
		env.params[p.Name] = p
	}
	table := n.Obj.Methods
	if fo.Static {
		table = n.Obj.Statics
		fo.Const = len(fd.Params) == 0 && isUpperSnake(fd.Name.Name)
	}
	if prev, ok := table[fo.Name]; ok {
		d := c.errorf(diag.Duplicate, fd.Name.Sp, "method `%s` is already declared on `%s`", fo.Name, n.Obj.Name)
		note(d, prev.Span, "previous declaration")
		return false
	}
	if n.Obj.field(fo.Name) != nil && !c.mod.IsStd {
		c.errorf(diag.Duplicate, fd.Name.Sp, "`%s` is both a field and a method of `%s`", fo.Name, n.Obj.Name)
	}
	table[fo.Name] = fo
	return true
}

// resolveSig fills the parameter, result, error, and effect information of
// fo from its declaration.
func (c *checker) resolveSig(fo *FuncObj, env *typeEnv, iface bool) {
	fd := fo.Decl
	own := c.newTParams(fd.TypeParams)
	fo.TParams = append(fo.TParams, own...)
	env = env.with(fo.TParams)
	var elided []*TParam
	env.elided = &elided
	c.resolveConstraints(fd.TypeParams, own, env)
	seen := map[string]bool{}
	for _, p := range fd.Params {
		if p.Self {
			continue
		}
		if !iface {
			c.checkName(p.Name, "parameter")
		}
		if seen[p.Name.Name] && p.Name.Name != "_" {
			c.errorf(diag.Duplicate, p.Name.Sp, "parameter `%s` is declared twice", p.Name.Name)
		}
		seen[p.Name.Name] = true
		var pt Type = badType
		if p.Type != nil {
			pt = c.resolveType(p.Type, env)
		}
		fo.Params = append(fo.Params, &ParamInfo{Name: p.Name.Name, Type: pt, Var: p.Var, Default: p.Default, Span: p.Name.Sp})
	}
	// An effect argument left out of the result or error type is inferred
	// from the body, like a local binding: a variable chosen by the caller
	// could never be satisfied by a body that builds the value (core 6.2,
	// reading recorded in the 0.4 corpus report).
	renv := *env
	renv.elided = nil
	fo.Result = unitType
	if fd.Result != nil {
		fo.Result = c.resolveType(fd.Result, &renv)
	}
	fo.Err = c.resolveErrSpec(fd.Err, &renv)
	var uses []*ast.Effect
	hasUses := false
	for _, cl := range fd.Clauses {
		if u, ok := cl.(*ast.UsesClause); ok {
			for _, e := range u.Effects {
				if e.Sub == nil && e.Name.Name == realtimeLabel && !iface {
					// a discipline, not an effect (core 6.5)
					fo.Realtime = true
					fo.RealtimeSpan = e.Sp
					continue
				}
				uses = append(uses, e)
			}
			hasUses = true
		}
	}
	set := c.resolveEffects(uses, env)
	if fo.Realtime {
		c.realtimeFns = append(c.realtimeFns, fo)
		for _, e := range uses {
			name := e.Name.Name
			if e.Sub != nil {
				name += "." + e.Sub.Name
			}
			if effectLabels[name] && name != "ffi" {
				d := c.errorf(diag.RealtimeBlocking, e.Sp, "a real-time function may not perform `%s`; the only effect it may declare is `ffi`", name)
				if d != nil {
					note(d, fo.RealtimeSpan, "`%s` is real-time (core 6.5)", fo.DisplayName())
				}
			}
		}
	}
	// Effect arguments omitted in the signature are implicit effect
	// variables of the function, which it may perform (core 6.2).
	fo.TParams = append(fo.TParams, elided...)
	for _, p := range fo.TParams {
		if p.Elided {
			set.add(EffItem{Param: p})
		}
	}
	if fo.Pub || iface || fo.Module.IsStd || hasUses {
		fo.Declared = set
	} else {
		c.inferredFns = append(c.inferredFns, fo)
	}
	fo.sigDone = true
}

func (c *checker) resolveErrSpec(es *ast.ErrSpec, env *typeEnv) Type {
	if es == nil {
		return nil
	}
	if es.Type == nil {
		return c.named("Error")
	}
	t := c.resolveType(es.Type, env)
	if c.isNever(t) {
		return nil
	}
	return t
}

// resolveEffects resolves effect labels and effect variables.
func (c *checker) resolveEffects(effs []*ast.Effect, env *typeEnv) *EffSet {
	set := &EffSet{}
	for _, e := range effs {
		name := e.Name.Name
		if e.Sub != nil {
			name += "." + e.Sub.Name
		}
		if effectLabels[name] {
			set.add(EffItem{Label: name})
			continue
		}
		if name == realtimeLabel {
			rtLabelError(c, e)
			continue
		}
		if e.Sub == nil {
			if p, ok := env.params[name]; ok && p.IsEffect {
				set.add(EffItem{Param: p})
				continue
			}
		}
		var cands []string
		for l := range effectLabels {
			cands = append(cands, l)
		}
		for n, p := range env.params {
			if p.IsEffect {
				cands = append(cands, n)
			}
		}
		sort.Strings(cands)
		d := c.errorf(diag.UnknownEffect, e.Sp, "unknown effect `%s`", name)
		if d != nil {
			if s := nearest(name, cands); len(s) > 0 {
				note(d, token.Span{}, "did you mean `%s`?", strings.Join(s, "`, `"))
				fix(d, fmt.Sprintf("replace with `%s`", s[0]), replaceEdit(e.Sp, s[0]))
			} else {
				note(d, token.Span{}, "effects are net, net.dial, net.listen, fs, fs.read, fs.write, clock, rand, env, proc, stdio, ffi, log, go, or an effect variable declared as `fx: Effects` (core 6.1, 6.2)")
			}
		}
	}
	return set
}

// ---------------------------------------------------------------------------
// Type expressions

// lookupTypeIn finds a type name visible in the current module: module
// scope, then the prelude.
func (c *checker) lookupModuleObj(m *Module, name string) Object {
	if o, ok := m.Scope[name]; ok {
		return o
	}
	if o, ok := c.prelude.Scope[name]; ok {
		return o
	}
	return nil
}

func (c *checker) visibleTypeNames(env *typeEnv) []string {
	var out []string
	for n := range env.params {
		out = append(out, n)
	}
	for _, m := range []*Module{env.mod, c.prelude} {
		if m == nil {
			continue
		}
		for n, o := range m.Scope {
			if _, ok := o.(*TypeObj); ok {
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

func (c *checker) resolveType(t ast.Type, env *typeEnv) Type {
	switch x := t.(type) {
	case nil:
		return badType
	case *ast.BadType:
		return badType
	case *ast.OptionalType:
		return c.optionOf(c.resolveType(x.Elem, env))
	case *ast.TupleType:
		if len(x.Elems) == 0 {
			return unitType
		}
		el := make([]Type, len(x.Elems))
		for i, e := range x.Elems {
			el[i] = c.resolveType(e, env)
		}
		return &Tuple{Elems: el}
	case *ast.FnType:
		ft := &Func{Result: unitType}
		for _, p := range x.Params {
			ft.Params = append(ft.Params, FParam{Var: p.Var, T: c.resolveType(p.Type, env)})
		}
		if x.Result != nil {
			ft.Result = c.resolveType(x.Result, env)
		}
		ft.Err = c.resolveErrSpec(x.Err, env)
		if len(x.Uses) > 0 {
			ft.Eff = c.resolveEffects(x.Uses, env)
		}
		return ft
	case *ast.NamedType:
		return c.resolveNamedType(x, env)
	}
	return badType
}

func (c *checker) resolveNamedType(x *ast.NamedType, env *typeEnv) Type {
	var obj *TypeObj
	last := x.Path[len(x.Path)-1]
	switch len(x.Path) {
	case 1:
		name := x.Path[0].Name
		if p, ok := env.params[name]; ok {
			if p.IsEffect {
				c.errorf(diag.WrongKind, x.Sp, "`%s` is an effect variable, not a type", name)
				return badType
			}
			if len(x.Args) > 0 {
				c.errorf(diag.BadArguments, x.Sp, "type parameter `%s` takes no type arguments", name)
			}
			c.rec.name(x.Path[0], p)
			return &ParamType{P: p}
		}
		if name == "Effects" {
			c.errorf(diag.WrongKind, x.Sp, "`Effects` is only valid as the kind of an effect variable: `fx: Effects` (core 6.2)")
			return badType
		}
		o := c.lookupModuleObj(env.mod, name)
		if o == nil {
			if env.implicit != nil && len(x.Args) == 0 && isUpperCamel(name) {
				p := &TParam{Name: name, Implicit: true, Span: x.Path[0].Sp}
				if env.argParam != nil {
					p.Constraint = env.argParam.Constraint
				}
				*env.implicit = append(*env.implicit, p)
				env.params = copyParams(env.params)
				env.params[name] = p
				c.rec.name(x.Path[0], p)
				return &ParamType{P: p}
			}
			if t, ok := c.habitType(x, env); ok {
				return t
			}
			if !env.quiet {
				c.unresolved(x.Path[0].Sp, "type", name, c.visibleTypeNames(env))
			}
			return badType
		}
		to, ok := o.(*TypeObj)
		if !ok {
			c.errorf(diag.WrongKind, x.Sp, "`%s` is not a type", name)
			return badType
		}
		c.rec.name(x.Path[0], to)
		obj = to
	case 2:
		modName := x.Path[0].Name
		var imp *Import
		if env.file != nil {
			imp = env.file.Imports[modName]
			// `testing` is predeclared anywhere in a _test.xo file (core 9.2),
			// including helper signatures outside test blocks.
			if imp == nil && modName == "testing" && env.file.IsTest {
				imp = &Import{Name: "testing", Module: c.modules["std/testing"]}
			}
		}
		if imp == nil {
			c.unresolved(x.Path[0].Sp, "module", modName, importNames(env.file))
			return badType
		}
		imp.Used = true
		if imp.Module == nil {
			return badType
		}
		c.rec.name(x.Path[0], imp.Module)
		o := imp.Module.Scope[last.Name]
		if o == nil {
			var cands []string
			for n, oo := range imp.Module.Scope {
				if _, ok := oo.(*TypeObj); ok {
					cands = append(cands, n)
				}
			}
			sort.Strings(cands)
			c.unresolved(last.Sp, "type of module `"+modName+"`", last.Name, cands)
			return badType
		}
		to, ok := o.(*TypeObj)
		if !ok {
			c.errorf(diag.WrongKind, x.Sp, "`%s.%s` is not a type", modName, last.Name)
			return badType
		}
		if !to.Pub && imp.Module != env.mod {
			c.errorf(diag.NotVisible, last.Sp, "type `%s` is not `pub` in module `%s`", last.Name, imp.Module.Path)
		}
		c.rec.name(last, to)
		obj = to
	default:
		c.errorf(diag.UnresolvedName, x.Sp, "type path `%s` has too many segments", typePathString(x))
		return badType
	}
	if !obj.resolving {
		c.resolveTypeObj(obj)
	}
	args := c.resolveArgsFor(obj, x.Args, env)
	for _, a := range args {
		// Remember where an effect argument was left out, for the XO0701
		// note on it (decision 0087).
		if ea, ok := a.(*EffArg); ok && ea.Set != nil {
			for _, it := range ea.Set.Items {
				if it.Param != nil && it.Param.Elided && it.Param.ElidedAt.File == "" {
					it.Param.ElidedAt = x.Sp
					it.Param.ElidedText = typeText(x)
				}
			}
		}
	}
	if obj.Kind == KAlias {
		if len(args) != len(obj.TParams) {
			c.errorf(diag.BadArguments, x.Sp, "`%s` takes %d type argument(s), found %d", obj.Name, len(obj.TParams), len(args))
			return badType
		}
		target := c.aliasTarget(obj, x.Sp)
		return subst(target, typeArgsSubst(obj.TParams, args))
	}
	if len(args) != len(obj.TParams) {
		if env.implicit != nil && len(args) == 0 {
			// receiver `List` without arguments: treat as fully generic
			for _, p := range obj.TParams {
				ip := &TParam{Name: p.Name, Implicit: true, IsEffect: p.IsEffect, Constraint: p.Constraint, Elided: p.IsEffect}
				*env.implicit = append(*env.implicit, ip)
				if p.IsEffect {
					args = append(args, &EffArg{Set: &EffSet{Items: []EffItem{{Param: ip}}}})
				} else {
					args = append(args, &ParamType{P: ip})
				}
			}
		} else {
			c.errorf(diag.BadArguments, x.Sp, "`%s` takes %d type argument(s), found %d", obj.QualName(), len(obj.TParams), len(args))
			return badType
		}
	}
	n := &Named{Obj: obj, Args: args}
	if env.mod != nil && !env.mod.IsStd && obj.Module == c.prelude && (obj.Name == "Map" || obj.Name == "Set") {
		c.checkKeyType(n, x.Sp)
	}
	return n
}

func copyParams(m map[string]*TParam) map[string]*TParam {
	n := map[string]*TParam{}
	for k, v := range m {
		n[k] = v
	}
	return n
}

func typePathString(x *ast.NamedType) string {
	var parts []string
	for _, p := range x.Path {
		parts = append(parts, p.Name)
	}
	return strings.Join(parts, ".")
}

func importNames(f *File) []string {
	if f == nil {
		return nil
	}
	return sortedKeys(f.Imports)
}

// resolveArgsFor resolves the type arguments written for obj. When every
// effect argument is left out (`Cache[Int, Item]` for `Cache[K, V, fx:
// Effects]`, core 6.2), the written arguments go to the type parameters in
// order and each effect parameter gets an elided argument (see elidedEffect).
func (c *checker) resolveArgsFor(obj *TypeObj, as []ast.Type, env *typeEnv) []Type {
	nonEff := 0
	for _, p := range obj.TParams {
		if !p.IsEffect {
			nonEff++
		}
	}
	if len(as) == len(obj.TParams) || len(as) != nonEff || (len(as) == 0 && env.implicit != nil) {
		args := make([]Type, len(as))
		for i, a := range as {
			var tp *TParam
			if i < len(obj.TParams) {
				tp = obj.TParams[i]
			}
			args[i] = c.resolveTypeArg(a, tp, env)
		}
		return args
	}
	args := make([]Type, 0, len(obj.TParams))
	i := 0
	for _, tp := range obj.TParams {
		if tp.IsEffect {
			args = append(args, c.elidedEffect(obj, tp, env))
			continue
		}
		args = append(args, c.resolveTypeArg(as[i], tp, env))
		i++
	}
	return args
}

// elidedEffect is the argument of effect parameter tp of obj when it is
// left out (core 6.2, decision 0033): an implicit effect parameter of the
// method receiver, function, or type being declared, or else an effect
// inference variable (local bindings and expressions).
func (c *checker) elidedEffect(obj *TypeObj, tp *TParam, env *typeEnv) *EffArg {
	collect := env.implicit
	if collect == nil {
		collect = env.elided
	}
	if collect == nil {
		return &EffArg{Set: &EffSet{Items: []EffItem{{EVar: &EffVar{}}}}}
	}
	p := &TParam{Name: obj.Name + "." + tp.Name, Implicit: true, IsEffect: true, Elided: true}
	*collect = append(*collect, p)
	return &EffArg{Set: &EffSet{Items: []EffItem{{Param: p}}}}
}

// resolveTypeArg resolves the argument for type parameter tp: a type, or an
// effect set when tp is an effect parameter.
func (c *checker) resolveTypeArg(a ast.Type, tp *TParam, env *typeEnv) Type {
	if tp != nil && tp.IsEffect {
		return &EffArg{Set: c.effectArg(a, env)}
	}
	saved := env.argParam
	env.argParam = tp
	t := c.resolveType(a, env)
	env.argParam = saved
	return t
}

// effectArg resolves an effect set written as a type argument: a label
// (`net.dial`), an effect variable, or a parenthesized list.
func (c *checker) effectArg(a ast.Type, env *typeEnv) *EffSet {
	set := &EffSet{}
	switch x := a.(type) {
	case *ast.TupleType:
		for _, e := range x.Elems {
			set.addAll(c.effectArg(e, env))
		}
		return set
	case *ast.NamedType:
		if len(x.Args) > 0 || len(x.Path) > 2 {
			break
		}
		name := x.Path[0].Name
		if len(x.Path) == 2 {
			name += "." + x.Path[1].Name
		}
		if effectLabels[name] {
			set.add(EffItem{Label: name})
			return set
		}
		if len(x.Path) == 1 {
			if p, ok := env.params[name]; ok && p.IsEffect {
				set.add(EffItem{Param: p})
				return set
			}
			if env.implicit != nil && isSnake(name) {
				p := &TParam{Name: name, Implicit: true, IsEffect: true}
				*env.implicit = append(*env.implicit, p)
				env.params = copyParams(env.params)
				env.params[name] = p
				set.add(EffItem{Param: p})
				return set
			}
		}
		c.errorf(diag.UnknownEffect, x.Sp, "unknown effect `%s` in an effect argument", name)
		return set
	}
	c.errorf(diag.UnknownEffect, a.Span(), "an effect argument must be an effect label, an effect variable, or a parenthesized list of them")
	return set
}
