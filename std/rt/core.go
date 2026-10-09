// Package rt is the runtime linked into every program the Xo compiler
// builds (P1, decision 0002). Generated Go code imports it for checked
// arithmetic, persistent collections, Option and Result, faults, error
// frames, scopes and cancellation, capabilities, and the standard library.
//
// Calling convention shared with internal/gen (knowledgebase
// infrastructure/toolchain.md, "Code generation"):
//
//   - Every Xo function, method, and function value takes the current *Task
//     first. The task carries cancellation, the shield of `defer`, held
//     mutexes, and the test world.
//   - Methods of non scalar types are Go methods `M_<name>`. Methods of the
//     scalar prelude types (Int, Float, Str, Bytes, Rune, Bool, Duration,
//     Time, sized numbers), methods with their own type parameters, and
//     methods restricted to one instantiation (`List[Str].join`) are top
//     level functions `<Type>_<name>(tk, recv, args...)`. Associated
//     functions are `<Type>_<name>(tk, args...)`.
//   - Std types outside the prelude are named `<Module>_<Name>`
//     (`Http_Request`, `Task_Timed`); std functions `<Module>_<name>`.
//   - Struct fields are `F_<name>`; enums are a Tag (1 based variant index)
//     and a payload P holding a `<Enum>_<Variant>` struct.
//   - A function value of Xo type `fn(A) -> R ! E` is a Go
//     `func(*Task, A) Result[R, E]`; one that cannot fail uses E = Never.
package rt

import (
	"fmt"
	"strings"
)

// Unit is the Xo type `()`.
type Unit struct{}

// Never is the Xo type with no values. It appears as a type argument, for
// example the error type of a function value that cannot fail.
type Never struct{}

// M_display makes Never satisfy Display and Error (core 2.4).
func (Never) M_display(*Task) string { return "never" }

// M_debug implements Debug.
func (Never) M_debug(*Task) string { return "never" }

// M_status makes Never satisfy http.StatusError.
func (Never) M_status(*Task) int64 { return 500 }

// Absurd converts a value of type Never to any type. It is only reached in
// generated code after an expression of type Never, which never completes.
func Absurd[T any](Never) T { panic("rt: a value of type Never was produced") }

// Zero returns the zero value of T.
func Zero[T any]() T {
	var z T
	return z
}

// Option is `T?`: Some(V) when Some is true, else None. The zero value is
// None.
type Option[T any] struct {
	Some bool
	V    T
}

// Some wraps v.
func Some[T any](v T) Option[T] { return Option[T]{Some: true, V: v} }

// None returns an empty optional.
func None[T any]() Option[T] { return Option[T]{} }

// Result is `Result[T, E]`, the return value of a function `-> T ! E`. Tr
// holds the error frames appended by each `?` that propagated it (core
// 11.1).
type Result[T, E any] struct {
	Ok bool
	V  T
	E  E
	Tr *Trace
}

// Ok wraps a success value.
func Ok[T, E any](v T) Result[T, E] { return Result[T, E]{Ok: true, V: v} }

// Err wraps an error.
func Err[T, E any](e E) Result[T, E] { return Result[T, E]{E: e} }

// ErrTr wraps an error that keeps the frames of an earlier Result.
func ErrTr[T, E any](e E, tr *Trace) Result[T, E] { return Result[T, E]{E: e, Tr: tr} }

// Trace is a persistent list of error frames, newest last.
type Trace struct {
	Prev *Trace
	Fn   string // function name and arguments: `users.fetch_user(id=42)`
	Loc  string // `users/fetch.xo:18`
}

// Frame appends a frame for a `?` in function fn at loc.
func (t *Trace) Frame(fn, loc string) *Trace { return &Trace{Prev: t, Fn: fn, Loc: loc} }

