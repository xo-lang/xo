package rt

import (
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Go callbacks under the deterministic scheduler (decision 0073, part 2).
//
// Go code runs beside the scheduler: a Go call keeps running while other
// threads take turns once it lent its turn, and Go's goroutines call
// callbacks (sync.WaitGroup.Go) whenever Go gets to them. Before this
// decision a callback thread joined the ready threads the moment its
// goroutine arrived, and a lent thread the moment its Go call returned,
// so the same seed gave different interleavings.
//
// Now both wait in a pending list, and the scheduler admits them only at
// a decision, after Go's goroutines settled: every goroutine but the one
// deciding is parked, sleeping, or in a system call or I/O (goSettled),
// so whatever Go code the program's own Go calls set off has arrived.
// Admission order is fixed: returning threads by id, then callback calls
// by callback (numbered where the Xo function crossed into Go) and
// encoded arguments; thread ids are given at admission. Once an Xo
// function crossed into Go (armed), a Go call's return is a scheduling
// point, so callbacks Go started during the call join there.
//
// A recording logs each admitted callback call in the scheduler stream
// (`c<callback>`) and as an effect `go.callback` of the new thread with
// its arguments; a replay admits the pending call of that callback whose
// arguments match at the same point, and returns of Go calls are matched
// by the picks that name their threads.
//
// Limits: Go code woken by a timer or I/O of its own arrives when it
// arrives (as timers and I/O always did under `xo run --seed`), and a
// settle gives up after settleLimit when a goroutine keeps running.

// cbArrival is a call of a Go callback that arrived on a goroutine of
// Go's and waits for admission.
type cbArrival struct {
	th   *thread
	tk   *Task
	cb   *GoCallback
	args []any  // the Xo arguments, for the trace
	enc  string // their encoding, for the order and replay matching
}

// settleLimit bounds the wait for Go's goroutines at one decision.
const settleLimit = 200 * time.Millisecond

// startCallback queues the thread of a callback called on a goroutine of
// Go's and waits for its turn. A thread that holds the turn inside a Go
// call may be waiting for this callback (WaitGroup.Wait): once Go's
// goroutines settled with it still in the call, it lends its turn.
func (s *sched) startCallback(a *cbArrival) {
	s.mu.Lock()
	s.cbLive.Add(1)
	s.pend = append(s.pend, a)
	s.goDirty = true
	if c := s.cur; c == nil {
		if !s.picking {
			s.pickLocked()
		}
	} else if c.inGo.Load() > 0 && !c.lent.Load() {
		s.lendWhenQuietLocked(c)
	}
	s.mu.Unlock()
	<-a.th.turn
}

// endCallback removes the thread of a Go callback; Go code goes on on its
// goroutine, so the next decision settles first.
func (s *sched) endCallback(th *thread) {
	s.mu.Lock()
	s.cbLive.Add(-1)
	s.goDirty = true
	s.endLocked(th)
	s.mu.Unlock()
}

// lendWhenQuietLocked lends c's turn from another goroutine once Go's
// goroutines settled, if c is still inside its Go call and a callback
// still waits.
func (s *sched) lendWhenQuietLocked(c *thread) {
	if s.lending {
		return
	}
	s.lending = true
	go func() {
		waitGoSettled()
		s.mu.Lock()
		s.lending = false
		if s.cur == c && c.inGo.Load() > 0 && !c.lent.Load() && len(s.pend) > 0 {
			s.lendLocked(c)
			s.goDirty = true
			if !s.picking {
				s.pickLocked()
			}
		}
		s.mu.Unlock()
	}()
}

// settleLocked is called by pickLocked once armed: when Go code may have
// run since the last decision, it waits (unlocked) for Go's goroutines to
// settle, then admits what arrived.
func (s *sched) settleLocked() {
	if !s.goDirty && s.lentN == 0 && len(s.pend) == 0 && len(s.pendRet) == 0 {
		return
	}
	s.mu.Unlock()
	waitGoSettled()
	s.mu.Lock()
	s.goDirty = false
	s.admitReturnsLocked()
	if len(s.pend) == 0 {
		return
	}
	pend := s.pend
	s.pend = nil
	sort.SliceStable(pend, func(i, j int) bool {
		if pend[i].cb.seq != pend[j].cb.seq {
			return pend[i].cb.seq < pend[j].cb.seq
		}
		return pend[i].enc < pend[j].enc
	})
	for _, a := range pend {
		s.admitLocked(a)
	}
}

// admitReturnsLocked makes the threads whose Go calls returned ready, by
// id.
func (s *sched) admitReturnsLocked() {
	if len(s.pendRet) == 0 {
		return
	}
	rs := s.pendRet
	s.pendRet = nil
	sort.Slice(rs, func(i, j int) bool { return rs[i].id < rs[j].id })
	for _, th := range rs {
		th.st = stReady
		s.ready = append(s.ready, th)
	}
}

// admitLocked gives a callback call its thread id and makes it ready; a
// recording logs it.
func (s *sched) admitLocked(a *cbArrival) {
	th := a.th
	th.id = th.ids.Add(1)
	s.startLocked(th)
	if s.rec != nil {
		s.rec.tok("c" + strconv.FormatInt(a.cb.seq, 10))
		s.rec.add(a.tk, "go.callback", a.args, nil)
	}
}

// replayArrivalLocked admits the pending call of callback seq whose
// arguments match the trace's next effect of the thread it becomes, and
// reports whether there was one.
func (s *sched) replayArrivalLocked(seq int64) bool {
	p := s.rp
	for i, a := range s.pend {
		if a.cb.seq != seq || !p.matchCallback(a.th.ids.Load()+1, a.enc) {
			continue
		}
		s.pend = append(s.pend[:i:i], s.pend[i+1:]...)
		s.admitLocked(a)
		p.next(a.tk, "go.callback", a.args)
		return true
	}
	return false
}

// matchCallback reports whether the next recorded effect of thread id is
// a callback call with these encoded arguments.
func (p *replayer) matchCallback(id int64, enc string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	q, i := p.queues[id], p.pos[id]
	if i >= len(q) || q[i].Op != "go.callback" {
		return false
	}
	if len(q[i].A) == 0 {
		return enc == ""
	}
	return fxText(q[i].A) == enc
}

// encodeArgs is the encoding of a callback call's arguments ("" for
// none).
func encodeArgs(args []any) string {
	if len(args) == 0 {
		return ""
	}
	enc := make([]any, len(args))
	for i, a := range args {
		enc[i] = fxEncode(a)
	}
	return fxText(enc)
}

// waitGoSettled waits until goSettled, at most settleLimit.
func waitGoSettled() {
	start := time.Now()
	for i := 0; !goSettled(); i++ {
		if time.Since(start) > settleLimit {
			return
		}
		if i < 8 {
			runtime.Gosched()
			continue
		}
		osSleep(time.Duration(min(1<<(i-8), 1000)) * time.Microsecond)
	}
}

// goSettled reports whether no goroutine but the caller is running or
// runnable.
func goSettled() bool {
	buf := make([]byte, 1<<14)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	gs := strings.Split(string(buf), "\n\n")
	for _, g := range gs[1:] {
		i := strings.IndexByte(g, '[')
		if i < 0 {
			continue
		}
		st := g[i+1:]
		if j := strings.IndexAny(st, ",]"); j >= 0 {
			st = st[:j]
		}
		switch st {
		case "running", "runnable", "preempted":
			return false
		}
	}
	return true
}
