package check

import (
	"math/big"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// Real-time functions (core 6.5, decision 0133): a function declared with
// `uses realtime` may not allocate (XO1401), loop without a bound
// (XO1402), call what is not known to be real-time (XO1403), recurse
// (XO1404), or block (XO1405). The rules are syntactic and run after
// every body is checked, on the recorded resolutions.

// realtimeLabel is the discipline written in a uses clause. It is not an
// effect: resolveSig removes it before the effects are resolved.
const realtimeLabel = "realtime"

// rtBounded lists the prelude functions and methods a real-time function
// may call: they compute a scalar (or an Option of an element) without
// allocating, in time bounded by their operands. `List.set` writes in
// place when the List is unique (decision 0046); a shared List is copied,
// which the native runtime reports as XO-F007.
var rtBounded = map[string]bool{
	"fault":   true,
	"Int.abs": true, "Int.pow": true, "Int.wrapping_add": true, "Int.wrapping_sub": true,
	"Int.wrapping_mul": true, "Int.saturating_add": true, "Int.saturating_sub": true,
	"Int.saturating_mul": true, "Int.checked_add": true, "Int.checked_sub": true,
	"Int.checked_mul": true, "Int.checked_div": true,
	"Int8.try": true, "Int16.try": true, "Int32.try": true, "Int64.try": true,
	"UInt8.try": true, "UInt16.try": true, "UInt32.try": true, "UInt64.try": true, "Rune.try": true,
	"Float.floor": true, "Float.ceil": true, "Float.round": true, "Float.trunc": true,
	"Float.abs": true, "Float.sqrt": true, "Float.is_nan": true,
	"Str.len": true, "Str.is_empty": true, "Str.contains": true, "Str.starts_with": true,
	"Str.ends_with": true, "Str.find": true,
	"Bytes.len": true, "Bytes.is_empty": true, "Bytes.decode_rune": true,
	"Rune.is_digit": true, "Rune.is_letter": true, "Rune.is_space": true,
	"Rune.to_lower": true, "Rune.to_upper": true, "Rune.is_print": true,
	"List.len": true, "List.is_empty": true, "List.get": true, "List.first": true,
	"List.last": true, "List.contains": true, "List.index_of": true, "List.set": true,
	"Map.len": true, "Map.is_empty": true, "Map.get": true, "Map.contains": true,
	"Map.get_or_fault": true,
	"Set.len":          true, "Set.is_empty": true, "Set.contains": true,
	"Option.is_some": true, "Option.is_none": true, "Option.or_fault": true,
	"Result.is_ok": true, "Result.is_err": true, "Result.ok": true, "Result.err": true,
	"Result.or_fault":    true,
	"Duration.from_secs": true, "Duration.from_secs_int": true, "Duration.from_millis": true,
	"Duration.from_nanos": true, "Duration.seconds": true, "Duration.millis": true,
	"Duration.nanos": true, "Duration.scale": true, "Duration.ceil_seconds": true,
	"Duration.abs":   true,
	"Time.from_unix": true, "Time.from_unix_millis": true, "Time.unix": true,
	"Time.unix_millis": true,
	"Atomic.get":       true, "Atomic.set": true, "Atomic.add": true, "Atomic.max": true,
	"Atomic.swap": true, "Atomic.compare_and_swap": true,
	"Chan.len": true, "Buf.len": true, "StrBuilder.len": true, "BytesBuilder.len": true,
	"Secret.reveal": true,
}

// rtBlocking lists the prelude functions and methods that wait for
// another task or the scheduler (XO1405).
var rtBlocking = map[string]bool{
	"dbg":       true,
	"Chan.send": true, "Chan.recv": true, "Chan.try_send": true, "Chan.try_recv": true,
	"Chan.close": true, "Chan.new": true,
	"Mutex.with": true, "Mutex.with_read": true, "Mutex.new": true,
	"Task.await":  true,
	"Scope.spawn": true, "Scope.spawn_background": true, "Scope.cancel": true, "Scope.group": true,
}

// rtFunc is one real-time function being checked.
type rtFunc struct {
	fo    *FuncObj
	calls []rtCall // calls of other real-time functions, for XO1404
}

type rtCall struct {
	to *FuncObj
	at token.Span
}

// checkRealtime applies the rules of core 6.5 to every function marked
// `uses realtime`, then reports recursion among them.
func (c *checker) checkRealtime() {
	var fns []*rtFunc
	byFn := map[*FuncObj]*rtFunc{}
	for _, fo := range c.realtimeFns {
		if fo.Decl == nil || fo.Decl.Body == nil || fo.Module.IsStd {
			continue
		}
		rf := &rtFunc{fo: fo}
		fns = append(fns, rf)
		byFn[fo] = rf
		w := &rtWalker{c: c, fn: rf}
		for _, cl := range fo.Decl.Clauses {
			switch x := cl.(type) {
			case *ast.RequiresClause:
				w.node(x.Cond)
			case *ast.EnsuresClause:
				w.node(x.Cond)
			}
		}
		w.node(fo.Decl.Body)
	}
	c.realtimeCycles(fns, byFn)
}

// realtimeCycles reports each cycle of calls among real-time functions
// once (XO1404), at the call that closes it.
func (c *checker) realtimeCycles(fns []*rtFunc, byFn map[*FuncObj]*rtFunc) {
	const (
		white = iota
		grey
		black
	)
	color := map[*rtFunc]int{}
	var stack []*rtFunc
	var stackCalls []rtCall
	var visit func(f *rtFunc)
	visit = func(f *rtFunc) {
		color[f] = grey
		stack = append(stack, f)
		for _, call := range f.calls {
			g := byFn[call.to]
			if g == nil {
				continue
			}
			switch color[g] {
			case white:
				stackCalls = append(stackCalls, call)
				visit(g)
				stackCalls = stackCalls[:len(stackCalls)-1]
			case grey:
				// the cycle: from g on the stack to f, then this call
				i := len(stack) - 1
				for stack[i] != g {
					i--
				}
				var names []string
				for _, s := range stack[i:] {
					names = append(names, "`"+s.fo.DisplayName()+"`")
				}
				names = append(names, "`"+g.fo.DisplayName()+"`")
				var d *diag.Diagnostic
				if f == g {
					d = c.errorf(diag.RealtimeRecursion, call.at, "real-time function `%s` calls itself", f.fo.DisplayName())
				} else {
					d = c.errorf(diag.RealtimeRecursion, call.at, "recursion among real-time functions: %s", strings.Join(names, " calls "))
					for _, sc := range stackCalls[i:] {
						note(d, sc.at, "`%s` is called here", sc.to.DisplayName())
					}
				}
				if d != nil {
					note(d, token.Span{}, "real-time code may not recurse, so its stack depth is bounded; rewrite the recursion as a loop with `bound N` (core 6.5)")
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[f] = black
	}
	for _, f := range fns {
		if color[f] == white {
			visit(f)
		}
	}
}

type rtWalker struct {
	c  *checker
	fn *rtFunc
}

func (w *rtWalker) report(code diag.Code, sp token.Span, format string, args ...any) *diag.Diagnostic {
	d := w.c.errorf(code, sp, format, args...)
	if d != nil {
		note(d, w.fn.fo.Decl.Name.Sp, "`%s` is real-time (`uses realtime`, core 6.5)", w.fn.fo.DisplayName())
	}
	return d
}

// node checks n and its descendants.
func (w *rtWalker) node(n ast.Node) {
	if n == nil || isNilNode(n) {
		return
	}
	c := w.c
	switch x := n.(type) {
	case *ast.FnLit:
		w.report(diag.RealtimeAlloc, x.Sp, "a closure allocates its environment in a real-time function")
		return
	case *ast.ListLit:
		w.report(diag.RealtimeAlloc, x.Sp, "a List literal allocates in a real-time function; build the List during initialization and write it by index")
	case *ast.MapLit:
		w.report(diag.RealtimeAlloc, x.Sp, "a Map literal allocates in a real-time function")
	case *ast.SetLit:
		w.report(diag.RealtimeAlloc, x.Sp, "a Set literal allocates in a real-time function")
	case *ast.StringLit:
		for _, p := range x.Parts {
			if p.Expr != nil {
				w.report(diag.RealtimeAlloc, x.Sp, "string interpolation allocates in a real-time function")
				break
			}
		}
	case *ast.BinaryExpr:
		if x.Op == token.Plus && w.growable(x) {
			w.report(diag.RealtimeAlloc, x.Sp, "`+` on `%s` allocates in a real-time function", deep(c.exprType[x]))
		}
	case *ast.IndexExpr:
		if r, ok := unparen(x.Index).(*ast.RangeExpr); ok {
			w.report(diag.RealtimeAlloc, r.Sp, "slicing allocates in a real-time function; pass the List and the bounds")
		}
	case *ast.AssignStmt:
		if x.Op == token.PlusAssign && w.growable(x.Target) {
			w.report(diag.RealtimeAlloc, x.Sp, "`+=` on `%s` allocates in a real-time function", deep(c.exprType[x.Target]))
		}
		if ix, ok := x.Target.(*ast.IndexExpr); ok && c.isPrelude(c.exprType[ix.X], "Map") {
			w.report(diag.RealtimeAlloc, x.Sp, "storing into a Map may grow it in a real-time function; use a List indexed by a small Int")
		}
	case *ast.ScopeExpr:
		w.report(diag.RealtimeBlocking, x.Sp, "`scope` waits for its tasks; real-time functions may not block")
	case *ast.SelectExpr:
		w.report(diag.RealtimeBlocking, x.Sp, "`select` waits on channels; real-time functions may not block")
	case *ast.WhileExpr:
		w.report(diag.RealtimeLoop, x.Sp, "an infinite loop in a real-time function; loops need `bound N`")
	case *ast.WhileStmt:
		if x.Bound == nil && x.Cond == nil && x.Value == nil {
			w.report(diag.RealtimeLoop, loopHead(x.Sp), "an infinite `while` loop in a real-time function; write `while cond bound N`")
		} else if x.Bound == nil {
			d := w.report(diag.RealtimeLoop, loopHead(x.Sp), "`while` loop without a bound in a real-time function")
			if head := whileHead(x); d != nil && head != nil {
				fix(d, "add `bound N` (choose N)", insertEdit(x.Sp.File, head.Span().End, " bound N"))
			}
		}
	case *ast.ForStmt:
		w.forStmt(x)
		w.node(x.Bound)
		w.node(x.Body)
		return
	case *ast.CallExpr:
		if w.call(x) {
			return
		}
	}
	for _, ch := range children(n) {
		w.node(ch)
	}
}

// forStmt checks the bound of a `for` and its iterated value.
func (w *rtWalker) forStmt(x *ast.ForStmt) {
	c := w.c
	it := unparen(x.Iter)
	if c.isPrelude(c.exprType[x.Iter], "Chan") {
		w.report(diag.RealtimeBlocking, x.Iter.Span(), "a `for` over a channel waits for values; real-time functions may not block")
		w.node(x.Iter)
		return
	}
	if r, ok := it.(*ast.RangeExpr); ok {
		w.node(r.Lo)
		w.node(r.Hi)
		if x.Bound == nil {
			if _, ok := rangeCount(c, r); !ok {
				d := w.report(diag.RealtimeLoop, x.Iter.Span(), "`for` over a range whose ends are not constant needs `bound N` in a real-time function")
				if d != nil {
					fix(d, "add `bound N` (choose N)", insertEdit(x.Sp.File, x.Iter.Span().End, " bound N"))
				}
			}
		}
		return
	}
	// `for (i, x) in xs.indexed()` walks xs without building the pairs
	if call, ok := it.(*ast.CallExpr); ok {
		if ci := c.callInfo[call]; ci != nil && ci.fn != nil && ci.fn.Module == c.prelude && ci.fn.Name == "indexed" && ci.recvExpr != nil {
			w.node(ci.recvExpr)
		} else {
			w.node(x.Iter)
		}
	} else {
		w.node(x.Iter)
	}
	if x.Bound == nil {
		d := w.report(diag.RealtimeLoop, x.Iter.Span(), "`for` over a collection needs `bound N` in a real-time function")
		if d != nil {
			fix(d, "add `bound N` (choose N)", insertEdit(x.Sp.File, x.Iter.Span().End, " bound N"))
		}
	}
}

// growable reports whether e has a type whose `+` builds a new value
// (Str, List, Bytes).
func (w *rtWalker) growable(e ast.Expr) bool {
	t := w.c.exprType[e]
	return w.c.isPrelude(t, "Str") || w.c.isPrelude(t, "List") || w.c.isPrelude(t, "Bytes")
}

// call checks a call; it returns true when the arguments are not to be
// walked (the message of `fault`).
func (w *rtWalker) call(x *ast.CallExpr) bool {
	c := w.c
	ci := c.callInfo[x]
	if ci == nil {
		// a variant constructor, a conversion, or a call that did not check
		return false
	}
	fo := ci.fn
	if fo == nil {
		w.report(diag.RealtimeCall, x.Fun.Span(), "call of a function value in a real-time function; its callee is not known to be real-time")
		return false
	}
	if fo.Module != nil && fo.Module.IsPrelude {
		key := fo.Name
		if fo.Recv != nil {
			key = fo.Recv.Name + "." + fo.Name
		}
		switch {
		case key == "fault":
			w.node(x.Fun)
			return true
		case rtBounded[key]:
		case rtBlocking[key]:
			w.report(diag.RealtimeBlocking, x.Fun.Span(), "`%s` blocks or synchronizes with other tasks; real-time functions may not block", key)
		case fo.Iface:
			w.report(diag.RealtimeBlocking, x.Fun.Span(), "`%s` is a capability call; real-time functions perform no effects except `ffi`", key)
		default:
			w.report(diag.RealtimeAlloc, x.Fun.Span(), "`%s` may allocate or run for a time its operands do not bound; it is not allowed in a real-time function", key)
		}
		return false
	}
	if fo.Module != nil && fo.Module.C != nil {
		return false
	}
	if fo.Iface {
		w.report(diag.RealtimeCall, x.Fun.Span(), "call of interface method `%s` in a real-time function; its implementation is not known", fo.DisplayName())
		return false
	}
	if !fo.Realtime {
		d := w.report(diag.RealtimeCall, x.Fun.Span(), "`%s` is not real-time; a real-time function may call only real-time functions", fo.DisplayName())
		if d != nil && fo.Module != nil && !fo.Module.IsStd && fo.Decl != nil && fo.Module.Dependency == "" {
			note(d, fo.Decl.Name.Sp, "mark `%s` with `uses realtime` if it follows the rules", fo.DisplayName())
		}
		return false
	}
	w.fn.calls = append(w.fn.calls, rtCall{to: fo, at: x.Fun.Span()})
	return false
}

// loopHead is the span of a loop's first line (its keyword and header).
func loopHead(sp token.Span) token.Span {
	if sp.End.Line > sp.Start.Line {
		sp.End = token.Pos{Line: sp.Start.Line, Col: sp.Start.Col + 5, Offset: sp.Start.Offset + 5}
	}
	return sp
}

// checkLoopBound checks the `bound N` of a loop (core 3.5): a constant
// Int that is not negative, and for a `for` over a constant range, at
// least the range's length.
func (c *checker) checkLoopBound(bound ast.Expr, iter ast.Expr) {
	if bound == nil {
		return
	}
	c.check(bound, c.named("Int"))
	n, ok := constInt(c, bound, 0)
	if !ok {
		c.errorf(diag.RealtimeLoop, bound.Span(), "a loop bound must be a constant Int (a literal or a `const`)")
		return
	}
	if n.Sign() < 0 {
		c.errorf(diag.RealtimeLoop, bound.Span(), "a loop bound must not be negative (it is %s)", n)
		return
	}
	if r, ok := unparen(iterOrNil(iter)).(*ast.RangeExpr); ok {
		if cnt, ok := rangeCount(c, r); ok && cnt.Cmp(n) > 0 {
			d := c.errorf(diag.RealtimeLoop, iter.Span(), "this range runs %s times, more than its bound %s", cnt, n)
			if d != nil {
				note(d, bound.Span(), "the bound")
			}
		}
	}
}

func iterOrNil(e ast.Expr) ast.Expr {
	if e == nil {
		return &ast.BadExpr{}
	}
	return e
}

// rangeCount is the number of values of a range with constant ends.
func rangeCount(c *checker, r *ast.RangeExpr) (*big.Int, bool) {
	if r.Lo == nil || r.Hi == nil {
		return nil, false
	}
	lo, ok1 := constInt(c, r.Lo, 0)
	hi, ok2 := constInt(c, r.Hi, 0)
	if !ok1 || !ok2 {
		return nil, false
	}
	n := new(big.Int).Sub(hi, lo)
	if r.Inclusive {
		n.Add(n, big.NewInt(1))
	}
	if n.Sign() < 0 {
		n.SetInt64(0)
	}
	return n, true
}

// constInt evaluates a constant Int expression: literals, `const` names,
// parentheses, unary minus, and + - * / % of constants.
func constInt(c *checker, e ast.Expr, depth int) (*big.Int, bool) {
	if depth > 32 || e == nil {
		return nil, false
	}
	switch x := e.(type) {
	case *ast.IntLit:
		v := strings.ReplaceAll(x.Value, "_", "")
		n, ok := new(big.Int).SetString(v, 0)
		return n, ok
	case *ast.ParenExpr:
		return constInt(c, x.X, depth+1)
	case *ast.UnaryExpr:
		if x.Op != token.Minus {
			return nil, false
		}
		n, ok := constInt(c, x.X, depth+1)
		if !ok {
			return nil, false
		}
		return new(big.Int).Neg(n), true
	case *ast.BinaryExpr:
		a, ok1 := constInt(c, x.X, depth+1)
		b, ok2 := constInt(c, x.Y, depth+1)
		if !ok1 || !ok2 {
			return nil, false
		}
		switch x.Op {
		case token.Plus:
			return new(big.Int).Add(a, b), true
		case token.Minus:
			return new(big.Int).Sub(a, b), true
		case token.Star:
			return new(big.Int).Mul(a, b), true
		case token.Slash:
			if b.Sign() == 0 {
				return nil, false
			}
			return new(big.Int).Quo(a, b), true
		case token.Percent:
			if b.Sign() == 0 {
				return nil, false
			}
			return new(big.Int).Rem(a, b), true
		}
		return nil, false
	case *ast.Ident:
		if v := c.identObj[x]; v != nil {
			if cd, ok := v.decl.(*ast.ConstDecl); ok && v.isConst {
				return constInt(c, cd.Value, depth+1)
			}
			return nil, false
		}
		if o, ok := c.rec.ref[x].(*ConstObj); ok && o.Decl != nil {
			return constInt(c, o.Decl.Value, depth+1)
		}
	case *ast.SelectorExpr:
		if o, ok := c.rec.ref[x].(*ConstObj); ok && o.Decl != nil {
			return constInt(c, o.Decl.Value, depth+1)
		}
	}
	return nil, false
}

// LoopBound returns the constant bound of a loop's `bound N` clause.
func (i *Info) LoopBound(e ast.Expr) (int64, bool) {
	n, ok := constInt(i.c, e, 0)
	if !ok || !n.IsInt64() {
		return 0, false
	}
	return n.Int64(), true
}

func rtLabelError(c *checker, e *ast.Effect) {
	d := c.errorf(diag.UnknownEffect, e.Sp, "`realtime` marks a function declaration; it cannot appear here")
	if d != nil {
		note(d, token.Span{}, "write `uses realtime` on a `fn` declaration; closures, function types, and tests cannot be real-time (core 6.5)")
	}
}
