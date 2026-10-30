package rt

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

// waiter is a task blocked on a runtime object (channel, mutex,
// semaphore, select). Exactly one party claims it: a waker that hands it a
// value or ownership, or the task itself when it is canceled.
type waiter struct {
	tk    *Task
	ch    chan struct{}
	fired atomic.Bool
	idx   int  // select: the case that fired
	shut  bool // channel closed while waiting to send
	// lock: the permit was handed over, not just freed (see lock);
	// written before the signal closes ch
	granted bool
	// lock: a reader (Mutex.with_read) rather than an exclusive acquirer
	shared bool
}

func newWaiter(tk *Task) *waiter { return &waiter{tk: tk, ch: make(chan struct{})} }

// claim marks the waiter as taken; only the first claim succeeds.
func (w *waiter) claim() bool { return w.fired.CompareAndSwap(false, true) }

// signal wakes a claimed waiter, counting it as running in a test world.
func (w *waiter) signal() {
	wakeTask(w.tk)
	close(w.ch)
}

// signalLocked is signal for callers that hold the world lock.
func (w *waiter) signalLocked() {
	if w.tk != nil && w.tk.w != nil {
		w.tk.w.wakeLocked(w.tk)
	}
	close(w.ch)
}

// wait blocks until w is signalled or the task is canceled. It reports
// false when the task was canceled before anyone claimed w. k and obj say
// what the task waits on (sched.go).
func (tk *Task) wait(w *waiter, k waitKind, obj any) bool {
	tk.blockOn(k, obj, true)
	// a signal wins over a cancellation that came at the same time, so the
	// outcome does not depend on Go's random choice in select
	select {
	case <-w.ch:
		tk.unblock()
		return true
	default:
	}
	select {
	case <-w.ch:
		tk.unblock()
		return true
	case <-tk.Done():
		if w.claim() {
			tk.unblock()
			return false
		}
		<-w.ch
		tk.unblock()
		return true
	}
}

// event is a one shot event that tasks can wait for (a finished task, an
// empty scope).
type event struct {
	mu      sync.Mutex
	fired   bool
	ch      chan struct{}
	waiters []*Task
}

func newEvent() *event { return &event{ch: make(chan struct{})} }

// fire sets the event and wakes its waiters. The waiters are woken under
// e.mu, so a waiter that leaves early (canceled) and finds the event not
// fired is never woken late.
func (e *event) fire() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.fired {
		return
	}
	e.fired = true
	for _, tk := range e.waiters {
		wakeTask(tk)
	}
	e.waiters = nil
	close(e.ch)
}

func (e *event) isSet() bool {
	select {
	case <-e.ch:
		return true
	default:
		return false
	}
}

// wait blocks until the event fires. When cancelable is set it reports
// false if the task is canceled first. k and obj say what the task waits
// on (sched.go).
func (e *event) wait(tk *Task, cancelable bool, k waitKind, obj any) bool {
	e.mu.Lock()
	if e.fired {
		e.mu.Unlock()
		return true
	}
	e.waiters = append(e.waiters, tk)
	e.mu.Unlock()
	tk.blockOn(k, obj, cancelable)
	var done <-chan struct{}
	if cancelable {
		done = tk.Done()
	}
	select {
	case <-e.ch:
		tk.unblock()
		return true
	case <-done:
		e.mu.Lock()
		if e.fired {
			// fired at the same time: the event wins
			e.mu.Unlock()
			tk.unblock()
			return true
		}
		for i, x := range e.waiters {
			if x == tk {
				e.waiters = append(e.waiters[:i], e.waiters[i+1:]...)
				break
			}
		}
		e.mu.Unlock()
		tk.unblock()
		return false
	}
}

// Scope is the handle bound by `scope s { ... }` (core 7.1).
//
// The body runs as a task on bodyNode, a child of the enclosing task's
// node. Spawned tasks hang below tasksNode, a child of bodyNode, so
// canceling the body cancels its tasks, while `s.cancel()` cancels only
// the tasks.
type Scope struct {
	parent    *Task
	body      *Task
	bodyNode  *cnode
	tasksNode *cnode

	mu      sync.Mutex
	running int
	idle    *event // fired when running drops to 0
	tasks   []*taskRec
	failure *failure
	closed  bool
}

type taskRec struct {
	node       *cnode
	background bool
	done       *event
	th         *thread
}

// failure is the first failure of a child task: an error value (of the
// task's own error type, converted when the scope propagates it) or a
// fault.
type failure struct {
	err     any
	tr      *Trace
	fault   *FaultErr
	repanic any // a non fault unwind (test `proc.exit`) to raise in the scope
}

