package ssa

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Clock, network, environment, and shutdown (decision 0069), lowered to
// calls of std/llrt's event loop:
//
//   - Duration and Time are Int nanoseconds (the Go backend's
//     representation), Bytes is a Str (an immutable byte string; the
//     checker keeps the two apart), Clock is a capability (zero size),
//     Net, Proc, and Env are the lists of their attenuations (listsType),
//     Listener and Conn are handles.
//   - `clock.sleep` is a cancellation point that unwinds; the network
//     operations report cancellation as `NetErr.Canceled` (core 7.2). A
//     network call records its outcome in the runtime (rt.net_status,
//     rt.net_msg), from which the Result is built (netResult).
//   - Every operation that can wait makes the program build with
//     unwinding checks (needConc), so a shutdown signal can unwind it.

// statusTags are the runtime's status codes of NetErr's variants (the Go
// backend's tags).
var statusTags = map[string]int64{"InvalidAddr": 1, "Refused": 2, "Timeout": 3, "Io": 4, "Canceled": 5, "Denied": 6}

// ioCall lowers a prelude method of Clock, Net, Listener, Conn, Proc,
// Env, Duration, Time, Str, or Bytes that the event loop or inline code
// implements; ok is false for anything else.
func (s *fn) ioCall(x *ast.CallExpr, ci *check.Call) (*Value, bool) {
	b := s.b
	fo := ci.Fn
	if fo.Module == nil || fo.Recv == nil {
		return nil, false
	}
	recvName := fo.Recv.Name
	if fo.Module.Path == "std/testing" {
		// a fake of a test is its capability (build_testmode.go)
		c, ok := testCaps[recvName]
		if !ok {
			return nil, false
		}
		recvName = c
	} else if !fo.Module.IsPrelude {
		return nil, false
	}
	m := b.mod
	intT, boolT, strT, floatT := m.Builtin(KInt), m.Builtin(KBool), m.Builtin(KStr), m.Builtin(KFloat)
	key := recvName + "." + fo.Name
	if recvName == "Fs" {
		return s.fsCall(x, ci), true
	}
	if recvName == "Stdio" && fo.Module.Path == "std/testing" {
		switch fo.Name {
		case "write", "read_line", "read_all", "lines", "read_secret", "read_bytes", "read_all_bytes",
			"stdin_is_tty", "stdout_is_tty", "stderr_is_tty", "width":
			// a test's FakeStdio: the same functions, whose runtime
			// reads the input set_input gave and has no terminal
			// (std/llrt/xo_stdin.c)
			if stdioReads(fo.Name) {
				b.needConc = true
				if !b.conc {
					return s.undef(s.natOf(x)), true
				}
			}
			if nm := b.nativeMod("std/prelude"); nm != nil {
				if impl, ok := nm.Scope["stdio_"+fo.Name].(*check.FuncObj); ok && impl.Recv == nil {
					return s.callAs(x, ci, impl), true
				}
			}
		}
	}
	if v, ok := s.testCapCall(x, ci, key); ok {
		return v, true
	}
	switch recvName {
	case "Clock", "Net", "Listener", "Conn", "Proc", "Env", "Duration", "Time", "Bytes", "Fs", "StrBuilder", "BytesBuilder", "Rand":
	case "Str":
		if fo.Name != "bytes" {
			return nil, false
		}
	default:
		return nil, false
	}
	nt := s.natOf(x)
	es := argExprs(fo.Params, x)
	arg := func(i int, t *Type) *Value {
		if es[i] == nil {
			if fo.Params[i].Default != nil {
				return s.valueAs(fo.Params[i].Default, t)
			}
			b.unsupported(x, "a call with a missing argument")
			return s.undef(t)
		}
		return s.valueAs(es[i], t)
	}
	// operations that wait: the program needs unwinding checks
	switch key {
	case "Clock.sleep", "Clock.sleep_or_cancel", "Net.listen", "Net.dial", "Listener.accept", "Conn.read", "Conn.write", "Proc.pipeline":
		b.needConc = true
		if !b.conc {
			return s.undef(nt), true
		}
	}
	pos := b.posLine(x)
	mul := func(v *Value, k int64) *Value {
		r := s.add(OpMul, intT, nil, v, s.cint(k))
		r.Pos = pos
		return r
	}
	if fo.Static {
		switch key {
		case "Duration.from_nanos":
			return arg(0, intT), true
		case "Duration.from_millis":
			return mul(arg(0, intT), 1e6), true
		case "Duration.from_secs_int":
			return mul(arg(0, intT), 1e9), true
		case "Time.from_unix":
			return mul(arg(0, intT), 1e9), true
		case "Time.from_unix_millis":
			return mul(arg(0, intT), 1e6), true
		case "Time.UNIX_EPOCH":
			return s.cint(0), true
		case "StrBuilder.new", "BytesBuilder.new":
			return s.add(OpConst, strT, ""), true
		case "BytesBuilder.with_capacity":
			return s.rt("rs_with_cap", strT, arg(0, intT)), true
		}
		b.unsupported(x, "`"+fo.DisplayName()+"`")
		return s.undef(nt), true
	}
	switch key {
	case "StrBuilder.write", "BytesBuilder.write", "BytesBuilder.write_str", "BytesBuilder.write_byte", "BytesBuilder.write_rune":
		// the builder is the text so far; appending to it in place
		// needs the only reference, which a move only builder has
		p, ok := s.place(ci.Recv)
		if !ok {
			return s.undef(nt), true
		}
		cur := p.get()
		switch fo.Name {
		case "write_byte":
			p.set(s.rt("rs_push_byte", strT, cur, arg(0, m.IntType("UInt8"))))
		case "write_rune":
			p.set(s.rt("rs_push_rune", strT, cur, arg(0, m.IntType("Rune"))))
		default:
			p.set(s.call("str.concat", strT, "", cur, arg(0, strT)))
		}
		return s.unit(), true
	}
	if name, ok := capImpls[key]; ok {
		// the native prelude's implementation (a capability's method has
		// no implementation lookup of its own)
		if nm := b.nativeMod("std/prelude"); nm != nil {
			if impl, ok := nm.Scope[name].(*check.FuncObj); ok {
				return s.callAs(x, ci, impl), true
			}
		}
	}
	recv := s.expr(ci.Recv)
	switch key {
	case "StrBuilder.len", "BytesBuilder.len":
		return s.call("Str.len", intT, "", recv), true
	case "StrBuilder.freeze", "BytesBuilder.freeze":
		return recv, true
	case "Fs.read":
		l := s.rt("fs_read", m.List(strT), arg(0, strT))
		ok := nt.Variants[nt.VariantIndex("Ok")].Fields[0].Type
		optT := b.typ(b.preludeType("Option", b.preludeType("Bytes")), x)
		return s.fsResult(s.popOne(l, optT), ok, nt), true
	case "Env.args":
		return s.rt("env_args", m.List(strT)), true
	case "Proc.exit":
		s.rt("exit", nil, arg(0, intT))
		if b.opts.Tests {
			// in testing.run_main the exit unwinds main back to it
			// (decision 0108): the runtime returns unwinding
			s.check("")
		}
		s.add(OpUnreachable, nil, nil)
		s.dead()
		return s.undef(nt), true
	case "Rand.int":
		// the operating system's generator (decision 0083)
		return s.rt("rand_int", intT, arg(0, intT), arg(1, intT), s.add(OpConst, strT, pos)), true
	case "Rand.float":
		return s.rt("rand_float", floatT), true
	case "Rand.bool":
		return s.rt("rand_bool", boolT), true
	case "Rand.bytes":
		return s.rt("rand_bytes", strT, arg(0, intT)), true
	case "Clock.now":
		return s.rt("clock_now", intT), true
	case "Clock.since":
		r := s.add(OpSub, intT, nil, s.rt("clock_now", intT), arg(0, intT))
		r.Pos = pos
		return r, true
	case "Clock.sleep":
		s.rt("clock_sleep", nil, arg(0, intT))
		s.check("")
		return s.unit(), true
	case "Clock.sleep_or_cancel":
		// Ok(()) or Err(ClockErr.Canceled), never an unwind (decision 0088)
		c := s.rt("clock_sleep_or_cancel", intT, arg(0, intT))
		join := s.newBlock()
		res := join.AddParam(nt)
		yes, no := s.branch(s.add(OpEq, boolT, nil, c, s.cint(0)))
		s.cur = yes
		s.jump(join, s.add(OpVariant, nt, nt.VariantIndex("Ok"), s.unit()))
		s.cur = no
		ei := nt.VariantIndex("Err")
		et := nt.Variants[ei].Fields[0].Type
		s.jump(join, s.add(OpVariant, nt, ei, s.add(OpVariant, et, et.VariantIndex("Canceled"))))
		s.seal(join)
		s.cur = join
		return res, true
	case "Proc.set_grace":
		s.rt("set_grace", nil, arg(0, intT))
		return s.unit(), true
	case "Net.listen":
		// the Net's attenuation lists first (Net.only, decision 0175)
		h := s.rt("net_listen", m.Handle("Listener"), s.add(OpField, strT, 0, recv), arg(0, strT))
		return s.netResult(h, nt), true
	case "Net.dial":
		h := s.rt("net_dial", m.Handle("Conn"), s.add(OpField, strT, 0, recv), arg(0, strT))
		return s.netResult(h, nt), true
	case "Listener.addr":
		return s.call("list.index", strT, "", s.rt("listener_addr", m.List(strT), recv), s.cint(0)), true
	case "Listener.accept":
		h := s.rt("listener_accept", m.Handle("Conn"), recv)
		return s.netResult(h, nt), true
	case "Listener.close":
		s.rt("listener_close", nil, recv)
		return s.unit(), true
	case "Conn.read":
		l := s.rt("conn_read", m.List(strT), recv, arg(0, intT))
		ok := nt.Variants[nt.VariantIndex("Ok")].Fields[0].Type
		return s.netResult(s.popOne(l, ok), nt), true
	case "Conn.write":
		s.rt("conn_write", nil, recv, arg(0, strT))
		return s.netResult(s.unit(), nt), true
	case "Conn.close":
		s.rt("conn_close", nil, recv)
		return s.unit(), true
	case "Str.bytes":
		return recv, true
	case "Bytes.len":
		return s.call("Str.len", intT, "", recv), true
	case "Bytes.is_empty":
		return s.add(OpEq, boolT, nil, s.call("Str.len", intT, "", recv), s.cint(0)), true
	case "Bytes.to_str":
		join := s.newBlock()
		res := join.AddParam(nt)
		yes, no := s.branch(s.rt("utf8_valid", boolT, recv))
		s.cur = yes
		s.jump(join, s.variant(nt, "Some", []*Value{recv}, x))
		s.cur = no
		s.jump(join, s.variant(nt, "None", nil, x))
		s.seal(join)
		s.cur = join
		return res, true
	case "Duration.nanos":
		return recv, true
	case "Duration.millis":
		return s.add(OpDiv, intT, nil, recv, s.cint(1e6)), true
	case "Duration.seconds":
		return s.add(OpDiv, floatT, nil, s.add(OpIntToFloat, floatT, nil, recv), s.add(OpConst, floatT, 1e9)), true
	case "Duration.abs":
		return s.call("Int.abs", intT, pos, recv), true
	case "Time.unix":
		return s.rt("floor_div", intT, recv, s.cint(1e9)), true
	case "Time.unix_millis":
		return s.rt("floor_div", intT, recv, s.cint(1e6)), true
	}
	b.unsupported(x, "the method `"+fo.DisplayName()+"`")
	return s.undef(nt), true
}

