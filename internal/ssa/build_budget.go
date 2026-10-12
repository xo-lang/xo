package ssa

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Resource budgets (std.md 7, decisions 0173 and 0175): in a program that
// calls task.with_budget, the code of the program's own modules polls the
// budget (`rt.tick`, then an unwinding check) at the start of every loop
// iteration (loopBound) and at the entry of every function, closure, and
// test body; std code does not poll. The runtime (std/llrt/xo_rt.c,
// "Resource budgets") counts the polls of each context and every 1024 of
// them charges its steps, running time, and allocation to its budgets;
// a poll unwinds the context when a ceiling of its budgets was passed.
// with_budget runs its closure as the body of a scope that carries the
// budget, as with_timeout does with its deadline.

// usesBudgets reports whether the program calls task.with_budget in one
// of its own modules (internal/gen's usesBudgets): only then does its
// code poll.
func (b *builder) usesBudgets() bool {
	found := false
	for _, m := range b.info.AllModules() {
		if m.IsStd || m.Native {
			continue
		}
		for _, f := range m.Files {
			if f.AST == nil {
				continue
			}
			check.Inspect(f.AST, func(n ast.Node) bool {
				if found {
					return false
				}
				if call, ok := n.(*ast.CallExpr); ok {
					if ci := b.info.CallOf(call); ci != nil && isWithBudget(ci.Fn) {
						found = true
					}
				}
				return !found
			})
		}
	}
	return found
}

// isWithBudget reports whether fo is std/task's with_budget.
func isWithBudget(fo *check.FuncObj) bool {
	return fo != nil && fo.Recv == nil && fo.Module != nil && fo.Module.Path == "std/task" && fo.Name == "with_budget"
}

// poll emits the budget poll when the program uses budgets and the code
// being built is the program's own.
func (s *fn) poll() {
	if !s.polls || !s.b.budgets || s.isDead() {
		return
	}
	s.rt("tick", nil)
	s.check("")
}

// withBudget lowers `task.with_budget(clock, b, f)` (std.md 7): f runs
// as the body of a scope with the budget b (rt.budget_open, which faults
// on a negative ceiling); rt.budget_end, once the scope closed, tells the
// resource whose ceiling was passed (the result is then
// Err(Exceeded(resource)) and an unwinding stops), else f's own outcome
// stands: its value Ok, its error e Err(Failed(e)), its unwinding goes on.
func (s *fn) withBudget(x *ast.CallExpr, fo *check.FuncObj) *Value {
	b := s.b
	m := b.mod
	nt := s.natOf(x)
	intT, boolT, strT := m.Builtin(KInt), m.Builtin(KBool), m.Builtin(KStr)
	es := argExprs(fo.Params, x)
	if es[0] == nil || es[1] == nil || es[2] == nil {
		b.unsupported(x, "a call with a missing argument")
		return s.undef(nt)
	}
	s.expr(es[0])
	bv := s.expr(es[1])
	var lim [3]*Value
	for i, name := range []string{"cpu", "alloc", "steps"} {
		k, ft := s.fieldIndex(bv.Type, name)
		if k < 0 {
			b.unsupported(x, "this budget")
			return s.undef(nt)
		}
		lim[i] = s.add(OpField, ft, k, bv)
	}
	ct, ok := check.Resolve(s.natCheck(es[2])).(*check.Func)
	if !ok {
		b.unsupported(x, "this closure")
		return s.undef(nt)
	}
	errT := failErr(s.apply(ct.Err))
	rt := b.typ(b.resultOf(s.apply(ct.Result), errT), x)
	fv := s.valueAs(es[2], m.FuncType(nil, rt))
	h := s.rt("scope_open", m.Handle("Scope"))
	s.rt("budget_open", nil, h, lim[0], lim[1], lim[2], s.loc(x))
	if s.pending != nil {
		s.flushLanding()
	}
	r := s.cur.Add(OpCallValue, rt, nil, fv)
	u := s.cur.Add(OpCall, boolT, "rt.unwinding")
	if b.faultUnwind {
		// a fault in f closes the scope and goes on (decision 0085)
		r.Pad = u
	}
	join := s.newBlock()
	res := join.AddParam(nt)
	ei := nt.VariantIndex("Err")
	budT := nt.Variants[ei].Fields[0].Type
	// exceeded is Err(Exceeded(the name of resource k)), k 1 cpu, 2
	// alloc, 3 steps
	exceeded := func(k *Value) {
		isCPU, other := s.branch(s.add(OpEq, boolT, nil, k, s.cint(1)))
		s.cur = isCPU
		s.jump(join, s.add(OpVariant, nt, ei, s.variant(budT, "Exceeded", []*Value{s.add(OpConst, strT, "cpu")}, x)))
		s.cur = other
		isAlloc, steps := s.branch(s.add(OpEq, boolT, nil, k, s.cint(2)))
		s.cur = isAlloc
		s.jump(join, s.add(OpVariant, nt, ei, s.variant(budT, "Exceeded", []*Value{s.add(OpConst, strT, "alloc")}, x)))
		s.cur = steps
		s.jump(join, s.add(OpVariant, nt, ei, s.variant(budT, "Exceeded", []*Value{s.add(OpConst, strT, "steps")}, x)))
	}
	unw, done := s.branch(u)
	s.b.checks = append(s.b.checks, unwindCheck{s.f, u, "", r})
	// unwinding: a ceiling passed, or a cancellation from outside
	s.cur = unw
	s.rt("scope_close", intT, h, s.cint(2))
	k := s.rt("budget_end", intT, h)
	over, out := s.branch(s.add(OpGt, boolT, nil, k, s.cint(0)))
	s.cur = over
	exceeded(k)
	s.cur = out
	s.unwind()
	// f returned
	s.cur = done
	s.rt("scope_close", intT, h, s.cint(0))
	k = s.rt("budget_end", intT, h)
	s.check("")
	late, intime := s.branch(s.add(OpGt, boolT, nil, k, s.cint(0)))
	s.cur = late
	exceeded(k)
	s.cur = intime
	okI := nt.VariantIndex("Ok")
	if errT == nil {
		s.jump(join, s.add(OpVariant, nt, okI, r))
	} else {
		fok, ferr := s.branch(s.add(OpEq, boolT, nil, s.add(OpTag, intT, nil, r), s.cint(int64(rt.VariantIndex("Ok")))))
		s.cur = fok
		v := s.add(OpPayload, rt.Variants[rt.VariantIndex("Ok")].Fields[0].Type, PayloadRef{rt.VariantIndex("Ok"), 0}, r)
		s.jump(join, s.add(OpVariant, nt, okI, v))
		s.cur = ferr
		rei := rt.VariantIndex("Err")
		e := s.add(OpPayload, rt.Variants[rei].Fields[0].Type, PayloadRef{rei, 0}, r)
		s.jump(join, s.add(OpVariant, nt, ei, s.variant(budT, "Failed", []*Value{e}, x)))
	}
	s.seal(join)
	s.cur = join
	return res
}
