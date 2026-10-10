// Package defhash computes per definition content hashes (core 4.3,
// decision 0125): a stable hash of every top level definition of a
// module (function, method, type, constant), over its syntax tree with
// positions, comments, and doc comments dropped, local names erased
// (numbered by first binding), and the definitions of the same module it
// references replaced by their own hashes. Renaming a local variable
// keeps a definition's hash; changing what it computes, or what a
// definition it uses computes, changes it.
//
// The hashes are syntactic: they need no type checking and no
// dependencies, so `xo publish --check` can hash an old tag as extracted.
// What that leaves out is in decision 0125.
package defhash

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/parser"
	"github.com/xo-lang/xo/internal/token"
)

// Def is one top level definition and its hashes.
type Def struct {
	ID      string `json:"id"`      // <package path>.<name>
	Package string `json:"package"` // import path of its package
	Name    string `json:"name"`    // name, or Type.method
	Kind    string `json:"kind"`    // fn, method, struct, enum, interface, type, const
	Pub     bool   `json:"pub"`
	// Hash covers the definition and, through their hashes, the
	// definitions of the module it references.
	Hash string `json:"hash"`
	// Own covers the definition alone: references by name.
	Own string `json:"own"`
	// Refs are the definitions of the module it references, by ID.
	Refs []string `json:"refs,omitempty"`
	File string   `json:"file"`
	Line int      `json:"line"`

	key  string // package directory relative to the module, ".", name
	refs []string
}

// prefix names the hash scheme; a change to the normalization takes a
// new one.
const prefix = "d1:"

type fileInfo struct {
	imports map[string]string // local name -> import path
}

type pkgInfo struct {
	path string
	rel  string            // directory relative to the module root, "." for the root
	defs map[string]string // name or Type.method -> key
}

type declRef struct {
	decl ast.Decl
	file *fileInfo
	path string
	line int
}

type defInfo struct {
	def   *Def
	pkg   *pkgInfo
	decls []declRef
}

