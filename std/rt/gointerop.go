package rt

import (
	"context"
	"errors"
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
	typ string // the Go type of a replayed error (gofx.go); "" otherwise
}

// GoErr wraps a non nil Go error.
func GoErr(err error) Go_Error { return Go_Error{err: err, msg: err.Error()} }

// Go_error is `go.error(message)` (decision 0416): Go's errors.New.
func Go_error(_ *Task, msg string) Go_Error { return GoErr(errors.New(msg)) }

// GoErrorOf is the Go error of an Xo `go.Error` an Xo callback returns
// to Go (decision 0416): the Go error it came from, so errors.Is sees
// io.EOF, or one with its text (a replayed error).
func GoErrorOf(e Go_Error) error {
	if e.err != nil {
		return e.err
	}
	return errors.New(e.msg)
}

// GoErrOpt is a Go error that is not a final result as `go.Error?`:
// None for nil (decision 0416).
func GoErrOpt(err error) Option[Go_Error] {
	if err == nil {
		return Option[Go_Error]{}
	}
	return Option[Go_Error]{Some: true, V: GoErr(err)}
}

// GoErrorOfOpt is the Go error of a `go.Error?`: nil for None.
func GoErrorOfOpt(o Option[Go_Error]) error {
	if !o.Some {
		return nil
	}
	return GoErrorOf(o.V)
}

// M_display is the Go error's text.
func (e Go_Error) M_display(*Task) string { return e.msg }

// M_message is the Go error's text.
func (e Go_Error) M_message(*Task) string { return e.msg }

// M_go_type is the Go dynamic type of the error.
func (e Go_Error) M_go_type(*Task) string { return e.goType() }

func (e Go_Error) goType() string {
	if e.typ != "" {
		return e.typ
	}
	return fmt.Sprintf("%T", e.err)
}

// Unwrap returns the Go error.
func (e Go_Error) Unwrap() error { return e.err }

// XoDebug implements Debug.
func (e Go_Error) XoDebug(b *strings.Builder) {
	b.WriteString("go.Error(")
	writeQuoted(b, e.msg)
	b.WriteByte(')')
}

// XoEq compares the error texts and dynamic types.
func (e Go_Error) XoEq(o any) bool {
	x, ok := o.(Go_Error)
	return ok && x.msg == e.msg && x.goType() == e.goType()
}

// XoHash hashes the error text.
func (e Go_Error) XoHash() uint64 { return maphash.String(hashSeed, e.msg) ^ 0x67 }

// GoCallback is an Xo function passed to Go as a callback (decision 0067).
// The adapter (internal/gobind) makes one where the function crosses into
// Go, on the goroutine of the task that passes it, and brackets every
// call with Begin and End. Go may call it in two ways:
//
//   - On that goroutine, during a Go call of the task (strings.Map,
//     sync.Once.Do): the task is suspended in Go, so the callback runs as
//     the task, sharing its thread record, held mutexes, and cancellation,
//     like a scope body. The request's arena is not shared: the callback
//     may store what it makes in Go state that outlives the request
//     (decision 0066, hole 2).
//   - On another goroutine (sync.WaitGroup.Go, time.AfterFunc), while the
//     task runs or after it ended: the call is a task of its own, a child
//     of the passing task's cancellation node (canceled with it), on a
//     thread of its own with its own held mutex set, counted by the
//     deadlock monitor and taking turns under the deterministic
//     scheduler. Before decision 0067 such a call shared the passing
//     task's thread and held mutex set: a Mutex the task held looked held
//     by the callback (a false XO-F004), two goroutines wrote one set, and
//     both used one scheduler turn.
//
// The two are told apart by the goroutine (curG) and by whether the task
// is inside a Go call (thread.inGo): a call on the task's goroutine while
// it is not in a Go call cannot happen, and a call while it is in one
// comes from Go code it called only if the goroutines match.
type GoCallback struct {
	own *Task   // the task that passed the callback
	tk  *Task   // own, without the request's arena (the synchronous case)
	g   uintptr // own's goroutine
	seq int64   // under the deterministic scheduler: its number (gosched.go)
}

