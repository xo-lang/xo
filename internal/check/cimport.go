package check

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/cbind"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/parser"
	"github.com/xo-lang/xo/internal/token"
)

// loadCImport resolves `use c "<header>"` (decision 0113): it binds the
// header (internal/cbind) and loads the declaration file as a std like
// module whose functions perform `ffi`, or the effects of the line's
// `uses`. Without a native backend the line is XO1201: C code needs a
// native build.
func (c *checker) loadCImport(m *Module, f *File, u *ast.UseDecl, imp *Import) {
	if !c.native {
		d := c.errorf(diag.NoGCUnavailable, u.Sp, "`use c \"%s\"` is impossible on the Go backend: C code is called from native builds only", u.Path)
		note(d, token.Span{}, "check and build with `--no-gc` (a native backend: arm64 or LLVM); the Go backend reaches Go packages with `use go` (decision 0113)")
	}
	var uses []string
	if u.Uses != nil {
		uses = []string{}
		for _, e := range u.Uses {
			n := e.Name.Name
			if e.Sub != nil {
				n += "." + e.Sub.Name
			}
			if !effectLabels[n] || n == "go" {
				d := c.errorf(diag.CBindFailed, e.Sp, "unknown effect `%s` in the `uses` of `use c \"%s\"`", n, u.Path)
				note(d, token.Span{}, "the effects are the labels of core 6.1 (`fs.read`, `net`, `clock`, ...), or `none`")
				imp.Used = true
				return
			}
			uses = append(uses, n)
		}
	}
	root := m.Dir
	if r, found := c.findModRoot(m.Dir); found {
		root = r.dir
	}
	opts := cbind.Options{Header: u.Path, ModDir: m.Dir, Links: u.Links, Uses: uses, Dir: cbind.CacheDir(root)}
	b, err := cbind.Bind(opts)
	if err != nil {
		msg := err.Error()
		var be *cbind.Error
		if errors.As(err, &be) {
			msg = be.Msg
		}
		d := c.errorf(diag.CBindFailed, u.Sp, "cannot bind C header `%s`: %s", u.Path, msg)
		note(d, token.Span{}, "`use c` reads the header with clang (`xo bind --c %s` runs this step alone); a header path is relative to the module directory or on the C include path (decision 0113)", u.Path)
		imp.Used = true
		return
	}
	dep, ok := c.modules[b.Key]
	if !ok {
		dep = &Module{Path: b.Key, Name: b.Name, Dir: filepath.Dir(b.DeclPath), IsStd: true, C: b, Scope: map[string]Object{}}
		c.modules[b.Key] = dep
		df := c.parseFile(dep, b.DeclPath, b.Decl, parser.SignatureMode)
		dep.Files = append(dep.Files, df)
		dep.state = 1
		if err := c.loadImports(dep); err != nil {
			c.errorf(diag.CBindFailed, u.Sp, "cannot load the binding of C header `%s`: %v", u.Path, err)
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
			d := c.errorf(diag.ImportNeedsAlias, u.Sp, "import of C header `%s` needs `as`: `%s` is not a valid module name", u.Path, dep.Name)
			fix(d, "add `as c_"+dep.Name+"`", insertEdit(u.Sp.File, u.Sp.End, " as c_"+dep.Name))
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

// cSkipped reports XO1105 when name was skipped by the binding of C
// module m. It returns true when it reported.
func (c *checker) cSkipped(m *Module, name string, sp token.Span) bool {
	if m == nil || m.C == nil {
		return false
	}
	reason, ok := m.C.Skipped[name]
	if !ok {
		return false
	}
	d := c.errorf(diag.CNameSkipped, sp, "C function `%s` of `%s` is not bound: %s", name, m.C.Header, strings.TrimSpace(reason))
	note(d, token.Span{}, "the binding lists skipped names at the top of %s; write a C wrapper with a signature that maps (scalars, const char *, a const byte pointer and a length, flat structs, opaque struct pointers) and bind its header", m.C.DeclPath)
	return true
}

// CModules are the modules of the checked program bound from C headers
// with `use c`, in load order (internal/build links their adapters).
func (r *Result) CModules() []*Module {
	if r == nil || r.c == nil {
		return nil
	}
	var out []*Module
	for _, m := range r.c.order {
		if m.C != nil {
			out = append(out, m)
		}
	}
	return out
}
