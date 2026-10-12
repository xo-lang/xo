package ssa

import (
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Implementation modules of the native backends (decision 0074).
//
// std/native/<name>.xo implements std module std/<name> (and
// std/native/prelude.xo the prelude) in Xo, checked with the program
// when it is built for a native backend (check.Options.Native). The
// builder lowers a std function or method that has an implementation
// there to a call of it, before any lowering of its own:
//
//   - a method `T.name` of a type T that the implementation module
//     declares too: that type's method `name`;
//   - any other method `T.name` (also a static one): the function
//     `t_name` (T in lower case), whose first parameter is the receiver
//     unless the method is static;
//   - a function `name`: the function `name`.
//
// The implementation has the declaration's type parameters, in order. A
// std struct that the implementation module declares too is lowered as
// the implementation's struct: opaque handles such as http.Router get
// fields, and http.Request gets hidden ones.
//
// The intrinsics in std/native/rt.xo are calls of the C runtime
// (`rt.<name>`), except the generic ones lowered in intrinsic.

// nativeRT is the path of the intrinsics module.
const nativeRT = "native/rt"

// nativeMod returns the implementation module of the std module path,
// or nil.
func (b *builder) nativeMod(path string) *check.Module {
	if b.natives == nil {
		b.natives = map[string]*check.Module{}
		for _, m := range b.info.AllModules() {
			if m.Native {
				b.natives[m.Implements] = m
			}
		}
	}
	return b.natives[path]
}

// logFrom is the attribution of the record written by the call at of
// the std/log function fo (decision 0173, as internal/gen's logFrom):
// "<module path>@<version>" when the call is in a dependency's source,
// else "".
func (b *builder) logFrom(fo *check.FuncObj, at ast.Node) string {
	if at == nil || fo == nil || fo.Recv != nil || fo.Module == nil || fo.Module.Path != "std/log" {
		return ""
	}
	switch fo.Name {
	case "debug", "info", "warn", "error":
	default:
		return ""
	}
	if b.fileMods == nil {
		b.fileMods = map[string]*check.Module{}
		for _, m := range b.info.AllModules() {
			if m.IsStd {
				continue
			}
			for _, f := range m.Files {
				b.fileMods[f.Path] = m
			}
		}
	}
	if m := b.fileMods[at.Span().File]; m != nil {
		return m.Dependency
	}
	return ""
}

// implOf returns the implementation of the std function fo, or nil.
func (b *builder) implOf(fo *check.FuncObj) *check.FuncObj {
	if fo.Module == nil || !fo.Module.IsStd || fo.Iface {
		return nil
	}
	nm := b.nativeMod(fo.Module.Path)
	if nm == nil {
		return nil
	}
	name := fo.Name
	if fo.Recv != nil {
		if !fo.Static {
			if t, ok := nm.Scope[fo.Recv.Name].(*check.TypeObj); ok {
				if m := t.Methods[fo.Name]; m != nil {
					return m
				}
			}
		}
		name = strings.ToLower(fo.Recv.Name) + "_" + fo.Name
	}
	if f, ok := nm.Scope[name].(*check.FuncObj); ok && f.Recv == nil {
		return f
	}
	return nil
}

// implType returns the implementation module's type of the std type
// obj, or nil.
func (b *builder) implType(obj *check.TypeObj) *check.TypeObj {
	if obj.Module == nil || !obj.Module.IsStd {
		return nil
	}
	nm := b.nativeMod(obj.Module.Path)
	if nm == nil {
		return nil
	}
	t, _ := nm.Scope[obj.Name].(*check.TypeObj)
	return t
}

// implNamed is t with every std type that has an implementation type
// (implType) replaced by it, in t's arguments too, for the names of
// lowered types: a std type and its implementation lower to the same
// type, so the types built from them must too.
func (b *builder) implNamed(t check.Type) check.Type {
	switch r := check.Resolve(check.Deep(t)).(type) {
	case *check.Named:
		obj := r.Obj
		if r.Obj.Kind == check.KStruct {
			if it := b.implType(r.Obj); it != nil {
				obj = it
			}
		}
		if obj == r.Obj && len(r.Args) == 0 {
			return r
		}
		args := make([]check.Type, len(r.Args))
		for i, a := range r.Args {
			args[i] = b.implNamed(a)
		}
		return &check.Named{Obj: obj, Args: args}
	case *check.Tuple:
		es := make([]check.Type, len(r.Elems))
		for i, e := range r.Elems {
			es[i] = b.implNamed(e)
		}
		return &check.Tuple{Elems: es}
	}
	return t
}

// isNeverResult reports whether t is Result[T, Never], which is lowered
// as T.
func isNeverResult(t check.Type) bool {
	_, et, ok := resultParts(t)
	return ok && check.IsNever(et)
}

// intrinsic lowers a call of a function of std/native/rt.xo.
func (s *fn) intrinsic(x *ast.CallExpr, ci *check.Call) *Value {
	b := s.b
	fo := ci.Fn
	nt := s.natOf(x)
	es := argExprs(fo.Params, x)
	for i := range es {
		if es[i] == nil {
			b.unsupported(x, "an intrinsic call with a missing argument")
			return s.undef(nt)
		}
	}
	if len(typeParams(fo)) > 0 {
		return s.genericIntrinsic(x, ci, es)
	}
	switch fo.Name {
	case "fs_root", "fs_mode":
		// the fields of an Fs (decision 0102)
		v := s.expr(es[0])
		if !isFs(v.Type) {
			b.unsupported(x, "`"+fo.Name+"` of a value that is not an Fs")
			return s.undef(nt)
		}
		i := 0
		if fo.Name == "fs_mode" {
			i = 1
		}
		return s.add(OpField, v.Type.Fields[i].Type, i, v)
	case "fs_new":
		t := b.fsType()
		return s.add(OpStruct, t, nil, s.valueAs(es[0], t.Fields[0].Type), s.valueAs(es[1], t.Fields[1].Type))
	case "dyn_text", "dyn_raw":
		v := s.expr(es[0])
		if !isDyn(v.Type) {
			b.unsupported(x, "`"+fo.Name+"` of a value that is not an interface value")
			return s.undef(nt)
		}
		if fo.Name == "dyn_text" {
			return s.add(OpField, nt, 0, v)
		}
		return s.add(OpField, nt, 1, v)
	}
	var args []*Value
	for i, p := range fo.Params {
		args = append(args, s.valueAs(es[i], b.typ(p.Type, x)))
	}
	if check.IsNever(fo.Result) {
		// fault_at: does not return
		s.rt(fo.Name, nil, args...)
		s.add(OpUnreachable, nil, nil)
		s.dead()
		return s.undef(nt)
	}
	return s.rt(fo.Name, nt, args...)
}

// locArgs are the arguments of the parameters of impl that follow those
// of the std function it implements: each is a `rt.Loc`, the `file:line`
// of the call (for faults located at the caller, as std/rt's FaultHere).
func (s *fn) locArgs(x *ast.CallExpr, implParams []*check.ParamInfo, n int) []*Value {
	var out []*Value
	for i := n; i < len(implParams); i++ {
		nm, ok := check.Resolve(implParams[i].Type).(*check.Named)
		if !ok || nm.Obj.Name != "Loc" || nm.Obj.Module == nil || nm.Obj.Module.Path != nativeRT {
			s.b.unsupported(x, "an implementation with more parameters than the std function")
			continue
		}
		out = append(out, s.add(OpConst, s.b.mod.Builtin(KStr), s.b.posLine(x)))
	}
	return out
}

// genericIntrinsic lowers the generic intrinsics.
func (s *fn) genericIntrinsic(x *ast.CallExpr, ci *check.Call, es []ast.Expr) *Value {
	b := s.b
	fo := ci.Fn
	nt := s.natOf(x)
	m := b.mod
	if v, ok := s.sqlIntrinsic(x, ci, es); ok {
		return v
	}
	if v, ok := s.jsonIntrinsic(x, ci, es); ok {
		return v
	}
	tps := typeParams(fo)
	targ := func(i int) check.Type {
		if ci.Subst == nil || i >= len(tps) {
			return nil
		}
		return check.Deep(s.apply(ci.Subst.Type(tps[i])))
	}
	switch fo.Name {
	case "launder1", "launder2":
		// the function itself (only its effects are accounted for here)
		return s.valueAs(es[0], nt)
	case "cast":
		// a std type and its implementation lower to one type
		v := s.valueAs(es[0], b.typ(targ(0), x))
		if v.Type != nt {
			b.unsupported(x, "the cast of `"+v.Type.Name+"` to `"+nt.Name+"`")
			return s.undef(nt)
		}
		return v
	case "status_of":
		ct := targ(0)
		v := s.valueAs(es[0], b.typ(ct, x))
		if md := s.methodOf(ct, "status"); md != nil {
			st := s.callMethod(md, ct, v, x)
			return s.variant(nt, "Some", []*Value{st}, x)
		}
		return s.variant(nt, "None", nil, x)
	case "any_of":
		ct := targ(0)
		t := b.typ(ct, x)
		v := s.valueAs(es[0], t)
		drop := s.add(OpFuncAddr, m.Builtin(KInt), b.dropList(t))
		return s.rt("any_new", nt, s.one(v), drop, s.add(OpConst, m.Builtin(KStr), t.Name))
	case "any_get":
		ct := targ(0)
		t := b.typ(ct, x)
		a := s.valueAs(es[0], b.typ(fo.Params[0].Type, x))
		l := s.rt("any_get", m.List(t), a, s.add(OpConst, m.Builtin(KStr), t.Name))
		return s.popOne(l, nt)
	}
	b.unsupported(x, "the intrinsic `"+fo.Name+"`")
	return s.undef(nt)
}
