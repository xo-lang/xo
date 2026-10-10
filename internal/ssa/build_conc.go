package ssa

import (
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Structured concurrency (core 7, decision 0068): scopes, tasks,
// channels, Mutex, and Atomic, lowered to calls of the task runtime of
// std/llrt (`rt.*` calls, see RuntimeCalls).
//
//   - Handles. Task, Scope, task.Group, Chan, Mutex, and Atomic values are
//     KHandle: one pointer to a shared, reference counted runtime object.
//   - Values cross into the runtime as lists of one element (a List is
//     one pointer whatever its element), marked shared first (OpShare,
//     decision 0058): a spawned closure (its captures), a task's result,
//     a value sent on a channel, the value of a Mutex. The runtime
//     releases lists it still holds through an entry function of the
//     element type (droplist).
//   - A spawned closure runs in an entry function (Func.Entry) that the
//     runtime calls on a worker thread: it calls the closure, then
//     records the result (or that the task unwound).
//   - Unwinding. A cancellation point (send, recv, await, waiting for a
//     lock) in a canceled task sets the runtime's unwinding flag and
//     returns a placeholder. After each such call, each call of a
//     function that may unwind, and each call of a function value, the
//     builder emits `rt.unwinding` and a branch to the unwind path: close
//     the scopes being left (mode 2) and return zero values. A scope
//     whose child failed turns the unwinding into the child's error,
//     returned as by `?`. After building, checks after calls of functions
//     that cannot unwind are removed (pruneUnwind).
//   - Scope exits. Every way out of a scope body closes it: the end of
//     the body (mode 0), `return`, `?`, `break`, `continue` (mode 1), and
//     unwinding (mode 2). A scope with a spawn that can fail checks the
//     close's result and returns the failed task's error.
//
// A program that uses none of this is built without unwinding checks:
// Build tries without them first and builds again with them when it
// meets a concurrency construct.

// scopeCtx is a `scope` whose body is being built.
type scopeCtx struct {
	h       *Value // the Scope handle
	sites   []*spawnSite
	unwindB *Block // closes the scope while unwinding (lazy)
	failB   *Block // returns a failed task's error (lazy)
}

// spawnSite is a spawn whose task can fail with an error that is not
// awaited: its scope may have to return that error.
type spawnSite struct {
	id   int
	errT check.Type // the task's error type
	resT check.Type // the task's success type
}

// unwindCheck is a check after a call (b.checks): callee is the user
// function called, or "" for a call that may always unwind.
type unwindCheck struct {
	f      *Func
	u      *Value // the rt.unwinding (or rt.landing) call
	callee string
	w      *Value // the call checked (its Pad is u), or nil
}

// concType lowers the prelude and std/task types of core 7 (nil when obj
// is not one).
func (b *builder) concType(obj *check.TypeObj, r *check.Named, at ast.Node) *Type {
	if obj.Module == nil {
		return nil
	}
	name := ""
	switch {
	case obj.Module.IsPrelude:
		switch obj.Name {
		case "Chan", "Mutex", "Atomic", "Scope", "Task", "Listener", "Conn":
			name = obj.Name
		}
	case obj.Module.Path == "std/task" && obj.Name == "Group":
		name = "task.Group"
	}
	if name == "" {
		return nil
	}
	b.needConc = true
	var es []*Type
	var ns []string
	for _, a := range r.Args {
		t := b.typ(a, at)
		es = append(es, t)
		ns = append(ns, t.Name)
	}
	if len(ns) > 0 {
		name += "[" + strings.Join(ns, ", ") + "]"
	}
	return b.mod.Handle(name, es...)
}

// taskType is the handle type Task[T, E] of a task whose closure returns
// res and fails with errT (nil: cannot fail).
func (b *builder) taskType(res, errT check.Type, at ast.Node) *Type {
	if res == nil {
		res = &check.Tuple{}
	}
	if errT == nil {
		errT = b.preludeType("Never")
	}
	return b.typ(b.preludeType("Task", res, errT), at)
}

func (s *fn) rt(name string, t *Type, args ...*Value) *Value {
	if t == nil {
		t = s.b.mod.Builtin(KUnit)
	}
	return s.add(OpCall, t, "rt."+name, args...)
}

// loc is the `file:line` of n as a Str constant (for faults the runtime
// raises, located like the Go backend's).
func (s *fn) loc(n ast.Node) *Value {
	return s.add(OpConst, s.b.mod.Builtin(KStr), s.b.posLine(n))
}

// one is a list of one element v, marked shared: how a value crosses
// into the runtime.
func (s *fn) one(v *Value) *Value {
	lt := s.b.mod.List(v.Type)
	l := s.call("list.push", lt, "", s.call("list.new", lt, ""), v)
	s.add(OpShare, nil, nil, l)
	return l
}

// ---------------------------------------------------------------------------
// Unwinding

// unwindTarget is where a check jumps when the current code unwinds: the
// innermost scope's unwind block, or the function's.
func (s *fn) unwindTarget() *Block {
	if n := len(s.scopes); n > 0 {
		sc := s.scopes[n-1]
		if sc.unwindB == nil {
			sc.unwindB = s.newBlock()
		}
		return sc.unwindB
	}
	if s.unwindB == nil {
		s.unwindB = s.newBlock()
	}
	return s.unwindB
}

// check emits the unwinding check after a call (callee: see
// unwindCheck). Without unwinding checks (no concurrency) a build whose
// faults unwind gets a check that only a fault takes (`rt.landing`,
// landing.go). The call is the check's landing site (Value.Pad).
func (s *fn) check(callee string) {
	s.checkThen(callee, nil)
}

// checkThen is check with cleanup code run on the unwind path first
// (unlocking a mutex).
func (s *fn) checkThen(callee string, cleanup func()) {
	if s.isDead() || s.shield > 0 || (!s.b.conc && !s.b.faultUnwind) {
		return
	}
	w := s.lastInstr()
	if s.pending == w {
		s.pending = nil
	}
	name := "rt.unwinding"
	if !s.b.conc {
		name = "rt.landing"
	}
	u := s.cur.Add(OpCall, s.b.mod.Builtin(KBool), name)
	if w != nil && (w.Op == OpCall || w.Op == OpCallValue) && s.b.faultUnwind {
		w.Pad = u
	}
	yes, no := s.branch(u)
	s.f.markRest(no)
	s.cur = yes
	if cleanup != nil {
		cleanup()
	}
	s.unwind()
	s.cur = no
	if cleanup != nil {
		callee = "" // never removed
	}
	s.b.checks = append(s.b.checks, unwindCheck{s.f, u, callee, w})
}

// lastInstr is the last instruction of the current block, or nil.
func (s *fn) lastInstr() *Value {
	if n := len(s.cur.Instrs); n > 0 {
		return s.cur.Instrs[n-1]
	}
	return nil
}

// finishFn emits the function's unwind block: zero results.
func (s *fn) finishFn() {
	if s.unwindB == nil {
		return
	}
	saved := s.cur
	s.seal(s.unwindB)
	s.cur = s.unwindB
	s.f.UnwindExit = s.unwindB
	if s.b.faultUnwind && !s.f.Catches {
		// a fault goes on to the caller (decision 0085)
		s.cur.Add(OpCall, s.b.mod.Builtin(KUnit), "rt.fault_continue")
	}
	var rs []*Value
	for _, t := range s.f.Results {
		rs = append(rs, s.zero(t, map[*Type]bool{}))
	}
	if s.packOrig != nil {
		// a packed closure hands back the value it was given (the mutex
		// keeps it, packedValue)
		pt := s.f.Results[0]
		rs = []*Value{s.add(OpTuple, pt, nil, s.zero(pt.Elems[0], map[*Type]bool{}), s.packOrig)}
	}
	s.add(OpReturn, nil, nil, rs...)
	s.cur = saved
}

// zero is a value of t that owns nothing a caller could not release:
// what an unwinding function returns.
func (s *fn) zero(t *Type, seen map[*Type]bool) *Value {
	m := s.b.mod
	switch t.Kind {
	case KUnit:
		return s.unit()
	case KInt:
		return s.add(OpConst, t, int64(0)) // t: Int, a sized integer, or Rune
	case KFloat:
		return s.add(OpConst, t, 0.0)
	case KBool:
		return s.bool(false)
	case KStr:
		return s.add(OpConst, t, "")
	case KCap:
		return s.add(OpCap, t, "zero")
	case KList:
		return s.rt("rl_empty", t)
	case KHandle:
		return s.rt("h_zero", t)
	case KBox:
		return s.add(OpBox, t, nil, s.zero(t.Elems[0], seen))
	case KFunc:
		return s.add(OpClosure, t, s.b.zeroFunc(t))
	case KStruct, KTuple:
		var vs []*Value
		for i := 0; i < numFields(t); i++ {
			vs = append(vs, s.zero(fieldType(t, i), seen))
		}
		if t.Kind == KStruct {
			return s.add(OpStruct, t, nil, vs...)
		}
		return s.add(OpTuple, t, nil, vs...)
	case KEnum:
		seen[t] = true
		defer delete(seen, t)
		best := -1
		for i, v := range t.Variants {
			ok := true
			for _, f := range v.Fields {
				ok = ok && zeroable(f.Type, seen)
			}
			if ok && (best < 0 || len(v.Fields) < len(t.Variants[best].Fields)) {
				best = i
			}
		}
		if best < 0 {
			best = 0
		}
		var vs []*Value
		for _, f := range t.Variants[best].Fields {
			vs = append(vs, s.zero(f.Type, seen))
		}
		return s.add(OpVariant, t, best, vs...)
	}
	_ = m
	return s.unit()
}

// zeroable reports whether zero can make a t without going through an
// enum in seen (a recursive type; lists and function values end it).
func zeroable(t *Type, seen map[*Type]bool) bool {
	switch t.Kind {
	case KEnum:
		if seen[t] {
			return false
		}
		seen[t] = true
		defer delete(seen, t)
		for _, v := range t.Variants {
			ok := true
			for _, f := range v.Fields {
				ok = ok && zeroable(f.Type, seen)
			}
			if ok {
				return true
			}
		}
		return false
	case KStruct, KTuple:
		for i := 0; i < numFields(t); i++ {
			if !zeroable(fieldType(t, i), seen) {
				return false
			}
		}
	case KBox:
		return zeroable(t.Elems[0], seen)
	}
	return true
}

// zeroFunc returns the function of the zero value of function type t: it
// returns zero.
func (b *builder) zeroFunc(t *Type) string {
	name := "zero." + t.Name
	if f := b.mod.Func(name); f != nil {
		return name
	}
	f := b.mod.NewFunc(name, append([]*Type{t}, t.Elems...), []*Type{t.Ret})
	f.Indirect = true
	c := b.newFn(f, nil, nil)
	c.add(OpReturn, nil, nil, c.zero(t.Ret, map[*Type]bool{}))
	return name
}

// pruneUnwind removes the checks after calls of functions that cannot
// unwind: a function may unwind when it has a check after a
// cancellation point or a function value call, or after a call of a
// function that may unwind.
//
// With fault unwinding (decision 0085) a check after a call of a function
// that cannot unwind but can fault becomes a check only a fault takes
// (`rt.landing`); without concurrency every check is one.
func (b *builder) pruneUnwind() {
	may := map[*Func]bool{}
	if b.conc {
		for _, c := range b.checks {
			if c.callee == "" {
				may[c.f] = true
			}
		}
		for changed := true; changed; {
			changed = false
			for _, c := range b.checks {
				if c.callee != "" && !may[c.f] && may[b.mod.Func(c.callee)] {
					may[c.f] = true
					changed = true
				}
			}
		}
	}
	var faults map[*Func]bool
	if b.faultUnwind {
		faults = b.mod.mayFault()
	}
	for _, c := range b.checks {
		if b.conc && (c.callee == "" || may[b.mod.Func(c.callee)]) {
			continue
		}
		if b.faultUnwind && (c.callee == "" || faults[b.mod.Func(c.callee)]) {
			c.u.Aux = "rt.landing"
			continue
		}
		if c.w != nil && c.w.Pad == c.u {
			c.w.Pad = nil
		}
		bl := c.u.Block
		br := bl.Term()
		yes, no := br.Succs[0], br.Succs[1]
		var preds []*Edge
		for _, e := range yes.To.Preds {
			if e != yes {
				preds = append(preds, e)
			}
		}
		yes.To.Preds = preds
		br.Op, br.Args, br.Succs = OpJump, nil, []*Edge{no}
		var ins []*Value
		for _, v := range bl.Instrs {
			if v != c.u {
				ins = append(ins, v)
			}
		}
		bl.Instrs = ins
	}
}

// ---------------------------------------------------------------------------
// Scopes

// closeEarly closes scope sc for an early exit (mode 1: `return`, `?`,
// `break`, `continue`): a failed child sends the exit to the scope's
// failure block.
func (s *fn) closeEarly(sc *scopeCtx) {
	// the scope is closed from here on: a fault raised by its close
	// unwinds the scopes around it (s.scopes is what unwindTarget sees)
	saved := s.scopes
	for i, x := range s.scopes {
		if x == sc {
			s.scopes = s.scopes[:i:i]
			break
		}
	}
	st := s.rt("scope_close", s.b.mod.Builtin(KInt), sc.h, s.cint(1))
	s.closeStatus(sc, st)
	s.scopes = saved
}

// closeStatus acts on the status st of closing sc: 1 (a child failed
// with an error) goes to the scope's failure block; 3 (a child faulted,
// decision 0085) raises the child's fault here.
func (s *fn) closeStatus(sc *scopeCtx, st *Value) {
	if len(sc.sites) > 0 {
		s.failIf(sc, st)
	}
	if !s.b.faultUnwind {
		return
	}
	c := s.add(OpEq, s.b.mod.Builtin(KBool), nil, st, s.cint(3))
	yes, no := s.branch(c)
	s.cur = yes
	s.rt("fault_rethrow", nil, sc.h)
	s.add(OpUnreachable, nil, nil)
	s.cur = no
}

// failIf goes to sc's failure block when the close status st is 1.
func (s *fn) failIf(sc *scopeCtx, st *Value) {
	c := s.add(OpEq, s.b.mod.Builtin(KBool), nil, st, s.cint(1))
	yes, no := s.branch(c)
	s.cur = yes
	if sc.failB == nil {
		sc.failB = s.newBlock()
	}
	s.jump(sc.failB)
	s.cur = no
}

// scopeExpr lowers `scope s { body }` (core 7.1).
func (s *fn) scopeExpr(x *ast.ScopeExpr) *Value {
	b := s.b
	b.needConc = true
	t := s.natOf(x)
	if !b.conc {
		return s.undef(t)
	}
	sh := b.mod.Handle("Scope")
	h := s.rt("scope_open", sh)
	if vo := b.info.VarOf(x.Name); vo != nil {
		s.writeVar(vo, h)
	}
	sc := &scopeCtx{h: h, sites: b.failingSpawns(s, x.Body)}
	s.scopes = append(s.scopes, sc)
	s.exits = append(s.exits, exitEntry{scope: sc})
	v := s.block(x.Body)
	s.exits = s.exits[:len(s.exits)-1]
	s.scopes = s.scopes[:len(s.scopes)-1]
	if !s.isDead() {
		st := s.rt("scope_close", b.mod.Builtin(KInt), h, s.cint(0))
		s.closeStatus(sc, st)
	}
	s.finishScope(sc, x)
	return v
}

// finishScope emits sc's unwind and failure blocks (after its body:
// every edge into them is known, and the scope is no longer open).
func (s *fn) finishScope(sc *scopeCtx, x *ast.ScopeExpr) {
	saved := s.cur
	if sc.unwindB != nil {
		s.seal(sc.unwindB)
		s.cur = sc.unwindB
		st := s.rt("scope_close", s.b.mod.Builtin(KInt), sc.h, s.cint(2))
		s.closeStatus(sc, st)
		s.unwind()
	}
	if sc.failB != nil {
		s.seal(sc.failB)
		s.cur = sc.failB
		s.scopeFailure(sc, x)
	}
	s.cur = saved
}

// scopeFailure returns the error of the scope's failed task, converted
// to the function's error type as by `?` (core 7.1).
func (s *fn) scopeFailure(sc *scopeCtx, x *ast.ScopeExpr) {
	b := s.b
	m := b.mod
	if s.errT == nil {
		b.unsupported(x, "a failing task in a scope of a function that cannot fail")
		s.add(OpUnreachable, nil, nil)
		s.dead()
		return
	}
	site := s.rt("scope_fail_site", m.Builtin(KInt), sc.h)
	for _, sp := range sc.sites {
		c := s.add(OpEq, m.Builtin(KBool), nil, site, s.cint(int64(sp.id)))
		yes, no := s.branch(c)
		s.cur = yes
		tt := b.taskType(sp.resT, sp.errT, x)
		rt := b.typ(b.resultOf(sp.resT, sp.errT), x)
		th := s.rt("scope_fail_task", tt, sc.h)
		l := s.rt("task_result", m.List(rt), th)
		r := s.call("list.index", rt, "", l, s.cint(0))
		et := rt.Variants[rt.VariantIndex("Err")].Fields[0].Type
		e := s.add(OpPayload, et, PayloadRef{rt.VariantIndex("Err"), 0}, r)
		want := s.resultT()
		s.ret(s.add(OpVariant, want, want.VariantIndex("Err"), s.coerceErr(e, sp.errT, s.errT, x)), x)
		s.cur = no
	}
	s.add(OpUnreachable, nil, nil)
	s.dead()
}

// failingSpawns lists the spawns in body whose task can fail with an
// error that is not awaited (core 7.1: a failing child fails the scope).
func (b *builder) failingSpawns(s *fn, body *ast.Block) []*spawnSite {
	var out []*spawnSite
	s.noteBindings(body)
	check.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		ci := b.info.CallOf(call)
		if ci == nil || !isSpawn(ci.Fn) || b.consumed(call) {
			return true
		}
		ft, ok := check.Resolve(s.natCheck(call.Args[0].Value)).(*check.Func)
		if !ok || failErr(ft.Err) == nil {
			return true
		}
		out = append(out, &spawnSite{id: b.siteID(call), errT: s.apply(ft.Err), resT: s.apply(ft.Result)})
		return true
	})
	return out
}

