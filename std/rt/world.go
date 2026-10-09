package rt

import (
	"container/heap"
	"sort"
	"sync"
	"time"
)

// World is the world of one test (std.md 8): the virtual clock and the
// bookkeeping that lets it auto advance. When every task of the test is
// blocked, time jumps to the next timer (std.md 8.2).
//
// Accounting: `running` counts tasks that are not blocked. A task that
// blocks decrements it. Whoever wakes a blocked task (a timer, a channel
// operation, a released mutex, a finished task, a cancellation) counts the
// woken task as running again before it is signalled, so the clock never
// moves while a woken task has not yet run. A woken task's own unblock is
// then a no op.
type World struct {
	mu        sync.Mutex
	cond      *sync.Cond
	running   int
	blocked   map[*Task]bool
	pending   map[*Task]bool // woken before they blocked
	now       Time
	timers    timerHeap
	seq       int
	sleeps    []Duration
	advancing bool
	log       *logSink // log configuration of the test, if configured
	// stuck is closed when every task is blocked and no timer is pending:
	// the test can never finish (a deadlock).
	stuck     chan struct{}
	stuckOnce sync.Once
	// sch is the deterministic scheduler (core 7.4, sched.go), nil when
	// the test's goroutines run freely (tests with real capabilities). It
	// shares mu and replaces running, blocked, and pending.
	sch *sched
	// FakeClock.advance under the scheduler: the thread that advances and
	// the target time
	adv       *thread
	advTarget Time
}

// NewSchedWorld returns a world whose tasks run on the deterministic
// scheduler with the given seed (core 7.4).
func NewSchedWorld(seed int64) *World {
	w := NewWorld()
	w.sch = newSched(&w.mu, w, seed)
	return w
}

// NewWorld returns a world whose virtual clock starts at
// 2026-01-01T00:00:00Z (std.md 8.2).
func NewWorld() *World {
	w := &World{blocked: map[*Task]bool{}, pending: map[*Task]bool{}, stuck: make(chan struct{}), now: Time(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano())}
	w.cond = sync.NewCond(&w.mu)
	return w
}

// NewTask returns a root task that lives in the world and counts as
// running. Under the scheduler it returns once the task has the turn.
func (w *World) NewTask() *Task {
	tk := &Task{w: w, th: newRootThread("test")}
	tk.node = newNodeOf(nil, w, nodeRoot, tk)
	if s := w.sch; s != nil {
		s.start(tk.th)
		<-tk.th.turn
		return tk
	}
	w.mu.Lock()
	w.running++
	w.mu.Unlock()
	return tk
}

// EndTask marks a root task of the world as finished.
func (w *World) EndTask(tk *Task) {
	tk.th.mu.Lock()
	tk.th.done = true
	tk.th.mu.Unlock()
	if s := w.sch; s != nil {
		s.end(tk.th)
		return
	}
	w.taskEnded()
}

// helperLocked runs f on a new goroutine that counts as a running task, so
// the virtual clock waits for it (stopState.after). w.mu is held. Under
// the scheduler the goroutine is a thread that waits for its turn.
func (w *World) helperLocked(f func()) {
	if s := w.sch; s != nil {
		var parent *thread
		if len(s.threads) > 0 {
			parent = s.threads[0]
		}
		var th *thread
		if parent != nil {
			th = parent.newChild("runtime")
		} else {
			th = newRootThread("runtime")
		}
		s.startLocked(th)
		go func() {
			<-th.turn
			defer s.end(th)
			f()
		}()
		return
	}
	w.running++
	go func() {
		defer w.taskEnded()
		f()
	}()
}

// schedIdleLocked is called by the scheduler when no thread is ready: it
// ends a FakeClock.advance or fires the next timer and reports whether
// that may have made a thread ready. With nothing left to fire while
// threads are alive and none is lent to a Go call, every thread is
// blocked for good: a deadlock.
func (w *World) schedIdleLocked() bool {
	w.dropDeadLocked()
	if w.adv != nil {
		if len(w.timers) > 0 && w.timers[0].at <= w.advTarget {
			w.fireNextLocked()
			return true
		}
		if w.now < w.advTarget {
			w.now = w.advTarget
		}
		th := w.adv
		w.adv = nil
		th.st = stBlocked
		w.sch.wakeLocked(th)
		return true
	}
	if len(w.timers) > 0 {
		w.fireNextLocked()
		return true
	}
	// a thread lent while in a Go call (gointerop.go) comes back when the
	// call returns: not a deadlock, unless the call waits only for
	// callbacks that are stuck and the goroutines show it cannot return
	// (godeadlock.go, decision 0070)
	if len(w.sch.threads) > 0 {
		if w.sch.lentN == 0 {
			w.stuckOnce.Do(func() { close(w.stuck) })
		} else if w.sch.goStuckLocked() != nil {
			w.sch.confirmGoLocked()
		}
	}
	return false
}

