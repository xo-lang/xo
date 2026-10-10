package check

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// check checks e against want and returns the resulting type. want may be
// nil (no expectation), a type, a *retWant (return position), or discard.
func (c *checker) check(e ast.Expr, want Type) Type {
	t := c.check0(e, want)
	c.exprType[e] = t
	return t
}

func (c *checker) check0(e ast.Expr, want Type) Type {
	if want == nil {
		return c.infer(e, nil)
	}
	switch x := e.(type) {
	case *ast.ParenExpr:
		return c.check(x.X, want)
	case *ast.Block:
		return c.block(x, want)
	case *ast.IfExpr:
		return c.ifExpr(x, want)
	case *ast.MatchExpr:
		return c.matchExpr(x, want)
	case *ast.ScopeExpr:
		return c.scopeExpr(x, want)
	case *ast.SelectExpr:
		return c.selectExpr(x, want)
	}
	switch w := want.(type) {
	case *retWant:
		return c.checkRet(e, w)
	case discardWant:
		t := c.infer(e, nil)
		return c.coerceTo(t, discard, e.Span(), e)
	}
	t := c.infer(e, want)
	if c.isNever(t) {
		c.neverExpr[e] = true
	}
	return c.coerceTo(t, want, e.Span(), e)
}

// coerceTo checks a computed type t against want, reporting mismatches at
// sp. It returns the type of the value after implicit conversion.
func (c *checker) coerceTo(t, want Type, sp token.Span, e ast.Expr) Type {
	switch w := want.(type) {
	case nil:
		return t
	case discardWant:
		if _, _, ok := c.resultParts(t); ok {
			c.unhandled(e, sp, t)
		}
		return t
	case *retWant:
		return c.coerceRet(t, w, sp, e)
	}
	if isBad(t) {
		return want
	}
	if c.assignable(t, want, sp) {
		return want
	}
	c.mismatch(sp, want, t, e)
	return badType
}

func (c *checker) mismatch(sp token.Span, want, got Type, e ast.Expr) {
	w, g := deep(want), deep(got)
	if containsBad(w) || containsBad(g) {
		return
	}
	d := c.errorf(diag.TypeMismatch, sp, "expected `%s`, found `%s`", w, g)
	if d == nil {
		return
	}
	if c.isNumeric(w) && c.isNumeric(g) {
		note(d, token.Span{}, "there are no implicit numeric conversions; convert explicitly with `%s(...)` (core 2.1)", w)
		if e != nil {
			fix(d, fmt.Sprintf("convert with `%s(...)`", w), insertEdit(sp.File, sp.Start, w.String()+"("), insertEdit(sp.File, sp.End, ")"))
		}
	}
	if _, _, ok := c.resultParts(g); ok {
		if _, _, ok2 := c.resultParts(w); !ok2 && e != nil {
			note(d, token.Span{}, "the value is a `Result`; propagate the error with `?` or handle it with `match`")
			if c.canPropagate() {
				fix(d, "propagate with ?", insertEdit(sp.File, sp.End, "?"))
			}
		}
	}
	if strings.HasSuffix(deep(g).String(), "TestOs") && deep(w).String() == "Os" {
		note(d, token.Span{}, "a `TestOs` is not an `Os`; to run code that takes `Os` in a test, wrap it in a main and run it with `testing.run_main(fn(o: Os) -> () ! Error { ... }, os)`, which gives it an `Os` backed by the test's fakes (std.md 8); or let the function take only the capabilities it uses (`stdio: Stdio`), which a test passes directly (`os.stdio`)")
	}
	if gi := c.optionElem(g); gi != nil && c.optionElem(w) == nil && c.assignable(gi, w, token.Span{}) {
		note(d, token.Span{}, "the value is optional; unwrap it with `??`, `?`, `if let`, or `match` (core 2.5)")
	}
}

// unhandled reports XO0201 for a discarded Result (core 2.6).
func (c *checker) unhandled(e ast.Expr, sp token.Span, t Type) {
	what := "this expression"
	if call, ok := e.(*ast.CallExpr); ok {
		if name := calleeName(call.Fun); name != "" {
			what = "`" + name + "`"
		}
	}
	d := c.errorf(diag.UnhandledResult, sp, "result of %s is not handled", what)
	if d == nil {
		return
	}
	if _, et, ok := c.resultParts(t); ok {
		note(d, token.Span{}, "%s can fail with `%s`; propagate with `?`, match on it, or discard it with `.ignore_err(\"reason\")` (core 2.6)", what, deep(et))
	}
	if c.canPropagate() {
		fix(d, "propagate with ?", insertEdit(sp.File, sp.End, "?"))
	}
	if e != nil {
		fix(d, "discard with a reason", ignoreErrEdits(e)...)
	}
}

func calleeName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		if b := calleeName(x.X); b != "" {
			return b + "." + x.Sel.Name
		}
		return x.Sel.Name
	case *ast.TypeArgsExpr:
		return calleeName(x.X)
	}
	return ""
}

// checkRet checks a value in return position of a function `-> T ! E`
// (core 2.6): `Err(e)` with e: E, `Ok(x)` with x: T, a T, or a Result[T, E].
func (c *checker) checkRet(e ast.Expr, w *retWant) Type {
	if call, ok := e.(*ast.CallExpr); ok {
		if id, ok := call.Fun.(*ast.Ident); ok && (id.Name == "Err" || id.Name == "Ok") && c.lookupLocal(id.Name) == nil && c.mod.Scope[id.Name] == nil {
			if len(call.Args) == 1 && call.Args[0].Name == nil {
				res := c.resultOf(w.T, w.E)
				c.exprType[id] = res
				if id.Name == "Err" {
					c.check(call.Args[0].Value, w.E)
				} else {
					c.check(call.Args[0].Value, w.T)
				}
				return res
			}
		}
	}
	t := c.infer(e, w.T)
	return c.coerceRet(t, w, e.Span(), e)
}

func (c *checker) isResultType(t Type) bool {
	_, _, ok := c.resultParts(t)
	return ok
}

func (c *checker) coerceRet(t Type, w *retWant, sp token.Span, e ast.Expr) Type {
	res := c.resultOf(w.T, w.E)
	if isBad(t) || c.isNever(t) {
		return res
	}
	if tt, te, ok := c.resultParts(t); ok && !c.isResultType(w.T) {
		m := c.mark()
		if c.unify(te, w.E) && c.unify(tt, w.T) {
			return res
		}
		c.undo(m)
		if c.unify(tt, w.T) && !isOpenVar(te) {
			d := c.errorf(diag.TypeMismatch, sp, "expected `%s ! %s`, found `%s`; a returned Result must have the declared error type", deep(w.T), deep(w.E), deep(t))
			if d != nil && c.errConverts(te, w.E) {
				note(d, token.Span{}, "`%s` converts to `%s` at a `?` site", deep(te), deep(w.E))
				fix(d, "propagate with ?", insertEdit(sp.File, sp.End, "?"))
			}
			return res
		}
		c.undo(m)
	}
	if c.isResultType(w.T) && c.isResultType(t) && c.tryUnify(t, w.T) {
		d := c.errorf(diag.ResultNeedsOk, sp, "the success value of `%s ! %s` is itself a Result; write `Ok(...)` explicitly", deep(w.T), deep(w.E))
		if d != nil {
			note(d, token.Span{}, "a bare Result here would be ambiguous between success and returning the value as is (core 2.6)")
			fix(d, "wrap in Ok(...)", insertEdit(sp.File, sp.Start, "Ok("), insertEdit(sp.File, sp.End, ")"))
		}
		return res
	}
	if isUnit(w.T) {
		// A `-> () ! E` body may end in a discarded non Result value.
		if !c.isResultType(t) {
			return res
		}
	}
	if c.assignable(t, w.T, sp) {
		return res
	}
	c.mismatch(sp, w.T, t, e)
	return res
}

