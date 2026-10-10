package check

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// flowBody runs the flow checks on one body after typing: moves of
// move only values (XO0510, core 5.3) and lost updates (XO0520, core 5.2).
func (c *checker) flowBody(b *bodyInfo) {
	mv := &moveWalker{c: c, moved: map[*VarObj]token.Span{}}
	mv.block(b.body, true)
	c.lostUpdates(b.body)
	c.storedUpdates(b.body)
	c.splitCriticalSections(b.body)
}

// ---------------------------------------------------------------------------
// Split critical sections (core 5.4)

// pathText renders `a` or `a.b.c` for comparing mutex receivers; other
// expressions give "".
func pathText(e ast.Expr) string {
	switch x := unparen(e).(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		if p := pathText(x.X); p != "" {
			return p + "." + x.Sel.Name
		}
	}
	return ""
}

// mutexWith returns the receiver path of a `<mutex>.with(...)` call, or
// with read set, also of a `<mutex>.with_read(...)` call (a value read
// there is as stale as one read in `with`, but a later `with_read` acts
// on nothing).
func (c *checker) mutexWith(e ast.Expr, read bool) (string, *ast.CallExpr) {
	call, ok := unparen(e).(*ast.CallExpr)
	if !ok {
		return "", nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "with" && !(read && sel.Sel.Name == "with_read") {
		return "", nil
	}
	n, ok := resolve(c.exprType[sel.X]).(*Named)
	if !ok || n.Obj.Module != c.prelude || n.Obj.Name != "Mutex" {
		return "", nil
	}
	return pathText(sel.X), call
}

// splitCriticalSections warns (XO0804) when a value read inside one
// `m.with(...)` (or derived from such a value) is used inside a later
// `m.with(...)` on the same mutex: another task may have changed the state
// in between, so the second step acts on a stale read (check then act).
func (c *checker) splitCriticalSections(body *ast.Block) {
	tainted := map[*VarObj]string{}
	// decided[m] is a stale value that a branch already acted on: a later
	// `m.with` runs only because of a check made in an earlier section.
	decided := map[string]*ast.Ident{}
	refs := func(n ast.Node, mutex string) *ast.Ident {
		var hit *ast.Ident
		inspect(n, func(m ast.Node) bool {
			if hit != nil {
				return false
			}
			if id, ok := m.(*ast.Ident); ok {
				if v := c.identObj[id]; v != nil && tainted[v] != "" && (mutex == "" || tainted[v] == mutex) {
					hit = id
				}
			}
			return true
		})
		return hit
	}
	bind := func(p ast.Pattern, mutex string) {
		inspect(p, func(m ast.Node) bool {
			if bp, ok := m.(*ast.BindPat); ok {
				if v := c.identObj[bp.Name]; v != nil {
					tainted[v] = mutex
				}
			}
			return true
		})
	}
	inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.LetStmt:
			if mu, _ := c.mutexWith(x.Value, true); mu != "" {
				bind(x.Pattern, mu)
			} else if id := refs(x.Value, ""); id != nil {
				bind(x.Pattern, tainted[c.identObj[id]])
			}
		case *ast.VarStmt:
			if v := c.identObj[x.Name]; v != nil {
				if mu, _ := c.mutexWith(x.Value, true); mu != "" {
					tainted[v] = mu
				} else if id := refs(x.Value, ""); id != nil {
					tainted[v] = tainted[c.identObj[id]]
				}
			}
		case *ast.IfExpr:
			cond := x.Cond
			if cond == nil {
				cond = x.Value
			}
			if id := refs(cond, ""); id != nil {
				if mu := tainted[c.identObj[id]]; decided[mu] == nil {
					decided[mu] = id
				}
			}
		case *ast.CallExpr:
			mu, call := c.mutexWith(x, false)
			if mu == "" || c.splitWarned[call] {
				return true
			}
			if id := decided[mu]; id != nil {
				c.splitWarned[call] = true
				d := c.warnf(diag.SplitCriticalSection, call.Sp, "this `%s.with` acts on a decision about `%s`, which was read in an earlier `%s.with`", mu, id.Name, mu)
				if d != nil {
					note(d, token.Span{}, "another task may have changed it since; do the check and the action in one `with` (core 5.4)")
				}
				return true
			}
			for _, a := range call.Args {
				if id := refs(a.Value, mu); id != nil {
					c.splitWarned[call] = true
					d := c.warnf(diag.SplitCriticalSection, id.Sp, "`%s` was read in an earlier `%s.with`; another task may have changed it since", id.Name, mu)
					if d != nil {
						note(d, token.Span{}, "do the read and the update in one `with` so the check and the action are atomic (core 5.4)")
					}
					break
				}
			}
		}
		return true
	})
}

