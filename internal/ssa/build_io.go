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
//     checker keeps the two apart), Clock, Net, Proc, and Env are
//     capabilities (zero size), Listener and Conn are handles.
//   - `clock.sleep` is a cancellation point that unwinds; the network
//     operations report cancellation as `NetErr.Canceled` (core 7.2). A
//     network call records its outcome in the runtime (rt.net_status,
//     rt.net_msg), from which the Result is built (netResult).
//   - Every operation that can wait makes the program build with
//     unwinding checks (needConc), so a shutdown signal can unwind it.

// statusTags are the runtime's status codes of NetErr's variants (the Go
// backend's tags).
var statusTags = map[string]int64{"InvalidAddr": 1, "Refused": 2, "Timeout": 3, "Io": 4, "Canceled": 5}

// ioCall lowers a prelude method of Clock, Net, Listener, Conn, Proc,
// Env, Duration, Time, Str, or Bytes that the event loop or inline code
// implements; ok is false for anything else.
func (s *fn) ioCall(x *ast.CallExpr, ci *check.Call) (*Value, bool) {
	b := s.b
	fo := ci.Fn
	if fo.Module == nil || !fo.Module.IsPrelude || fo.Recv == nil {
		return nil, false
	}
	m := b.mod
	intT, boolT, strT, floatT := m.Builtin(KInt), m.Builtin(KBool), m.Builtin(KStr), m.Builtin(KFloat)
	key := fo.Recv.Name + "." + fo.Name
	switch fo.Recv.Name {
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
	case "Clock.sleep", "Net.listen", "Net.dial", "Listener.accept", "Conn.read", "Conn.write":
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
	case "Proc.set_grace":
		s.rt("set_grace", nil, arg(0, intT))
		return s.unit(), true
	case "Env.get":
		return s.popOne(s.rt("env_get", m.List(strT), arg(0, strT)), nt), true
	case "Net.listen":
		h := s.rt("net_listen", m.Handle("Listener"), arg(0, strT))
		return s.netResult(h, nt), true
	case "Net.dial":
		h := s.rt("net_dial", m.Handle("Conn"), arg(0, strT))
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
	r := s.add(OpCallValue, rt, nil, fv)
	u := s.rt("unwinding", boolT)
	join := s.newBlock()
	res := join.AddParam(nt)
	ei := nt.VariantIndex("Err")
	timedT := nt.Variants[ei].Fields[0].Type
	unw, done := s.branch(u)
	s.b.checks = append(s.b.checks, unwindCheck{s.f, u, ""})
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
