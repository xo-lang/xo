package gen

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// fnGen generates the body of one Go function (and the closures nested in
// it, which are emitted inline).
type fnGen struct {
	pg       *pkgGen
	g        *gen
	info     *check.Info
	out      *strings.Builder
	ind      int
	names    map[*check.VarObj]string
	used     map[string]int
	fr       *frame
	lastLine string
	// scopes maps the VarObj of `scope s` to its Go scope variable.
	scopes map[*check.VarObj]*scopeInfo
	// oldVals holds the `old(...)` values of an ensures clause.
	oldVals map[ast.Expr]string
	// boundTo maps a spawn call to the binding its handle is stored in.
	boundTo map[*ast.CallExpr]*check.VarObj
	// writebacks store places copied for a `var` receiver or argument
	// back after the call (place).
	writebacks []func()
	// litParams maps parameters of a specialized copy to the closure
	// literals the copy inlines (mono.go).
	litParams map[*check.VarObj]*ast.FnLit
	// rng is the range analysis of the body being generated (ranges.go):
	// the Int operations proved not to fault.
	rng *rangeAn
	// arenaTemps are producer calls whose List is an arena temporary, and
	// arenaResp the response calls in return position of a handler
	// (arena.go, Options.Arenas).
	arenaTemps map[*ast.CallExpr]bool
	arenaResp  map[*ast.CallExpr]bool
}

// frame is an Xo function being generated: a declared function, a
// closure, a test, or an initializer.
type frame struct {
	parent   *frame
	fo       *check.FuncObj
	name     string // for error frames
	params   []paramRef
	resT     check.Type // declared result
	errT     check.Type // nil: cannot fail
	goRes    string     // Go result type, "" when void
	isResult bool       // the Go function returns rt.Result
	loops    []*loopInfo
	iife     int       // nesting depth of block closures (defer, scope)
	ctl      []*ctlSet // codes used inside each open block closure
	retVar   string    // variable holding the return value inside block closures
	nextCode int
	isTest   bool
}

type paramRef struct {
	xo   string
	code string
	t    check.Type
}

type loopInfo struct {
	label   string // Xo label or ""
	goLabel string
	used    bool
	depth   int // frame.iife at the loop
	brk     int // control codes, 0 until needed
	cont    int
}

// ctlSet records the control codes returned by one block closure.
type ctlSet struct {
	codes map[int]func()
}

const ctlReturn = 1

func (pg *pkgGen) newFn(fo *check.FuncObj) *fnGen {
	return &fnGen{pg: pg, g: pg.g, info: pg.g.info, out: &strings.Builder{}, ind: 1, names: map[*check.VarObj]string{}, used: map[string]int{}, scopes: map[*check.VarObj]*scopeInfo{}, boundTo: map[*ast.CallExpr]*check.VarObj{}}
}

// ---------------------------------------------------------------------------
// Emission

func (f *fnGen) line(format string, args ...any) {
	for i := 0; i < f.ind; i++ {
		f.out.WriteByte('\t')
	}
	fmt.Fprintf(f.out, format, args...)
	f.out.WriteByte('\n')
}

// at emits a //line directive for n before the next statement.
func (f *fnGen) at(n ast.Node) {
	if n == nil {
		return
	}
	d := f.g.lineDirective(n)
	if d == f.lastLine {
		return
	}
	f.lastLine = d
	f.out.WriteString(d)
	f.out.WriteByte('\n')
}

// capture runs gen with output redirected and returns what it wrote.
func (f *fnGen) capture(gen func()) string {
	saved := f.out
	var b strings.Builder
	f.out = &b
	gen()
	f.out = saved
	return b.String()
}

func (f *fnGen) tmp(prefix string) string {
	f.pg.tmp++
	return fmt.Sprintf("x_%s%d", prefix, f.pg.tmp)
}

// declName assigns a fresh Go name to a local binding.
func (f *fnGen) declName(v *check.VarObj) string {
	base := "v_" + v.Name
	if v.Name == "_" {
		base = "v__"
	}
	n := f.used[base]
	f.used[base] = n + 1
	name := base
	if n > 0 {
		name = fmt.Sprintf("%s_%d", base, n)
	}
	f.names[v] = name
	return name
}

// varName is the Go expression for a local binding.
func (f *fnGen) varName(v *check.VarObj) string {
	if n, ok := f.names[v]; ok {
		return n
	}
	return "MISSING_VAR_" + v.Name
}

func (f *fnGen) rt() string { return f.pg.rt() }

func (f *fnGen) goType(t check.Type) string { return f.pg.goType(t) }

// ---------------------------------------------------------------------------
// Types of expressions

// typeOf is the type of e in context (after implicit conversion).
func (f *fnGen) typeOf(e ast.Expr) check.Type {
	if t := f.info.TypeOf(e); t != nil {
		return t
	}
	return f.info.NaturalType(e)
}