// ---------------------------------------------------------------------------
// Moves (core 5.3)

func (c *checker) isMoveOnly(t Type) bool {
	n, ok := resolve(t).(*Named)
	return ok && n.Obj.Module == c.prelude && (n.Obj.Name == "Buf" || n.Obj.Name == "StrBuilder" || n.Obj.Name == "BytesBuilder")
}

type moveWalker struct {
	c     *checker
	moved map[*VarObj]token.Span
}

func (w *moveWalker) copyState() map[*VarObj]token.Span {
	m := make(map[*VarObj]token.Span, len(w.moved))
	for k, v := range w.moved {
		m[k] = v
	}
	return m
}

func union(a, b map[*VarObj]token.Span) map[*VarObj]token.Span {
	out := make(map[*VarObj]token.Span, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if _, ok := out[k]; !ok {
			out[k] = v
		}
	}
	return out
}

func (w *moveWalker) block(b *ast.Block, tailMoves bool) {
	if b == nil {
		return
	}
	for i, s := range b.Stmts {
		if es, ok := s.(*ast.ExprStmt); ok && i == len(b.Stmts)-1 && tailMoves {
			w.expr(es.X, true)
			continue
		}
		w.stmt(s)
	}
}

func (w *moveWalker) stmt(s ast.Stmt) {
	switch x := s.(type) {
	case *ast.ExprStmt:
		w.expr(x.X, false)
	case *ast.LetStmt:
		w.expr(x.Value, true)
		// a binding declared in a loop body is a new binding each time
		// round: a move in the last iteration does not carry over
		if bp, ok := x.Pattern.(*ast.BindPat); ok {
			if v := w.c.identObj[bp.Name]; v != nil {
				delete(w.moved, v)
			}
		}
	case *ast.VarStmt:
		w.expr(x.Value, true)
		if v := w.c.identObj[x.Name]; v != nil {
			delete(w.moved, v)
		}
	case *ast.AssignStmt:
		w.expr(x.Value, true)
		if id, ok := x.Target.(*ast.Ident); ok && x.Op == token.Assign {
			if v := w.c.identObj[id]; v != nil {
				delete(w.moved, v) // reassignment gives the binding a new value
			}
			return
		}
		w.expr(x.Target, false)
	case *ast.DeferStmt:
		w.stmt(x.Body)
	case *ast.ReturnStmt:
		if x.Value != nil {
			w.expr(x.Value, true)
		}
	case *ast.ForStmt:
		w.expr(x.Iter, false)
		w.loop(x.Body)
	case *ast.WhileStmt:
		if x.Cond != nil {
			w.expr(x.Cond, false)
		}
		if x.Value != nil {
			w.expr(x.Value, false)
		}
		w.loop(x.Body)
	}
}

// loop walks a loop body twice so a move in one iteration is seen by the
// next; the loop may also not run at all.
func (w *moveWalker) loop(body *ast.Block) {
	before := w.copyState()
	w.block(body, false)
	w.block(body, false)
	w.moved = union(before, w.moved)
}

