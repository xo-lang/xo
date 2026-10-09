package rt

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

// Error frames (core 11.1, decision 0092). Each `?` that propagates an
// error appends a frame to the error's Trace. A frame keeps the function,
// the location, and the parameter values themselves; their Debug text is
// produced only when the trace is printed (Lines: main's error report,
// crash bundles, test failures, handler error logs), and that formatting
// stops after 65 bytes per value, since a frame shows at most 64.
//
// Values are captured by copy: every Xo value is immutable or copied on
// write (Lists are marked shared by the generated code before capture, a
// Map is a persistent log, Str and Bytes are immutable), so formatting
// later prints what formatting at capture time would have. The generated
// code formats eagerly (FrameTextOf) the few types whose Debug text can
// change in place: a Buf, which `freeze` turns into an unshared List, and
// an Atomic, whose Debug text is its current value.

// Trace is a persistent list of error frames, newest last.
type Trace struct {
	Prev *Trace
	Fn   string // the function: `users.fetch_user`
	Loc  string // `users/fetch.xo:18`
	// Names are the parameter names, separated by spaces ("" when the
	// function has none, capabilities left out).
	Names string
	// Args are the captured values in Names order; nil when parameters
	// are not recorded (`--error-args=false`), shown as `(...)`.
	Args frameArgs
}

// frameArgs are the parameter values of one frame.
type frameArgs interface{ frameArg(i int) any }

// frameArgMax is the most bytes of Debug text a frame shows per value.
const frameArgMax = 64

// Frame0 appends a frame without parameter values: the function has no
// parameters (names is ""), or a release build does not record them.
func Frame0(prev *Trace, fn, loc, names string) *Trace {
	return &Trace{Prev: prev, Fn: fn, Loc: loc, Names: names}
}

// The frames of functions with one to six parameters keep the values in
// the same allocation as the frame.

type frame1[A any] struct {
	t Trace
	a A
}

func (f *frame1[A]) frameArg(int) any { return f.a }

// Frame1 appends a frame with one parameter value.
func Frame1[A any](prev *Trace, fn, loc, names string, a A) *Trace {
	f := &frame1[A]{t: Trace{Prev: prev, Fn: fn, Loc: loc, Names: names}, a: a}
	f.t.Args = f
	return &f.t
}

type frame2[A, B any] struct {
	t Trace
	a A
	b B
}

func (f *frame2[A, B]) frameArg(i int) any {
	if i == 0 {
		return f.a
	}
	return f.b
}

// Frame2 appends a frame with two parameter values.
func Frame2[A, B any](prev *Trace, fn, loc, names string, a A, b B) *Trace {
	f := &frame2[A, B]{t: Trace{Prev: prev, Fn: fn, Loc: loc, Names: names}, a: a, b: b}
	f.t.Args = f
	return &f.t
}

type frame3[A, B, C any] struct {
	t Trace
	a A
	b B
	c C
}

func (f *frame3[A, B, C]) frameArg(i int) any {
	switch i {
	case 0:
		return f.a
	case 1:
		return f.b
	}
	return f.c
}

// Frame3 appends a frame with three parameter values.
func Frame3[A, B, C any](prev *Trace, fn, loc, names string, a A, b B, c C) *Trace {
	f := &frame3[A, B, C]{t: Trace{Prev: prev, Fn: fn, Loc: loc, Names: names}, a: a, b: b, c: c}
	f.t.Args = f
	return &f.t
}

type frame4[A, B, C, D any] struct {
	t Trace
	a A
	b B
	c C
	d D
}

func (f *frame4[A, B, C, D]) frameArg(i int) any {
	switch i {
	case 0:
		return f.a
	case 1:
		return f.b
	case 2:
		return f.c
	}
	return f.d
}

// Frame4 appends a frame with four parameter values.
func Frame4[A, B, C, D any](prev *Trace, fn, loc, names string, a A, b B, c C, d D) *Trace {
	f := &frame4[A, B, C, D]{t: Trace{Prev: prev, Fn: fn, Loc: loc, Names: names}, a: a, b: b, c: c, d: d}
	f.t.Args = f
	return &f.t
}

type frame5[A, B, C, D, E any] struct {
	t Trace
	a A
	b B
	c C
	d D
	e E
}

func (f *frame5[A, B, C, D, E]) frameArg(i int) any {
	switch i {
	case 0:
		return f.a
	case 1:
		return f.b
	case 2:
		return f.c
	case 3:
		return f.d
	}
	return f.e
}

// Frame5 appends a frame with five parameter values.
func Frame5[A, B, C, D, E any](prev *Trace, fn, loc, names string, a A, b B, c C, d D, e E) *Trace {
	f := &frame5[A, B, C, D, E]{t: Trace{Prev: prev, Fn: fn, Loc: loc, Names: names}, a: a, b: b, c: c, d: d, e: e}
	f.t.Args = f
	return &f.t
}

type frame6[A, B, C, D, E, F any] struct {
	t Trace
	a A
	b B
	c C
	d D
	e E
	f F
}

func (f *frame6[A, B, C, D, E, F]) frameArg(i int) any {
	switch i {
	case 0:
		return f.a
	case 1:
		return f.b
	case 2:
		return f.c
	case 3:
		return f.d
	case 4:
		return f.e
	}
	return f.f
}

