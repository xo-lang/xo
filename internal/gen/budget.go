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

// usesBudgets reports whether the program calls task.with_budget
// anywhere; only then does generated code poll (the poll measured 45% on
// a call-bound recursion and 18% on a list sieve without a budget,
// bench/results/budget-poll-2026-10-11.md). Cached per generation.
func (g *gen) usesBudgets() bool {
	if g.budgets != 0 {
		return g.budgets > 0
	}
	g.budgets = -1
	for _, m := range g.info.AllModules() {
		if m.IsStd {
			continue
		}
		for _, file := range m.Files {
			if file.AST == nil {
				continue
			}
			check.Inspect(file.AST, func(n ast.Node) bool {
				if g.budgets > 0 {
					return false
				}
				if call, ok := n.(*ast.CallExpr); ok {
					if ci := g.info.CallOf(call); ci != nil && ci.Fn != nil && ci.Fn.Module != nil && ci.Fn.Module.Path == "std/task" && ci.Fn.Name == "with_budget" {
						g.budgets = 1
					}
				}
				return g.budgets <= 0
			})
		}
	}
	return g.budgets > 0
}

// tickLoop reports whether loops poll the budget.
func (f *fnGen) tickLoop() bool { return f.g.usesBudgets() }

// mayRecurse reports whether body calls user code: a function or method
// of a user module, a function value, or an interface method; false when
// the program uses no budget.
func (f *fnGen) mayRecurse(body ast.Node) bool {
	if !f.g.usesBudgets() {
		return false
	}
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