// expr walks e; move is true when e's value is moved (bound, passed to a
// non var parameter, returned, stored).
func (w *moveWalker) expr(e ast.Expr, move bool) {
	switch x := e.(type) {
	case nil:
		return
	case *ast.Ident:
		v := w.c.identObj[x]
		if v == nil || !w.c.isMoveOnly(v.Type) {
			return
		}
		if at, ok := w.moved[v]; ok {
			d := w.c.errorf(diag.UseOfMovedValue, x.Sp, "use of moved value `%s`", x.Name)
			note(d, at, "`%s` was moved here", x.Name)
			note(d, token.Span{}, "`Buf`, `StrBuilder`, and `BytesBuilder` are move only: assigning, passing, returning, freezing, or capturing in a closure moves them (core 5.3)")
			return
		}
		if move {
			w.moved[v] = x.Sp
		}
	case *ast.ParenExpr:
		w.expr(x.X, move)
	case *ast.CallExpr:
		ci := w.c.callInfo[x]
		var recv ast.Expr
		if ci != nil {
			recv = ci.recvExpr
		}
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok && recv != nil && sel.X == recv {
			w.expr(recv, ci.fn != nil && ci.fn.Moves)
		} else if ta, ok := x.Fun.(*ast.TypeArgsExpr); ok && recv != nil {
			if sel, ok := ta.X.(*ast.SelectorExpr); ok && sel.X == recv {
				w.expr(recv, ci.fn != nil && ci.fn.Moves)
			}
		} else {
			w.expr(x.Fun, false)
		}
		varArg := map[ast.Expr]bool{}
		known := map[ast.Expr]bool{}
		if ci != nil {
			for _, p := range ci.params {
				known[p.arg] = true
				if p.isVar {
					varArg[p.arg] = true
				}
			}
		}
		for _, a := range x.Args {
			if !known[a.Value] && ci != nil && ci.fn == nil && len(ci.params) == 0 {
				w.expr(a.Value, true)
				continue
			}
			w.expr(a.Value, !varArg[a.Value])
		}
	case *ast.FnLit:
		// captured move only values move into the closure
		inspect(x.Body, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				if v := w.c.identObj[id]; v != nil && v.ctx != nil && w.c.isMoveOnly(v.Type) && !declaredWithin(v, x) {
					w.expr(id, true)
				}
			}
			return true
		})
	case *ast.Block:
		w.block(x, move)
	case *ast.IfExpr:
		if x.Cond != nil {
			w.expr(x.Cond, false)
		}
		if x.Value != nil {
			w.expr(x.Value, false)
		}
		start := w.copyState()
		w.block(x.Then, move)
		afterThen := w.moved
		w.moved = start
		if x.Else != nil {
			w.expr(x.Else, move)
		}
		w.moved = union(afterThen, w.moved)
	case *ast.MatchExpr:
		w.expr(x.Subject, false)
		start := w.copyState()
		result := w.copyState()
		for _, arm := range x.Arms {
			w.moved = copyMap(start)
			if arm.Guard != nil {
				w.expr(arm.Guard, false)
			}
			if arm.Body != nil {
				if es, ok := arm.Body.(*ast.ExprStmt); ok {
					w.expr(es.X, move)
				} else {
					w.stmt(arm.Body)
				}
			}
			result = union(result, w.moved)
		}
		w.moved = result
	case *ast.SelectExpr:
		start := w.copyState()
		result := w.copyState()
		for _, arm := range x.Arms {
			w.moved = copyMap(start)
			w.expr(arm.Op, false)
			w.expr(arm.After, false)
			if arm.Body != nil {
				w.stmt(arm.Body)
			}
			result = union(result, w.moved)
		}
		w.moved = result
	case *ast.WhileExpr:
		w.loop(x.Body)
	case *ast.ScopeExpr:
		w.block(x.Body, move)
	case *ast.StructLit:
		for _, f := range x.Fields {
			w.expr(f.Value, true)
		}
	case *ast.WithExpr:
		w.expr(x.X, false)
		for _, f := range x.Fields {
			w.expr(f.Value, true)
		}
	case *ast.ListLit:
		for _, el := range x.Elems {
			w.expr(el, true)
		}
	case *ast.SetLit:
		for _, el := range x.Elems {
			w.expr(el, true)
		}
	case *ast.TupleExpr:
		for _, el := range x.Elems {
			w.expr(el, true)
		}
	case *ast.MapLit:
		for _, en := range x.Entries {
			w.expr(en.Key, true)
			w.expr(en.Value, true)
		}
	default:
		for _, ch := range children(e) {
			if ce, ok := ch.(ast.Expr); ok {
				w.expr(ce, false)
			} else if cs, ok := ch.(ast.Stmt); ok {
				w.stmt(cs)
			}
		}
	}
}