// infer computes the type of e, using hint for inference only.
func (c *checker) infer(e ast.Expr, hint Type) Type {
	switch hint.(type) {
	case *retWant, discardWant:
		hint = nil
	}
	t := c.infer0(e, hint)
	if t == nil {
		t = badType
	}
	c.exprType[e] = t
	c.rec.natType[e] = t
	return t
}

func (c *checker) infer0(e ast.Expr, hint Type) Type {
	switch x := e.(type) {
	case *ast.BadExpr:
		return badType
	case *ast.IntLit:
		if hint != nil && c.isInt(hint) {
			return resolve(hint)
		}
		return c.named("Int")
	case *ast.FloatLit:
		if hint != nil && c.isPrelude(hint, "Float32") {
			return resolve(hint)
		}
		return c.named("Float")
	case *ast.DurationLit:
		return c.named("Duration")
	case *ast.RuneLit:
		if hint != nil && c.isInt(hint) {
			// a character as its code point (core 1.6, decision 0079)
			c.runeFits(x, resolve(hint))
			return resolve(hint)
		}
		return c.named("Rune")
	case *ast.BytesLit:
		return c.named("Bytes")
	case *ast.StringLit:
		for _, p := range x.Parts {
			if p.Expr != nil {
				t := c.infer(p.Expr, nil)
				if !c.implementsName(t, "Display", p.Expr.Span()) {
					d := c.errorf(diag.NotImplemented, p.Expr.Span(), "`%s` does not implement `Display`, so it cannot be interpolated", deep(t))
					if d != nil {
						if c.isPrelude(t, "Bytes") {
							note(d, token.Span{}, "convert with `.to_str()` first (std.md 1)")
						} else if el := c.optionElem(t); el != nil {
							note(d, token.Span{}, "unwrap the optional first, for example with `??` (core 2.5)")
						} else {
							note(d, token.Span{}, "implement `fn display(self) -> Str` or add `derive Display` (core 2.9)")
						}
					}
				}
			}
		}
		return c.named("Str")
	case *ast.HoleExpr:
		return c.hole(x, hint)
	case *ast.UnitExpr:
		return unitType
	case *ast.ParenExpr:
		return c.infer(x.X, hint)
	case *ast.TupleExpr:
		var hs []Type
		if ht, ok := resolve(hint).(*Tuple); ok && len(ht.Elems) == len(x.Elems) {
			hs = ht.Elems
		}
		el := make([]Type, len(x.Elems))
		for i, e := range x.Elems {
			if hs != nil {
				el[i] = c.check(e, hs[i])
			} else {
				el[i] = c.infer(e, nil)
			}
		}
		return &Tuple{Elems: el}
	case *ast.ListLit:
		return c.collectionLit("List", x.Elems, hint, x.Sp)
	case *ast.SetLit:
		return c.collectionLit("Set", x.Elems, hint, x.Sp)
	case *ast.MapLit:
		return c.mapLit(x, hint)
	case *ast.StructLit:
		return c.structLit(x, hint)
	case *ast.WithExpr:
		return c.withExpr(x, hint)
	case *ast.UnaryExpr:
		return c.unary(x, hint)
	case *ast.BinaryExpr:
		return c.binary(x, hint)
	case *ast.IsExpr:
		return c.isExpr(x)
	case *ast.RangeExpr:
		return c.rangeExpr(x)
	case *ast.Ident:
		return c.ident(x, hint)
	case *ast.SelectorExpr:
		return c.selector(x, hint)
	case *ast.CallExpr:
		return c.call(x, hint)
	case *ast.TypeArgsExpr:
		return c.typeArgsValue(x, hint)
	case *ast.IndexExpr:
		bt := c.infer(x.X, nil)
		t := c.indexType(bt, x)
		if n, ok := resolve(bt).(*Named); ok && n.Obj.Module == c.prelude && n.Obj.Name == "Map" {
			// A missing key is the Map form of a nil dereference, so reads
			// must say what happens when the key is absent (core 3.6).
			// Writes (`m[k] = v`) go through placeType and stay allowed.
			d := c.errorf(diag.MapIndexRead, x.Sp, "reading a Map by index faults when the key is missing")
			if d != nil {
				note(d, token.Span{}, "use `m.get(k)` (an optional) or `m.get_or_fault(k, \"why the key must exist\")` (core 3.6)")
				// The fix keeps the faulting behavior of the index read
				// and makes the missing reason visible (xo fix, 0.6).
				fix(d, "use get_or_fault",
					replaceEdit(token.Span{File: x.Sp.File, Start: x.X.Span().End, End: x.Index.Span().Start}, ".get_or_fault("),
					replaceEdit(token.Span{File: x.Sp.File, Start: x.Index.Span().End, End: x.Sp.End}, `, "TODO: why the key must exist")`))
			}
		}
		return t
	case *ast.TryExpr:
		return c.try(x)
	case *ast.IfExpr:
		return c.ifExpr(x, nil)
	case *ast.MatchExpr:
		return c.matchExpr(x, nil)
	case *ast.Block:
		return c.block(x, nil)
	case *ast.ScopeExpr:
		return c.scopeExpr(x, nil)
	case *ast.SelectExpr:
		return c.selectExpr(x, nil)
	case *ast.FnLit:
		return c.fnLit(x, hint)
	case *ast.WhileExpr:
		lf := c.loopBody("", true, x.Body, nil)
		if lf.hasBreak {
			return unitType
		}
		return c.named("Never")
	}
	return badType
}

func (c *checker) hole(x *ast.HoleExpr, hint Type) Type {
	t := hint
	if t == nil {
		t = c.newVar()
	}
	desc := "unknown"
	if hint != nil && !isOpenVar(hint) {
		desc = "`" + deep(hint).String() + "`"
	}
	d := c.warnf(diag.Hole, x.Sp, "hole of type %s", desc)
	if d != nil && hint != nil && !isOpenVar(hint) {
		var fits []string
		for s := c.sc; s != nil; s = s.parent {
			for _, v := range s.order {
				if v.Type != nil && identical(v.Type, hint) && !hasOpenVars(v.Type) {
					fits = append(fits, v.Name)
				}
			}
		}
		if len(fits) > 0 {
			note(d, token.Span{}, "in scope values that fit: `%s`", strings.Join(fits, "`, `"))
		}
	}
	if d != nil && c.ctx != nil && c.ctx.fn != nil && c.ctx.fn.Declared != nil {
		if s := c.ctx.fn.Declared.String(); s != "" {
			note(d, token.Span{}, "effects in scope: `%s`", s)
		} else {
			note(d, token.Span{}, "no effects in scope (pure)")
		}
	}
	return t
}