// nat is the type of e before implicit conversion.
func (f *fnGen) nat(e ast.Expr) check.Type {
	if t := f.info.NaturalType(e); t != nil {
		return t
	}
	return f.info.TypeOf(e)
}

// ---------------------------------------------------------------------------
// Function bodies

// funcBody generates a declared function's body, with contracts.
func (f *fnGen) funcBody(fd *ast.FnDecl) string {
	return f.capture(func() {
		fo := f.fr.fo
		var ensures []*ast.EnsuresClause
		for _, cl := range fd.Clauses {
			switch c := cl.(type) {
			case *ast.RequiresClause:
				f.requires(c)
			case *ast.EnsuresClause:
				ensures = append(ensures, c)
			}
		}
		// a var method of a type with an invariant checks it afterwards
		// (core 9.1)
		inv := fo != nil && fo.RecvVar && fo.Recv != nil && fo.Recv.Invariant != nil && !fo.Recv.Module.IsStd
		if len(ensures) > 0 || inv {
			f.withEnsures(fo, fd, ensures, inv)
			return
		}
		f.topBlock(fd.Body)
	})
}

// topBlock generates the body block of the current frame in return
// position. Go defers can be used directly for the body's own defers.
func (f *fnGen) topBlock(b *ast.Block) {
	body := f.capture(func() {
		f.stmts(b, dest{kind: dReturn}, true)
	})
	if f.fr.retVar != "" {
		res := f.fr.goRes
		f.line("var %s %s", f.fr.retVar, res)
	}
	f.out.WriteString(body)
	if f.fr.goRes != "" && !endsInReturn(body, f.ind) {
		f.line("panic(%q)", "unreachable: missing return")
	}
}

// endsInReturn reports whether the last statement of body is a `return`
// at indentation ind (a Go terminating statement, so no panic is needed
// after it; the panic would only add to the function's inlining cost).
func endsInReturn(body string, ind int) bool {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	last := lines[len(lines)-1]
	pre := strings.Repeat("\t", ind)
	return strings.HasPrefix(last, pre+"return") && !strings.HasPrefix(last, pre+"\t") &&
		(len(last) == len(pre)+len("return") || last[len(pre)+len("return")] == ' ')
}

func (f *fnGen) requires(c *ast.RequiresClause) {
	f.at(c)
	cond := f.val(c.Cond)
	if c.Else == nil {
		f.line("%sRequire(%s, %q, %q%s)", f.rt(), cond, f.g.source(c.Cond), f.g.loc(c.Cond), f.contractVals(c.Cond))
		return
	}
	f.line("if !(%s) {", cond)
	f.ind++
	ev := f.val(c.Else)
	ev = f.coerceErr(ev, f.typeOf(c.Else), f.fr.errT)
	f.emitReturnValue(fmt.Sprintf("%sErr[%s](%s)", f.rt(), f.goType(f.fr.resT), ev))
	f.ind--
	f.line("}")
}

// contractVals lists the values of the identifiers and fields in a
// contract expression, printed when it fails (core 9.1).
func (f *fnGen) contractVals(e ast.Expr) string {
	var parts []string
	seen := map[string]bool{}
	inspectExpr(e, func(n ast.Expr) bool {
		switch x := n.(type) {
		case *ast.Ident:
			if v := f.info.VarOf(x); v != nil {
				src := x.Name
				if !seen[src] {
					seen[src] = true
					parts = append(parts, fmt.Sprintf("%q, %s", src, f.varName(v)))
				}
			}
		case *ast.CallExpr:
			return false
		}
		return true
	})
	if len(parts) == 0 {
		return ""
	}
	return ", " + strings.Join(parts, ", ")
}