func copyMap(m map[*VarObj]token.Span) map[*VarObj]token.Span {
	out := make(map[*VarObj]token.Span, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// declaredWithin reports whether v is declared inside the closure f.
func declaredWithin(v *VarObj, f *ast.FnLit) bool {
	return v.Span.Start.Offset >= f.Sp.Start.Offset && v.Span.End.Offset <= f.Sp.End.Offset && v.Span.File == f.Sp.File
}

// ---------------------------------------------------------------------------
// Lost updates (core 5.2, decision 0023)

// copyOutInit reports whether e copies an element out of a collection:
// `m.get(k)`, `m[k]`, `xs[i]`, `xs.get(i)`, `first`, `last`, `find`,
// optionally followed by `??`, `?`, or `.or_err(...)?` (core 5.2, decision
// 0034).
func (c *checker) copyOutInit(e ast.Expr) bool {
	for {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
			continue
		case *ast.TryExpr:
			e = x.X
			if call, ok := e.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "or_err" {
					e = sel.X
				}
			}
			continue
		case *ast.BinaryExpr:
			if x.Op != token.Coalesce {
				return false
			}
			e = x.X
			continue
		}
		break
	}
	isColl := func(t Type) bool {
		n, ok := resolve(t).(*Named)
		return ok && n.Obj.Module == c.prelude && (n.Obj.Name == "List" || n.Obj.Name == "Map")
	}
	switch x := e.(type) {
	case *ast.IndexExpr:
		if _, isRange := x.Index.(*ast.RangeExpr); isRange {
			return false
		}
		return isColl(c.exprType[x.X])
	case *ast.CallExpr:
		sel, ok := x.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		switch sel.Sel.Name {
		case "get", "first", "last", "find":
			return isColl(c.exprType[sel.X])
		}
	}
	return false
}

type luEvent struct {
	id      *ast.Ident
	mutate  bool
	store   bool
	inDefer bool
	loops   []ast.Node
}

// unparen strips parentheses around an expression.
func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

func (c *checker) lostUpdates(body *ast.Block) {
	// candidate bindings: `var x = <copy out>` in this body (not in nested
	// closures, which are their own bodies)
	type cand struct {
		v    *VarObj
		decl *ast.VarStmt
		// loops enclosing the declaration
		loops map[ast.Node]bool
	}
	var cands []*cand
	var stack []ast.Node
	var walkDecl func(n ast.Node)
	walkDecl = func(n ast.Node) {
		if n == nil || isNilNode(n) {
			return
		}
		switch x := n.(type) {
		case *ast.FnLit:
			return
		case *ast.VarStmt:
			if c.copyOutInit(x.Value) {
				if v := c.identObj[x.Name]; v != nil {
					lp := map[ast.Node]bool{}
					for _, l := range stack {
						lp[l] = true
					}
					cands = append(cands, &cand{v: v, decl: x, loops: lp})
				}
			}
		case *ast.ForStmt, *ast.WhileStmt, *ast.WhileExpr:
			stack = append(stack, n)
			for _, ch := range children(n) {
				walkDecl(ch)
			}
			stack = stack[:len(stack)-1]
			return
		}
		for _, ch := range children(n) {
			walkDecl(ch)
		}
	}
	walkDecl(body)
	if len(cands) == 0 {
		return
	}
	// mutation sites: assignment targets, var receivers, var arguments
	mutating := map[*ast.Ident]bool{}
	inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if id, ok := c.placeRoot(x.Target); ok {
				mutating[id] = true
			}
		case *ast.CallExpr:
			if ci := c.callInfo[x]; ci != nil {
				if ci.fn != nil && ci.fn.RecvVar && ci.recvExpr != nil {
					if id, ok := c.placeRoot(ci.recvExpr); ok {
						mutating[id] = true
					}
				}
				for _, p := range ci.params {
					if p.isVar {
						if id, ok := c.placeRoot(p.arg); ok {
							mutating[id] = true
						}
					}
				}
			}
		}
		return true
	})
	// store sites: the whole value passed as a call argument (which may store
	// it, as `m.put(k, v)` does) or assigned as a whole (`m[k] = v`,
	// `st.cur = v`). Reads, field reads, and returns do not count (core 5.2).
	storing := map[*ast.Ident]bool{}
	inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			for _, a := range x.Args {
				if id, ok := unparen(a.Value).(*ast.Ident); ok {
					storing[id] = true
				}
			}
		case *ast.AssignStmt:
			if id, ok := unparen(x.Value).(*ast.Ident); ok {
				storing[id] = true
			}
		}
		return true
	})
	for _, cd := range cands {
		var events []luEvent
		var loops []ast.Node
		var walk func(n ast.Node, inDefer bool)
		walk = func(n ast.Node, inDefer bool) {
			if n == nil || isNilNode(n) {
				return
			}
			switch x := n.(type) {
			case *ast.Ident:
				if c.identObj[x] == cd.v && x != cd.decl.Name {
					events = append(events, luEvent{id: x, mutate: mutating[x], store: !mutating[x] && storing[x], inDefer: inDefer, loops: append([]ast.Node{}, loops...)})
				}
				return
			case *ast.DeferStmt:
				walk(x.Body, true)
				return
			case *ast.ForStmt, *ast.WhileStmt, *ast.WhileExpr:
				loops = append(loops, n)
				for _, ch := range children(n) {
					walk(ch, inDefer)
				}
				loops = loops[:len(loops)-1]
				return
			case *ast.AssignStmt:
				// the value is evaluated before the target is written
				walk(x.Value, inDefer)
				walk(x.Target, inDefer)
				return
			}
			for _, ch := range children(n) {
				walk(ch, inDefer)
			}
		}
		walk(body, false)
		var lost *luEvent
		for i, ev := range events {
			if !ev.mutate {
				continue
			}
			// A copy out of a collection that is changed must be stored again
			// after its last change; reading it afterwards is not enough,
			// since the collection still holds the old value.
			readAfter := false
			for j, r := range events {
				if !r.store {
					continue
				}
				if r.inDefer || j > i {
					readAfter = true
					break
				}
				// a read earlier in a loop body that also contains the
				// mutation runs again on the next iteration
				for _, l := range ev.loops {
					if cd.loops[l] {
						continue
					}
					for _, rl := range r.loops {
						if rl == l {
							readAfter = true
						}
					}
				}
				if readAfter {
					break
				}
			}
			if !readAfter {
				lost = &events[i]
			}
		}
		if lost == nil {
			continue
		}
		c.lostVars[cd.v] = true
		d := c.errorf(diag.LostUpdate, lost.id.Sp, "mutation of `%s` is never written back", cd.v.Name)
		if d != nil {
			note(d, cd.decl.Value.Span(), "`%s` is a copy of a collection element taken here; changing it does not change the collection (core 5.2)", cd.v.Name)
			note(d, token.Span{}, "write it back (for example `m.put(k, %s)`) or mutate in place with `update` (core 5.2)", cd.v.Name)
		}
	}
}