// capImpls are the capability methods implemented in the native prelude
// (std/native/prelude.xo) by a function of their own.
var capImpls = map[string]string{
	"Rand.shuffle":   "rand_shuffle",
	"Env.config_dir": "env_config_dir", // decision 0163
	"Env.cache_dir":  "env_cache_dir",
	"Env.get":        "env_get", // decision 0172: attenuation
	"Env.vars":       "env_vars",
	"Env.home":       "env_home",
	"Env.only":       "env_only",
	"Proc.only":      "proc_only",
	"Net.only":       "net_only", // decision 0175
	"Proc.cmd":       "proc_cmd", // decision 0161
	"Proc.pipeline":  "proc_pipeline",
}

// nativeWaits reports whether fo is a prelude method that the native
// prelude implements and that waits on the event loop (decision 0172):
// the program needs unwinding checks.
func nativeWaits(fo *check.FuncObj) bool {
	if fo.Recv == nil || fo.Static || fo.Module == nil || !fo.Module.IsPrelude {
		return false
	}
	switch fo.Recv.Name + "." + fo.Name {
	case "Cmd.output", "Cmd.run", "Cmd.spawn", "Child.write", "Child.read", "Child.read_err", "Child.read_line", "Child.wait":
		return true
	}
	return false
}

// stdioReads reports whether the Stdio method name reads standard input
// (it waits on the event loop: the program needs unwinding checks).
func stdioReads(name string) bool {
	switch name {
	case "read_line", "read_all", "lines", "read_secret", "read_bytes", "read_all_bytes":
		return true
	}
	return false
}

