package check

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/token"
	"github.com/xo-lang/xo/std"
)

// denyRules are the `deny` lines of one main module's xo.mod (decision
// 0095).
type denyRules struct {
	file  string // xo.mod, as diagnostics name it
	rules []mod.Deny
}

// loadDenies reads the deny rules of the main module at root and reports
// a rule naming a package that does not exist as XO1301 on its line.
// Malformed deny lines are reported by module loading.
func (c *checker) loadDenies(root modRoot) {
	if c.deps.denies == nil {
		c.deps.denies = map[string]*denyRules{}
	}
	f, _ := mod.ReadFile(root.dir)
	if f == nil || len(f.Denies) == 0 {
		return
	}
	dr := &denyRules{file: relDir(f.File), rules: f.Denies}
	c.deps.denies[root.dir] = dr
	for _, r := range f.Denies {
		for _, pat := range append([]string{r.From}, r.To...) {
			if c.patternExists(pat) {
				continue
			}
			pos := token.Pos{Line: r.Line, Col: 1}
			d := c.errorf(diag.ModFileInvalid, token.Span{File: dr.file, Start: pos, End: pos}, "deny rule names `%s`, which is not a package of this module, of a required module, or of std", pat)
			note(d, token.Span{}, "a pattern is a package path, or a path ending in `/...` for every package under it (decision 0095)")
		}
	}
}

// patternExists reports whether a deny pattern names at least one
// package directory: of std, of the main module, or of a required module.
func (c *checker) patternExists(pat string) bool {
	base := mod.PatternBase(pat)
	wild := base != pat
	if base == "std" {
		return wild
	}
	if rest, ok := strings.CutPrefix(base, "std/"); ok {
		if _, err := fs.Stat(std.Files, rest+".xo"); err == nil {
			return true
		}
		if wild {
			entries, _ := fs.ReadDir(std.Files, ".")
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), rest+"/") {
					return true
				}
			}
			if info, err := fs.Stat(std.Files, rest); err == nil && info.IsDir() {
				return true
			}
		}
		return false
	}
	for p := range c.deps.failed {
		if base == p || strings.HasPrefix(base, p+"/") {
			return true // already reported on its require line
		}
	}
	owner, ok := c.ownerRoot(base)
	if !ok {
		if !wild {
			return false
		}
		// a pattern above a module path: `example.com/...`
		for _, r := range c.roots {
			if strings.HasPrefix(r.path, base+"/") {
				return true
			}
		}
		return false
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(base, owner.path), "/")
	dir := filepath.Join(owner.dir, filepath.FromSlash(rel))
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	if wild {
		return true
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".xo") {
			return true
		}
	}
	return false
}

// checkDeny reports XO1312 when a deny rule of m's main module forbids
// the import u. Each direct `use` is checked, used or not; the first rule
// that forbids it is named.
func (c *checker) checkDeny(m *Module, u *ast.UseDecl) {
	if len(c.deps.denies) == 0 || m.IsStd || m.Dependency != "" {
		return
	}
	root, ok := c.findModRoot(m.Dir)
	if !ok {
		return
	}
	dr := c.deps.denies[root.dir]
	if dr == nil {
		return
	}
	// Match the path as loaded: `use a/./b` is `a/b` (core 4.1).
	p := u.Path
	if !u.Go && !u.C {
		p = path.Clean(p)
	}
	for _, r := range dr.rules {
		t := r.Denies(m.Path, p)
		if t == "" {
			continue
		}
		d := c.errorf(diag.ImportDenied, u.Sp, "`%s` may not import `%s`: denied by `%s` (xo.mod line %d)", m.Path, p, r.String(), r.Line)
		pos := token.Pos{Line: r.Line, Col: 1}
		if t != p {
			note(d, token.Span{File: dr.file, Start: pos, End: pos}, "the rule; `%s` matches `%s` (decision 0095)", t, p)
		} else {
			note(d, token.Span{File: dr.file, Start: pos, End: pos}, "the rule (decision 0095)")
		}
		note(d, token.Span{}, "move what %s needs into a package it may import, or change the rule; `xo query imports %s` lists its imports", m.Path, m.Path)
		return
	}
}
