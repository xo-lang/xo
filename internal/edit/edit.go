// Package edit implements `xo edit` (core 10.4): semantic edits that find
// every reference through the checker's resolved symbols, rewrite all of
// them at once, and are rejected (nothing written) unless the edited
// program checks clean.
package edit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/format"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/parser"
	"github.com/xo-lang/xo/internal/query"
)

// Options configures an edit.
type Options struct {
	Dirs   []string // module directories (default ".")
	DryRun bool     // compute and check the edit, write nothing
}

// Site is a source position named in a result.
type Site struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Col    int    `json:"col"`
	Reason string `json:"reason,omitempty"`
}

// Result is the JSON summary of an edit.
type Result struct {
	Op     string `json:"op"`
	Target string `json:"target"`
	// OK is false when the edit was rejected; then nothing is written.
	OK      bool   `json:"ok"`
	Written bool   `json:"written"`
	Reason  string `json:"reason,omitempty"`
	// Files are the files the edit changes (or would change), sorted.
	Files []string `json:"files"`
	// Edits counts the text replacements.
	Edits int `json:"edits"`
	// Diagnostics are the errors that rejected the edit: of the program
	// before the edit, or of the edited program.
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
	// Sites are the places that blocked the edit (change-sig call sites
	// that cannot be rewritten mechanically).
	Sites []Site `json:"sites,omitempty"`
}

// textEdit replaces src[start:end] of one file.
type textEdit struct {
	file       string // path as the checker names it
	start, end int
	text       string
}

// session is a checked program about to be edited.
type session struct {
	opt  Options
	q    *query.Session
	res  *Result
	errs []diag.Diagnostic
}

func load(op, target string, opt Options) (*session, error) {
	if len(opt.Dirs) == 0 {
		opt.Dirs = []string{"."}
	}
	// a workspace is one program: an edit reaches every module of it
	// (decision 0086)
	opt.Dirs = mod.WorkspaceDirs(opt.Dirs)
	q, err := query.Load(opt.Dirs)
	if err != nil {
		return nil, err
	}
	s := &session{opt: opt, q: q, res: &Result{Op: op, Target: target, Files: []string{}, Diagnostics: []diag.Diagnostic{}}}
	s.errs = errorsOf(q.Res.Diagnostics)
	return s, nil
}

func errorsOf(ds []diag.Diagnostic) []diag.Diagnostic {
	out := []diag.Diagnostic{}
	for _, d := range ds {
		if d.Severity == diag.Error {
			out = append(out, d)
		}
	}
	return out
}

// reject marks the result rejected.
func (s *session) reject(reason string) *Result {
	s.res.OK = false
	s.res.Reason = reason
	return s.res
}

// precheck rejects an edit of a program that does not check clean: the
// resolved symbols of a broken program can miss references.
func (s *session) precheck() *Result {
	if len(s.errs) > 0 {
		s.res.Diagnostics = s.errs
		return s.reject("the program does not check clean before the edit")
	}
	return nil
}

// userFiles lists the files of the non std modules.
func (s *session) userFiles() []*check.File {
	var out []*check.File
	for _, m := range s.q.Info.AllModules() {
		if m.IsStd {
			continue
		}
		out = append(out, m.Files...)
	}
	return out
}

func (s *session) fileByPath(path string) *check.File {
	for _, f := range s.userFiles() {
		if f.Path == path {
			return f
		}
	}
	return nil
}