// withEnsures runs the body in a closure, then checks the ensures clauses
// against `result` and the `old(...)` values captured on entry.
func (f *fnGen) withEnsures(fo *check.FuncObj, fd *ast.FnDecl, ens []*ast.EnsuresClause, inv bool) {
	// capture old(...) values first
	olds := map[ast.Expr]string{}
	for _, c := range ens {
		inspectExpr(c.Cond, func(n ast.Expr) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "old" && len(call.Args) == 1 && f.info.VarOf(id) == nil {
					name := f.tmp("old")
					f.line("%s := %s", name, f.val(call.Args[0].Value))
					f.line("_ = %s", name)
					olds[call] = name
					return false
				}
			}
			return true
		})
	}
	f.oldVals = olds
	res := f.tmp("res")
	goRes := f.fr.goRes
	inner := f.capture(func() {
		f.ind++
		saved := f.fr
		fr := *saved
		fr.retVar = ""
		f.fr = &fr
		f.topBlock(fd.Body)
		f.fr = saved
		f.ind--
	})
	if goRes == "" {
		f.line("func() {")
		f.out.WriteString(inner)
		f.line("}()")
	} else {
		f.line("%s := func() %s {", res, goRes)
		f.out.WriteString(inner)
		f.line("}()")
	}
	for _, c := range ens {
		if rv := f.info.ResultVar(c); rv != nil {
			switch {
			case goRes == "":
				f.names[rv] = f.rt() + "Unit{}"
			case f.fr.isResult:
				f.names[rv] = res + ".V"
			default:
				f.names[rv] = res
			}
		}
		check := func() {
			f.at(c)
			cond := f.val(c.Cond)
			f.line("%sEnsure(%s, %q, %q%s)", f.rt(), cond, f.g.source(c.Cond), f.g.loc(c.Cond), f.contractVals(c.Cond))
		}
		if f.fr.isResult {
			f.line("if %s.Ok {", res)
			f.ind++
			check()
			f.ind--
			f.line("}")
		} else {
			check()
		}
	}
	if inv {
		recv := f.names[f.info.RecvVar(fo)]
		check := fmt.Sprintf("%s.XoInvariant(tk, %q)", recv, f.g.loc(fd.Name))
		if f.fr.isResult {
			f.line("if %s.Ok {", res)
			f.line("\t%s", check)
			f.line("}")
		} else {
			f.line("%s", check)
		}
	}
	f.oldVals = nil
	if goRes != "" {
		f.line("return %s", res)
	}
}

// ---------------------------------------------------------------------------
// Destinations

type destKind int

const (
	dDiscard destKind = iota
	dAssign
	dReturn
)

// dest is where the value of a block, if, or match goes.
type dest struct {
	kind destKind
	name string     // dAssign: Go variable
	typ  check.Type // dAssign: its Xo type
	// pre is emitted before a tail value is returned (dReturn): a scope
	// marks its body as completed.
	pre string
}

// into generates e with its value delivered to d.
func (f *fnGen) into(e ast.Expr, d dest) {
	switch x := e.(type) {
	case *ast.ParenExpr:
		f.into(x.X, d)
		return
	case *ast.Block:
		f.block(x, d)
		return
	case *ast.IfExpr:
		f.ifExpr(x, d)
		return
	case *ast.MatchExpr:
		f.matchExpr(x, d)
		return
	case *ast.ScopeExpr:
		f.scopeExpr(x, d)
		return
	case *ast.SelectExpr:
		f.selectExpr(x, d)
		return
	case *ast.WhileExpr:
		f.loopWhile(nil, nil, x.Body, x)
		f.deliverUnit(d)
		return
	}
	switch d.kind {
	case dDiscard:
		f.discard(e)
	case dAssign:
		if check.IsUnit(d.typ) && !check.IsUnit(f.nat(e)) && !check.IsNever(f.nat(e)) {
			f.discard(e)
			f.line("%s = %sUnit{}", d.name, f.rt())
			return
		}
		code := f.expr(e)
		f.line("%s = %s", d.name, f.coerce(code, f.nat(e), d.typ))
	case dReturn:
		if d.pre != "" {
			f.line("%s", d.pre)
		}
		f.ret(e)
	}
}

// deliverUnit delivers `()` to d (after a statement or a loop).
func (f *fnGen) deliverUnit(d dest) {
	switch d.kind {
	case dAssign:
		if check.IsUnit(d.typ) {
			f.line("%s = %sUnit{}", d.name, f.rt())
		}
	case dReturn:
		if d.pre != "" {
			f.line("%s", d.pre)
		}
		f.retUnit()
	}
}

// discard evaluates e for its effects.
func (f *fnGen) discard(e ast.Expr) {
	switch x := e.(type) {
	case *ast.ParenExpr:
		f.discard(x.X)
		return
	case *ast.Ident, *ast.IntLit, *ast.FloatLit, *ast.StringLit, *ast.UnitExpr, *ast.BytesLit, *ast.RuneLit, *ast.DurationLit, *ast.FnLit:
		if s, ok := x.(*ast.StringLit); ok {
			for _, p := range s.Parts {
				if p.Expr != nil {
					f.discard(p.Expr)
				}
			}
		}
		return
	case *ast.CallExpr:
		code, stmt := f.callStmt(x)
		if stmt {
			f.line("%s", code)
		} else if code != "" {
			f.line("_ = %s", code)
		}
		return
	case *ast.BinaryExpr:
		if f.isExpectCompare(x) {
			f.line("%s", f.expr(x))
			return
		}
	}
	code := f.expr(e)
	if code != "" && !strings.HasPrefix(code, f.rt()+"Unit{}") {
		f.line("_ = %s", code)
	}
}

// ---------------------------------------------------------------------------
// Returning

