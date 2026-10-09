package ssa

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/token"
)

// Test binaries on the native backends (`xo test --no-gc`, decision
// 0083). A test binary has a function per test block of the root modules
// and a main that runs the one test the environment variable XO_TEST
// names (one process per test: a fault fails that test only). The C
// runtime runs it in its test world (std/llrt/xo_testing.c): a virtual
// clock that jumps to the next timer when every context is blocked,
// seeded random numbers, a fake environment, captured standard streams
// and log records, and in memory listeners that http.server serves and
// the http client reaches in process. So the fakes of std/testing are
// the capabilities themselves (testCaps); their control methods are
// calls of the test world.
//
// A test's outcome reaches the harness (internal/build, cmd/xo) through
// its exit status and standard output: 0 passed; `test_fail` writes
// XO-TEST-FAIL and the message and exits 1 (a failed `expect`, an error
// returned by the body); a fault exits 70 as in programs.

// testCaps maps the fakes of std/testing to the capabilities they stand
// for.
var testCaps = map[string]string{
	"TestOs":    "Os",
	"FakeNet":   "Net",
	"MemFs":     "Fs",
	"FakeClock": "Clock",
	"FakeEnv":   "Env",
	"FakeStdio": "Stdio",
	"FakeProc":  "Proc",
}

// TestDecls lists the test blocks of the root modules of res in the
// order a test binary numbers them (module, file, declaration).
func TestDecls(res *check.Result) []*ast.TestDecl {
	var out []*ast.TestDecl
	for _, m := range res.Modules {
		for _, f := range m.Files {
			for _, d := range f.AST.Decls {
				if td, ok := d.(*ast.TestDecl); ok {
					out = append(out, td)
				}
			}
		}
	}
	return out
}

// TestName is the name of a test block (its string, without escapes).
func TestName(td *ast.TestDecl) string {
	var b strings.Builder
	for _, p := range td.Name.Parts {
		b.WriteString(p.Text)
	}
	return b.String()
}

// buildTests declares a function per test block and the test entry,
// main.
func (b *builder) buildTests(res *check.Result) {
	m := b.mod
	if len(res.Modules) > 0 {
		b.main = res.Modules[0]
	}
	osT, _ := m.Named("Os", KCap)
	unit := &check.Tuple{}
	errT := b.preludeType("Error")
	rt := b.typ(b.resultOf(unit, errT), nil)
	type entry struct {
		i    int
		name string
	}
	var tests []entry
	for i, td := range TestDecls(res) {
		if len(td.For) > 0 {
			b.unsupported(td, "a property test (`test ... for`)")
			continue
		}
		if len(td.Uses) > 0 {
			// skipped without --real; real capabilities are the Go
			// backend's (cmd/xo reports them as skipped)
			continue
		}
		name := fmt.Sprintf("test.%d", i)
		f := m.NewFunc(name, []*Type{osT}, []*Type{rt})
		td := td
		b.queue = append(b.queue, func() { b.testBody(td, f, errT) })
		tests = append(tests, entry{i, name})
	}
	intT, st := m.Builtin(KInt), m.Builtin(KStr)
	b.genFunc("main", nil, []*Type{m.Builtin(KUnit)}, func(s *fn, ps []*Value) {
		idx := s.rt("test_index", intT)
		for _, t := range tests {
			t := t
			s.ifThen(s.add(OpEq, m.Builtin(KBool), nil, idx, s.cint(int64(t.i))), func() {
				r := s.call(t.name, rt, "", s.zero(osT, map[*Type]bool{}))
				if ei := rt.VariantIndex("Err"); ei >= 0 {
					s.ifThen(s.add(OpEq, m.Builtin(KBool), nil, s.add(OpTag, intT, nil, r), s.cint(int64(ei))), func() {
						e := s.add(OpPayload, rt.Variants[ei].Fields[0].Type, PayloadRef{ei, 0}, r)
						msg := s.appendDisplay(s.add(OpConst, st, "error: "), e, errT, nil)
						s.rt("test_fail", nil, msg)
						s.add(OpUnreachable, nil, nil)
						s.dead()
					})
				}
				s.retv(s.unit())
			})
		}
		s.rt("test_fail", nil, s.add(OpConst, st, "no test with this index"))
		s.add(OpUnreachable, nil, nil)
		s.dead()
	})
}

