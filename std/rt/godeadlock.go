package rt

import (
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// Deadlocks through Go calls (decision 0073). A task inside a Go call
// (`use go`) is running Go code, which no Xo wait record describes: Go may
// be reading the network, sleeping, or waiting for an Xo callback that Go
// runs on a goroutine of its own (sync.WaitGroup.Go then Wait). The last
// case is a deadlock when that callback is itself blocked on an Xo
// primitive that only the calling task (or another stuck task) can
// release. The runtime reports it only when it can show it:
//
//  1. Every live thread is blocked on an Xo primitive, or is inside a Go
//     call while it has foreign callback threads alive and all of those
//     are blocked on Xo primitives or are such threads in turn (a
//     fixpoint, goWaitersOf).
//  2. Every goroutine of the process (but the one checking) is parked
//     where only another goroutine of the program can wake it (goQuiet):
//     Xo runtime goroutines anywhere but running or in a system call, and
//     goroutines inside a Go call only in a `sync` package wait
//     (WaitGroup.Wait, Mutex.Lock, RWMutex, Cond.Wait). A goroutine that
//     runs, sleeps, waits for I/O or a system call, waits on a channel or
//     select inside Go code (which may be a timer), or belongs to Go code
//     with no Xo frame at all (a library's own goroutine) means something
//     may still happen, so nothing is reported.
//  3. No Go runtime timer is pending (gotimers_on.go): a timer runs no
//     goroutine until it fires, so time.AfterFunc calling WaitGroup.Done
//     shows in no stack. While one is pending the check waits past its
//     deadline and looks again. The runtime's own watchers sleep in a
//     system call (osSleep), not on Go timers, so they never hold it off.
//     Builds that cannot see Go timers (plain go test, targets other than
//     darwin and linux) never report a deadlock through a Go call.
//
// Steps 2 and 3 run twice in a row with a yield between them
// (goStuckProof), so a timer made and fired between two looks shows as a
// moving goroutine. Under the deterministic scheduler, the proof also
// needs the scheduler to be still around it: no callback call or Go call
// return waiting for admission, no decision or lending in progress, no
// turn handed out (confirmGoLocked). A Go timer that fired runs its
// function on a goroutine that becomes an arrival, and the arrival
// waits for Go's goroutines to settle (sleeping in osSleep, which the
// classifier otherwise ignores) before it is admitted; a proof taken in
// that window saw no timer, parked goroutines, and the old threads, and
// reported a correct program (update 2026-10-08).

// goCallWait is what a thread inside a Go call waits for: its foreign
// callbacks. top is the innermost Go function of the call
// (`sync.(*WaitGroup).Wait`), when the goroutine profile shows it.
type goCallWait struct {
	top string
	cbs []*thread
}

func (g *goCallWait) waitDesc() (string, []*thread) {
	cbs := append([]*thread(nil), g.cbs...)
	sort.Slice(cbs, func(i, j int) bool { return cbs[i].id < cbs[j].id })
	names := make([]string, len(cbs))
	for i, c := range cbs {
		names[i] = c.shortLabel()
	}
	return "for " + strings.Join(names, ", "), cbs
}

// goWaitersOf returns the threads inside a Go call that wait only for
// stuck foreign callbacks, each with its live callbacks. cbs are the live
// foreign callback threads; blocked reports a thread blocked on an Xo
// primitive that only another task can end.
func goWaitersOf(cbs []*thread, blocked func(*thread) bool) map[*thread][]*thread {
	by := map[*thread][]*thread{}
	for _, c := range cbs {
		if o := c.cbOf; o != nil && o.inGo.Load() > 0 {
			by[o] = append(by[o], c)
		}
	}
	in := map[*thread]bool{}
	for changed := true; changed; {
		changed = false
		for o, cs := range by {
			if in[o] {
				continue
			}
			all := true
			for _, c := range cs {
				if !in[c] && !blocked(c) {
					all = false
					break
				}
			}
			if all {
				in[o] = true
				changed = true
			}
		}
	}
	out := make(map[*thread][]*thread, len(in))
	for o := range in {
		out[o] = by[o]
	}
	return out
}

// goWaitSnaps adds a wait record for each thread in ws (inside a Go call,
// waiting for its callbacks) to snaps, located at the innermost Xo frame
// of its goroutine, and sorts them by thread id.
func goWaitSnaps(snaps []waitSnap, ws map[*thread][]*thread, info map[int64]stackInfo) []waitSnap {
	for o, cbs := range ws {
		in := info[o.id]
		snaps = append(snaps, waitSnap{th: o, kind: waitGoCall, obj: &goCallWait{top: in.top, cbs: cbs}, loc: in.loc})
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].th.id < snaps[j].th.id })
	return snaps
}