// netResult is the Result (type rt) of a network operation whose value
// on success is ok: Ok(ok) when the runtime reports no error, else
// Err(the NetErr it reports).
func (s *fn) netResult(ok *Value, rt *Type) *Value {
	m := s.b.mod
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	st := s.rt("net_status", intT)
	join := s.newBlock()
	res := join.AddParam(rt)
	yes, no := s.branch(s.add(OpEq, boolT, nil, st, s.cint(0)))
	s.cur = yes
	s.jump(join, s.add(OpVariant, rt, rt.VariantIndex("Ok"), ok))
	s.cur = no
	ei := rt.VariantIndex("Err")
	et := rt.Variants[ei].Fields[0].Type
	e := s.call(s.b.netErrFunc(et), et, "", st)
	s.jump(join, s.add(OpVariant, rt, ei, e))
	s.seal(join)
	s.cur = join
	return res
}

// fsResult is the Result (type rt) of a file system operation whose
// value on success is v (an Option of the ok type, from popOne): Ok of
// its payload when the runtime reports no error, else Err(the FsErr it
// reports).
func (s *fn) fsResult(v *Value, ok, rt *Type) *Value {
	m := s.b.mod
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	st := s.rt("fs_status", intT)
	join := s.newBlock()
	res := join.AddParam(rt)
	yes, no := s.branch(s.add(OpEq, boolT, nil, st, s.cint(0)))
	s.cur = yes
	some := v.Type.VariantIndex("Some")
	s.jump(join, s.add(OpVariant, rt, rt.VariantIndex("Ok"), s.add(OpPayload, ok, PayloadRef{some, 0}, v)))
	s.cur = no
	ei := rt.VariantIndex("Err")
	et := rt.Variants[ei].Fields[0].Type
	e := s.call(s.b.fsErrFunc(et), et, "", st)
	s.jump(join, s.add(OpVariant, rt, ei, e))
	s.seal(join)
	s.cur = join
	return res
}

