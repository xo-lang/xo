package rt

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// lock is a cancelable FIFO lock with explicit waiters (see Chan).
type lock struct {
	mu      sync.Mutex
	permits int
	q       []*waiter
}

// acquire takes a permit, waiting if none is free. It reports false when
// the task was canceled while waiting.
func (l *lock) acquire(tk *Task) bool {
	l.mu.Lock()
	if l.permits > 0 {
		l.permits--
		l.mu.Unlock()
		return true
	}
	w := newWaiter(tk)
	l.q = append(l.q, w)
	l.mu.Unlock()
	if tk.wait(w) {
		return true
	}
	l.mu.Lock()
	for i, x := range l.q {
		if x == w {
			l.q = append(l.q[:i], l.q[i+1:]...)
			break
		}
	}
	l.mu.Unlock()
	return false
}

// release hands the permit to the next waiter or frees it.
func (l *lock) release() {
	l.mu.Lock()
	for len(l.q) > 0 {
		w := l.q[0]
		l.q = l.q[1:]
		if w.claim() {
			l.mu.Unlock()
			w.signal()
			return
		}
	}
	l.permits++
	l.mu.Unlock()
}

// heldSite is where a task acquired a mutex: the caller's program
// counters, resolved to `file.xo:line` only when a re-entrant use is
// reported (formatting the location on every `with` cost an allocation
// and a full stack walk). 14 frames keep the value small enough to be
// stored inline in the map; the caller of Mutex_with is generated code,
// whose first frame is an Xo frame.
type heldSite struct {
	pcs [14]uintptr
	n   int
}

func (h heldSite) loc() string { return xoLocationPCs(h.pcs[:h.n]) }

// Mutex is `Mutex[T]` (core 5.4): a handle; copies share the state.
type Mutex[T any] struct {
	l lock
	v T
}

// Mutex_new is `Mutex[T].new(v)`.
func Mutex_new[T any](_ *Task, v T) *Mutex[T] {
	m := &Mutex[T]{v: v}
	m.l.permits = 1
	return m
}

// XoDebug implements Debug.
func (m *Mutex[T]) XoDebug(b *strings.Builder) { b.WriteString("<mutex>") }

// Mutex_with is `m.with(f)` (core 5.4): f gets a copy of the value; the
// copy is written back only when f returns successfully. Waiting is a
// cancellation point that unwinds. Re-entrant use in one task faults
// (XO-F004).
func Mutex_with[T, R, E any](tk *Task, m *Mutex[T], f func(*Task, *T) Result[R, E]) Result[R, E] {
	if prev, ok := tk.held[m]; ok {
		FaultCode("XO-F004", xoLocation(2), "Mutex.with called re-entrantly in one task; the mutex is already held from %s", prev.loc())
	}
	// a cancellation point only while waiting (core 7.2)
	if !m.l.acquire(tk) {
		tk.Unwind()
	}
	if tk.held == nil {
		tk.held = map[any]heldSite{}
	}
	var site heldSite
	site.n = runtime.Callers(2, site.pcs[:])
	tk.held[m] = site
	defer func() {
		delete(tk.held, m)
		m.l.release()
	}()
	v := m.v
	r := f(tk, &v)
	if r.Ok {
		m.v = v
	}
	return r
}

// Atomic is `Atomic[T]` for Int and Bool (std.md 2.7).
type Atomic[T any] struct {
	mu sync.Mutex
	v  T
}

// Atomic_new is `Atomic[T].new(v)`.
func Atomic_new[T any](_ *Task, v T) *Atomic[T] { return &Atomic[T]{v: v} }

// M_get is `a.get()`.
func (a *Atomic[T]) M_get(*Task) T {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.v
}

// M_set is `a.set(v)`.
func (a *Atomic[T]) M_set(_ *Task, v T) {
	a.mu.Lock()
	a.v = v
	a.mu.Unlock()
}

// M_swap is `a.swap(v)`.
func (a *Atomic[T]) M_swap(_ *Task, v T) T {
	a.mu.Lock()
	defer a.mu.Unlock()
	old := a.v
	a.v = v
	return old
}

// M_compare_and_swap is `a.compare_and_swap(old, new)`.
func (a *Atomic[T]) M_compare_and_swap(_ *Task, old, nv T) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !Eq(a.v, old) {
		return false
	}
	a.v = nv
	return true
}

// XoDebug implements Debug.
func (a *Atomic[T]) XoDebug(b *strings.Builder) {
	b.WriteString("Atomic(")
	debugTo(b, a.M_get(nil))
	b.WriteString(")")
}

