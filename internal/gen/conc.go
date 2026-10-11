package gen

import (
	"fmt"
	"sort"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// scopeInfo describes a `scope s` being generated.
type scopeInfo struct{}

func isStdMethod(fo *check.FuncObj, mod, typ, name string) bool {
	if fo == nil || fo.Recv == nil || fo.Name != name || fo.Recv.Name != typ {
		return false
	}
	m := fo.Recv.Module
	if mod == "prelude" {
		return m.IsPrelude
	}
	return m.IsStd && m.Path == "std/"+mod
}

// specialStd generates std calls that need more than the calling
// convention: spawning, `fault`, and `dbg`.
func (f *fnGen) specialStd(x *ast.CallExpr, ci *check.Call) (string, callKind, bool) {
	fo := ci.Fn
	r := f.rt()
	switch {
	case fo.Recv == nil && fo.Module.IsPrelude && fo.Name == "fault":
		return fmt.Sprintf("%sFault(tk, %s, %q)", r, f.val(x.Args[0].Value), f.g.loc(x)), ckCall, true
	case fo.Recv == nil && fo.Module.IsPrelude && fo.Name == "dbg":
		a := x.Args[0].Value
		return fmt.Sprintf("%sDbg(tk, %s, %q, %q)", r, f.val(a), f.g.source(a), f.g.loc(x)), ckCall, true
	case isStdMethod(fo, "prelude", "Scope", "spawn"), isStdMethod(fo, "prelude", "Scope", "spawn_background"):
		bg := fo.Name == "spawn_background"
		codes := f.operands(2, func(i int) string {
			if i == 0 {
				return f.val(ci.Recv)
			}
			return f.val(x.Args[0].Value)
		})
		return fmt.Sprintf("%sSpawn(tk, %s, %s, %v, %v)", r, codes[0], codes[1], bg, f.consumed(x)), ckCall, true
	case isStdMethod(fo, "task", "Group", "spawn"):
		codes := f.operands(2, func(i int) string {
			if i == 0 {
				return f.val(ci.Recv)
			}
			return f.val(x.Args[0].Value)
		})
		return fmt.Sprintf("%sGroupSpawn(tk, %s, %s, %v)", r, codes[0], codes[1], f.consumed(x)), ckCall, true
	case isStdMethod(fo, "task", "Group", "spawn_background"):
		codes := f.operands(2, func(i int) string {
			if i == 0 {
				return f.val(ci.Recv)
			}
			return f.val(x.Args[0].Value)
		})
		return fmt.Sprintf("%sGroupSpawnBackground(tk, %s, %s)", r, codes[0], codes[1]), ckVoid, true
	}
	return "", ckValue, false
}

// consumed reports whether the task started by call x has its handle bound
// with let and awaited, so its error does not fail the scope (core 7.1).
func (f *fnGen) consumed(x *ast.CallExpr) bool {
	v := f.boundTo[x]
	return v != nil && v.Awaited()
}

// noteBinding records the binding a spawn call's handle is stored in.
func (f *fnGen) noteBinding(e ast.Expr, v *check.VarObj) {
	if call, ok := e.(*ast.CallExpr); ok && v != nil {
		f.boundTo[call] = v
	}
}

// scopeExpr generates `scope s { body }` (core 7.1): the body runs in a
// block closure whose deferred Close applies the exit rules (cancel
// background tasks or all tasks, wait for every task, propagate a child's
// fault or error).
func (f *fnGen) scopeExpr(x *ast.ScopeExpr, d dest) {
	r := f.rt()
	sv := f.tmp("scope")
	f.line("%s := %sNewScope(tk)", sv, r)
	if hv := f.info.VarOf(x.Name); hv != nil {
		f.names[hv] = sv
	}
	fr := f.fr
	ctl := f.tmp("ctl")
	done := f.tmp("done")
	set := &ctlSet{codes: map[int]func(){}}
	fr.iife++
	fr.ctl = append(fr.ctl, set)
	body := f.capture(func() {
		f.ind++
		f.line("tk := %s.Body()", sv)
		f.line("_ = tk")
		f.line("%s := false", done)
		f.line("defer func() { %sr = %s.Close(recover(), %s, %sr) }()", ctl, sv, done, ctl)
		dd := d
		dd.pre = done + " = true"
		f.stmts(x.Body, dd, true)
		if !endsTerminating(f.out.String(), f.ind) {
			f.line("%s = true", done)
			f.line("return 0")
		}
		f.ind--
	})
	fr.ctl = fr.ctl[:len(fr.ctl)-1]
	fr.iife--
	f.line("%s := func() (%sr int) {", ctl, ctl)
	f.out.WriteString(body)
	f.line("}()")
	f.line("if %s == %sCtlScopeFail {", ctl, r)
	f.ind++
	f.scopeFailure(sv, x)
	f.ind--
	f.line("}")
	f.afterIIFE(ctl, set)
}

// scopeFailure propagates the error of a failed child task out of the
// scope, converted to the enclosing function's error type (core 7.1).
func (f *fnGen) scopeFailure(sv string, x *ast.ScopeExpr) {
	r := f.rt()
	fr := f.fr
	fe, ftr := f.tmp("fe"), f.tmp("ftr")
	if fr.errT == nil || !fr.isResult {
		// the trace is unused: the failure becomes a fault
		f.line("%s, _ := %s.Failure()", fe, sv)
		f.line("panic(%sAsFault(%sDisplay(tk, %s)))", r, r, fe)
		return
	}
	// error types of the tasks started in this scope
	types := map[string]check.Type{}
	inspectNode(x.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ci := f.info.CallOf(call)
		if ci == nil || ci.Fn == nil {
			return true
		}
		fo := ci.Fn
		if isStdMethod(fo, "prelude", "Scope", "spawn") || isStdMethod(fo, "prelude", "Scope", "spawn_background") || isStdMethod(fo, "task", "Group", "spawn") || isStdMethod(fo, "task", "Group", "spawn_background") {
			ft, ok := check.Resolve(f.typeOf(call.Args[0].Value)).(*check.Func)
			if ok && ft.Err != nil && !check.IsNever(ft.Err) {
				types[f.goType(ft.Err)] = ft.Err
			}
		}
		return true
	})
	var keys []string
	for k := range types {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ii, jj := f.g.isInterface(types[keys[i]]), f.g.isInterface(types[keys[j]])
		if ii != jj {
			return jj
		}
		return keys[i] < keys[j]
	})
	if len(keys) == 0 && !f.g.isPrelude(fr.errT, "Error") {
		// no task of the scope fails with an error (only a fault fails
		// it): the trace is unused
		f.line("%s, _ := %s.Failure()", fe, sv)
		f.line("panic(%sAsFault(%sDisplay(tk, %s)))", r, r, fe)
		return
	}
	f.line("%s, %s := %s.Failure()", fe, ftr, sv)
	ev := f.tmp("e")
	sp := x.Span()
	loc := fmt.Sprintf("%s:%d", f.g.linePath(sp.File), sp.Start.Line)
	tr := f.frameCode(ftr, loc)
	f.line("switch %s := %s.(type) {", ev, fe)
	for _, k := range keys {
		f.line("case %s:", k)
		f.ind++
		conv := f.coerceErr(ev, types[k], fr.errT)
		f.emitReturnValue(fmt.Sprintf("%sErrTr[%s](%s, %s)", r, f.goType(fr.resT), conv, tr))
		f.ind--
	}
	f.line("default:")
	f.ind++
	f.line("_ = %s", ev)
	if f.g.isPrelude(fr.errT, "Error") {
		f.emitReturnValue(fmt.Sprintf("%sErrTr[%s](%sIface[%sIError](%s), %s)", r, f.goType(fr.resT), r, r, fe, tr))
	} else {
		f.line("panic(%sAsFault(%sDisplay(tk, %s)))", r, r, fe)
	}
	f.ind--
	f.line("}")
}