// fsErrFunc returns the function that makes the FsErr (type et) of a
// runtime status code: the variant's tag plus one (std/rt's FsErr tags);
// its path and Io's message come from rt.fs_msg (a list of the path and
// the message).
func (b *builder) fsErrFunc(et *Type) string {
	m := b.mod
	name := "xo.fserr"
	if m.Func(name) != nil {
		return name
	}
	intT, boolT, strT := m.Builtin(KInt), m.Builtin(KBool), m.Builtin(KStr)
	f := m.NewFunc(name, []*Type{intT}, []*Type{et})
	cur := f.Blocks[0]
	st := f.Params()[0]
	for vi, v := range et.Variants {
		yes, no := f.NewBlock(), f.NewBlock()
		c := cur.Add(OpEq, boolT, nil, st, cur.Add(OpConst, intT, int64(vi+1)))
		cur.Branch(c, yes, no)
		var fs []*Value
		if len(v.Fields) > 0 {
			l := yes.Add(OpCall, m.List(strT), "rt.fs_msg")
			for i := range v.Fields {
				fs = append(fs, yes.Add(OpCall, strT, "list.index", l, yes.Add(OpConst, intT, int64(i))))
			}
		}
		yes.Add(OpReturn, nil, nil, yes.Add(OpVariant, et, vi, fs...))
		cur = no
	}
	cur.Add(OpUnreachable, nil, nil)
	return name
}