// Atomic_add is `a.add(n)` on Atomic[Int]: returns the new value; overflow
// faults.
func Atomic_add(_ *Task, a *Atomic[int64], n int64) int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.v = AddS(a.v, n, "")
	return a.v
}

// Task_Semaphore is `task.Semaphore`: copies share permits.
type Task_Semaphore struct{ l *lock }

// Task_semaphore is `task.semaphore(permits)`.
func Task_semaphore(_ *Task, permits int64) Task_Semaphore {
	if permits <= 0 {
		FaultHere("a semaphore needs at least one permit, got %d", permits)
	}
	return Task_Semaphore{l: &lock{permits: int(permits)}}
}

// XoDebug implements Debug.
func (s Task_Semaphore) XoDebug(b *strings.Builder) { b.WriteString("<semaphore>") }

// Task_Semaphore_with is `s.with(f)`: waits for a permit (a cancellation
// point), runs f, releases the permit.
func Task_Semaphore_with[R, E any](tk *Task, s Task_Semaphore, f func(*Task) Result[R, E]) Result[R, E] {
	if !s.l.acquire(tk) {
		tk.Unwind()
	}
	defer s.l.release()
	return f(tk)
}

// Task_isolate is `task.isolate(f)` (core 8): runs f and turns a fault
// into `Err(Isolated.Faulted(info))`. Cancellation still unwinds.
func Task_isolate[T, E any](tk *Task, f func(*Task) Result[T, E]) (res Result[T, Task_Isolated[E]]) {
	defer func() {
		if r := recover(); r != nil {
			if IsUnwind(r) {
				panic(r)
			}
			if _, ok := r.(*exitUnwind); ok {
				panic(r)
			}
			info := faultInfo(AsFault(r))
			res = Err[T](Task_Isolated[E]{Tag: 1, P: Task_Isolated_Faulted[E]{F_info: info}})
		}
	}()
	inner := f(tk)
	if inner.Ok {
		return Ok[T, Task_Isolated[E]](inner.V)
	}
	return ErrTr[T](Task_Isolated[E]{Tag: 2, P: Task_Isolated_Failed[E]{F0: inner.E}}, inner.Tr)
}

func faultInfo(f *FaultErr) Task_Fault {
	var frames []string
	for _, l := range strings.Split(strings.TrimSpace(f.Stack), "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			frames = append(frames, l)
		}
	}
	msg := f.Msg
	if f.Code != "" {
		msg = f.Code + ": " + msg
	}
	return Task_Fault{F_msg: msg, F_location: f.Loc, F_frames: ListOf(frames...)}
}

// Task_with_timeout is `task.with_timeout(clock, d, f)` (std.md 7): runs f
// in a child scope; on expiry cancels it, waits for it to unwind, and
// returns `Err(TimedOut)`.
func Task_with_timeout[T, E any](tk *Task, clock Clock, d Duration, f func(*Task) Result[T, E]) Result[T, Task_Timed[E]] {
	tk.CheckCancel()
	s := NewScope(tk)
	h := Spawn(tk, s, f, false, true)
	timedOut := awaitDeadline(tk, s, h.done, clock, d, nil)
	if timedOut {
		s.shutdown()
		return Err[T](Task_Timed[E]{Tag: 1})
	}
	s.shutdown()
	if h.fault != nil {
		panic(h.fault)
	}
	if h.canceled {
		tk.CheckCancel()
		return Err[T](Task_Timed[E]{Tag: 1})
	}
	if h.res.Ok {
		return Ok[T, Task_Timed[E]](h.res.V)
	}
	return ErrTr[T](Task_Timed[E]{Tag: 2, P: Task_Timed_Failed[E]{F0: h.res.E}}, h.res.Tr)
}

// awaitDeadline waits in tk until done fires or d passes on clock. It
// reports whether the deadline passed first. onFire, when set, runs as
// the deadline passes (in the timer callback, before tk is woken). When
// tk is canceled first, the scope s is shut down and tk unwinds.
func awaitDeadline(tk *Task, s *Scope, done *event, clock Clock, d Duration, onFire func()) bool {
	timerCh := make(chan struct{})
	var fired atomic.Bool
	stop := clockAfter(tk, clock, d, func(locked bool) {
		if fired.CompareAndSwap(false, true) {
			if onFire != nil {
				onFire()
			}
			if tk.w != nil {
				if locked {
					tk.w.wakeLocked(tk)
				} else {
					tk.w.wake(tk)
				}
			}
			close(timerCh)
		}
	})
	done.mu.Lock()
	if !done.fired {
		done.waiters = append(done.waiters, tk)
	}
	done.mu.Unlock()
	tk.block()
	canceled := false
	select {
	case <-done.ch:
	case <-timerCh:
	case <-tk.Done():
		canceled = true
	}
	done.mu.Lock()
	for i, x := range done.waiters {
		if x == tk {
			done.waiters = append(done.waiters[:i], done.waiters[i+1:]...)
			break
		}
	}
	done.mu.Unlock()
	tk.unblock()
	stop()
	timedOut := !done.isSet() && fired.Load()
	if canceled && !done.isSet() {
		s.shutdown()
		tk.Unwind()
	}
	return timedOut
}

