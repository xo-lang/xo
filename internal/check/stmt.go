package check

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// retWant is the expected type of a value in return position of a function
// declared `-> T ! E` (core 2.6): a T, `Err(e)`, or a Result[T, E].
type retWant struct {
	T Type
	E Type
}

func (r *retWant) String() string { return r.T.String() + " ! " + r.E.String() }

// discardWant marks a value that is discarded: an expression statement or
// the tail of a block whose value is not used. A discarded Result is XO0201.
type discardWant struct{}

func (discardWant) String() string { return "()" }

var discard = discardWant{}

// scope is a block scope of local bindings.
type scope struct {
	parent *scope
	names  map[string]*VarObj
	order  []*VarObj
}

// fnCtx is the function, closure, or test whose body is being checked.
type fnCtx struct {
	parent  *fnCtx
	fn      *FuncObj
	name    string
	result  Type
	errT    Type // nil: cannot fail
	errVar  *Var // inferred closure error type
	eff     *EffSet
	isTest  bool
	closure bool
	loops   []*loopFrame
	// ensures is set while checking an ensures clause (`result`, `old`).
	ensures bool
	// groups holds the tasks started on `os.tasks` of this main or test;
	// ext the tasks started on groups whose origin is not visible (core
	// 7.1, groups.go).
	groups *scopeFrame
	ext    *scopeFrame
}

type loopFrame struct {
	label    string
	hasBreak bool
	infinite bool
}

// scopeFrame tracks the tasks spawned on one `scope s` handle (core 7.1).
type scopeFrame struct {
	ctx    *fnCtx
	spawns []*spawnSite
	span   token.Span
}

type spawnSite struct {
	call  *ast.CallExpr
	errT  Type
	bound *VarObj
}

func (c *checker) pushScope() {
	c.sc = &scope{parent: c.sc, names: map[string]*VarObj{}}
}

// popScope closes the current scope and collects unused bindings, which
// are reported after the flow checks (XO0110).
func (c *checker) popScope() {
	for _, v := range c.sc.order {
		if v.Kind != VLocal || v.Used {
			continue
		}
		if strings.HasPrefix(v.Name, "_") && !c.isResultType(v.Type) {
			continue
		}
		c.unused = append(c.unused, v)
	}
	c.sc = c.sc.parent
}

// reportUnused reports XO0110 for unused bindings, except those already
// reported as lost updates (XO0520 is the more precise diagnostic).
func (c *checker) reportUnused() {
	for _, v := range c.unused {
		if c.lostVars[v] {
			continue
		}
		if _, et, ok := c.resultParts(v.Type); ok {
			// A Result bound to a name that is never read is an ignored
			// error, even when the name starts with `_` (core 2.6).
			d := c.errorf(diag.UnhandledResult, v.Span, "result bound to `%s` is never used, so its error is ignored", v.Name)
			if d != nil {
				note(d, token.Span{}, "the value can fail with `%s`; propagate with `?`, match on it, or discard it with `.ignore_err(\"reason\")` (core 2.6)", deep(et))
				if ls, ok := v.decl.(*ast.LetStmt); ok {
					if _, single := ls.Pattern.(*ast.BindPat); single {
						fix(d, "discard with a reason", append([]diag.Edit{replaceEdit(v.Span, "_")}, ignoreErrEdits(ls.Value)...)...)
					}
				}
			}
			continue
		}
		d := c.errorf(diag.UnusedBinding, v.Span, "`%s` is never used", v.Name)
		if d != nil && c.dollarHabit(d, v) {
			continue
		}
		if d != nil {
			note(d, token.Span{}, "prefix the name with `_` or write `_` to discard it (core 3.1)")
			fix(d, fmt.Sprintf("rename to `_%s`", v.Name), replaceEdit(v.Span, "_"+v.Name))
		}
	}
}

// declareVar adds a binding to the current scope. Shadowing in the same
// block is an error (core 3.1).
func (c *checker) declareVar(v *VarObj) {
	if v.Name == "_" {
		return
	}
	v.ctx = c.ctx
	if prev, ok := c.sc.names[v.Name]; ok {
		d := c.errorf(diag.Duplicate, v.Span, "`%s` is already declared in this block; shadowing is allowed only in a nested block (core 3.1)", v.Name)
		note(d, prev.Span, "previous declaration")
		prev.Used = true
	}
	c.sc.names[v.Name] = v
	c.sc.order = append(c.sc.order, v)
}

func (c *checker) lookupLocal(name string) *VarObj {
	for s := c.sc; s != nil; s = s.parent {
		if v, ok := s.names[name]; ok {
			return v
		}
	}
	return nil
}

// perform records effects in the current function or closure.
func (c *checker) perform(s *EffSet) {
	if c.ctx == nil || s == nil {
		return
	}
	// std's own logging stays ambient: a std function may write log
	// records without declaring `log` (decision 0156). The effect is
	// introduced only by the pub std/log functions, so dropping the
	// `log` label here, where a std module performs it, keeps it off
	// std's inferred effects; user code still gets it.
	if c.mod != nil && (c.mod.IsStd || c.mod.Native) {
		var kept []EffItem
		dropped := false
		for _, it := range s.Items {
			if it.Label == "log" {
				dropped = true
				continue
			}
			kept = append(kept, it)
		}
		if dropped {
			s = &EffSet{Items: kept}
		}
	}
	c.ctx.eff.addAll(s)
}