func (c *checker) collectionLit(kind string, elems []ast.Expr, hint Type, sp token.Span) Type {
	var et Type
	if h, ok := resolve(hint).(*Named); ok && h.Obj.Module == c.prelude && h.Obj.Name == kind {
		et = h.Args[0]
	}
	if et == nil {
		if len(elems) == 0 {
			return c.named(kind, c.newVar())
		}
		et = c.infer(elems[0], nil)
		if isNever(et) {
			et = c.newVar()
		}
		for _, e := range elems[1:] {
			c.check(e, et)
		}
		return c.named(kind, et)
	}
	for _, e := range elems {
		c.check(e, et)
	}
	return c.named(kind, et)
}

func (c *checker) mapLit(x *ast.MapLit, hint Type) Type {
	var kt, vt Type
	if h, ok := resolve(hint).(*Named); ok && h.Obj.Module == c.prelude && h.Obj.Name == "Map" {
		kt, vt = h.Args[0], h.Args[1]
	}
	if kt == nil {
		kt, vt = c.newVar(), c.newVar()
	}
	for _, en := range x.Entries {
		c.check(en.Key, kt)
		c.check(en.Value, vt)
	}
	return c.named("Map", kt, vt)
}

func (c *checker) rangeExpr(x *ast.RangeExpr) Type {
	var et Type
	if x.Lo != nil {
		et = c.infer(x.Lo, nil)
	}
	if x.Hi != nil {
		if et == nil {
			et = c.infer(x.Hi, nil)
		} else {
			c.check(x.Hi, et)
		}
	}
	if et == nil {
		et = c.named("Int")
	}
	if !isBad(et) && !c.isInt(et) && !c.isPrelude(et, "Rune") {
		c.errorf(diag.BadOperand, x.Sp, "a range needs integer or Rune bounds, found `%s`", deep(et))
		return badType
	}
	return c.named("Range", et)
}

// indexType types `x[i]` and `x[a..b]` (core 3.6).
func (c *checker) indexType(bt Type, x *ast.IndexExpr) Type {
	r := resolve(bt)
	if isBad(r) {
		c.infer(x.Index, nil)
		return badType
	}
	n, ok := r.(*Named)
	if !ok || n.Obj.Module != c.prelude {
		c.infer(x.Index, nil)
		c.errorf(diag.BadOperand, x.Sp, "`%s` cannot be indexed", deep(bt))
		return badType
	}
	if rng, isRange := x.Index.(*ast.RangeExpr); isRange {
		switch n.Obj.Name {
		case "List", "Bytes", "Str":
			if rng.Lo != nil {
				c.check(rng.Lo, c.named("Int"))
			}
			if rng.Hi != nil {
				c.check(rng.Hi, c.named("Int"))
			}
			c.exprType[rng] = c.named("Range", c.named("Int"))
			return n
		}
		c.errorf(diag.BadOperand, x.Sp, "`%s` cannot be sliced; slicing works on List, Bytes, and Str (core 3.6)", deep(bt))
		return badType
	}
	switch n.Obj.Name {
	case "List":
		c.check(x.Index, c.named("Int"))
		return n.Args[0]
	case "Bytes":
		c.check(x.Index, c.named("Int"))
		return c.named("UInt8")
	case "Map":
		c.check(x.Index, n.Args[0])
		return n.Args[1]
	}
	c.infer(x.Index, nil)
	d := c.errorf(diag.BadOperand, x.Sp, "`%s` cannot be indexed; indexing works on List, Bytes, and Map (core 3.6)", deep(bt))
	if n.Obj.Name == "Str" {
		note(d, token.Span{}, "slice with a range (`s[a..b]`) or use `.runes()` or `.bytes()`")
	}
	return badType
}

// try types `x?` (core 2.5, 2.6).
func (c *checker) try(x *ast.TryExpr) Type {
	t := c.infer(x.X, nil)
	r := resolve(t)
	if isBad(r) {
		return badType
	}
	if tt, te, ok := c.resultParts(r); ok {
		c.propagate(te, x.Sp, diag.ErrorNoConvert, "`?`")
		return tt
	}
	if el := c.optionElem(r); el != nil {
		ctx := c.ctx
		if ctx.errT != nil && !isOpenVar(ctx.errT) || ctx.errVar != nil && ctx.errVar.Ref != nil {
			d := c.errorf(diag.OptionInResultFn, x.Sp, "`?` on an optional value in a function that returns a Result")
			if d != nil {
				note(d, token.Span{}, "convert the optional to a Result first: `.or_err(e)?` (core 2.5)")
				fix(d, "convert with .or_err(...)", insertEdit(x.Sp.File, x.X.Span().End, ".or_err(???)"))
			}
			return el
		}
		if c.optionElem(ctx.result) != nil {
			return el
		}
		if isOpenVar(ctx.result) && ctx.closure {
			c.unify(ctx.result, c.optionOf(c.newVar()))
			return el
		}
		d := c.errorf(diag.TryNeedsFailing, x.Sp, "`?` on an optional value needs a function that returns an optional, but `%s` returns `%s`", ctx.name, deep(ctx.result))
		if d != nil {
			note(d, token.Span{}, "use `??` to supply a default, `if let`, or `match` (core 2.5)")
			if src := c.file.Src; x.Sp.End.Offset < len(src) && src[x.Sp.End.Offset] == '.' {
				// `x?.f` (decision 0087)
				c.safeNavs[x] = true
				c.safeNavDiag[x] = len(c.diags.Items) - 1
				note(d, token.Span{}, "`x?.f` is optional chaining in TypeScript, Kotlin, and Swift; Xo's postfix `?` instead returns early from the function. To keep the value optional, write `x.map(fn(v) { v.f })`, then `?? default` (spec/std.md 2)")
			}
		}
		return el
	}
	if isOpenVar(r) {
		c.errorf(diag.CannotInfer, x.X.Span(), "the type of this value must be known before `?`")
		return badType
	}
	c.errorf(diag.TypeMismatch, x.Sp, "`?` needs a Result or an optional value, found `%s`", deep(t))
	return badType
}

// canPropagate reports whether `?` may be inserted here: the enclosing
// function or closure can fail (a declared or still open error type). A
// "propagate with ?" fix offered where it cannot only trades one error
// for XO0205 (Codex r3, task 07).
func (c *checker) canPropagate() bool {
	return c.ctx != nil && (c.ctx.errT != nil || c.ctx.errVar != nil)
}

