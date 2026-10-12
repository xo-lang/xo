package gen

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Resource budgets (decision 0173, std/rt/budget.go): generated code
// polls the task's budget with `tk.Tick()` at the start of every loop
// iteration (emitLoop) and at the entry of every function or closure
// that may recurse. Unbounded work needs a loop or recursion, and every
// function on a cycle of calls calls something, so a body that calls no
// user code (only std functions that take no function) needs no poll:
// leaf functions keep their size, which matters for Go's inliner.

// mayRecurse reports whether body calls user code: a function or method
// of a user module, a function value, or an interface method.
func (f *fnGen) mayRecurse(body ast.Node) bool {
	found := false
	inspectNode(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ci := f.info.CallOf(call)
		switch {
		case ci == nil:
			// a variant constructor or a conversion, unless the callee
			// is a function value
			if t := f.info.TypeOf(call.Fun); t != nil {
				_, found = check.Resolve(t).(*check.Func)
			}
		case ci.Fn == nil || ci.Fn.Module == nil || !ci.Fn.Module.IsStd || ci.Fn.Iface:
			found = true
		}
		return !found
	})
	return found
}