func isSpawn(fo *check.FuncObj) bool {
	if fo == nil || fo.Recv == nil || fo.Module == nil {
		return false
	}
	switch {
	case fo.Module.IsPrelude && fo.Recv.Name == "Scope":
		return fo.Name == "spawn" || fo.Name == "spawn_background"
	case fo.Module.Path == "std/task" && fo.Recv.Name == "Group":
		return fo.Name == "spawn" || fo.Name == "spawn_background"
	}
	return false
}

func (b *builder) siteID(call *ast.CallExpr) int {
	if id, ok := b.sites[call]; ok {
		return id
	}
	id := len(b.sites)
	b.sites[call] = id
	return id
}

// consumed reports whether the task a spawn starts has its handle bound
// with `let` and awaited, so its error does not fail the scope.
func (b *builder) consumed(call *ast.CallExpr) bool {
	vo := b.boundTo[call]
	return vo != nil && vo.Awaited()
}

// ---------------------------------------------------------------------------
// Tasks

// spawn lowers `s.spawn(f)`, `s.spawn_background(f)`, `g.spawn(f)`,
// `g.spawn_background(f)`: the closure crosses to the task (marked
// shared) in a list of one, run by the entry function of its type.
func (s *fn) spawn(x *ast.CallExpr, h *Value, background bool, ret bool) *Value {
	b := s.b
	m := b.mod
	arg := x.Args[0].Value
	ct, ok := check.Resolve(s.natCheck(arg)).(*check.Func)
	if !ok {
		b.unsupported(x, "this spawn")
		return s.undef(s.natOf(x))
	}
	errT := failErr(s.apply(ct.Err))
	res := s.apply(ct.Result)
	rt := b.typ(b.resultOf(res, errT), x)
	ft := m.FuncType(nil, rt)
	tt := b.taskType(res, errT, x)
	f := s.valueAs(arg, ft)
	fl := s.one(f)
	flags := int64(0)
	if background {
		flags |= 1
	}
	if b.consumed(x) {
		flags |= 2
	}
	site := int64(-1)
	if id, ok := b.sites[x]; ok {
		site = int64(id)
	}
	intT := m.Builtin(KInt)
	run := s.add(OpFuncAddr, intT, b.runner(ft, tt, errT != nil))
	drop := s.add(OpFuncAddr, intT, b.dropList(rt))
	fdrop := s.add(OpFuncAddr, intT, b.dropList(ft))
	t := s.rt("task_spawn", tt, h, fl, run, drop, fdrop, s.cint(flags), s.cint(site), s.loc(x))
	if !ret {
		return s.unit()
	}
	return s.coerce(t, s.natOf(x), x)
}