// DeadlockFault is the XO-F001 fault of a world whose tasks are all
// blocked (after Stuck is closed): the wait cycle, or every blocked task.
func (w *World) DeadlockFault() *FaultErr {
	w.mu.Lock()
	var snaps []waitSnap
	var goDead map[*thread][]*thread
	if s := w.sch; s != nil {
		snaps = s.snapsLocked()
		goDead = s.goDead
	} else {
		seen := map[*thread]bool{}
		for tk := range w.blocked {
			if !seen[tk.th] {
				seen[tk.th] = true
				if sn, ok := tk.th.snap(nil); ok {
					snaps = append(snaps, sn)
				}
			}
		}
	}
	w.mu.Unlock()
	if len(goDead) > 0 {
		snaps = goWaitSnaps(snaps, goDead, lazyStacks())
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].th.id < snaps[j].th.id })
	return deadlockFault(snaps, "every task is blocked and no timer is pending")
}

// Stuck is closed when every task of the world is blocked for good.
func (w *World) Stuck() <-chan struct{} { return w.stuck }

func (w *World) taskStarted() {
	w.mu.Lock()
	w.running++
	w.mu.Unlock()
}

func (w *World) taskEnded() {
	w.mu.Lock()
	w.running--
	w.changedLocked()
	w.mu.Unlock()
}

func (w *World) block(tk *Task) {
	w.mu.Lock()
	if w.pending[tk] {
		// a waker already counted this task as running
		delete(w.pending, tk)
		w.mu.Unlock()
		return
	}
	if !w.blocked[tk] {
		w.blocked[tk] = true
		w.running--
	}
	w.changedLocked()
	w.mu.Unlock()
}

func (w *World) unblock(tk *Task) {
	w.mu.Lock()
	if w.blocked[tk] {
		delete(w.blocked, tk)
		w.running++
		w.cond.Broadcast()
	}
	delete(w.pending, tk)
	w.mu.Unlock()
}

// wake counts a blocked task as running; called by the waker.
func (w *World) wake(tk *Task) {
	if w == nil || tk == nil {
		return
	}
	w.mu.Lock()
	w.wakeLocked(tk)
	w.mu.Unlock()
}

func (w *World) wakeLocked(tk *Task) {
	if s := w.sch; s != nil {
		s.wakeLocked(tk.th)
		return
	}
	if w.blocked[tk] {
		delete(w.blocked, tk)
		w.running++
		w.cond.Broadcast()
		return
	}
	w.pending[tk] = true
}

// wakeCanceled counts every task blocked below the canceled nodes as
// running: they are about to unwind.
func (w *World) wakeCanceled(nodes []*cnode) {
	w.mu.Lock()
	w.wakeCanceledLocked(nodes)
	w.mu.Unlock()
}

// wakeCanceledLocked is wakeCanceled with w.mu already held.
func (w *World) wakeCanceledLocked(nodes []*cnode) {
	set := make(map[*cnode]bool, len(nodes))
	for _, n := range nodes {
		set[n] = true
	}
	if s := w.sch; s != nil {
		s.wakeCanceledLocked(set)
		return
	}
	for tk := range w.blocked {
		if tk.node.under(set) {
			w.wakeLocked(tk)
		}
	}
}

// changedLocked advances the clock when every task is blocked.
func (w *World) changedLocked() {
	w.cond.Broadcast()
	if w.advancing {
		return
	}
	for w.running <= 0 {
		w.dropDeadLocked()
		if len(w.timers) == 0 {
			if len(w.blocked) > 0 {
				w.stuckOnce.Do(func() { close(w.stuck) })
			}
			return
		}
		w.fireNextLocked()
	}
}

// dropDeadLocked removes stopped timers from the top of the heap.
func (w *World) dropDeadLocked() {
	for len(w.timers) > 0 && w.timers[0].dead {
		heap.Pop(&w.timers)
	}
}