// testBody builds the function of test block td: `os` is its parameter,
// and the body has the type `() ! Error` (core 9.2).
func (b *builder) testBody(td *ast.TestDecl, f *Func, errT check.Type) {
	s := b.newFn(f, &check.Tuple{}, errT)
	if osv := b.info.TestOs(td); osv != nil {
		s.writeVar(osv, f.Params()[0])
	}
	v := s.block(td.Body)
	s.ret(v, td.Body)
	s.finishFn()
}

// isExpect reports whether x is the predeclared `expect(...)` of a test.
func (s *fn) isExpect(x *ast.CallExpr) bool {
	id, ok := x.Fun.(*ast.Ident)
	info := s.b.info
	return ok && id.Name == "expect" && info.VarOf(id) == nil && info.Ref(id) == nil && info.CallOf(x) == nil && len(x.Args) == 1
}

// expectCall lowers `expect(cond)`: a false condition fails the test.
func (s *fn) expectCall(x *ast.CallExpr) *Value {
	b := s.b
	boolT := b.mod.Builtin(KBool)
	ok := s.valueAs(x.Args[0].Value, boolT)
	s.ifThen(s.add(OpNot, boolT, nil, ok), func() {
		s.testFail(s.add(OpConst, b.mod.Builtin(KStr), "expect failed: "+b.sourceText(x.Args[0].Value)+" at "+b.pos(x)))
	})
	return s.unit()
}

// testFail fails the test with msg (it does not return).
func (s *fn) testFail(msg *Value) {
	s.rt("test_fail", nil, msg)
	s.add(OpUnreachable, nil, nil)
	s.dead()
}

// expectCompare lowers `expect(a) op b`: the comparison, and both values
// (their Debug text) in the failure message, as std/rt's ExpectCmp.
func (s *fn) expectCompare(x *ast.BinaryExpr) *Value {
	b := s.b
	m := b.mod
	st, boolT := m.Builtin(KStr), m.Builtin(KBool)
	a := x.X.(*ast.CallExpr).Args[0].Value
	l := s.expr(a)
	ct := s.natCheck(a)
	if ot := s.natCheck(x.Y); l.Type.VariantIndex("Some") < 0 {
		// `expect(x) == Some(..)` of a value that is not an Option: the
		// comparison is of Options
		if n, ok := check.Resolve(check.Deep(ot)).(*check.Named); ok && n.Obj.Module != nil && n.Obj.Module.IsPrelude && n.Obj.Name == "Option" {
			if t := b.typ(ot, x); t.VariantIndex("Some") >= 0 && t.Variants[t.VariantIndex("Some")].Fields[0].Type == l.Type {
				l = s.coerce(l, t, x)
				ct = ot
			}
		}
	}
	r := s.valueAs(x.Y, l.Type)
	ok := s.compareValues(x.Op, l, r, x)
	s.ifThen(s.add(OpNot, boolT, nil, ok), func() {
		msg := s.add(OpConst, st, "expect failed: "+b.sourceText(x)+" at "+b.pos(x)+"\n      left:  ")
		msg = s.appendDebug(msg, l, ct, x)
		msg = s.call("str.concat", st, "", msg, s.add(OpConst, st, "\n      right: "))
		msg = s.appendDebug(msg, r, ct, x)
		s.testFail(msg)
	})
	return s.unit()
}

// isExpectCompare reports whether x is `expect(a) op b`.
func (s *fn) isExpectCompare(x *ast.BinaryExpr) bool {
	c, ok := x.X.(*ast.CallExpr)
	if !ok || !s.isExpect(c) {
		return false
	}
	_, isCmp := cmpOps[x.Op]
	return isCmp
}