// rtPrefix is the prefix of the functions of this package in stack
// traces ("xoprog/rt.").
var rtPrefix = func() string {
	name := runtime.FuncForPC(reflect.ValueOf(rtAnchor).Pointer()).Name()
	return strings.TrimSuffix(name, "rtAnchor")
}()

func rtAnchor() {}

// syncWaits are the goroutine states of waits in package sync, which only
// another goroutine ends.
var syncWaits = map[string]bool{
	"semacquire":          true,
	"sync.WaitGroup.Wait": true,
	"sync.Mutex.Lock":     true,
	"sync.RWMutex.Lock":   true,
	"sync.RWMutex.RLock":  true,
	"sync.Cond.Wait":      true,
}

// goNextTimerHook stands in for goNextTimerRuntime in tests.
var goNextTimerHook atomic.Pointer[func() (time.Duration, bool, bool)]

// goTimersClear reports whether no Go timer is pending; otherwise how long
// to wait before looking again (past the earliest timer, or a while when
// timers cannot be seen in this build).
func goTimersClear() (bool, time.Duration) {
	d, pending, known := goNextTimerRuntime()
	if h := goNextTimerHook.Load(); h != nil {
		d, pending, known = (*h)()
	}
	switch {
	case !known:
		return false, 2 * time.Second
	case pending:
		return false, max(d, 0) + 5*time.Millisecond
	}
	return true, 0
}

// goStuckProof is the proof of the top of the file at one moment: no Go
// timer pending and every goroutine parked, twice in a row, so a timer
// made and fired between two looks shows as a moving goroutine. retry
// is set when a Go timer is pending: look again after it.
func goStuckProof() (ok bool, retry time.Duration) {
	for i := 0; i < 2; i++ {
		if i > 0 {
			// let a goroutine readied since the first look run, or show
			// as runnable
			runtime.Gosched()
		}
		if clear, wait := goTimersClear(); !clear {
			return false, wait
		}
		if !goQuiet() {
			return false, 0
		}
	}
	return true, 0
}

// goQuiet reports whether every goroutine but the caller is parked where
// only another goroutine of the program can wake it (see the top of the
// file). It stops the world briefly to read every stack.
func goQuiet() bool {
	buf := make([]byte, 1<<16)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	gs := strings.Split(string(buf), "\n\n")
	// the first goroutine is the caller
	for _, g := range gs[1:] {
		if !goParked(g) {
			return false
		}
	}
	return true
}

// goParked classifies one goroutine of a runtime.Stack dump for goQuiet.
func goParked(g string) bool {
	lines := strings.Split(strings.TrimSpace(g), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "goroutine ") {
		return true
	}
	state := lines[0]
	if i := strings.IndexByte(state, '['); i >= 0 {
		state = state[i+1:]
		if j := strings.IndexAny(state, ",]"); j >= 0 {
			state = state[:j]
		}
	}
	if strings.Contains(g, rtPrefix+"waitGoSettled(") {
		// a decision or a lend of the deterministic scheduler waiting
		// for Go's goroutines to settle, often in osSleep: it is about
		// to admit what arrived
		return false
	}
	moving := false
	switch state {
	case "running", "runnable", "syscall", "IO wait", "preempted":
		moving = true
	}
	ours := false
	top := ""
	// frames: a function line, then its file line
	for i := 1; i < len(lines); i += 2 {
		l := lines[i]
		if strings.HasPrefix(l, "created by ") {
			break
		}
		file := ""
		if i+1 < len(lines) {
			file = lines[i+1]
		}
		fn := l
		if k := strings.LastIndexByte(fn, '('); k > 0 && strings.HasSuffix(fn, ")") {
			fn = fn[:k]
		}
		switch {
		case strings.HasPrefix(fn, "os/signal."), strings.HasPrefix(fn, "testing.(*M)."), strings.HasPrefix(fn, "testing.(*T).Run"):
			// waits for signals (which deadlock detection ignores), the
			// go test driver
			return true
		case strings.HasPrefix(fn, rtPrefix):
			if fn == rtPrefix+"osSleep" {
				// a watcher of the runtime (the replay's, the monitor's,
				// a test's time limit) sleeping between looks
				return true
			}
			ours = true
			if top == "" {
				top = "rt"
			}
		case strings.Contains(file, ".xo:"):
			ours = true
			if top == "" {
				top = "xo"
			}
		case top == "" && !strings.HasPrefix(fn, "runtime.") && !strings.HasPrefix(fn, "internal/") && !strings.HasPrefix(fn, "sync.runtime_"):
			top = fn
		}
	}
	if moving || !ours {
		// running, in I/O or a system call, or Go code with no Xo frame
		// (a library's goroutine, which a timer or the network may wake)
		return false
	}
	switch top {
	case "rt", "xo", "":
		// parked in the Xo runtime: its waits are counted by the monitor
		// or the scheduler
		return true
	}
	// inside a Go call or Go code under a callback: only a sync wait
	return syncWaits[state] && strings.HasPrefix(top, "sync.")
}

