package rt

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// lock is a cancelable lock with explicit waiters (see Chan).
//
// Under a test world or the deterministic scheduler it is strictly FIFO:
// release hands the permit to the first waiter. In a real program that
// handoff made a lock convoy under contention (the permit sat unused until
// the woken goroutine was scheduled, most of the request time of
// bench/perf/http under 32 workers), so there release frees the permit
// and wakes the first waiter, which competes for it again like a new
// acquirer (barging, as Go's sync.Mutex). A waiter that loses lockStarve
// times in a row sets starving, which switches release back to handoff
// until a handed off waiter gets the permit. Decision 0047.
//
// A Mutex lock also has a shared mode (Mutex.with_read, decision 0063):
// many readers or one writer. The mutex is held exclusively while permits
// is 0 and shared while readers > 0, never both. Writers are preferred:
// while a writer waits (wwait > 0) a new reader queues instead of joining
// the readers inside, so a stream of readers cannot starve a writer. A
// release wakes the waiter at the head of the queue, and when that is a
// reader, every reader queued right behind it too (they do not exclude
// each other); a woken reader may enter although a writer waits, since it
// was queued ahead of that writer.
type lock struct {
	mu      sync.Mutex
	permits int
	readers int
	// wwait counts the exclusive acquirers that wait or were woken and
	// retry; new readers queue while it is positive (writer preference)
	wwait int
	q     []*waiter
	// starving: a waiter lost the race too often; hand off (see above)
	starving bool
	// a Mutex: where it was made and who holds it since where, for
	// deadlock reports and `xo dump`; rdrs are the readers inside
	mutex bool
	made  [3]uintptr
	owner *Task
	site  heldSite
	rdrs  []*Task
}

// lockStarve is how many lost races make a waiter ask for handoff.
const lockStarve = 4

// barging reports whether tk runs in a real program, where the lock may
// let a running task take a permit ahead of a woken waiter.
func barging(tk *Task) bool {
	return tk != nil && tk.w == nil && (tk.th == nil || tk.th.sch == nil)
}

// acquire takes a permit, waiting if none is free. It reports false when
// the task was canceled while waiting.
func (l *lock) acquire(tk *Task) bool { return l.acquireMode(tk, false, nil) }

// free reports whether an acquirer may enter now. woken: a reader that was
// queued and woken keeps its place ahead of writers that queued later.
func (l *lock) free(shared, woken bool) bool {
	if shared {
		return l.permits > 0 && (l.wwait == 0 || woken)
	}
	return l.permits > 0 && l.readers == 0
}

// acquireMode is acquire in exclusive or shared (reader) mode. A reader
// is recorded in rdrs. site, when set, is where an exclusive holder took
// the mutex: it becomes owner and site in the same critical section of
// l.mu that takes the permit (a second one after acquiring made every
// holder wait for l.mu behind the waiters hammering it, inside its hold).
func (l *lock) acquireMode(tk *Task, shared bool, site *heldSite) bool {
	k := waitSemaphore
	switch {
	case l.mutex && shared:
		k = waitMutexRead
	case l.mutex:
		k = waitMutex
	}
	lost := 0
	// pending: this writer is counted in wwait
	pending := false
	for {
		l.mu.Lock()
		if l.free(shared, lost > 0) {
			if shared {
				l.readers++
				l.rdrs = append(l.rdrs, tk)
			} else {
				l.permits--
				if site != nil {
					l.owner, l.site = tk, *site
				}
			}
			if pending {
				l.wwait--
			}
			if lost > 0 {
				l.starving = false
			}
			l.mu.Unlock()
			return true
		}
		if !shared && !pending {
			pending = true
			l.wwait++
		}
		w := newWaiter(tk)
		w.shared = shared
		if lost > 0 {
			// a woken waiter that lost the permit keeps its place
			l.q = append(l.q, nil)
			copy(l.q[1:], l.q)
			l.q[0] = w
			if lost >= lockStarve {
				l.starving = true
			}
		} else {
			l.q = append(l.q, w)
		}
		l.mu.Unlock()
		if !tk.wait(w, k, l) {
			l.mu.Lock()
			for i, x := range l.q {
				if x == w {
					l.q = append(l.q[:i], l.q[i+1:]...)
					break
				}
			}
			if pending {
				l.wwait--
			}
			// a writer that leaves may unblock the readers queued behind it
			l.wakeLocked()
			return false
		}
		if w.granted {
			l.mu.Lock()
			l.starving = false
			if shared {
				l.rdrs = append(l.rdrs, tk)
			} else if site != nil {
				l.owner, l.site = tk, *site
			}
			if pending {
				l.wwait--
			}
			l.mu.Unlock()
			return true
		}
		lost++
	}
}

