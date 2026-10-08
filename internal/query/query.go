// Package query answers the semantic queries of `xo query` (core 10.3)
// over a check run: the type at a position, where a name is defined, who
// calls a function, which effects it performs and through which calls,
// the holes, and the exported surface of a module.
package query

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/format"
	"github.com/xo-lang/xo/internal/token"
)

// Session is a checked program to query.
type Session struct {
	Res  *check.Result
	Info *check.Info
}

// Load checks the modules under dirs.
func Load(dirs []string) (*Session, error) {
	res, err := check.Check(dirs)
	if err != nil {
		return nil, err
	}
	return &Session{Res: res, Info: res.Info()}, nil
}

// Pos is a source position in query results.
type Pos struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
}

func posOf(sp token.Span) Pos { return Pos{File: sp.File, Line: sp.Start.Line, Col: sp.Start.Col} }

func (p Pos) String() string { return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Col) }

// userModules returns the loaded modules that are not std or Go bindings.
func (s *Session) userModules() []*check.Module {
	var out []*check.Module
	for _, m := range s.Info.AllModules() {
		if !m.IsStd {
			out = append(out, m)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// type

// TypeResult is the answer of `xo query type file:line:col`.
type TypeResult struct {
	Pos     Pos    `json:"pos"`
	Expr    string `json:"expr"`
	Kind    string `json:"kind"` // "expr" or "binding"
	Type    string `json:"type"`
	Natural string `json:"natural,omitempty"` // type before an implicit conversion, when different
	Span    Pos    `json:"span"`
}

// Type returns the type of the innermost expression or binding pattern at
// line:col of file.
func (s *Session) Type(file string, line, col int) (*TypeResult, error) {
	f := s.findFile(file)
	if f == nil {
		return nil, fmt.Errorf("%s is not part of the checked modules", file)
	}
	target := offsetOf(f.Src, line, col)
	if target < 0 {
		return nil, fmt.Errorf("%s has no position %d:%d", file, line, col)
	}
	var best ast.Node
	bestLen := -1
	consider := func(n ast.Node) {
		sp := n.Span()
		if sp.Start.Offset <= target && target < sp.End.Offset || (sp.Start.Offset == target && sp.End.Offset == target) {
			if l := sp.End.Offset - sp.Start.Offset; bestLen < 0 || l < bestLen {
				best, bestLen = n, l
			}
		}
	}
	for _, d := range f.AST.Decls {
		check.Inspect(d, func(n ast.Node) bool {
			sp := n.Span()
			if target < sp.Start.Offset || target >= sp.End.Offset {
				// keep walking only nodes that contain the target
				return false
			}
			if e, ok := n.(ast.Expr); ok && s.Info.TypeOf(e) != nil {
				consider(e)
			}
			for _, pt := range patternsOf(n) {
				walkPattern(pt, func(q ast.Pattern) {
					if _, ok := q.(*ast.BindPat); ok && s.Info.PatType(q) != nil {
						consider(q)
					}
				})
			}
			return true
		})
	}
	if best == nil {
		return nil, fmt.Errorf("no typed expression at %s:%d:%d", file, line, col)
	}
	sp := best.Span()
	r := &TypeResult{Pos: Pos{File: f.Path, Line: line, Col: col}, Expr: string(f.Src[sp.Start.Offset:sp.End.Offset]), Span: posOf(sp)}
	switch x := best.(type) {
	case ast.Expr:
		r.Kind = "expr"
		r.Type = s.Info.TypeOf(x).String()
		if nt := s.Info.NaturalType(x); nt != nil && nt.String() != r.Type {
			r.Natural = nt.String()
		}
	case ast.Pattern:
		r.Kind = "binding"
		r.Type = s.Info.PatType(x).String()
	}
	return r, nil
}

func (s *Session) findFile(path string) *check.File {
	abs, _ := filepath.Abs(path)
	for _, m := range s.Info.AllModules() {
		for _, f := range m.Files {
			fa, _ := filepath.Abs(f.Path)
			if fa == abs || f.Path == path {
				return f
			}
		}
	}
	return nil
}

// offsetOf converts a 1 based line and byte column to an offset.
func offsetOf(src []byte, line, col int) int {
	l, start := 1, 0
	for i := 0; i < len(src) && l < line; i++ {
		if src[i] == '\n' {
			l++
			start = i + 1
		}
	}
	if l != line || col < 1 {
		return -1
	}
	off := start + col - 1
	if off > len(src) {
		return -1
	}
	if i := bytes.IndexByte(src[start:off], '\n'); i >= 0 {
		return -1 // past the end of the line
	}
	return off
}

// patternsOf returns the patterns held directly by n.
func patternsOf(n ast.Node) []ast.Pattern {
	switch x := n.(type) {
	case *ast.LetStmt:
		return []ast.Pattern{x.Pattern}
	case *ast.ForStmt:
		return []ast.Pattern{x.Pattern}
	case *ast.IfExpr:
		if x.Pattern != nil {
			return []ast.Pattern{x.Pattern}
		}
	case *ast.MatchExpr:
		var out []ast.Pattern
		for _, a := range x.Arms {
			out = append(out, a.Pattern)
		}
		return out
	}
	return nil
}

func walkPattern(p ast.Pattern, f func(ast.Pattern)) {
	if p == nil {
		return
	}
	f(p)
	switch x := p.(type) {
	case *ast.OrPat:
		for _, a := range x.Alts {
			walkPattern(a, f)
		}
	case *ast.TuplePat:
		for _, e := range x.Elems {
			walkPattern(e, f)
		}
	case *ast.ListPat:
		for _, e := range x.Elems {
			walkPattern(e, f)
		}
	case *ast.VariantPat:
		for _, fl := range x.Fields {
			walkPattern(fl.Pattern, f)
		}
	case *ast.StructPat:
		for _, fl := range x.Fields {
			walkPattern(fl.Pattern, f)
		}
	}
}

// ---------------------------------------------------------------------------
// Names

// Def is one declaration a name resolves to.
type Def struct {
	Kind      string `json:"kind"` // fn, method, struct, enum, interface, alias, newtype, const, field, variant
	Name      string `json:"name"`
	Module    string `json:"module"`
	Pos       Pos    `json:"pos"`
	Signature string `json:"signature"`
	Doc       string `json:"doc,omitempty"`

	fn  *check.FuncObj
	obj check.Object
}

// Def returns the declarations name refers to: `name`, `Type.member`,
// `module.name`, `module.Type.member`, where module is a module's local
// name (`http`) or import path (`std/http`, `example.com/app/users`).
func (s *Session) Def(name string) ([]Def, error) {
	defs := s.resolve(name)
	if len(defs) == 0 {
		return nil, fmt.Errorf("%q does not name a declaration in the checked modules", name)
	}
	return defs, nil
}

// Objects returns the checker objects (functions, methods, types, and
// constants) that name refers to, resolved as by Def. Fields and variants
// are not objects and are left out.
func (s *Session) Objects(name string) []check.Object {
	var out []check.Object
	for _, d := range s.resolve(name) {
		if d.obj != nil {
			out = append(out, d.obj)
		}
	}
	return out
}

func (s *Session) resolve(name string) []Def {
	var out []Def
	seen := map[string]bool{}
	add := func(ds []Def) {
		for _, d := range ds {
			k := d.Kind + "|" + d.Pos.String() + "|" + d.Name
			if !seen[k] {
				seen[k] = true
				out = append(out, d)
			}
		}
	}
	// module path qualified: std/http.server, example.com/a/b.f
	if i := strings.LastIndex(name, "/"); i >= 0 {
		if j := strings.IndexByte(name[i:], '.'); j > 0 {
			if m := s.ModuleByPath(name[:i+j]); m != nil {
				add(s.lookup(m, strings.Split(name[i+j+1:], ".")))
			}
		}
		return out
	}
	parts := strings.Split(name, ".")
	if len(parts) > 1 {
		for _, m := range s.Info.AllModules() {
			if m.Name == parts[0] {
				add(s.lookup(m, parts[1:]))
			}
		}
	}
	for _, m := range s.userModules() {
		add(s.lookup(m, parts))
	}
	if len(out) == 0 {
		add(s.lookup(s.Info.Prelude(), parts))
	}
	return out
}

// ModuleByPath finds a loaded module by import path or directory.
func (s *Session) ModuleByPath(path string) *check.Module {
	if m := s.Info.Module(path); m != nil {
		return m
	}
	abs, _ := filepath.Abs(path)
	for _, m := range s.Info.AllModules() {
		if m.IsStd {
			continue
		}
		if da, _ := filepath.Abs(m.Dir); da == abs || strings.HasSuffix(m.Path, "/"+path) {
			return m
		}
	}
	return nil
}

func (s *Session) lookup(m *check.Module, segs []string) []Def {
	if m == nil || len(segs) == 0 || len(segs) > 2 {
		return nil
	}
	obj := m.Scope[segs[0]]
	if len(segs) == 1 {
		switch o := obj.(type) {
		case *check.FuncObj:
			return []Def{s.fnDef(o)}
		case *check.TypeObj:
			return []Def{s.typeDef(o)}
		case *check.ConstObj:
			return []Def{s.constDef(o)}
		}
		return nil
	}
	t, ok := obj.(*check.TypeObj)
	if !ok {
		return nil
	}
	member := segs[1]
	if f := t.Methods[member]; f != nil {
		return []Def{s.fnDef(f)}
	}
	if f := t.Statics[member]; f != nil {
		return []Def{s.fnDef(f)}
	}
	for _, f := range t.IfaceMeths {
		if f.Name == member {
			return []Def{s.fnDef(f)}
		}
	}
	for _, f := range t.Fields {
		if f.Name == member {
			d := Def{Kind: "field", Name: t.Name + "." + f.Name, Module: m.Path, Pos: posOf(f.Span), Signature: f.Name + ": " + f.Type.String()}
			if f.Pub {
				d.Signature = "pub " + d.Signature
			}
			return []Def{d}
		}
	}
	for _, v := range t.Variants {
		if v.Name == member {
			sig := v.Name
			if !v.Unit {
				var fs []string
				for _, f := range v.Fields {
					if f.From {
						fs = append(fs, "from "+f.Type.String())
					} else {
						fs = append(fs, f.Name+": "+f.Type.String())
					}
				}
				sig += "(" + strings.Join(fs, ", ") + ")"
			}
			return []Def{{Kind: "variant", Name: t.Name + "." + v.Name, Module: m.Path, Pos: posOf(v.Span), Signature: sig}}
		}
	}
	return nil
}

func fileOf(m *check.Module, path string) *check.File {
	for _, f := range m.Files {
		if f.Path == path {
			return f
		}
	}
	return nil
}

func docText(d *ast.Doc) string {
	if d == nil {
		return ""
	}
	return d.Text
}

func (s *Session) fnDef(f *check.FuncObj) Def {
	d := Def{Kind: "fn", Name: f.DisplayName(), Module: f.Module.Path, Pos: posOf(f.Span), fn: f, obj: f}
	if f.Recv != nil {
		d.Kind = "method"
	}
	if f.Decl != nil {
		d.Pos = posOf(f.Decl.Name.Sp)
		d.Doc = docText(f.Decl.Doc)
		if file := fileOf(f.Module, f.Decl.Sp.File); file != nil {
			d.Signature = format.Decl(f.Decl, file.Src, true)
		}
	}
	return d
}

var typeKinds = map[check.TypeKind]string{check.KStruct: "struct", check.KEnum: "enum", check.KInterface: "interface", check.KAlias: "alias", check.KNewtype: "newtype"}

func (s *Session) typeDef(t *check.TypeObj) Def {
	d := Def{Kind: typeKinds[t.Kind], Name: t.Name, Module: t.Module.Path, Pos: posOf(t.Span), obj: t}
	if decl, ok := t.Decl().(ast.Decl); ok && decl != nil {
		if file := fileOf(t.Module, decl.Span().File); file != nil {
			d.Signature = format.Decl(decl, file.Src, true)
		}
		d.Doc = declDoc(decl)
	}
	return d
}

func (s *Session) constDef(c *check.ConstObj) Def {
	d := Def{Kind: "const", Name: c.Name, Module: c.Module.Path, obj: c}
	if c.Decl != nil {
		d.Pos = posOf(c.Decl.Name.Sp)
		d.Doc = docText(c.Decl.Doc)
		if file := fileOf(c.Module, c.Decl.Sp.File); file != nil {
			d.Signature = format.Decl(c.Decl, file.Src, true)
		}
	}
	return d
}

func declDoc(d ast.Decl) string {
	switch x := d.(type) {
	case *ast.FnDecl:
		return docText(x.Doc)
	case *ast.StructDecl:
		return docText(x.Doc)
	case *ast.EnumDecl:
		return docText(x.Doc)
	case *ast.InterfaceDecl:
		return docText(x.Doc)
	case *ast.TypeDecl:
		return docText(x.Doc)
	case *ast.ConstDecl:
		return docText(x.Doc)
	case *ast.TestDecl:
		return docText(x.Doc)
	}
	return ""
}

// funcTarget resolves name to exactly one function or method.
func (s *Session) funcTarget(name string) (*check.FuncObj, error) {
	var fns []*check.FuncObj
	for _, d := range s.resolve(name) {
		if d.fn != nil {
			fns = append(fns, d.fn)
		}
	}
	switch len(fns) {
	case 0:
		return nil, fmt.Errorf("%q does not name a function or method in the checked modules", name)
	case 1:
		return fns[0], nil
	}
	var names []string
	for _, f := range fns {
		names = append(names, f.Module.Path+"."+f.DisplayName())
	}
	return nil, fmt.Errorf("%q is ambiguous: %s; qualify it with the module", name, strings.Join(names, ", "))
}

// ---------------------------------------------------------------------------
// callers

// Caller is one reference to a function: a call, or a use as a value.
type Caller struct {
	Caller string `json:"caller"` // enclosing declaration: `fn`, `Type.method`, `test "name"`, `const NAME`
	Module string `json:"module"`
	Kind   string `json:"kind"` // "call" or "value"
	Pos    Pos    `json:"pos"`
}

// CallersResult is the answer of `xo query callers`.
type CallersResult struct {
	Target  string   `json:"target"`
	Callers []Caller `json:"callers"`
}

// Callers lists every call of the function name, and every use of it as a
// value, in the user modules.
func (s *Session) Callers(name string) (*CallersResult, error) {
	target, err := s.funcTarget(name)
	if err != nil {
		return nil, err
	}
	r := &CallersResult{Target: target.Module.Path + "." + target.DisplayName(), Callers: []Caller{}}
	for _, m := range s.userModules() {
		for _, f := range m.Files {
			for _, d := range f.AST.Decls {
				encl := s.enclosing(m, d)
				seen := map[ast.Expr]bool{}
				check.Inspect(d, func(n ast.Node) bool {
					switch x := n.(type) {
					case *ast.CallExpr:
						if ci := s.Info.CallOf(x); ci != nil && ci.Fn == target {
							r.Callers = append(r.Callers, Caller{Caller: encl, Module: m.Path, Kind: "call", Pos: posOf(x.Sp)})
							seen[x.Fun] = true
							if ta, ok := x.Fun.(*ast.TypeArgsExpr); ok {
								seen[ta.X] = true
							}
						}
					case *ast.Ident, *ast.SelectorExpr:
						e := x.(ast.Expr)
						if !seen[e] && s.Info.Ref(e) == check.Object(target) {
							r.Callers = append(r.Callers, Caller{Caller: encl, Module: m.Path, Kind: "value", Pos: posOf(e.Span())})
						}
					}
					return true
				})
			}
		}
	}
	return r, nil
}

func (s *Session) enclosing(m *check.Module, d ast.Decl) string {
	switch x := d.(type) {
	case *ast.FnDecl:
		if fo := s.Info.FuncOf(m, x); fo != nil {
			return fo.DisplayName()
		}
		return x.Name.Name
	case *ast.TestDecl:
		if x.Name != nil && len(x.Name.Parts) == 1 {
			return fmt.Sprintf("test %q", x.Name.Parts[0].Text)
		}
		return "test"
	case *ast.ConstDecl:
		return "const " + x.Name.Name
	case *ast.StructDecl:
		return "struct " + x.Name.Name
	}
	return "?"
}

// ---------------------------------------------------------------------------
// effects

// Step is one link of an effect's call chain.
type Step struct {
	Message string `json:"message"`
	Pos     Pos    `json:"pos"`
}

// Effect is one effect a function performs with its call chain.
type Effect struct {
	Effect string `json:"effect"`
	Param  bool   `json:"param,omitempty"`
	Chain  []Step `json:"chain"`
}

// EffectsResult is the answer of `xo query effects`.
type EffectsResult struct {
	Target string `json:"target"`
	// Declared is the `uses` set of a pub function or a function with a
	// clause; Inferred is set when the effects were inferred instead.
	Declared  []string `json:"declared"`
	Inferred  bool     `json:"inferred"`
	Performed []Effect `json:"performed"`
}

// Effects returns the effects the function name performs, each with the
// call chain from its body to the place that performs it.
func (s *Session) Effects(name string) (*EffectsResult, error) {
	target, err := s.funcTarget(name)
	if err != nil {
		return nil, err
	}
	r := &EffectsResult{Target: target.Module.Path + "." + target.DisplayName(), Declared: []string{}, Performed: []Effect{}}
	if target.Declared == nil {
		r.Inferred = true
	} else if str := target.Declared.String(); str != "" {
		r.Declared = strings.Split(str, ", ")
	}
	for _, e := range s.Info.Performed(target) {
		ef := Effect{Effect: e.Effect, Param: e.Param, Chain: []Step{}}
		for _, st := range e.Chain {
			ef.Chain = append(ef.Chain, Step{Message: st.Message, Pos: posOf(st.Span)})
		}
		r.Performed = append(r.Performed, ef)
	}
	return r, nil
}

// ---------------------------------------------------------------------------
// holes

// Holes returns the hole reports (XO0900) of the checked modules.
func (s *Session) Holes() []diag.Diagnostic {
	out := []diag.Diagnostic{}
	for _, d := range s.Res.Diagnostics {
		if d.Code == string(diag.Hole) {
			out = append(out, d)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// api

// APIResult is the exported surface of a module.
type APIResult struct {
	Module string `json:"module"`
	Items  []Def  `json:"items"`
}

// API returns the pub declarations of the module with the given import
// path or directory, outside test files, in file and source order. Struct
// signatures list only pub fields.
func (s *Session) API(path string) (*APIResult, error) {
	m := s.ModuleByPath(path)
	if m == nil {
		return nil, fmt.Errorf("no module %q among the checked modules", path)
	}
	return &APIResult{Module: m.Path, Items: s.Surface(m, false)}, nil
}

// Surface lists the declarations of m: pub ones only unless all is set.
// Test files and test blocks are left out.
func (s *Session) Surface(m *check.Module, all bool) []Def {
	out := []Def{}
	files := append([]*check.File(nil), m.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, f := range files {
		if f.IsTest {
			continue
		}
		for _, d := range f.AST.Decls {
			def, ok := s.surfaceDef(m, f, d, all)
			if ok {
				out = append(out, def)
			}
		}
	}
	return out
}

func (s *Session) surfaceDef(m *check.Module, f *check.File, d ast.Decl, all bool) (Def, bool) {
	switch x := d.(type) {
	case *ast.FnDecl:
		if !x.Pub && !all {
			return Def{}, false
		}
		if fo := s.Info.FuncOf(m, x); fo != nil {
			return s.fnDef(fo), true
		}
		return Def{Kind: "fn", Name: x.Name.Name, Module: m.Path, Pos: posOf(x.Name.Sp), Signature: format.Decl(x, f.Src, true), Doc: docText(x.Doc)}, true
	case *ast.ConstDecl:
		if !x.Pub && !all {
			return Def{}, false
		}
		return Def{Kind: "const", Name: x.Name.Name, Module: m.Path, Pos: posOf(x.Name.Sp), Signature: format.Decl(x, f.Src, true), Doc: docText(x.Doc)}, true
	case *ast.StructDecl:
		if !x.Pub && !all {
			return Def{}, false
		}
		c := *x
		if !all {
			c.Fields = nil
			for _, fl := range x.Fields {
				if fl.Pub {
					c.Fields = append(c.Fields, fl)
				}
			}
			c.Invariant = nil
		}
		return Def{Kind: "struct", Name: x.Name.Name, Module: m.Path, Pos: posOf(x.Name.Sp), Signature: format.Decl(&c, f.Src, true), Doc: docText(x.Doc)}, true
	case *ast.EnumDecl:
		if !x.Pub && !all {
			return Def{}, false
		}
		return Def{Kind: "enum", Name: x.Name.Name, Module: m.Path, Pos: posOf(x.Name.Sp), Signature: format.Decl(x, f.Src, true), Doc: docText(x.Doc)}, true
	case *ast.InterfaceDecl:
		if !x.Pub && !all {
			return Def{}, false
		}
		return Def{Kind: "interface", Name: x.Name.Name, Module: m.Path, Pos: posOf(x.Name.Sp), Signature: format.Decl(x, f.Src, true), Doc: docText(x.Doc)}, true
	case *ast.TypeDecl:
		if !x.Pub && !all {
			return Def{}, false
		}
		kind := "alias"
		if x.Newtype {
			kind = "newtype"
		}
		return Def{Kind: kind, Name: x.Name.Name, Module: m.Path, Pos: posOf(x.Name.Sp), Signature: format.Decl(x, f.Src, true), Doc: docText(x.Doc)}, true
	}
	return Def{}, false
}
