package lsp

import (
	"reflect"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/token"
)

// identSite is one identifier in a file with the nodes above it.
type identSite struct {
	id    *ast.Ident
	stack []ast.Node // ancestors, innermost last
	field string     // field of the parent that holds id
	index int        // index within that field when it is a slice, else -1
}

var (
	identType = reflect.TypeOf((*ast.Ident)(nil))
	nodeType  = reflect.TypeOf((*ast.Node)(nil)).Elem()
	spanType  = reflect.TypeOf(token.Span{})
	docType   = reflect.TypeOf((*ast.Doc)(nil))
)

// idents returns every identifier of f in source order. Unlike
// check.Inspect it also visits declaration names, types, and patterns.
func idents(f *ast.File) []identSite {
	var out []identSite
	var walk func(v reflect.Value, stack []ast.Node, field string, index int)
	walk = func(v reflect.Value, stack []ast.Node, field string, index int) {
		switch v.Kind() {
		case reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem(), stack, field, index)
			}
		case reflect.Pointer:
			if v.IsNil() || v.Type() == docType {
				return
			}
			if v.Type() == identType {
				id := v.Interface().(*ast.Ident)
				out = append(out, identSite{id: id, stack: stack, field: field, index: index})
				return
			}
			if v.Type().Implements(nodeType) {
				stack = append(stack[:len(stack):len(stack)], v.Interface().(ast.Node))
			}
			walk(v.Elem(), stack, field, index)
		case reflect.Struct:
			if v.Type() == spanType {
				return
			}
			t := v.Type()
			for i := 0; i < t.NumField(); i++ {
				sf := t.Field(i)
				if !sf.IsExported() || sf.Name == "Base" {
					continue
				}
				walk(v.Field(i), stack, sf.Name, -1)
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i), stack, field, i)
			}
		}
	}
	walk(reflect.ValueOf(f), nil, "", -1)
	return out
}

// target is what an identifier names.
type target struct {
	kind  string // semantic token type, "" when unknown
	mods  int    // semantic token modifier bits
	obj   any    // check.Object, *check.FieldObj, *check.VariantObj, *check.Module
	def   token.Span
	label string // hover text when obj has no declaration to print
}

const (
	modDeclaration = 1 << iota
	modReadonly
	modDefaultLibrary
)

func (s *snapshot) parent(site identSite, up int) ast.Node {
	if i := len(site.stack) - 1 - up; i >= 0 {
		return site.stack[i]
	}
	return nil
}

