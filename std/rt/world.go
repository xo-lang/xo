package rt

import (
	"container/heap"
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
}

// NewWorld returns a world whose virtual clock starts at
// 2026-01-01T00:00:00Z (std.md 8.2).
func NewWorld() *World {
	w := &World{blocked: map[*Task]bool{}, pending: map[*Task]bool{}, stuck: make(chan struct{}), now: Time(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano())}
	w.cond = sync.NewCond(&w.mu)
	return w
}

// NewTask returns a root task that lives in the world and counts as
// running.
func (w *World) NewTask() *Task {
	w.mu.Lock()
	w.running++
	w.mu.Unlock()
	return &Task{node: newNode(nil, w), w: w}
}

// EndTask marks a root task of the world as finished.
func (w *World) EndTask(*Task) { w.taskEnded() }

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
	tk.CheckCancel()
	w.mu.Lock()
	w.sleeps = append(w.sleeps, d)
	w.mu.Unlock()
	ch, stop := w.timer(tk, d)
	tk.block()
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