// netErrFunc returns the function that makes the NetErr (type et) of a
// runtime status code: the message of InvalidAddr and Io comes from
// rt.net_msg.
func (b *builder) netErrFunc(et *Type) string {
	m := b.mod
	name := "xo.neterr"
	if m.Func(name) != nil {
		return name
	}
	intT, boolT, strT := m.Builtin(KInt), m.Builtin(KBool), m.Builtin(KStr)
	f := m.NewFunc(name, []*Type{intT}, []*Type{et})
	cur := f.Blocks[0]
	st := f.Params()[0]
	for vi, v := range et.Variants {
		yes, no := f.NewBlock(), f.NewBlock()
		c := cur.Add(OpEq, boolT, nil, st, cur.Add(OpConst, intT, statusTags[v.Name]))
		cur.Branch(c, yes, no)
		var fs []*Value
		if len(v.Fields) == 1 {
			l := yes.Add(OpCall, m.List(strT), "rt.net_msg")
			fs = append(fs, yes.Add(OpCall, strT, "list.index", l, yes.Add(OpConst, intT, int64(0))))
		}
		yes.Add(OpReturn, nil, nil, yes.Add(OpVariant, et, vi, fs...))
		cur = no
	}
	cur.Add(OpUnreachable, nil, nil)
	return name
}

// withTimeout lowers `task.with_timeout(clock, d, f)` (std.md 7): f runs
// as the body of a scope whose deadline cancels it; when f unwinds
// because of the deadline (and not a cancellation of the code around
// it), the result is Err(TimedOut); f's own error e is Err(Failed(e)).
func (s *fn) withTimeout(x *ast.CallExpr, fo *check.FuncObj) *Value {
	b := s.b
	m := b.mod
	nt := s.natOf(x)
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	es := argExprs(fo.Params, x)
	if es[0] == nil || es[1] == nil || es[2] == nil {
		b.unsupported(x, "a call with a missing argument")
		return s.undef(nt)
	}
	s.expr(es[0])
	d := s.valueAs(es[1], intT)
	ct, ok := check.Resolve(s.natCheck(es[2])).(*check.Func)
	if !ok {
		b.unsupported(x, "this closure")
		return s.undef(nt)
	}
	errT := failErr(s.apply(ct.Err))
	rt := b.typ(b.resultOf(s.apply(ct.Result), errT), x)
	fv := s.valueAs(es[2], m.FuncType(nil, rt))
	h := s.rt("scope_open", m.Handle("Scope"))
	s.rt("scope_deadline", nil, h, d)
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
	timedT := nt.Variants[ei].Fields[0].Type
	unw, done := s.branch(u)
	s.b.checks = append(s.b.checks, unwindCheck{s.f, u, "", r})
	// unwinding: the deadline, or a cancellation from outside
	s.cur = unw
	s.rt("scope_close", intT, h, s.cint(2))
	to, out := s.branch(s.rt("scope_timed_out", boolT, h))
	s.cur = to
	s.jump(join, s.add(OpVariant, nt, ei, s.variant(timedT, "TimedOut", nil, x)))
	s.cur = out
	s.unwind()
	// f returned
	s.cur = done
	s.rt("scope_close", intT, h, s.cint(0))
	// f finished after the deadline fired (it did not reach a
	// cancellation point): timed out all the same, as std/rt's
	// awaitDeadline decides by which came first
	late, intime := s.branch(s.rt("scope_timed_out", boolT, h))
	s.cur = late
	s.jump(join, s.add(OpVariant, nt, ei, s.variant(timedT, "TimedOut", nil, x)))
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
		s.jump(join, s.add(OpVariant, nt, ei, s.variant(timedT, "Failed", []*Value{e}, x)))
	}
	s.seal(join)
	s.cur = join
	return res
}

