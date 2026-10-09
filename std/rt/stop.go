package rt

import (
	"sync"
	"sync/atomic"
	"time"
)

// DefaultGrace is the time between a stop request and the cancellation of
// main's root scope unless `os.proc.set_grace(d)` says otherwise (core 7.5).
const DefaultGrace = Duration(30 * time.Second)

// stopState is the two phase shutdown state of one program run (core 7.5):
// RunMain or testing.run_main. Every task of the run points to it
// (Task.stop).
//
// Signals are counted: the first requests a stop (stop_requested is true,
// the stopping channel and ev are closed, the grace timer starts), the
// second cancels main's root scope, the third exits with status 130. The
// grace timer cancels the root scope when it fires first.
//
// Locking: st.mu may be held while taking channel, event, cancellation
// tree, and world locks, never the other way around. Timers of a test
// world therefore never touch st from their callback (which holds the
// world lock): they start a goroutine counted as a running task (see
// after), so the virtual clock cannot move until it is done.
type stopState struct {
	w *World // the test world, nil in a real program

	mu        sync.Mutex
	requested atomic.Bool
	canceled  atomic.Bool // the stop state canceled main's root scope
	exited    atomic.Bool // a third signal arrived
	ch        *Chan[Unit] // task.stopping()
	ev        *event      // fired with ch, for runtime waiters (serve)
	grace     Duration
	stopAt    int64  // when the stop was requested (virtual or monotonic ns)
	graceStop func() // stops the pending grace timer
	signals   int
	finished  bool
	timers    []func()

	cancelRoot func() // cancels main's root scope
	exit       func() // exits the process with status 130 (real programs)
	proc       *fakeProc
}

func newStopState(w *World) *stopState {
	return &stopState{w: w, ch: &Chan[Unit]{ext: true}, ev: newEvent(), grace: DefaultGrace}
}

// now is the time on the run's clock.
func (st *stopState) now() int64 {
	if st.w != nil {
		return int64(st.w.Now())
	}
	return fxMono()
}

// after calls f with st.mu held after d on the run's clock, unless the run
// has finished. It returns a function that stops the timer. In a test
// world, f runs on its own goroutine that counts as a running task.
func (st *stopState) after(d Duration, f func()) func() {
	run := func() {
		st.mu.Lock()
		defer st.mu.Unlock()
		if !st.finished {
			f()
		}
	}
	if st.w != nil {
		w := st.w
		return w.timerFunc(d, func() {
			// w.mu is held here: the goroutine counts as running before it
			// starts, so the clock waits for it
			w.helperLocked(run)
		})
	}
	return realAfter(d, run)
}

// signal delivers SIGINT or SIGTERM.
func (st *stopState) signal() {
	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.finished {
		st.signalLocked()
	}
}

func (st *stopState) signalLocked() {
	st.signals++
	switch st.signals {
	case 1:
		st.requestLocked()
	case 2:
		st.cancelLocked()
	case 3:
		st.exited.Store(true)
		st.cancelLocked()
		if st.exit != nil {
			st.exit()
		}
	}
}

// requestLocked enters phase 1: stop requested.
func (st *stopState) requestLocked() {
	if st.requested.Load() {
		return
	}
	st.stopAt = st.now()
	st.requested.Store(true)
	Chan_close(nil, st.ch)
	st.ev.fire()
	st.armGraceLocked()
}

// armGraceLocked (re)starts the grace timer for the time left of the grace
// period.
func (st *stopState) armGraceLocked() {
	if st.graceStop != nil {
		st.graceStop()
		st.graceStop = nil
	}
	if st.canceled.Load() {
		return
	}
	left := st.grace - Duration(st.now()-st.stopAt)
	if left <= 0 {
		st.cancelLocked()
		return
	}
	st.graceStop = st.after(left, st.cancelLocked)
}

// cancelLocked enters phase 2: main's root scope is canceled.
func (st *stopState) cancelLocked() {
	if st.canceled.Load() || st.cancelRoot == nil {
		return
	}
	st.canceled.Store(true)
	st.cancelRoot()
}

// setGrace is `os.proc.set_grace(d)`. After a stop was requested it moves
// the deadline to stop time + d.
func (st *stopState) setGrace(d Duration) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.grace = max(d, 0)
	if st.requested.Load() && !st.finished {
		st.armGraceLocked()
	}
}

// signalAfter delivers a signal after d (FakeProc.signal_after).
func (st *stopState) signalAfter(d Duration) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.finished {
		return
	}
	st.timers = append(st.timers, st.after(d, st.signalLocked))
}

// finish ends the run: pending timers are stopped and later signals are
// ignored.
func (st *stopState) finish() {
	st.mu.Lock()
	st.finished = true
	stops := st.timers
	st.timers = nil
	if st.graceStop != nil {
		stops = append(stops, st.graceStop)
		st.graceStop = nil
	}
	st.mu.Unlock()
	for _, f := range stops {
		f()
	}
}

// Task_stop_requested is `task.stop_requested()` (core 7.5).
// It is a scheduling point, like task.is_canceled().
func Task_stop_requested(tk *Task) bool {
	tk.Yield()
	return tk.stop != nil && tk.stop.requested.Load()
}

// Task_stopping is `task.stopping()`: a channel closed when a stop is
// requested. Outside a program run it is a channel that never closes.
func Task_stopping(tk *Task) *Chan[Unit] {
	if tk.stop == nil {
		return &Chan[Unit]{ext: true}
	}
	return tk.stop.ch
}

// stopDone returns a channel closed when a stop is requested, or nil.
func (tk *Task) stopDone() <-chan struct{} {
	if tk.stop == nil {
		return nil
	}
	return tk.stop.ev.ch
}