// resolve says what the identifier at site of file f names.
func (s *snapshot) resolve(f *check.File, site identSite) target {
	info := s.info
	id := site.id
	p := s.parent(site, 0)
	g := s.parent(site, 1)
	if v := info.VarOf(id); v != nil {
		return varTarget(v, id)
	}
	switch x := p.(type) {
	case *ast.FnDecl:
		if site.field == "Name" {
			var fo *check.FuncObj
			if fo = info.FuncOf(f.Module, x); fo == nil {
				if iface, ok := g.(*ast.InterfaceDecl); ok {
					fo = ifaceMethod(f.Module, iface, id.Name)
				}
			}
			t := target{kind: "function", mods: modDeclaration, def: id.Sp}
			if _, ok := g.(*ast.InterfaceDecl); ok || x.Recv != nil {
				t.kind = "method"
			}
			if fo != nil {
				t.obj = fo
			}
			return t
		}
	case *ast.Receiver:
		if fd, ok := g.(*ast.FnDecl); ok {
			if v := info.RecvVar(info.FuncOf(f.Module, fd)); v != nil {
				return varTarget(v, id)
			}
		}
		return target{kind: "parameter", mods: modDeclaration, def: id.Sp}
	case *ast.Param:
		if site.field == "Name" {
			var vs []*check.VarObj
			switch y := g.(type) {
			case *ast.FnDecl:
				if fo := info.FuncOf(f.Module, y); fo != nil {
					vs = info.ParamVars(fo)
				}
			case *ast.FnLit:
				vs = info.ClosureParams(y)
			}
			for _, v := range vs {
				if v != nil && v.Name == id.Name {
					return varTarget(v, id)
				}
			}
			return target{kind: "parameter", mods: modDeclaration, def: id.Sp}
		}
	case *ast.StructDecl, *ast.EnumDecl, *ast.InterfaceDecl, *ast.TypeDecl:
		switch site.field {
		case "Name":
			if to, ok := f.Module.Scope[id.Name].(*check.TypeObj); ok {
				t := typeTarget(to)
				t.mods |= modDeclaration
				t.def = id.Sp
				return t
			}
			return target{kind: "type", mods: modDeclaration, def: id.Sp}
		case "Derive":
			return s.typeName(f, []*ast.Ident{id}, "interface")
		}
	case *ast.TypeParam:
		if site.field == "Name" {
			return target{kind: "typeParameter", mods: modDeclaration, def: id.Sp, label: id.Name}
		}
	case *ast.Field:
		if sd, ok := g.(*ast.StructDecl); ok {
			if to, ok := f.Module.Scope[sd.Name.Name].(*check.TypeObj); ok {
				if fo := fieldOf(to, id.Name); fo != nil {
					return target{kind: "property", mods: modDeclaration, obj: fo, def: id.Sp}
				}
			}
		}
		return target{kind: "property", mods: modDeclaration, def: id.Sp}
	case *ast.Variant:
		if ed, ok := g.(*ast.EnumDecl); ok {
			if to, ok := f.Module.Scope[ed.Name.Name].(*check.TypeObj); ok {
				if vo := variantOf(to, id.Name); vo != nil {
					return target{kind: "enumMember", mods: modDeclaration, obj: vo, def: id.Sp}
				}
			}
		}
		return target{kind: "enumMember", mods: modDeclaration, def: id.Sp}
	case *ast.VariantField:
		return target{kind: "property", mods: modDeclaration, def: id.Sp}
	case *ast.ConstDecl:
		if site.field == "Name" {
			t := target{kind: "variable", mods: modDeclaration | modReadonly, def: id.Sp}
			if co, ok := f.Module.Scope[id.Name].(*check.ConstObj); ok && co.Decl == x {
				t.obj = co
			}
			return t
		}
	case *ast.NamedType:
		if site.index == len(x.Path)-1 {
			return s.typeName(f, x.Path, "")
		}
		return s.importName(f, id)
	case *ast.UseDecl:
		return target{kind: "namespace", mods: modDeclaration, def: id.Sp}
	case *ast.SelectorExpr:
		if site.field == "Sel" {
			return s.selector(f, x, g)
		}
	case *ast.FieldInit:
		if site.field == "Name" {
			if e, ok := g.(ast.Expr); ok {
				if to := namedObj(info.TypeOf(e)); to != nil {
					if fo := fieldOf(to, id.Name); fo != nil {
						return target{kind: "property", obj: fo, def: fo.Span}
					}
				}
			}
			return target{kind: "property"}
		}
	case *ast.Arg:
		if site.field == "Name" {
			return target{kind: "parameter"}
		}
	case *ast.PatField:
		if site.field == "Name" {
			return target{kind: "property"}
		}
	case *ast.VariantPat:
		if site.index == len(x.Path)-1 {
			if vo := info.PatVariant(x); vo != nil {
				return variantTarget(vo)
			}
			if to := info.PatNewtype(x); to != nil {
				return typeTarget(to)
			}
		}
		return s.typeOrImport(f, x.Path, site.index)
	case *ast.StructPat:
		if site.index == len(x.Path)-1 {
			return s.typeName(f, x.Path, "")
		}
		return s.typeOrImport(f, x.Path, site.index)
	case *ast.Effect, *ast.ForStmt, *ast.WhileStmt:
		if site.field == "Label" || site.field == "Name" || site.field == "Sub" {
			return target{}
		}
	}
	// an identifier in expression position
	if o := info.Ref(id); o != nil {
		return objTarget(o)
	}
	if vo := info.Ctor(id); vo != nil {
		return variantTarget(vo)
	}
	if call, ok := p.(*ast.CallExpr); ok && call.Fun == ast.Expr(id) {
		if ci := info.CallOf(call); ci != nil && ci.Fn != nil {
			return objTarget(ci.Fn)
		}
		if to := info.TypeCall(call); to != nil {
			return typeTarget(to)
		}
	}
	if sel, ok := p.(*ast.SelectorExpr); ok && sel.X == ast.Expr(id) {
		if t := s.importName(f, id); t.kind != "" {
			return t
		}
	}
	if isUpper(id.Name) {
		if t := s.typeName(f, []*ast.Ident{id}, ""); t.kind != "" {
			return t
		}
	}
	if co, ok := f.Module.Scope[id.Name].(*check.ConstObj); ok {
		return objTarget(co)
	}
	return target{}
}