// release ends an exclusive hold: it hands the permit to the next waiter
// or frees it and wakes the next waiter (see lock).
func (l *lock) release() {
	l.mu.Lock()
	l.owner = nil
	l.permits++
	l.wakeLocked()
}

// releaseRead ends tk's shared hold; the last reader out wakes the next
// waiter.
func (l *lock) releaseRead(tk *Task) {
	l.mu.Lock()
	l.readers--
	for i, x := range l.rdrs {
		if x == tk {
			last := len(l.rdrs) - 1
			l.rdrs[i] = l.rdrs[last]
			l.rdrs[last] = nil
			l.rdrs = l.rdrs[:last]
			break
		}
	}
	if l.readers > 0 {
		l.mu.Unlock()
		return
	}
	l.wakeLocked()
}

// wakeLocked wakes the waiters that may enter now and unlocks l.mu: the
// head of the queue, and the readers right behind a reader at the head.
// Under a test world, the deterministic scheduler, or starvation they get
// the permit handed over; in a real run they compete for it again.
func (l *lock) wakeLocked() {
	var first *waiter
	var more []*waiter
	bg := false
	for len(l.q) > 0 {
		w := l.q[0]
		if l.permits == 0 || !w.shared && l.readers > 0 {
			break
		}
		l.q = l.q[1:]
		if !w.claim() {
			continue // canceled: it removes nothing, it is out of the queue
		}
		b := barging(w.tk)
		if l.starving || !b {
			w.granted = true
			if w.shared {
				l.readers++
			} else {
				l.permits--
			}
		}
		bg = bg || b
		if first == nil {
			first = w
		} else {
			more = append(more, w)
		}
		if !w.shared || len(l.q) == 0 || !l.q[0].shared {
			break
		}
	}
	l.mu.Unlock()
	if first == nil {
		return
	}
	first.signal()
	for _, w := range more {
		w.signal()
	}
	if bg {
		// the woken goroutine waits in this processor's runnext slot until
		// this one blocks, which can be a whole request later; let it run
		// now (knowledgebase/gotchas/lock-convoy-runnext-wake.md)
		runtime.Gosched()
	}
}

// heldSite is where a task acquired a mutex: the caller's program
// counters, resolved to `file.xo:line` only when a re-entrant use or a
// deadlock is reported (formatting the location on every `with` cost an
// allocation and a full stack walk). The caller of Mutex_with is
// generated code, whose first frame is an Xo frame, so two frames are
// enough: walking 14 on every `with` was 64% of bench/perf/ctx mutex's
// instructions on the Go backend
// (bench/results/go-backend-gap-2026-10-22.md).
type heldSite struct {
	pcs [2]uintptr
	n   int
}

func (h heldSite) loc() string { return xoLocationPCs(h.pcs[:h.n]) }

// holdSite records in tk.held that tk is taking mutex m, from the caller
// of the caller of holdSite (Mutex_with's caller), before it waits: the
// stack walk and the map insert stay out of the critical section. The
// caller deletes the entry if the wait is canceled.
func holdSite(tk *Task, m any) heldSite {
	var site heldSite
	site.n = runtime.Callers(3, site.pcs[:])
	if tk.held == nil {
		tk.held = new(heldSet)
	}
	tk.held.put(m, site)
	return site
}

// heldSet is the set of mutexes a task holds, with where it took them. A
// task rarely holds more than one at once, so that one lives inline (the
// map it replaced cost two allocations per task that took a mutex); a
// request task's set is part of its reqBlock. A scope body shares its
// parent's set (scope.go). Only the task's own thread touches it.
type heldSet struct {
	ms    [1]any
	sites [1]heldSite
	more  map[any]heldSite
}

// get is the site where m was taken, if the task holds m.
func (h *heldSet) get(m any) (heldSite, bool) {
	if h == nil {
		return heldSite{}, false
	}
	for i, x := range h.ms {
		if x == m {
			return h.sites[i], true
		}
	}
	s, ok := h.more[m]
	return s, ok
}

func (h *heldSet) put(m any, s heldSite) {
	for i, x := range h.ms {
		if x == nil {
			h.ms[i], h.sites[i] = m, s
			return
		}
	}
	if h.more == nil {
		h.more = map[any]heldSite{}
	}
	h.more[m] = s
}

func (h *heldSet) del(m any) {
	if h == nil {
		return
	}
	for i, x := range h.ms {
		if x == m {
			h.ms[i], h.sites[i] = nil, heldSite{}
			return
		}
	}
	delete(h.more, m)
}

// Mutex is `Mutex[T]` (core 5.4): a handle; copies share the state.
type Mutex[T any] struct {
	l lock
	v T
}