// NewScope opens a scope in task tk.
func NewScope(tk *Task) *Scope {
	s := &Scope{parent: tk, idle: newEvent()}
	// the body runs on the opening task's goroutine (thread), so it may
	// share its arena (see Arena)
	s.body = &Task{w: tk.w, held: tk.held, shield: tk.shield, arena: tk.arena, stop: tk.stop, th: tk.th, bud: tk.bud, dl: tk.dl}
	bn := &cnode{parent: tk.node, done: make(chan struct{}), w: tk.w, seq: nodeSeq.Add(1), kind: nodeBody, tk: s.body}
	runtime.Callers(2, bn.site[:2])
	s.bodyNode = bn.link()
	s.body.node = s.bodyNode
	s.tasksNode = newNodeOf(s.bodyNode, tk.w, nodeTasks, nil)
	s.idle.fire()
	return s
}

// Body is the task the scope body runs as.
func (s *Scope) Body() *Task { return s.body }

// CtlScopeFail is the control code a scope body closure returns when a
// child task failed and the failure must propagate out of the scope.
const CtlScopeFail = -1

// Close ends the scope (core 7.1 exit rules). r is the recovered panic of
// the body, completed tells whether the body ran to its end, and ctl is
// the body's control code. It waits for every task, then re-raises the
// body's panic, raises a child's fault, or returns CtlScopeFail for a
// child's error.
func (s *Scope) Close(r any, completed bool, ctl int) int {
	if x, ok := r.(*arithFault); ok {
		// convert while the stack is still the operation's (panicStack)
		r = x.fault()
	} else if x, ok := r.(*sizedFault); ok {
		r = x.fault()
	}
	s.mu.Lock()
	if completed && r == nil {
		for _, t := range s.tasks {
			if t.background {
				t.node.cancel()
			}
		}
	} else {
		s.tasksNode.cancel()
	}
	s.mu.Unlock()
	s.wait()
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.tasksNode.detach()
	s.bodyNode.detach()
	f := s.failure
	if r != nil {
		if IsUnwind(r) && f != nil && !s.parent.node.canceled.Load() {
			// the body was canceled because a child failed
			if f.repanic != nil {
				panic(f.repanic)
			}
			if f.fault != nil {
				panic(f.fault)
			}
			return CtlScopeFail
		}
		panic(r)
	}
	if f != nil {
		if f.repanic != nil {
			panic(f.repanic)
		}
		if f.fault != nil {
			panic(f.fault)
		}
		return CtlScopeFail
	}
	return ctl
}

// shutdown cancels every task, waits for them, and detaches the scope.
func (s *Scope) shutdown() {
	s.tasksNode.cancel()
	s.wait()
	s.tasksNode.detach()
	s.bodyNode.detach()
}

// wait blocks until every task of the scope has finished. It is not a
// cancellation point: a canceled body still waits for its tasks to unwind.
func (s *Scope) wait() {
	for {
		s.mu.Lock()
		ev := s.idle
		s.mu.Unlock()
		ev.wait(s.parent, false, waitScope, s)
		s.mu.Lock()
		n := s.running
		s.mu.Unlock()
		if n == 0 {
			return
		}
	}
}

// Failure returns the error of the failed child task and its frames.
func (s *Scope) Failure() (any, *Trace) {
	if s.failure == nil {
		return nil, nil
	}
	return s.failure.err, s.failure.tr
}

func (s *Scope) fail(f *failure) {
	s.mu.Lock()
	first := s.failure == nil
	if first {
		s.failure = f
	}
	s.mu.Unlock()
	if first {
		s.bodyNode.cancel()
	}
}

// M_cancel is `s.cancel()`: cancels the scope's tasks, not the body. Tasks
// spawned later start in a fresh node.
func (s *Scope) M_cancel(*Task) {
	s.mu.Lock()
	old := s.tasksNode
	s.tasksNode = newNodeOf(s.bodyNode, s.parent.w, nodeTasks, nil)
	s.mu.Unlock()
	old.cancel()
}

// M_group is `s.group()` (core 7.1).
func (s *Scope) M_group(*Task) Task_Group { return Task_Group{s: s} }

// XoDebug implements Debug.
func (s *Scope) XoDebug(b *strings.Builder) { b.WriteString("<scope>") }

// Handle is a task handle `Task[T, E]`.
type Handle[T, E any] struct {
	th       *thread // the task's thread, for deadlock reports
	done     *event
	res      Result[T, E]
	fault    *FaultErr
	canceled bool
}

// XoDebug implements Debug.
func (h *Handle[T, E]) XoDebug(b *strings.Builder) { b.WriteString("<task>") }

