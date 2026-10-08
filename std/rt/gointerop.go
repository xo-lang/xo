package rt

import (
	"context"
	"fmt"
	"hash/maphash"
	"slices"
	"strings"
	"time"
)

// Go interop (decision 0041, prototype 2026-10-08). The adapters written
// by `xo bind` (internal/gobind) are package rt files copied into the build
// directory as rt/xo_gobind_<path>.go. They call the helpers below.

// Go_Error is `go.Error` (std/go.xo): the error of a Go `(T, error)`
// result. The text is captured when the error crosses the boundary.
type Go_Error struct {
	err error
	msg string
}

// GoErr wraps a non nil Go error.
func GoErr(err error) Go_Error { return Go_Error{err: err, msg: err.Error()} }

// M_display is the Go error's text.
func (e Go_Error) M_display(*Task) string { return e.msg }

// M_message is the Go error's text.
func (e Go_Error) M_message(*Task) string { return e.msg }

// M_go_type is the Go dynamic type of the error.
func (e Go_Error) M_go_type(*Task) string { return fmt.Sprintf("%T", e.err) }

// Unwrap returns the Go error.
func (e Go_Error) Unwrap() error { return e.err }

// XoDebug implements Debug.
func (e Go_Error) XoDebug(b *strings.Builder) {
	b.WriteString("go.Error(")
	b.WriteString(quote(e.msg))
	b.WriteByte(')')
}

// XoEq compares the error texts and dynamic types.
func (e Go_Error) XoEq(o any) bool {
	x, ok := o.(Go_Error)
	return ok && x.msg == e.msg && fmt.Sprintf("%T", x.err) == fmt.Sprintf("%T", e.err)
}

// XoHash hashes the error text.
func (e Go_Error) XoHash() uint64 { return maphash.String(hashSeed, e.msg) ^ 0x67 }

// GoBoundary is deferred by every adapter: a Go panic becomes an Xo fault
// located at the calling .xo line. A fault raised by an Xo callback that
// the Go code called passes through unchanged.
func GoBoundary(name string) {
	r := recover()
	if r == nil {
		return
	}
	switch r.(type) {
	case *FaultErr, *arithFault:
		panic(r)
	}
	panic(newFault("", "", fmt.Sprintf("Go panic in %s: %v", name, r)))
}

// taskContext is the context.Context supplied to Go functions that take
// one: it is done when the calling task is canceled (core 7.2).
type taskContext struct{ n *cnode }

func (c taskContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c taskContext) Done() <-chan struct{}       { return c.n.done }
func (c taskContext) Value(any) any               { return nil }
func (c taskContext) Err() error {
	if c.n.canceled.Load() {
		return context.Canceled
	}
	return nil
}

// GoContext returns the context.Context of the current task.
func GoContext(tk *Task) context.Context {
	if tk == nil || tk.node == nil {
		return context.Background()
	}
	return taskContext{n: tk.node}
}

// GoListOf copies a Go slice into a List, converting each element.
func GoListOf[T, X any](xs []T, f func(T) X) List[X] {
	if len(xs) == 0 {
		return List[X]{}
	}
	out := make([]X, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return ListOf(out...)
}

// GoListCopy copies a Go slice into a List.
func GoListCopy[T any](xs []T) List[T] {
	if len(xs) == 0 {
		return List[T]{}
	}
	return ListOf(slices.Clone(xs)...)
}

// GoSliceOf copies a List into a new Go slice, converting each element.
func GoSliceOf[X, T any](l List[X], f func(X) T) []T {
	s := l.Slice()
	out := make([]T, len(s))
	for i, x := range s {
		out[i] = f(x)
	}
	return out
}

// GoSliceCopy copies a List into a new Go slice.
func GoSliceCopy[T any](l List[T]) []T { return slices.Clone(l.Slice()) }

// GoMapOf copies a Go map into a Map, converting keys and values. Entries
// are inserted in key order so iteration is deterministic.
func GoMapOf[K comparable, V, XK, XV any](m map[K]V, fk func(K) XK, fv func(V) XV) Map[XK, XV] {
	type kv struct {
		k XK
		v XV
	}
	es := make([]kv, 0, len(m))
	for k, v := range m {
		es = append(es, kv{fk(k), fv(v)})
	}
	slices.SortFunc(es, func(a, b kv) int { return Cmp(a.k, b.k) })
	var out Map[XK, XV]
	for _, e := range es {
		out.M_put(nil, e.k, e.v)
	}
	return out
}

// GoMapToGo copies a Map into a new Go map, converting keys and values.
func GoMapToGo[XK, XV any, K comparable, V any](m Map[XK, XV], fk func(XK) K, fv func(XV) V) map[K]V {
	out := make(map[K]V, m.n)
	for k, v := range m.All() {
		out[fk(k)] = fv(v)
	}
	return out
}

// GoPtrTo returns a pointer to a copy of v (a Go value held by a handle).
func GoPtrTo[T any](v T) *T { return &v }

// GoArrayLen faults when a Bytes value passed where Go wants a fixed size
// array has the wrong length.
func GoArrayLen(got, want int, what string) {
	if got != want {
		FaultHere("Go interop: %s needs exactly %d bytes, got %d", what, want, got)
	}
}

// GoNotNil faults when a Go function returned a nil pointer where the
// binding promised a value (`(*T, error)` with a nil error).
func GoNotNil(ok bool, what string) {
	if !ok {
		FaultHere("Go interop: %s returned a nil pointer without an error", what)
	}
}

// GoHashComparable hashes a comparable Go value held by a handle.
func GoHashComparable[T comparable](v T) uint64 { return maphash.Comparable(hashSeed, v) }

// GoEqIface compares two Go interface values held by handles; values whose
// dynamic type is not comparable are equal only if both are nil.
func GoEqIface(a, b any) (eq bool) {
	defer func() {
		if recover() != nil {
			eq = false
		}
	}()
	return a == b
}

// GoHashIface hashes a Go interface value held by a handle.
func GoHashIface(v any) (h uint64) {
	defer func() {
		if recover() != nil {
			h = 0
		}
	}()
	return maphash.Comparable(hashSeed, v)
}

// GoDebugHandle writes the Debug text of a handle: `<go *regexp.Regexp>`.
func GoDebugHandle(b *strings.Builder, goType string) {
	b.WriteString("<go ")
	b.WriteString(goType)
	b.WriteByte('>')
}

// GoCollect materializes an iter.Seq result.
func GoCollect[T any](seq func(func(T) bool)) []T {
	var out []T
	for x := range seq {
		out = append(out, x)
	}
	return out
}

// GoCollect2 materializes an iter.Seq2 result as tuples.
func GoCollect2[K, V, X any](seq func(func(K, V) bool), f func(K, V) X) List[X] {
	var out []X
	for k, v := range seq {
		out = append(out, f(k, v))
	}
	return ListOf(out...)
}