// Mutex_new is `Mutex[T].new(v)`.
func Mutex_new[T any](_ *Task, v T) *Mutex[T] {
	m := &Mutex[T]{v: v}
	m.l.permits = 1
	m.l.mutex = true
	runtime.Callers(2, m.l.made[:])
	return m
}

// XoDebug implements Debug.
func (m *Mutex[T]) XoDebug(b *strings.Builder) { b.WriteString("<mutex>") }

// Mutex_with is `m.with(f)` (core 5.4): f gets a copy of the value; the
// copy is written back only when f returns successfully. Waiting is a
// cancellation point that unwinds. Re-entrant use in one task faults
// (XO-F004).
func Mutex_with[T, R, E any](tk *Task, m *Mutex[T], f func(*Task, *T) Result[R, E]) Result[R, E] {
	tk.Yield()
	if prev, ok := tk.held.get(m); ok {
		FaultCode("XO-F004", xoLocation(2), "Mutex.with called re-entrantly in one task; the mutex is already held from %s", prev.loc())
	}
	// the bookkeeping happens before the wait, so the critical section
	// does not pay for the stack walk
	site := holdSite(tk, m)
	var t0, t1 time.Time
	if lockStatsOn {
		t0 = time.Now()
	}
	// a cancellation point only while waiting (core 7.2)
	if !m.l.acquireMode(tk, false, &site) {
		tk.held.del(m)
		tk.Unwind()
	}
	if lockStatsOn {
		t1 = time.Now()
	}
	defer func() {
		tk.held.del(m)
		if lockStatsOn {
			t2 := time.Now()
			m.l.release()
			lockStatRecord(site.pcs[0], false, t1.Sub(t0), t2.Sub(t1))
			return
		}
		m.l.release()
	}()
	v := m.v
	// m.v keeps the old value if f fails or faults, so f's writes to
	// lists in v must copy them (see List)
	MarkP(&v)
	r := f(tk, &v)
	if r.Ok {
		m.v = v
	}
	return r
}

// Mutex_with_read is `m.with_read(f)` (std.md 2.7, decision 0063): f
// gets a read only copy of the value while the mutex is held in shared
// mode; other readers may hold it at the same time, writers wait (see
// lock). Waiting is a cancellation point that unwinds. Re-entrant use in
// one task (with or with_read inside either) faults (XO-F004): a nested
// read could deadlock behind a waiting writer.
func Mutex_with_read[T, R, E any](tk *Task, m *Mutex[T], f func(*Task, T) Result[R, E]) Result[R, E] {
	tk.Yield()
	if prev, ok := tk.held.get(m); ok {
		FaultCode("XO-F004", xoLocation(2), "Mutex.with_read called re-entrantly in one task; the mutex is already held from %s", prev.loc())
	}
	site := holdSite(tk, m)
	var t0, t1 time.Time
	if lockStatsOn {
		t0 = time.Now()
	}
	if !m.l.acquireMode(tk, true, nil) {
		tk.held.del(m)
		tk.Unwind()
	}
	if lockStatsOn {
		t1 = time.Now()
	}
	defer func() {
		tk.held.del(m)
		if lockStatsOn {
			t2 := time.Now()
			m.l.releaseRead(tk)
			lockStatRecord(site.pcs[0], true, t1.Sub(t0), t2.Sub(t1))
			return
		}
		m.l.releaseRead(tk)
	}()
	// readers share m.v: the copy f gets must not write m.v's lists in
	// place (see List); marking is atomic, so readers may race on it.
	// Own, not MarkP(&v): that moved v to the heap on every call.
	return f(tk, Own(m.v))
}

// Atomic is `Atomic[T]` for Int and Bool (std.md 2.7): one atomic.Int64
// holding the value (a Bool as 0 or 1). A sync.Mutex and a defer per
// operation cost about 98 instructions a get against native's 6
// (bench/results/go-backend-gap-2026-10-22.md).
type Atomic[T any] struct {
	n atomic.Int64
}

// atomEnc is v as the int64 Atomic stores. T is Int or Bool; the size
// test is a constant in each of the two instantiations.
func atomEnc[T any](v T) int64 {
	if unsafe.Sizeof(v) == 8 {
		return *(*int64)(unsafe.Pointer(&v))
	}
	return int64(*(*uint8)(unsafe.Pointer(&v)))
}

// atomDec is the T that atomEnc encoded as n.
func atomDec[T any](n int64) T {
	var v T
	if unsafe.Sizeof(v) == 8 {
		*(*int64)(unsafe.Pointer(&v)) = n
	} else {
		*(*uint8)(unsafe.Pointer(&v)) = uint8(n)
	}
	return v
}

// Atomic_new is `Atomic[T].new(v)`.
func Atomic_new[T any](_ *Task, v T) *Atomic[T] {
	a := &Atomic[T]{}
	a.n.Store(atomEnc(v))
	return a
}