// ---------------------------------------------------------------------------
// Modules and bodies

func (c *checker) checkModule(m *Module) {
	c.mod = m
	for _, f := range m.Files {
		c.file = f
		for _, d := range f.AST.Decls {
			switch x := d.(type) {
			case *ast.FnDecl:
				c.checkFnDecl(x)
			case *ast.TestDecl:
				c.checkTest(x)
			case *ast.ConstDecl:
				if co, ok := m.Scope[x.Name.Name].(*ConstObj); ok && co.Decl == x {
					c.constType(co)
				}
			case *ast.StructDecl:
				if t, ok := m.Scope[x.Name.Name].(*TypeObj); ok && t.structDecl == x {
					c.checkStructExtras(t)
					c.checkDerives(t, x.Derive)
				}
			case *ast.EnumDecl:
				if t, ok := m.Scope[x.Name.Name].(*TypeObj); ok && t.enumDecl == x {
					c.checkDerives(t, x.Derive)
				}
			}
		}
	}
}

// findFuncObj returns the FuncObj created for a declaration.
func (c *checker) findFuncObj(fd *ast.FnDecl) *FuncObj {
	if fd.Recv == nil {
		if fo, ok := c.mod.Scope[fd.Name.Name].(*FuncObj); ok && fo.Decl == fd {
			return fo
		}
		return nil
	}
	for _, o := range c.mod.Scope {
		t, ok := o.(*TypeObj)
		if !ok {
			continue
		}
		if fo := t.Methods[fd.Name.Name]; fo != nil && fo.Decl == fd {
			return fo
		}
		if fo := t.Statics[fd.Name.Name]; fo != nil && fo.Decl == fd {
			return fo
		}
	}
	return nil
}

func (c *checker) withCtx(ctx *fnCtx, f func()) {
	savedCtx, savedSc := c.ctx, c.sc
	c.ctx = ctx
	f()
	c.ctx, c.sc = savedCtx, savedSc
}

// sinkCtx is a context for expressions outside function bodies (defaults,
// constants, invariants): their effects go to a throwaway set.
func (c *checker) sinkCtx() *fnCtx {
	return &fnCtx{name: "initializer", result: unitType, eff: &EffSet{}}
}

func (c *checker) checkFnDecl(fd *ast.FnDecl) {
	fo := c.findFuncObj(fd)
	if fo == nil || !fo.sigDone {
		return
	}
	ctx := &fnCtx{fn: fo, name: fo.DisplayName(), result: fo.Result, errT: fo.Err, eff: &EffSet{}}
	fo.body = ctx.eff
	c.withCtx(ctx, func() {
		c.sc = nil
		c.pushScope()
		var params []*VarObj
		if fd.Recv != nil && !fo.Static {
			rv := &VarObj{Name: fd.Recv.Name.Name, Type: fo.RecvType, Mutable: fd.Recv.Var, Kind: VRecv, Span: fd.Recv.Name.Sp, Used: true}
			c.declareVar(rv)
			params = append(params, rv)
			c.rec.recvVar[fo] = rv
		}
		for i, p := range fo.Params {
			pv := &VarObj{Name: p.Name, Type: p.Type, Mutable: p.Var, Kind: VParam, Span: p.Span, Used: true}
			c.declareVar(pv)
			params = append(params, pv)
			c.rec.paramVars[fo] = append(c.rec.paramVars[fo], pv)
			if p.Default != nil {
				c.checkDefault(p.Default, p.Type)
			}
			_ = i
		}
		c.checkClauses(fd, fo)
		if fd.Body != nil {
			c.checkBody(fd.Body, ctx)
			c.bodies = append(c.bodies, &bodyInfo{body: fd.Body, params: params})
		}
		c.finishGroups(ctx)
		c.popScope()
	})
	if fo.Declared != nil && fd.Body != nil {
		name := fo.DisplayName()
		pub := fo.Pub
		c.obligations = append(c.obligations, effObligation{sub: ctx.eff, sup: fo.Declared, span: fd.Name.Sp, fnDecl: fd, what: func(s string) string {
			if s == "" {
				if pub {
					return fmt.Sprintf("`%s` is pub with no `uses` clause, so it is pure", name)
				}
				return fmt.Sprintf("`%s` is pure", name)
			}
			return fmt.Sprintf("`%s` declares `uses %s`", name, s)
		}})
	}
}

// bodyWant is the expected type of a function body.
func (c *checker) bodyWant(ctx *fnCtx) Type {
	if ctx.errT != nil {
		return &retWant{T: ctx.result, E: ctx.errT}
	}
	if isUnit(ctx.result) {
		return discard
	}
	return ctx.result
}