// runner returns the entry function that runs a task whose closure has
// type ft (fails: its Result is a failure or success, not a value): it calls the closure and records its result (shared), or that
// the task unwound.
func (b *builder) runner(ft, tt *Type, fails bool) string {
	m := b.mod
	name := "xo.run[" + tt.Name + "]"
	if m.Func(name) != nil {
		return name
	}
	unit, boolT, intT := m.Builtin(KUnit), m.Builtin(KBool), m.Builtin(KInt)
	rt := ft.Ret
	f := m.NewFunc(name, []*Type{m.List(ft), tt}, []*Type{unit})
	f.Entry = true
	e := f.Blocks[0]
	fl, t := f.Params()[0], f.Params()[1]
	fv := e.Add(OpCall, ft, "list.index", fl, e.Add(OpConst, intT, int64(0)))
	r := e.Add(OpCallValue, rt, nil, fv)
	u := e.Add(OpCall, boolT, "rt.unwinding")
	if b.faultUnwind {
		// a fault in the task lands here too: rt.task_unwound records it
		// for the scope (decision 0085)
		r.Pad = u
	}
	unw, done := f.NewBlock(), f.NewBlock()
	e.Branch(u, unw, done)
	unw.Add(OpCall, unit, "rt.task_unwound", t)
	unw.Add(OpReturn, nil, nil, unw.Add(OpTuple, unit, nil))
	var failed *Value
	if i := rt.VariantIndex("Err"); fails && rt.Kind == KEnum && i >= 0 && rt.VariantIndex("Ok") >= 0 {
		failed = done.Add(OpEq, boolT, nil, done.Add(OpTag, intT, nil, r), done.Add(OpConst, intT, int64(i)))
	} else {
		failed = done.Add(OpConst, boolT, false)
	}
	lt := m.List(rt)
	l := done.Add(OpCall, lt, "list.push", done.Add(OpCall, lt, "list.new"), r)
	done.Add(OpShare, nil, nil, l)
	done.Add(OpCall, unit, "rt.task_done", t, l, failed)
	done.Add(OpReturn, nil, nil, done.Add(OpTuple, unit, nil))
	return name
}