// Frame6 appends a frame with six parameter values.
func Frame6[A, B, C, D, E, F any](prev *Trace, fn, loc, names string, a A, b B, c C, d D, e E, f F) *Trace {
	fr := &frame6[A, B, C, D, E, F]{t: Trace{Prev: prev, Fn: fn, Loc: loc, Names: names}, a: a, b: b, c: c, d: d, e: e, f: f}
	fr.t.Args = fr
	return &fr.t
}

// frameN holds the values of a function with more than six parameters.
type frameN []any

func (f frameN) frameArg(i int) any { return f[i] }

// FrameN appends a frame with any number of parameter values.
func FrameN(prev *Trace, fn, loc, names string, args ...any) *Trace {
	return &Trace{Prev: prev, Fn: fn, Loc: loc, Names: names, Args: frameN(args)}
}

// FrameText is a parameter value formatted when its frame was captured
// (FrameTextOf): its Debug text is the text itself.
type FrameText string

// XoDebug implements Debug.
func (t FrameText) XoDebug(b *strings.Builder) { b.WriteString(string(t)) }

// FrameTextOf formats v for a frame now, bounded like a frame's text: for
// the values whose Debug text may change after capture (see the top of
// this file).
func FrameTextOf(v any) FrameText { return FrameText(debugBounded(v, frameArgMax+1)) }

// Label is the frame's function and parameters as printed:
// `users.fetch_user(id=42, name="ada")`.
func (t *Trace) Label() string {
	var b strings.Builder
	b.WriteString(t.Fn)
	b.WriteByte('(')
	if t.Names != "" {
		if t.Args == nil {
			b.WriteString("...")
		} else {
			for i, n := range strings.Fields(t.Names) {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(n)
				b.WriteByte('=')
				b.WriteString(frameDebug(t.Args.frameArg(i)))
			}
		}
	}
	b.WriteByte(')')
	return b.String()
}

// Lines returns the frames innermost first, formatted as in core 11.1.
func (t *Trace) Lines() []string {
	var out []string
	for f := t; f != nil; f = f.Prev {
		out = append(out, fmt.Sprintf("  at %-34s %s", f.Label(), f.Loc))
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// frameDebug is the Debug text of a frame's value, cut to 64 bytes
// (61 and `...`).
func frameDebug(v any) string {
	s := debugBounded(v, frameArgMax+1)
	if len(s) > frameArgMax {
		s = s[:frameArgMax-3] + "..."
	}
	return s
}

// ---------------------------------------------------------------------------
// Bounded Debug formatting
//
// debugBounded formats like Debug but stops once the text is at least
// limit bytes long: every value is formatted through debugTo, which checks
// the length of a bounded builder on entry and panics with debugStop past
// the limit, and long strings and byte strings are quoted only as far as
// needed. Everything written is a prefix of the full Debug text, so a cut
// of the result equals the same cut of the full text.

// debugStop ends a bounded format early.
type debugStop struct{}

// boundedDebug is a builder being formatted with a limit.
type boundedDebug struct {
	b     *strings.Builder
	limit int
}

var (
	boundedMu sync.Mutex
	// boundedActive are the bounded formats in progress (copied on
	// write, so debugTo reads it with one atomic load).
	boundedActive atomic.Pointer[[]*boundedDebug]
)

// boundedFor returns the limit of b when b is being formatted bounded.
func boundedFor(b *strings.Builder) *boundedDebug {
	p := boundedActive.Load()
	if p == nil {
		return nil
	}
	for _, x := range *p {
		if x.b == b {
			return x
		}
	}
	return nil
}

func setBounded(add, remove *boundedDebug) {
	boundedMu.Lock()
	defer boundedMu.Unlock()
	var next []*boundedDebug
	if p := boundedActive.Load(); p != nil {
		for _, x := range *p {
			if x != remove {
				next = append(next, x)
			}
		}
	}
	if add != nil {
		next = append(next, add)
	}
	if len(next) == 0 {
		boundedActive.Store(nil)
		return
	}
	boundedActive.Store(&next)
}

// debugBounded is the Debug text of v, or a prefix of it at least limit
// bytes long.
func debugBounded(v any, limit int) (s string) {
	b := &strings.Builder{}
	bd := &boundedDebug{b: b, limit: limit}
	setBounded(bd, nil)
	defer func() {
		setBounded(nil, bd)
		if r := recover(); r != nil {
			if _, ok := r.(debugStop); !ok {
				panic(r)
			}
		}
		s = b.String()
	}()
	debugTo(b, v)
	return ""
}

// checkBound stops a bounded format of b once it reached its limit.
func checkBound(b *strings.Builder) {
	if bd := boundedFor(b); bd != nil && b.Len() >= bd.limit {
		panic(debugStop{})
	}
}

// writeQuoted writes quote(s); in a bounded format only as much of it as
// the limit needs, then stops the format.
func writeQuoted(b *strings.Builder, s string) {
	bd := boundedFor(b)
	if bd == nil {
		b.WriteString(quote(s))
		return
	}
	room := bd.limit - b.Len()
	// every rune of k bytes quotes to at least k bytes
	k := 0
	for k < len(s) && k <= room {
		_, w := utf8.DecodeRuneInString(s[k:])
		k += w
	}
	if k >= len(s) {
		b.WriteString(quote(s))
		return
	}
	q := quote(s[:k])
	b.WriteString(q[:len(q)-1])
	panic(debugStop{})
}