func (c *checker) checkBody(b *ast.Block, ctx *fnCtx) {
	c.block(b, c.bodyWant(ctx))
}

func (c *checker) checkDefault(e ast.Expr, t Type) {
	c.withCtx(c.sinkCtx(), func() {
		saved := c.sc
		c.sc = nil
		c.pushScope()
		c.check(e, t)
		c.popScope()
		c.sc = saved
	})
}

func (c *checker) checkClauses(fd *ast.FnDecl, fo *FuncObj) {
	for _, cl := range fd.Clauses {
		switch x := cl.(type) {
		case *ast.RequiresClause:
			c.pureClause(x.Cond, func() { c.check(x.Cond, c.named("Bool")) })
			if x.Else != nil {
				if fo.Err == nil {
					d := c.errorf(diag.TryNeedsFailing, x.Else.Span(), "`requires ... else` returns an error, but `%s` cannot fail", fo.DisplayName())
					fix(d, "declare an error type: `! E`", insertEdit(fd.Sp.File, fdHeaderEnd(fd), " ! Error"))
					continue
				}
				c.pureClause(x.Else, func() {
					// `requires c else e` returns Err(e): e must have the
					// declared error type (core 9.1), with only the implicit
					// conversions of core 2.10 that apply to a return.
					c.check(x.Else, fo.Err)
				})
			}
		case *ast.EnsuresClause:
			c.pushScope()
			rv := &VarObj{Name: "result", Type: fo.Result, Kind: VPredeclared, Span: x.Sp, Used: true}
			c.declareVar(rv)
			c.rec.resultVar[x] = rv
			saved := c.ctx.ensures
			c.ctx.ensures = true
			c.pureClause(x.Cond, func() { c.check(x.Cond, c.named("Bool")) })
			c.ctx.ensures = saved
			c.sc.names["result"].Used = true
			c.popScope()
		}
	}
}

func fdHeaderEnd(fd *ast.FnDecl) token.Pos {
	switch {
	case fd.Result != nil:
		return fd.Result.Span().End
	case len(fd.Params) > 0:
		return fd.Params[len(fd.Params)-1].Sp.End
	}
	return fd.Name.Sp.End
}

// pureClause checks a contract expression whose effects must be empty
// (core 9.1).
func (c *checker) pureClause(e ast.Expr, f func()) {
	saved := c.ctx.eff
	sink := &EffSet{}
	c.ctx.eff = sink
	f()
	c.ctx.eff = saved
	if len(sink.Items) > 0 {
		c.obligations = append(c.obligations, effObligation{sub: sink, sup: &EffSet{}, span: e.Span(), pure: true})
	}
}

// checkStructExtras checks field defaults and the invariant of a struct.
func (c *checker) checkStructExtras(t *TypeObj) {
	for _, f := range t.Fields {
		if f.Default != nil {
			c.checkDefault(f.Default, f.Type)
		}
	}
	if t.Invariant == nil {
		return
	}
	c.withCtx(c.sinkCtx(), func() {
		c.sc = nil
		c.pushScope()
		for _, f := range t.Fields {
			fv := &VarObj{Name: f.Name, Type: f.Type, Kind: VParam, Span: f.Span, Used: true}
			c.declareVar(fv)
			c.rec.invVars[t] = append(c.rec.invVars[t], fv)
		}
		c.pureClause(t.Invariant, func() { c.check(t.Invariant, c.named("Bool")) })
		c.popScope()
	})
}

func (c *checker) checkTest(td *ast.TestDecl) {
	ctx := &fnCtx{name: "test", result: unitType, errT: c.named("Error"), eff: &EffSet{}, isTest: true}
	c.withCtx(ctx, func() {
		c.sc = nil
		c.pushScope()
		testing := c.modules["std/testing"]
		osv := &VarObj{Name: "os", Type: &Named{Obj: testing.Scope["TestOs"].(*TypeObj)}, Kind: VPredeclared, Used: true}
		c.sc.names["os"] = osv
		c.rec.testOs[td] = osv
		osv.ctx = ctx
		env := &typeEnv{file: c.file, mod: c.mod, params: map[string]*TParam{}}
		for _, p := range td.For {
			c.checkName(p.Name, "parameter")
			var pt Type = badType
			if p.Type != nil {
				pt = c.resolveType(p.Type, env)
			}
			pv := &VarObj{Name: p.Name.Name, Type: pt, Kind: VParam, Span: p.Name.Sp, Used: true}
			c.declareVar(pv)
			c.rec.testParams[td] = append(c.rec.testParams[td], pv)
		}
		if len(td.Uses) > 0 {
			c.resolveEffects(td.Uses, env)
		}
		if td.Body != nil {
			c.checkBody(td.Body, ctx)
			c.bodies = append(c.bodies, &bodyInfo{body: td.Body})
		}
		c.finishGroups(ctx)
		c.popScope()
	})
}

