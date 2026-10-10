package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
)

// ---------------------------------------------------------------------------
// imports and deps (decision 0095)

// ImportEdge is one `use` line: the importing package, the imported
// path, and where the line is.
type ImportEdge struct {
	From string `json:"from"` // importing package
	Path string `json:"path"` // imported path as written
	Pos  Pos    `json:"pos"`  // the `use` line
	Std  bool   `json:"std,omitempty"`
	Go   bool   `json:"go,omitempty"`   // `use go "..."` (decision 0041)
	C    bool   `json:"c,omitempty"`    // `use c "<header>"` (decision 0116)
	Test bool   `json:"test,omitempty"` // in a _test.xo file
	// Missing is set when the path resolves to no package.
	Missing bool `json:"missing,omitempty"`
	// Denied is the XO1312 message when a deny rule of xo.mod forbids
	// the line.
	Denied string `json:"denied,omitempty"`
}

// ImportsResult is the answer of `xo query imports` and `xo query deps`.
type ImportsResult struct {
	Package    string       `json:"package"`
	Transitive bool         `json:"transitive"`
	Imports    []ImportEdge `json:"imports"`
	// Packages are the distinct imported paths, sorted (for deps: every
	// package reached).
	Packages []string `json:"packages"`
}

// Imports lists the `use` lines of the package with the given import
// path or directory. With transitive set (`deps`), it follows every
// imported package that is not std or a Go binding and lists their `use`
// lines too.
func (s *Session) Imports(path string, transitive bool) (*ImportsResult, error) {
	m := s.ModuleByPath(path)
	if m == nil || m.IsStd {
		return nil, fmt.Errorf("no package %q among the checked modules", path)
	}
	denied := map[string]string{}
	for _, d := range s.Res.Diagnostics {
		if d.Code == string(diag.ImportDenied) {
			denied[fmt.Sprintf("%s:%d:%d", d.Span.File, d.Span.Line, d.Span.Col)] = d.Message
		}
	}
	res := &ImportsResult{Package: m.Path, Transitive: transitive, Imports: []ImportEdge{}, Packages: []string{}}
	seen := map[string]bool{m.Path: true}
	reached := map[string]bool{}
	queue := []*check.Module{m}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		files := append([]*check.File(nil), cur.Files...)
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
		for _, f := range files {
			for _, u := range f.AST.Uses {
				if u.Path == "" {
					continue
				}
				e := ImportEdge{From: cur.Path, Path: u.Path, Pos: posOf(u.Sp), Go: u.Go, C: u.C, Test: f.IsTest,
					Std: !u.Go && !u.C && strings.HasPrefix(u.Path, "std/")}
				e.Denied = denied[e.Pos.String()]
				var dep *check.Module
				if !u.Go && !u.C {
					dep = s.Info.Module(u.Path)
					e.Missing = dep == nil
				}
				res.Imports = append(res.Imports, e)
				reached[u.Path] = true
				if transitive && dep != nil && !dep.IsStd && !seen[dep.Path] {
					seen[dep.Path] = true
					queue = append(queue, dep)
				}
			}
		}
		if !transitive {
			break
		}
	}
	for p := range reached {
		res.Packages = append(res.Packages, p)
	}
	sort.Strings(res.Packages)
	return res, nil
}
