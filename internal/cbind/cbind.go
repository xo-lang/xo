// Package cbind binds C headers for `use c "<header>"` on the native
// backends (decision 0093 stage 2, decision 0113).
//
// Bind reads the header with clang's JSON AST dump and writes two
// artifacts:
//
//   - an Xo declaration file in the bodiless form of std/*.xo, loaded by
//     internal/check as a std like module whose functions perform the
//     effect `ffi` (or the `uses` of the `use c` line);
//   - a C adapter: one function per binding, `xo_c_<prefix>_<name>`, with
//     the calling convention of the native runtime's calls (decision
//     0068: an integer or Bool is an int64_t, a Float a double, a Str or
//     Bytes its bytes and length, a Str result a runtime string and its
//     length through a pointer). The adapter converts to the C types,
//     copies strings that C reads as NUL terminated, and calls the C
//     function, so the code generators never see a C type.
//
// Boundary types (decision 0093): scalars, `Str` (a `const char *`
// parameter or result, copied), `Str` or `Bytes` from a const byte pointer
// followed by an integer length parameter (no copy: C must not keep the
// pointer after the call returns), flat structs of scalars (copied
// field by field), and opaque handles (a pointer to a struct the header
// does not define). Everything else is skipped with the reason.
package cbind

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/xo-lang/xo/internal/token"
)

// Version is the format version of generated C bindings.
const Version = 1

// Options says what to bind.
type Options struct {
	// Header is the header as written in `use c "<header>"`: a path
	// relative to ModDir when such a file exists, else a header on the C
	// compiler's include path (`zlib.h`).
	Header string
	// ModDir is the directory of the Xo module that imports the header.
	ModDir string
	// Links are the `link` clauses.
	Links []string
	// Uses are the effect labels of the `uses` clause; nil means `ffi`,
	// empty means none.
	Uses []string
	// Dir is the cache directory the declaration file is written to.
	Dir string
}

// Binding is one bound header.
type Binding struct {
	Header     string // as written
	HeaderPath string // the file clang read
	Local      bool   // found relative to the module directory
	Name       string // default module name
	Key        string // identity: header, links, and effects
	Prefix     string // adapter symbol prefix: xo_<Prefix><name>
	DeclPath   string
	Decl       []byte
	Adapter    []byte
	// IncludeDirs are the -I directories the adapter compiles with.
	IncludeDirs []string
	// LinkFiles are C sources, objects, and archives to link (absolute);
	// LinkLibs are -l names.
	LinkFiles []string
	LinkLibs  []string
	// Skipped maps the Xo name of a function that does not bind to why.
	Skipped map[string]string
	// Funcs are the bound functions by Xo name.
	Funcs map[string]*Func
	// Bound and Total count functions.
	Bound, Total int
}

// Func is one bound function.
type Func struct {
	XoName string
	CName  string
	// Replay is "value" (only boundary values cross) or "handle" (a C
	// handle crosses: a replay must run it again).
	Replay string
}

// Error is a header that cannot be bound.
type Error struct {
	Header string
	Msg    string
}

func (e *Error) Error() string { return fmt.Sprintf("cannot bind C header %q: %s", e.Header, e.Msg) }

// CacheDir is the binding cache of the Xo project rooted at root:
// $XO_CBIND_DIR when set, else <root>/.xo/cbind.
func CacheDir(root string) string {
	if d := os.Getenv("XO_CBIND_DIR"); d != "" {
		return d
	}
	return filepath.Join(root, ".xo", "cbind")
}

var (
	memoMu sync.Mutex
	memo   = map[string]*Binding{}
)

// Bind binds a header, reusing a binding made earlier in this process
// for the same header file contents, links, and effects.
func Bind(opts Options) (*Binding, error) {
	b := &binder{opts: opts, typedefs: map[string]string{}, records: map[string]*record{}}
	if err := b.locate(); err != nil {
		return nil, &Error{Header: opts.Header, Msg: err.Error()}
	}
	data, _ := os.ReadFile(b.bnd.HeaderPath)
	h := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00%q\x00%q\x00%s", Version, b.bnd.HeaderPath, data, opts.Links, opts.Uses, opts.Dir)))
	mk := hex.EncodeToString(h[:])
	memoMu.Lock()
	if old := memo[mk]; old != nil {
		memoMu.Unlock()
		return old, nil
	}
	memoMu.Unlock()
	if err := b.bind(); err != nil {
		return nil, &Error{Header: opts.Header, Msg: err.Error()}
	}
	memoMu.Lock()
	memo[mk] = b.bnd
	memoMu.Unlock()
	return b.bnd, nil
}

type record struct {
	name     string
	complete bool
	union    bool
	fields   []field
}

type field struct {
	name string
	t    string // clang's type spelling
}

type binder struct {
	opts     Options
	bnd      *Binding
	clang    string
	typedefs map[string]string
	records  map[string]*record // by tag
	decls    []decl

	handles map[string]string // struct tag -> Xo type name
	structs map[string]string // struct tag -> Xo type name (flat, copied)
	used    map[string]bool   // Xo names in use
}

