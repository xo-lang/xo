package gen

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Contract fuzzing (core 9.1): `xo test` runs every function with contracts
// on generated inputs that satisfy its `requires` clauses; a fault (a
// failed `ensures`, an overflow, an index out of range) is a failure.
// Capability parameters receive the test's fakes. Functions whose inputs
// cannot be generated (function values, type parameters, interfaces,
// handles, std types) are not fuzzed.

// capField is the TestOs field that provides a capability parameter, or "".
func (g *gen) capField(t check.Type) string {
	n := g.named(t)
	if n == nil || !n.Obj.Module.IsPrelude {
		return ""
	}
	switch n.Obj.Name {
	case "Net", "Fs", "Clock", "Rand", "Env", "Proc", "Stdio":
		return "F_" + lowerFirst(n.Obj.Name)
	case "Os":
		return "os"
	}
	return ""
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]+'a'-'A') + s[1:]
}

// generatable reports whether rt.Gen can make values of type t.
func (g *gen) generatable(t check.Type, seen map[*check.TypeObj]bool) bool {
	switch x := check.Resolve(t).(type) {
	case *check.Tuple:
		if len(x.Elems) > 4 {
			return false
		}
		for _, e := range x.Elems {
			if !g.generatable(e, seen) {
				return false
			}
		}
		return true
	case *check.Named:
		o := x.Obj
		args := typeArgs(x)
		if o.Module.IsPrelude {
			switch o.Name {
			case "Int", "Int8", "Int16", "Int32", "Int64", "UInt8", "UInt16", "UInt32", "UInt64",
				"Float", "Float32", "Bool", "Str", "Bytes", "Rune", "Duration", "Time":
				return true
			case "List", "Set", "Option", "Secret", "Map", "Result":
				for _, a := range args {
					if !g.generatable(a, seen) {
						return false
					}
				}
				return true
			}
			return false
		}
		if o.Module.IsStd {
			return false
		}
		if seen[o] {
			return true
		}
		seen[o] = true
		s := check.NewSubst(o.TParams, x.Args)
		switch o.Kind {
		case check.KStruct:
			for _, f := range o.Fields {
				if !g.generatable(s.Apply(f.Type), seen) {
					return false
				}
			}
			return true
		case check.KEnum:
			for _, v := range o.Variants {
				for _, f := range v.Fields {
					if !g.generatable(s.Apply(f.Type), seen) {
						return false
					}
				}
			}
			return true
		case check.KNewtype:
			return g.generatable(s.Apply(o.Target), seen)
		}
	}
	return false
}

// genFuzz emits a contract fuzz test for fo when it has contracts and
// generatable inputs.
func (pg *pkgGen) genFuzz(fo *check.FuncObj) {
	fd := fo.Decl
	var reqs []*ast.RequiresClause
	hasEnsures := false
	for _, cl := range fd.Clauses {
		switch c := cl.(type) {
		case *ast.RequiresClause:
			reqs = append(reqs, c)
		case *ast.EnsuresClause:
			hasEnsures = true
		}
	}
	if len(reqs) == 0 && !hasEnsures {
		return
	}
	if len(typeParams(fo.TParams)) > 0 || fo.Static {
		return
	}
	info := pg.g.info
	type param struct {
		v     *check.VarObj
		t     check.Type
		isVar bool
		cap   string
	}
	var ps []param
	if rv := info.RecvVar(fo); rv != nil {
		ps = append(ps, param{v: rv, t: fo.RecvType, isVar: fo.RecvVar})
	}
	for i, pv := range info.ParamVars(fo) {
		ps = append(ps, param{v: pv, t: fo.Params[i].Type, isVar: fo.Params[i].Var})
	}
	for i := range ps {
		ps[i].cap = pg.g.capField(ps[i].t)
		if ps[i].cap == "" && !pg.g.generatable(ps[i].t, map[*check.TypeObj]bool{}) {
			return
		}
	}
	idx := len(pg.tests)
	name := fmt.Sprintf("FZ_%d", idx)
	rt := pg.rt()
	// the precondition: every requires clause
	fg := pg.newFn(nil)
	fg.fr = &frame{name: fo.Name, resT: &check.Tuple{}}
	var sig []string
	for i, p := range ps {
		gn := fmt.Sprintf("a%d", i)
		gt := pg.goType(p.t)
		fg.names[p.v] = gn
		sig = append(sig, gn+" "+gt)
	}
	pre := fg.capture(func() {
		for _, r := range reqs {
			fg.line("if !(%s) {", fg.val(r.Cond))
			fg.line("\treturn false")
			fg.line("}")
		}
		fg.line("return true")
	})
	pg.p("func %s_pre(tk *%sTask%s) bool {", name, rt, prefixComma(sig))
	pg.out.WriteString(pre)
	pg.p("}\n")
	// the case: skip inputs that do not satisfy requires, then call
	var args []string
	recv := ""
	for i, p := range ps {
		a := fmt.Sprintf("a%d", i)
		if i == 0 && info.RecvVar(fo) != nil {
			recv = a
			continue
		}
		if p.isVar {
			a = "&" + a
		}
		args = append(args, a)
	}
	cg := pg.newFn(nil)
	cg.fr = &frame{name: fo.Name, resT: &check.Tuple{}}
	call, kind := cg.emitCall(fo, nil, recv, args, nil)
	var plain []string
	for i := range ps {
		plain = append(plain, fmt.Sprintf("a%d", i))
	}
	pg.p("func %s(tk *%sTask, os %sTesting_TestOs%s) %sResult[%sUnit, %sIError] {", name, rt, rt, prefixComma(sig), rt, rt, rt)
	pg.p("\t_ = os")
	pg.p("\tif !%s_pre(tk%s) {\n\t\treturn %sOk[%sUnit, %sIError](%sUnit{})\n\t}", name, prefixComma(plain), rt, rt, rt, rt)
	if kind == ckVoid {
		pg.p("\t%s", call)
	} else {
		pg.p("\t_ = %s", call)
	}
	pg.p("\treturn %sOk[%sUnit, %sIError](%sUnit{})\n}\n", rt, rt, rt, rt)
	tr := testRef{name: "contract fuzz: " + pg.frameName(fo), fn: name, loc: pg.g.loc(fd), params: len(ps), fuzz: true}
	// the Xo source of the call, for a regression test (core 9.2)
	var xargs []string
	first := 0
	callee := fo.Name
	if info.RecvVar(fo) != nil {
		callee = "{0}." + fo.Name
		first = 1
	}
	for i := first; i < len(ps); i++ {
		xargs = append(xargs, fmt.Sprintf("{%d}", i))
	}
	tr.regCall = callee + "(" + strings.Join(xargs, ", ") + ")"
	switch {
	case fo.Err != nil:
		tr.regResult = "result"
	case isUnitType(fo.Result):
		tr.regResult = "unit"
	default:
		tr.regResult = "value"
	}
	for _, p := range ps {
		tr.regVars = append(tr.regVars, p.isVar)
	}
	for _, p := range ps {
		tr.paramNames = append(tr.paramNames, p.v.Name)
		tr.paramTypes = append(tr.paramTypes, pg.goType(p.t))
		tr.paramCaps = append(tr.paramCaps, p.cap)
	}
	pg.tests = append(pg.tests, tr)
}

// isUnitType reports whether t is `()` (or absent).
func isUnitType(t check.Type) bool {
	if t == nil {
		return true
	}
	tu, ok := t.(*check.Tuple)
	return ok && len(tu.Elems) == 0
}