// Lines returns the frames innermost first, formatted as in core 11.1.
func (t *Trace) Lines() []string {
	var out []string
	for f := t; f != nil; f = f.Prev {
		out = append(out, fmt.Sprintf("  at %-34s %s", f.Fn, f.Loc))
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// FrameLabel formats a frame label from a function name and argument pairs
// (name, value); each value is its Debug text truncated to 64 bytes (core
// 11.1).
func FrameLabel(fn string, args ...any) string {
	var b strings.Builder
	b.WriteString(fn)
	b.WriteByte('(')
	for i := 0; i+1 < len(args); i += 2 {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(args[i].(string))
		b.WriteByte('=')
		s := Debug(args[i+1])
		if len(s) > 64 {
			s = s[:61] + "..."
		}
		b.WriteString(s)
	}
	b.WriteByte(')')
	return b.String()
}

// Tuple2 to Tuple6 are tuples `(A, B, ...)`.
type Tuple2[A, B any] struct {
	F0 A
	F1 B
}

// Tuple3 is a three element tuple.
type Tuple3[A, B, C any] struct {
	F0 A
	F1 B
	F2 C
}

// Tuple4 is a four element tuple.
type Tuple4[A, B, C, D any] struct {
	F0 A
	F1 B
	F2 C
	F3 D
}

// Tuple5 is a five element tuple.
type Tuple5[A, B, C, D, E any] struct {
	F0 A
	F1 B
	F2 C
	F3 D
	F4 E
}

// Tuple6 is a six element tuple.
type Tuple6[A, B, C, D, E, F any] struct {
	F0 A
	F1 B
	F2 C
	F3 D
	F4 E
	F5 F
}

// Secret is `Secret[T]`: redacted in every output path.
type Secret[T any] struct{ v T }

// Secret_new is `Secret[T].new(v)`.
// Under a recording or ring buffers, the value is redacted from traces
// (core 11.3).
func Secret_new[T any](_ *Task, v T) Secret[T] {
	if fxMode == fxRecord || fxMode == fxRing {
		fxSecret(any(v))
	}
	return Secret[T]{v: v}
}

// Secret_reveal returns the wrapped value.
func Secret_reveal[T any](_ *Task, s Secret[T]) T { return s.v }

// M_display implements Display.
func (s Secret[T]) M_display(*Task) string { return "<redacted>" }

// XoDebug implements Debug.
func (s Secret[T]) XoDebug(b *strings.Builder) { b.WriteString("<redacted>") }

// XoEq implements Eq.
func (s Secret[T]) XoEq(o any) bool { return Eq(s.v, o.(Secret[T]).v) }

// XoHash implements Hash.
func (s Secret[T]) XoHash() uint64 { return Hash(s.v) }

// Dbg is the predeclared `dbg(x)` (core 11.4): prints the source text and
// the Debug value of x to standard error and returns x.
func Dbg[T any](_ *Task, x T, src, loc string) T {
	stderrWrite(fmt.Sprintf("[%s] %s = %s\n", loc, src, Debug(x)))
	return x
}

// Iface converts v to the interface type I. Values whose Go type already
// implements I are converted directly; values whose Go methods differ from
// I's (a method that cannot fail where I's can) are wrapped in an adapter
// registered with RegAdapter; builtin values (Int, Str, List, ...) are
// boxed in Dyn so they carry Display and Debug methods (core 2.10).
func Iface[I any](v any) I {
	if i, ok := v.(I); ok {
		return i
	}
	if fs := adapters[any((*I)(nil))]; fs != nil {
		for _, f := range fs {
			if a, ok := f(v); ok {
				return a.(I)
			}
		}
	}
	if i, ok := any(Dyn{V: v}).(I); ok {
		return i
	}
	panic(fmt.Sprintf("rt: %T does not implement the interface", v))
}

// adapters holds, per interface type I (keyed by a nil *I), constructors
// that wrap a value in a generated adapter implementing I. Written only by
// RegAdapter from the init of package main, so reads need no lock.
var adapters = map[any][]func(any) (any, bool){}

// RegAdapter registers f, which wraps a value whose Go type needs an
// adapter to implement I (or reports false). Generic Go code converts a
// type parameter to an interface with Iface, where the concrete type, and
// so the adapter generated for it, is unknown; the generator registers
// every adapter such a conversion can need (gen/adapt.go).
func RegAdapter[I any](f func(v any) (I, bool)) {
	k := any((*I)(nil))
	adapters[k] = append(adapters[k], func(v any) (any, bool) { return f(v) })
}

// Dyn boxes a builtin value so it can be stored in an interface type such
// as Error, Display, or Debug (core 2.10).
type Dyn struct{ V any }

// M_display implements Display.
func (d Dyn) M_display(tk *Task) string { return Display(tk, d.V) }

// M_debug implements Debug.
func (d Dyn) M_debug(*Task) string { return Debug(d.V) }

// XoDebug implements Debug.
func (d Dyn) XoDebug(b *strings.Builder) { debugTo(b, d.V) }

// XoEq implements Eq.
func (d Dyn) XoEq(o any) bool {
	if p, ok := o.(Dyn); ok {
		return EqAny(d.V, p.V)
	}
	return EqAny(d.V, o)
}

// XoHash implements Hash.
func (d Dyn) XoHash() uint64 { return HashAny(d.V) }

// XoCmp implements Ord.
func (d Dyn) XoCmp(o any) int {
	if p, ok := o.(Dyn); ok {
		return CmpAny(d.V, p.V)
	}
	return CmpAny(d.V, o)
}

// Unbox returns the builtin value inside an interface value, if any.
func Unbox(v any) any {
	if d, ok := v.(Dyn); ok {
		return d.V
	}
	return v
}

// IDisplay is the prelude interface Display.
type IDisplay interface{ M_display(tk *Task) string }

// IError is the prelude interface Error, the open error type `! Error`.
type IError interface{ M_display(tk *Task) string }

// IDebug is the prelude interface Debug used as a type (for example the
// values of log fields).
type IDebug interface{ M_debug(tk *Task) string }

// IMarker is a prelude marker interface (Eq, Hash, Ord, Json) used as a
// type; any value satisfies it.
type IMarker = any

// Box holds a struct field whose type contains the struct itself by value
// (`struct Node { next: Node? }`), which Go cannot lay out inline. The
// pointed to value is never changed: writing the field stores a new box,
// so value semantics hold.
type Box[T any] struct{ p *T }

// BoxOf boxes v.
func BoxOf[T any](v T) Box[T] { return Box[T]{p: &v} }

// Get returns the boxed value (the zero value for an empty box).
func (b Box[T]) Get() T {
	if b.p == nil {
		var z T
		return z
	}
	return *b.p
}

// XoEq implements Eq.
func (b Box[T]) XoEq(o any) bool {
	p, ok := o.(Box[T])
	return ok && Eq(b.Get(), p.Get())
}

// XoHash implements Hash.
func (b Box[T]) XoHash() uint64 { return Hash(b.Get()) }

// XoCmp implements Ord.
func (b Box[T]) XoCmp(o any) int { return Cmp(b.Get(), o.(Box[T]).Get()) }

// XoDebug implements Debug.
func (b Box[T]) XoDebug(sb *strings.Builder) { debugTo(sb, b.Get()) }

// XoJsonEnc encodes the boxed value.
func (b Box[T]) XoJsonEnc(tk *Task, w *JsonW) { JsonEnc(tk, w, b.Get()) }

// XoJsonDec decodes into the box.
func (b *Box[T]) XoJsonDec(tk *Task, x any, path string) *Json_DecodeErr {
	var v T
	if err := JsonDec(tk, &v, x, path); err != nil {
		return err
	}
	*b = BoxOf(v)
	return nil
}

// XoMark methods let MarkP reach the lists inside the generic value types
// (see List). Generated code marks values of concrete types through
// static helpers instead (internal/gen/own.go).

// XoMark marks the lists held by the optional's value.
func (o *Option[T]) XoMark() {
	if o.Some {
		MarkP(&o.V)
	}
}

// XoMark marks the lists held by the result's value or error.
func (r *Result[T, E]) XoMark() {
	if r.Ok {
		MarkP(&r.V)
	} else {
		MarkP(&r.E)
	}
}

// XoMark marks the lists held by the tuple's elements.
func (t *Tuple2[A, B]) XoMark() { MarkP(&t.F0); MarkP(&t.F1) }

// XoMark marks the lists held by the tuple's elements.
func (t *Tuple3[A, B, C]) XoMark() { MarkP(&t.F0); MarkP(&t.F1); MarkP(&t.F2) }

// XoMark marks the lists held by the tuple's elements.
func (t *Tuple4[A, B, C, D]) XoMark() {
	MarkP(&t.F0)
	MarkP(&t.F1)
	MarkP(&t.F2)
	MarkP(&t.F3)
}

// XoMark marks the lists held by the tuple's elements.
func (t *Tuple5[A, B, C, D, E]) XoMark() {
	MarkP(&t.F0)
	MarkP(&t.F1)
	MarkP(&t.F2)
	MarkP(&t.F3)
	MarkP(&t.F4)
}

// XoMark marks the lists held by the tuple's elements.
func (t *Tuple6[A, B, C, D, E, F]) XoMark() {
	MarkP(&t.F0)
	MarkP(&t.F1)
	MarkP(&t.F2)
	MarkP(&t.F3)
	MarkP(&t.F4)
	MarkP(&t.F5)
}
