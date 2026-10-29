package rt

import (
	"os"
	"os/signal"
	"sync"
)

// Proc.notify, stop_notify and request_stop (decision 0407): Go's
// signal.Notify per subscription, forwarding into the Xo channel without
// blocking (a signal of a kind already pending is dropped, as Go drops a
// send to a full channel). While a subscription lists Interrupt or
// Terminate, main.go's watcher leaves that signal to it.

// sigSub is one subscription.
type sigSub struct {
	ch   *Chan[Signal]
	mask int
	goch chan os.Signal
}

var sigSubs struct {
	mu   sync.Mutex
	subs []*sigSub
}

// Signal tags are 1 based (the generated enum: Hangup is 1); sigSystem
// and the masks are indexed from 0.

// sigIndex is the 0 based index of a system signal, or -1.
func sigIndex(s os.Signal) int {
	for i, x := range sigSystem {
		if x != nil && x == s {
			return i
		}
	}
	return -1
}

// sigTaken reports whether a subscription takes the signal s (main.go
// skips the shutdown for it).
func sigTaken(s os.Signal) bool {
	i := sigIndex(s)
	if i < 0 {
		return false
	}
	sigSubs.mu.Lock()
	defer sigSubs.mu.Unlock()
	for _, x := range sigSubs.subs {
		if x.mask&(1<<i) != 0 {
			return true
		}
	}
	return false
}

// requestStop is core 7.5's phase 1 from the program, set by main.go.
var requestStop func()

func procNotify(tk *Task, signals List[Signal]) *Chan[Signal] {
	n := int64(len(signals.s))
	if n == 0 {
		n = 1
	}
	c := Chan_new[Signal](tk, n)
	// fed from outside the program's tasks: a receive on it is not a
	// deadlock (as task.stopping()'s channel)
	c.ext = true
	sub := &sigSub{ch: c, goch: make(chan os.Signal, 8)}
	var sys []os.Signal
	for _, s := range signals.s {
		i := s.Tag - 1
		if i < 0 || i >= len(sigSystem) {
			continue
		}
		sub.mask |= 1 << i
		if x := sigSystem[i]; x != nil {
			sys = append(sys, x)
		}
	}
	sigSubs.mu.Lock()
	sigSubs.subs = append(sigSubs.subs, sub)
	sigSubs.mu.Unlock()
	if len(sys) > 0 {
		signal.Notify(sub.goch, sys...)
		go func() {
			for s := range sub.goch {
				if i := sigIndex(s); i >= 0 {
					Chan_try_send(nil, c, Signal{Tag: i + 1})
				}
			}
		}()
	}
	return c
}

func procStopNotify(tk *Task, c *Chan[Signal]) {
	sigSubs.mu.Lock()
	var found *sigSub
	for k, x := range sigSubs.subs {
		if x.ch == c {
			found = x
			sigSubs.subs = append(sigSubs.subs[:k], sigSubs.subs[k+1:]...)
			break
		}
	}
	sigSubs.mu.Unlock()
	if found == nil {
		return
	}
	signal.Stop(found.goch)
	close(found.goch)
	Chan_close(tk, c)
}

// M_notify is `os.proc.notify(signals)`.
func (realProc) M_notify(tk *Task, signals List[Signal]) *Chan[Signal] {
	return procNotify(tk, signals)
}

// M_stop_notify is `os.proc.stop_notify(c)`.
func (realProc) M_stop_notify(tk *Task, c *Chan[Signal]) { procStopNotify(tk, c) }

// M_request_stop is `os.proc.request_stop()`.
func (realProc) M_request_stop(tk *Task) {
	if f := requestStop; f != nil {
		f()
	}
}

func (p *onlyProc) M_notify(tk *Task, signals List[Signal]) *Chan[Signal] {
	return p.inner.M_notify(tk, signals)
}
func (p *onlyProc) M_stop_notify(tk *Task, c *Chan[Signal]) { p.inner.M_stop_notify(tk, c) }
func (p *onlyProc) M_request_stop(tk *Task)                 { p.inner.M_request_stop(tk) }

func (f Testing_FakeProc) M_notify(tk *Task, signals List[Signal]) *Chan[Signal] {
	if f.p.real != nil {
		return f.p.real.M_notify(tk, signals)
	}
	return Chan_new[Signal](tk, 1)
}
func (f Testing_FakeProc) M_stop_notify(tk *Task, c *Chan[Signal]) {
	if f.p.real != nil {
		f.p.real.M_stop_notify(tk, c)
		return
	}
	Chan_close(tk, c)
}
func (f Testing_FakeProc) M_request_stop(tk *Task) {
	if f.p.real != nil {
		f.p.real.M_request_stop(tk)
	}
}