// propagate checks that an error of type et can leave the current function
// (by `?` or by a failed child task), reporting code otherwise.
func (c *checker) propagate(et Type, sp token.Span, code diag.Code, how string) {
	ctx := c.ctx
	et = resolve(et)
	if isBad(et) || c.isNever(et) {
		return
	}
	if ctx.errT == nil {
		if ctx.errVar != nil {
			if ctx.errVar.Ref == nil {
				c.bind(ctx.errVar, et)
				return
			}
			if !c.errConverts(et, ctx.errVar.Ref) {
				c.errorf(code, sp, "%s propagates `%s`, which does not convert to `%s`, the error type of this closure", how, deep(et), deep(ctx.errVar.Ref))
			}
			return
		}
		if code == diag.ScopeErrorConvert {
			d := c.errorf(code, sp, "a task here can fail with `%s`, which propagates out of the scope, but `%s` cannot fail", deep(et), ctx.name)
			if d != nil {
				note(d, token.Span{}, "await the handle and handle its error, or declare an error type that `%s` converts to (core 7.1)", deep(et))
			}
			return
		}
		d := c.errorf(diag.TryNeedsFailing, sp, "%s propagates `%s`, but `%s` cannot fail", how, deep(et), ctx.name)
		if d != nil && ctx.fn != nil && ctx.fn.Decl != nil {
			note(d, token.Span{}, "declare the error type: `-> %s ! %s`, or handle the error with `match` (core 2.6)", deep(ctx.result), deep(et))
			fix(d, fmt.Sprintf("declare `! %s`", deep(et)), insertEdit(ctx.fn.Decl.Sp.File, fdHeaderEnd(ctx.fn.Decl), " ! "+deep(et).String()))
		} else if d != nil {
			note(d, token.Span{}, "this closure's type comes from where it is passed and has no error type; handle the error inside it with `match`, `??`, or `.ignore_err(\"reason\")` (core 2.6)")
		}
		return
	}
	if isOpenVar(ctx.errT) {
		c.unify(ctx.errT, et)
		return
	}
	if c.errConverts(et, ctx.errT) {
		return
	}
	d := c.errorf(code, sp, "%s propagates `%s`, which does not convert to `%s`, the error type of `%s`", how, deep(et), deep(ctx.errT), ctx.name)
	if d == nil {
		return
	}
	if code == diag.ScopeErrorConvert {
		note(d, token.Span{}, "a task error that is not consumed by an `await` propagates out of the scope as if by `?` (core 7.1)")
	}
	note(d, token.Span{}, "an error converts when it is the same type, when the target is `Error` and it implements `Display`, or when the target enum has a `from %s` variant (core 2.6)", deep(et))
	if n, ok := resolve(ctx.errT).(*Named); ok && n.Obj.Kind == KEnum && n.Obj.Module == c.mod {
		note(d, n.Obj.Span, "add a variant such as `%s(from %s)` to `%s`", toUpperCamel(lastName(deep(et).String())), deep(et), n.Obj.Name)
	}
}

func lastName(s string) string {
	if i := strings.IndexByte(s, '['); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndexByte(s, '.'); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// ---------------------------------------------------------------------------
// Control flow

// join computes the type of branches with no expected type.
func (c *checker) join(ts []Type, spans []token.Span) Type {
	var base Type
	for i, t := range ts {
		if isBad(t) {
			return badType
		}
		if c.isNever(t) {
			continue
		}
		if base == nil {
			base = t
			continue
		}
		// No implicit conversion here: the target type is not known
		// (core 2.10), so branches must agree exactly.
		if c.tryUnify(t, base) {
			continue
		}
		d := c.errorf(diag.TypeMismatch, spans[i], "branches have different types: `%s` and `%s`", deep(base), deep(t))
		if d != nil && (c.optionElem(base) != nil || c.optionElem(t) != nil) {
			note(d, token.Span{}, "wrap the plain value in `Some(...)`, or give the expression an expected type")
		}
		return badType
	}
	if base == nil {
		return c.named("Never")
	}
	return base
}

func (c *checker) ifExpr(x *ast.IfExpr, want Type) Type {
	c.pushScope()
	if x.Cond != nil {
		c.check(x.Cond, c.named("Bool"))
	} else {
		vt := c.infer(x.Value, nil)
		pat := x.Pattern
		if bp, ok := pat.(*ast.BindPat); ok && c.optionElem(vt) != nil {
			// `if let e = opt` is shorthand for `if let Some(e) = opt` (core 2.5)
			c.checkName(bp.Name, "variable")
			c.pattern(&ast.VariantPat{Base: bp.Base, Path: []*ast.Ident{{Base: bp.Base, Name: "Some"}}, Parens: true, Fields: []*ast.PatField{{Base: bp.Base, Pattern: bp}}}, vt, true)
		} else {
			c.pattern(pat, vt, true)
		}
	}
	if x.Else == nil {
		c.block(x.Then, discard)
		c.popScope()
		t := Type(unitType)
		switch want.(type) {
		case nil, discardWant:
			return t
		}
		n := len(c.diags.Items)
		r := c.coerceTo(t, want, x.Sp, x)
		c.missingElseNote(x, want, n)
		return r
	}
	if want != nil {
		open, n := isOpenVar(valueWant(want)), len(c.diags.Items)
		c.block(x.Then, want)
		c.popScope()
		c.check(x.Else, want)
		c.missingElseFix(x, open, n)
		if w, ok := want.(*retWant); ok {
			return c.resultOf(w.T, w.E)
		}
		if _, ok := want.(discardWant); ok {
			return unitType
		}
		return want
	}
	t1 := c.block(x.Then, nil)
	c.popScope()
	t2 := c.infer(x.Else, nil)
	return c.join([]Type{t1, t2}, []token.Span{x.Then.Sp, x.Else.Span()})
}

func (c *checker) matchExpr(x *ast.MatchExpr, want Type) Type {
	st := c.infer(x.Subject, nil)
	if isOpenVar(st) {
		c.errorf(diag.CannotInfer, x.Subject.Span(), "the type of the matched value must be known here")
		st = badType
	}
	var rows []*ipat
	var ts []Type
	var spans []token.Span
	for _, arm := range x.Arms {
		c.pushScope()
		ip := c.pattern(arm.Pattern, st, true)
		if arm.Guard != nil {
			c.check(arm.Guard, c.named("Bool"))
		} else {
			rows = append(rows, ip)
		}
		var t Type
		if arm.Body != nil {
			t = c.armBody(arm.Body, want)
			ts = append(ts, t)
			spans = append(spans, arm.Body.Span())
		}
		c.popScope()
	}
	c.checkExhaustive(x, rows, st)
	c.warnWildcardOwnEnum(x, st)
	switch w := want.(type) {
	case nil:
		return c.join(ts, spans)
	case *retWant:
		return c.resultOf(w.T, w.E)
	case discardWant:
		return unitType
	}
	return want
}

// armBody checks a match or select arm body: an expression, a block, or a
// single statement (type `()`, or Never for return, break, continue).
func (c *checker) armBody(s ast.Stmt, want Type) Type {
	if es, ok := s.(*ast.ExprStmt); ok {
		if want == nil {
			return c.infer(es.X, nil)
		}
		return c.check(es.X, want)
	}
	c.stmt(s)
	var t Type = unitType
	if c.stmtNever(s) {
		t = c.named("Never")
	}
	if want == nil {
		return t
	}
	return c.coerceTo(t, want, s.Span(), nil)
}

func (c *checker) scopeExpr(x *ast.ScopeExpr, want Type) Type {
	c.pushScope()
	frame := &scopeFrame{ctx: c.ctx, span: x.Sp}
	h := &VarObj{Name: x.Name.Name, Type: c.named("Scope"), Kind: VLocal, Span: x.Name.Sp, Used: true, scopeHandle: frame}
	c.checkName(x.Name, "scope name")
	c.declareVar(h)
	c.identObj[x.Name] = h
	t := c.block(x.Body, want)
	c.popScope()
	// child failure propagates out of the scope (core 7.1)
	for _, sp := range frame.spawns {
		if sp.bound != nil && sp.bound.awaited {
			continue
		}
		saved := c.ctx
		c.ctx = frame.ctx
		c.propagate(sp.errT, sp.call.Sp, diag.ScopeErrorConvert, "this task")
		c.ctx = saved
	}
	if c.containsScope(t) {
		c.errorf(diag.ScopeEscape, x.Sp, "the value of `scope %s` contains a scope handle, which would outlive its scope (core 7.1)", x.Name.Name)
	}
	c.closureEscapes(x, h)
	c.groupEscapes(x, h)
	return t
}

// closureEscapes reports XO0802 when a closure that captures the scope
// handle h leaves the scope: as (part of) the scope's value, by assignment
// to a binding declared outside the scope, or sent on a channel. Such a
// closure could start a task after the scope has ended.
func (c *checker) closureEscapes(x *ast.ScopeExpr, h *VarObj) {
	captures := map[ast.Node]bool{}
	inspect(x.Body, func(n ast.Node) bool {
		if fl, ok := n.(*ast.FnLit); ok {
			inspect(fl.Body, func(m ast.Node) bool {
				if id, ok := m.(*ast.Ident); ok && c.identObj[id] == h {
					captures[fl] = true
				}
				return true
			})
		}
		return true
	})
	if len(captures) == 0 {
		return
	}
	// locals bound to a capturing closure
	bound := map[*VarObj]bool{}
	inspect(x.Body, func(n ast.Node) bool {
		if ls, ok := n.(*ast.LetStmt); ok && captures[ls.Value] {
			if bp, ok := ls.Pattern.(*ast.BindPat); ok {
				if v := c.identObj[bp.Name]; v != nil {
					bound[v] = true
				}
			}
		}
		return true
	})
	isCapturing := func(e ast.Expr) bool {
		if captures[e] {
			return true
		}
		if id, ok := e.(*ast.Ident); ok && bound[c.identObj[id]] {
			return true
		}
		return false
	}
	report := func(e ast.Expr, how string) {
		d := c.errorf(diag.ScopeEscape, e.Span(), "a closure that captures scope handle `%s` %s, so it could start a task after the scope ends (core 7.1)", h.Name, how)
		note(d, h.Span, "`%s` is bound here", h.Name)
	}
	// the scope's value: follow value positions only (a call of the
	// closure inside the scope is fine)
	var value func(e ast.Expr)
	tail := func(b *ast.Block) {
		if b == nil || len(b.Stmts) == 0 {
			return
		}
		if es, ok := b.Stmts[len(b.Stmts)-1].(*ast.ExprStmt); ok {
			value(es.X)
		}
	}
	value = func(e ast.Expr) {
		if e == nil {
			return
		}
		if isCapturing(e) {
			report(e, "is the value of the scope")
			return
		}
		switch y := e.(type) {
		case *ast.ParenExpr:
			value(y.X)
		case *ast.TupleExpr:
			for _, el := range y.Elems {
				value(el)
			}
		case *ast.ListLit:
			for _, el := range y.Elems {
				value(el)
			}
		case *ast.StructLit:
			for _, f := range y.Fields {
				value(f.Value)
			}
		case *ast.MapLit:
			for _, en := range y.Entries {
				value(en.Value)
			}
		case *ast.Block:
			tail(y)
		case *ast.IfExpr:
			tail(y.Then)
			value(y.Else)
		case *ast.MatchExpr:
			for _, arm := range y.Arms {
				if es, ok := arm.Body.(*ast.ExprStmt); ok {
					value(es.X)
				}
			}
		}
	}
	tail(x.Body)
	inspect(x.Body, func(n ast.Node) bool {
		switch y := n.(type) {
		case *ast.AssignStmt:
			if isCapturing(y.Value) {
				if id, ok := c.placeRoot(y.Target); ok {
					if v := c.identObj[id]; v != nil && v.Span.Start.Offset < x.Sp.Start.Offset {
						report(y.Value, "is stored in `"+id.Name+"`, which outlives the scope")
					}
				}
			}
		case *ast.CallExpr:
			if sel, ok := y.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "send" {
				for _, a := range y.Args {
					if isCapturing(a.Value) {
						report(a.Value, "is sent on a channel")
					}
				}
			}
		}
		return true
	})
}

func (c *checker) containsScope(t Type) bool {
	switch x := resolve(t).(type) {
	case *Named:
		if x.Obj.Module == c.prelude && x.Obj.Name == "Scope" {
			return true
		}
		for _, a := range x.Args {
			if c.containsScope(a) {
				return true
			}
		}
		if x.Obj.Kind == KStruct && x.Obj.Module != c.prelude {
			for _, f := range x.Obj.Fields {
				if c.containsScopeShallow(f.Type) {
					return true
				}
			}
		}
	case *Tuple:
		for _, e := range x.Elems {
			if c.containsScope(e) {
				return true
			}
		}
	case *Func:
		return false
	}
	return false
}

func (c *checker) containsScopeShallow(t Type) bool {
	n, ok := resolve(t).(*Named)
	return ok && n.Obj.Module == c.prelude && n.Obj.Name == "Scope"
}

// noteSpawnBinding records that a spawned task handle is bound to v, so an
// `await` on v consumes its error (core 7.1).
func (c *checker) noteSpawnBinding(e ast.Expr, v *VarObj) {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return
	}
	bind := func(f *scopeFrame) {
		if f == nil {
			return
		}
		for _, sp := range f.spawns {
			if sp.call == call {
				sp.bound = v
			}
		}
	}
	for s := c.sc; s != nil; s = s.parent {
		for _, o := range s.order {
			bind(o.scopeHandle)
		}
	}
	for ctx := c.ctx; ctx != nil; ctx = ctx.parent {
		bind(ctx.groups)
		bind(ctx.ext)
	}
}