// M_get is `a.get()`.
func (a *Atomic[T]) M_get(tk *Task) T {
	tk.Yield()
	return atomDec[T](a.n.Load())
}

// M_set is `a.set(v)`.
func (a *Atomic[T]) M_set(tk *Task, v T) {
	tk.Yield()
	a.n.Store(atomEnc(v))
}

// M_swap is `a.swap(v)`.
func (a *Atomic[T]) M_swap(tk *Task, v T) T {
	tk.Yield()
	return atomDec[T](a.n.Swap(atomEnc(v)))
}

// M_compare_and_swap is `a.compare_and_swap(old, new)`.
func (a *Atomic[T]) M_compare_and_swap(tk *Task, old, nv T) bool {
	tk.Yield()
	return a.n.CompareAndSwap(atomEnc(old), atomEnc(nv))
}

// XoDebug implements Debug.
func (a *Atomic[T]) XoDebug(b *strings.Builder) {
	b.WriteString("Atomic(")
	debugTo(b, a.M_get(nil))
	b.WriteString(")")
}

// Atomic_add is `a.add(n)` on Atomic[Int]: returns the new value; overflow
// faults (and leaves the value unchanged).
func Atomic_add(tk *Task, a *Atomic[int64], n int64) int64 {
	tk.Yield()
	for {
		old := a.n.Load()
		v := AddS(old, n, "")
		if a.n.CompareAndSwap(old, v) {
			return v
		}
	}
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
	tk.Yield()
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
	h := spawnUnder(tk, clock, d, func() *Handle[T, E] { return Spawn(tk, s, f, false, true) })
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

// spawnUnder runs spawn, which starts a task from tk, with tk's deadline
// lowered to d from now on clock while it does, so the task works under
// it (task.deadline, decision 0430).
func spawnUnder[H any](tk *Task, clock Clock, d Duration, spawn func() H) H {
	old := tk.dl
	at := Time(int64(clock.M_now(tk)) + int64(d))
	if old == nil || int64(at) < int64(dlOn(tk, old, clock)) {
		tk.dl = &taskDeadline{clock: clock, at: at}
	}
	h := spawn()
	tk.dl = old
	return h
}

// dlOn is the deadline dl as an instant on clock.
func dlOn(tk *Task, dl *taskDeadline, clock Clock) Time {
	return Time(int64(clock.M_now(tk)) + int64(dl.at) - int64(dl.clock.M_now(tk)))
}

// Task_deadline is `task.deadline(clock)` (decision 0430): the nearest
// with_timeout deadline the task works under, on clock.
func Task_deadline(tk *Task, clock Clock) Option[Time] {
	if tk.dl == nil {
		return Option[Time]{}
	}
	return Option[Time]{Some: true, V: dlOn(tk, tk.dl, clock)}
}

// awaitDeadline waits in tk until done fires or d passes on clock. It
// reports whether the deadline passed first. onFire, when set, runs as
// the deadline passes (in the timer callback, before tk is woken). When
// tk is canceled first, the scope s is shut down and tk unwinds.
func awaitDeadline(tk *Task, s *Scope, done *event, clock Clock, d Duration, onFire func()) bool {
	timerCh := make(chan struct{})
	// claimed is taken by the timer when it fires, or by the waiting task
	// when it leaves first (then the timer can no longer wake it late)
	var claimed, fired atomic.Bool
	stop := clockAfter(tk, clock, d, func(locked bool) {
		if claimed.CompareAndSwap(false, true) {
			fired.Store(true)
			if onFire != nil {
				onFire()
			}
			if locked {
				tk.w.wakeLocked(tk)
			} else {
				wakeTask(tk)
			}
			close(timerCh)
		}
	})
	done.mu.Lock()
	if !done.fired {
		done.waiters = append(done.waiters, tk)
	}
	done.mu.Unlock()
	tk.blockOn(waitTimeout, s, true)
	select {
	case <-done.ch:
	case <-timerCh:
	case <-tk.Done():
	}
	if !claimed.CompareAndSwap(false, true) {
		// the timer fired: let its wakeup land before the wait ends
		<-timerCh
	}
	// decided from the state, not from which case select chose
	canceled := tk.Canceled()
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
	return realAfter(d, func() { f(false) })
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
	h := spawnUnder(tk, clock, d, func() *Handle[Unit, Never] { return Spawn(tk, s, func(t *Task) Result[Unit, Never] {
		mapConcurrent(t, items, limit, func(t *Task, i int, x T) {
			r := f(t, x)
			mu.Lock()
			if !expired {
				res[i], fin[i] = r, true
			}
			mu.Unlock()
		})
		return Ok[Unit, Never](Unit{})
	}, false, true) })
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
