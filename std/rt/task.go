package rt

import (
	"sync"
	"sync/atomic"
)

// Task is the current task (core 7). Every generated function takes it as
// its first parameter, which is how the scope tree flows down implicitly:
// cancellation, the cancellation shield of `defer`, held mutexes, and the
// test world of fakes.
type Task struct {
	node   *cnode
	shield int32
	held   *heldSet // mutexes held by this task, with the acquisition site
	w      *World   // the test world (virtual clock), nil in real programs
	// arena is the request scoped arena of an HTTP request task (arena.go),
	// nil otherwise. Spawned tasks never inherit it (child), so only one
	// goroutine uses an arena at a time.
	arena *Arena
	// stop is the shutdown state of the program run the task belongs to
	// (core 7.5, stop.go), nil outside RunMain and testing.run_main.
	stop *stopState
	// th is the goroutine the task runs on (sched.go): what it is blocked
	// on, and its turn under the deterministic scheduler.
	th *thread
	// bud is the resource budget the task works under (budget.go,
	// decision 0173), nil when none.
	bud *budget
	// dl is the nearest task.with_timeout deadline the task works under
	// (decision 0430), nil when none; a spawned task keeps its spawner's
	dl *taskDeadline
}

// taskDeadline is a with_timeout deadline: the instant at on clock.
type taskDeadline struct {
	clock Clock
	at    Time
}

// cnode is a node of the cancellation tree. Canceling a node cancels every
// node below it. Cancellation is sticky (core 7.2).
type cnode struct {
	mu       sync.Mutex
	parent   *cnode
	children map[*cnode]struct{}
	canceled atomic.Bool
	done     chan struct{} // made lazily (doneCh), guarded by mu
	// closeDone: cancel must close done (it was made before the cancel)
	closeDone bool
	w         *World
	// for `xo dump` (sched.go), set before the node is linked in
	seq  uint64
	kind uint8
	tk   *Task      // the task that runs as this node (task, scope body, root)
	site [4]uintptr // a scope body: where the scope was opened
}

func newNode(parent *cnode, w *World) *cnode { return newNodeOf(parent, w, nodePlain, nil) }

// newNodeOf is newNode with the node's kind and task for `xo dump`.
func newNodeOf(parent *cnode, w *World, kind uint8, tk *Task) *cnode {
	return initNode(new(cnode), parent, w, kind, tk)
}

// initNode is newNodeOf in memory the caller allocated (a zero node).
func initNode(n *cnode, parent *cnode, w *World, kind uint8, tk *Task) *cnode {
	n.parent, n.w, n.seq, n.kind, n.tk = parent, w, nodeSeq.Add(1), kind, tk
	return n.link()
}

// closedDone is the done channel of a node canceled before anyone asked
// for it.
var closedDone = func() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}()

// doneCh returns the channel closed when n is canceled. It is made on
// first use: most request and task nodes end without anyone waiting on
// cancellation, and the channel was one allocation per task.
func (n *cnode) doneCh() chan struct{} {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.done == nil {
		if n.canceled.Load() {
			n.done = closedDone
		} else {
			n.done = make(chan struct{})
		}
	}
	return n.done
}

// link adds n to its parent's children (or cancels it at once when the
// parent is canceled).
func (n *cnode) link() *cnode {
	parent := n.parent
	if parent != nil {
		parent.mu.Lock()
		if parent.canceled.Load() {
			parent.mu.Unlock()
			n.cancel()
			return n
		}
		if parent.children == nil {
			parent.children = map[*cnode]struct{}{}
		}
		parent.children[n] = struct{}{}
		parent.mu.Unlock()
	}
	return n
}

// cancel cancels n and its subtree. In a test world, tasks blocked below n
// are counted as running again before they are woken, so the virtual clock
// does not move while they unwind (std.md 8.2).
// It takes the world lock, so virtual clock timer callbacks must not call
// it (see stopState.after).
func (n *cnode) cancel() {
	var nodes []*cnode
	var collect func(c *cnode)
	collect = func(c *cnode) {
		c.mu.Lock()
		if c.canceled.Load() {
			c.mu.Unlock()
			return
		}
		c.canceled.Store(true)
		if c.done == nil {
			c.done = closedDone
		} else {
			c.closeDone = true
		}
		kids := make([]*cnode, 0, len(c.children))
		for k := range c.children {
			kids = append(kids, k)
		}
		c.mu.Unlock()
		nodes = append(nodes, c)
		for _, k := range kids {
			collect(k)
		}
	}
	collect(n)
	if len(nodes) == 0 {
		return
	}
	if n.w != nil {
		n.w.wakeCanceled(nodes)
	} else if realSched != nil {
		realSched.wakeCanceled(nodes)
	}
	if m := runMon.Load(); m != nil {
		m.gen.Add(1)
	}
	for _, c := range nodes {
		if c.closeDone {
			close(c.done)
		}
	}
}