// ret returns the value of e from the current frame (core 2.6).
func (f *fnGen) ret(e ast.Expr) {
	fr := f.fr
	if call, ok := e.(*ast.CallExpr); ok && f.isRetCtor(call) {
		id := call.Fun.(*ast.Ident)
		arg := call.Args[0].Value
		if id.Name == "Err" {
			v := f.coerceErr(f.val(arg), f.typeOf(arg), fr.errT)
			f.emitReturnValue(fmt.Sprintf("%sErr[%s](%s)", f.rt(), f.goType(fr.resT), v))
			return
		}
		v := f.coerce(f.expr(arg), f.nat(arg), fr.resT)
		f.emitReturnValue(f.okValue(v))
		return
	}
	nat := f.nat(e)
	if check.IsNever(nat) {
		f.discard(e)
		return
	}
	if check.IsUnit(nat) && check.IsUnit(fr.resT) {
		f.discard(e)
		f.retUnit()
		return
	}
	if fr.isResult {
		if _, _, ok := f.g.resultParts(nat); ok {
			if _, _, rok := f.g.resultParts(fr.resT); !rok {
				code := f.expr(e)
				f.emitReturnValue(f.coerceResult(code, nat, fr.resT, fr.errT))
				return
			}
		}
	}
	if fr.goRes == "" {
		f.discard(e)
		f.emitReturnValue("")
		return
	}
	if check.IsUnit(fr.resT) && !check.IsUnit(nat) {
		// a `-> ()` body may end in a value that is discarded (core 3.2)
		f.discard(e)
		f.retUnit()
		return
	}
	v := f.coerce(f.expr(e), nat, fr.resT)
	if fr.isResult {
		v = f.okValue(v)
	}
	f.emitReturnValue(v)
}

// isRetCtor reports a bare `Err(x)` or `Ok(x)` in return position, which
// the checker types without recording a constructor.
func (f *fnGen) isRetCtor(call *ast.CallExpr) bool {
	id, ok := call.Fun.(*ast.Ident)
	if !ok || (id.Name != "Err" && id.Name != "Ok") || len(call.Args) != 1 {
		return false
	}
	return f.info.VarOf(id) == nil && f.info.Ref(id) == nil && f.info.Ctor(call) == nil && f.info.CallOf(call) == nil
}

func (f *fnGen) okValue(v string) string {
	fr := f.fr
	if !fr.isResult {
		return v
	}
	return fmt.Sprintf("%sOk[%s, %s](%s)", f.rt(), f.goType(fr.resT), f.errGo(fr.errT), v)
}

func (f *fnGen) errGo(e check.Type) string {
	if e == nil {
		return f.rt() + "Never"
	}
	return f.goType(e)
}

// retUnit returns `()` from the current frame.
func (f *fnGen) retUnit() {
	fr := f.fr
	switch {
	case fr.goRes == "":
		f.emitReturnValue("")
	case fr.isResult:
		f.emitReturnValue(f.okValue(f.rt() + "Unit{}"))
	default:
		f.emitReturnValue(f.rt() + "Unit{}")
	}
}

// emitReturnValue returns a Go value (already in the frame's result form)
// from the current frame, through any open block closures.
func (f *fnGen) emitReturnValue(v string) {
	fr := f.fr
	if fr.iife == 0 {
		if v == "" {
			f.line("return")
		} else {
			f.line("return %s", v)
		}
		return
	}
	if v != "" {
		if fr.retVar == "" {
			fr.retVar = f.tmp("ret")
		}
		f.line("%s = %s", fr.retVar, v)
	}
	f.jump(ctlReturn)
}

// jump returns control code c from the innermost block closure.
func (f *fnGen) jump(c int) {
	fr := f.fr
	set := fr.ctl[len(fr.ctl)-1]
	if _, ok := set.codes[c]; !ok {
		set.codes[c] = nil
	}
	f.line("return %d", c)
}

// afterIIFE dispatches the control codes returned by a block closure that
// just ran (its result is in variable ctl).
func (f *fnGen) afterIIFE(ctl string, set *ctlSet) {
	if len(set.codes) == 0 {
		return
	}
	var codes []int
	for c := range set.codes {
		codes = append(codes, c)
	}
	sort.Ints(codes)
	f.line("switch %s {", ctl)
	for _, c := range codes {
		f.line("case %d:", c)
		f.ind++
		f.dispatch(c)
		f.ind--
	}
	f.line("}")
}

// dispatch performs the jump for control code c at the current depth.
func (f *fnGen) dispatch(c int) {
	fr := f.fr
	if c == ctlReturn {
		if fr.iife > 0 {
			f.jump(ctlReturn)
			return
		}
		switch {
		case fr.goRes == "":
			f.line("return")
		default:
			if fr.retVar == "" {
				fr.retVar = f.tmp("ret")
			}
			f.line("return %s", fr.retVar)
		}
		return
	}
	for _, l := range fr.loops {
		if l.brk == c {
			f.breakLoop(l)
			return
		}
		if l.cont == c {
			f.continueLoop(l)
			return
		}
	}
	f.line("panic(%q)", "bad control code")
}