// apply applies edits in memory, checks the edited program, and writes it
// unless the check fails or DryRun is set.
func (s *session) apply(edits []textEdit) (*Result, error) {
	byFile := map[string][]textEdit{}
	for _, e := range edits {
		byFile[e.file] = append(byFile[e.file], e)
	}
	overlay := map[string][]byte{}
	out := map[string][]byte{} // abs -> final contents
	orig := map[string][]byte{}
	for name, es := range byFile {
		f := s.fileByPath(name)
		if f == nil {
			return nil, fmt.Errorf("edit of %s outside the user modules", name)
		}
		if f.Module != nil && f.Module.Dependency != "" {
			return s.reject(fmt.Sprintf("the edit would change %s in the required module %s; change it in its repository, or list its local copy in an xo.work workspace (decision 0086)", name, f.Module.Dependency)), nil
		}
		abs, err := filepath.Abs(name)
		if err != nil {
			return nil, err
		}
		src, err := applyEdits(f.Src, es)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", name, err)
		}
		// Keep canonical files canonical; leave others as rewritten.
		if canon, err := format.Source(name, f.Src); err == nil && bytes.Equal(canon, f.Src) {
			if o, err := format.Source(name, src); err == nil {
				src = o
			}
		}
		overlay[abs] = src
		out[abs] = src
		orig[abs] = f.Src
		s.res.Files = append(s.res.Files, name)
	}
	sort.Strings(s.res.Files)
	s.res.Edits = len(edits)
	cr, err := check.CheckOverlay(s.opt.Dirs, overlay)
	if err != nil {
		return nil, err
	}
	if errs := errorsOf(cr.Diagnostics); len(errs) > 0 {
		s.res.Diagnostics = errs
		return s.reject("the edited program does not check clean"), nil
	}
	s.res.OK = true
	if s.opt.DryRun {
		return s.res, nil
	}
	if err := writeAll(out, orig); err != nil {
		return nil, err
	}
	s.res.Written = true
	return s.res, nil
}

// writeAll writes every file through a temporary file and a rename, so a
// reader never sees a half written file. If a write fails, the files
// already replaced are restored.
func writeAll(out, orig map[string][]byte) error {
	var tmps []string
	defer func() {
		for _, t := range tmps {
			os.Remove(t)
		}
	}()
	paths := make([]string, 0, len(out))
	for p := range out {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	staged := map[string]string{}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return err
		}
		f, err := os.CreateTemp(filepath.Dir(p), ".xo-edit-*")
		if err != nil {
			return err
		}
		tmps = append(tmps, f.Name())
		_, werr := f.Write(out[p])
		cerr := f.Close()
		if werr != nil {
			return werr
		}
		if cerr != nil {
			return cerr
		}
		if err := os.Chmod(f.Name(), info.Mode().Perm()); err != nil {
			return err
		}
		staged[p] = f.Name()
	}
	var done []string
	for _, p := range paths {
		if err := os.Rename(staged[p], p); err != nil {
			for _, d := range done {
				_ = os.WriteFile(d, orig[d], 0o644)
			}
			return err
		}
		done = append(done, p)
	}
	return nil
}

func applyEdits(src []byte, es []textEdit) ([]byte, error) {
	sort.SliceStable(es, func(i, j int) bool { return es[i].start < es[j].start })
	var b bytes.Buffer
	pos := 0
	for _, e := range es {
		if e.start < pos || e.end < e.start || e.end > len(src) {
			return nil, fmt.Errorf("overlapping edits at offset %d", e.start)
		}
		b.Write(src[pos:e.start])
		b.WriteString(e.text)
		pos = e.end
	}
	b.Write(src[pos:])
	return b.Bytes(), nil
}

// ---------------------------------------------------------------------------
// target resolution

func (s *session) object(name string) (check.Object, error) {
	objs := s.q.Objects(name)
	var user []check.Object
	for _, o := range objs {
		if m := moduleOf(o); m != nil && !m.IsStd {
			user = append(user, o)
		}
	}
	switch len(user) {
	case 0:
		if len(objs) > 0 {
			return nil, fmt.Errorf("%q is declared in std or a binding, which xo edit does not change", name)
		}
		return nil, fmt.Errorf("%q does not name a declaration in the checked modules", name)
	case 1:
		return user[0], nil
	}
	return nil, fmt.Errorf("%q is ambiguous; qualify it with the module", name)
}

func moduleOf(o check.Object) *check.Module {
	switch x := o.(type) {
	case *check.FuncObj:
		return x.Module
	case *check.TypeObj:
		return x.Module
	case *check.ConstObj:
		return x.Module
	}
	return nil
}

var identRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// nameIdent returns the identifier that names the callee of a call or a
// reference: `f`, `m.f`, `x.f`, or the same with type arguments.
func nameIdent(e ast.Expr) *ast.Ident {
	if ta, ok := e.(*ast.TypeArgsExpr); ok {
		e = ta.X
	}
	switch x := e.(type) {
	case *ast.Ident:
		return x
	case *ast.SelectorExpr:
		return x.Sel
	}
	return nil
}

// ---------------------------------------------------------------------------
// rename

// Rename renames the function, method, or constant target to newName and
// updates every reference. newName may repeat target's qualifier
// (`users.fetch_user users.load_user`).
func Rename(opt Options, target, newName string) (*Result, error) {
	s, err := load("rename", target, opt)
	if err != nil {
		return nil, err
	}
	if i := strings.LastIndexByte(newName, '.'); i >= 0 {
		if j := strings.LastIndexByte(target, '.'); j < 0 || newName[:i] != target[:j] {
			return nil, fmt.Errorf("new name %q must be a plain name or keep the qualifier of %q", newName, target)
		}
		newName = newName[i+1:]
	}
	if !identRE.MatchString(newName) {
		return nil, fmt.Errorf("%q is not an identifier", newName)
	}
	obj, err := s.object(target)
	if err != nil {
		return nil, err
	}
	if r := s.precheck(); r != nil {
		return r, nil
	}
	var decl *ast.Ident
	switch o := obj.(type) {
	case *check.FuncObj:
		if o.Decl == nil || o.Iface {
			return nil, fmt.Errorf("%q: renaming interface methods is not supported yet", target)
		}
		decl = o.Decl.Name
	case *check.ConstObj:
		if o.Decl == nil {
			return nil, fmt.Errorf("%q has no declaration to rename", target)
		}
		decl = o.Decl.Name
	default:
		return nil, fmt.Errorf("%q: rename supports functions, methods, and constants so far (not types)", target)
	}
	if decl.Name == newName {
		s.res.OK = true
		return s.res, nil
	}
	seen := map[int]map[string]bool{}
	var edits []textEdit
	add := func(id *ast.Ident) {
		if id == nil {
			return
		}
		sp := id.Sp
		if seen[sp.Start.Offset] == nil {
			seen[sp.Start.Offset] = map[string]bool{}
		}
		if seen[sp.Start.Offset][sp.File] {
			return
		}
		seen[sp.Start.Offset][sp.File] = true
		edits = append(edits, textEdit{file: sp.File, start: sp.Start.Offset, end: sp.End.Offset, text: newName})
	}
	add(decl)
	for _, f := range s.userFiles() {
		for _, d := range f.AST.Decls {
			check.Inspect(d, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					if ci := s.q.Info.CallOf(x); ci != nil && ci.Fn != nil && check.Object(ci.Fn) == obj {
						add(nameIdent(x.Fun))
					}
				case *ast.Ident, *ast.SelectorExpr:
					if s.q.Info.Ref(x.(ast.Expr)) == obj {
						add(nameIdent(x.(ast.Expr)))
					}
				}
				return true
			})
		}
	}
	return s.apply(edits)
}

// ---------------------------------------------------------------------------
// add-field

