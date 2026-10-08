package rt

import (
	"runtime"
	"strings"
	"sync"
)

// Chan is a channel handle `Chan[T]` (core 7.3). Copies share the
// channel. It keeps explicit queues of blocked tasks so a waker can count
// the task it wakes as running (test world, std.md 8.2) and so `select`
// can wait on several channels.
type Chan[T any] struct {
	mu     sync.Mutex
	buf    []T
	cap    int
	closed bool
	recvq  []*recvEnt[T]
	sendq  []*sendEnt[T]
	made   [3]uintptr // where Chan.new was called, for deadlock reports
	ext    bool       // closed from outside the program's tasks (task.stopping())
}

type recvEnt[T any] struct {
	w   *waiter
	idx int
	set func(v T, ok bool)
}

type sendEnt[T any] struct {
	w   *waiter
	idx int
	v   T
}

// Chan_new is `Chan[T].new(cap)`.
func Chan_new[T any](_ *Task, capacity int64) *Chan[T] {
	if capacity < 0 {
		FaultHere("negative channel capacity %d", capacity)
	}
	c := &Chan[T]{cap: int(capacity)}
	runtime.Callers(2, c.made[:])
	return c
}

// XoDebug implements Debug.
func (c *Chan[T]) XoDebug(b *strings.Builder) { b.WriteString("<chan>") }

// takeRecv pops the first unclaimed waiting receiver. Caller holds c.mu.
func (c *Chan[T]) takeRecv() *recvEnt[T] {
	for len(c.recvq) > 0 {
		r := c.recvq[0]
		c.recvq = c.recvq[1:]
		if r.w.claim() {
			return r
		}
	}
	return nil
}

// takeSend pops the first unclaimed waiting sender. Caller holds c.mu.
func (c *Chan[T]) takeSend() *sendEnt[T] {
	for len(c.sendq) > 0 {
		s := c.sendq[0]
		c.sendq = c.sendq[1:]
		if s.w.claim() {
			return s
		}
	}
	return nil
}

// trySendLocked sends without blocking; caller holds c.mu and has checked
// that the channel is open.
func (c *Chan[T]) trySendLocked(x T) bool {
	if r := c.takeRecv(); r != nil {
		r.set(x, true)
		r.w.idx = r.idx
		r.w.signal()
		return true
	}
	if len(c.buf) < c.cap {
		c.buf = append(c.buf, x)
		return true
	}
	return false
}

// tryRecvLocked receives without blocking; caller holds c.mu.
func (c *Chan[T]) tryRecvLocked() (T, bool, bool) {
	if len(c.buf) > 0 {
		v := c.buf[0]
		var zero T
		c.buf[0] = zero
		c.buf = c.buf[1:]
		if s := c.takeSend(); s != nil {
			c.buf = append(c.buf, s.v)
			s.w.idx = s.idx
			s.w.signal()
		}
		return v, true, true
	}
	if s := c.takeSend(); s != nil {
		s.w.idx = s.idx
		s.w.signal()
		return s.v, true, true
	}
	if c.closed {
		var zero T
		return zero, false, true
	}
	var zero T
	return zero, false, false
}

// M_send is `ch.send(x)`: blocks when full; a cancellation point that
// unwinds. Sending on a closed channel faults.
func (c *Chan[T]) M_send(tk *Task, x T) {
	tk.Yield()
	tk.CheckCancel()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		FaultHere("send on a closed channel")
	}
	if c.trySendLocked(x) {
		c.mu.Unlock()
		return
	}
	w := newWaiter(tk)
	c.sendq = append(c.sendq, &sendEnt[T]{w: w, v: x})
	c.mu.Unlock()
	if !tk.wait(w, waitSend, c) {
		c.dequeue(w)
		tk.Unwind()
	}
	if w.shut {
		FaultHere("send on a closed channel")
	}
}