// NewGoCallback is called where an Xo function crosses into Go as a
// callback, on the passing task's goroutine.
func NewGoCallback(tk *Task) *GoCallback {
	cb := &GoCallback{own: tk, tk: tk, g: curG()}
	if tk != nil && tk.arena != nil {
		cb.tk = &Task{node: tk.node, shield: tk.shield, held: tk.held, w: tk.w, stop: tk.stop, th: tk.th}
	}
	if tk != nil {
		if th := tk.th; th != nil {
			if !th.labeled && (th.mon != nil || th.sch != nil) {
				// a deadlock report through a Go call locates the call
				// from the goroutine profile (decision 0073); once per
				// thread
				labelThread(th)
			}
			if s := th.sch; s != nil {
				// numbered in the order the running thread passes them
				s.armed.Store(true)
				cb.seq = s.cbSeq.Add(1)
			}
		}
	}
	return cb
}

// Begin starts a call of the callback and returns the task it runs with;
// the adapter defers End with it. args returns the call's Xo arguments
// (nil: none); it is called only for a call on another goroutine under
// the deterministic scheduler, which orders and records calls by them.
func (cb *GoCallback) Begin(args func() []any) *Task {
	o := cb.own
	if o == nil || o.th == nil || o.node == nil {
		return cb.tk
	}
	th := o.th
	if th.inGo.Load() > 0 && curG() == cb.g {
		// Go code the task called calls back: the task leaves the Go call
		// for the duration (it takes the scheduler's turn back if it lent
		// it) and enters it again at End
		goLeave(th, false)
		return cb.tk
	}
	cth := new(thread)
	if th.sch != nil {
		// the scheduler gives the id when it admits the call (gosched.go)
		th.initChildPending(cth, "callback")
	} else {
		th.initChild(cth, "callback")
	}
	ctk := &Task{w: o.w, stop: o.stop, th: cth}
	ctk.node = newNodeOf(o.node, o.w, nodeTask, ctk)
	cth.cbOf = th
	th.cbN.Add(1)
	// Go's goroutine carries the label of the goroutine that started it:
	// this thread's own replaces it (lazyStacks)
	if cth.mon != nil && cth.sch == nil {
		labelThread(cth)
	} else if o.w == nil || cth.sch != nil {
		clearLabels()
	}
	if m := cth.mon; m != nil {
		m.started()
		m.addCallback(cth)
	}
	if s := cth.sch; s != nil {
		a := &cbArrival{th: cth, tk: ctk, cb: cb}
		if args != nil {
			a.args = args()
			a.enc = encodeArgs(a.args)
		}
		s.startCallback(a)
		labelThread(cth)
	} else if o.w != nil {
		o.w.taskStarted()
	}
	return ctk
}

// End ends a call started by Begin. A call on another goroutine ends its
// task: a cancellation unwind stops there (the callback returns zero
// values to Go), a fault goes on into Go's goroutine as before. A
// synchronous call re-enters the Go call; its panics go on to the
// adapter's GoExit.
func (cb *GoCallback) End(ctk *Task) {
	if ctk == cb.tk {
		if o := cb.own; o != nil && o.th != nil && o.node != nil {
			goEnter(o.th)
		}
		return
	}
	r := recover()
	ctk.node.detach()
	th := ctk.th
	th.mu.Lock()
	th.done = true
	th.mu.Unlock()
	th.cbOf.cbN.Add(-1)
	if m := th.mon; m != nil {
		m.dropCallback(th)
		m.ended()
	}
	if s := th.sch; s != nil {
		s.endCallback(th)
	} else if ctk.w != nil {
		ctk.w.taskEnded()
	}
	if r != nil && !IsUnwind(r) {
		panic(r)
	}
}

// GoEnter is called by every adapter before its Go call; it defers
// GoExit.
func GoEnter(tk *Task) {
	if tk != nil && tk.th != nil {
		goEnter(tk.th)
	}
}

// GoExit is deferred by every adapter that calls Go: the task leaves the
// Go call, and a Go panic becomes an Xo fault located at the calling .xo
// line. A fault or an unwind raised by an Xo callback that the Go code
// called passes through unchanged.
func GoExit(tk *Task, name string) {
	r := recover()
	if tk != nil && tk.th != nil {
		goExit(tk.th)
	}
	if r == nil {
		return
	}
	goRepanic(name, r)
}

