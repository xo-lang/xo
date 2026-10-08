package check

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// Task groups (core 7.1, decision 0032).
//
// A `task.Group` is created by `s.group()` inside `scope s` or received as
// `os.tasks`. Its tasks belong to the scope that created it, so:
//
//   - A task started on a group whose origin is visible in the function (a
//     local bound to `s.group()`, `os.tasks` of `main` or of a test) is a
//     child of that scope: an error that is not consumed by `await` must
//     convert to the error type of the function that owns the scope
//     (XO0801), exactly like `s.spawn`.
//   - A task started on any other group (a struct field, a parameter) fails
//     a scope this function cannot see, so it must not fail at all unless
//     its handle is bound and awaited (XO0801), the rule already used for
//     scope handles received from elsewhere.
//   - A value that contains a group created by `s.group()` must not leave
//     that scope: as the scope's value, by `return`, by assignment or a
//     `var` method call on a binding declared outside the scope, or on a
//     channel (XO0802). `os.tasks` lives for the whole program and may be
//     stored anywhere.

// isTaskGroup reports whether t is `task.Group`.
func isTaskGroup(t Type) bool {
	n, ok := resolve(t).(*Named)
	return ok && n.Obj.Module != nil && n.Obj.Module.Path == "std/task" && n.Obj.Name == "Group"
}

// containsGroup reports whether a value of type t can hold a task group:
// the type itself, a type argument, a tuple element, or a field of a user
// struct or enum, at any depth.
func (c *checker) containsGroup(t Type) bool {
	return c.containsGroupSeen(t, map[*TypeObj]bool{})
}

func (c *checker) containsGroupSeen(t Type, seen map[*TypeObj]bool) bool {
	switch x := resolve(t).(type) {
	case *Named:
		if isTaskGroup(x) {
			return true
		}
		for _, a := range x.Args {
			if c.containsGroupSeen(a, seen) {
				return true
			}
		}
		if seen[x.Obj] || x.Obj.Module == nil || x.Obj.Module.IsStd {
			return false
		}
		seen[x.Obj] = true
		for _, f := range x.Obj.Fields {
			if c.containsGroupSeen(f.Type, seen) {
				return true
			}
		}
		for _, v := range x.Obj.Variants {
			for _, f := range v.Fields {
				if c.containsGroupSeen(f.Type, seen) {
					return true
				}
			}
		}
	case *Tuple:
		for _, e := range x.Elems {
			if c.containsGroupSeen(e, seen) {
				return true
			}
		}
	}
	return false
}

// groupCallFrame returns the scope frame of `h.group()` when call is one.
func (c *checker) groupCallFrame(e ast.Expr) *scopeFrame {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "group" || len(call.Args) != 0 {
		return nil
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil
	}
	if v := c.identObj[id]; v != nil {
		return v.scopeHandle
	}
	return nil
}

// noteGroupBinding records that v holds the group created by `h.group()`.
func (c *checker) noteGroupBinding(e ast.Expr, v *VarObj) {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			break
		}
		e = p.X
	}
	if f := c.groupCallFrame(e); f != nil {
		v.groupFrame = f
		return
	}
	if id, ok := e.(*ast.Ident); ok {
		if o := c.identObj[id]; o != nil && o.groupFrame != nil {
			v.groupFrame = o.groupFrame
		}
	}
}

// groupFrameOf returns the frame that owns the tasks of the group recv, or
// nil when its origin is not visible here.
func (c *checker) groupFrameOf(recv ast.Expr) *scopeFrame {
	if f := c.groupCallFrame(recv); f != nil {
		return f
	}
	switch x := recv.(type) {
	case *ast.ParenExpr:
		return c.groupFrameOf(x.X)
	case *ast.Ident:
		if v := c.identObj[x]; v != nil {
			return v.groupFrame
		}
	case *ast.SelectorExpr:
		// `os.tasks` of main or of a test: the root scope of that body
		id, ok := x.X.(*ast.Ident)
		if !ok || x.Sel.Name != "tasks" {
			return nil
		}
		v := c.identObj[id]
		if v == nil || v.ctx == nil || !c.isOsValue(v) {
			return nil
		}
		if v.ctx.groups == nil {
			v.ctx.groups = &scopeFrame{ctx: v.ctx, span: x.Sp}
		}
		return v.ctx.groups
	}
	return nil
}

// isOsValue reports whether v is the `os` of a test or the `os: Os`
// parameter of `main`.
func (c *checker) isOsValue(v *VarObj) bool {
	if v.Kind == VPredeclared && v.Name == "os" {
		return true
	}
	if v.Kind != VParam || !c.isPrelude(v.Type, "Os") {
		return false
	}
	fn := v.ctx.fn
	return fn != nil && fn.Recv == nil && fn.Name == "main"
}