// AddField adds the field `name: Type = default` (spec) to the struct
// target. The declaration gets `name: Type` and every struct literal of
// the type gets `name: default`. Without `= default`, the edit is
// rejected when the type has struct literals.
func AddField(opt Options, target, spec string) (*Result, error) {
	s, err := load("add-field", target, opt)
	if err != nil {
		return nil, err
	}
	fsrc := "struct XoEditField {\n" + spec + "\n}\n"
	pf, ds := parser.ParseFile("field", []byte(fsrc), 0)
	if errs := errorsOf(ds); len(errs) > 0 || len(pf.Decls) != 1 {
		return nil, fmt.Errorf("field %q does not parse as `name: Type = default`", spec)
	}
	sd, ok := pf.Decls[0].(*ast.StructDecl)
	if !ok || len(sd.Fields) != 1 {
		return nil, fmt.Errorf("field %q does not parse as one field `name: Type = default`", spec)
	}
	nf := sd.Fields[0]
	fname := nf.Name.Name
	ftype := text([]byte(fsrc), nf.Type)
	pub := ""
	if nf.Pub {
		pub = "pub "
	}
	def := ""
	if nf.Default != nil {
		def = text([]byte(fsrc), nf.Default)
	}
	obj, err := s.object(target)
	if err != nil {
		return nil, err
	}
	t, ok := obj.(*check.TypeObj)
	if !ok || t.Kind != check.KStruct {
		return nil, fmt.Errorf("%q is not a struct", target)
	}
	decl, ok := t.Decl().(*ast.StructDecl)
	if !ok || decl == nil {
		return nil, fmt.Errorf("%q has no struct declaration", target)
	}
	for _, f := range t.Fields {
		if f.Name == fname {
			return nil, fmt.Errorf("%s already has a field %s", t.Name, fname)
		}
	}
	if r := s.precheck(); r != nil {
		return r, nil
	}
	file := s.fileByPath(decl.Sp.File)
	if file == nil {
		return nil, fmt.Errorf("%s: file not found", decl.Sp.File)
	}
	var edits []textEdit
	// The declaration: a new line after the last field, or inside `{}`.
	line := pub + fname + ": " + ftype
	if n := len(decl.Fields); n > 0 {
		last := decl.Fields[n-1]
		at := last.Sp.End.Offset
		if last.Default != nil && last.Default.Span().End.Offset > at {
			at = last.Default.Span().End.Offset
		}
		edits = append(edits, textEdit{file: decl.Sp.File, start: at, end: at, text: "\n" + indentOf(file.Src, last.Sp.Start.Offset) + line})
	} else {
		open := bytes.IndexByte(file.Src[decl.Name.Sp.End.Offset:decl.Sp.End.Offset], '{')
		if open < 0 {
			return nil, fmt.Errorf("%s: cannot find the struct body", t.Name)
		}
		at := decl.Name.Sp.End.Offset + open + 1
		edits = append(edits, textEdit{file: decl.Sp.File, start: at, end: at, text: "\n  " + line + "\n"})
	}
	// Every struct literal of the type.
	var missing []Site
	for _, f := range s.userFiles() {
		for _, d := range f.AST.Decls {
			check.Inspect(d, func(n ast.Node) bool {
				lit, ok := n.(*ast.StructLit)
				if !ok {
					return true
				}
				nt, ok := check.Resolve(s.q.Info.TypeOf(lit)).(*check.Named)
				if !ok || nt.Obj != t {
					return true
				}
				if def == "" {
					missing = append(missing, site(lit, "struct literal needs a value for "+fname))
					return true
				}
				edits = append(edits, literalEdit(f.Src, lit, fname+": "+def))
				return true
			})
		}
	}
	if len(missing) > 0 {
		s.res.Sites = missing
		return s.reject("the new field has no default and the type has struct literals"), nil
	}
	return s.apply(edits)
}

// literalEdit inserts `init` as the last field of a struct literal.
func literalEdit(src []byte, lit *ast.StructLit, init string) textEdit {
	file := lit.Sp.File
	if n := len(lit.Fields); n > 0 {
		at := lit.Fields[n-1].Sp.End.Offset
		if v := lit.Fields[n-1].Value; v != nil && v.Span().End.Offset > at {
			at = v.Span().End.Offset
		}
		// A trailing comma stays last: `a: 1,\n}` becomes `a: 1, f: d,\n}`.
		q := at
		for q < len(src) && (src[q] == ' ' || src[q] == '\t' || src[q] == '\n' || src[q] == '\r') {
			q++
		}
		if q < len(src) && src[q] == ',' {
			return textEdit{file: file, start: q + 1, end: q + 1, text: " " + init + ","}
		}
		return textEdit{file: file, start: at, end: at, text: ", " + init}
	}
	end := lit.Sp.End.Offset - 1 // the closing brace
	return textEdit{file: file, start: end, end: end, text: init}
}

