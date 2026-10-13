package ssa

import (
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Raw memory intrinsics (decision 0207): rt.mem_load, mem_store,
// mem_copy, mem_move, mem_set, mem_cmp, layout, rs_set_len, and the
// generic obj_addr and obj_at, for the runtime modules only
// (std/native/llrt). The emitters lower them inline; the builder checks
// the constant operands and folds rt.layout from the table below, which
// selfhost/ssa mirrors (layout_value in call.xo).

// Layout is the table of rt.layout: the offsets and sizes of the C
// runtime's objects (std/llrt xo_rt.c: xo_rstr {rc, cap, data}, xo_rlist
// {rc, len, cap} then the elements) on the 64 bit native targets, and of
// the wasm runtime's (selfhost/wasm rt.xo: 32 bit counts and addresses).
// The keys are those of std/native/rt.xo's `layout`.
var Layout = []struct {
	Name         string
	Native, Wasm int64
}{
	{"string data", 16, 8},
	{"string capacity", 8, 4},
	{"list elements", 24, 16},
	{"list length", 8, 4},
	{"list capacity", 16, 8},
	{"count", 0, 0},
	{"address bytes", 8, 4},
	{"count bytes", 8, 4},
}

// rawIntrinsic checks and folds the raw memory intrinsics: (nil, false)
// for any other intrinsic.
func (s *fn) rawIntrinsic(x *ast.CallExpr, name string, args []*Value) (*Value, bool) {
	b := s.b
	switch name {
	case "layout":
		k, ok := constWord(args[0])
		if !ok || k < 0 || k >= int64(len(Layout)) {
			b.unsupported(x, "`rt.layout` of a key that is not a constant of the layout table")
			return s.undef(b.mod.Builtin(KInt)), true
		}
		return s.cint(Layout[k].Native), true
	case "mem_load", "mem_store":
		w, ok := constWord(args[len(args)-1])
		if !ok || (w != 1 && w != 2 && w != 4 && w != 8) {
			b.unsupported(x, "`rt."+name+"` of a width that is not a constant 1, 2, 4, or 8")
			return s.undef(b.mod.Builtin(KInt)), true
		}
	}
	return nil, false
}

// rawGeneric lowers rt.obj_addr[T] and rt.obj_at[T] (decision 0207):
// (nil, false) for another generic intrinsic.
func (s *fn) rawGeneric(x *ast.CallExpr, name string, t *Type, es []ast.Expr) (*Value, bool) {
	b := s.b
	intT := b.mod.Builtin(KInt)
	switch name {
	case "obj_addr":
		if t.Kind != KStr && t.Kind != KList && t.Kind != KHandle {
			b.unsupported(x, "`rt.obj_addr` of a `"+t.Name+"` (a Str, List, or handle)")
			return s.undef(intT), true
		}
		return s.rt("obj_addr", intT, s.valueAs(es[0], t)), true
	case "obj_at":
		if t.Kind != KList && t.Kind != KHandle {
			b.unsupported(x, "`rt.obj_at` of a `"+t.Name+"` (a List or handle)")
			return s.undef(t), true
		}
		return s.rt("obj_at", t, s.valueAs(es[0], intT)), true
	}
	return nil, false
}

// CABIModule is the runtime module whose functions are C ABI exports
// (decision 0207): every top level function of std/native/llrt/cabi.xo,
// and of a module cabi_<feature>.xo beside it (exports only the C of a
// feature calls, imported by that feature's implementation module), is
// called by the C runtime as xo_x_<name>, with Int and Float parameters
// (an address is an Int) and an Int, Float, Str, or no result (a Str is
// returned as its string object, its length stored through a pointer the
// C caller adds after the parameters, as the C runtime returns strings).
// Only std's implementation modules can load them (decision 0074): the
// marker is the module, not syntax.
const CABIModule = "native/llrt/cabi"

// IsCABIModule reports whether the module path is a marker module of C
// ABI exports (CABIModule or CABIModule_<feature>).
func IsCABIModule(path string) bool {
	return path == CABIModule || strings.HasPrefix(path, CABIModule+"_")
}

// ExportPrefix is the C symbol prefix of an export.
const ExportPrefix = "xo_x_"

// declareExports declares every function of the marker modules, in the
// order of their paths and then of their names, as roots of the build:
// the C runtime calls them. cabi's always (the native prelude imports
// it); a cabi_<feature>'s when the program builds code of a module that
// imports it (std/testing loads std/http into every program, so loading
// is not the test). A parameter or result outside the C ABI's shapes is
// unsupported.
func (b *builder) declareExports() {
	var mods []*check.Module
	for _, m := range b.info.AllModules() {
		if IsCABIModule(m.Path) {
			mods = append(mods, m)
		}
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].Path < mods[j].Path })
	for _, mod := range mods {
		if mod.Path == CABIModule || b.importerBuilt(mod) {
			b.declareExportsOf(mod)
		}
	}
}

// importerBuilt reports whether the program builds code of a module that
// imports mod (a feature's marker module: its C runs only then).
func (b *builder) importerBuilt(mod *check.Module) bool {
	for _, m := range b.info.AllModules() {
		if !b.declMods[m.Path] {
			continue
		}
		for _, f := range m.Files {
			for _, imp := range f.Imports {
				if imp.Module == mod {
					return true
				}
			}
		}
	}
	return false
}

func (b *builder) declareExportsOf(mod *check.Module) {
	var names []string
	for n, o := range mod.Scope {
		if fo, ok := o.(*check.FuncObj); ok && fo.Recv == nil && fo.Decl != nil {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		fo := mod.Scope[n].(*check.FuncObj)
		if len(typeParams(fo)) > 0 {
			b.unsupported(fo.Decl, "a generic C ABI export")
			continue
		}
		f := b.declare(fo, nil, fo.Decl)
		f.Entry = true
		f.Export = ExportPrefix + n
		for _, p := range f.Params() {
			if k := p.Type.Kind; k != KInt && k != KFloat {
				b.unsupported(fo.Decl, "a C ABI export with a `"+p.Type.Name+"` parameter")
			}
		}
		if len(f.Results) != 1 || len(f.VarParams) > 0 {
			b.unsupported(fo.Decl, "a C ABI export with `var` parameters")
		} else if k := f.Results[0].Kind; k != KInt && k != KFloat && k != KStr && k != KUnit {
			b.unsupported(fo.Decl, "a C ABI export with a `"+f.Results[0].Name+"` result")
		}
	}
}