// testCapCall lowers the control methods of the fakes (FakeClock.advance,
// FakeEnv.set, ...) and the printing methods of FakeStdio; key is the
// capability's name and the method's (`Clock.advance`). ok is false for
// a method of the capability itself, which ioCall lowers.
func (s *fn) testCapCall(x *ast.CallExpr, ci *check.Call, key string) (*Value, bool) {
	b := s.b
	fo := ci.Fn
	if fo.Module == nil || fo.Module.Path != "std/testing" {
		return nil, false
	}
	m := b.mod
	intT, strT := m.Builtin(KInt), m.Builtin(KStr)
	nt := s.natOf(x)
	es := argExprs(fo.Params, x)
	arg := func(i int, t *Type) *Value {
		if i >= len(es) || es[i] == nil {
			b.unsupported(x, "a call with a missing argument")
			return s.undef(t)
		}
		return s.valueAs(es[i], t)
	}
	switch key {
	case "Stdio.print", "Stdio.println", "Stdio.eprint", "Stdio.eprintln":
		r := s.expr(ci.Recv)
		return s.call(key, m.Builtin(KUnit), "", r, arg(0, strT)), true
	case "Stdio.output":
		return s.rt("test_output", strT, s.cint(1)), true
	case "Stdio.errors":
		return s.rt("test_output", strT, s.cint(2)), true
	case "Clock.advance":
		b.needConc = true
		if !b.conc {
			return s.undef(nt), true
		}
		s.rt("test_advance", nil, arg(0, intT))
		return s.unit(), true
	case "Clock.set":
		s.rt("test_clock_set", nil, arg(0, intT))
		return s.unit(), true
	case "Clock.sleeps":
		return s.rt("test_sleeps", m.List(intT)), true
	case "Env.set":
		s.rt("test_env_set", nil, arg(0, strT), arg(1, strT))
		return s.unit(), true
	case "Env.set_args":
		s.rt("test_set_args", nil, arg(0, m.List(strT)))
		return s.unit(), true
	case "Env.program":
		return s.add(OpConst, strT, "test"), true
	case "Proc.signal_after":
		s.rt("test_signal_after", nil, arg(0, intT))
		return s.unit(), true
	case "Proc.exit_code":
		return s.popOne(s.rt("test_exit_code", m.List(intT)), nt), true
	}
	return nil, false
}

// testCall lowers the functions of std/testing that need the builder:
// run_main. ok is false for the others (std/native/testing.xo implements
// them, or they are not available).
func (s *fn) testCall(x *ast.CallExpr, ci *check.Call) (*Value, bool) {
	fo := ci.Fn
	if fo.Module == nil || fo.Module.Path != "std/testing" || fo.Recv != nil || fo.Name != "run_main" {
		return nil, false
	}
	return s.runMain(x, ci), true
}

// runMain lowers `testing.run_main(main, os, args)` (std.md 8.4): main
// runs below a node of its own, which a second stop signal (or the grace
// period) cancels as the root of a program; an error from main is
// printed on the fake standard error and is status 1; when main unwound
// because of the stop, the unwinding ends here and the status is 0.
func (s *fn) runMain(x *ast.CallExpr, ci *check.Call) *Value {
	b := s.b
	m := b.mod
	fo := ci.Fn
	intT, strT, boolT := m.Builtin(KInt), m.Builtin(KStr), m.Builtin(KBool)
	b.needConc = true
	if !b.conc {
		return s.undef(intT)
	}
	es := argExprs(fo.Params, x)
	if es[0] == nil || es[1] == nil {
		b.unsupported(x, "a call with a missing argument")
		return s.undef(intT)
	}
	ft := b.typ(s.apply(fo.Params[0].Type), x)
	osT, _ := m.Named("Os", KCap)
	mainV := s.valueAs(es[0], ft)
	osV := s.valueAs(es[1], osT)
	var args *Value
	if len(es) > 2 && es[2] != nil {
		args = s.valueAs(es[2], m.List(strT))
	} else {
		args = s.call("list.new", m.List(strT), "")
	}
	if ft.Kind != KFunc || len(ft.Elems) != 1 {
		b.unsupported(x, "this `main` for testing.run_main")
		return s.undef(intT)
	}
	s.rt("test_main_begin", nil, args)
	r := s.add(OpCallValue, ft.Ret, nil, mainV, osV)
	code := s.cint(0)
	if ei := ft.Ret.VariantIndex("Err"); ei >= 0 {
		join := s.newBlock()
		res := join.AddParam(intT)
		failed := s.add(OpEq, boolT, nil, s.add(OpTag, intT, nil, r), s.cint(int64(ei)))
		yes, no := s.branch(failed)
		s.cur = yes
		e := s.add(OpPayload, ft.Ret.Variants[ei].Fields[0].Type, PayloadRef{ei, 0}, r)
		msg := s.appendDisplay(s.add(OpConst, strT, "error: "), e, b.preludeType("Error"), x)
		s.rt("test_eprint", nil, s.call("str.concat", strT, "", msg, s.add(OpConst, strT, "\n")))
		s.jump(join, s.cint(1))
		s.cur = no
		s.jump(join, s.cint(0))
		s.seal(join)
		s.cur = join
		code = res
	}
	out := s.rt("test_main_end", intT, code)
	s.check("")
	return out
}

var _ = token.Eq