// dropList returns the entry function that releases a list of t the
// runtime holds.
func (b *builder) dropList(t *Type) string {
	m := b.mod
	name := "xo.droplist[" + t.Name + "]"
	if m.Func(name) != nil {
		return name
	}
	unit := m.Builtin(KUnit)
	f := m.NewFunc(name, []*Type{m.List(t)}, []*Type{unit})
	f.Entry = true
	e := f.Blocks[0]
	e.Add(OpReturn, nil, nil, e.Add(OpTuple, unit, nil))
	return name
}

// await lowers `t.await`: the task's result (a cancellation point).
func (s *fn) await(x *ast.SelectorExpr) *Value {
	b := s.b
	b.needConc = true
	t := s.natOf(x)
	if !b.conc {
		return s.undef(t)
	}
	h := s.expr(x.X)
	l := s.rt("task_await", b.mod.List(t), h, s.loc(x))
	s.check("")
	return s.call("list.index", t, "", l, s.cint(0))
}

// ---------------------------------------------------------------------------
// Calls of the concurrency types

// concCall lowers a method of Scope, Task, Chan, Mutex, Atomic, or
// task.Group, or a std/task function; ok is false for anything else.
func (s *fn) concCall(x *ast.CallExpr, ci *check.Call) (*Value, bool) {
	b := s.b
	fo := ci.Fn
	if fo.Module == nil {
		return nil, false
	}
	m := b.mod
	nt := s.natOf(x)
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	recv := ""
	switch {
	case fo.Recv == nil && fo.Module.Path == "std/task":
		recv = "task"
	case fo.Recv == nil:
		return nil, false
	case fo.Module.IsPrelude:
		switch fo.Recv.Name {
		case "Scope", "Task", "Chan", "Mutex", "Atomic":
			recv = fo.Recv.Name
		}
	case fo.Module.Path == "std/task" && fo.Recv.Name == "Group":
		recv = "Group"
	}
	if recv == "" {
		return nil, false
	}
	if recv == "task" && fo.Name == "isolate" && b.faultUnwind {
		return s.isolate(x), true
	}
	b.needConc = true
	if !b.conc {
		return s.undef(nt), true
	}
	arg := func(i int, t *Type) *Value {
		es := argExprs(fo.Params, x)
		if es[i] == nil {
			if fo.Params[i].Default != nil {
				return s.valueAs(fo.Params[i].Default, t)
			}
			b.unsupported(x, "a call with a missing argument")
			return s.undef(t)
		}
		return s.valueAs(es[i], t)
	}
	key := recv + "." + fo.Name
	switch key {
	case "task.is_canceled":
		return s.rt("is_canceled", boolT), true
	case "task.with_timeout":
		return s.withTimeout(x, fo), true
	case "task.stop_requested":
		return s.rt("stop_requested", boolT), true
	case "task.stopping":
		return s.rt("task_stopping", nt, s.add(OpFuncAddr, intT, b.dropList(nt.Elems[0]))), true
	case "Scope.spawn", "Scope.spawn_background", "Group.spawn", "Group.spawn_background":
		h := s.expr(ci.Recv)
		bg := strings.HasSuffix(key, "background")
		return s.spawn(x, h, bg, key != "Group.spawn_background"), true
	case "Scope.cancel":
		return s.rt("scope_cancel", nil, s.expr(ci.Recv)), true
	case "Scope.group":
		return s.rt("scope_group", nt, s.expr(ci.Recv)), true
	case "Chan.new":
		et := nt.Elems[0]
		drop := s.add(OpFuncAddr, intT, b.dropList(et))
		return s.rt("chan_new", nt, arg(0, intT), drop, s.loc(x)), true
	case "Mutex.new", "Atomic.new":
		et := nt.Elems[0]
		v := arg(0, et)
		if recv == "Atomic" {
			return s.rt("atomic_new", nt, v), true
		}
		drop := s.add(OpFuncAddr, intT, b.dropList(et))
		return s.rt("mutex_new", nt, s.one(v), drop), true
	}
	if ci.Recv == nil {
		b.unsupported(x, "the std function `"+fo.Module.Name+"."+fo.DisplayName()+"`")
		return s.undef(nt), true
	}
	h := s.expr(ci.Recv)
	if h.Type.Kind != KHandle {
		b.unsupported(x, "the method `"+fo.DisplayName()+"` here")
		return s.undef(nt), true
	}
	var et *Type
	if len(h.Type.Elems) > 0 {
		et = h.Type.Elems[0]
	}
	switch key {
	case "Chan.send":
		l := s.one(arg(0, et))
		s.rt("chan_send", nil, h, l, s.loc(x))
		s.check("")
		return s.unit(), true
	case "Chan.try_send":
		return s.rt("chan_try_send", boolT, h, s.one(arg(0, et))), true
	case "Chan.recv", "Chan.try_recv":
		l := s.rt(strings.ReplaceAll(strings.ToLower(key), ".", "_"), m.List(et), h)
		if fo.Name == "recv" {
			s.check("")
		}
		return s.popOne(l, nt), true
	case "Chan.close":
		return s.rt("chan_close", nil, h), true
	case "Chan.len":
		return s.rt("chan_len", intT, h), true
	case "Atomic.get":
		return s.rt("atomic_get", et, h), true
	case "Atomic.set":
		return s.rt("atomic_set", nil, h, arg(0, et)), true
	case "Atomic.swap":
		return s.rt("atomic_swap", et, h, arg(0, et)), true
	case "Atomic.compare_and_swap":
		return s.rt("atomic_cas", boolT, h, arg(0, et), arg(1, et)), true
	case "Atomic.add":
		return s.rt("atomic_add", intT, h, arg(0, intT), s.loc(x)), true
	case "Atomic.max":
		return s.rt("atomic_max", intT, h, arg(0, intT)), true
	case "Mutex.with", "Mutex.with_read":
		return s.mutexWith(x, h, et, fo.Name == "with_read"), true
	}
	b.unsupported(x, "the method `"+fo.DisplayName()+"`")
	return s.undef(nt), true
}