// durationLit is a Duration literal: Int nanoseconds.
func (s *fn) durationLit(x *ast.DurationLit) *Value {
	var n int64
	for _, c := range x.Nanos {
		if c < '0' || c > '9' {
			s.b.unsupported(x, "this Duration literal")
			return s.cint(0)
		}
		n = n*10 + int64(c-'0')
	}
	return s.cint(n)
}

// isPreludeNamed reports whether checker type t is one of the prelude
// types names.
func isPreludeNamed(t check.Type, names ...string) bool {
	n, ok := check.Resolve(check.Deep(t)).(*check.Named)
	if !ok || n.Obj.Module == nil || !n.Obj.Module.IsPrelude {
		return false
	}
	for _, name := range names {
		if n.Obj.Name == name {
			return true
		}
	}
	return false
}

// fsType is the lowered Fs (decision 0102): its root ("" for os.fs) and
// mode bits (1 a sub capability, 2 read only, 4 write only).
func (b *builder) fsType() *Type {
	m := b.mod
	t, created := m.Named("xo.Fs", KStruct)
	if created {
		t.Fields = []Field{{"root", m.Builtin(KStr)}, {"mode", m.Builtin(KInt)}}
	}
	return t
}

// fsCall lowers a method of Fs: the function fs_<name> of
// std/native/prelude.xo, whose first parameter is the Fs (std/rt's
// realFs, ported to Xo over the system calls of std/llrt/xo_fs.c). In a
// test binary every Fs is a view of the test's MemFs (memFsCall).
func (s *fn) fsCall(x *ast.CallExpr, ci *check.Call) *Value {
	b := s.b
	fo := ci.Fn
	if v, ok := s.memFsCall(x, ci); ok {
		return v
	}
	if fo.Name == "lock" {
		// waits, polling on the event loop's timers (decision 0172)
		b.needConc = true
		if !b.conc {
			return s.undef(s.natOf(x))
		}
	}
	if nm := b.nativeMod("std/prelude"); nm != nil {
		if impl, ok := nm.Scope["fs_"+fo.Name].(*check.FuncObj); ok && impl.Recv == nil {
			return s.callAs(x, ci, impl)
		}
	}
	b.unsupported(x, "the method `Fs."+fo.Name+"`")
	return s.undef(s.natOf(x))
}

// isFs reports whether t is the lowered Fs.
func isFs(t *Type) bool { return t.Kind == KStruct && t.Name == "xo.Fs" }

// listsType is the lowered Proc, Env, or Net (decisions 0172 and 0175):
// the lists of its attenuations (Proc.only, Env.only, Net.only) in one
// Str, "" for none (std/native/prelude.xo cap_admits, the C runtime's
// net_admits).
func (b *builder) listsType(name string) *Type {
	m := b.mod
	t, created := m.Named("xo."+name, KStruct)
	if created {
		t.Fields = []Field{{"lists", m.Builtin(KStr)}}
	}
	return t
}

// isLists reports whether t is the lowered Proc, Env, or Net.
func isLists(t *Type) bool {
	return t.Kind == KStruct && (t.Name == "xo.Proc" || t.Name == "xo.Env" || t.Name == "xo.Net")
}