// clockAfter calls f after d on the given clock: the virtual clock of a
// test or real time.
func clockAfter(tk *Task, c Clock, d Duration, f func(locked bool)) func() {
	if fc, ok := c.(Testing_FakeClock); ok && fc.w != nil {
		return fc.w.timerFunc(d, func() { f(true) })
	}
	if tk.w != nil {
		return tk.w.timerFunc(d, func() { f(true) })
	}
	t := time.AfterFunc(time.Duration(d), func() { f(false) })
	return func() { t.Stop() }
}

// Task_map_concurrent is `task.map_concurrent(items, limit, f)` (std.md
// 7): results are in input order; one failure does not stop the others.
func Task_map_concurrent[T, R, E any](tk *Task, items List[T], limit int64, f func(*Task, T) Result[R, E]) List[Result[R, E]] {
	if limit <= 0 {
		FaultHere("map_concurrent needs a positive limit, got %d", limit)
	}
	out := make([]Result[R, E], len(items.s))
	mapConcurrent(tk, items, limit, func(t *Task, i int, x T) { out[i] = f(t, x) })
	return ListOf(out...)
}

// mapConcurrent runs each(t, i, items[i]) for every item in a child scope
// of tk, with at most limit running at once, and waits for all of them.
func mapConcurrent[T any](tk *Task, items List[T], limit int64, each func(*Task, int, T)) {
	s := NewScope(tk)
	sem := &lock{permits: int(limit)}
	body := s.Body()
	func() {
		completed := false
		defer func() { s.Close(recover(), completed, 0) }()
		for i, x := range items.s {
			body.CheckCancel()
			if !sem.acquire(body) {
				body.Unwind()
			}
			if body.Canceled() {
				// a finishing task handed its permit to the loop while the
				// scope was being canceled: start nothing more
				sem.release()
				body.Unwind()
			}
			Spawn(body, s, func(t *Task) Result[Unit, Never] {
				defer sem.release()
				each(t, i, x)
				return Ok[Unit, Never](Unit{})
			}, false, true)
		}
		completed = true
	}()
}

// Task_map_concurrent_until is `task.map_concurrent_until(clock, d, items,
// limit, f)` (std.md 7, decision 0043): map_concurrent with a deadline.
// When d passes on clock, the items still running or not started are
// canceled and give `Err(TimedOut)`; results that finished before the
// deadline are kept, in input order. A result that arrives after the
// deadline fired (a task finishing while it is being canceled) counts as
// timed out, so the cut is the instant the timer fires.
func Task_map_concurrent_until[T, R, E any](tk *Task, clock Clock, d Duration, items List[T], limit int64, f func(*Task, T) Result[R, E]) List[Result[R, Task_Timed[E]]] {
	if limit <= 0 {
		FaultHere("map_concurrent_until needs a positive limit, got %d", limit)
	}
	tk.CheckCancel()
	n := len(items.s)
	var mu sync.Mutex
	expired := false
	res := make([]Result[R, E], n)
	fin := make([]bool, n)
	s := NewScope(tk)
	h := Spawn(tk, s, func(t *Task) Result[Unit, Never] {
		mapConcurrent(t, items, limit, func(t *Task, i int, x T) {
			r := f(t, x)
			mu.Lock()
			if !expired {
				res[i], fin[i] = r, true
			}
			mu.Unlock()
		})
		return Ok[Unit, Never](Unit{})
	}, false, true)
	awaitDeadline(tk, s, h.done, clock, d, func() {
		mu.Lock()
		expired = true
		mu.Unlock()
	})
	s.shutdown()
	if h.fault != nil {
		panic(h.fault)
	}
	if h.canceled {
		tk.CheckCancel()
	}
	out := make([]Result[R, Task_Timed[E]], n)
	mu.Lock()
	defer mu.Unlock()
	for i := range out {
		switch {
		case !fin[i]:
			out[i] = Err[R](Task_Timed[E]{Tag: 1})
		case res[i].Ok:
			out[i] = Ok[R, Task_Timed[E]](res[i].V)
		default:
			out[i] = ErrTr[R](Task_Timed[E]{Tag: 2, P: Task_Timed_Failed[E]{F0: res[i].E}}, res[i].Tr)
		}
	}
	return ListOf(out...)
}