func (c *checker) selectExpr(x *ast.SelectExpr, want Type) Type {
	var ts []Type
	var spans []token.Span
	for _, arm := range x.Arms {
		c.pushScope()
		switch {
		case arm.After != nil:
			c.check(arm.After, c.named("Duration"))
			c.perform(&EffSet{Items: []EffItem{{Label: "clock", Span: arm.After.Span(), Via: "select `after`"}}})
		case arm.Bind != nil:
			t := c.infer(arm.Op, nil)
			c.checkName(arm.Bind, "variable")
			v := &VarObj{Name: arm.Bind.Name, Type: t, Kind: VLocal, Span: arm.Bind.Sp}
			c.declareVar(v)
			c.identObj[arm.Bind] = v
		default:
			c.check(arm.Op, discard)
		}
		if arm.Body != nil {
			ts = append(ts, c.armBody(arm.Body, want))
			spans = append(spans, arm.Body.Span())
		}
		c.popScope()
	}
	switch w := want.(type) {
	case nil:
		return c.join(ts, spans)
	case *retWant:
		return c.resultOf(w.T, w.E)
	case discardWant:
		return unitType
	}
	return want
}

// ---------------------------------------------------------------------------
// Operators

// isLiteral reports whether e is a numeric literal (possibly negated) or a
// rune literal, whose type adapts to its context.
func isLiteral(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.IntLit, *ast.FloatLit, *ast.RuneLit:
		return true
	case *ast.UnaryExpr:
		return x.Op == token.Minus && isNumLiteral(x.X)
	case *ast.ParenExpr:
		return isLiteral(x.X)
	}
	return false
}

// isNumLiteral is isLiteral without rune literals.
func isNumLiteral(e ast.Expr) bool {
	if p, ok := e.(*ast.ParenExpr); ok {
		return isNumLiteral(p.X)
	}
	if _, ok := e.(*ast.RuneLit); ok {
		return false
	}
	return isLiteral(e)
}