// constType returns the type of a constant, checking its initializer on
// first use.
func (c *checker) constType(co *ConstObj) Type {
	if co.done {
		return co.Type
	}
	if co.checking {
		c.errorf(diag.WrongKind, co.Decl.Name.Sp, "constant `%s` refers to itself", co.Name)
		co.Type = badType
		return badType
	}
	co.checking = true
	savedMod, savedFile := c.mod, c.file
	c.mod, c.file = co.Module, co.File
	var want Type
	if co.Decl.Type != nil {
		want = c.resolveType(co.Decl.Type, &typeEnv{file: co.File, mod: co.Module, params: map[string]*TParam{}})
	}
	var t Type
	c.withCtx(c.sinkCtx(), func() {
		saved := c.sc
		c.sc = nil
		c.pushScope()
		if want != nil {
			t = c.check(co.Decl.Value, want)
		} else {
			t = c.infer(co.Decl.Value, nil)
		}
		c.popScope()
		c.sc = saved
	})
	if !co.Module.IsStd {
		c.checkConstExpr(co.Decl.Value)
	}
	c.mod, c.file = savedMod, savedFile
	co.Type = deep(t)
	co.done, co.checking = true, false
	return co.Type
}

// checkConstExpr reports XO0408 for initializer parts that are not compile
// time values (core 3.1).
func (c *checker) checkConstExpr(e ast.Expr) {
	bad := func(n ast.Node, what string) {
		c.errorf(diag.ConstNotConstant, n.Span(), "%s is not a compile time value; a const initializer may use literals, other constants, struct and enum construction, and collection literals of those (core 3.1)", what)
	}
	switch x := e.(type) {
	case *ast.IntLit, *ast.FloatLit, *ast.DurationLit, *ast.RuneLit, *ast.BytesLit, *ast.UnitExpr, *ast.BadExpr:
	case *ast.StringLit:
		for _, p := range x.Parts {
			if p.Expr != nil {
				c.checkConstExpr(p.Expr)
			}
		}
	case *ast.Ident:
		// constants, true/false, unit variants; other names were already
		// reported by the type check if they are not values.
		if v := c.identObj[x]; v != nil && !v.isConst {
			bad(x, "a local binding")
		}
	case *ast.SelectorExpr:
		c.checkConstExpr(x.X)
	case *ast.ParenExpr:
		c.checkConstExpr(x.X)
	case *ast.UnaryExpr:
		c.checkConstExpr(x.X)
	case *ast.BinaryExpr:
		c.checkConstExpr(x.X)
		c.checkConstExpr(x.Y)
	case *ast.TupleExpr:
		for _, el := range x.Elems {
			c.checkConstExpr(el)
		}
	case *ast.ListLit:
		for _, el := range x.Elems {
			c.checkConstExpr(el)
		}
	case *ast.SetLit:
		for _, el := range x.Elems {
			c.checkConstExpr(el)
		}
	case *ast.MapLit:
		for _, en := range x.Entries {
			c.checkConstExpr(en.Key)
			c.checkConstExpr(en.Value)
		}
	case *ast.StructLit:
		for _, f := range x.Fields {
			c.checkConstExpr(f.Value)
		}
	case *ast.CallExpr:
		if ci := c.callInfo[x]; ci != nil && ci.fn != nil {
			bad(x, fmt.Sprintf("the call to `%s`", ci.fn.DisplayName()))
			return
		}
		if c.isConstructorCall(x) {
			for _, a := range x.Args {
				c.checkConstExpr(a.Value)
			}
			return
		}
		bad(x, "a call")
	default:
		bad(e, "this expression")
	}
}

// ---------------------------------------------------------------------------
// Blocks and statements

// block checks a block against want and returns its type.
func (c *checker) block(b *ast.Block, want Type) Type {
	c.pushScope()
	defer c.popScope()
	n := len(b.Stmts)
	var t Type = unitType
	for i, s := range b.Stmts {
		if i == n-1 {
			if es, ok := s.(*ast.ExprStmt); ok {
				if want == nil {
					return c.infer(es.X, nil)
				}
				return c.check(es.X, want)
			}
		}
		c.stmt(s)
	}
	if n > 0 && c.stmtNever(b.Stmts[n-1]) {
		t = c.named("Never")
	}
	if want == nil {
		return t
	}
	end := b.Sp
	end.Start = b.Sp.End
	if end.Start.Col > 1 {
		end.Start.Col--
		end.Start.Offset--
	}
	return c.coerceTo(t, want, end, b)
}

// blockNever reports whether a block never completes normally: its last
// statement is an infinite loop without `break`, a jump, or an expression
// of type `Never`.
func (c *checker) blockNever(b *ast.Block) bool {
	n := len(b.Stmts)
	if n == 0 {
		return false
	}
	if es, ok := b.Stmts[n-1].(*ast.ExprStmt); ok {
		if w, ok := es.X.(*ast.WhileExpr); ok {
			return !hasBreak(w.Body, "", 0)
		}
		if c.neverExpr[es.X] {
			return true
		}
	}
	return c.stmtNever(b.Stmts[n-1])
}