// fireNextLocked fires every timer due at the earliest instant.
func (w *World) fireNextLocked() {
	w.dropDeadLocked()
	if len(w.timers) == 0 {
		return
	}
	at := w.timers[0].at
	if at > w.now {
		w.now = at
	}
	for len(w.timers) > 0 && w.timers[0].at <= at {
		t := heap.Pop(&w.timers).(*vtimer)
		if !t.dead {
			t.dead = true
			t.fire()
		}
	}
}

// Now is the virtual time.
func (w *World) Now() Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.now
}

// SetNow sets the virtual time (FakeClock.set).
func (w *World) SetNow(t Time) {
	w.mu.Lock()
	w.now = t
	w.mu.Unlock()
}

// timer returns a channel closed when d has passed on the virtual clock.
// If tk is blocked waiting for it, the firing counts tk as running.
func (w *World) timer(tk *Task, d Duration) (<-chan struct{}, func()) {
	ch := make(chan struct{})
	stop := w.timerFunc(d, func() {
		w.wakeLocked(tk)
		close(ch)
	})
	return ch, stop
}

// timerFunc calls f, with the world locked, when d has passed on the
// virtual clock. The returned function cancels the timer.
func (w *World) timerFunc(d Duration, f func()) func() {
	w.mu.Lock()
	defer w.mu.Unlock()
	t := &vtimer{at: w.now + Time(max(d, 0)), seq: w.seq, fire: f}
	w.seq++
	heap.Push(&w.timers, t)
	return func() {
		w.mu.Lock()
		t.dead = true
		w.mu.Unlock()
	}
}

// Advance is FakeClock.advance(d): fires every timer due within d, in
// order, letting the woken tasks run (until every other task is blocked
// again) after each instant.
func (w *World) Advance(tk *Task, d Duration) {
	if s := w.sch; s != nil {
		// the thread gives up the turn; when no other thread is ready, the
		// scheduler fires the timers due by the target, then wakes it
		th := tk.th
		th.setWait(tk, waitAdvance, nil)
		w.mu.Lock()
		w.adv = th
		w.advTarget = w.now + Time(max(d, 0))
		th.st = stOutside
		if s.cur == th {
			s.cur = nil
			s.pickLocked()
		}
		w.mu.Unlock()
		<-th.turn
		th.clearWait()
		return
	}
	w.mu.Lock()
	target := w.now + Time(max(d, 0))
	w.advancing = true
	w.blocked[tk] = true
	w.running--
	for {
		for w.running > 0 {
			w.cond.Wait()
		}
		w.dropDeadLocked()
		if len(w.timers) == 0 || w.timers[0].at > target {
			break
		}
		w.fireNextLocked()
	}
	if w.now < target {
		w.now = target
	}
	w.advancing = false
	if w.blocked[tk] {
		delete(w.blocked, tk)
		w.running++
	}
	w.mu.Unlock()
}

// Sleep blocks tk for d of virtual time (FakeClock.sleep). It is a
// cancellation point that unwinds.
func (w *World) Sleep(tk *Task, d Duration) {
	tk.Yield()
	tk.CheckCancel()
	w.mu.Lock()
	w.sleeps = append(w.sleeps, d)
	w.mu.Unlock()
	ch, stop := w.timer(tk, d)
	tk.blockOn(waitSleep, nil, true)
	// the timer wins over a cancellation at the same instant
	select {
	case <-ch:
		tk.unblock()
		return
	default:
	}
	select {
	case <-ch:
		tk.unblock()
	case <-tk.Done():
		stop()
		tk.unblock()
		tk.Unwind()
	}
}

// Sleeps is FakeClock.sleeps().
func (w *World) Sleeps() []Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]Duration(nil), w.sleeps...)
}

type vtimer struct {
	at   Time
	seq  int
	fire func()
	dead bool
}

type timerHeap []*vtimer

func (h timerHeap) Len() int { return len(h) }
func (h timerHeap) Less(i, j int) bool {
	if h[i].at != h[j].at {
		return h[i].at < h[j].at
	}
	return h[i].seq < h[j].seq
}
func (h timerHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *timerHeap) Push(x any)   { *h = append(*h, x.(*vtimer)) }
func (h *timerHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

var monoStart = time.Now()

func monoNow() int64 { return int64(time.Since(monoStart)) }