// Module hashes every top level definition of the module in dir (module
// path modPath): the packages under dir, outside test files, vendor/,
// testdata/, dot directories, and nested modules. Definitions are sorted
// by ID.
func Module(dir, modPath string) ([]Def, error) {
	pkgs := map[string]*pkgInfo{} // by import path
	defs := map[string]*defInfo{} // by key
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir {
				n := d.Name()
				if n == "vendor" || n == "testdata" || strings.HasPrefix(n, ".") {
					return filepath.SkipDir
				}
				if _, err := os.Stat(filepath.Join(p, "xo.mod")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(p, ".xo") || strings.HasSuffix(p, "_test.xo") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		f, _ := parser.ParseFile(p, src, 0)
		if f == nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, filepath.Dir(p))
		rel = filepath.ToSlash(rel)
		pkgPath := modPath
		if rel != "." {
			pkgPath += "/" + rel
		}
		pkg := pkgs[pkgPath]
		if pkg == nil {
			pkg = &pkgInfo{path: pkgPath, rel: rel, defs: map[string]string{}}
			pkgs[pkgPath] = pkg
		}
		fi := &fileInfo{imports: map[string]string{}}
		for _, u := range f.Uses {
			if u.C {
				continue
			}
			name := localName(u.Path)
			if u.Alias != nil {
				name = u.Alias.Name
			}
			path := u.Path
			if u.Go {
				path = "go:" + path
			}
			fi.imports[name] = path
		}
		for _, decl := range f.Decls {
			name, kind, pub, line := describe(decl)
			if name == "" {
				continue
			}
			key := rel + "." + name
			pkg.defs[name] = key
			di := defs[key]
			if di == nil {
				di = &defInfo{pkg: pkg, def: &Def{ID: pkgPath + "." + name, Package: pkgPath, Name: name, Kind: kind, Pub: pub, File: p, Line: line, key: key}}
				defs[key] = di
			}
			di.decls = append(di.decls, declRef{decl: decl, file: fi, path: p, line: line})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	h := &hasher{pkgs: pkgs, defs: defs, deep: map[string]string{}}
	h.run()
	var out []Def
	for _, di := range defs {
		d := *di.def
		for _, r := range d.refs {
			d.Refs = append(d.Refs, defs[r].def.ID)
		}
		sort.Strings(d.Refs)
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// localName is the name an import is known by without `as`: its last
// element without a /vN major suffix, '-' read as '_'.
func localName(path string) string {
	if mod.IsRemote(path) {
		path, _ = mod.MajorSuffix(path)
	}
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		path = path[i+1:]
	}
	return strings.ReplaceAll(path, "-", "_")
}

// describe names a top level declaration; "" for tests and bad ones.
func describe(d ast.Decl) (name, kind string, pub bool, line int) {
	line = d.Span().Start.Line
	switch d := d.(type) {
	case *ast.FnDecl:
		if d.Name == nil {
			return
		}
		if d.Recv != nil {
			nt, ok := d.Recv.Type.(*ast.NamedType)
			if !ok || len(nt.Path) == 0 {
				return
			}
			return nt.Path[len(nt.Path)-1].Name + "." + d.Name.Name, "method", d.Pub, line
		}
		return d.Name.Name, "fn", d.Pub, line
	case *ast.StructDecl:
		return identName(d.Name), "struct", d.Pub, line
	case *ast.EnumDecl:
		return identName(d.Name), "enum", d.Pub, line
	case *ast.InterfaceDecl:
		return identName(d.Name), "interface", d.Pub, line
	case *ast.TypeDecl:
		return identName(d.Name), "type", d.Pub, line
	case *ast.ConstDecl:
		return identName(d.Name), "const", d.Pub, line
	}
	return
}

func identName(id *ast.Ident) string {
	if id == nil {
		return ""
	}
	return id.Name
}

// hasher computes the hashes of every definition: first each one's own
// encoding and the definitions it references, then the strongly
// connected components of the reference graph (mutual recursion), whose
// members are hashed together, callees first.
type hasher struct {
	pkgs map[string]*pkgInfo
	defs map[string]*defInfo
	deep map[string]string // key -> deep hash
	scc  map[string]int    // key -> component number
}

func sum(parts ...[]byte) string {
	h := sha256.New()
	for _, p := range parts {
		fmt.Fprintf(h, "%d:", len(p))
		h.Write(p)
	}
	return prefix + base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// encode is the canonical encoding of a definition (all its
// declarations, for one repeated in platform files); tok gives the token
// of a referenced definition.
func (h *hasher) encode(di *defInfo, tok func(key string) string) ([]byte, []string) {
	var encs [][]byte
	refs := map[string]bool{}
	for _, d := range di.decls {
		c := &ctx{h: h, pkg: di.pkg, file: d.file, self: di.def.key, tok: tok, locals: map[string]string{}, refs: refs, root: d.decl}
		c.enc(reflect.ValueOf(d.decl))
		encs = append(encs, c.buf.Bytes())
	}
	sort.Slice(encs, func(i, j int) bool { return bytes.Compare(encs[i], encs[j]) < 0 })
	var out []byte
	for _, e := range encs {
		out = append(out, e...)
		out = append(out, '\n')
	}
	var rs []string
	for r := range refs {
		if r != di.def.key {
			rs = append(rs, r)
		}
	}
	sort.Strings(rs)
	return out, rs
}

func (h *hasher) run() {
	// own hashes and the graph
	for _, di := range h.defs {
		enc, refs := h.encode(di, func(key string) string { return "@" + key })
		di.def.Own = sum([]byte(di.def.Kind), enc)
		di.def.refs = refs
	}
	// Tarjan's strongly connected components, in a fixed order; they
	// come out callees first
	keys := make([]string, 0, len(h.defs))
	for k := range h.defs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	index := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	h.scc = map[string]int{}
	n, comp := 0, 0
	var visit func(k string)
	visit = func(k string) {
		index[k], low[k] = n, n
		n++
		stack = append(stack, k)
		onStack[k] = true
		for _, r := range h.defs[k].def.refs {
			if _, seen := index[r]; !seen {
				visit(r)
				low[k] = min(low[k], low[r])
			} else if onStack[r] {
				low[k] = min(low[k], index[r])
			}
		}
		if low[k] == index[k] {
			var members []string
			for {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[top] = false
				h.scc[top] = comp
				members = append(members, top)
				if top == k {
					break
				}
			}
			comp++
			h.hashComponent(members)
		}
	}
	for _, k := range keys {
		if _, seen := index[k]; !seen {
			visit(k)
		}
	}
}

// hashComponent sets the deep hashes of one component: references inside
// it stay by name, references out of it are the (already computed) deep
// hashes, and every member covers the encodings of all members.
func (h *hasher) hashComponent(members []string) {
	sort.Strings(members)
	comp := h.scc[members[0]]
	tok := func(key string) string {
		if h.scc[key] == comp {
			return "@" + key
		}
		return "#" + h.deep[key]
	}
	encs := map[string][]byte{}
	for _, m := range members {
		encs[m], _ = h.encode(h.defs[m], tok)
	}
	for _, m := range members {
		parts := [][]byte{[]byte(h.defs[m].def.Kind), encs[m]}
		if len(members) > 1 {
			for _, o := range members {
				parts = append(parts, []byte(o), encs[o])
			}
		}
		h.deep[m] = sum(parts...)
		h.defs[m].def.Hash = h.deep[m]
	}
}

// ctx encodes one declaration.
type ctx struct {
	h      *hasher
	pkg    *pkgInfo
	file   *fileInfo
	self   string
	tok    func(key string) string
	locals map[string]string // local name -> token
	n      int
	refs   map[string]bool
	root   ast.Decl
	// named is set while the parameters of a declared function are
	// bound: they keep their names (named arguments see them).
	named bool
	buf   bytes.Buffer
}

func (c *ctx) w(tag string, s string) {
	fmt.Fprintf(&c.buf, "%s%d:%s", tag, len(s), s)
}

func (c *ctx) def(key string) {
	c.refs[key] = true
	if key == c.self {
		c.w("D", "@self")
		return
	}
	c.w("D", c.tok(key))
}

// bind binds a local name: a new token on its first binding.
func (c *ctx) bind(id *ast.Ident) {
	if id == nil {
		c.w("B", "")
		return
	}
	name := id.Name
	if name == "_" {
		c.w("B", "_")
		return
	}
	if c.named {
		c.locals[name] = "p:" + name
		c.w("B", "p:"+name)
		return
	}
	t, ok := c.locals[name]
	if !ok || strings.HasPrefix(t, "p:") {
		t = "$" + strconv.Itoa(c.n)
		c.n++
		c.locals[name] = t
	}
	c.w("B", t)
}

// ref is a name used as a value or a type: a local, a definition of the
// package, an imported package, or anything else (prelude, builtins) by
// name.
func (c *ctx) ref(name string) {
	if t, ok := c.locals[name]; ok {
		c.w("L", t)
		return
	}
	if key, ok := c.pkg.defs[name]; ok {
		c.def(key)
		return
	}
	if p, ok := c.file.imports[name]; ok {
		c.w("U", p)
		return
	}
	c.w("G", name)
}

// imported resolves alias.name to a definition of another package of
// the module.
func (c *ctx) imported(alias, name string) (string, bool) {
	if _, local := c.locals[alias]; local {
		return "", false
	}
	p, ok := c.file.imports[alias]
	if !ok {
		return "", false
	}
	pkg := c.h.pkgs[p]
	if pkg == nil {
		return "", false
	}
	key, ok := pkg.defs[name]
	return key, ok
}

// path encodes the names of a type or pattern path: Type, alias.Type,
// Type.Variant, alias.Type.Variant.
func (c *ctx) path(ids []*ast.Ident) {
	c.w("P", strconv.Itoa(len(ids)))
	if len(ids) == 0 {
		return
	}
	rest := ids[1:]
	if len(ids) >= 2 {
		if key, ok := c.imported(ids[0].Name, ids[1].Name); ok {
			c.def(key)
			rest = ids[2:]
		} else {
			c.ref(ids[0].Name)
		}
	} else {
		c.ref(ids[0].Name)
	}
	for _, id := range rest {
		c.w("N", id.Name)
	}
}

var (
	identType = reflect.TypeOf(&ast.Ident{})
	docType   = reflect.TypeOf(&ast.Doc{})
	baseType  = reflect.TypeOf(ast.Base{})
	kindType  = reflect.TypeOf(token.Kind(0))
)

// binders are the fields that bind a local name.
var binders = map[string]bool{
	"Param.Name": true, "Receiver.Name": true, "TypeParam.Name": true,
	"VarStmt.Name": true, "BindPat.Name": true, "SelectArm.Bind": true,
	"ScopeExpr.Name": true, "ForStmt.Label": true, "WhileStmt.Label": true,
}

func (c *ctx) enc(v reflect.Value) {
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			c.w("0", "")
			return
		}
		if id, ok := v.Interface().(*ast.Ident); ok {
			c.ref(id.Name) // an identifier in expression position
			return
		}
		c.enc(v.Elem())
	case reflect.Pointer:
		if v.IsNil() {
			c.w("0", "")
			return
		}
		if v.Type() == identType {
			c.w("N", v.Interface().(*ast.Ident).Name)
			return
		}
		c.node(v)
	case reflect.Slice:
		c.w("[", strconv.Itoa(v.Len()))
		for i := 0; i < v.Len(); i++ {
			c.enc(v.Index(i))
		}
	case reflect.Struct:
		c.fields(v)
	case reflect.String:
		c.w("s", v.String())
	case reflect.Bool:
		if v.Bool() {
			c.w("b", "1")
		} else {
			c.w("b", "0")
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.Type() == kindType {
			c.w("k", token.Kind(v.Int()).String())
			return
		}
		c.w("i", strconv.FormatInt(v.Int(), 10))
	default:
		c.w("?", fmt.Sprint(v.Interface()))
	}
}

// node encodes a pointer to a syntax tree struct, with the cases that
// are not plain field by field.
func (c *ctx) node(v reflect.Value) {
	switch n := v.Interface().(type) {
	case *ast.SelectorExpr:
		// Type.method, Type.CONST, or alias.name of a package of this
		// module: the definition itself
		if x, ok := n.X.(*ast.Ident); ok && n.Sel != nil {
			if _, local := c.locals[x.Name]; !local {
				if key, ok := c.pkg.defs[x.Name+"."+n.Sel.Name]; ok {
					c.w("(", "Sel")
					c.ref(x.Name)
					c.def(key)
					return
				}
				if key, ok := c.imported(x.Name, n.Sel.Name); ok {
					c.w("(", "Sel")
					c.def(key)
					return
				}
			}
		}
	case *ast.StructPat:
		// `Point{x, y}` binds the fields x and y: name the fields so a
		// renamed binding (`Point{x: a}`) encodes the same
		c.w("(", "StructPat")
		c.path(n.Path)
		c.w("[", strconv.Itoa(len(n.Fields)))
		for _, f := range n.Fields {
			name := ""
			if f.Name != nil {
				name = f.Name.Name
			} else if b, ok := f.Pattern.(*ast.BindPat); ok && b.Name != nil {
				name = b.Name.Name
			}
			c.w("N", name)
			c.enc(reflect.ValueOf(&f.Pattern).Elem())
		}
		c.enc(reflect.ValueOf(n.Rest))
		return
	case *ast.FnLit:
		saved := c.named
		c.named = false
		defer func() { c.named = saved }()
	}
	c.fields(v.Elem())
}

func (c *ctx) fields(v reflect.Value) {
	t := v.Type()
	c.w("(", t.Name())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fv := v.Field(i)
		if f.Type == baseType || f.Type == docType || !f.IsExported() {
			continue
		}
		key := t.Name() + "." + f.Name
		switch {
		case binders[key]:
			c.bind(fv.Interface().(*ast.Ident))
		case key == "FnDecl.Name" || key == "ConstDecl.Name":
			switch {
			case v.Addr().Interface() == c.root:
				// a definition's own name is not part of its content
			case key == "ConstDecl.Name":
				c.bind(fv.Interface().(*ast.Ident)) // a local constant
			default:
				c.enc(fv) // a method of an interface
			}
		case key == "FnDecl.Params":
			saved := c.named
			c.named = true
			c.enc(fv)
			c.named = saved
		case key == "Param.Type" || key == "Param.Default" || key == "FnDecl.Body" || key == "FnDecl.Clauses" || key == "FnDecl.Result" || key == "FnDecl.Err":
			saved := c.named
			c.named = false
			c.enc(fv)
			c.named = saved
		case key == "BreakStmt.Label" || key == "ContinueStmt.Label" || key == "Effect.Name":
			if id := fv.Interface().(*ast.Ident); id != nil {
				if t, ok := c.locals[id.Name]; ok {
					c.w("L", t)
				} else {
					c.w("N", id.Name)
				}
			} else {
				c.w("0", "")
			}
		case key == "NamedType.Path" || key == "VariantPat.Path":
			c.path(fv.Interface().([]*ast.Ident))
		default:
			c.enc(fv)
		}
	}
}