// popOne takes the element out of a list of one from the runtime as an
// Option (None for the empty list: a closed channel).
func (s *fn) popOne(l *Value, ot *Type) *Value {
	rt := s.b.mod.Tuple(l.Type, ot)
	r := s.call("list.pop", rt, "", l)
	return s.add(OpField, ot, 1, r)
}

// mutexWith lowers `m.with(f)` and `m.with_read(f)` (core 5.4): the value
// is taken from the mutex (in a list of one) under the lock and given to
// f; with a `var` parameter the closure returns the new value too, which
// goes back when f succeeds; otherwise (an error, unwinding, a read) the
// old value stays.
func (s *fn) mutexWith(x *ast.CallExpr, h *Value, et *Type, read bool) *Value {
	b := s.b
	m := b.mod
	nt := s.natOf(x)
	arg := x.Args[0].Value
	fv, packed := s.withClosure(arg, et)
	if fv == nil {
		return s.undef(nt)
	}
	lockFn, unlockFn := "mutex_lock", "mutex_unlock"
	if read {
		lockFn, unlockFn = "mutex_rlock", "mutex_runlock"
	}
	l := s.rt(lockFn, m.List(et), h, s.loc(x))
	s.check("")
	if packed {
		// the value moves out of the mutex's list into the closure, which
		// hands back what the mutex keeps (packedValue): the old value
		// when it fails or unwinds, else the new one. A closure that does
		// not fail after a change updates the value in place.
		var ect check.Type
		if ci := b.info.CallOf(x); ci != nil && ci.Recv != nil {
			if n, ok := check.Resolve(s.natCheck(ci.Recv)).(*check.Named); ok && len(n.Args) == 1 {
				ect = s.apply(n.Args[0])
			}
		}
		if ect == nil {
			b.unsupported(x, "this `with`")
			return s.undef(nt)
		}
		ot := b.typ(b.preludeType("Option", ect), x)
		if ot.Kind != KEnum || ot.VariantIndex("Some") < 0 || ot.Variants[ot.VariantIndex("Some")].Fields[0].Type != et {
			b.unsupported(x, "this `with`")
			return s.undef(nt)
		}
		pr := s.call("list.pop", m.Tuple(l.Type, ot), "", l)
		l1 := s.add(OpField, l.Type, 0, pr)
		o := s.add(OpField, ot, 1, pr)
		some := o.Type.VariantIndex("Some")
		v := s.add(OpPayload, et, PayloadRef{some, 0}, o)
		r := s.add(OpCallValue, fv.Type.Ret, nil, fv, v)
		res := s.add(OpField, r.Type.Elems[0], 0, r)
		nv := s.add(OpField, et, 1, r)
		l2 := s.call("list.push", l.Type, "", l1, nv)
		s.add(OpShare, nil, nil, l2)
		s.rt(unlockFn, nil, h, l2)
		s.check("")
		return res
	}
	v := s.call("list.index", et, "", l, s.cint(0))
	r := s.add(OpCallValue, fv.Type.Ret, nil, fv, v)
	s.checkThen("", func() { s.rt(unlockFn, nil, h, l) })
	if !packed {
		s.rt(unlockFn, nil, h, l)
		return r
	}
	res := s.add(OpField, r.Type.Elems[0], 0, r)
	nv := s.add(OpField, et, 1, r)
	store := func() {
		l2 := s.call("list.set", l.Type, "", l, s.cint(0), nv)
		s.add(OpShare, nil, nil, l2)
		s.rt(unlockFn, nil, h, l2)
	}
	rt := res.Type
	if rt.Kind == KEnum && rt.VariantIndex("Ok") >= 0 && rt.VariantIndex("Err") >= 0 && failErr(s.withErr(arg)) != nil {
		ok := s.add(OpEq, m.Builtin(KBool), nil, s.add(OpTag, m.Builtin(KInt), nil, res), s.cint(int64(rt.VariantIndex("Ok"))))
		join := s.newBlock()
		yes, no := s.branch(ok)
		s.cur = yes
		store()
		s.jump(join)
		s.cur = no
		s.rt(unlockFn, nil, h, l)
		s.jump(join)
		s.seal(join)
		s.cur = join
		return res
	}
	store()
	return res
}