// stmtNever reports whether a statement never completes.
func (c *checker) stmtNever(s ast.Stmt) bool {
	switch x := s.(type) {
	case *ast.ReturnStmt, *ast.BreakStmt, *ast.ContinueStmt:
		return true
	case *ast.WhileStmt:
		return x.Cond == nil && x.Pattern == nil && !hasBreak(x.Body, labelName(x.Label), 0)
	case *ast.ExprStmt:
		if t, ok := c.exprType[x.X]; ok {
			return c.isNever(t)
		}
	}
	return false
}

func labelName(id *ast.Ident) string {
	if id == nil {
		return ""
	}
	return id.Name
}

// hasBreak reports whether body contains a break that exits the loop with
// the given label (depth counts nested loops).
func hasBreak(n ast.Node, label string, depth int) bool {
	found := false
	var walk func(n ast.Node, depth int)
	walk = func(n ast.Node, depth int) {
		if found || n == nil {
			return
		}
		switch x := n.(type) {
		case *ast.BreakStmt:
			if (x.Label == nil && depth == 0) || (x.Label != nil && x.Label.Name == label && label != "") {
				found = true
			}
			return
		case *ast.ForStmt:
			walk(x.Body, depth+1)
			return
		case *ast.WhileStmt:
			walk(x.Body, depth+1)
			return
		case *ast.WhileExpr:
			walk(x.Body, depth+1)
			return
		case *ast.FnLit:
			return
		}
		for _, ch := range children(n) {
			walk(ch, depth)
		}
	}
	walk(n, depth)
	return found
}

func (c *checker) stmt(s ast.Stmt) {
	switch x := s.(type) {
	case *ast.ExprStmt:
		c.check(x.X, discard)
	case *ast.LetStmt:
		c.letStmt(x)
	case *ast.VarStmt:
		c.varStmt(x)
	case *ast.ConstDecl:
		c.localConst(x)
	case *ast.AssignStmt:
		c.assign(x)
	case *ast.DeferStmt:
		c.stmt(x.Body)
	case *ast.ReturnStmt:
		c.returnStmt(x)
	case *ast.BreakStmt:
		c.jump(x.Label, x.Sp, true)
	case *ast.ContinueStmt:
		c.jump(x.Label, x.Sp, false)
	case *ast.ForStmt:
		c.forStmt(x)
	case *ast.WhileStmt:
		if x.Pattern != nil {
			c.whileLet(x)
			return
		}
		if x.Cond != nil {
			c.check(x.Cond, c.named("Bool"))
		}
		c.checkLoopBound(x.Bound, nil)
		c.loopBody(labelName(x.Label), x.Cond == nil, x.Body, nil)
	case *ast.BadStmt:
	}
}

func (c *checker) loopBody(label string, infinite bool, body *ast.Block, before func()) *loopFrame {
	lf := &loopFrame{label: label, infinite: infinite}
	c.ctx.loops = append(c.ctx.loops, lf)
	c.pushScope()
	if before != nil {
		before()
	}
	c.block(body, discard)
	c.popScope()
	c.ctx.loops = c.ctx.loops[:len(c.ctx.loops)-1]
	return lf
}

func (c *checker) jump(label *ast.Ident, sp token.Span, isBreak bool) {
	kw := "continue"
	if isBreak {
		kw = "break"
	}
	if len(c.ctx.loops) == 0 {
		c.errorf(diag.BadJump, sp, "`%s` outside a loop", kw)
		return
	}
	if label == nil {
		if isBreak {
			c.ctx.loops[len(c.ctx.loops)-1].hasBreak = true
		}
		return
	}
	var names []string
	for i := len(c.ctx.loops) - 1; i >= 0; i-- {
		if c.ctx.loops[i].label == label.Name {
			if isBreak {
				c.ctx.loops[i].hasBreak = true
			}
			return
		}
		if c.ctx.loops[i].label != "" {
			names = append(names, c.ctx.loops[i].label)
		}
	}
	d := c.errorf(diag.BadJump, label.Sp, "loop label `%s` not found", label.Name)
	if s := nearest(label.Name, names); len(s) > 0 {
		note(d, token.Span{}, "did you mean `%s`?", s[0])
	}
}

func (c *checker) forStmt(x *ast.ForStmt) {
	it := c.infer(x.Iter, nil)
	var elem Type
	if x.GoRange {
		elem = c.goRange(x, it)
	} else {
		elem = c.iterElem(it, x.Iter)
	}
	c.checkLoopBound(x.Bound, x.Iter)
	c.loopBody(labelName(x.Label), false, x.Body, func() {
		ip := c.pattern(x.Pattern, elem, false)
		c.requireIrrefutable(ip, elem, x.Pattern, "a `for` pattern")
	})
}