// locate finds the header and the link inputs.
func (b *binder) locate() error {
	o := b.opts
	bnd := &Binding{Header: o.Header, Skipped: map[string]string{}, Funcs: map[string]*Func{}}
	b.bnd = bnd
	if o.Header == "" {
		return errors.New("empty header path")
	}
	local := filepath.Join(o.ModDir, o.Header)
	if filepath.IsAbs(o.Header) {
		local = o.Header
	}
	if st, err := os.Stat(local); err == nil && !st.IsDir() {
		abs, err := filepath.Abs(local)
		if err != nil {
			return err
		}
		bnd.HeaderPath = abs
		bnd.Local = true
		md, _ := filepath.Abs(o.ModDir)
		bnd.IncludeDirs = append(bnd.IncludeDirs, filepath.Dir(abs))
		if md != filepath.Dir(abs) {
			bnd.IncludeDirs = append(bnd.IncludeDirs, md)
		}
	}
	base := strings.TrimSuffix(filepath.Base(o.Header), filepath.Ext(o.Header))
	bnd.Name = moduleName(base)
	for _, l := range o.Links {
		if isLibName(l) {
			bnd.LinkLibs = append(bnd.LinkLibs, l)
			continue
		}
		p := l
		if !filepath.IsAbs(p) {
			p = filepath.Join(o.ModDir, l)
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return err
		}
		if _, err := os.Stat(abs); err != nil {
			return fmt.Errorf("`link %q`: no such file %s (a name without `/` or an extension links a library: `link \"z\"` is -lz)", l, abs)
		}
		bnd.LinkFiles = append(bnd.LinkFiles, abs)
	}
	effs := "ffi"
	if o.Uses != nil {
		effs = strings.Join(o.Uses, ",")
	}
	bnd.Key = "c:" + o.Header + "|" + strings.Join(o.Links, ",") + "|" + effs
	if bnd.Local {
		bnd.Key = "c:" + bnd.HeaderPath + "|" + strings.Join(bnd.LinkFiles, ",") + "|" + strings.Join(bnd.LinkLibs, ",") + "|" + effs
	}
	sum := sha256.Sum256([]byte(bnd.Key))
	bnd.Prefix = "c_" + bnd.Name + "_" + hex.EncodeToString(sum[:4]) + "_"
	clang, err := Clang()
	if err != nil {
		return err
	}
	b.clang = clang
	if !bnd.Local {
		// a header on the include path: clang reports where it is
		ds, err := dumpAST(clang, "#include <"+o.Header+">", nil)
		if err != nil {
			return fmt.Errorf("%v (a header path relative to the module directory, %s, was not found either)", err, o.ModDir)
		}
		for _, d := range ds {
			if d.file != "" && (strings.HasSuffix(d.file, "/"+o.Header) || d.file == o.Header) {
				bnd.HeaderPath = d.file
				break
			}
		}
		if bnd.HeaderPath == "" {
			return fmt.Errorf("clang read %s but found no declaration in it", o.Header)
		}
		b.decls = ds
	}
	return nil
}

// isLibName reports a `link` value that names a library (-l): no path
// separator and no extension.
func isLibName(l string) bool {
	return !strings.ContainsAny(l, "/\\") && filepath.Ext(l) == ""
}

// moduleName makes a header's base name a valid Xo module name.
func moduleName(base string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(snake(base)) {
		if r == '_' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			sb.WriteRune(r)
		} else {
			sb.WriteByte('_')
		}
	}
	s := strings.Trim(sb.String(), "_")
	if s == "" || s[0] >= '0' && s[0] <= '9' {
		s = "c_" + s
	}
	return xoName(s)
}

