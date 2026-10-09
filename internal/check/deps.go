package check

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/token"
)

// depState is what the checker knows about the dependencies of the main
// modules it loaded (internal/mod, decision 0086).
type depState struct {
	// resolved holds the xo.mod directories whose requirements were
	// resolved (main modules).
	resolved map[string]bool
	// depDirs holds the directories of dependency modules (cache,
	// vendor/, or workspace modules other than the main one).
	depDirs map[string]mod.Module
	// failed holds module paths with a module problem: imports of their
	// packages are not reported again.
	failed map[string]bool
	// mainDir is the first main module's directory (the `use go`
	// binding cache of dependencies lives there).
	mainDir string
	lists   []*mod.BuildList
	// requires caches the Xo requirements of xo.mod directories.
	requires map[string]map[string]bool
	// denies holds the deny rules of main modules, by xo.mod directory
	// (decision 0095).
	denies map[string]*denyRules
}

// resolveDeps resolves the requirements of the main module at root and
// registers the selected module directories as roots for imports. Module
// problems become diagnostics on their xo.mod, xo.lock, or xo.work line.
func (c *checker) resolveDeps(root modRoot) {
	d := &c.deps
	if d.resolved == nil {
		d.resolved, d.depDirs, d.failed = map[string]bool{}, map[string]mod.Module{}, map[string]bool{}
	}
	if _, dep := d.depDirs[root.dir]; d.resolved[root.dir] || dep {
		return
	}
	d.resolved[root.dir] = true
	if d.mainDir == "" {
		d.mainDir = root.dir
	}
	bl := mod.Load(root.dir, mod.Options{})
	d.lists = append(d.lists, bl)
	for _, p := range bl.Problems {
		c.modProblem(p)
	}
	for _, m := range bl.Modules {
		d.depDirs[m.Dir] = m
		c.findModRoot(m.Dir)
		if m.Workspace && m.Dir != root.dir {
			// a workspace module is checked as part of the program:
			// its own deny rules apply to its packages
			c.loadDenies(modRoot{dir: m.Dir})
		}
	}
	c.loadDenies(root)
}

// modProblem reports a module problem as a diagnostic.
func (c *checker) modProblem(p *mod.Problem) {
	if p.Module != "" {
		c.deps.failed[p.Module] = true
	}
	line := p.Line
	if line < 1 {
		line = 1
	}
	file := p.File
	if file != "" {
		file = relDir(file)
	}
	pos := token.Pos{Line: line, Col: 1}
	dg := c.errorf(p.Code, token.Span{File: file, Start: pos, End: pos}, "%s", p.Msg)
	for _, n := range p.Notes {
		note(dg, token.Span{}, "%s", n)
	}
}

// ownerRoot returns the registered module root that holds an import
// path: the longest module path that is the path or a prefix of it.
func (c *checker) ownerRoot(path string) (modRoot, bool) {
	var best modRoot
	ok := false
	for _, r := range c.roots {
		if (path == r.path || strings.HasPrefix(path, r.path+"/")) && len(r.path) > len(best.path) {
			best, ok = r, true
		}
	}
	return best, ok
}

// unrequired reports an import of another module's package that no
// requirement covers (XO1306). It returns false when the path is not
// such an import (the caller reports XO0601).
func (c *checker) unrequired(u *ast.UseDecl) bool {
	if !mod.IsRemote(u.Path) {
		return false
	}
	if _, ok := c.ownerRoot(u.Path); ok {
		return false
	}
	for p := range c.deps.failed {
		if u.Path == p || strings.HasPrefix(u.Path, p+"/") {
			return true // already reported on the xo.mod line
		}
	}
	modPath := u.Path
	if elems := strings.Split(u.Path, "/"); len(elems) > 3 && (elems[0] == "github.com" || elems[0] == "gitlab.com" || elems[0] == "bitbucket.org") {
		modPath = strings.Join(elems[:3], "/")
	}
	d := c.errorf(diag.ModuleNotRequired, u.Sp, "module of `%s` is not required by xo.mod", u.Path)
	note(d, token.Span{}, "`xo get %s` requires its latest version, `xo mod tidy` requires every imported module (core 4.2)", modPath)
	return true
}

// checkRequired reports XO1306 when module m imports a package of a
// required module version that m's own xo.mod does not require: the
// module is only in the build list because another module needs it, so
// its version is not m's choice (an upgrade of that other module could
// drop it). Workspace modules and the importer's own module are exempt.
func (c *checker) checkRequired(m *Module, u *ast.UseDecl) {
	if !mod.IsRemote(u.Path) || c.deps.depDirs == nil {
		return
	}
	owner, ok := c.ownerRoot(u.Path)
	if !ok {
		return
	}
	dm, isDep := c.deps.depDirs[owner.dir]
	if !isDep || dm.Workspace {
		return
	}
	from, ok := c.findModRoot(m.Dir)
	if !ok || from.dir == owner.dir {
		return
	}
	if c.deps.requires == nil {
		c.deps.requires = map[string]map[string]bool{}
	}
	reqs, ok := c.deps.requires[from.dir]
	if !ok {
		reqs = map[string]bool{}
		if f, _ := mod.ReadFile(from.dir); f != nil {
			for _, r := range f.Requires {
				reqs[r.Path] = true
			}
		}
		c.deps.requires[from.dir] = reqs
	}
	if reqs[owner.path] {
		return
	}
	d := c.errorf(diag.ModuleNotRequired, u.Sp, "module %s of `%s` is not required by %s (it is in the build only because another module requires it)", owner.path, u.Path, filepath.Join(relDir(from.dir), "xo.mod"))
	note(d, token.Span{}, "`xo get %s` (or `xo mod tidy`) requires the version the build selects (core 4.2)", owner.path)
}

// goBindRoot is the directory whose binding cache and `require go` pins
// a `use go` import in a module at root uses: the main module's for a
// dependency (the module cache is read only).
func (c *checker) goBindRoot(root string) string {
	if m, ok := c.deps.depDirs[root]; ok && !m.Workspace && c.deps.mainDir != "" {
		return c.deps.mainDir
	}
	return root
}

// Deps is the build list of the first main module checked (nil when it
// has no requirements and no workspace).
func (r *Result) Deps() *mod.BuildList {
	if r == nil || r.c == nil || len(r.c.deps.lists) == 0 {
		return nil
	}
	return r.c.deps.lists[0]
}

// isVendorDir reports whether dir is a vendor/ directory written by
// `xo vendor` (holding modules.txt): its modules are dependencies, not
// modules of the program named on the command line.
func isVendorDir(dir string) bool {
	if filepath.Base(dir) != "vendor" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, "modules.txt"))
	return err == nil
}

// dependencyOf is "<path>@<version>" when the module root at dir is a
// required module version (not a workspace module), else "".
func (c *checker) dependencyOf(dir string) string {
	if m, ok := c.deps.depDirs[dir]; ok && !m.Workspace {
		return m.Path + "@" + m.Version
	}
	return ""
}