// M_recv is `ch.recv()`: None when closed and drained; a cancellation
// point that unwinds.
func (c *Chan[T]) M_recv(tk *Task) Option[T] {
	tk.Yield()
	tk.CheckCancel()
	c.mu.Lock()
	if v, ok, done := c.tryRecvLocked(); done {
		c.mu.Unlock()
		return Option[T]{Some: ok, V: v}
	}
	w := newWaiter(tk)
	var res Option[T]
	c.recvq = append(c.recvq, &recvEnt[T]{w: w, set: func(v T, ok bool) { res = Option[T]{Some: ok, V: v} }})
	c.mu.Unlock()
	if !tk.wait(w, waitRecv, c) {
		c.dequeue(w)
		tk.Unwind()
	}
	return res
}

// M_try_send is `ch.try_send(x)`: false if full or closed; never blocks.
func (c *Chan[T]) M_try_send(tk *Task, x T) bool {
	tk.Yield()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	return c.trySendLocked(x)
}

// M_try_recv is `ch.try_recv()`: None if empty; never blocks.
func (c *Chan[T]) M_try_recv(tk *Task) Option[T] {
	tk.Yield()
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok, _ := c.tryRecvLocked()
	return Option[T]{Some: ok, V: v}
}

// M_close is `ch.close()`. Closing twice has no effect.
func (c *Chan[T]) M_close(tk *Task) {
	tk.Yield()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	for {
		r := c.takeRecv()
		if r == nil {
			break
		}
		var zero T
		r.set(zero, false)
		r.w.idx = r.idx
		r.w.signal()
	}
	for {
		s := c.takeSend()
		if s == nil {
			break
		}
		s.w.shut = true
		s.w.idx = s.idx
		s.w.signal()
	}
}

// M_len is `ch.len()`: the number of buffered values.
func (c *Chan[T]) M_len(*Task) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return int64(len(c.buf))
}

func (c *Chan[T]) hasLiveRecv() bool {
	for _, r := range c.recvq {
		if !r.w.fired.Load() {
			return true
		}
	}
	return false
}

func (c *Chan[T]) hasLiveSend() bool {
	for _, s := range c.sendq {
		if !s.w.fired.Load() {
			return true
		}
	}
	return false
}

// dequeue removes a waiter's entries after it woke or gave up.
func (c *Chan[T]) dequeue(w *waiter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rq := c.recvq[:0]
	for _, r := range c.recvq {
		if r.w != w {
			rq = append(rq, r)
		}
	}
	c.recvq = rq
	sq := c.sendq[:0]
	for _, s := range c.sendq {
		if s.w != w {
			sq = append(sq, s)
		}
	}
	c.sendq = sq
}

// ---------------------------------------------------------------------------
// select (core 7.3)

// Select is a `select` statement being built.
type Select struct {
	tk    *Task
	cases []*selCase
	ext   bool // an arm can be ended from outside the program's tasks (after, task.stopping())
}

type selCase struct {
	// try performs the operation if it can complete now.
	try func() bool
	// enqueue registers the waiter under the channel lock, or completes the
	// operation if it became ready; it reports completion.
	enqueue func(w *waiter, idx int) bool
	dequeue func(w *waiter)
	after   Duration
	isAfter bool
	obj     any // the channel, for deadlock reports
}

// NewSelect starts a select in task tk.
func NewSelect(tk *Task) *Select { return &Select{tk: tk} }

