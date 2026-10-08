package gen

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// SafeIntOps runs the range analysis of ranges.go on the body of a
// declared function for another backend (internal/llvm, the P3 prototype)
// and returns the Int operations it proved cannot fault: binary and
// compound assignment `+ - * / %`, unary minus, and List indexing, keyed by
// their syntax nodes. Operations inside closure literals of the body are
// included when the analysis models them. The result does not depend on
// the Go lowering; the analysis only asks which types are Go int64.
func SafeIntOps(info *check.Info, fo *check.FuncObj) map[ast.Node]bool {
	if fo == nil || fo.Decl == nil || fo.Decl.Body == nil {
		return nil
	}
	g := &gen{info: info, pkgs: map[*check.Module]*pkgGen{}, modRoot: map[string]string{}, prelude: info.Prelude()}
	g.initAdapt()
	pg := g.newPkg(fo.Module)
	fg := pg.newFn(fo)
	var fixed, byRef []*check.VarObj
	if rv := info.RecvVar(fo); rv != nil {
		if fo.RecvVar {
			byRef = append(byRef, rv)
		} else {
			fixed = append(fixed, rv)
		}
	}
	for i, pv := range info.ParamVars(fo) {
		if i < len(fo.Params) && fo.Params[i].Var {
			byRef = append(byRef, pv)
		} else {
			fixed = append(fixed, pv)
		}
	}
	fg.analyzeRanges(fo.Decl.Body, fixed, byRef)
	out := map[ast.Node]bool{}
	if fg.rng != nil {
		for n, ok := range fg.rng.safe {
			if ok {
				out[n] = true
			}
		}
	}
	return out
}
