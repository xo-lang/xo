package check

import (
	"sort"

	"github.com/xo-lang/xo/internal/ast"
)

// Accessors for the tools built on a check run (`xo vet`, `xo query`,
// `xo doc`). Like info.go they only read what the checker resolved.

// Inspect calls f for n and every expression, statement, and declaration
// below it in source order. If f returns false the children of that node
// are skipped. Types and patterns are not visited.
func Inspect(n ast.Node, f func(ast.Node) bool) { inspect(n, f) }

// EffectUse is one effect a function body performs (a label such as
// `fs.read`, or an effect parameter such as `fx`) with the call chain from
// the body to the place that performs it.
type EffectUse struct {
	Effect string
	Param  bool // an effect parameter rather than a label
	Chain  []EffectStep
}

// Performed returns the effects the body of f performs, sorted by name,
// each with its call chain (core 6.2). It is nil for a function without a
// checked body.
func (i *Info) Performed(f *FuncObj) []EffectUse {
	var fl *flatEff
	switch {
	case f.Declared == nil:
		fl = i.c.inferred[f]
	case f.body != nil:
		fl = flatten(f.body, i.c.inferred)
	}
	if fl == nil {
		return nil
	}
	var out []EffectUse
	for l, w := range fl.labels {
		out = append(out, EffectUse{Effect: l, Chain: chainSteps(w, l, i.c.inferred)})
	}
	for p, w := range fl.params {
		out = append(out, EffectUse{Effect: p.Name, Param: true, Chain: chainSteps(w, p.Name, i.c.inferred)})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Effect < out[b].Effect })
	return out
}

// Funcs returns the functions and methods declared in m's files, in source
// order (interface methods excluded).
func (i *Info) Funcs(m *Module) []*FuncObj {
	var out []*FuncObj
	for _, f := range m.Files {
		for _, d := range f.AST.Decls {
			if fd, ok := d.(*ast.FnDecl); ok {
				if fo := i.FuncOf(m, fd); fo != nil {
					out = append(out, fo)
				}
			}
		}
	}
	return out
}