// detach removes n from its parent so finished scopes do not accumulate.
func (n *cnode) detach() {
	if p := n.parent; p != nil {
		p.mu.Lock()
		delete(p.children, n)
		p.mu.Unlock()
	}
}

// under reports whether n is in the subtree of one of the nodes.
func (n *cnode) under(set map[*cnode]bool) bool {
	for c := n; c != nil; c = c.parent {
		if set[c] {
			return true
		}
	}
	return false
}

// NewRootTask returns a task with a fresh cancellation root.
func NewRootTask() *Task { return newRootTask("main") }

func newRootTask(kind string) *Task {
	tk := &Task{th: newRootThread(kind)}
	tk.node = newNodeOf(nil, nil, nodeRoot, tk)
	return tk
}

// InitTask is the task used to evaluate constants and defaults at program
// start.
var InitTask = NewRootTask()

// child returns a task for a new node below parent, on a new thread. It
// has no arena: a spawned task runs on its own goroutine (see Arena).
func (tk *Task) child(parent *cnode, kind string) *Task {
	c := &Task{w: tk.w, stop: tk.stop, th: tk.th.newChild(kind), dl: tk.dl}
	c.node = newNodeOf(parent, tk.w, nodeTask, c)
	if tk.bud != nil {
		// a task started under a budget works under it too (decision
		// 0173), wherever it is started
		c.bud = tk.bud
		c.th.budRun = monoNow()
		c.th.budQ, c.th.budLeft = budgetQuantum, budgetQuantum
	}
	return c
}

// Canceled reports whether a cancellation point here reports cancellation:
// the task is canceled and not inside a shielded `defer` (core 3.5, 7.2).
func (tk *Task) Canceled() bool {
	return tk.shield == 0 && tk.node.canceled.Load()
}

// Task_is_canceled is `task.is_canceled()`: the task's state, shield
// ignored. It is a scheduling point, so a loop that polls it lets the task
// that cancels it run under the deterministic scheduler.
func Task_is_canceled(tk *Task) bool {
	tk.Yield()
	return tk.node.canceled.Load()
}

// Done returns a channel closed on cancellation, or nil (never ready)
// while shielded.
func (tk *Task) Done() <-chan struct{} {
	if tk.shield > 0 {
		return nil
	}
	return tk.node.doneCh()
}

// cancelUnwind is the panic value that unwinds a canceled task from a
// blocking operation that cannot fail (core 7.2). Task boundaries recover
// it; it is never a fault.
type cancelUnwind struct{}

// IsUnwind reports whether a recovered panic value is a cancellation
// unwind.
func IsUnwind(r any) bool {
	_, ok := r.(cancelUnwind)
	return ok
}

// Unwind unwinds the current task, running `defer` blocks.
func (tk *Task) Unwind() { panic(cancelUnwind{}) }

// CheckCancel is a cancellation point of an operation that cannot fail.
func (tk *Task) CheckCancel() {
	if tk.Canceled() {
		panic(cancelUnwind{})
	}
}

// shieldClock is the monotonic clock Shielded measures with (a variable
// so a test can advance it).
var shieldClock = monoNow

// Shielded runs deferred code shielded from cancellation (core 3.5). A
// shielded defer that runs longer than five seconds of real time faults
// (XO-F005); P1 checks the limit when the deferred code returns.
func (tk *Task) Shielded(at string, f func()) {
	tk.shield++
	start := shieldClock()
	defer func() { tk.shield-- }()
	f()
	if el := shieldClock() - start; el > 5e9 && tk.w == nil {
		FaultCode("XO-F005", at, "a shielded defer ran for %s, longer than 5s", Duration(el))
	}
}

// blockOn and unblock bracket a blocking wait so the virtual clock of a
// test can tell when every task is blocked (std.md 8.2), the deterministic
// scheduler can hand the turn on (core 7.4), and deadlock reports and
// `xo dump` can say what the task waits on and where (core 11.4).
// cancelable tells whether cancellation ends the wait (it does not while
// shielded).
func (tk *Task) blockOn(k waitKind, obj any, cancelable bool) {
	if tk.bud != nil {
		tk.budgetBlock()
	}
	th := tk.th
	th.setWait(tk, k, obj)
	if s := th.sch; s != nil {
		s.block(th, tk, cancelable && tk.shield == 0)
	} else if tk.w != nil {
		tk.w.block(tk)
	}
}

func (tk *Task) unblock() {
	th := tk.th
	th.clearWait()
	if s := th.sch; s != nil {
		s.unblock(th)
	} else if tk.w != nil {
		tk.w.unblock(tk)
	}
	if tk.bud != nil {
		tk.budgetUnblock()
	}
}

// wakeTask is called by whoever ends a blocked task's wait, before it
// signals the task: the task counts as running again at once.
func wakeTask(tk *Task) {
	if tk == nil {
		return
	}
	th := tk.th
	th.woken()
	if tk.w != nil {
		tk.w.wake(tk)
	} else if s := th.sch; s != nil {
		s.wake(th)
	}
}