// runeFits reports a rune literal x used as a value of the integer type t
// (decision 0079) whose code point t cannot hold. An 8 bit type takes only
// ASCII: any other character is several bytes in UTF-8, so comparing a
// byte with it is a mistake even where the code point would fit.
func (c *checker) runeFits(x *ast.RuneLit, t Type) {
	n, ok := t.(*Named)
	if !ok {
		return
	}
	limit := map[string]rune{"Int8": 0x7f, "UInt8": 0x7f, "Int16": 0x7fff, "UInt16": 0xffff}
	max, sized := limit[n.Obj.Name]
	if !sized || x.Value <= max {
		return
	}
	d := c.errorf(diag.TypeMismatch, x.Sp, "the rune literal %s (U+%04X) does not fit `%s`", x.Raw, x.Value, n.Obj.Name)
	if d != nil && max == 0x7f {
		note(d, token.Span{}, "a byte type takes only ASCII characters: any other character is two or more bytes in UTF-8 (compare with its bytes, `\\xHH`, or decode the runes)")
	}
}

func (c *checker) unary(x *ast.UnaryExpr, hint Type) Type {
	switch x.Op {
	case token.Bang:
		c.check(x.X, c.named("Bool"))
		return c.named("Bool")
	case token.Minus:
		t := c.infer(x.X, hint)
		if isBad(t) {
			return badType
		}
		if c.isNumeric(t) || c.isPrelude(t, "Duration") {
			return t
		}
		c.errorf(diag.BadOperand, x.Sp, "unary `-` is not defined for `%s`", deep(t))
		return badType
	case token.Tilde:
		t := c.infer(x.X, hint)
		if isBad(t) || c.isInt(t) {
			return t
		}
		c.errorf(diag.BadOperand, x.Sp, "`~` is not defined for `%s`", deep(t))
		return badType
	}
	return badType
}

// operandHint is the expected type of the right operand given the left.
func (c *checker) operandHint(op token.Kind, xt Type) Type {
	switch op {
	case token.AndAnd, token.OrOr:
		return c.named("Bool")
	case token.Coalesce:
		return c.optionElem(xt)
	case token.Star, token.Slash:
		if c.isPrelude(xt, "Duration") {
			if op == token.Star {
				return c.named("Int")
			}
			return nil
		}
	case token.Minus, token.Plus:
		if c.isPrelude(xt, "Time") {
			if op == token.Plus {
				return c.named("Duration")
			}
			return nil
		}
	case token.Shl, token.Shr:
		return nil
	}
	if isOpenVar(xt) {
		return nil
	}
	return xt
}

func (c *checker) binary(x *ast.BinaryExpr, hint Type) Type {
	if call, ok := x.X.(*ast.CallExpr); ok && c.isExpectCall(call) {
		return c.expectCompare(x, call)
	}
	var xt, yt Type
	switch x.Op {
	case token.AndAnd, token.OrOr:
		c.check(x.X, c.named("Bool"))
		c.check(x.Y, c.named("Bool"))
		return c.named("Bool")
	case token.Coalesce:
		xt = c.infer(x.X, c.optionOfHint(hint))
		el := c.optionElem(xt)
		if el == nil {
			// the left side already failed or is reported below; a bad hint
			// keeps `[]` or `{}` on the right from cascading (core 10.2)
			c.infer(x.Y, badType)
			if !isBad(xt) && !c.safeNav(x.X) {
				d := c.errorf(diag.BadOperand, x.Sp, "`??` needs an optional value on the left, found `%s`", deep(xt))
				if _, _, ok := c.resultParts(xt); ok {
					note(d, token.Span{}, "for a Result, use `.ok() ?? default` or `match`")
				}
			}
			return badType
		}
		yt = c.infer(x.Y, el)
		if c.optionElem(yt) != nil && c.tryUnify(yt, xt) {
			return xt
		}
		if c.assignable(yt, el, x.Y.Span()) {
			return el
		}
		c.mismatch(x.Y.Span(), el, yt, x.Y)
		return badType
	}
	if isLiteral(x.X) && !isLiteral(x.Y) {
		yt = c.infer(x.Y, nil)
		xt = c.infer(x.X, c.operandHint(x.Op, yt))
	} else {
		var h Type
		if isLiteral(x.X) && isLiteral(x.Y) {
			h = hint
		}
		xt = c.infer(x.X, h)
		yt = c.infer(x.Y, c.operandHint(x.Op, xt))
	}
	return c.binaryType(x.Op, xt, yt, x.Sp, x.X, x.Y)
}

// isExpr types `x is Pattern` (core 3.5): a Bool test that binds nothing.
func (c *checker) isExpr(x *ast.IsExpr) Type {
	t := c.infer(x.X, nil)
	if isOpenVar(t) {
		c.errorf(diag.CannotInfer, x.X.Span(), "the type of the tested value must be known here")
		t = badType
	}
	c.pushScope()
	c.pattern(x.Pattern, t, true)
	for _, v := range c.sc.order {
		v.Used = true // the parser reports bindings in an `is` pattern
	}
	c.popScope()
	return c.named("Bool")
}

func (c *checker) optionOfHint(h Type) Type {
	if h == nil {
		return nil
	}
	return c.optionOf(h)
}

// binaryType computes the result type of `x op y` (core 2.1, 15).
func (c *checker) binaryType(op token.Kind, xt, yt Type, sp token.Span, xe, ye ast.Expr) Type {
	if isBad(xt) || isBad(yt) {
		return badType
	}
	boolT := c.named("Bool")
	fail := func() Type {
		dx, dy := deep(xt), deep(yt)
		d := c.errorf(diag.BadOperand, sp, "operator `%s` is not defined for `%s` and `%s`", op, dx, dy)
		if d != nil && c.isNumeric(dx) && c.isNumeric(dy) {
			note(d, token.Span{}, "there are no implicit numeric conversions; convert one side explicitly, for example `%s(x)` (core 2.1)", dx)
		}
		if d != nil && (c.isInt(dx) && c.isPrelude(dy, "Rune") || c.isPrelude(dx, "Rune") && c.isInt(dy)) {
			it := dx
			if !c.isInt(it) {
				it = dy
			}
			note(d, token.Span{}, "a Rune converts to its code point with `%s(r)`; a rune literal such as 'a' is already one where an integer is expected (core 2.1)", it)
		}
		if d != nil && (c.isPrelude(dx, "Duration") || c.isPrelude(dy, "Duration") || c.isPrelude(dx, "Time")) {
			note(d, token.Span{}, "Duration and Time support d1 + d2, d1 - d2, d * n, n * d, d / n, d1 / d2, t + d, t - d, t1 - t2, and comparisons (core 2.1)")
		}
		return badType
	}
	switch op {
	case token.Eq, token.NotEq:
		if c.tryUnify(xt, yt) || c.assignable(yt, xt, sp) || c.assignable(xt, yt, sp) {
			if !c.isEqType(xt, "Eq") {
				d := c.errorf(diag.NotImplemented, sp, "`%s` does not implement `Eq`, so it cannot be compared with `%s`", deep(xt), op)
				noteIncomparable(d, c.incomparable(xt))
			}
			return boolT
		}
		return fail()
	case token.Lt, token.LtEq, token.Gt, token.GtEq:
		if !c.tryUnify(xt, yt) {
			return fail()
		}
		if !c.isOrd(xt) && !isOpenVar(xt) {
			d := c.errorf(diag.NotImplemented, sp, "`%s` does not implement `Ord`, so it cannot be compared with `%s`", deep(xt), op)
			note(d, token.Span{}, "add `derive Ord` to the type declaration (std.md 1)")
			return boolT
		}
		return boolT
	}
	dur, tim := c.isPrelude(xt, "Duration"), c.isPrelude(xt, "Time")
	ydur, ytim := c.isPrelude(yt, "Duration"), c.isPrelude(yt, "Time")
	switch op {
	case token.Plus:
		if dur && ydur || tim && ydur {
			return xt
		}
		if c.isNumeric(xt) && c.tryUnify(xt, yt) {
			return xt
		}
		if (c.isPrelude(xt, "Str") || c.isPrelude(xt, "Bytes") || c.isPrelude(xt, "List")) && c.tryUnify(xt, yt) {
			return xt
		}
		return fail()
	case token.Minus:
		if dur && ydur || tim && ydur {
			return xt
		}
		if tim && ytim {
			return c.named("Duration")
		}
		if c.isNumeric(xt) && c.tryUnify(xt, yt) {
			return xt
		}
		return fail()
	case token.Star:
		if dur && c.isPrelude(yt, "Int") {
			return xt
		}
		if ydur && c.isPrelude(xt, "Int") {
			return yt // n * d (core 2.1)
		}
		if c.isNumeric(xt) && c.tryUnify(xt, yt) {
			return xt
		}
		return fail()
	case token.Slash:
		if dur && c.isPrelude(yt, "Int") {
			return xt
		}
		if dur && ydur {
			return c.named("Int")
		}
		if c.isNumeric(xt) && c.tryUnify(xt, yt) {
			return xt
		}
		return fail()
	case token.Percent:
		if c.isNumeric(xt) && c.tryUnify(xt, yt) {
			return xt
		}
		return fail()
	case token.Amp, token.Pipe, token.Caret:
		if c.isInt(xt) && c.tryUnify(xt, yt) {
			return xt
		}
		if c.isPrelude(xt, "Bool") && c.tryUnify(xt, yt) && op != token.Caret {
			return fail()
		}
		return fail()
	case token.Shl, token.Shr:
		if c.isInt(xt) && c.isInt(yt) {
			return xt
		}
		return fail()
	}
	return fail()
}