// Spawn starts a task in scope s (`s.spawn(f)`, `s.spawn_background(f)`).
// consumed is true when the handle is bound and awaited, so the task's
// error does not fail the scope (core 7.1).
func Spawn[T, E any](tk *Task, s *Scope, f func(*Task) Result[T, E], background, consumed bool) *Handle[T, E] {
	return spawnWith(tk, s, f, background, consumed, nil)
}

// spawnWith is Spawn whose task runs under the budget bud when it is set
// (Task_with_budget), else under the spawning task's.
func spawnWith[T, E any](tk *Task, s *Scope, f func(*Task) Result[T, E], background, consumed bool, bud *budget) *Handle[T, E] {
	h := &Handle[T, E]{done: newEvent()}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		FaultHere("a task was started on a scope that has ended")
	}
	child := tk.child(s.tasksNode, "task")
	if bud != nil {
		bud.start(child)
	}
	node := child.node
	th := child.th
	runtime.Callers(2, th.site[:2])
	h.th = th
	s.tasks = append(s.tasks, &taskRec{node: node, background: background, done: h.done, th: th})
	if s.running == 0 {
		s.idle = newEvent()
	}
	s.running++
	s.mu.Unlock()
	if th.mon != nil {
		th.mon.started()
	}
	if sch := th.sch; sch != nil {
		sch.start(th)
	} else if tk.w != nil {
		tk.w.taskStarted()
	}
	labeled := tk.th != nil && tk.th.labeled
	go func() {
		if sch := th.sch; sch != nil {
			<-th.turn
			if labeled {
				// a spawner that passed a Go callback labeled its
				// goroutine (NewGoCallback); this one is not it
				clearLabels()
			}
		} else if tk.w == nil {
			// drop the profiler label inherited from the spawning
			// goroutine: this thread sets its own when it first blocks
			clearLabels()
		}
		defer func() {
			s.mu.Lock()
			s.running--
			var idle *event
			if s.running == 0 {
				idle = s.idle
			}
			s.mu.Unlock()
			h.done.fire()
			if idle != nil {
				idle.fire()
			}
			th.mu.Lock()
			th.done = true
			th.mu.Unlock()
			if th.mon != nil {
				th.mon.ended()
			}
			if sch := th.sch; sch != nil {
				sch.end(th)
			} else if child.w != nil {
				child.w.taskEnded()
			}
		}()
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			if IsUnwind(r) {
				h.canceled = true
				return
			}
			if _, ok := r.(*exitUnwind); ok {
				s.fail(&failure{repanic: r})
				return
			}
			h.fault = AsFault(r)
			s.fail(&failure{fault: h.fault})
		}()
		h.res = f(child)
		if !h.res.Ok && !consumed && !node.canceled.Load() {
			s.fail(&failure{err: any(h.res.E), tr: h.res.Tr})
		}
	}()
	// a scheduling point: the new task may run first
	tk.Yield()
	return h
}

// Await is `t.await` (core 7.1): waits for the task (a cancellation point
// that unwinds) and yields its result. A fault in the task is raised in
// the awaiting task.
func Await[T, E any](tk *Task, h *Handle[T, E]) Result[T, E] {
	tk.Yield()
	if !h.done.isSet() {
		tk.CheckCancel()
		if !h.done.wait(tk, true, waitAwait, h) {
			tk.Unwind()
		}
	}
	if h.fault != nil {
		panic(h.fault)
	}
	if h.canceled {
		tk.CheckCancel()
		FaultHere("awaited a task that was canceled before it finished")
	}
	return h.res
}

// Task_Group is a task group `task.Group` (core 7.1): a storable handle
// whose tasks belong to the scope that created it.
type Task_Group struct{ s *Scope }

// XoDebug implements Debug.
func (g Task_Group) XoDebug(b *strings.Builder) { b.WriteString("<group>") }

// XoEq implements Eq: groups are equal when they belong to one scope.
func (g Task_Group) XoEq(o any) bool {
	p, ok := o.(Task_Group)
	return ok && p.s == g.s
}

// GroupSpawn is `g.spawn(f)`.
func GroupSpawn[T, E any](tk *Task, g Task_Group, f func(*Task) Result[T, E], consumed bool) *Handle[T, E] {
	return Spawn(tk, g.s, f, false, consumed)
}

// GroupSpawnBackground is `g.spawn_background(f)`.
func GroupSpawnBackground[E any](tk *Task, g Task_Group, f func(*Task) Result[Unit, E]) {
	Spawn(tk, g.s, f, true, false)
}