// SelRecv adds an arm `v = ch.recv()`; the returned slot holds the value
// when the arm fires.
func SelRecv[T any](s *Select, c *Chan[T]) *Option[T] {
	slot := new(Option[T])
	s.cases = append(s.cases, &selCase{
		try: func() bool {
			c.mu.Lock()
			defer c.mu.Unlock()
			v, ok, done := c.tryRecvLocked()
			if done {
				*slot = Option[T]{Some: ok, V: v}
			}
			return done
		},
		enqueue: func(w *waiter, idx int) bool {
			c.mu.Lock()
			defer c.mu.Unlock()
			if len(c.buf) > 0 || c.closed || c.hasLiveSend() {
				if !w.claim() {
					return true
				}
				v, ok, d := c.tryRecvLocked()
				if !d {
					w.fired.Store(false)
					c.recvq = append(c.recvq, &recvEnt[T]{w: w, idx: idx, set: func(v T, ok bool) { *slot = Option[T]{Some: ok, V: v} }})
					return false
				}
				*slot = Option[T]{Some: ok, V: v}
				w.idx = idx
				close(w.ch)
				return true
			}
			c.recvq = append(c.recvq, &recvEnt[T]{w: w, idx: idx, set: func(v T, ok bool) { *slot = Option[T]{Some: ok, V: v} }})
			return false
		},
		dequeue: c.dequeue,
		obj:     c,
	})
	if c.ext {
		s.ext = true
	}
	return slot
}

// SelSend adds an arm `ch.send(v)`.
func SelSend[T any](s *Select, c *Chan[T], v T) {
	s.cases = append(s.cases, &selCase{
		try: func() bool {
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.closed {
				FaultHere("send on a closed channel")
			}
			return c.trySendLocked(v)
		},
		enqueue: func(w *waiter, idx int) bool {
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.closed || len(c.buf) < c.cap || c.hasLiveRecv() {
				if !w.claim() {
					return true
				}
				if c.closed {
					w.shut = true
				} else if !c.trySendLocked(v) {
					// the receivers in the queue were stale: wait after all
					w.fired.Store(false)
					c.sendq = append(c.sendq, &sendEnt[T]{w: w, idx: idx, v: v})
					return false
				}
				w.idx = idx
				close(w.ch)
				return true
			}
			c.sendq = append(c.sendq, &sendEnt[T]{w: w, idx: idx, v: v})
			return false
		},
		dequeue: c.dequeue,
		obj:     c,
	})
	if c.ext {
		s.ext = true
	}
}

// SelAfter adds an arm `after d`.
func SelAfter(s *Select, d Duration) {
	s.cases = append(s.cases, &selCase{isAfter: true, after: d})
	s.ext = true
}

// Wait blocks until one arm can proceed, performs it, and returns its
// index. It is a cancellation point that unwinds.
func (s *Select) Wait() int {
	tk := s.tk
	tk.Yield()
	tk.CheckCancel()
	for i, c := range s.cases {
		if c.isAfter {
			if c.after <= 0 {
				return i
			}
			continue
		}
		if c.try() {
			return i
		}
	}
	w := newWaiter(tk)
	done := false
	for i, c := range s.cases {
		if c.isAfter {
			continue
		}
		if c.enqueue(w, i) {
			done = true
			break
		}
	}
	var stops []func()
	if !done {
		for i, c := range s.cases {
			if !c.isAfter {
				continue
			}
			idx := i
			stops = append(stops, afterFunc(tk, c.after, func(locked bool) {
				if w.claim() {
					w.idx = idx
					if locked {
						w.signalLocked()
					} else {
						w.signal()
					}
				}
			}))
		}
	}
	ok := true
	if !done {
		ok = tk.wait(w, waitSelect, s)
	} else {
		<-w.ch
	}
	for _, stop := range stops {
		stop()
	}
	for _, c := range s.cases {
		if c.dequeue != nil {
			c.dequeue(w)
		}
	}
	if !ok {
		tk.Unwind()
	}
	if w.shut {
		FaultHere("send on a closed channel")
	}
	return w.idx
}

// afterFunc calls f after d: on the virtual clock of a test world (f runs
// with the world locked) or in real time.
func afterFunc(tk *Task, d Duration, f func(locked bool)) func() {
	if tk.w != nil {
		return tk.w.timerFunc(d, func() { f(true) })
	}
	return realAfter(d, func() { f(false) })
}