// iterElem returns the element type of an iterable (List, Set, Map, Range,
// Chan). A `for` over a channel receives until it is closed and drained;
// each receive is a cancellation point (core 3.5, 7.3).
func (c *checker) iterElem(t Type, e ast.Expr) Type {
	r := resolve(t)
	if isBad(r) {
		return badType
	}
	if n, ok := r.(*Named); ok && n.Obj.Module == c.prelude {
		switch n.Obj.Name {
		case "List", "Set", "Range", "Buf", "Chan":
			return n.Args[0]
		case "Bytes":
			// each byte (decision 0138)
			return c.named("UInt8")
		case "Map":
			return &Tuple{Elems: []Type{n.Args[0], n.Args[1]}}
		}
	}
	if _, ok := r.(*Var); ok {
		c.errorf(diag.CannotInfer, e.Span(), "the type of the iterated value must be known here")
		return badType
	}
	c.errorf(diag.BadOperand, e.Span(), "cannot iterate over `%s`; `for` works on List, Set, Map, Chan, Bytes, and ranges", deep(t))
	return badType
}

func (c *checker) letStmt(x *ast.LetStmt) {
	var t Type
	if x.Type != nil {
		want := c.resolveType(x.Type, c.localEnv())
		t = c.check(x.Value, want)
		if isBad(t) {
			t = want
		}
	} else {
		t = c.infer(x.Value, nil)
	}
	if _, ok := x.Pattern.(*ast.WildcardPat); ok {
		if _, et, ok := c.resultParts(t); ok {
			c.discardNeedsReason(x, et)
		}
	}
	if bp, ok := x.Pattern.(*ast.BindPat); ok {
		c.checkName(bp.Name, "variable")
	}
	ip := c.pattern(x.Pattern, t, false)
	c.requireIrrefutable(ip, t, x.Pattern, "a `let` pattern")
	if bp, ok := x.Pattern.(*ast.BindPat); ok {
		if v := c.lookupLocal(bp.Name.Name); v != nil {
			v.decl = x
			c.noteSpawnBinding(x.Value, v)
			c.noteGroupBinding(x.Value, v)
		}
	}
}

// localConst checks a `const` in a block (core 3.1): an immutable binding
// whose initializer is a compile time value.
func (c *checker) localConst(x *ast.ConstDecl) {
	var t Type
	if x.Type != nil {
		want := c.resolveType(x.Type, c.localEnv())
		t = c.check(x.Value, want)
		if isBad(t) {
			t = want
		}
	} else {
		t = c.infer(x.Value, nil)
	}
	before := len(c.diags.Items)
	c.checkConstExpr(x.Value)
	if len(c.diags.Items) > before && strings.ToUpper(x.Name.Name) != x.Name.Name {
		// TypeScript's `const x = f()` is a run time binding: Xo's `let`
		// (decision 0087). One diagnostic instead of XO0408 and XO0101.
		c.diags.Items = c.diags.Items[:before]
		d := c.errorf(diag.ConstNotConstant, x.Value.Span(), "`const %s` is initialized with a value computed at run time", x.Name.Name)
		note(d, token.Span{}, "a TypeScript and JavaScript `const` binding is Xo's `let`; an Xo `const` is a compile time UPPER_SNAKE constant (core 3.1)")
		fix(d, "write `let`", replaceEdit(token.Span{File: x.Sp.File, Start: x.Sp.Start, End: token.Pos{Line: x.Sp.Start.Line, Col: x.Sp.Start.Col + 5, Offset: x.Sp.Start.Offset + 5}}, "let"))
		v := &VarObj{Name: x.Name.Name, Type: t, Kind: VLocal, Span: x.Name.Sp, decl: x}
		c.declareVar(v)
		c.identObj[x.Name] = v
		return
	}
	c.checkName(x.Name, "constant")
	v := &VarObj{Name: x.Name.Name, Type: t, Kind: VLocal, Span: x.Name.Sp, decl: x, isConst: true}
	c.declareVar(v)
	c.identObj[x.Name] = v
}

func (c *checker) varStmt(x *ast.VarStmt) {
	var t Type
	if x.Type != nil {
		want := c.resolveType(x.Type, c.localEnv())
		t = c.check(x.Value, want)
		if isBad(t) {
			t = want
		}
	} else {
		t = c.infer(x.Value, nil)
	}
	c.checkName(x.Name, "variable")
	v := &VarObj{Name: x.Name.Name, Type: t, Mutable: true, Kind: VLocal, Span: x.Name.Sp, isVarStmt: true, decl: x}
	c.declareVar(v)
	c.identObj[x.Name] = v
	c.noteSpawnBinding(x.Value, v)
	c.noteGroupBinding(x.Value, v)
}

// discardNeedsReason reports XO0206 for `let _ = <Result>` (core 2.6,
// decision 0031): an error is discarded only with `.ignore_err("reason")`.
func (c *checker) discardNeedsReason(x *ast.LetStmt, et Type) {
	d := c.errorf(diag.DiscardNeedsReason, x.Sp, "`let _ =` discards a Result, so its error is ignored without a reason")
	if d == nil {
		return
	}
	note(d, token.Span{}, "the value can fail with `%s`; propagate with `?`, match on it, or discard it with `.ignore_err(\"reason\")`, which yields an optional and records the reason (core 2.6)", deep(et))
	fix(d, "discard with a reason", ignoreErrEdits(x.Value)...)
}

