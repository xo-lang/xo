package rt

import (
	"errors"
	"net"
	"strconv"
	"sync"
	"time"
)

// FakeNet's raw connections (decision 0292, std.md 8.1): a dial in a
// test reaches the in memory listener at its address; the listener's
// accept gives the other end. Each direction is a memPipe of at most
// memPipeSize bytes; reads and writes wait as tasks (the deadlock report
// names them, the virtual clock moves while every context waits).

// memPipeSize is what one direction of a raw in memory connection
// buffers before a write waits.
const memPipeSize = 64 << 10

// memPipe is one direction of an in memory connection.
type memPipe struct {
	mu      sync.Mutex
	buf     []byte
	wclosed bool   // the writer closed: reads end after buf
	rclosed bool   // the reader closed: writes fail
	changed *event // fired, and replaced, on every change
}

func newMemPipe() *memPipe { return &memPipe{changed: newEvent()} }

// notify wakes the waiters of the pipe; p.mu is held.
func (p *memPipe) notify() {
	ev := p.changed
	p.changed = newEvent()
	ev.fire()
}

// memEnd is one side of an in memory connection: it reads in and
// writes out. It is a Conn's net.Conn, marked as FakeNet's (fakeConn), so
// tls.client passes its bytes through (std.md 27); Conn's read and write
// call read and write below, which wait as tasks, and never its Read
// and Write.
type memEnd struct {
	in, out     *memPipe
	local, peer string

	dlMu   sync.Mutex
	rd, wr memDeadline // the user's deadlines (decision 0430)
}

// memDeadline is a FakeNet deadline. It is kept on the test world's
// virtual clock when it was set on that clock, else in real time, and is
// read without a clock call: clock.now yields the turn, and the reads
// and writes below check it holding their pipe's lock.
type memDeadline struct {
	set bool
	w   *World // the virtual clock at is on, nil for real time
	at  Time   // w's virtual time, or real time in Unix ns
}

// left is the time until d, which is set.
func (d memDeadline) left() Duration {
	if d.w != nil {
		return Duration(d.at - d.w.Now())
	}
	return Duration(int64(d.at) - time.Now().UnixNano())
}

// setDeadline sets e's user read and or write deadline, t on clock, and
// wakes a waiting read or write to take it.
func (e *memEnd) setDeadline(tk *Task, clock Clock, t Option[Time], read, write bool) {
	var d memDeadline
	if t.Some {
		left := Duration(int64(t.V) - int64(clock.M_now(tk)))
		if fc, ok := clock.(Testing_FakeClock); ok && !fc.real && fc.w != nil {
			d = memDeadline{set: true, w: fc.w, at: fc.w.Now() + Time(left)}
		} else {
			d = memDeadline{set: true, at: Time(time.Now().UnixNano()) + Time(left)}
		}
	}
	e.dlMu.Lock()
	if read {
		e.rd = d
	}
	if write {
		e.wr = d
	}
	e.dlMu.Unlock()
	for _, x := range []struct {
		p  *memPipe
		on bool
	}{{e.in, read}, {e.out, write}} {
		if x.on {
			x.p.mu.Lock()
			x.p.notify()
			x.p.mu.Unlock()
		}
	}
}

// deadline is e's read (or write) deadline.
func (e *memEnd) deadline(write bool) memDeadline {
	e.dlMu.Lock()
	defer e.dlMu.Unlock()
	if write {
		return e.wr
	}
	return e.rd
}

// timedOut reports whether e's read (or write) deadline has passed.
func (e *memEnd) timedOut(write bool) bool {
	d := e.deadline(write)
	return d.set && d.left() <= 0
}

// waitChange waits for ev, a change of e's pipe, or until e's read (or
// write) deadline: a waiting operation is woken by its deadline, on the
// virtual clock when it was set on it, so a test world's time moves to
// it (decision 0430; it reported a deadlock with "no timer is pending").
// It reports false when tk was canceled.
func (e *memEnd) waitChange(tk *Task, ev *event, write bool, k waitKind) bool {
	if d := e.deadline(write); d.set {
		var stop func()
		if d.w != nil {
			stop = d.w.timerFunc(d.left(), ev.fireLocked)
		} else {
			stop = realAfter(d.left(), ev.fire)
		}
		defer stop()
	}
	return ev.wait(tk, true, k, nil)
}

// changedEvent is the event a waiter of p waits on; p.mu is held. One a
// deadline fired (waitChange) is replaced, or every later waiter would
// return at once.
func (p *memPipe) changedEvent() *event {
	if p.changed.isSet() {
		p.changed = newEvent()
	}
	return p.changed
}

func (e *memEnd) xoFakeNet() {}

var errMemDirect = errors.New("an in memory connection is read and written through its task")

func (e *memEnd) Read([]byte) (int, error)         { return 0, errMemDirect }
func (e *memEnd) Write([]byte) (int, error)        { return 0, errMemDirect }
func (e *memEnd) Close() error                     { e.close(); return nil }
func (e *memEnd) LocalAddr() net.Addr              { return memAddr(e.local) }
func (e *memEnd) RemoteAddr() net.Addr             { return memAddr(e.peer) }
func (e *memEnd) SetDeadline(time.Time) error      { return nil }
func (e *memEnd) SetReadDeadline(time.Time) error  { return nil }
func (e *memEnd) SetWriteDeadline(time.Time) error { return nil }