// goStuckLocked is called with no thread running or ready: it returns
// the threads lent to Go calls when every thread is blocked on an Xo
// primitive or lent to a Go call that waits only for stuck callbacks, and
// nil otherwise (a thread waiting on the outside world, a Go call with no
// stuck callback).
func (s *sched) goStuckLocked() map[*thread][]*thread {
	var cbs, lent []*thread
	for _, th := range s.threads {
		switch {
		case th.st == stBlocked:
		case th.lent.Load():
			lent = append(lent, th)
		default:
			return nil
		}
		if th.cbOf != nil {
			cbs = append(cbs, th)
		}
	}
	if len(lent) == 0 {
		return nil
	}
	ws := goWaitersOf(cbs, func(th *thread) bool { return th.st == stBlocked })
	for _, th := range lent {
		if ws[th] == nil {
			return nil
		}
	}
	return ws
}

// confirmGoLocked starts a goroutine that reports the deadlock of a test
// world (closes stuck) once the goroutines show that the Go calls of
// goStuckLocked cannot return, unless a thread runs again first.
func (s *sched) confirmGoLocked() {
	if s.confirming || s.w == nil {
		return
	}
	s.confirming = true
	moves := s.moves
	go func() {
		d := 2 * time.Millisecond
		for {
			osSleep(d)
			s.mu.Lock()
			still := s.stillLocked()
			s.mu.Unlock()
			q, retry := goStuckProof()
			s.mu.Lock()
			if s.moves != moves || s.cur != nil || len(s.ready) > 0 {
				s.confirming = false
				s.mu.Unlock()
				return
			}
			ws := s.goStuckLocked()
			if ws == nil {
				s.confirming = false
				s.mu.Unlock()
				return
			}
			// the scheduler must have been still before and after the
			// proof, with no turn handed out in between
			if q && still && s.stillLocked() && s.moves == moves {
				s.goDead = ws
				s.confirming = false
				w := s.w
				s.mu.Unlock()
				w.stuckOnce.Do(func() { close(w.stuck) })
				return
			}
			s.mu.Unlock()
			d = min(2*d, 100*time.Millisecond)
			if retry > 0 {
				// a Go timer is pending: look again once it fired
				d = min(retry, 2*time.Second)
			}
		}
	}()
}

// stillLocked reports whether nothing waits for admission and no
// decision or lend is in progress: a callback call that arrived (a Go
// timer's function, say) is not a thread yet, so goStuckLocked cannot
// see it, but it will run.
func (s *sched) stillLocked() bool {
	return !s.picking && !s.lending && len(s.pend) == 0 && len(s.pendRet) == 0
}

// osAfter returns a channel closed after d, timed with osSleep (no Go
// timer), unless stop is closed first; the goroutine ends within 50 ms
// of either.
func osAfter(d time.Duration, stop <-chan struct{}) <-chan struct{} {
	c := make(chan struct{})
	end := time.Now().Add(d)
	go func() {
		for {
			left := time.Until(end)
			if left <= 0 {
				close(c)
				return
			}
			select {
			case <-stop:
				return
			default:
			}
			osSleep(min(left, 50*time.Millisecond))
		}
	}()
	return c
}
