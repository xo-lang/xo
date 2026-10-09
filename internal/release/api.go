// Package release compares the exported API and effects of two versions
// of an Xo module (decision 0086 step 4): `xo publish --check` rejects a
// minor or patch release that removes or changes an export or adds an
// effect to one, and `xo get` prints what an upgrade changes.
package release

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/format"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/parser"
	"github.com/xo-lang/xo/internal/query"
)

// Item is one export of a module version: a pub declaration of one of
// its packages, as `xo query api` lists it.
type Item struct {
	Package   string   `json:"package"`
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Signature string   `json:"signature"`
	Effects   []string `json:"effects,omitempty"` // a function's `uses` clause
	File      string   `json:"file"`
	Line      int      `json:"line"`
	// shape is the signature without the `uses` clause.
	shape string
}

func (it Item) key() string { return it.Package + "." + it.Name }

// ModuleAPI lists the exports of the module in dir (module path modPath):
// the pub declarations of every package under dir, outside test files,
// from the syntax tree (no dependencies are needed). Nested modules,
// vendor/, testdata/, and dot directories are skipped.
func ModuleAPI(dir, modPath string) ([]Item, error) {
	var out []Item
	seen := map[string]bool{}
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
		pkg := modPath
		if rel != "." {
			pkg += "/" + filepath.ToSlash(rel)
		}
		for _, decl := range f.Decls {
			def, ok := query.DeclSurface(pkg, src, decl, false)
			if !ok {
				continue
			}
			it := Item{Package: pkg, Kind: def.Kind, Name: def.Name, Signature: def.Signature, shape: def.Signature, File: p, Line: def.Pos.Line}
			if fd, ok := decl.(*ast.FnDecl); ok {
				it.Effects, it.shape = fnEffects(fd, src)
			}
			// a declaration repeated in platform files counts once
			if seen[it.key()] {
				continue
			}
			seen[it.key()] = true
			out = append(out, it)
		}
		return nil
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].key() < out[j].key() })
	return out, err
}

// fnEffects returns the effects of a function's `uses` clause and its
// signature without the clause.
func fnEffects(fd *ast.FnDecl, src []byte) ([]string, string) {
	c := *fd
	c.Clauses = nil
	var effs []string
	for _, cl := range fd.Clauses {
		u, ok := cl.(*ast.UsesClause)
		if !ok {
			c.Clauses = append(c.Clauses, cl)
			continue
		}
		for _, e := range u.Effects {
			name := e.Name.Name
			if e.Sub != nil {
				name += "." + e.Sub.Name
			}
			effs = append(effs, name)
		}
	}
	sort.Strings(effs)
	return effs, format.Decl(&c, src, true)
}

// Change is one difference between the exports of two versions.
type Change struct {
	// Kind is removed, added, changed (signature or declaration kind),
	// effects_added, or effects_removed.
	Kind     string   `json:"kind"`
	Package  string   `json:"package"`
	Name     string   `json:"name"`
	ItemKind string   `json:"item_kind"`
	Old      string   `json:"old,omitempty"` // old signature
	New      string   `json:"new,omitempty"` // new signature
	Effects  []string `json:"effects,omitempty"`
	Breaking bool     `json:"breaking"`
	File     string   `json:"file,omitempty"` // the new declaration
	Line     int      `json:"line,omitempty"`
}

// Diff compares the exports of an old and a new version. A removed
// export, a changed signature, and an effect added to a function are
// breaking; an added export and a removed effect are not.
func Diff(old, new []Item) []Change {
	oldBy := map[string]Item{}
	for _, it := range old {
		oldBy[it.key()] = it
	}
	newBy := map[string]Item{}
	for _, it := range new {
		newBy[it.key()] = it
	}
	var out []Change
	for _, o := range old {
		n, ok := newBy[o.key()]
		if !ok {
			out = append(out, Change{Kind: "removed", Package: o.Package, Name: o.Name, ItemKind: o.Kind, Old: o.Signature, Breaking: true})
			continue
		}
		ch := Change{Package: n.Package, Name: n.Name, ItemKind: n.Kind, Old: o.Signature, New: n.Signature, File: n.File, Line: n.Line}
		if n.Kind != o.Kind || n.shape != o.shape {
			ch.Kind, ch.Breaking = "changed", true
			ch.Effects = minus(n.Effects, o.Effects)
			out = append(out, ch)
			continue
		}
		if added := minus(n.Effects, o.Effects); len(added) > 0 {
			ch.Kind, ch.Breaking, ch.Effects = "effects_added", true, added
			out = append(out, ch)
			continue
		}
		if removed := minus(o.Effects, n.Effects); len(removed) > 0 {
			ch.Kind, ch.Effects = "effects_removed", removed
			out = append(out, ch)
		}
	}
	for _, n := range new {
		if _, ok := oldBy[n.key()]; !ok {
			out = append(out, Change{Kind: "added", Package: n.Package, Name: n.Name, ItemKind: n.Kind, New: n.Signature, File: n.File, Line: n.Line})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Package != out[j].Package {
			return out[i].Package < out[j].Package
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// minus lists the effects of a not admitted by b (`fs` admits `fs.read`).
func minus(a, b []string) []string {
	c := &mod.Ceiling{Effects: b}
	var out []string
	for _, e := range a {
		if !c.Admits(e) {
			out = append(out, e)
		}
	}
	return out
}

// Bump is the smallest version bump the changes need: major when one
// breaks, minor when one adds, patch otherwise.
func Bump(changes []Change) string {
	bump := "patch"
	for _, c := range changes {
		if c.Breaking {
			return "major"
		}
		bump = "minor"
	}
	return bump
}

// Describe is a one line description of a change for text output.
func (c Change) Describe() string {
	name := c.Package + "." + c.Name
	switch c.Kind {
	case "removed":
		return "removed " + c.ItemKind + " " + name
	case "added":
		return "added " + c.ItemKind + " " + name
	case "effects_added":
		return c.ItemKind + " " + name + " now uses " + strings.Join(c.Effects, ", ")
	case "effects_removed":
		return c.ItemKind + " " + name + " no longer uses " + strings.Join(c.Effects, ", ")
	}
	s := "changed " + c.ItemKind + " " + name
	if len(c.Effects) > 0 {
		s += " (now also uses " + strings.Join(c.Effects, ", ") + ")"
	}
	return s
}