// memAddr is an in memory connection's address.
type memAddr string

func (a memAddr) Network() string { return "tcp" }
func (a memAddr) String() string  { return string(a) }

// newMemPair returns the dialer's and the listener's ends of a new in
// memory connection to addr.
func newMemPair(addr string, port int) (*memEnd, *memEnd) {
	a, b := newMemPipe(), newMemPipe()
	from := "127.0.0.1:" + strconv.Itoa(port)
	return &memEnd{in: a, out: b, local: from, peer: addr}, &memEnd{in: b, out: a, local: addr, peer: from}
}

// read returns at most max bytes, waiting while nothing is buffered;
// None at the end of the stream.
func (e *memEnd) read(tk *Task, max int64) Result[Option[Bytes], NetErr] {
	p := e.in
	for {
		p.mu.Lock()
		if p.rclosed {
			p.mu.Unlock()
			return Err[Option[Bytes]](netErr(4, "read tcp: "+netClosedText))
		}
		if len(p.buf) > 0 {
			n := min(int64(len(p.buf)), max, 1<<20)
			b := Bytes(p.buf[:n])
			p.buf = append([]byte(nil), p.buf[n:]...)
			p.notify()
			p.mu.Unlock()
			return Ok[Option[Bytes], NetErr](Some(b))
		}
		if p.wclosed {
			p.mu.Unlock()
			return Ok[Option[Bytes], NetErr](Option[Bytes]{})
		}
		if e.timedOut(false) {
			p.mu.Unlock()
			return Err[Option[Bytes]](netErr(3, ""))
		}
		ev := p.changedEvent()
		p.mu.Unlock()
		if !e.waitChange(tk, ev, false, waitNetRead) {
			return Err[Option[Bytes]](netErr(5, ""))
		}
	}
}

// write writes every byte of data, waiting while the pipe is full.
func (e *memEnd) write(tk *Task, data Bytes) Result[Unit, NetErr] {
	p := e.out
	rest := []byte(data)
	for {
		p.mu.Lock()
		if p.wclosed {
			p.mu.Unlock()
			return Err[Unit](netErr(4, "write tcp: "+netClosedText))
		}
		if p.rclosed {
			p.mu.Unlock()
			return Err[Unit](netErr(4, "write tcp: connection reset by peer"))
		}
		if room := memPipeSize - len(p.buf); room > 0 && len(rest) > 0 {
			n := min(room, len(rest))
			p.buf = append(p.buf, rest[:n]...)
			rest = rest[n:]
			p.notify()
		}
		if len(rest) == 0 {
			p.mu.Unlock()
			return Ok[Unit, NetErr](Unit{})
		}
		if e.timedOut(true) {
			p.mu.Unlock()
			return Err[Unit](netErr(3, ""))
		}
		ev := p.changedEvent()
		p.mu.Unlock()
		if !e.waitChange(tk, ev, true, waitNetWrite) {
			return Err[Unit](netErr(5, ""))
		}
	}
}

// close ends the connection on this side: the peer's reads end after
// what it has buffered, and its writes fail.
func (e *memEnd) close() {
	for _, x := range []struct {
		p   *memPipe
		out bool
	}{{e.out, true}, {e.in, false}} {
		x.p.mu.Lock()
		if x.out {
			x.p.wclosed = true
		} else {
			x.p.rclosed = true
			x.p.buf = nil
		}
		x.p.notify()
		x.p.mu.Unlock()
	}
}

// memQueue is an in memory listener's backlog of dialed connections.
type memQueue struct {
	mu      sync.Mutex
	pending []*memEnd
	closed  bool
	changed *event
	// fired by the first accept, as the listener's ready event: a client
	// request to a listener no http.server serves goes on as a raw dial
	accepted *event
}

// push queues the server end of a dialed connection; false when the
// listener is closed.
func (q *memQueue) push(e *memEnd) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	q.pending = append(q.pending, e)
	ev := q.changed
	q.changed = newEvent()
	ev.fire()
	return true
}

// accept waits for the next dialed connection.
func (q *memQueue) accept(tk *Task) Result[Conn, NetErr] {
	if q.accepted != nil {
		q.accepted.fire()
	}
	for {
		q.mu.Lock()
		if q.closed {
			q.mu.Unlock()
			return Err[Conn](netErr(4, netClosedText))
		}
		if len(q.pending) > 0 {
			e := q.pending[0]
			q.pending = q.pending[1:]
			q.mu.Unlock()
			return Ok[Conn, NetErr](Conn{c: e})
		}
		ev := q.changed
		q.mu.Unlock()
		if !ev.wait(tk, true, waitNetAccept, nil) {
			return Err[Conn](netErr(5, ""))
		}
	}
}

// close stops the listener: a waiting accept fails, and the queued
// connections are reset.
func (q *memQueue) close() {
	q.mu.Lock()
	q.closed = true
	pending := q.pending
	q.pending = nil
	ev := q.changed
	q.changed = newEvent()
	q.mu.Unlock()
	ev.fire()
	for _, e := range pending {
		e.close()
	}
}