// recordGroupSpawn notes a task started on a group for the failure rule.
func (c *checker) recordGroupSpawn(recv ast.Expr, call *ast.CallExpr) {
	var errT Type
	if len(call.Args) == 1 {
		if ft, ok := resolve(c.exprType[call.Args[0].Value]).(*Func); ok {
			errT = ft.Err
		}
	}
	if errT == nil {
		return
	}
	site := &spawnSite{call: call, errT: errT}
	if f := c.groupFrameOf(recv); f != nil {
		f.spawns = append(f.spawns, site)
		return
	}
	if c.ctx.ext == nil {
		c.ctx.ext = &scopeFrame{ctx: c.ctx}
	}
	c.ctx.ext.spawns = append(c.ctx.ext.spawns, site)
}

// finishGroups applies the failure rule to the group tasks of a finished
// body.
func (c *checker) finishGroups(ctx *fnCtx) {
	if ctx.groups != nil {
		for _, sp := range ctx.groups.spawns {
			if sp.bound != nil && sp.bound.awaited {
				continue
			}
			saved := c.ctx
			c.ctx = ctx
			c.propagate(sp.errT, sp.call.Sp, diag.ScopeErrorConvert, "this task")
			c.ctx = saved
		}
	}
	if ctx.ext == nil {
		return
	}
	for _, sp := range ctx.ext.spawns {
		if sp.bound != nil && sp.bound.awaited {
			continue
		}
		e := resolve(sp.errT)
		if isBad(e) || c.isNever(e) || isOpenVar(e) {
			continue
		}
		d := c.errorf(diag.ScopeErrorConvert, sp.call.Sp, "a task started on a task group from outside this function can fail with `%s`; handle the error inside the task, or bind its handle and await it", deep(e))
		note(d, token.Span{}, "a failing group task fails the scope that created the group, which this function cannot see (core 7.1)")
	}
}

// groupEscapes reports XO0802 for values holding a group created by
// `h.group()` that leave the scope x.
func (c *checker) groupEscapes(x *ast.ScopeExpr, h *VarObj) {
	tainted := map[*VarObj]bool{}
	mentions := func(e ast.Expr) bool {
		found := false
		inspect(e, func(n ast.Node) bool {
			if found {
				return false
			}
			switch y := n.(type) {
			case *ast.Ident:
				if v := c.identObj[y]; v != nil && (tainted[v] || v.groupFrame == h.scopeHandle && v.groupFrame != nil) {
					found = true
				}
			case *ast.CallExpr:
				if c.groupCallFrame(y) == h.scopeHandle {
					found = true
				}
			}
			return !found
		})
		return found
	}
	// escapes reports whether e carries a group of this scope in a value
	// that can outlive it: a type holding a group, or a closure.
	escapes := func(e ast.Expr) bool {
		if e == nil || !mentions(e) {
			return false
		}
		t := c.exprType[e]
		if _, isFn := resolve(t).(*Func); isFn {
			return true
		}
		return c.containsGroup(t)
	}
	outside := func(v *VarObj) bool {
		return v != nil && v.Span.File == x.Sp.File && v.Span.Start.Offset < x.Sp.Start.Offset
	}
	report := func(e ast.Expr, how string) {
		d := c.errorf(diag.ScopeEscape, e.Span(), "a value holding the task group of `scope %s` %s, so it could start tasks after the scope ends (core 7.1)", h.Name, how)
		note(d, h.Span, "the group belongs to the scope bound here; store `os.tasks` instead for program lifetime work")
	}
	inspect(x.Body, func(n ast.Node) bool {
		switch y := n.(type) {
		case *ast.FnLit:
			return false // its returns and assignments are its own
		case *ast.LetStmt:
			if bp, ok := y.Pattern.(*ast.BindPat); ok && mentions(y.Value) {
				if v := c.identObj[bp.Name]; v != nil {
					tainted[v] = true
				}
			}
		case *ast.VarStmt:
			if mentions(y.Value) {
				if v := c.identObj[y.Name]; v != nil {
					tainted[v] = true
				}
			}
		case *ast.AssignStmt:
			if id, ok := c.placeRoot(y.Target); ok {
				v := c.identObj[id]
				if outside(v) && escapes(y.Value) {
					report(y.Value, "is stored in `"+id.Name+"`, which outlives the scope")
				} else if v != nil && mentions(y.Value) {
					tainted[v] = true
				}
			}
		case *ast.ReturnStmt:
			if escapes(y.Value) {
				report(y.Value, "is returned from the function")
			}
		case *ast.CallExpr:
			ci := c.callInfo[y]
			if sel, ok := y.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "send" {
				for _, a := range y.Args {
					if escapes(a.Value) {
						report(a.Value, "is sent on a channel")
					}
				}
			} else if ci != nil && ci.fn != nil && ci.fn.RecvVar && ci.recvExpr != nil {
				if id, ok := c.placeRoot(ci.recvExpr); ok && outside(c.identObj[id]) {
					for _, a := range y.Args {
						if escapes(a.Value) {
							report(a.Value, "is stored in `"+id.Name+"`, which outlives the scope")
						}
					}
				}
			}
		}
		return true
	})
	// the scope's value
	if n := len(x.Body.Stmts); n > 0 {
		if es, ok := x.Body.Stmts[n-1].(*ast.ExprStmt); ok && escapes(es.X) {
			report(es.X, "is the value of the scope")
		}
	}
}
