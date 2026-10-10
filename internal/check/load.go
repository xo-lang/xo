package check

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gobind"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/parser"
	"github.com/xo-lang/xo/internal/token"
	"github.com/xo-lang/xo/std"
)

// modRoot maps a module path prefix declared in an xo.mod to its directory.
type modRoot struct {
	path    string
	dir     string
	edition int
	gc      GCConfig
}

// localName derives a module's local name from an import path (core 4.1):
// the last segment with `-` replaced by `_`. ok is false when the result is
// not a valid identifier or is a keyword (XO0602). A /vN major version
// suffix of a module path is skipped (`use github.com/acme/billing/v2`
// binds `billing`).
func localName(path string) (string, bool) {
	if mod.IsRemote(path) {
		path, _ = mod.MajorSuffix(path)
	}
	seg := path
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		seg = path[i+1:]
	}
	name := strings.ReplaceAll(seg, "-", "_")
	if name == "" || token.Lookup(name) != token.Ident {
		return name, false
	}
	for i, ch := range name {
		if !(ch == '_' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || i > 0 && ch >= '0' && ch <= '9') {
			return name, false
		}
	}
	return name, true
}

func (c *checker) loadPrelude() error {
	m, err := c.loadStd("prelude")
	if err != nil {
		return err
	}
	if m == nil {
		return fmt.Errorf("std/prelude.xo is missing")
	}
	m.IsPrelude = true
	c.prelude = m
	// std/testing provides the predeclared `os`, `expect`, and `testing` of
	// test blocks (core 9.2), so it is always loaded.
	if _, err := c.loadStd("testing"); err != nil {
		return err
	}
	return nil
}

// loadStd loads std/<name>.xo, returning nil if it does not exist.
func (c *checker) loadStd(name string) (*Module, error) {
	path := "std/" + name
	if m, ok := c.modules[path]; ok {
		return m, nil
	}
	src, err := fs.ReadFile(std.Files, name+".xo")
	if err != nil {
		return nil, nil
	}
	m := &Module{Path: path, Name: name, Dir: "std", IsStd: true, Scope: map[string]Object{}}
	if name == "prelude" {
		m.IsPrelude = true
	}
	c.modules[path] = m
	f := c.parseFile(m, "std/"+name+".xo", src, parser.SignatureMode)
	m.Files = append(m.Files, f)
	m.state = 1
	if err := c.loadImports(m); err != nil {
		return nil, err
	}
	m.state = 2
	c.order = append(c.order, m)
	return m, nil
}

func (c *checker) parseFile(m *Module, path string, src []byte, mode parser.Mode) *File {
	af, ds := parser.ParseFile(path, src, mode)
	for _, d := range ds {
		c.diags.Add(d)
	}
	return &File{AST: af, Path: path, Module: m, Imports: map[string]*Import{}, IsTest: strings.HasSuffix(path, "_test.xo"), Src: src}
}

// loadNative loads the implementation modules of the native backends
// (decision 0074): std/native/prelude.xo and std/native/<name>.xo for
// every std module <name> loaded so far, until no new std module appears
// (an implementation may import more std modules).
func (c *checker) loadNative() error {
	done := map[string]bool{}
	for {
		var todo []string
		for path, m := range c.modules {
			if !m.IsStd || m.Go != nil || done[path] || !strings.HasPrefix(path, "std/") {
				continue
			}
			todo = append(todo, path)
		}
		if len(todo) == 0 {
			return nil
		}
		sort.Strings(todo)
		for _, path := range todo {
			done[path] = true
			name := strings.TrimPrefix(path, "std/")
			src, err := fs.ReadFile(std.NativeFiles, "native/"+name+".xo")
			if err != nil {
				continue
			}
			npath := "native/" + name
			m := &Module{Path: npath, Name: name, Dir: "std/native", Native: true, Implements: path, Scope: map[string]Object{}, Edition: CurrentEdition}
			c.modules[npath] = m
			m.Files = append(m.Files, c.parseFile(m, "std/native/"+name+".xo", src, 0))
			m.state = 1
			if err := c.loadImports(m); err != nil {
				return err
			}
			m.state = 2
			c.order = append(c.order, m)
		}
	}
}