// ---------------------------------------------------------------------------
// Struct literals and updates

// typeOfExpr interprets e as a type name: `User`, `http.Response`,
// `Job[Str]`, `Mutex[Map[Str, Int]]`. It returns nil if e does not name a
// type (without reporting).
func (c *checker) typeOfExpr(e ast.Expr) (*TypeObj, []Type, bool) {
	switch x := e.(type) {
	case *ast.Ident:
		if c.lookupLocal(x.Name) != nil {
			return nil, nil, false
		}
		if t, ok := c.lookupModuleObj(c.mod, x.Name).(*TypeObj); ok {
			if p := c.localEnv().params[x.Name]; p != nil {
				return nil, nil, false
			}
			c.rec.name(x, t)
			return t, nil, true
		}
	case *ast.SelectorExpr:
		id, ok := x.X.(*ast.Ident)
		if !ok || c.lookupLocal(id.Name) != nil {
			return nil, nil, false
		}
		imp := c.file.Imports[id.Name]
		if imp == nil && id.Name == "testing" && c.inTestContext() {
			imp = &Import{Name: "testing", Module: c.modules["std/testing"]}
		}
		if imp == nil || imp.Module == nil {
			return nil, nil, false
		}
		if t, ok := imp.Module.Scope[x.Sel.Name].(*TypeObj); ok {
			imp.Used = true
			c.rec.name(id, imp.Module)
			c.rec.name(x.Sel, t)
			if !t.Pub && imp.Module != c.mod {
				c.errorf(diag.NotVisible, x.Sel.Sp, "type `%s` is not `pub` in module `%s`", t.Name, imp.Module.Path)
			}
			return t, nil, true
		}
	case *ast.TypeArgsExpr:
		t, _, ok := c.typeOfExpr(x.X)
		if !ok {
			return nil, nil, false
		}
		if !t.resolving {
			c.resolveTypeObj(t)
		}
		args := c.resolveArgsFor(t, x.Types, c.localEnv())
		if args == nil {
			args = []Type{}
		}
		return t, args, true
	}
	return nil, nil, false
}

// instantiate returns obj applied to explicit args or fresh variables.
func (c *checker) instantiate(obj *TypeObj, args []Type, sp token.Span) (*Named, bool) {
	if args != nil {
		if len(args) != len(obj.TParams) {
			c.errorf(diag.BadArguments, sp, "`%s` takes %d type argument(s), found %d", obj.QualName(), len(obj.TParams), len(args))
			return nil, false
		}
		return &Named{Obj: obj, Args: args}, true
	}
	n := &Named{Obj: obj}
	for _, p := range obj.TParams {
		n.Args = append(n.Args, c.freshArg(p))
	}
	return n, true
}

// aliasNamed resolves an alias object to its target named type.
func (c *checker) aliasNamed(obj *TypeObj, args []Type, sp token.Span) (*TypeObj, []Type) {
	for i := 0; obj != nil && obj.Kind == KAlias && i < 10; i++ {
		t := c.aliasTarget(obj, sp)
		if args != nil {
			t = subst(t, typeArgsSubst(obj.TParams, args))
		} else if len(obj.TParams) > 0 {
			vs := make([]Type, len(obj.TParams))
			for j := range vs {
				vs[j] = c.freshArg(obj.TParams[j])
			}
			t = subst(t, typeArgsSubst(obj.TParams, vs))
		}
		n, ok := resolve(t).(*Named)
		if !ok {
			return nil, nil
		}
		obj, args = n.Obj, n.Args
	}
	return obj, args
}

func (c *checker) structLit(x *ast.StructLit, hint Type) Type {
	obj, args, ok := c.typeOfExpr(x.Type)
	if !ok {
		c.infer(x.Type, nil)
		for _, f := range x.Fields {
			c.infer(f.Value, nil)
		}
		return badType
	}
	if obj.Kind == KAlias {
		obj, args = c.aliasNamed(obj, args, x.Type.Span())
		if obj == nil {
			return badType
		}
	}
	if obj.Kind != KStruct {
		c.errorf(diag.WrongKind, x.Type.Span(), "`%s` is not a struct, so it has no struct literal", obj.QualName())
		for _, f := range x.Fields {
			c.infer(f.Value, nil)
		}
		return badType
	}
	if obj.Opaque {
		c.errorf(diag.NotVisible, x.Type.Span(), "`%s` is an opaque handle and cannot be built with a struct literal; use its constructor function", obj.QualName())
		return badType
	}
	nt, ok := c.instantiate(obj, args, x.Type.Span())
	if !ok {
		return badType
	}
	if hint != nil {
		c.tryUnify(nt, hint)
	}
	s := typeArgsSubst(obj.TParams, nt.Args)
	seen := map[string]bool{}
	for _, f := range x.Fields {
		fo := obj.field(f.Name.Name)
		if fo == nil {
			var cands []string
			for _, ff := range obj.Fields {
				cands = append(cands, ff.Name)
			}
			c.unresolved(f.Name.Sp, "field of `"+obj.Name+"`", f.Name.Name, cands)
			c.infer(f.Value, nil)
			continue
		}
		if seen[fo.Name] {
			c.errorf(diag.BadArguments, f.Name.Sp, "field `%s` is given twice", fo.Name)
		}
		seen[fo.Name] = true
		if !fo.Pub && obj.Module != c.mod {
			c.errorf(diag.NotVisible, f.Name.Sp, "field `%s` of `%s` is not `pub`", fo.Name, obj.Name)
		}
		c.check(f.Value, subst(fo.Type, s))
	}
	var missing []string
	for _, fo := range obj.Fields {
		if !seen[fo.Name] && fo.Default == nil {
			missing = append(missing, fo.Name)
		}
	}
	if len(missing) > 0 {
		d := c.errorf(diag.BadArguments, x.Type.Span(), "missing field(s) `%s` in `%s` literal", strings.Join(missing, "`, `"), obj.Name)
		if d != nil {
			note(d, token.Span{}, "construction names every field without a default (core 2.2)")
			var parts []string
			for _, m := range missing {
				parts = append(parts, m+": ???")
			}
			end := x.Sp.End
			end.Col--
			end.Offset--
			sep := ", "
			if len(x.Fields) == 0 {
				sep = ""
			}
			fix(d, "add the missing fields as holes", insertEdit(x.Sp.File, end, sep+strings.Join(parts, ", ")))
		}
	}
	return nt
}