func indentOf(src []byte, off int) string {
	start := bytes.LastIndexByte(src[:off], '\n') + 1
	i := start
	for i < off && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	return string(src[start:i])
}

func text(src []byte, n ast.Node) string {
	sp := n.Span()
	return string(src[sp.Start.Offset:sp.End.Offset])
}

func site(n ast.Node, reason string) Site {
	sp := n.Span()
	return Site{File: sp.File, Line: sp.Start.Line, Col: sp.Start.Col, Reason: reason}
}

// ---------------------------------------------------------------------------
// change-sig

// ChangeSig replaces the parameters, result, and error type of the
// function or method target with sig, `(params) -> R ! E`, and rewrites
// its call sites. Arguments are matched to the new parameters by name (a
// parameter renamed in place keeps its position); parameters added with a
// default need no argument. A call that passes an argument for a removed
// parameter, or lacks one for a new parameter without a default, cannot be
// rewritten mechanically: the edit is rejected with those sites. Argument
// order, and so evaluation order, is kept; arguments that move become
// named.
func ChangeSig(opt Options, target, sig string) (*Result, error) {
	s, err := load("change-sig", target, opt)
	if err != nil {
		return nil, err
	}
	sig = strings.TrimSpace(sig)
	if !strings.HasPrefix(sig, "(") {
		return nil, fmt.Errorf("signature %q must start with the parameter list `(`", sig)
	}
	fsrc := "fn xo_edit_sig" + sig + " {}\n"
	pf, ds := parser.ParseFile("sig", []byte(fsrc), 0)
	if errs := errorsOf(ds); len(errs) > 0 || len(pf.Decls) != 1 {
		return nil, fmt.Errorf("signature %q does not parse as `(params) -> Result ! Err`", sig)
	}
	nd, ok := pf.Decls[0].(*ast.FnDecl)
	if !ok {
		return nil, fmt.Errorf("signature %q does not parse", sig)
	}
	if len(nd.Clauses) > 0 {
		return nil, fmt.Errorf("signature %q has clauses; change-sig replaces the parameters, result, and error type only", sig)
	}
	newText := fsrc[len("fn xo_edit_sig"):sigEnd([]byte(fsrc), nd)]
	obj, err := s.object(target)
	if err != nil {
		return nil, err
	}
	fo, ok := obj.(*check.FuncObj)
	if !ok || fo.Decl == nil || fo.Iface {
		return nil, fmt.Errorf("%q is not a function or method with a body", target)
	}
	if r := s.precheck(); r != nil {
		return r, nil
	}
	od := fo.Decl
	file := s.fileByPath(od.Sp.File)
	if file == nil {
		return nil, fmt.Errorf("%s: file not found", od.Sp.File)
	}
	from := od.Name.Sp.End.Offset
	if n := len(od.TypeParams); n > 0 {
		from = od.TypeParams[n-1].Sp.End.Offset
	}
	open := bytes.IndexByte(file.Src[from:], '(')
	if open < 0 {
		return nil, fmt.Errorf("%s: cannot find the parameter list", target)
	}
	edits := []textEdit{{file: od.Sp.File, start: from + open, end: sigEnd(file.Src, od), text: newText}}

	// old parameter name -> new parameter name ("" when removed)
	oldNames := paramNames(od.Params)
	newNames := paramNames(nd.Params)
	inNew := map[string]int{}
	for i, n := range newNames {
		inNew[n] = i
	}
	inOld := map[string]bool{}
	for _, n := range oldNames {
		inOld[n] = true
	}
	mapTo := map[string]string{}
	for i, n := range oldNames {
		if _, ok := inNew[n]; ok {
			mapTo[n] = n
		} else if i < len(newNames) && !inOld[newNames[i]] {
			mapTo[n] = newNames[i] // renamed in place
		}
	}
	// A parameter renamed in place is renamed in the body too.
	for _, pv := range s.q.Info.ParamVars(fo) {
		to, ok := mapTo[pv.Name]
		if !ok || to == pv.Name || od.Body == nil {
			continue
		}
		check.Inspect(od.Body, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && s.q.Info.VarOf(id) == pv {
				edits = append(edits, textEdit{file: id.Sp.File, start: id.Sp.Start.Offset, end: id.Sp.End.Offset, text: to})
			}
			return true
		})
	}

	var blocked []Site
	for _, f := range s.userFiles() {
		for _, d := range f.AST.Decls {
			check.Inspect(d, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				ci := s.q.Info.CallOf(call)
				if ci == nil || ci.Fn != fo {
					return true
				}
				e, why := rewriteCall(f.Src, call, oldNames, newNames, nd.Params, mapTo)
				if why != "" {
					blocked = append(blocked, site(call, why))
				} else if e != nil {
					edits = append(edits, *e)
				}
				return true
			})
		}
	}
	if len(blocked) > 0 {
		s.res.Sites = blocked
		return s.reject("some call sites cannot be rewritten mechanically"), nil
	}
	return s.apply(edits)
}