// ignoreErrEdits appends `.ignore_err("TODO: reason")` to e, parenthesizing
// it when e is not a postfix expression.
func ignoreErrEdits(e ast.Expr) []diag.Edit {
	sp := e.Span()
	const call = `.ignore_err("TODO: reason")`
	switch e.(type) {
	case *ast.Ident, *ast.CallExpr, *ast.SelectorExpr, *ast.IndexExpr, *ast.TryExpr, *ast.ParenExpr, *ast.TypeArgsExpr:
		return []diag.Edit{insertEdit(sp.File, sp.End, call)}
	}
	return []diag.Edit{insertEdit(sp.File, sp.Start, "("), insertEdit(sp.File, sp.End, ")"+call)}
}

// localEnv is the type environment inside the current body: the type
// parameters of the enclosing function.
func (c *checker) localEnv() *typeEnv {
	env := &typeEnv{file: c.file, mod: c.mod, params: map[string]*TParam{}}
	for ctx := c.ctx; ctx != nil; ctx = ctx.parent {
		if ctx.fn != nil {
			for _, p := range ctx.fn.TParams {
				env.params[p.Name] = p
			}
		}
	}
	return env
}

func (c *checker) returnStmt(x *ast.ReturnStmt) {
	want := c.bodyWant(c.ctx)
	if x.Value == nil {
		c.coerceTo(unitType, want, x.Sp, nil)
		return
	}
	c.check(x.Value, want)
}

// ---------------------------------------------------------------------------
// Assignment and mutation (core 5.2, 3.4)

// placeRoot returns the root binding of a place expression (`x`, `x.f.g`,
// `x[i].f`) and whether e is a place at all.
func (c *checker) placeRoot(e ast.Expr) (*ast.Ident, bool) {
	switch x := e.(type) {
	case *ast.Ident:
		return x, true
	case *ast.SelectorExpr:
		return c.placeRoot(x.X)
	case *ast.IndexExpr:
		return c.placeRoot(x.X)
	case *ast.ParenExpr:
		return c.placeRoot(x.X)
	}
	return nil, false
}

// requireMutable checks that e is a place rooted at a mutable binding of the
// current function: a `var` binding, `var` parameter, or `var` receiver
// (core 5.2). code is XO0501 for var method calls and XO0502 otherwise.
func (c *checker) requireMutable(e ast.Expr, code diag.Code, what string) {
	id, ok := c.placeRoot(e)
	if !ok {
		c.errorf(code, e.Span(), "%s needs a `var` binding, but the receiver is a temporary value", what)
		return
	}
	v := c.lookupLocal(id.Name)
	if v == nil {
		return // reported as unresolved by the type check
	}
	if v.Mutable && v.ctx == c.ctx {
		return
	}
	if v.Mutable && v.ctx != c.ctx {
		d := c.errorf(code, id.Sp, "%s mutates `%s`, which is captured by a closure; closures capture by value (core 3.3)", what, id.Name)
		note(d, v.Span, "`%s` is declared here, outside the closure", id.Name)
		note(d, token.Span{}, "to share mutable state with a closure, capture a `Mutex` or `Atomic` handle (core 5.4)")
		return
	}
	kind := "an immutable binding"
	switch v.Kind {
	case VParam:
		kind = "an immutable parameter"
	case VRecv:
		kind = "a receiver that is not `var`"
	case VPredeclared:
		kind = "a predeclared name"
	}
	d := c.errorf(code, id.Sp, "%s needs `%s` to be `var`, but it is %s", what, id.Name, kind)
	if d != nil && v.Kind == VLocal && v.decl != nil {
		if ls, ok := v.decl.(*ast.LetStmt); ok && v.Span.File != "" {
			fix(d, fmt.Sprintf("declare `%s` with `var`", id.Name), replaceEdit(token.Span{File: ls.Sp.File, Start: ls.Sp.Start, End: token.Pos{Line: ls.Sp.Start.Line, Col: ls.Sp.Start.Col + 3, Offset: ls.Sp.Start.Offset + 3}}, "var"))
			note(d, token.Span{}, "TypeScript's `let` can be reassigned; Xo's `let` cannot, like `const`, and `var` declares a binding that changes (core 3.1)")
		}
	}
	if d != nil {
		note(d, v.Span, "`%s` is declared here", id.Name)
	}
}