// withErr is the error type of the closure given to `with`.
func (s *fn) withErr(e ast.Expr) check.Type {
	if ct, ok := check.Resolve(s.natCheck(e)).(*check.Func); ok {
		return s.apply(ct.Err)
	}
	return nil
}

// withClosure lowers the closure given to `with`: a literal with a `var`
// parameter becomes a function value returning (result, new value)
// (packed); anything else is an ordinary function value.
func (s *fn) withClosure(e ast.Expr, et *Type) (*Value, bool) {
	b := s.b
	lit, ok := e.(*ast.FnLit)
	ct, isFn := check.Resolve(s.natCheck(e)).(*check.Func)
	if !isFn || len(ct.Params) != 1 {
		b.unsupported(e, "this closure")
		return nil, false
	}
	if !ct.Params[0].Var {
		return s.valueAs(e, b.mod.FuncType([]*Type{et}, b.typ(b.resultOf(s.apply(ct.Result), failErr(s.apply(ct.Err))), e))), false
	}
	if !ok {
		b.unsupported(e, "a function value with a `var` parameter that is not a closure literal")
		return nil, false
	}
	return s.closureWith(lit, ct, et), true
}

// ---------------------------------------------------------------------------
// Channel loops

// forChan lowers `for p in ch` (core 3.5, 7.3): receives until the
// channel is closed and drained; each receive is a cancellation point.
func (s *fn) forChan(x *ast.ForStmt, label string) {
	b := s.b
	m := b.mod
	b.needConc = true
	h := s.value(x.Iter)
	if !b.conc {
		return
	}
	et := h.Type.Elems[0]
	ot := b.typ(b.preludeType("Option", s.elemCheck(x.Iter)), x)
	head, body, exit := s.newBlock(), s.newBlock(), s.newBlock()
	s.jump(head)
	s.cur = head
	l := s.rt("chan_recv", m.List(et), h)
	s.check("")
	o := s.popOne(l, ot)
	some := ot.VariantIndex("Some")
	c := s.add(OpEq, m.Builtin(KBool), nil, s.add(OpTag, m.Builtin(KInt), nil, o), s.cint(int64(some)))
	s.cur.Branch(c, body, exit)
	s.seal(body)
	s.cur = body
	elem := s.add(OpPayload, et, PayloadRef{some, 0}, o)
	fail := s.newBlock()
	s.pattern(elem, x.Pattern, fail)
	cont := s.cur
	s.cur = fail
	s.seal(fail)
	s.add(OpUnreachable, nil, nil)
	s.cur = cont
	s.loops = append(s.loops, &loopCtx{label: label, brk: exit, cont: head, scopes: len(s.scopes), exits: len(s.exits)})
	s.block(x.Body)
	s.loops = s.loops[:len(s.loops)-1]
	s.jump(head)
	s.seal(head)
	s.seal(exit)
	s.cur = exit
}

