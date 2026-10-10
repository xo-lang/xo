package gen

import (
	"bytes"
	"fmt"
	"go/format"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// ---------------------------------------------------------------------------
// AST walking

// inspectNode calls fn for n and its descendants (expressions, statements,
// and blocks; closures included) until fn returns false for a node.
func inspectNode(n ast.Node, fn func(ast.Node) bool) {
	if n == nil {
		return
	}
	if !fn(n) {
		return
	}
	walk := func(c ast.Node) {
		if c != nil {
			inspectNode(c, fn)
		}
	}
	switch x := n.(type) {
	case *ast.Block:
		for _, s := range x.Stmts {
			walk(s)
		}
	case *ast.ExprStmt:
		walk(x.X)
	case *ast.LetStmt:
		walkExpr(x.Value, walk)
	case *ast.VarStmt:
		walkExpr(x.Value, walk)
	case *ast.ConstDecl:
		walkExpr(x.Value, walk)
	case *ast.AssignStmt:
		walkExpr(x.Target, walk)
		walkExpr(x.Value, walk)
	case *ast.DeferStmt:
		walk(x.Body)
	case *ast.ReturnStmt:
		walkExpr(x.Value, walk)
	case *ast.ForStmt:
		walkExpr(x.Iter, walk)
		walkExpr(x.Bound, walk)
		walk(x.Body)
	case *ast.WhileStmt:
		walkExpr(x.Cond, walk)
		walkExpr(x.Value, walk)
		walkExpr(x.Bound, walk)
		walk(x.Body)
	case *ast.ParenExpr:
		walkExpr(x.X, walk)
	case *ast.TupleExpr:
		for _, e := range x.Elems {
			walkExpr(e, walk)
		}
	case *ast.ListLit:
		for _, e := range x.Elems {
			walkExpr(e, walk)
		}
	case *ast.SetLit:
		for _, e := range x.Elems {
			walkExpr(e, walk)
		}
	case *ast.MapLit:
		for _, en := range x.Entries {
			walkExpr(en.Key, walk)
			walkExpr(en.Value, walk)
		}
	case *ast.StructLit:
		for _, fi := range x.Fields {
			walkExpr(fi.Value, walk)
		}
	case *ast.WithExpr:
		walkExpr(x.X, walk)
		for _, fi := range x.Fields {
			walkExpr(fi.Value, walk)
		}
	case *ast.StringLit:
		for _, p := range x.Parts {
			walkExpr(p.Expr, walk)
		}
	case *ast.UnaryExpr:
		walkExpr(x.X, walk)
	case *ast.BinaryExpr:
		walkExpr(x.X, walk)
		walkExpr(x.Y, walk)
	case *ast.IsExpr:
		walkExpr(x.X, walk)
	case *ast.RangeExpr:
		walkExpr(x.Lo, walk)
		walkExpr(x.Hi, walk)
	case *ast.SelectorExpr:
		walkExpr(x.X, walk)
	case *ast.CallExpr:
		walkExpr(x.Fun, walk)
		for _, a := range x.Args {
			walkExpr(a.Value, walk)
		}
	case *ast.TypeArgsExpr:
		walkExpr(x.X, walk)
	case *ast.IndexExpr:
		walkExpr(x.X, walk)
		walkExpr(x.Index, walk)
	case *ast.TryExpr:
		walkExpr(x.X, walk)
	case *ast.IfExpr:
		walkExpr(x.Cond, walk)
		walkExpr(x.Value, walk)
		walk(x.Then)
		walkExpr(x.Else, walk)
	case *ast.MatchExpr:
		walkExpr(x.Subject, walk)
		for _, arm := range x.Arms {
			walkExpr(arm.Guard, walk)
			walk(arm.Body)
		}
	case *ast.ScopeExpr:
		walk(x.Body)
	case *ast.SelectExpr:
		for _, arm := range x.Arms {
			walkExpr(arm.Op, walk)
			walkExpr(arm.After, walk)
			walk(arm.Body)
		}
	case *ast.FnLit:
		walk(x.Body)
	case *ast.WhileExpr:
		walk(x.Body)
	}
}

func walkExpr(e ast.Expr, walk func(ast.Node)) {
	if e == nil {
		return
	}
	walk(e)
}

// inspectExpr walks the expressions of e.
func inspectExpr(e ast.Expr, fn func(ast.Expr) bool) {
	inspectNode(e, func(n ast.Node) bool {
		if x, ok := n.(ast.Expr); ok {
			return fn(x)
		}
		return true
	})
}

// ---------------------------------------------------------------------------
// Std defaults

// stdDefault evaluates the default value of a std parameter or field. Std
// declaration files are not type checked, so the defaults are literal forms.
func (f *fnGen) stdDefault(e ast.Expr, t check.Type) string {
	r := f.rt()
	switch x := e.(type) {
	case *ast.IntLit:
		return f.intLit(x.Value, t)
	case *ast.DurationLit:
		return fmt.Sprintf("%sDuration(%s)", r, x.Nanos)
	case *ast.BytesLit:
		return fmt.Sprintf("%sBytes(%s)", r, strconv.Quote(x.Value))
	case *ast.StringLit:
		var s string
		for _, p := range x.Parts {
			s += p.Text
		}
		return strconv.Quote(s)
	case *ast.MapLit, *ast.ListLit, *ast.SetLit:
		return f.goType(t) + "{}"
	case *ast.SelectorExpr:
		if n := f.g.named(t); n != nil && n.Obj.Kind == check.KEnum {
			for _, v := range n.Obj.Variants {
				if v.Name == x.Sel.Name {
					return f.variantValue(v, n, nil)
				}
			}
		}
	case *ast.Ident:
		if x.Name == "true" || x.Name == "false" {
			return x.Name
		}
	}
	f.g.errorf(e, "unsupported std default value")
	return "nil"
}

// ---------------------------------------------------------------------------
// Interface adapters (core 2.4, 2.10)

// methodSig is the Go signature tail of a method under a substitution.
func (pg *pkgGen) methodSig(fo *check.FuncObj, sub *check.Subst) string {
	var parts []string
	for _, p := range fo.Params {
		t := p.Type
		if sub != nil {
			t = sub.Apply(t)
		}
		s := pg.goType(t)
		if p.Var {
			s = "*" + s
		}
		parts = append(parts, s)
	}
	res := ""
	switch {
	case returnsResult(fo):
		r, e := fo.Result, fo.Err
		if sub != nil {
			r, e = sub.Apply(r), sub.Apply(e)
		}
		res = pg.resultOf(r, e)
	case returnsVoid(fo):
	default:
		r := fo.Result
		if sub != nil {
			r = sub.Apply(r)
		}
		res = pg.goType(r)
	}
	return "(" + strings.Join(parts, ", ") + ") " + res
}

func containsParam(t check.Type) bool {
	switch x := check.Resolve(t).(type) {
	case *check.ParamType:
		return true
	case *check.Named:
		for _, a := range x.Args {
			if containsParam(a) {
				return true
			}
		}
	case *check.Tuple:
		for _, e := range x.Elems {
			if containsParam(e) {
				return true
			}
		}
	case *check.Func:
		for _, p := range x.Params {
			if containsParam(p.T) {
				return true
			}
		}
		return containsParam(x.Result) || x.Err != nil && containsParam(x.Err)
	}
	return false
}

// ifaceAdapter returns the name of a generated adapter type that makes a
// value of type from satisfy interface to when its Go methods differ (a
// method that cannot fail where the interface's can, or an error that
// converts). It returns "" when the value satisfies the interface as is.
func (pg *pkgGen) ifaceAdapter(from, to check.Type) string {
	tn := pg.g.named(to)
	if tn == nil || containsParam(from) || containsParam(to) {
		return ""
	}
	isub := check.NewSubst(tn.Obj.TParams, tn.Args)
	type pair struct {
		im, m *check.FuncObj
		ms    *check.Subst
	}
	var pairs []pair
	differ := false
	for _, im := range tn.Obj.IfaceMeths {
		if len(typeParams(ownTParams(im))) > 0 {
			continue
		}
		m, ms := pg.g.info.FindMethod(from, im.Name)
		if m == nil {
			return ""
		}
		if m.Iface && !pg.g.isInterface(from) {
			return ""
		}
		if pg.methodSig(m, ms) != pg.methodSig(im, isub) || !methodIsGoMethod(m) {
			differ = true
		}
		pairs = append(pairs, pair{im, m, ms})
	}
	if !differ {
		return ""
	}
	key := pg.goType(from) + "|" + pg.goType(to)
	name := "xoAdapt_" + strconv.Itoa(len(pg.adapt))
	for k := range pg.adapt {
		if strings.HasPrefix(k, key+"=") {
			return strings.TrimPrefix(k, key+"=")
		}
	}
	pg.adapt[key+"="+name] = true
	rt := pg.rt()
	pg.imports["strings"] = "strings"
	var b strings.Builder
	fmt.Fprintf(&b, "type %s struct{ V %s }\n\n", name, pg.goType(from))
	fmt.Fprintf(&b, "func (a %s) XoDebug(b *strings.Builder) { %sDebugTo(b, a.V) }\n", name, rt)
	fmt.Fprintf(&b, "func (a %s) XoEq(o any) bool { if p, ok := o.(%s); ok { return %sEqAny(a.V, p.V) }; return false }\n", name, name, rt)
	fmt.Fprintf(&b, "func (a %s) XoHash() uint64 { return %sHashAny(a.V) }\n\n", name, rt)
	for _, p := range pairs {
		fg := pg.newFn(nil)
		fg.fr = &frame{}
		var ps, args []string
		for i, prm := range p.im.Params {
			t := isub.Apply(prm.Type)
			s := pg.goType(t)
			if prm.Var {
				s = "*" + s
			}
			ps = append(ps, fmt.Sprintf("p%d %s", i, s))
			args = append(args, fmt.Sprintf("p%d", i))
		}
		call, kind := fg.emitCall(p.m, p.ms, "a.V", args, nil)
		ires := ""
		var body string
		mr := p.m.Result
		if p.ms != nil {
			mr = p.ms.Apply(mr)
		}
		switch {
		case returnsResult(p.im):
			ir, ie := isub.Apply(p.im.Result), isub.Apply(p.im.Err)
			ires = pg.resultOf(ir, ie)
			switch {
			case kind == ckVoid:
				body = fmt.Sprintf("%s; return %sOk[%s, %s](%sUnit{})", call, rt, pg.goType(ir), pg.goType(ie), rt)
			case returnsResult(p.m) && !strings.HasSuffix(call, ".V"):
				me := p.m.Err
				if p.ms != nil {
					me = p.ms.Apply(me)
				}
				body = fmt.Sprintf("r := %s; if r.Ok { return %sOk[%s, %s](r.V) }; return %sErrTr[%s](%s, r.Tr)", call, rt, pg.goType(ir), pg.goType(ie), rt, pg.goType(ir), fg.coerceErr("r.E", me, ie))
			default:
				body = fmt.Sprintf("return %sOk[%s, %s](%s)", rt, pg.goType(ir), pg.goType(ie), fg.coerce(call, mr, ir))
			}
		case returnsVoid(p.im):
			body = call
		default:
			ir := isub.Apply(p.im.Result)
			ires = pg.goType(ir)
			if kind == ckVoid {
				body = call + "; return " + rt + "Unit{}"
			} else {
				body = "return " + fg.coerce(call, mr, ir)
			}
		}
		fmt.Fprintf(&b, "func (a %s) %s(tk *%sTask%s) %s { %s }\n", name, goMethod(p.im.Name), rt, prefixComma(ps), ires, body)
	}
	pg.out.WriteString(b.String())
	pg.out.WriteString("\n")
	return name
}

// ---------------------------------------------------------------------------
// Entry points

// mainFile is the program's package main: it runs the Xo `main` of mod
// through XoMain, which mainEntryFile adds to mod's package. Package main
// names no runtime type: a Go package that mentions one (rt.Task, rt.Os)
// compiles every method of every generic type reachable from it (gotcha
// go-generic-method-sets-per-package.md), so the closure that calls F_main
// lives in the module's package, which pays that cost anyway.
func (g *gen) mainFile(mod *check.Module) ([]byte, error) {
	pg := g.pkgs[mod]
	fo := mod.Scope["main"].(*check.FuncObj)
	if len(fo.Params) > 1 {
		return nil, &Error{Msg: "`main` takes at most one parameter, `os: Os`"}
	}
	var b bytes.Buffer
	b.WriteString("// Code generated by xo. DO NOT EDIT.\n\npackage main\n\nimport (\n\t\"os\"\n")
	pre := g.enableArenas("rt.") + gcConfig("rt.", mod)
	if pre != "" {
		fmt.Fprintf(&b, "\trt %q\n", ModulePath+"/rt")
	}
	fmt.Fprintf(&b, "\t%s %q\n)\n\n", pg.name, ModulePath+"/"+pg.dir)
	fmt.Fprintf(&b, "func main() {\n\t%sos.Exit(%s.XoMain())\n}\n", pre, pg.name)
	return format.Source(b.Bytes())
}

// mainEntryFile is the file of mod's package that defines XoMain, the run
// of the Xo `main` that package main calls (mainFile).
func (g *gen) mainEntryFile(mod *check.Module) ([]byte, error) {
	pg := g.pkgs[mod]
	fo := mod.Scope["main"].(*check.FuncObj)
	rt := "rt."
	var call string
	args := ""
	if len(fo.Params) == 1 {
		args = ", osv"
	}
	switch {
	case returnsResult(fo):
		call = fmt.Sprintf("r := F_main(tk%s); if !r.Ok { return %sErrTr[%sUnit](%sIface[%sIError](r.E), r.Tr) }; return %sOk[%sUnit, %sIError](%sUnit{})", args, rt, rt, rt, rt, rt, rt, rt, rt)
	case returnsVoid(fo):
		call = fmt.Sprintf("F_main(tk%s); return %sOk[%sUnit, %sIError](%sUnit{})", args, rt, rt, rt, rt)
	default:
		call = fmt.Sprintf("_ = F_main(tk%s); return %sOk[%sUnit, %sIError](%sUnit{})", args, rt, rt, rt, rt)
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by xo. DO NOT EDIT.\n\npackage %s\n\nimport rt %q\n\n", pg.name, ModulePath+"/rt")
	b.WriteString("// XoMain runs the program's main; package main calls it. Not inlined, so\n// package main does not compile the closure and the runtime types it names.\n//\n//go:noinline\n")
	fmt.Fprintf(&b, "func XoMain() int {\n\treturn %sRunMain(func(tk *%sTask, osv %sOs) %sResult[%sUnit, %sIError] { _ = osv; %s })\n}\n", rt, rt, rt, rt, rt, rt, call)
	return format.Source(b.Bytes())
}

// enableArenas is the statement that turns request scoped arenas on at the
// start of main (Options.Arenas), or "".
func (g *gen) enableArenas(rt string) string {
	if !g.opts.Arenas {
		return ""
	}
	return rt + "EnableArenas(); "
}

// gcConfig is the statement that passes the `gc` lines of the main
// module's xo.mod to the runtime (decision 0065), or "".
func gcConfig(rt string, mod *check.Module) string {
	c := mod.GC
	if !c.IsSet() || c == (check.GCConfig{}) {
		return ""
	}
	return fmt.Sprintf("%sGCConfig(%d, %d, %d); ", rt, c.Percent, c.MinHeap, c.MemoryLimit)
}

// testMainFile runs the test tables of the root modules. Like mainFile it
// names no runtime type: each table reaches rt.RunTestTables as an any
// through the module's XoTests.
func (g *gen) testMainFile() ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by xo. DO NOT EDIT.\n\npackage main\n\nimport (\n\t\"os\"\n\trt \"" + ModulePath + "/rt\"\n")
	var refs []string
	for _, m := range g.info.AllModules() {
		pg := g.pkgs[m]
		if pg == nil || !m.Root {
			continue
		}
		fmt.Fprintf(&b, "\t%s %q\n", pg.name, ModulePath+"/"+pg.dir)
		refs = append(refs, pg.name+".XoTests()")
	}
	b.WriteString(")\n\n")
	fmt.Fprintf(&b, "func main() {\n\t%sos.Exit(rt.RunTestTables(os.Args[1:], %s))\n}\n", g.enableArenas("rt."), strings.Join(append([]string{}, refs...), ", "))
	if len(refs) == 0 {
		b.Reset()
		b.WriteString("package main\n\nimport (\n\t\"os\"\n\trt \"" + ModulePath + "/rt\"\n)\n\nfunc main() { " + g.enableArenas("rt.") + "os.Exit(rt.RunTestTables(os.Args[1:])) }\n")
	}
	return format.Source(b.Bytes())
}

// genTest generates a test block as a function of the test's `os`.
func (pg *pkgGen) genTest(td *ast.TestDecl) {
	fg := pg.newFn(nil)
	errT := &check.Named{Obj: pg.g.prelude.Scope["Error"].(*check.TypeObj)}
	unit := &check.Tuple{}
	fr := &frame{name: pg.m.Name + ".test", resT: unit, errT: errT, isResult: true, isTest: true}
	fr.goRes = pg.resultOf(unit, errT)
	fg.fr = fr
	osv := pg.g.info.TestOs(td)
	if osv != nil {
		fg.names[osv] = "v_os"
	}
	params := pg.g.info.TestParams(td)
	var ps []string
	for _, p := range params {
		name := fg.declName(p)
		ps = append(ps, name+" "+pg.goType(p.Type))
	}
	fg.analyzeRanges(td.Body, nil, nil)
	body := fg.capture(func() { fg.topBlock(td.Body) })
	idx := len(pg.tests)
	name := fmt.Sprintf("T_%d", idx)
	text := ""
	for _, p := range td.Name.Parts {
		text += p.Text
	}
	seed := int64(0)
	if td.Seed != nil {
		seed, _ = strconv.ParseInt(td.Seed.Value, 10, 64)
	}
	var uses []string
	for _, e := range td.Uses {
		u := e.Name.Name
		if e.Sub != nil {
			u += "." + e.Sub.Name
		}
		uses = append(uses, u)
	}
	tr := testRef{name: text, fn: name, loc: pg.g.loc(td), seed: seed, params: len(params), uses: uses}
	for _, p := range params {
		tr.paramNames = append(tr.paramNames, p.Name)
		tr.paramTypes = append(tr.paramTypes, pg.goType(p.Type))
	}
	pg.tests = append(pg.tests, tr)
	pg.p("%s", pg.g.lineDirective(td))
	pg.p("func %s(tk *%sTask, v_os %sTesting_TestOs%s) %s {", name, pg.rt(), pg.rt(), prefixComma(ps), fr.goRes)
	pg.p("\t_ = v_os")
	pg.out.WriteString(body)
	pg.p("}\n")
}

// absDir is the module's directory, where its tests run (as `go test`
// runs a package's tests in its directory).
func (pg *pkgGen) absDir() string {
	abs, err := filepath.Abs(pg.m.Dir)
	if err != nil {
		return pg.m.Dir
	}
	return abs
}

type testRef struct {
	name       string
	fn         string
	loc        string
	seed       int64
	params     int
	uses       []string
	paramNames []string
	paramTypes []string
	paramCaps  []string // TestOs field for a capability parameter, "os" for Os
	fuzz       bool     // a contract fuzz test (core 9.1)
	// regression template for a contract fuzz test (core 9.2): the Xo call
	// with {i} for parameter i, and how its result is consumed.
	regCall   string
	regResult string // "result", "value", or "unit"
	regVars   []bool
}

func (pg *pkgGen) genTestTable() {
	rt := pg.rt()
	pg.p("// XoTests is Tests as an any, for package main (testMainFile).")
	pg.p("//go:noinline")
	pg.p("func XoTests() any { return Tests }")
	pg.p("")
	pg.p("var Tests = []%sTestCase{", rt)
	for _, t := range pg.tests {
		if t.params > 0 {
			var gens, args, pairs []string
			if t.fuzz {
				gens = append(gens, "g.Small = true")
			}
			for i, n := range t.paramNames {
				c := ""
				if i < len(t.paramCaps) {
					c = t.paramCaps[i]
				}
				switch {
				case c == "os":
					args = append(args, rt+"TestOsAsOs(os)")
				case c != "":
					args = append(args, "os."+c)
				default:
					gens = append(gens, fmt.Sprintf("a%d := %sGen[%s](g)", i, rt, t.paramTypes[i]))
					args = append(args, fmt.Sprintf("a%d", i))
					pairs = append(pairs, strconv.Quote(n), fmt.Sprintf("a%d", i))
				}
			}
			pg.p("\t{Name: %q, Module: %q, Dir: %q, Loc: %q, Seed: %d, Prop: func(g *%sGenRand) (string, func(*%sTask, %sTesting_TestOs) %sResult[%sUnit, %sIError]) {", t.name, pg.m.Path, pg.absDir(), t.loc, t.seed, rt, rt, rt, rt, rt, rt)
			for _, gl := range gens {
				pg.p("\t\t%s", gl)
			}
			pg.p("\t\treturn %sPropInput(%s), func(tk *%sTask, os %sTesting_TestOs) %sResult[%sUnit, %sIError] { return %s(tk, os, %s) }", rt, strings.Join(pairs, ", "), rt, rt, rt, rt, rt, t.fn, strings.Join(args, ", "))
			if t.regCall != "" {
				var names, caps, vars []string
				for i, n := range t.paramNames {
					names = append(names, strconv.Quote(n))
					caps = append(caps, strconv.Quote(t.paramCaps[i]))
					vars = append(vars, strconv.FormatBool(i < len(t.regVars) && t.regVars[i]))
				}
				pg.p("\t}, Regress: &%sRegressInfo{Call: %q, Result: %q, Names: []string{%s}, Caps: []string{%s}, Vars: []bool{%s}}},", rt, t.regCall, t.regResult, strings.Join(names, ", "), strings.Join(caps, ", "), strings.Join(vars, ", "))
				continue
			}
			pg.p("\t}},")
			continue
		}
		uses := ""
		if len(t.uses) > 0 {
			var qs []string
			for _, u := range t.uses {
				qs = append(qs, strconv.Quote(u))
			}
			uses = ", Uses: []string{" + strings.Join(qs, ", ") + "}"
		}
		pg.p("\t{Name: %q, Module: %q, Dir: %q, Loc: %q, Seed: %d, Fn: %s%s},", t.name, pg.m.Path, pg.absDir(), t.loc, t.seed, t.fn, uses)
	}
	pg.p("}\n")
}