// selector resolves the Sel of x; g is the parent of x.
func (s *snapshot) selector(f *check.File, x *ast.SelectorExpr, g ast.Node) target {
	info := s.info
	if o := info.Ref(x); o != nil {
		return objTarget(o)
	}
	if vo := info.Ctor(x); vo != nil {
		return variantTarget(vo)
	}
	if call, ok := g.(*ast.CallExpr); ok && call.Fun == ast.Expr(x) {
		if ci := info.CallOf(call); ci != nil && ci.Fn != nil {
			return objTarget(ci.Fn)
		}
		if to := info.TypeCall(call); to != nil {
			return typeTarget(to)
		}
		return target{kind: "method"}
	}
	if xid, ok := x.X.(*ast.Ident); ok && info.VarOf(xid) == nil {
		if im := f.Imports[xid.Name]; im != nil && im.Module != nil {
			if o := im.Module.Scope[x.Sel.Name]; o != nil {
				return objTarget(o)
			}
		}
	}
	if to := namedObj(info.TypeOf(x.X)); to != nil {
		if fo := fieldOf(to, x.Sel.Name); fo != nil {
			return target{kind: "property", obj: fo, def: fo.Span}
		}
		if m := to.Methods[x.Sel.Name]; m != nil {
			return objTarget(m)
		}
	}
	return target{kind: "property"}
}

// typeName resolves a type path (`User`, `http.Request`). kind overrides
// the token type when the name is not found.
func (s *snapshot) typeName(f *check.File, path []*ast.Ident, kind string) target {
	var o check.Object
	name := path[len(path)-1].Name
	switch len(path) {
	case 1:
		if o = f.Module.Scope[name]; o == nil && s.info.Prelude() != nil {
			o = s.info.Prelude().Scope[name]
		}
	case 2:
		if im := f.Imports[path[0].Name]; im != nil && im.Module != nil {
			o = im.Module.Scope[name]
		}
	}
	if to, ok := o.(*check.TypeObj); ok {
		return typeTarget(to)
	}
	if kind == "" && isUpper(name) {
		kind = "type"
	}
	return target{kind: kind}
}

func (s *snapshot) typeOrImport(f *check.File, path []*ast.Ident, i int) target {
	if i == 0 {
		if t := s.importName(f, path[0]); t.kind != "" {
			return t
		}
	}
	return s.typeName(f, path[:i+1], "")
}

func (s *snapshot) importName(f *check.File, id *ast.Ident) target {
	im := f.Imports[id.Name]
	if im == nil {
		return target{}
	}
	t := target{kind: "namespace", label: "use " + im.Decl.Path}
	if im.Module != nil {
		t.obj = im.Module
		if im.Module.IsStd {
			t.mods |= modDefaultLibrary
		}
	}
	if im.Decl != nil {
		t.def = im.Decl.Sp
	}
	return t
}