func (c *checker) withExpr(x *ast.WithExpr, hint Type) Type {
	t := c.infer(x.X, hint)
	n, ok := resolve(t).(*Named)
	if !ok || n.Obj.Kind != KStruct || n.Obj.Opaque {
		for _, f := range x.Fields {
			c.infer(f.Value, nil)
		}
		if !isBad(t) {
			c.errorf(diag.BadOperand, x.Sp, "`with` needs a struct value, found `%s`", deep(t))
		}
		return badType
	}
	s := typeArgsSubst(n.Obj.TParams, n.Args)
	for _, f := range x.Fields {
		fo := n.Obj.field(f.Name.Name)
		if fo == nil {
			var cands []string
			for _, ff := range n.Obj.Fields {
				cands = append(cands, ff.Name)
			}
			c.unresolved(f.Name.Sp, "field of `"+n.Obj.Name+"`", f.Name.Name, cands)
			c.infer(f.Value, nil)
			continue
		}
		if !fo.Pub && n.Obj.Module != c.mod {
			c.errorf(diag.NotVisible, f.Name.Sp, "field `%s` of `%s` is not `pub`", fo.Name, n.Obj.Name)
		}
		c.check(f.Value, subst(fo.Type, s))
	}
	return t
}

// ---------------------------------------------------------------------------
// Closures (core 3.3)

func (c *checker) fnLit(x *ast.FnLit, hint Type) Type {
	ft, _ := resolve(hint).(*Func)
	if ft != nil && len(ft.Params) != len(x.Params) {
		c.errorf(diag.TypeMismatch, x.Sp, "expected a function with %d parameter(s), found a closure with %d", len(ft.Params), len(x.Params))
		ft = nil
	}
	env := c.localEnv()
	ctx := &fnCtx{parent: c.ctx, name: "closure", eff: &EffSet{}, closure: true, isTest: c.ctx != nil && c.ctx.isTest}
	var params []FParam
	var vars []*VarObj
	for i, p := range x.Params {
		var pt Type
		switch {
		case p.Type != nil:
			pt = c.resolveType(p.Type, env)
			if ft != nil {
				c.unify(ft.Params[i].T, pt)
			}
		case ft != nil:
			pt = ft.Params[i].T
		default:
			if hint == nil {
				c.errorf(diag.CannotInfer, p.Name.Sp, "cannot infer the type of parameter `%s`; annotate it: `%s: T`", p.Name.Name, p.Name.Name)
			}
			pt = badType
		}
		isVar := p.Var
		if p.Var && ft != nil && !ft.Params[i].Var {
			// the caller passes the value read only (`Mutex.with_read`,
			// decision 0063); a `var` parameter would change a copy only
			// (the binding stays mutable so its uses do not cascade)
			d := c.errorf(diag.TypeMismatch, p.Name.Sp, "parameter `%s` cannot be `var`: the expected function type passes it read only", p.Name.Name)
			note(d, token.Span{}, "changes to it would never be written back; to change a `Mutex` value use `with`, not `with_read` (core 5.4)")
		}
		params = append(params, FParam{Var: ft != nil && ft.Params[i].Var || ft == nil && p.Var, T: pt})
		c.checkName(p.Name, "parameter")
		v := &VarObj{Name: p.Name.Name, Type: pt, Mutable: isVar, Kind: VParam, Span: p.Name.Sp, Used: true}
		vars = append(vars, v)
		c.identObj[p.Name] = v
	}
	switch {
	case x.Result != nil:
		ctx.result = c.resolveType(x.Result, env)
		if ft != nil {
			c.unify(ft.Result, ctx.result)
		}
	case ft != nil:
		ctx.result = ft.Result
	default:
		ctx.result = c.newVar()
	}
	switch {
	case x.Err != nil:
		ctx.errT = c.resolveErrSpec(x.Err, env)
		if ft != nil && ft.Err != nil && ctx.errT != nil && isOpenVar(ft.Err) {
			c.unify(ft.Err, ctx.errT)
		}
	case x.Result != nil:
		// an explicit signature without `!` cannot fail
		ctx.errT = nil
		if ft != nil && ft.Err != nil && isOpenVar(ft.Err) {
			c.unify(ft.Err, c.named("Never"))
		}
	case ft != nil && ft.Err != nil:
		ctx.errT = ft.Err
	case ft != nil:
		ctx.errT = nil
	default:
		ctx.errVar = c.newVar()
	}
	var declared *EffSet
	if x.Uses != nil {
		declared = c.resolveEffects(x.Uses, env)
	}
	c.withCtx(ctx, func() {
		c.pushScope()
		for _, v := range vars {
			c.declareVar(v)
		}
		want := c.bodyWant(ctx)
		if ctx.errVar != nil {
			want = &retWant{T: ctx.result, E: ctx.errVar}
		}
		if isOpenVar(ctx.result) && ctx.errT == nil && ctx.errVar == nil {
			want = ctx.result
		}
		c.block(x.Body, want)
		if isOpenVar(ctx.result) && c.blockNever(x.Body) {
			c.neverResults = append(c.neverResults, ctx.result)
		}
		c.finishGroups(ctx)
		c.popScope()
	})
	c.bodies = append(c.bodies, &bodyInfo{body: x.Body, params: vars})
	var errT Type
	if ctx.errT != nil {
		errT = ctx.errT
		if isOpenVar(errT) {
			// an inferred error type of a generic callback: none was
			// propagated, so the closure cannot fail
			c.unify(errT, c.named("Never"))
			errT = nil
		}
	} else if ctx.errVar != nil {
		if ctx.errVar.Ref != nil {
			errT = ctx.errVar.Ref
		}
	}
	if errT != nil && c.isNever(errT) {
		errT = nil
	}
	eff := ctx.eff
	if declared != nil {
		c.obligations = append(c.obligations, effObligation{sub: ctx.eff, sup: declared, span: x.Sp, usesClause: c.usesClauseSpan(x), what: func(s string) string {
			if s == "" {
				return "the closure declares no effects"
			}
			return "the closure declares `uses " + s + "`"
		}})
		eff = declared
	}
	c.rec.closureVar[x] = vars
	return &Func{Params: params, Result: ctx.result, Err: errT, Eff: eff}
}

// freshArg is a fresh inference argument for type parameter p.
func (c *checker) freshArg(p *TParam) Type {
	if p.IsEffect {
		return &EffArg{Set: &EffSet{Items: []EffItem{{EVar: &EffVar{}}}}}
	}
	return c.newVar()
}