func (f *fnGen) breakLoop(l *loopInfo) {
	if l.depth == f.fr.iife {
		l.used = true
		f.line("break %s", l.goLabel)
		return
	}
	if l.brk == 0 {
		f.fr.nextCode++
		l.brk = f.fr.nextCode + 1
	}
	f.jump(l.brk)
}

func (f *fnGen) continueLoop(l *loopInfo) {
	if l.depth == f.fr.iife {
		l.used = true
		f.line("continue %s", l.goLabel)
		return
	}
	if l.cont == 0 {
		f.fr.nextCode++
		l.cont = f.fr.nextCode + 1
	}
	f.jump(l.cont)
}

// iife runs gen inside a block closure `func() int { ... }()` and
// dispatches its control codes afterwards. pre is emitted inside the
// closure first (Go defers, scope setup); post wraps the call result.
func (f *fnGen) iife(gen func()) {
	fr := f.fr
	ctl := f.tmp("ctl")
	set := &ctlSet{codes: map[int]func(){}}
	fr.iife++
	fr.ctl = append(fr.ctl, set)
	body := f.capture(func() {
		f.ind++
		gen()
		f.line("return 0")
		f.ind--
	})
	fr.ctl = fr.ctl[:len(fr.ctl)-1]
	fr.iife--
	f.line("%s := func() int {", ctl)
	f.out.WriteString(body)
	f.line("}()")
	f.line("_ = %s", ctl)
	f.afterIIFE(ctl, set)
}

// ---------------------------------------------------------------------------
// Blocks and statements

func hasDefer(b *ast.Block) bool {
	for _, s := range b.Stmts {
		if _, ok := s.(*ast.DeferStmt); ok {
			return true
		}
	}
	return false
}

// block generates a nested block with its value delivered to d.
func (f *fnGen) block(b *ast.Block, d dest) {
	if hasDefer(b) {
		f.iife(func() { f.stmts(b, d, true) })
		return
	}
	f.line("{")
	f.ind++
	f.stmts(b, d, false)
	f.ind--
	f.line("}")
}

// stmts generates the statements of b; the tail expression goes to d.
// directDefer allows Go defer statements (the enclosing Go function ends
// with the block).
func (f *fnGen) stmts(b *ast.Block, d dest, directDefer bool) {
	n := len(b.Stmts)
	for i, s := range b.Stmts {
		if i == n-1 {
			if es, ok := s.(*ast.ExprStmt); ok {
				f.at(es)
				f.into(es.X, d)
				return
			}
		}
		f.stmt(s)
	}
	if n > 0 && f.stmtNever(b.Stmts[n-1]) {
		return
	}
	f.deliverUnit(d)
}

// stmtNever reports whether a statement never completes.
func (f *fnGen) stmtNever(s ast.Stmt) bool {
	switch x := s.(type) {
	case *ast.ReturnStmt, *ast.BreakStmt, *ast.ContinueStmt:
		return true
	case *ast.ExprStmt:
		if t := f.info.TypeOf(x.X); t != nil && check.IsNever(t) {
			return true
		}
		if w, ok := x.X.(*ast.WhileExpr); ok {
			return !loopHasBreak(w.Body, "")
		}
	case *ast.WhileStmt:
		if x.Cond == nil {
			label := ""
			if x.Label != nil {
				label = x.Label.Name
			}
			return !loopHasBreak(x.Body, label)
		}
	}
	return false
}

// loopHasBreak reports whether a loop body contains a break that leaves
// the loop (unlabeled at depth 0, or with the loop's label).
func loopHasBreak(body *ast.Block, label string) bool {
	found := false
	var walk func(n ast.Node, depth int)
	walk = func(n ast.Node, depth int) {
		inspectNode(n, func(m ast.Node) bool {
			if found {
				return false
			}
			switch x := m.(type) {
			case *ast.BreakStmt:
				if x.Label == nil && depth == 0 || x.Label != nil && x.Label.Name == label && label != "" {
					found = true
				}
			case *ast.ForStmt:
				if m != n {
					walk(x.Body, depth+1)
					return false
				}
			case *ast.WhileStmt:
				if m != n {
					walk(x.Body, depth+1)
					return false
				}
			case *ast.WhileExpr:
				if m != n {
					walk(x.Body, depth+1)
					return false
				}
			case *ast.FnLit:
				return false
			}
			return true
		})
	}
	walk(body, 0)
	return found
}