func (b *binder) bind() error {
	bnd := b.bnd
	if b.decls == nil {
		ds, err := dumpAST(b.clang, "#include \""+bnd.HeaderPath+"\"", bnd.IncludeDirs)
		if err != nil {
			return err
		}
		b.decls = ds
	}
	// typedefs and records from every file: the header's types may come
	// from the headers it includes
	for _, d := range b.decls {
		n := d.n
		switch n.str("kind") {
		case "TypedefDecl":
			if t := n.obj("type"); t != nil {
				q := t.str("desugaredQualType")
				if q == "" {
					q = t.str("qualType")
				}
				b.typedefs[n.str("name")] = q
			}
		case "RecordDecl":
			name := n.str("name")
			if name == "" {
				continue
			}
			r := b.records[name]
			if r == nil {
				r = &record{name: name, union: n.str("tagUsed") == "union"}
				b.records[name] = r
			}
			if n.flag("completeDefinition") {
				r.complete = true
				r.fields = nil
				for _, e := range n.list("inner") {
					f, ok := e.(*obj)
					if !ok || f.str("kind") != "FieldDecl" {
						continue
					}
					ft := f.obj("type")
					q := ""
					if ft != nil {
						q = ft.str("desugaredQualType")
						if q == "" {
							q = ft.str("qualType")
						}
					}
					r.fields = append(r.fields, field{name: f.str("name"), t: q})
				}
			}
		}
	}
	b.handles = map[string]string{}
	b.structs = map[string]string{}
	b.used = map[string]bool{}
	var fns []*cfunc
	seenFn := map[string]bool{}
	for _, d := range b.decls {
		n := d.n
		if n.str("kind") != "FunctionDecl" || !b.inHeader(d.file) || n.flag("isImplicit") {
			continue
		}
		name := n.str("name")
		if seenFn[name] {
			continue
		}
		seenFn[name] = true
		bnd.Total++
		f, err := b.function(n)
		if err != nil {
			bnd.Skipped[xoName(snake(name))] = fmt.Sprintf("%v (%s)", err, cSignature(n))
			continue
		}
		fns = append(fns, f)
	}
	sort.Slice(fns, func(i, j int) bool { return fns[i].xo < fns[j].xo })
	bnd.Bound = len(fns)
	for _, f := range fns {
		bnd.Funcs[f.xo] = &Func{XoName: f.xo, CName: f.c, Replay: f.replay}
	}
	bnd.Decl = b.declFile(fns)
	bnd.Adapter = b.adapterFile(fns)
	if b.opts.Dir != "" {
		bnd.DeclPath = filepath.Join(b.opts.Dir, bnd.Prefix[:len(bnd.Prefix)-1]+".xo")
		if err := os.MkdirAll(b.opts.Dir, 0o755); err != nil {
			return err
		}
		if old, err := os.ReadFile(bnd.DeclPath); err != nil || string(old) != string(bnd.Decl) {
			if err := os.WriteFile(bnd.DeclPath, bnd.Decl, 0o644); err != nil {
				return err
			}
		}
		ap := strings.TrimSuffix(bnd.DeclPath, ".xo") + ".c"
		if old, err := os.ReadFile(ap); err != nil || string(old) != string(bnd.Adapter) {
			if err := os.WriteFile(ap, bnd.Adapter, 0o644); err != nil {
				return err
			}
		}
	} else {
		bnd.DeclPath = bnd.Prefix[:len(bnd.Prefix)-1] + ".xo"
	}
	return nil
}

// inHeader reports whether a declaration's file is the bound header.
func (b *binder) inHeader(file string) bool {
	if file == "" {
		return false
	}
	if file == b.bnd.HeaderPath {
		return true
	}
	if abs, err := filepath.Abs(file); err == nil && abs == b.bnd.HeaderPath {
		return true
	}
	return false
}

// cSignature is the C declaration of a function, for doc comments.
func cSignature(n *obj) string {
	ret := ""
	if t := n.obj("type"); t != nil {
		q := t.str("qualType")
		if i := strings.Index(q, "("); i >= 0 {
			ret = strings.TrimSpace(q[:i])
		}
	}
	var ps []string
	for _, e := range n.list("inner") {
		p, ok := e.(*obj)
		if !ok || p.str("kind") != "ParmVarDecl" {
			continue
		}
		q := ""
		if t := p.obj("type"); t != nil {
			q = t.str("qualType")
		}
		if nm := p.str("name"); nm != "" {
			if strings.HasSuffix(q, "*") {
				q += nm
			} else {
				q += " " + nm
			}
		}
		ps = append(ps, q)
	}
	if n.flag("variadic") {
		ps = append(ps, "...")
	}
	if len(ps) == 0 {
		ps = []string{"void"}
	}
	sep := " "
	if strings.HasSuffix(ret, "*") {
		sep = ""
	}
	return ret + sep + n.str("name") + "(" + strings.Join(ps, ", ") + ")"
}

// ---------------------------------------------------------------------------
// Names

// snake converts a C name to snake_case: zlibVersion -> zlib_version.
func snake(name string) string {
	rs := []rune(name)
	var sb strings.Builder
	for i, r := range rs {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := rs[i-1]
				nextLower := i+1 < len(rs) && unicode.IsLower(rs[i+1])
				if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextLower) {
					sb.WriteByte('_')
				}
			}
			sb.WriteRune(unicode.ToLower(r))
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// upperCamel converts a C tag to UpperCamel: tiny_point -> TinyPoint.
func upperCamel(name string) string {
	var sb strings.Builder
	up := true
	for _, r := range name {
		if r == '_' {
			up = true
			continue
		}
		if up {
			sb.WriteRune(unicode.ToUpper(r))
			up = false
			continue
		}
		sb.WriteRune(r)
	}
	s := sb.String()
	if s == "" || !unicode.IsUpper(rune(s[0])) {
		s = "C" + s
	}
	return s
}

// xoName makes a snake_case name usable in Xo: keywords get a `_` suffix.
func xoName(s string) string {
	if s == "" || s == "_" {
		return s
	}
	if token.Lookup(s) != token.Ident || s == "true" || s == "false" || s == "self" || s == "static" || s == "go" {
		return s + "_"
	}
	return s
}