// elemCheck is the checker type T of a Chan[T] expression.
func (s *fn) elemCheck(e ast.Expr) check.Type {
	if n, ok := check.Resolve(s.natCheck(e)).(*check.Named); ok && len(n.Args) == 1 {
		return n.Args[0]
	}
	return nil
}

// noteBinding records the binding a spawn call's handle is stored in.
func (s *fn) noteBinding(e ast.Expr, vo *check.VarObj) {
	if call, ok := e.(*ast.CallExpr); ok && vo != nil {
		s.b.boundTo[call] = vo
	}
}

// noteBindings records every spawn bound with `let` or `var` in n.
func (s *fn) noteBindings(n ast.Node) {
	check.Inspect(n, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.LetStmt:
			if bp, ok := x.Pattern.(*ast.BindPat); ok {
				s.noteBinding(x.Value, s.b.info.VarOf(bp.Name))
			}
		case *ast.VarStmt:
			s.noteBinding(x.Value, s.b.info.VarOf(x.Name))
		}
		return true
	})
}

// closureWith lowers a closure literal with one `var` parameter of type
// et (given to Mutex.with): a function value returning its result and
// the parameter's final value as a tuple.
func (s *fn) closureWith(x *ast.FnLit, ct *check.Func, et *Type) *Value {
	b := s.b
	errT := failErr(s.apply(ct.Err))
	rv := b.typ(b.resultOf(s.apply(ct.Result), errT), x)
	t := b.mod.FuncType([]*Type{et}, b.mod.Tuple(rv, et))
	var caps []*check.VarObj
	seen := map[*check.VarObj]bool{}
	check.Inspect(x.Body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			if vo := b.info.VarOf(id); vo != nil && !seen[vo] {
				if _, outer := s.varType[vo]; outer {
					seen[vo] = true
					caps = append(caps, vo)
				}
			}
		}
		return true
	})
	var vals []*Value
	var capTs []*Type
	for _, vo := range caps {
		v := s.read(vo)
		vals = append(vals, v)
		capTs = append(capTs, v.Type)
	}
	b.nclos++
	name := s.f.Name + ".fn" + itoa(b.nclos)
	f := b.mod.NewFunc(name, []*Type{t, et}, []*Type{t.Ret})
	f.Indirect = true
	f.Captures = capTs
	// a fault in it hands back the value it was given, like cancellation
	// (the mutex keeps it); the caller goes on unwinding (decision 0085)
	f.Catches = true
	b.queue = append(b.queue, func() {
		c := b.newFn(f, s.apply(ct.Result), errT)
		c.sub = s.sub
		c.packVar = true
		env := f.Params()[0]
		for i, vo := range caps {
			c.writeVar(vo, c.add(OpCapture, capTs[i], i, env))
		}
		c.packOrig = f.Params()[1]
		if ps := b.info.ClosureParams(x); len(ps) == 1 && ps[0] != nil {
			c.writeVar(ps[0], f.Params()[1])
			c.varParams = []*check.VarObj{ps[0]}
		}
		v := c.block(x.Body)
		c.ret(v, x.Body)
		c.finishFn()
	})
	return s.add(OpClosure, t, name, vals...)
}