func (f *fnGen) stmt(s ast.Stmt) {
	f.at(s)
	switch x := s.(type) {
	case *ast.ExprStmt:
		f.into(x.X, dest{kind: dDiscard})
	case *ast.LetStmt:
		f.letStmt(x)
	case *ast.VarStmt:
		v := f.info.VarOf(x.Name)
		f.noteBinding(x.Value, v)
		code := f.val(x.Value)
		if v == nil {
			f.line("_ = %s", code)
			return
		}
		name := f.declName(v)
		f.line("var %s %s = %s", name, f.goType(v.Type), code)
		f.line("_ = %s", name)
	case *ast.ConstDecl:
		v := f.info.VarOf(x.Name)
		code := f.val(x.Value)
		if v == nil {
			f.line("_ = %s", code)
			return
		}
		name := f.declName(v)
		f.line("%s := %s", name, code)
		f.line("_ = %s", name)
	case *ast.AssignStmt:
		f.assign(x)
	case *ast.DeferStmt:
		f.deferStmt(x)
	case *ast.ReturnStmt:
		if x.Value == nil {
			f.retUnit()
			return
		}
		f.ret(x.Value)
	case *ast.BreakStmt:
		if l := f.findLoop(x.Label); l != nil {
			f.breakLoop(l)
		}
	case *ast.ContinueStmt:
		if l := f.findLoop(x.Label); l != nil {
			f.continueLoop(l)
		}
	case *ast.ForStmt:
		f.forStmt(x)
	case *ast.WhileStmt:
		f.loopWhile(x.Label, x.Cond, x.Body, x)
	case *ast.BadStmt:
		f.g.errorf(x, "bad statement")
	}
}

func (f *fnGen) findLoop(label *ast.Ident) *loopInfo {
	ls := f.fr.loops
	if len(ls) == 0 {
		return nil
	}
	if label == nil {
		return ls[len(ls)-1]
	}
	for i := len(ls) - 1; i >= 0; i-- {
		if ls[i].label == label.Name {
			return ls[i]
		}
	}
	return nil
}

func (f *fnGen) letStmt(x *ast.LetStmt) {
	if bp, ok := x.Pattern.(*ast.BindPat); ok {
		v := f.info.VarOf(bp.Name)
		f.noteBinding(x.Value, v)
		code := f.val(x.Value)
		if v == nil {
			f.line("_ = %s", code)
			return
		}
		name := f.declName(v)
		f.line("var %s %s = %s", name, f.goType(v.Type), code)
		f.line("_ = %s", name)
		return
	}
	if _, ok := x.Pattern.(*ast.WildcardPat); ok {
		f.discard(x.Value)
		return
	}
	code := f.val(x.Value)
	subj := f.tmp("let")
	f.line("%s := %s", subj, code)
	f.line("_ = %s", subj)
	t := f.info.PatType(x.Pattern)
	if t == nil {
		t = f.typeOf(x.Value)
	}
	m := f.pattern(x.Pattern, subj, t)
	f.declareBinds(m)
}

// deferStmt registers deferred code (core 3.5): a Go defer of a shielded
// closure. The enclosing block runs in its own Go function (block), so Go's
// function scoped defer gives Xo's block scoped one.
func (f *fnGen) deferStmt(x *ast.DeferStmt) {
	loc := f.g.loc(x)
	saved := f.fr
	// deferred code is its own frame for control flow: it cannot return
	// from or break out of the enclosing function.
	fr := &frame{parent: saved, name: saved.name, params: saved.params, resT: check.Type(nil), isTest: saved.isTest}
	fr.goRes = ""
	f.fr = fr
	body := f.capture(func() {
		f.ind++
		f.stmt(x.Body)
		f.ind--
	})
	f.fr = saved
	f.line("defer tk.Shielded(%q, func() {", loc)
	f.out.WriteString(body)
	f.line("})")
}

// ---------------------------------------------------------------------------
// Loops

func (f *fnGen) pushLoop(label *ast.Ident) *loopInfo {
	l := &loopInfo{goLabel: f.tmp("L"), depth: f.fr.iife}
	if label != nil {
		l.label = label.Name
	}
	f.fr.loops = append(f.fr.loops, l)
	return l
}

func (f *fnGen) popLoop() { f.fr.loops = f.fr.loops[:len(f.fr.loops)-1] }

// emitLoop writes `label: for header {` body `}`, with the label only when
// a break or continue uses it.
func (f *fnGen) emitLoop(l *loopInfo, header, body string) {
	if l.used {
		f.out.WriteString(l.goLabel + ":\n")
	}
	if header == "" {
		f.line("for {")
	} else {
		f.line("for %s {", header)
	}
	f.out.WriteString(body)
	f.line("}")
}

func (f *fnGen) loopWhile(label *ast.Ident, cond ast.Expr, body *ast.Block, at ast.Node) {
	l := f.pushLoop(label)
	text := f.capture(func() {
		f.ind++
		if cond != nil {
			c := f.val(cond)
			f.line("if !(%s) {", c)
			f.line("\tbreak")
			f.line("}")
		}
		f.block(body, dest{kind: dDiscard})
		f.ind--
	})
	f.popLoop()
	f.emitLoop(l, "", text)
}