// sigEnd is the offset just past the signature of d: its error type,
// result type, or parameter list.
func sigEnd(src []byte, d *ast.FnDecl) int {
	if d.Err != nil {
		return d.Err.Sp.End.Offset
	}
	if d.Result != nil {
		return d.Result.Span().End.Offset
	}
	from := d.Name.Sp.End.Offset
	if n := len(d.Params); n > 0 {
		from = d.Params[n-1].Sp.End.Offset
		if p := d.Params[n-1]; p.Default != nil && p.Default.Span().End.Offset > from {
			from = p.Default.Span().End.Offset
		}
	} else if n := len(d.TypeParams); n > 0 {
		from = d.TypeParams[n-1].Sp.End.Offset
	}
	if i := bytes.IndexByte(src[from:], ')'); i >= 0 {
		return from + i + 1
	}
	return from
}

func paramNames(ps []*ast.Param) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		if p.Name != nil {
			out[i] = p.Name.Name
		}
	}
	return out
}

// rewriteCall returns the edit of one call's argument list (nil when it
// does not change), or why it cannot be rewritten.
func rewriteCall(src []byte, call *ast.CallExpr, oldNames, newNames []string, newParams []*ast.Param, mapTo map[string]string) (*textEdit, string) {
	type arg struct {
		param string // new parameter name
		value string
		named bool
	}
	var args []arg
	passed := map[string]bool{}
	for i, a := range call.Args {
		old := ""
		if a.Name != nil {
			old = a.Name.Name
		} else if i < len(oldNames) {
			old = oldNames[i]
		}
		to, ok := mapTo[old]
		if !ok {
			return nil, fmt.Sprintf("passes an argument for removed parameter %s", old)
		}
		passed[to] = true
		args = append(args, arg{param: to, value: text(src, a.Value), named: a.Name != nil})
	}
	for i, n := range newNames {
		if !passed[n] && newParams[i].Default == nil {
			return nil, fmt.Sprintf("needs an argument for new parameter %s, which has no default", n)
		}
	}
	idx := map[string]int{}
	for i, n := range newNames {
		idx[n] = i
	}
	var parts []string
	positional := true
	for i, a := range args {
		if positional && (a.named || idx[a.param] != i) {
			positional = false
		}
		if positional {
			parts = append(parts, a.value)
		} else {
			parts = append(parts, a.param+": "+a.value)
		}
	}
	var old []string
	for _, a := range call.Args {
		old = append(old, text(src, a))
	}
	if strings.Join(parts, ", ") == strings.Join(old, ", ") {
		return nil, ""
	}
	if len(call.Args) == 0 {
		return nil, "" // nothing passed and nothing needed
	}
	start := call.Args[0].Sp.Start.Offset
	end := call.Args[len(call.Args)-1].Sp.End.Offset
	return &textEdit{file: call.Sp.File, start: start, end: end, text: strings.Join(parts, ", ")}, ""
}