// loadNativeRT loads the intrinsics module of the native backends,
// std/native/rt.xo: bodiless declarations that internal/ssa lowers to
// calls of the C runtime.
func (c *checker) loadNativeRT(path string) (*Module, error) {
	if m, ok := c.modules[path]; ok {
		return m, nil
	}
	src, err := fs.ReadFile(std.NativeFiles, path+".xo")
	if err != nil {
		return nil, nil
	}
	name := strings.TrimPrefix(path, "native/")
	m := &Module{Path: path, Name: name, Dir: "std/native", IsStd: true, Scope: map[string]Object{}}
	c.modules[path] = m
	m.Files = append(m.Files, c.parseFile(m, "std/"+path+".xo", src, parser.SignatureMode))
	m.state = 1
	if err := c.loadImports(m); err != nil {
		return nil, err
	}
	m.state = 2
	c.order = append(c.order, m)
	return m, nil
}

// loadDirs finds every directory under dirs that holds .xo files and loads
// it as a module.
func (c *checker) loadDirs(dirs []string) ([]*Module, error) {
	var moduleDirs []string
	seen := map[string]bool{}
	for _, d := range dirs {
		info, err := os.Stat(d)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%s is not a directory (a module is a directory, core 1.1)", d)
		}
		err = filepath.WalkDir(d, func(path string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if e.IsDir() {
				if path != d && (e.Name() == "testdata" || strings.HasPrefix(e.Name(), ".") || isVendorDir(path)) {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, ".xo") {
				dir := filepath.Dir(path)
				if !seen[dir] {
					seen[dir] = true
					moduleDirs = append(moduleDirs, dir)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(moduleDirs)
	// resolve the requirements of every main module before loading, so
	// imports of required modules find their selected versions
	for _, dir := range moduleDirs {
		if r, ok := c.findModRoot(dir); ok {
			c.resolveDeps(r)
		}
	}
	var roots []*Module
	for _, dir := range moduleDirs {
		m, err := c.loadDir(dir)
		if err != nil {
			return nil, err
		}
		m.Root = true
		roots = append(roots, m)
	}
	return roots, nil
}

// readSource reads a module source file, from the overlay when it holds
// the file (xo fix checks rewritten sources before writing them).
func (c *checker) readSource(p string) ([]byte, error) {
	if c.overlay != nil {
		if abs, err := filepath.Abs(p); err == nil {
			if src, ok := c.overlay[abs]; ok {
				return src, nil
			}
		}
	}
	return os.ReadFile(p)
}

// findModRoot looks for xo.mod in dir or its parents and registers it.
func (c *checker) findModRoot(dir string) (modRoot, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return modRoot{}, false
	}
	for d := abs; ; {
		data, err := os.ReadFile(filepath.Join(d, "xo.mod"))
		if err == nil {
			r := modRoot{dir: d, edition: CurrentEdition, gc: NoGCConfig}
			sc := bufio.NewScanner(bytes.NewReader(data))
			for sc.Scan() {
				fields := strings.Fields(sc.Text())
				if len(fields) == 2 && fields[0] == "module" && r.path == "" {
					r.path = fields[1]
				}
				if len(fields) >= 1 && fields[0] == "gc" {
					if err := r.gc.parseGCLine(fields[1:]); err != nil && c.modErr == nil {
						c.modErr = fmt.Errorf("%s: %v", filepath.Join(d, "xo.mod"), err)
					}
				}
				if len(fields) >= 1 && fields[0] == "edition" {
					arg := ""
					if len(fields) == 2 {
						arg = fields[1]
					}
					e, err := parseEdition(arg)
					if err != nil && c.modErr == nil {
						c.modErr = fmt.Errorf("%s: %v", filepath.Join(d, "xo.mod"), err)
					}
					if err == nil {
						r.edition = e
					}
				}
			}
			if r.path == "" {
				return modRoot{}, false
			}
			known := false
			for _, x := range c.roots {
				if x == r {
					known = true
				}
			}
			if !known {
				c.roots = append(c.roots, r)
			}
			return r, true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return modRoot{}, false
		}
		d = parent
	}
}

// modulePathFor returns the import path of the module in dir.
func (c *checker) modulePathFor(dir string) string {
	abs, _ := filepath.Abs(dir)
	if r, ok := c.findModRoot(dir); ok {
		rel, err := filepath.Rel(r.dir, abs)
		if err == nil {
			if rel == "." {
				return r.path
			}
			return r.path + "/" + filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(filepath.Clean(dir))
}

func (c *checker) loadDir(dir string) (*Module, error) {
	dir = relDir(dir)
	path := c.modulePathFor(dir)
	if m, ok := c.modules[path]; ok {
		return m, nil
	}
	name, _ := localName(path)
	m := &Module{Path: path, Name: name, Dir: dir, Scope: map[string]Object{}, Edition: CurrentEdition, GC: NoGCConfig}
	if r, ok := c.findModRoot(dir); ok {
		m.Edition = r.edition
		m.GC = r.gc
		m.Dependency = c.dependencyOf(r.dir)
	}
	c.modules[path] = m
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".xo") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		p := filepath.Join(dir, n)
		src, err := c.readSource(p)
		if err != nil {
			return nil, err
		}
		m.Files = append(m.Files, c.parseFile(m, p, src, 0))
	}
	m.state = 1
	if err := c.loadImports(m); err != nil {
		return nil, err
	}
	m.state = 2
	c.order = append(c.order, m)
	return m, nil
}

// loadImports resolves each file's `use` declarations, loading modules as
// needed. Cycles are XO0604.
func (c *checker) loadImports(m *Module) error {
	for _, f := range m.Files {
		for _, u := range f.AST.Uses {
			if u.Path == "" {
				continue
			}
			imp := &Import{Decl: u}
			if u.Go {
				c.loadGoImport(m, f, u, imp)
				continue
			}
			if u.C {
				c.loadCImport(m, f, u, imp)
				continue
			}
			if u.Alias != nil {
				imp.Name = u.Alias.Name
			} else {
				name, ok := localName(u.Path)
				imp.Name = name
				if !ok {
					d := c.errorf(diag.ImportNeedsAlias, u.Sp, "import `%s` needs `as`: `%s` is not a valid module name", u.Path, name)
					if d != nil {
						alt := toSnake(name)
						if _, ok2 := localName(alt); !ok2 || alt == "" {
							alt = "m_" + strings.Trim(toSnake(name), "_")
						}
						note(d, token.Span{}, "the local name is the last path segment with `-` replaced by `_` (core 4.1)")
						fix(d, fmt.Sprintf("add `as %s`", alt), insertEdit(u.Sp.File, u.Sp.End, " as "+alt))
					}
					imp.Name = ""
				}
			}
			dep, err := c.resolveImport(u.Path)
			if strings.HasPrefix(u.Path, "native/") {
				// the intrinsics of the native backends: only their
				// implementation modules see them (decision 0074), and
				// other intrinsics modules (an Any of native/rt)
				dep, err = nil, nil
				if m.Native || (m.IsStd && strings.HasPrefix(m.Path, "native/")) {
					dep, err = c.loadNativeRT(u.Path)
				}
			}
			if err != nil {
				return err
			}
			if dep != nil && !m.IsStd {
				c.checkDeny(m, u)
			}
			if dep == nil && c.unrequired(u) {
				imp.Used = true
			} else if dep == nil {
				d := c.errorf(diag.UnresolvedName, u.Sp, "module `%s` not found", u.Path)
				if strings.HasPrefix(u.Path, "std/") {
					var cands []string
					entries, _ := fs.ReadDir(std.Files, ".")
					for _, e := range entries {
						n := strings.TrimSuffix(e.Name(), ".xo")
						if n != "prelude" && strings.HasSuffix(e.Name(), ".xo") {
							cands = append(cands, "std/"+n)
						}
					}
					if s := nearest(u.Path, cands); len(s) > 0 {
						note(d, token.Span{}, "did you mean `%s`?", strings.Join(s, "`, `"))
					}
				}
				imp.Used = true
			} else if dep.state == 1 {
				c.errorf(diag.ImportCycle, u.Sp, "import cycle: `%s` imports `%s`, which imports it back", m.Path, dep.Path)
				imp.Used = true
				dep = nil
			} else if !m.IsStd {
				c.checkRequired(m, u)
			}
			imp.Module = dep
			if imp.Name == "" {
				imp.Used = true
				continue
			}
			if prev, ok := f.Imports[imp.Name]; ok {
				c.errorf(diag.Duplicate, u.Sp, "`%s` is already imported by `use %s`", imp.Name, prev.Decl.Path)
				continue
			}
			f.Imports[imp.Name] = imp
		}
	}
	return nil
}

func (c *checker) resolveImport(path string) (*Module, error) {
	if strings.HasPrefix(path, "std/") {
		return c.loadStd(strings.TrimPrefix(path, "std/"))
	}
	if m, ok := c.modules[path]; ok {
		return m, nil
	}
	if r, ok := c.ownerRoot(path); ok {
		rel := strings.TrimPrefix(strings.TrimPrefix(path, r.path), "/")
		dir := filepath.Join(r.dir, filepath.FromSlash(rel))
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return c.loadDir(dir)
		}
	}
	return nil, nil
}

// checkUnusedImports reports XO0110 for imports never referenced.
func (c *checker) checkUnusedImports(f *File) {
	for _, u := range f.AST.Uses {
		for _, imp := range f.Imports {
			if imp.Decl != u || imp.Used {
				continue
			}
			d := c.errorf(diag.UnusedBinding, u.Sp, "import `%s` is never used", u.Path)
			fix(d, "remove the import", replaceEdit(u.Sp, ""))
		}
	}
}

// relDir makes an absolute directory relative to the working directory when
// it lies below it, so diagnostics use the same paths however a module was
// reached (named on the command line or imported through xo.mod).
func relDir(dir string) string {
	if !filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}
	wd, err := os.Getwd()
	if err != nil {
		return dir
	}
	rel, err := filepath.Rel(wd, dir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return dir
	}
	return rel
}

// loadGoImport resolves `use go "<path>"` (decision 0041): it loads the
// cached binding of the Go package, binding it first when the cache has
// none (internal/gobind), and loads the declaration file as a std like
// module whose functions carry the `go` effect.
func (c *checker) loadGoImport(m *Module, f *File, u *ast.UseDecl, imp *Import) {
	key := "go:" + u.Path
	if c.goFailed[key] {
		imp.Used = true
		return
	}
	dep, ok := c.modules[key]
	if !ok {
		root := m.Dir
		if r, found := c.findModRoot(m.Dir); found {
			root = c.goBindRoot(r.dir)
		}
		pins := gobind.ReadPins(filepath.Join(root, "xo.mod"))
		opts := gobind.Options{Dir: gobind.CacheDir(root), Version: gobind.PinFor(pins, u.Path), Rerun: gobind.ReadRerun(filepath.Join(root, "xo.mod"))[u.Path]}
		b, err := gobind.LoadOrBind(u.Path, opts)
		if err != nil {
			d := c.errorf(diag.GoBindFailed, u.Sp, "cannot bind Go package `%s`: %s", u.Path, gobindMsg(err))
			note(d, token.Span{}, "`use go` binds the package with `xo bind %s` into %s (decision 0041)", u.Path, opts.Dir)
			if c.goFailed == nil {
				c.goFailed = map[string]bool{}
			}
			c.goFailed[key] = true
			imp.Used = true
			return
		}
		dep = &Module{Path: key, Name: b.Name, Dir: filepath.Dir(b.DeclPath), IsStd: true, Go: b, Scope: map[string]Object{}}
		c.modules[key] = dep
		df := c.parseFile(dep, b.DeclPath, b.Decl, parser.SignatureMode)
		dep.Files = append(dep.Files, df)
		dep.state = 1
		if err := c.loadImports(dep); err != nil {
			c.errorf(diag.GoBindFailed, u.Sp, "cannot load the binding of Go package `%s`: %v", u.Path, err)
		}
		dep.state = 2
		c.order = append(c.order, dep)
	}
	imp.Module = dep
	if u.Alias != nil {
		imp.Name = u.Alias.Name
	} else {
		imp.Name = dep.Name
		if _, ok := localName(dep.Name); !ok {
			d := c.errorf(diag.ImportNeedsAlias, u.Sp, "import of Go package `%s` needs `as`: its package name `%s` is not a valid module name", u.Path, dep.Name)
			fix(d, "add `as go_"+dep.Name+"`", insertEdit(u.Sp.File, u.Sp.End, " as go_"+dep.Name))
			imp.Used = true
			return
		}
	}
	if prev, ok := f.Imports[imp.Name]; ok {
		c.errorf(diag.Duplicate, u.Sp, "`%s` is already imported by `use %s`", imp.Name, prev.Decl.Path)
		return
	}
	f.Imports[imp.Name] = imp
}

func gobindMsg(err error) string {
	var be *gobind.Error
	if errors.As(err, &be) {
		return be.Msg
	}
	return err.Error()
}

// goSkipped reports XO1102 when name was skipped by the binding of Go
// module m (qual is "name" or "Type.name"). It returns true when it
// reported.
func (c *checker) goSkipped(m *Module, qual string, sp token.Span) bool {
	if m == nil || m.Go == nil {
		return false
	}
	reason, ok := m.Go.Skipped[qual]
	if !ok {
		return false
	}
	d := c.errorf(diag.GoNameSkipped, sp, "Go `%s` in `%s` is not bound: %s", qual, m.Go.GoPath, reason)
	note(d, token.Span{}, "the binding lists skipped names at the top of %s; write a Go wrapper with a mappable signature or use another function", m.Go.DeclPath)
	return true
}