func (f *fnGen) forStmt(x *ast.ForStmt) {
	if f.arenasOn() {
		f.noteArenaTemp(x.Iter)
	}
	it := f.val(x.Iter)
	itT := f.typeOf(x.Iter)
	itv := f.tmp("it")
	f.line("%s := %s", itv, it)
	l := f.pushLoop(x.Label)
	n := f.g.named(itT)
	kind := ""
	if n != nil && n.Obj.Module.IsPrelude {
		kind = n.Obj.Name
	}
	elemT := f.info.PatType(x.Pattern)
	var header string
	var elem string
	switch kind {
	case "List", "Buf":
		elem = f.tmp("e")
		header = fmt.Sprintf("_, %s := range %s.Items()", elem, itv)
	case "Set":
		elem = f.tmp("e")
		header = fmt.Sprintf("%s := range %s.All()", elem, itv)
	case "Map":
		k, v := f.tmp("k"), f.tmp("v")
		header = fmt.Sprintf("%s, %s := range %s.All()", k, v, itv)
		elem = fmt.Sprintf("%sTuple2[%s, %s]{F0: %s, F1: %s}", f.rt(), f.goType(n.Args[0]), f.goType(n.Args[1]), k, v)
		if tp, ok := x.Pattern.(*ast.TuplePat); ok && len(tp.Elems) == 2 {
			elem = ""
			body := f.capture(func() {
				f.ind++
				f.line("_, _ = %s, %s", k, v)
				m1 := f.pattern(tp.Elems[0], k, n.Args[0])
				m2 := f.pattern(tp.Elems[1], v, n.Args[1])
				f.declareBinds(m1)
				f.declareBinds(m2)
				f.block(x.Body, dest{kind: dDiscard})
				f.ind--
			})
			f.popLoop()
			f.emitLoop(l, header, body)
			return
		}
	case "Chan":
		// receive until closed and drained (core 3.5, 7.3); M_recv is a
		// cancellation point that unwinds
		rv := f.tmp("rv")
		elem = rv + ".V"
		body := f.capture(func() {
			f.ind++
			f.line("%s := %s.M_recv(tk)", rv, itv)
			f.line("if !%s.Some {", rv)
			f.line("\tbreak")
			f.line("}")
			ev := f.tmp("el")
			f.line("%s := %s", ev, elem)
			f.line("_ = %s", ev)
			m := f.pattern(x.Pattern, ev, elemT)
			f.declareBinds(m)
			f.block(x.Body, dest{kind: dDiscard})
			f.ind--
		})
		f.popLoop()
		f.emitLoop(l, "", body)
		return
	case "Range":
		elem = f.tmp("i")
		header = fmt.Sprintf("%s := range %s.All()", elem, itv)
		if rng, ok := unparen(x.Iter).(*ast.RangeExpr); ok {
			// a range literal: a counted loop, not an iterator (no
			// closure call per iteration); bounds are evaluated once
			if rng.Inclusive {
				// stop after Hi without computing Hi + 1 into the
				// binding (Hi may be the maximum value)
				more := f.tmp("more")
				header = fmt.Sprintf("%s, %s := %s.Lo, %s.Lo <= %s.Hi; %s; %s, %s = %s+1, %s < %s.Hi",
					elem, more, itv, itv, itv, more, elem, more, elem, elem, itv)
			} else {
				header = fmt.Sprintf("%s := %s.Lo; %s < %s.Hi; %s++", elem, itv, elem, itv, elem)
			}
		}
	default:
		f.g.errorf(x.Iter, "cannot iterate over %s", itT)
		f.popLoop()
		return
	}
	body := f.capture(func() {
		f.ind++
		ev := f.tmp("el")
		f.line("%s := %s", ev, elem)
		f.line("_ = %s", ev)
		m := f.pattern(x.Pattern, ev, elemT)
		f.declareBinds(m)
		f.block(x.Body, dest{kind: dDiscard})
		f.ind--
	})
	f.popLoop()
	f.emitLoop(l, header, body)
}

// ---------------------------------------------------------------------------
// Assignment (core 5.2)

// pstep is one step of a place below its root binding.
type pstep struct {
	kind int    // psField, psBoxed, psList, psMap
	name string // Go field (F_x, F0, F1, ...)
	idx  string // index temporary (psList, psMap)
	loc  string
}

const (
	psField = iota // addressable field, tuple field, or newtype value
	psBoxed        // field stored in an rt.Box
	psList         // List element
	psMap          // Map value
)

// placeRef is a resolved place: a root binding and steps. Index operands
// are evaluated when the place is resolved (left to right, before the
// assigned value).
type placeRef struct {
	root  string
	steps []pstep
}

// rmw reports whether writing the place needs read, modify, write (a box
// or a collection element on the path).
func (p *placeRef) rmw() bool {
	for _, s := range p.steps {
		if s.kind != psField {
			return true
		}
	}
	return false
}

