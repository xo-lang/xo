package ssa

import (
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Error frames on the native backends (decisions 0092 and 0104).
//
// The Err variant of every Result has a last, hidden field: the error's
// trace, the struct xo.trace {frames: List[(Str, Str, fn() -> Str)]}
// with one frame per `?` that propagated the error, innermost first:
// the function's name (`users.fetch_user`), the location of the `?`
// (`main.xo:12`), and a closure over the parameters' values at the `?`
// that formats them as `id=42, name="ada"` (each value's Debug text cut
// to 64 bytes) when the trace is printed. A new error (any other Err)
// has no frames. The trace is not part of the error's value: `==`,
// hashing, and order skip it, and Debug text never shows it.
//
// main's error report prints the frames after `error: <text>`, as std/rt
// does: `  at <label padded to 34> <location>`. Functions of std and of
// the native implementation modules record no frames (the Go backend's
// std is Go code).

// traceField is the name of the hidden trace field of Err.
const traceField = "xo.tr"

// traceType is xo.trace.
func (b *builder) traceType() *Type {
	m := b.mod
	t, created := m.Named("xo.trace", KStruct)
	if created {
		st := m.Builtin(KStr)
		frame := m.Tuple(st, st, m.FuncType(nil, st))
		t.Fields = []Field{{"frames", m.List(frame)}}
	}
	return t
}

func isTrace(t *Type) bool { return t.Kind == KStruct && t.Name == "xo.trace" }

// emptyTrace is the trace of a new error.
func (s *fn) emptyTrace() *Value {
	t := s.b.traceType()
	return s.add(OpStruct, t, nil, s.rt("rl_empty", t.Fields[0].Type))
}

// errTrace is the trace of the Err payload of r (tag errI), or a new one
// when r's type has none.
func (s *fn) errTrace(r *Value, errI int) *Value {
	fs := r.Type.Variants[errI].Fields
	if n := len(fs); n > 0 && fs[n-1].Name == traceField {
		return s.add(OpPayload, fs[n-1].Type, PayloadRef{errI, n - 1}, r)
	}
	return s.emptyTrace()
}

// frameOwner is the named function whose frame a `?` here records, or nil
// (std and native code record none).
func (s *fn) frameOwner() *check.FuncObj {
	fo := s.fo
	if fo == nil {
		fo = s.frameFo
	}
	if fo == nil || fo.Module == nil || fo.Module.IsStd || fo.Module.Native {
		return nil
	}
	return fo
}

// frameName is std/rt's frame name of fo: `module.fn` or
// `module.Type.method`.
func frameName(fo *check.FuncObj) string {
	if fo.Recv != nil {
		return fo.Module.Name + "." + fo.Recv.Name + "." + fo.Name
	}
	return fo.Module.Name + "." + fo.Name
}

// frameVars are the bindings a frame of fo shows: the receiver and the
// parameters, without capabilities.
func (b *builder) frameVars(fo *check.FuncObj) []*check.VarObj {
	var vars []*check.VarObj
	if fo.Recv != nil {
		vars = append(vars, b.info.RecvVar(fo))
	}
	vars = append(vars, b.info.ParamVars(fo)...)
	var out []*check.VarObj
	for _, vo := range vars {
		if vo != nil && !isCapabilityType(vo.Type) {
			out = append(out, vo)
		}
	}
	return out
}

// isCapabilityType reports a capability or a test's fake of one.
func isCapabilityType(t check.Type) bool {
	n, ok := check.Resolve(check.Deep(t)).(*check.Named)
	if !ok || n.Obj.Module == nil {
		return false
	}
	if n.Obj.Module.IsPrelude {
		switch n.Obj.Name {
		case "Os", "Stdio", "Clock", "Net", "Proc", "Env", "Fs", "Rand":
			return true
		}
	}
	if n.Obj.Module.Path == "std/testing" {
		_, ok := testCaps[n.Obj.Name]
		return ok
	}
	return false
}

// pushFrame is trace tr with the frame of the `?` at x added.
func (s *fn) pushFrame(tr *Value, x ast.Node) *Value {
	b := s.b
	fo := s.frameOwner()
	if fo == nil {
		return tr
	}
	m := b.mod
	st := m.Builtin(KStr)
	t := b.traceType()
	lt := t.Fields[0].Type
	ft := lt.Elems[0]
	var vals []*Value
	var names []string
	var cts []check.Type
	for _, vo := range b.frameVars(fo) {
		if _, ok := s.varType[vo]; !ok {
			continue
		}
		ct := s.apply(vo.Type)
		v := s.read(vo)
		if isPreludeNamed(ct, "Atomic") {
			// its Debug text changes in place: taken now, as std/rt's
			// FrameTextOf (decision 0092)
			v = s.appendDebug(s.add(OpConst, st, ""), v, ct, x)
			ct = nil
		}
		names = append(names, vo.Name)
		cts = append(cts, ct)
		vals = append(vals, v)
	}
	args := s.add(OpClosure, ft.Elems[2], b.frameArgsFunc(names, cts, vals, x), vals...)
	frame := s.add(OpTuple, ft, nil, s.add(OpConst, st, frameName(fo)), s.add(OpConst, st, b.posLine(x)), args)
	l := s.add(OpField, lt, 0, tr)
	return s.add(OpStruct, t, nil, s.call("list.push", lt, "", l, frame))
}

// frameArgsFunc returns the code of the closure that formats a frame's
// parameters (names, of checker types cts, captured as vals).
func (b *builder) frameArgsFunc(names []string, cts []check.Type, vals []*Value, at ast.Node) string {
	m := b.mod
	st, intT := m.Builtin(KStr), m.Builtin(KInt)
	var key []string
	var capTs []*Type
	for i, n := range names {
		if cts[i] == nil {
			key = append(key, n+": text")
		} else {
			key = append(key, n+": "+b.implNamed(cts[i]).String())
		}
		capTs = append(capTs, vals[i].Type)
	}
	name := "xo.frameargs(" + strings.Join(key, ", ") + ")"
	if m.Func(name) != nil {
		return name
	}
	ft := m.FuncType(nil, st)
	f := m.NewFunc(name, []*Type{ft}, []*Type{st})
	f.Indirect = true
	f.Captures = capTs
	s := b.newFn(f, nil, nil)
	env := f.Params()[0]
	acc := s.add(OpConst, st, "")
	for i, n := range names {
		sep := n + "="
		if i > 0 {
			sep = ", " + sep
		}
		acc = s.call("str.concat", st, "", acc, s.add(OpConst, st, sep))
		v := s.add(OpCapture, capTs[i], i, env)
		text := v
		if cts[i] != nil {
			text = s.appendDebug(s.add(OpConst, st, ""), v, cts[i], at)
		}
		// 64 bytes at most: 61 and `...` (std/rt frameDebug)
		cut := local("cut")
		s.writeVar(cut, text)
		long := s.add(OpGt, m.Builtin(KBool), nil, s.call("Str.len", intT, "", text), s.cint(64))
		s.ifThen(long, func() {
			short := s.rt("str_slice", st, text, s.cint(0), s.cint(61))
			s.writeVar(cut, s.call("str.concat", st, "", short, s.add(OpConst, st, "...")))
		})
		acc = s.call("str.concat", st, "", acc, s.read(cut))
	}
	s.retv(acc)
	s.finishFn()
	return name
}

// printTrace writes the frames of trace tr to standard error, one line
// each, innermost first: `  at <label padded to 34> <location>`.
func (s *fn) printTrace(tr *Value) {
	m := s.b.mod
	st, intT, boolT := m.Builtin(KStr), m.Builtin(KInt), m.Builtin(KBool)
	lt := tr.Type.Fields[0].Type
	ft := lt.Elems[0]
	l := s.add(OpField, lt, 0, tr)
	s.forUpTo(s.call("list.len", intT, "", l), func(i *Value) {
		fr := s.call("list.index", ft, "", l, i)
		args := s.add(OpCallValue, st, nil, s.add(OpField, ft.Elems[2], 2, fr))
		s.check("")
		label := s.call("str.concat", st, "", s.add(OpField, st, 0, fr), s.add(OpConst, st, "("))
		label = s.call("str.concat", st, "", label, args)
		label = s.call("str.concat", st, "", label, s.add(OpConst, st, ")"))
		line := local("line")
		s.writeVar(line, s.call("str.concat", st, "", s.add(OpConst, st, "  at "), label))
		// fmt's %-34s pads to 34 runes
		pad := s.add(OpSub, intT, nil, s.cint(34), s.rt("str_rune_count", intT, label))
		s.ifThen(s.add(OpGt, boolT, nil, pad, s.cint(0)), func() {
			s.writeVar(line, s.call("str.concat", st, "", s.read(line), s.rt("str_repeat", st, s.add(OpConst, st, " "), pad)))
		})
		out := s.call("str.concat", st, "", s.read(line), s.add(OpConst, st, " "))
		s.rt("eprintln", nil, s.call("str.concat", st, "", out, s.add(OpField, st, 1, fr)))
	})
}

// userFields are the fields of variant v without the hidden trace of
// Err (the fields a program writes).
func userFields(v *Variant) []Field {
	if n := len(v.Fields); n > 0 && v.Fields[n-1].Name == traceField {
		return v.Fields[:n-1]
	}
	return v.Fields
}
