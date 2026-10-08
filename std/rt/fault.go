package rt

import (
	"fmt"
	"runtime"
	"strings"
)

// FaultErr is a fault (core 8): a bug, not an error. It travels as a Go
// panic until a task boundary, `task.isolate`, an http handler, the test
// runner, or main.
type FaultErr struct {
	Msg    string // the message with evaluated values
	Loc    string // file.xo:line:col (or file.xo:line when taken from the stack)
	Code   string // XO-Fnnn for runtime faults with a code, else ""
	Stack  string // Go stack at the fault (frames carry .xo positions)
	Detail string // more lines: the values of a failed contract
}

func (f *FaultErr) Error() string {
	s := f.Msg
	if f.Code != "" {
		s = f.Code + ": " + s
	}
	if f.Loc != "" {
		s += " at " + f.Loc
	}
	if f.Detail != "" {
		s += "\n" + f.Detail
	}
	return s
}

// M_display is the one line text of the fault: `msg at file:line:col`.
func (f *FaultErr) M_display(*Task) string { return f.Error() }

// Faultf raises a fault at loc. An empty loc is replaced by the innermost
// .xo position on the stack.
func Faultf(loc, format string, args ...any) {
	panic(newFault("", loc, fmt.Sprintf(format, args...)))
}

// FaultHere raises a fault located at the innermost .xo frame of the stack
// (the generated code's //line directives give the position).
func FaultHere(format string, args ...any) {
	panic(newFault("", "", fmt.Sprintf(format, args...)))
}

// FaultCode raises a fault with a stable code (XO-F004, XO-F005).
func FaultCode(code, loc, format string, args ...any) {
	panic(newFault(code, loc, fmt.Sprintf(format, args...)))
}

func newFault(code, loc, msg string) *FaultErr {
	f := &FaultErr{Msg: msg, Loc: loc, Code: code, Stack: stack(3)}
	if f.Loc == "" {
		f.Loc = xoLocation(3)
	}
	return f
}

// xoLocation returns `file.xo:line` of the innermost caller frame whose
// file is an Xo source.
func xoLocation(skip int) string {
	pcs := make([]uintptr, 64)
	n := runtime.Callers(skip+1, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if strings.HasSuffix(f.File, ".xo") {
			return fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		if !more {
			return ""
		}
	}
}

// stack returns the Xo frames of the current goroutine's Go stack: thanks
// to //line directives they name .xo files and lines (core 11.5); runtime,
// rt, and entry frames are left out.
func stack(skip int) string {
	pcs := make([]uintptr, 64)
	n := runtime.Callers(skip+1, pcs)
	// only Xo frames: generated code maps every line to a .xo file
	return framesText(pcs[:n])
}

// panicStack is stack for a fault raised as a panic of a plain value
// (arithFault) and converted when recovered: called from a deferred
// function while the panic is in flight, it keeps the frames below the
// innermost runtime.gopanic, the stack of the panicking operation. It
// keeps as many frames as a fault raised in place by an out of line call
// would (stack(3) in newFault starts two rt frames above the operation).
func panicStack() string {
	pcs := make([]uintptr, 128)
	n := runtime.Callers(1, pcs)
	pcs = pcs[:n]
	for i, pc := range pcs {
		if fn := runtime.FuncForPC(pc - 1); fn != nil && fn.Name() == "runtime.gopanic" {
			pcs = pcs[i+1:]
			if len(pcs) > 62 {
				pcs = pcs[:62]
			}
			return framesText(pcs)
		}
	}
	return stack(2)
}

// framesText formats the Xo frames of pcs as stack does.
func framesText(pcs []uintptr) string {
	frames := runtime.CallersFrames(pcs)
	var b strings.Builder
	for {
		f, more := frames.Next()
		if strings.HasSuffix(f.File, ".xo") && !isGlue(f.Function) {
			fmt.Fprintf(&b, "  %s\n      %s:%d\n", strings.TrimPrefix(f.Function, "xoprog/m/"), f.File, f.Line)
		}
		if !more {
			break
		}
	}
	return b.String()
}

// isGlue reports generated test glue: contract fuzz cases and the test
// table's closures, whose positions are not meaningful.
func isGlue(name string) bool {
	return strings.Contains(name, ".FZ_") || strings.Contains(name, ".init.func")
}

// Fault is the predeclared `fault(msg)` (core 8). It never returns.
func Fault(_ *Task, msg, loc string) Never {
	panic(newFault("", loc, msg))
}

// Unreachable marks code the checker proved unreachable (exhaustive match).
func Unreachable(loc string) {
	panic(newFault("", loc, "unreachable code reached (internal compiler error)"))
}

// Hole is an evaluated `???` under `--holes` (core 3.7).
func Hole[T any](loc string) T {
	panic(newFault("", loc, "evaluated a hole `???`"))
}

// AsFault converts a recovered panic value to a fault. Go runtime errors
// (an rt bug or a stack overflow) become faults too.
func AsFault(r any) *FaultErr {
	switch x := r.(type) {
	case *FaultErr:
		return x
	case *arithFault:
		return x.fault()
	case runtime.Error:
		return &FaultErr{Msg: "internal fault: " + x.Error(), Loc: xoLocation(3), Stack: stack(3)}
	case error:
		return &FaultErr{Msg: x.Error(), Loc: xoLocation(3), Stack: stack(3)}
	}
	return &FaultErr{Msg: fmt.Sprint(r), Loc: xoLocation(3), Stack: stack(3)}
}

// Require checks a `requires` contract (core 9.1).
func Require(ok bool, text, loc string, vals ...any) {
	if !ok {
		f := newFault("", loc, "requires failed: "+text)
		f.Detail = contractVals(vals)
		panic(f)
	}
}

// Ensure checks an `ensures` contract (core 9.1).
func Ensure(ok bool, text, loc string, vals ...any) {
	if !ok {
		f := newFault("", loc, "ensures failed: "+text)
		f.Detail = contractVals(vals)
		panic(f)
	}
}

// Invariant checks a struct invariant (core 9.1).
func Invariant(ok bool, typ, text, loc string) {
	if !ok {
		panic(newFault("", loc, "invariant of "+typ+" failed: "+text))
	}
}

// contractVals formats (name, value) pairs printed under a failed contract.
func contractVals(vals []any) string {
	if len(vals) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(" ")
	for i := 0; i+1 < len(vals); i += 2 {
		fmt.Fprintf(&b, "  %s = %s", vals[i], Debug(vals[i+1]))
	}
	return b.String()
}

// OrFault is the shared body of `or_fault` on Option and Result.
func orFault(msg string) {
	panic(newFault("", "", msg))
}
