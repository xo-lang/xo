package rt

import (
	"strings"
	"sync"
)

// Task_Once is `task.Once[T, E]` (std.md 7, decision 0043): a handle; copies
// share the state. The first caller of get runs f in its own task; the
// others queue as waiters. When f finishes, its result (value, error, or
// fault) is stored and every waiter is woken to share it. When the runner
// unwinds before f finishes (cancellation, a test's `proc.exit`), the run
// is handed to the first waiter that is still waiting, which then runs f.
type Task_Once[T, E any] struct{ s *onceState[T, E] }

type onceState[T, E any] struct {
	mu      sync.Mutex
	running bool // a caller is running f
	done    bool
	res     Result[T, E]
	fault   *FaultErr
	q       []*onceWaiter
	runner  *thread // the thread running f, for deadlock reports
}

// onceWaiter is a caller waiting for the result. run is set (under mu,
// before the signal) when the waiter is handed the run instead of a
// result.
type onceWaiter struct {
	w   *waiter
	run bool
}

// Task_once is `task.once[T, E]()`.
func Task_once[T, E any](*Task) Task_Once[T, E] {
	return Task_Once[T, E]{s: &onceState[T, E]{}}
}

// XoDebug implements Debug.
func (o Task_Once[T, E]) XoDebug(b *strings.Builder) { b.WriteString("<once>") }

// XoEq implements Eq: two handles are equal when they share the state.
func (o Task_Once[T, E]) XoEq(x any) bool {
	p, ok := x.(Task_Once[T, E])
	return ok && p.s == o.s
}

// M_get is `o.get(f)` (a Go method: get's only own type parameter is an
// effect). Waiting is a cancellation point that unwinds; a canceled waiter
// leaves the queue without affecting the run.
func (o Task_Once[T, E]) M_get(tk *Task, f func(*Task) Result[T, E]) Result[T, E] {
	tk.Yield()
	s := o.s
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return s.result()
	}
	if s.running {
		ow := &onceWaiter{w: newWaiter(tk)}
		s.q = append(s.q, ow)
		s.mu.Unlock()
		if !tk.wait(ow.w, waitOnce, s) {
			s.mu.Lock()
			for i, x := range s.q {
				if x == ow {
					s.q = append(s.q[:i], s.q[i+1:]...)
					break
				}
			}
			s.mu.Unlock()
			tk.Unwind()
		}
		if !ow.run {
			return s.result()
		}
		// handed the run by a canceled runner; running is still set
	} else {
		s.running = true
		s.runner = tk.th
		s.mu.Unlock()
	}
	return s.run(tk, f)
}

// result is the stored result; a stored fault is raised in the caller.
func (s *onceState[T, E]) result() Result[T, E] {
	if s.fault != nil {
		panic(s.fault)
	}
	return s.res
}

// run calls f as the runner and publishes the outcome.
func (s *onceState[T, E]) run(tk *Task, f func(*Task) Result[T, E]) Result[T, E] {
	finished := false
	defer func() {
		if finished {
			return
		}
		r := recover()
		if r == nil {
			return
		}
		if _, exit := r.(*exitUnwind); IsUnwind(r) || exit {
			// unwinding (cancellation, a test's proc.exit): hand the run on
			s.handOff()
			panic(r)
		}
		// a fault is the shared outcome: later callers fault with it too
		fe := AsFault(r)
		s.finish(Result[T, E]{}, fe)
		panic(fe)
	}()
	res := f(tk)
	finished = true
	s.finish(res, nil)
	return res
}

// finish stores the outcome and wakes every waiter.
func (s *onceState[T, E]) finish(res Result[T, E], fe *FaultErr) {
	s.mu.Lock()
	s.res, s.fault, s.done, s.running, s.runner = res, fe, true, false, nil
	q := s.q
	s.q = nil
	s.mu.Unlock()
	for _, ow := range q {
		if ow.w.claim() {
			ow.w.signal()
		}
	}
}

// handOff gives the run to the first waiter still waiting, or clears
// running when there is none.
func (s *onceState[T, E]) handOff() {
	s.mu.Lock()
	for len(s.q) > 0 {
		ow := s.q[0]
		s.q = s.q[1:]
		if ow.w.claim() {
			ow.run = true
			s.runner = ow.w.tk.th
			s.mu.Unlock()
			ow.w.signal()
			return
		}
	}
	s.running = false
	s.runner = nil
	s.mu.Unlock()
}