func (c *checker) assign(x *ast.AssignStmt) {
	if c.habitAssignDecl(x) {
		return
	}
	if ix, ok := unparen(x.Target).(*ast.IndexExpr); ok && x.Op == token.Assign {
		c.mapStore = ix
	}
	tt := c.placeType(x.Target)
	c.mapStore = nil
	c.requireMutable(x.Target, diag.AssignImmutable, "assignment")
	if x.Op == token.Assign {
		c.check(x.Value, tt)
		return
	}
	op := map[token.Kind]token.Kind{
		token.PlusAssign: token.Plus, token.MinusAssign: token.Minus, token.StarAssign: token.Star,
		token.SlashAssign: token.Slash, token.PercentAssign: token.Percent, token.AmpAssign: token.Amp,
		token.PipeAssign: token.Pipe, token.CaretAssign: token.Caret, token.ShlAssign: token.Shl, token.ShrAssign: token.Shr,
	}[x.Op]
	yt := c.infer(x.Value, c.operandHint(op, tt))
	rt := c.binaryType(op, tt, yt, x.Sp, x.Target, x.Value)
	if !isBad(rt) && !c.assignable(rt, tt, x.Sp) {
		c.errorf(diag.TypeMismatch, x.Sp, "`%s` produces `%s`, which cannot be stored in `%s`", x.Op, deep(rt), deep(tt))
	}
}

// placeType types an assignment target without counting it as a read.
func (c *checker) placeType(e ast.Expr) Type {
	switch x := e.(type) {
	case *ast.Ident:
		v := c.lookupLocal(x.Name)
		if v == nil {
			return c.infer(x, nil)
		}
		c.identObj[x] = v
		c.rec.name(x, v)
		c.exprType[x] = v.Type
		return v.Type
	case *ast.SelectorExpr:
		bt := c.placeType(x.X)
		t := c.fieldOf(bt, x, true)
		c.exprType[x] = t
		return t
	case *ast.IndexExpr:
		bt := c.placeType(x.X)
		t := c.indexType(bt, x)
		c.exprType[x] = t
		if n, ok := resolve(bt).(*Named); ok && n.Obj.Module == c.prelude && n.Obj.Name == "Map" && c.mapStore != x {
			// `m[k].f = v`, `m[k] += 1`: these read the element first and
			// fault when the key is missing (core 3.6).
			d := c.errorf(diag.MapIndexRead, x.Sp, "changing a Map element in place faults when the key is missing")
			if d != nil {
				note(d, token.Span{}, "use `m.update(k, init, fn(var v) { ... })`, or assign the whole value with `m[k] = v` (core 3.6)")
			}
		}
		return t
	case *ast.ParenExpr:
		return c.placeType(x.X)
	}
	c.infer(e, nil)
	c.errorf(diag.AssignImmutable, e.Span(), "cannot assign to this expression; only bindings, fields, and elements of `var` bindings can be assigned")
	return badType
}

// checkDerives checks that the fields of a type deriving Json, OpenApi,
// Proto, or Ord implement it too (core 2.9). Fields whose type is a type
// parameter are checked where the type is used.
func (c *checker) checkDerives(t *TypeObj, ids []*ast.Ident) {
	for _, id := range ids {
		if id.Name == "SqlRow" {
			c.checkSqlRowDerive(t, id)
			continue
		}
		switch id.Name {
		case "Json", "OpenApi", "Proto", "Ord":
		default:
			continue
		}
		iface := c.named(id.Name)
		var fieldTypes []Type
		var names []string
		for _, f := range t.Fields {
			fieldTypes = append(fieldTypes, f.Type)
			names = append(names, f.Name)
		}
		for _, v := range t.Variants {
			for _, f := range v.Fields {
				fieldTypes = append(fieldTypes, f.Type)
				n := f.Name
				if f.From {
					n = "from"
				}
				names = append(names, v.Name+"."+n)
			}
		}
		for i, ft := range fieldTypes {
			if mentionsParam(ft) {
				continue
			}
			if !c.implements(ft, iface, token.Span{}) {
				c.errorf(diag.NotImplemented, id.Sp, "`derive %s` needs every field to implement `%s`, but `%s` has type `%s`", id.Name, id.Name, names[i], deep(ft))
			}
		}
	}
}

// checkSqlRowDerive checks `derive SqlRow` (std.md 10.4): a struct without
// type parameters whose fields are column types.
func (c *checker) checkSqlRowDerive(t *TypeObj, id *ast.Ident) {
	if t.Kind != KStruct {
		c.errorf(diag.WrongKind, id.Sp, "`derive SqlRow` applies only to structs; `%s` is not one", t.Name)
		return
	}
	if len(t.TParams) > 0 {
		c.errorf(diag.WrongKind, id.Sp, "`derive SqlRow` needs a struct without type parameters; `%s` has some", t.Name)
		return
	}
	for _, f := range t.Fields {
		if !c.isSqlColumn(f.Type) {
			d := c.errorf(diag.NotImplemented, id.Sp, "`derive SqlRow` needs every field to be a column type, but `%s` has type `%s`", f.Name, deep(f.Type))
			note(d, token.Span{}, "column types are Int, Float, Str, Bool, and Time, and an Option of one for a column that may be NULL (std.md 10.4)")
		}
	}
}

// mentionsParam reports whether t contains a type parameter.
func mentionsParam(t Type) bool {
	switch x := resolve(t).(type) {
	case *ParamType:
		return true
	case *Named:
		for _, a := range x.Args {
			if mentionsParam(a) {
				return true
			}
		}
	case *Tuple:
		for _, e := range x.Elems {
			if mentionsParam(e) {
				return true
			}
		}
	}
	return false
}