// GoBoundary is deferred by adapters that only read or write a field of
// a Go struct: a Go panic becomes an Xo fault, as in GoExit.
func GoBoundary(name string) {
	r := recover()
	if r == nil {
		return
	}
	goRepanic(name, r)
}

func goRepanic(name string, r any) {
	switch r.(type) {
	case *FaultErr, *arithFault, *sizedFault, cancelUnwind, *exitUnwind:
		panic(r)
	}
	panic(newFault("", "", fmt.Sprintf("Go panic in %s: %v", name, r)))
}

// goEnter records that th entered a Go call. Under the deterministic
// scheduler, while Go callback threads are alive, th gives its turn away:
// Go code may wait for a callback (sync.WaitGroup.Wait), which needs the
// turn to run.
func goEnter(th *thread) {
	s := th.sch
	if s == nil {
		th.inGo.Add(1)
		if th.cbN.Load() > 0 && th.mon != nil {
			th.mon.goMoved()
		}
		return
	}
	s.mu.Lock()
	th.inGo.Add(1)
	if s.armed.Load() {
		s.goDirty = true
	}
	if s.cbLive.Load() > 0 && s.cur == th {
		s.lendLocked(th)
		s.pickLocked()
	}
	s.mu.Unlock()
	if th.cbN.Load() > 0 && th.mon != nil {
		th.mon.goMoved()
	}
}

// goEnterLend is goEnter for a Go call that waits on something an Xo task
// may have to provide (a receive on a Go channel, decision 0421): under
// the deterministic scheduler th gives its turn away even with no
// callback alive, and its return joins as a lent thread's does.
func goEnterLend(th *thread) {
	s := th.sch
	if s == nil {
		goEnter(th)
		return
	}
	s.mu.Lock()
	th.inGo.Add(1)
	if s.armed.Load() {
		s.goDirty = true
	}
	if s.cur == th {
		s.lendLocked(th)
		s.pickLocked()
	}
	s.mu.Unlock()
	if th.cbN.Load() > 0 && th.mon != nil {
		th.mon.goMoved()
	}
}

// goExit records that th's Go call returned.
func goExit(th *thread) { goLeave(th, true) }

// goLeave records that th left a Go call: it returned (ret), or Go calls
// back on th's goroutine. A thread that lent its turn waits to be
// admitted again (gosched.go). Once an Xo function crossed into Go, a
// return is a scheduling point under the deterministic scheduler: Go's
// goroutines the call woke settle, and the callbacks they call join.
func goLeave(th *thread, ret bool) {
	if th.cbN.Load() > 0 && th.mon != nil {
		th.mon.gen.Add(1)
	}
	s := th.sch
	if s == nil {
		th.inGo.Add(-1)
		return
	}
	s.mu.Lock()
	th.inGo.Add(-1)
	armed := s.armed.Load()
	if armed {
		s.goDirty = true
	}
	switch {
	case th.lent.Load():
		th.lent.Store(false)
		s.lentN--
		s.pendRet = append(s.pendRet, th)
		if s.cur == nil && !s.picking {
			s.pickLocked()
		}
	case ret && armed && s.cur == th:
		th.st = stReady
		s.ready = append(s.ready, th)
		s.cur = nil
		s.pickLocked()
	default:
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	<-th.turn
}

// taskContext is the context.Context supplied to Go functions that take
// one: it is done when the calling task is canceled (core 7.2).
type taskContext struct{ n *cnode }

func (c taskContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c taskContext) Done() <-chan struct{}       { return c.n.doneCh() }
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
	s := List_Slice(l)
	out := make([]T, len(s))
	for i, x := range s {
		out[i] = f(x)
	}
	return out
}

// GoSliceCopy copies a List into a new Go slice.
func GoSliceCopy[T any](l List[T]) []T { return slices.Clone(List_Slice(l)) }

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
		Map_put(nil, &out, e.k, e.v)
	}
	return out
}

// GoMapToGo copies a Map into a new Go map, converting keys and values.
func GoMapToGo[XK, XV any, K comparable, V any](m Map[XK, XV], fk func(XK) K, fv func(XV) V) map[K]V {
	out := make(map[K]V, m.n)
	for k, v := range Map_All(m) {
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