func varTarget(v *check.VarObj, id *ast.Ident) target {
	t := target{kind: "variable", obj: v, def: v.Span}
	if v.Kind == check.VParam || v.Kind == check.VRecv {
		t.kind = "parameter"
	}
	if v.Span.Start.Offset == id.Sp.Start.Offset && v.Span.File == id.Sp.File {
		t.mods |= modDeclaration
	}
	return t
}

func objTarget(o check.Object) target {
	switch x := o.(type) {
	case *check.FuncObj:
		t := target{kind: "function", obj: x, def: x.Span}
		if x.Recv != nil || x.Iface {
			t.kind = "method"
		}
		if x.Decl != nil && x.Decl.Name != nil {
			t.def = x.Decl.Name.Sp
		}
		if x.Module != nil && x.Module.IsStd {
			t.mods |= modDefaultLibrary
		}
		return t
	case *check.ConstObj:
		t := target{kind: "variable", mods: modReadonly, obj: x}
		if x.Decl != nil {
			t.def = x.Decl.Name.Sp
		}
		if x.Module != nil && x.Module.IsStd {
			t.mods |= modDefaultLibrary
		}
		return t
	case *check.TypeObj:
		return typeTarget(x)
	case *check.VarObj:
		return varTarget(x, &ast.Ident{})
	}
	return target{}
}

func typeTarget(to *check.TypeObj) target {
	t := target{kind: "type", obj: to, def: to.Span}
	switch to.Kind {
	case check.KStruct:
		t.kind = "struct"
	case check.KEnum:
		t.kind = "enum"
	case check.KInterface:
		t.kind = "interface"
	}
	if to.Module != nil && to.Module.IsStd {
		t.mods |= modDefaultLibrary
	}
	return t
}

func variantTarget(vo *check.VariantObj) target {
	return target{kind: "enumMember", obj: vo, def: vo.Span}
}

func namedObj(t check.Type) *check.TypeObj {
	if t == nil {
		return nil
	}
	if n, ok := check.Deep(t).(*check.Named); ok {
		return n.Obj
	}
	return nil
}

func fieldOf(to *check.TypeObj, name string) *check.FieldObj {
	for _, f := range to.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func variantOf(to *check.TypeObj, name string) *check.VariantObj {
	for _, v := range to.Variants {
		if v.Name == name {
			return v
		}
	}
	return nil
}

func ifaceMethod(m *check.Module, d *ast.InterfaceDecl, name string) *check.FuncObj {
	if to, ok := m.Scope[d.Name.Name].(*check.TypeObj); ok {
		for _, fo := range to.IfaceMeths {
			if fo.Name == name {
				return fo
			}
		}
	}
	return nil
}

func isUpper(name string) bool { return name != "" && name[0] >= 'A' && name[0] <= 'Z' }

// declOf finds the declaration of a type in its module's files.
func declOf(to *check.TypeObj) (*check.File, ast.Decl) {
	if to.Module == nil {
		return nil, nil
	}
	for _, f := range to.Module.Files {
		for _, d := range f.AST.Decls {
			var name *ast.Ident
			switch x := d.(type) {
			case *ast.StructDecl:
				name = x.Name
			case *ast.EnumDecl:
				name = x.Name
			case *ast.InterfaceDecl:
				name = x.Name
			case *ast.TypeDecl:
				name = x.Name
			}
			if name != nil && name.Name == to.Name {
				return f, d
			}
		}
	}
	return nil, nil
}

// sameObj reports whether two targets name the same entity.
func sameObj(a, b target) bool {
	if a.obj != nil || b.obj != nil {
		return a.obj == b.obj
	}
	return a.def != (token.Span{}) && a.def == b.def
}

func trimDoc(s string) string { return strings.TrimSpace(s) }