// selectExpr generates `select { arms }` (core 7.3).
func (f *fnGen) selectExpr(x *ast.SelectExpr, d dest) {
	r := f.rt()
	sel := f.tmp("sel")
	f.line("%s := %sNewSelect(tk)", sel, r)
	slots := make([]string, len(x.Arms))
	for i, arm := range x.Arms {
		switch {
		case arm.After != nil:
			f.line("%sSelAfter(%s, %s)", r, sel, f.val(arm.After))
		default:
			call, ok := arm.Op.(*ast.CallExpr)
			var se *ast.SelectorExpr
			if ok {
				se, _ = call.Fun.(*ast.SelectorExpr)
			}
			if se == nil {
				f.g.errorf(arm.Op, "a select arm must be `ch.recv()`, `ch.send(x)`, or `after d`")
				continue
			}
			ch := f.val(se.X)
			switch se.Sel.Name {
			case "recv":
				if arm.Bind == nil {
					// `ch.recv() => body`: the value is not bound, so
					// no slot variable (Go rejects an unused one)
					f.line("%sSelRecv(%s, %s)", r, sel, ch)
					break
				}
				slots[i] = f.tmp("rv")
				f.line("%s := %sSelRecv(%s, %s)", slots[i], r, sel, ch)
			case "send":
				v := f.val(call.Args[0].Value)
				f.line("%sSelSend(%s, %s, %s)", r, sel, ch, v)
			default:
				f.g.errorf(arm.Op, "a select arm must be `ch.recv()`, `ch.send(x)`, or `after d`")
			}
		}
	}
	f.line("switch %s.Wait() {", sel)
	for i, arm := range x.Arms {
		f.line("case %d:", i)
		f.ind++
		if arm.Bind != nil && slots[i] != "" {
			if v := f.info.VarOf(arm.Bind); v != nil {
				name := f.declName(v)
				f.line("%s := *%s", name, slots[i])
				f.line("_ = %s", name)
			}
		}
		f.armBody(arm.Body, d)
		f.ind--
	}
	f.line("}")
}