// resolvePlace resolves an assignable place (core 3.5 Place).
func (f *fnGen) resolvePlace(e ast.Expr) *placeRef {
	switch x := e.(type) {
	case *ast.Ident:
		if v := f.info.VarOf(x); v != nil {
			return &placeRef{root: f.varName(v)}
		}
	case *ast.ParenExpr:
		return f.resolvePlace(x.X)
	case *ast.SelectorExpr:
		base := f.resolvePlace(x.X)
		bt := f.typeOf(x.X)
		var st pstep
		switch {
		case isDigits(x.Sel.Name):
			st = pstep{kind: psField, name: "F" + x.Sel.Name}
		case f.g.named(bt) != nil && f.g.named(bt).Obj.Kind == check.KNewtype:
			st = pstep{kind: psField, name: "F0"}
		case f.g.boxedFieldNamed(bt, x.Sel.Name):
			st = pstep{kind: psBoxed, name: goField(x.Sel.Name)}
		default:
			st = pstep{kind: psField, name: goField(x.Sel.Name)}
		}
		base.steps = append(base.steps, st)
		return base
	case *ast.IndexExpr:
		base := f.resolvePlace(x.X)
		n := f.g.named(f.typeOf(x.X))
		kind := psList
		if n != nil && n.Obj.Module.IsPrelude && n.Obj.Name == "Map" {
			kind = psMap
		}
		iv := f.tmp("ix")
		f.line("%s := %s", iv, f.val(x.Index))
		base.steps = append(base.steps, pstep{kind: kind, idx: iv, loc: f.g.loc(x)})
		return base
	}
	f.g.errorf(e, "not an assignable place")
	return &placeRef{root: "_"}
}

// readPlace is the current value of a place.
func (f *fnGen) readPlace(p *placeRef) string {
	code := p.root
	for _, st := range p.steps {
		switch st.kind {
		case psField:
			code += "." + st.name
		case psBoxed:
			code += "." + st.name + ".Get()"
		default:
			code += fmt.Sprintf(".At(%s, %q)", st.idx, st.loc)
		}
	}
	return code
}

// writePlace stores v into a place, writing modified copies back through
// boxes and collection elements.
func (f *fnGen) writePlace(p *placeRef, v string) {
	k := -1
	for i, st := range p.steps {
		if st.kind != psField {
			k = i
		}
	}
	if k < 0 {
		f.line("%s = %s", f.readPlace(p), v)
		return
	}
	outer := &placeRef{root: p.root, steps: p.steps[:k]}
	tmp := f.tmp("rw")
	f.line("%s := %s", tmp, f.readPlace(outer))
	st := p.steps[k]
	inner := v
	if k < len(p.steps)-1 {
		cur := f.tmp("rw")
		switch st.kind {
		case psBoxed:
			f.line("%s := %s.%s.Get()", cur, tmp, st.name)
		default:
			f.line("%s := %s.At(%s, %q)", cur, tmp, st.idx, st.loc)
		}
		f.writePlace(&placeRef{root: cur, steps: p.steps[k+1:]}, v)
		inner = cur
	}
	switch st.kind {
	case psBoxed:
		f.line("%s.%s = %sBoxOf(%s)", tmp, st.name, f.rt(), inner)
	case psList:
		f.line("%s.SetAt(%s, %s, %q)", tmp, st.idx, inner, st.loc)
	case psMap:
		f.line("%s.M_put(tk, %s, %s)", tmp, st.idx, inner)
	}
	f.writePlace(outer, tmp)
}

func (f *fnGen) assign(x *ast.AssignStmt) {
	tt := f.typeOf(x.Target)
	ref := f.resolvePlace(x.Target)
	if x.Op == assignOp {
		f.writePlace(ref, f.val(x.Value))
		return
	}
	op := compoundOp(x.Op)
	yt := f.nat(x.Value)
	yv := f.coerce(f.expr(x.Value), yt, yt)
	cur := f.readPlace(ref)
	f.writePlace(ref, f.binop(op, cur, yv, tt, yt, tt, x))
}

// place is the addressable Go expression of a place, for `var` receivers
// and `&` arguments. A place through a box or a collection element is
// copied into a temporary; wb records how to write it back after the call.
func (f *fnGen) place(e ast.Expr) string {
	ref := f.resolvePlace(e)
	if !ref.rmw() {
		return f.readPlace(ref)
	}
	tmp := f.tmp("pl")
	f.line("%s := %s", tmp, f.readPlace(ref))
	f.writebacks = append(f.writebacks, func() { f.writePlace(ref, tmp) })
	return tmp
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Source text

var sourceCache = map[string][]byte{}

// source returns the source text of a node, for contract and expect
// messages.
func (g *gen) source(n ast.Node) string {
	sp := n.Span()
	src, ok := sourceCache[sp.File]
	if !ok {
		src, _ = os.ReadFile(sp.File)
		sourceCache[sp.File] = src
	}
	if sp.Start.Offset < 0 || sp.End.Offset > len(src) || sp.Start.Offset > sp.End.Offset {
		return ""
	}
	return string(src[sp.Start.Offset:sp.End.Offset])
}