// ---------------------------------------------------------------------------
// Values mutated after being stored (core 5.2, decision 0034)

// storeMethods are the collection methods that store an argument.
var storeMethods = map[string]bool{"push": true, "put": true, "insert": true, "set": true, "add": true, "update": true}

// isCollection reports whether t is a prelude List, Map, or Set.
func (c *checker) isCollection(t Type) bool {
	n, ok := resolve(t).(*Named)
	return ok && n.Obj.Module == c.prelude && (n.Obj.Name == "List" || n.Obj.Name == "Map" || n.Obj.Name == "Set")
}

type suKind int

const (
	suStore  suKind = iota // the value is stored into a collection
	suReset                // `v = ...` gives the binding a new value
	suMutate               // field assignment, var receiver, var argument
)

type suEvent struct {
	kind    suKind
	id      *ast.Ident
	inDefer bool
	loops   []ast.Node
}

// storedUpdates reports XO0520 for a local `var` that is mutated after
// being stored into a collection (`xs.push(v)`, `m.put(k, v)`, `m[k] = v`)
// and not stored again: the stored copy does not see the change.
func (c *checker) storedUpdates(body *ast.Block) {
	type cand struct {
		v     *VarObj
		loops map[ast.Node]bool // loops enclosing the declaration
	}
	var cands []*cand
	var stack []ast.Node
	var walkDecl func(n ast.Node)
	walkDecl = func(n ast.Node) {
		if n == nil || isNilNode(n) {
			return
		}
		switch x := n.(type) {
		case *ast.FnLit:
			return
		case *ast.VarStmt:
			if v := c.identObj[x.Name]; v != nil && !c.lostVars[v] {
				lp := map[ast.Node]bool{}
				for _, l := range stack {
					lp[l] = true
				}
				cands = append(cands, &cand{v: v, loops: lp})
			}
		case *ast.ForStmt, *ast.WhileStmt, *ast.WhileExpr:
			stack = append(stack, n)
			for _, ch := range children(n) {
				walkDecl(ch)
			}
			stack = stack[:len(stack)-1]
			return
		}
		for _, ch := range children(n) {
			walkDecl(ch)
		}
	}
	walkDecl(body)
	if len(cands) == 0 {
		return
	}
	// classify identifier occurrences
	kinds := map[*ast.Ident]suKind{}
	isStoredArg := func(e ast.Expr) (*ast.Ident, bool) {
		for {
			p, ok := e.(*ast.ParenExpr)
			if !ok {
				break
			}
			e = p.X
		}
		id, ok := e.(*ast.Ident)
		return id, ok
	}
	inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if id, ok := x.Target.(*ast.Ident); ok && x.Op == token.Assign {
				kinds[id] = suReset
			} else if id, ok := c.placeRoot(x.Target); ok {
				kinds[id] = suMutate
			}
			if ix, ok := x.Target.(*ast.IndexExpr); ok && x.Op == token.Assign && c.isCollection(c.exprType[ix.X]) {
				if id, ok := isStoredArg(x.Value); ok {
					kinds[id] = suStore
				}
			}
		case *ast.CallExpr:
			ci := c.callInfo[x]
			if ci == nil {
				return true
			}
			if ci.fn != nil && ci.fn.RecvVar && ci.recvExpr != nil {
				if id, ok := c.placeRoot(ci.recvExpr); ok {
					kinds[id] = suMutate
				}
				if storeMethods[ci.fn.Name] && ci.fn.Recv != nil && ci.fn.Recv.Module == c.prelude && c.isCollection(c.exprType[ci.recvExpr]) {
					for _, p := range ci.params {
						if id, ok := isStoredArg(p.arg); ok && !p.isVar {
							kinds[id] = suStore
						}
					}
				}
			}
			for _, p := range ci.params {
				if p.isVar {
					if id, ok := c.placeRoot(p.arg); ok {
						kinds[id] = suMutate
					}
				}
			}
		}
		return true
	})
	for _, cd := range cands {
		var events []suEvent
		var loops []ast.Node
		var walk func(n ast.Node, inDefer bool)
		walk = func(n ast.Node, inDefer bool) {
			if n == nil || isNilNode(n) {
				return
			}
			switch x := n.(type) {
			case *ast.Ident:
				if c.identObj[x] != cd.v {
					return
				}
				if k, ok := kinds[x]; ok {
					events = append(events, suEvent{kind: k, id: x, inDefer: inDefer, loops: append([]ast.Node{}, loops...)})
				}
				return
			case *ast.DeferStmt:
				walk(x.Body, true)
				return
			case *ast.ForStmt, *ast.WhileStmt, *ast.WhileExpr:
				loops = append(loops, n)
				for _, ch := range children(n) {
					walk(ch, inDefer)
				}
				loops = loops[:len(loops)-1]
				return
			case *ast.AssignStmt:
				walk(x.Value, inDefer)
				walk(x.Target, inDefer)
				return
			}
			for _, ch := range children(n) {
				walk(ch, inDefer)
			}
		}
		walk(body, false)
		// a loop around both events that does not enclose the declaration
		// runs them again in the next iteration
		sharedLoop := func(a, b suEvent) bool {
			for _, l := range a.loops {
				if cd.loops[l] {
					continue
				}
				for _, m := range b.loops {
					if m == l {
						return true
					}
				}
			}
			return false
		}
		resetBetween := func(i, j int) bool {
			for k := i + 1; k < j; k++ {
				if events[k].kind == suReset {
					return true
				}
			}
			return false
		}
		var lost *suEvent
		var storedAt *suEvent
		for i, ev := range events {
			if ev.kind != suMutate || ev.inDefer {
				continue
			}
			var before *suEvent
			for j := range events {
				s := events[j]
				if s.kind != suStore || s.inDefer {
					continue
				}
				if (j < i && !resetBetween(j, i)) || (j > i && sharedLoop(ev, s)) {
					before = &events[j]
					break
				}
			}
			if before == nil {
				continue
			}
			again := false
			for j := range events {
				s := events[j]
				if s.kind != suStore {
					continue
				}
				if s.inDefer || (j > i && !resetBetween(i, j)) || (j < i && sharedLoop(ev, s)) {
					again = true
					break
				}
			}
			if !again {
				lost, storedAt = &events[i], before
			}
		}
		if lost == nil {
			continue
		}
		c.lostVars[cd.v] = true
		d := c.errorf(diag.LostUpdate, lost.id.Sp, "mutation of `%s` is never written back", cd.v.Name)
		if d != nil {
			note(d, storedAt.id.Sp, "`%s` was stored into a collection here; the stored copy does not see later changes (core 5.2)", cd.v.Name)
			note(d, token.Span{}, "make the change before storing `%s`, or store it again after the change", cd.v.Name)
		}
	}
}