// memFsCall lowers a method of Fs or MemFs in a test binary (decision
// 0108): the function memfs_<name> of std/native/testing.xo, whose first
// parameter is the Fs (a view of the test's in memory file system,
// std/rt's memFs ported to Xo). ok is false outside test binaries and
// for anything else.
func (s *fn) memFsCall(x *ast.CallExpr, ci *check.Call) (*Value, bool) {
	b := s.b
	fo := ci.Fn
	if !b.opts.Tests || fo.Recv == nil || fo.Static || fo.Module == nil {
		return nil, false
	}
	if !(fo.Module.IsPrelude && fo.Recv.Name == "Fs") && !(fo.Module.Path == "std/testing" && fo.Recv.Name == "MemFs") {
		return nil, false
	}
	if nm := b.nativeMod("std/testing"); nm != nil {
		if impl, ok := nm.Scope["memfs_"+fo.Name].(*check.FuncObj); ok && impl.Recv == nil {
			// Fs methods, and the MemFs methods that are Fs methods (a
			// TestOs's fs is a MemFs); MemFs's own control methods stay
			if b.opts.RealTests {
				if pm := b.nativeMod("std/prelude"); pm != nil {
					if real, ok := pm.Scope["fs_"+fo.Name].(*check.FuncObj); ok && real.Recv == nil {
						// `xo test --real` (decision 0120): the real Fs
						// when the test declares fs effects
						return s.callAsVia(x, ci, impl, func(callee *Func) string {
							return b.realFsWrapper(callee, b.declare(real, nil, x))
						}), true
					}
				}
			}
			return s.callAs(x, ci, impl), true
		}
	}
	b.unsupported(x, "the method `"+fo.Recv.Name+"."+fo.Name+"` of a test's MemFs")
	return s.undef(s.natOf(x)), true
}

// realFsWrapper returns xo.testfs[memfs], with the signature of the
// MemFs implementation memfs: at run time it calls the real Fs
// implementation real when the test declares fs effects with `xo test
// --real` (rt.test_fs_real: the mode bits of its view, std/rt's
// realCaps), else memfs.
func (b *builder) realFsWrapper(memfs, real *Func) string {
	m := b.mod
	intT, boolT := m.Builtin(KInt), m.Builtin(KBool)
	var ps []*Type
	for _, p := range memfs.Params() {
		ps = append(ps, p.Type)
	}
	return b.genFunc("xo.testfs["+memfs.Name+"]", ps, memfs.Results, func(s *fn, args []*Value) {
		tuple := func(r *Value) {
			s.add(OpReturn, nil, nil, r)
			s.dead()
		}
		ret := func(name string, f *Func, as []*Value) {
			if len(f.Results) == 1 {
				r := s.call(name, f.Results[0], "", as...)
				s.check(name)
				s.retv(r)
				return
			}
			t := m.Tuple(f.Results...)
			r := s.call(name, t, "", as...)
			s.check(name)
			var es []*Value
			for i := range f.Results {
				es = append(es, s.add(OpField, t.Elems[i], i, r))
			}
			tuple(s.add(OpTuple, t, nil, es...))
		}
		mode := s.rt("test_fs_real", intT)
		s.ifThen(s.add(OpGe, boolT, nil, mode, s.cint(0)), func() {
			as := append([]*Value(nil), args...)
			if len(as) > 0 && isFs(as[0].Type) {
				f := as[0]
				old := s.add(OpField, intT, 1, f)
				as[0] = s.add(OpSetField, f.Type, 1, f, s.add(OpOr, intT, nil, old, mode))
			}
			ret(real.Name, real, as)
		})
		ret(memfs.Name, memfs, args)
	})
}

// procRun lowers `Proc.run` and a test's `FakeProc.run` (std.md 3.7,
// decision 0107): the native prelude's proc_run, which waits on the
// event loop (the program needs unwinding checks). ok is false for any
// other call.
func (s *fn) procRun(x *ast.CallExpr, ci *check.Call) (*Value, bool) {
	b := s.b
	fo := ci.Fn
	if fo.Recv == nil || fo.Static || fo.Name != "run" || fo.Module == nil {
		return nil, false
	}
	if !(fo.Module.IsPrelude && fo.Recv.Name == "Proc") && !(fo.Module.Path == "std/testing" && fo.Recv.Name == "FakeProc") {
		return nil, false
	}
	b.needConc = true
	if !b.conc {
		return s.undef(s.natOf(x)), true
	}
	if nm := b.nativeMod("std/prelude"); nm != nil {
		if impl, ok := nm.Scope["proc_run"].(*check.FuncObj); ok && impl.Recv == nil {
			return s.callAs(x, ci, impl), true
		}
	}
	return nil, false
}
