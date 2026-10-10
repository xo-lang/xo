package ssa

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// os.tasks on the native backends (core 7.1, decision 0109). As std/rt's
// RunMain, main runs as the body of a root scope whose group is os.tasks:
// when main returns, the scope closes (background tasks are canceled,
// the others awaited), and a group task that failed is the program's
// error (`error: <text>`, status 1) before main's own. A test binary
// opens such a scope around the test it runs (TestOs.tasks lives for the
// test). The C runtime keeps the open scope (rt.prog_group_set) for
// `os.tasks` to read (rt.prog_group).

// usesOsTasks reports whether a module of the program reads `tasks` of
// an Os or a TestOs.
func (b *builder) usesOsTasks() bool {
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
				x, ok := n.(*ast.SelectorExpr)
				if !ok || x.Sel.Name != "tasks" {
					return true
				}
				t := b.info.NaturalType(x.X)
				if t == nil {
					t = b.info.TypeOf(x.X)
				}
				if nt, ok := check.Resolve(check.Deep(t)).(*check.Named); ok && nt.Obj.Module != nil &&
					(nt.Obj.Module.IsPrelude && nt.Obj.Name == "Os" || nt.Obj.Module.Path == "std/testing" && nt.Obj.Name == "TestOs") {
					found = true
				}
				return !found
			})
		}
	}
	return found
}

// rootScope runs call (the program's main, or a test) as the body of
// the scope of os.tasks, closed with mode (scope_close: 0 awaits the
// tasks that are not background tasks, 1 cancels every task, as when a
// test ends). A failed group task prints its error and exits with status
// 1 (fail, given the error's text, does that when set: a test fails
// instead).
func (s *fn) rootScope(sites []*spawnSite, at ast.Node, mode int64, call func(), fail func(msg *Value)) {
	b := s.b
	m := b.mod
	intT, st := m.Builtin(KInt), m.Builtin(KStr)
	h := s.rt("scope_open", m.Handle("Scope"))
	s.rt("prog_group_set", nil, h)
	sc := &scopeCtx{h: h, sites: sites}
	s.scopes = append(s.scopes, sc)
	s.exits = append(s.exits, exitEntry{scope: sc})
	call()
	s.exits = s.exits[:len(s.exits)-1]
	s.scopes = s.scopes[:len(s.scopes)-1]
	clear := func() { s.rt("prog_group_clear", nil) }
	if !s.isDead() {
		stv := s.rt("scope_close", intT, h, s.cint(mode))
		clear()
		s.closeStatus(sc, stv)
	}
	saved := s.cur
	if sc.unwindB != nil {
		s.seal(sc.unwindB)
		s.cur = sc.unwindB
		stv := s.rt("scope_close", intT, h, s.cint(2))
		clear()
		s.closeStatus(sc, stv)
		s.unwind()
	}
	if sc.failB != nil {
		s.seal(sc.failB)
		s.cur = sc.failB
		site := s.rt("scope_fail_site", intT, h)
		for _, sp := range sc.sites {
			yes, no := s.branch(s.add(OpEq, m.Builtin(KBool), nil, site, s.cint(int64(sp.id))))
			s.cur = yes
			tt := b.taskType(sp.resT, sp.errT, at)
			rt := b.typ(b.resultOf(sp.resT, sp.errT), at)
			th := s.rt("scope_fail_task", tt, h)
			l := s.rt("task_result", m.List(rt), th)
			r := s.call("list.index", rt, "", l, s.cint(0))
			ei := rt.VariantIndex("Err")
			e := s.add(OpPayload, rt.Variants[ei].Fields[0].Type, PayloadRef{ei, 0}, r)
			msg := s.appendDisplay(s.add(OpConst, st, "error: "), e, sp.errT, at)
			if fail != nil {
				fail(msg)
			} else {
				s.rt("eprintln", nil, msg)
				s.printTrace(s.errTrace(r, ei))
				s.rt("exit", nil, s.cint(1))
				s.add(OpUnreachable, nil, nil)
			}
			s.dead()
			s.cur = no
		}
		s.add(OpUnreachable, nil, nil)
		s.dead()
	}
	s.cur = saved
}

// tasksMain makes `main` of a program that uses os.tasks: the program's
// main (mf, whose error type is et or nil) becomes main.run, called in
// the root scope; its error is printed after the scope closed, as by
// mainWrapper.
func (b *builder) tasksMain(mf *Func, et check.Type, mainFo *check.FuncObj) {
	m := b.mod
	mf.Name = "main.run"
	var params []*Type
	for _, p := range mf.Params() {
		params = append(params, p.Type)
	}
	unit, intT, boolT, st := m.Builtin(KUnit), m.Builtin(KInt), m.Builtin(KBool), m.Builtin(KStr)
	at := mainFo.Decl
	b.genFunc("main", params, []*Type{unit}, func(s *fn, ps []*Value) {
		var sites []*spawnSite
		if mainFo.Decl != nil && mainFo.Decl.Body != nil {
			// only main's body sees os.tasks's origin, so only its
			// spawns can fail on it (XO0801)
			sites = b.failingSpawns(s, mainFo.Decl.Body)
		}
		rt := mf.Results[0]
		var r *Value
		s.rootScope(sites, at, 0, func() {
			r = s.call(mf.Name, rt, "", ps...)
			s.check(mf.Name)
		}, nil)
		if et != nil && !s.isDead() {
			ei := rt.VariantIndex("Err")
			s.ifThen(s.add(OpEq, boolT, nil, s.add(OpTag, intT, nil, r), s.cint(int64(ei))), func() {
				e := s.add(OpPayload, rt.Variants[ei].Fields[0].Type, PayloadRef{ei, 0}, r)
				msg := s.appendDisplay(s.add(OpConst, st, "error: "), e, et, at)
				s.rt("eprintln", nil, msg)
				s.printTrace(s.errTrace(r, ei))
				s.rt("exit", nil, s.cint(1))
			})
		}
		if !s.isDead() {
			s.retv(s.unit())
		}
	})
}