// selectExpr lowers `select { arms }` (core 7.3): the arms are handed to
// the runtime in source order, which performs the first that can
// proceed (or waits for one: a cancellation point); the arm's body runs
// with a receive's value bound. `after` arms need the clock, which the
// native backends do not have.
func (s *fn) selectExpr(x *ast.SelectExpr) *Value {
	b := s.b
	m := b.mod
	b.needConc = true
	t := s.typeOf(x)
	if !b.conc {
		return s.undef(t)
	}
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	sel := s.rt("select_new", m.Handle("Select"))
	recvs := make([]*Type, len(x.Arms)) // the Option a receive arm binds
	// the index select_wait returns for each arm: channel arms are
	// numbered in order, an `after` arm is -2
	rtIdx := make([]int64, len(x.Arms))
	n := int64(0)
	for i, a := range x.Arms {
		if a.After != nil {
			s.rt("select_after", nil, sel, s.valueAs(a.After, intT))
			rtIdx[i] = -2
			continue
		}
		rtIdx[i] = n
		n++
		call, ok := a.Op.(*ast.CallExpr)
		var se *ast.SelectorExpr
		if ok {
			se, _ = call.Fun.(*ast.SelectorExpr)
		}
		if se == nil {
			b.unsupported(a, "this `select` arm")
			continue
		}
		h := s.expr(se.X)
		if h.Type.Kind != KHandle || len(h.Type.Elems) != 1 {
			b.unsupported(a, "this `select` arm")
			continue
		}
		switch se.Sel.Name {
		case "recv":
			s.rt("select_recv", nil, sel, h)
			recvs[i] = b.typ(b.preludeType("Option", s.elemCheck(se.X)), a)
		case "send":
			s.rt("select_send", nil, sel, h, s.one(s.valueAs(call.Args[0].Value, h.Type.Elems[0])))
		default:
			b.unsupported(a, "this `select` arm")
		}
	}
	idx := s.rt("select_wait", intT, sel, s.loc(x))
	s.check("")
	hasVal := t.Kind != KUnit && !s.isNever(x)
	join := s.newBlock()
	var res *Value
	if hasVal {
		res = join.AddParam(t)
	}
	for i, a := range x.Arms {
		c := s.add(OpEq, boolT, nil, idx, s.cint(rtIdx[i]))
		yes, no := s.branch(c)
		s.cur = yes
		if ot := recvs[i]; ot != nil {
			et := ot.Variants[ot.VariantIndex("Some")].Fields[0].Type
			o := s.popOne(s.rt("select_take", m.List(et), sel, s.cint(int64(i))), ot)
			if a.Bind != nil {
				if vo := b.info.VarOf(a.Bind); vo != nil {
					s.writeVar(vo, o)
				}
			}
		}
		var av *Value
		if es, ok := a.Body.(*ast.ExprStmt); ok {
			av = s.expr(es.X)
		} else {
			s.stmt(a.Body)
			av = s.unit()
		}
		if hasVal {
			s.jump(join, s.coerce(av, t, a))
		} else {
			s.jump(join)
		}
		s.cur = no
	}
	s.add(OpUnreachable, nil, nil)
	s.seal(join)
	s.cur = join
	if res != nil {
		return res
	}
	return s.unit()
}
