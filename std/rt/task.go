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
	held   map[any]heldSite // mutexes held by this task, with the acquisition site
	w      *World           // the test world (virtual clock), nil in real programs
	// arena is the request scoped arena of an HTTP request task (arena.go),
	// nil otherwise. Spawned tasks never inherit it (child), so only one
	// goroutine uses an arena at a time.
	arena *Arena
}

// cnode is a node of the cancellation tree. Canceling a node cancels every
// node below it. Cancellation is sticky (core 7.2).
type cnode struct {
	mu       sync.Mutex
	parent   *cnode
	children map[*cnode]struct{}
	canceled atomic.Bool
	done     chan struct{}
	w        *World
}

func newNode(parent *cnode, w *World) *cnode {
	n := &cnode{parent: parent, done: make(chan struct{}), w: w}
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
func (n *cnode) cancel() { n.cancelWorld(false) }

// cancelLocked is cancel for callers that already hold the test world's lock,
// such as virtual clock timer callbacks.
func (n *cnode) cancelLocked() { n.cancelWorld(true) }

func (n *cnode) cancelWorld(worldLocked bool) {
	var nodes []*cnode
	var collect func(c *cnode)
	collect = func(c *cnode) {
		c.mu.Lock()
		if c.canceled.Load() {
			c.mu.Unlock()
			return
		}
		c.canceled.Store(true)
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
		if worldLocked {
			n.w.wakeCanceledLocked(nodes)
		} else {
			n.w.wakeCanceled(nodes)
		}
	}
	for _, c := range nodes {
		close(c.done)
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
func NewRootTask() *Task { return &Task{node: newNode(nil, nil)} }

// InitTask is the task used to evaluate constants and defaults at program
// start.
var InitTask = NewRootTask()

// child returns a task for a new node of the tree. It has no arena: a
// spawned task runs on its own goroutine (see Arena).
func (tk *Task) child(n *cnode) *Task { return &Task{node: n, w: tk.w} }

// Canceled reports whether a cancellation point here reports cancellation:
// the task is canceled and not inside a shielded `defer` (core 3.5, 7.2).
func (tk *Task) Canceled() bool {
	return tk.shield == 0 && tk.node.canceled.Load()
}

// Task_is_canceled is `task.is_canceled()`: the task's state, shield
// ignored.
func Task_is_canceled(tk *Task) bool { return tk.node.canceled.Load() }

// Done returns a channel closed on cancellation, or nil (never ready)
// while shielded.
func (tk *Task) Done() <-chan struct{} {
	if tk.shield > 0 {
		return nil
	}
	return tk.node.done
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

// Shielded runs deferred code shielded from cancellation (core 3.5). A
// shielded defer that runs longer than five seconds of real time faults
// (XO-F005); P1 checks the limit when the deferred code returns.
func (tk *Task) Shielded(at string, f func()) {
	tk.shield++
	start := monoNow()
	defer func() { tk.shield-- }()
	f()
	if el := monoNow() - start; el > 5e9 && tk.w == nil {
		FaultCode("XO-F005", at, "a shielded defer ran for %s, longer than 5s", Duration(el))
	}
}

// block and unblock bracket a blocking wait so the virtual clock of a test
// can tell when every task is blocked (std.md 8.2).
func (tk *Task) block() {
	if tk.w != nil {
		tk.w.block(tk)
	}
}

func (tk *Task) unblock() {
	if tk.w != nil {
		tk.w.unblock(tk)
	}
}
