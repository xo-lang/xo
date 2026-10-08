package rt

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// This file holds the parts of the runtime that watch tasks rather than run
// them: threads with what they are blocked on, the deterministic scheduler
// (core 7.4), the deadlock monitor of real runs and the deadlock report
// (core 11.4, XO-F001), and `xo dump` (core 11.4).

// waitKind is what a blocked thread waits for.
type waitKind uint8

const (
	waitNone waitKind = iota
	waitRecv
	waitSend
	waitSelect
	waitMutex
	waitSemaphore
	waitAwait
	waitScope
	waitOnce
	waitSleep
	waitTimeout
	waitStop
	waitServe
	waitDrain
	waitAdvance
)

var waitOps = [...]string{"", "receive", "send", "select", "Mutex.with", "Semaphore.with", "await", "scope end", "Once.get", "sleep", "with_timeout", "stop wait", "serve", "drain", "advance"}

// waitTarget is an object a thread can block on. It describes itself for
// reports and names the threads that can end the wait, when they are known
// (the holder of a mutex, an awaited task, the tasks of a scope).
type waitTarget interface {
	waitDesc() (string, []*thread)
}

// thread is one goroutine that runs Xo code: a program's main, a test, a
// spawned task, an HTTP request, or a runtime helper. Tasks that run on the
// same goroutine (a scope body and the task that opened it, the handler
// task of an in memory request) share it. A thread records what it is
// blocked on, with the source location, for deadlock reports and `xo dump`,
// and is the unit the deterministic scheduler hands the turn to.
type thread struct {
	id   int64
	kind string        // "main", "test", "task", "request", "runtime"
	site [4]uintptr    // where it was spawned
	ids  *atomic.Int64 // id source shared by the threads of a run
	sch  *sched        // the deterministic scheduler, nil when goroutines run freely
	mon  *monitor      // the deadlock monitor of a real run, nil otherwise

	// scheduler state, guarded by the scheduler's lock
	turn       chan struct{} // the turn is handed over here
	st         uint8
	pend       bool  // woken before it blocked
	stk        *Task // the task blocked on this thread
	cancelable bool  // the blocked wait ends on cancellation

	mu      sync.Mutex
	wtk     *Task // the task blocked on this thread, nil when not blocked
	wk      waitKind
	wobj    any
	wpcs    [8]uintptr
	wn      int
	counted bool // counted as blocked by the monitor
	done    bool
	// labeled: the goroutine carries the profiler label xo_thread=<id>
	// (lazy call sites, see setWait); touched only by the thread itself
	labeled bool
}

func newRootThread(kind string) *thread {
	th := &thread{kind: kind, ids: new(atomic.Int64)}
	th.id = th.ids.Add(1)
	return th
}

// newChild returns a thread of the same run.
func (p *thread) newChild(kind string) *thread {
	th := &thread{id: p.ids.Add(1), kind: kind, ids: p.ids, sch: p.sch, mon: p.mon}
	if th.sch != nil {
		th.turn = make(chan struct{}, 1)
	}
	return th
}

// label names the thread in reports: `main`, `test`, `task 3 (spawned at
// main.xo:7)`.
func (th *thread) label() string {
	switch th.kind {
	case "main", "test":
		return th.kind
	}
	s := fmt.Sprintf("%s %d", th.kind, th.id)
	if loc := xoLocationPCs(th.site[:]); loc != "" {
		s += " (spawned at " + loc + ")"
	}
	return s
}

func (th *thread) shortLabel() string {
	switch th.kind {
	case "main", "test":
		return th.kind
	}
	return fmt.Sprintf("%s %d", th.kind, th.id)
}

// setWait records what the thread is about to block on.
//
// The call site costs a stack walk (about 200ns), a third of an unbuffered
// channel handoff. Test worlds and the deterministic scheduler record it
// here; a plain real run does not, and recovers the call sites of blocked
// threads only when a dump or a deadlock report needs them, from the
// goroutine profile: each thread labels its goroutine with its id the
// first time it blocks (lazyLocs).
func (th *thread) setWait(tk *Task, k waitKind, obj any) {
	var pcs [8]uintptr
	n := 0
	if tk.w != nil || th.sch != nil {
		n = runtime.Callers(3, pcs[:])
	} else if !th.labeled {
		th.labeled = true
		pprof.SetGoroutineLabels(pprof.WithLabels(context.Background(), pprof.Labels("xo_thread", strconv.FormatInt(th.id, 10))))
	}
	count := th.mon != nil && countedWait(k, obj)
	th.mu.Lock()
	th.wtk, th.wk, th.wobj, th.wpcs, th.wn = tk, k, obj, pcs, n
	th.counted = count
	th.mu.Unlock()
	if count {
		th.mon.blockedOne()
	}
}

// clearWait records that the thread runs again.
func (th *thread) clearWait() {
	th.mu.Lock()
	th.wtk, th.wk, th.wobj = nil, waitNone, nil
	c := th.counted
	th.counted = false
	th.mu.Unlock()
	if c {
		th.mon.unblocked()
	}
}

// woken is called by a waker in a real run: the thread stops counting as
// blocked at once, so the monitor never sees a woken thread as blocked.
func (th *thread) woken() {
	if th.mon == nil {
		return
	}
	th.mu.Lock()
	c := th.counted
	th.counted = false
	th.mu.Unlock()
	if c {
		th.mon.unblocked()
	}
}

// countedWait reports whether a wait can only be ended by another task of
// the program: waits on timers, signals, and the outside world are not.
func countedWait(k waitKind, obj any) bool {
	switch k {
	case waitRecv, waitSend:
		if c, ok := obj.(interface{ isExt() bool }); ok && c.isExt() {
			return false
		}
		return true
	case waitSelect:
		s, ok := obj.(*Select)
		return ok && !s.ext
	case waitMutex, waitSemaphore, waitAwait, waitScope, waitOnce:
		return true
	}
	return false
}

// waitSnap is a blocked thread as seen by a report.
type waitSnap struct {
	th   *thread
	tk   *Task
	kind waitKind
	obj  any
	loc  string
}

// snap is the thread's wait, if it is blocked. locs holds the call sites
// of threads that did not record theirs (lazyLocs), or nil.
func (th *thread) snap(locs map[int64]string) (waitSnap, bool) {
	th.mu.Lock()
	defer th.mu.Unlock()
	if th.wtk == nil || th.done {
		return waitSnap{}, false
	}
	loc := xoLocationPCs(th.wpcs[:th.wn])
	if th.wn == 0 {
		loc = locs[th.id]
	}
	if sc, ok := th.wobj.(*Scope); ok && th.wk == waitScope {
		// Close runs in generated deferred code: the scope's own place
		// says more
		if l := xoLocationPCs(sc.bodyNode.site[:]); l != "" {
			loc = l
		}
	}
	return waitSnap{th: th, tk: th.wtk, kind: th.wk, obj: th.wobj, loc: loc}, true
}

// describe is `blocked in receive at main.xo:9 on a channel made at
// main.xo:4` and the threads that can end the wait.
func (w waitSnap) describe() (string, []*thread) {
	s := "blocked in " + waitOps[w.kind]
	if w.loc != "" {
		s += " at " + w.loc
	}
	var holders []*thread
	if t, ok := w.obj.(waitTarget); ok {
		d, hs := t.waitDesc()
		if d != "" {
			s += " " + d
		}
		holders = hs
	}
	return s, holders
}

// ---------------------------------------------------------------------------
// Deterministic scheduler (core 7.4)

const (
	stReady uint8 = iota + 1
	stRunning
	stBlocked
	stOutside // waiting on the outside world (I/O), off the scheduler
	stDone
)

// sched is the deterministic scheduler of `xo test` and `xo run --seed=N`
// (core 7.4). Exactly one thread holds the turn and runs; the others are
// ready, blocked, or outside (in I/O). At every scheduling point (channel
// operations, await, Mutex.with, sleep, effect calls, spawn, Atomic
// operations) the running thread gives up the turn and a seeded PRNG
// chooses the next thread among the ready ones, so the same seed gives the
// same interleaving.
//
// Wakers move a blocked thread to the ready list; a thread that blocks
// gives the turn away. In a test world the scheduler shares the world's
// lock and drives its virtual clock: when no thread is ready, the next
// timer fires; when no timer is pending either, every thread is blocked
// for good and the world reports a deadlock. With the real clock (`xo run
// --seed`) the scheduler waits for a timer or I/O to wake a thread.
type sched struct {
	mu      *sync.Mutex
	w       *World // nil with the real clock
	rng     *rand.Rand
	cur     *thread
	ready   []*thread
	threads []*thread // live threads in creation order
	picking bool
	started int // threads started after the first
}

func newSched(mu *sync.Mutex, w *World, seed int64) *sched {
	return &sched{mu: mu, w: w, rng: rand.New(rand.NewPCG(uint64(seed), 0x5eed5c4ed0a11ce5))}
}

// startLocked adds a new thread as ready. Its goroutine must wait for the
// turn (<-th.turn) before it runs.
func (s *sched) startLocked(th *thread) {
	th.sch = s
	if th.turn == nil {
		th.turn = make(chan struct{}, 1)
	}
	if len(s.threads) > 0 {
		s.started++
	}
	th.st = stReady
	s.ready = append(s.ready, th)
	s.threads = append(s.threads, th)
	if s.cur == nil && !s.picking {
		s.pickLocked()
	}
}

func (s *sched) start(th *thread) {
	s.mu.Lock()
	s.startLocked(th)
	s.mu.Unlock()
}

// pickLocked hands the turn to a ready thread chosen by the PRNG. With
// none ready it advances the virtual clock of a test world.
func (s *sched) pickLocked() {
	s.picking = true
	defer func() { s.picking = false }()
	for {
		if n := len(s.ready); n > 0 {
			i := 0
			if n > 1 {
				i = int(s.rng.Uint64() % uint64(n))
			}
			th := s.ready[i]
			s.ready = append(s.ready[:i], s.ready[i+1:]...)
			th.st = stRunning
			s.cur = th
			th.turn <- struct{}{}
			return
		}
		if s.w == nil || !s.w.schedIdleLocked() {
			return
		}
	}
}

// yield is a scheduling point: the running thread lets the PRNG choose who
// runs next (possibly itself).
func (s *sched) yield(th *thread) {
	s.mu.Lock()
	if s.cur != th || len(s.ready) == 0 {
		s.mu.Unlock()
		return
	}
	th.st = stReady
	s.ready = append(s.ready, th)
	s.cur = nil
	s.pickLocked()
	s.mu.Unlock()
	<-th.turn
}

// block gives the turn away and waits until a waker made the thread ready
// and the scheduler chose it.
func (s *sched) block(th *thread, tk *Task, cancelable bool) {
	s.mu.Lock()
	if th.pend {
		th.pend = false
		s.mu.Unlock()
		return
	}
	th.st = stBlocked
	th.stk = tk
	th.cancelable = cancelable
	if s.cur == th {
		s.cur = nil
		s.pickLocked()
	}
	s.mu.Unlock()
	<-th.turn
}

// unblock drops a wakeup that arrived after the wait ended.
func (s *sched) unblock(th *thread) {
	s.mu.Lock()
	th.pend = false
	th.stk = nil
	s.mu.Unlock()
}

// wakeLocked makes a blocked thread ready. A thread that has not blocked
// yet (it registered as a waiter and is about to block) keeps running.
func (s *sched) wakeLocked(th *thread) {
	switch th.st {
	case stBlocked:
		th.st = stReady
		th.stk = nil
		s.ready = append(s.ready, th)
		if s.cur == nil && !s.picking {
			s.pickLocked()
		}
	case stRunning:
		th.pend = true
	}
}

func (s *sched) wake(th *thread) {
	s.mu.Lock()
	s.wakeLocked(th)
	s.mu.Unlock()
}

// wakeCanceledLocked makes ready the threads blocked in a cancelable wait
// below the canceled nodes.
func (s *sched) wakeCanceledLocked(set map[*cnode]bool) {
	for _, th := range s.threads {
		if th.st == stBlocked && th.cancelable && th.stk != nil && th.stk.node.under(set) {
			s.wakeLocked(th)
		}
	}
}

func (s *sched) wakeCanceled(nodes []*cnode) {
	set := make(map[*cnode]bool, len(nodes))
	for _, n := range nodes {
		set[n] = true
	}
	s.mu.Lock()
	s.wakeCanceledLocked(set)
	s.mu.Unlock()
}

// endLocked removes a finished thread and hands the turn on.
func (s *sched) endLocked(th *thread) {
	th.st = stDone
	for i, x := range s.threads {
		if x == th {
			s.threads = append(s.threads[:i], s.threads[i+1:]...)
			break
		}
	}
	if s.cur == th {
		s.cur = nil
		s.pickLocked()
	}
}

func (s *sched) end(th *thread) {
	s.mu.Lock()
	s.endLocked(th)
	s.mu.Unlock()
}

// leave takes the running thread off the scheduler while it waits on the
// outside world (I/O): other threads run meanwhile.
func (s *sched) leave(th *thread) {
	s.mu.Lock()
	if s.cur != th {
		s.mu.Unlock()
		return
	}
	th.st = stOutside
	s.cur = nil
	s.pickLocked()
	s.mu.Unlock()
}

// enter puts a thread back after leave and waits for its turn.
func (s *sched) enter(th *thread) {
	s.mu.Lock()
	if th.st != stOutside {
		s.mu.Unlock()
		return
	}
	th.st = stReady
	s.ready = append(s.ready, th)
	if s.cur == nil && !s.picking {
		s.pickLocked()
	}
	s.mu.Unlock()
	<-th.turn
}

// snapsLocked lists the blocked threads in creation order.
func (s *sched) snapsLocked() []waitSnap {
	var out []waitSnap
	for _, th := range s.threads {
		if sn, ok := th.snap(nil); ok {
			out = append(out, sn)
		}
	}
	return out
}

// Yield is a scheduling point of the deterministic scheduler; it does
// nothing when goroutines run freely.
func (tk *Task) Yield() {
	if tk == nil || tk.th == nil {
		return
	}
	if s := tk.th.sch; s != nil {
		s.yield(tk.th)
	}
}

// leave and enter bracket a wait on the outside world (network, standard
// input, a child process) so other threads run under the deterministic
// scheduler meanwhile.
func (tk *Task) leave() {
	if tk != nil && tk.th != nil && tk.th.sch != nil {
		tk.th.sch.leave(tk.th)
	}
}

func (tk *Task) enter() {
	if tk != nil && tk.th != nil && tk.th.sch != nil {
		tk.th.sch.enter(tk.th)
	}
}

// realSched is the scheduler of `xo run --seed=N` (XO_SEED), nil otherwise.
var realSched *sched

// ---------------------------------------------------------------------------
// Deadlock monitor of real runs (core 11.4)

// monitor detects that every thread of a real program is blocked on
// something only another task could end (a channel, a mutex, an awaited
// task, a scope): then nothing can ever run again. Waits on timers,
// signals, and the outside world do not count. A waker uncounts the woken
// thread before it signals it; cancellation bumps gen. A suspected
// deadlock is reported only after the counts stayed equal and nothing
// moved for a quiet period, which covers the instants between a
// cancellation and the canceled threads resuming.
type monitor struct {
	live     atomic.Int64
	blocked  atomic.Int64
	gen      atomic.Uint64
	checking atomic.Bool
	quiet    time.Duration
	root     *cnode
	report   func(*FaultErr)
}

// runMon is the monitor of the running program (RunMain), nil otherwise.
var runMon atomic.Pointer[monitor]

func (m *monitor) started() {
	m.live.Add(1)
	m.gen.Add(1)
}

func (m *monitor) ended() {
	m.live.Add(-1)
	m.gen.Add(1)
	m.check()
}

func (m *monitor) blockedOne() {
	m.blocked.Add(1)
	m.gen.Add(1)
	m.check()
}

func (m *monitor) unblocked() {
	m.blocked.Add(-1)
	m.gen.Add(1)
}

func (m *monitor) allBlocked() bool {
	l := m.live.Load()
	return l > 0 && m.blocked.Load() >= l
}

func (m *monitor) check() {
	if m.allBlocked() && m.checking.CompareAndSwap(false, true) {
		go m.watch()
	}
}

func (m *monitor) watch() {
	for {
		g := m.gen.Load()
		time.Sleep(m.quiet)
		if !m.allBlocked() {
			m.checking.Store(false)
			// a block that raced with the reset starts the watch again
			if m.allBlocked() && m.checking.CompareAndSwap(false, true) {
				continue
			}
			return
		}
		if m.gen.Load() == g {
			break
		}
	}
	snaps := treeSnaps(m.root)
	m.report(deadlockFault(snaps, "every task is blocked"))
}

// clearLabels drops the goroutine's profiler labels (see setWait).
func clearLabels() { pprof.SetGoroutineLabels(context.Background()) }

// lazyLocs maps the ids of blocked threads that did not record their call
// site to the innermost Xo frame of their goroutine, read from the
// goroutine profile (see setWait).
func lazyLocs() map[int64]string {
	var b bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&b, 1); err != nil {
		return nil
	}
	out := map[int64]string{}
	for _, rec := range strings.Split(b.String(), "\n\n") {
		id, loc := int64(-1), ""
		for _, l := range strings.Split(rec, "\n") {
			if rest, ok := strings.CutPrefix(l, "# labels: "); ok {
				const key = `"xo_thread":"`
				if i := strings.Index(rest, key); i >= 0 {
					v := rest[i+len(key):]
					if j := strings.IndexByte(v, '"'); j >= 0 {
						if n, err := strconv.ParseInt(v[:j], 10, 64); err == nil {
							id = n
						}
					}
				}
				continue
			}
			if loc != "" || !strings.HasPrefix(l, "#\t") {
				continue
			}
			f := strings.Fields(l[1:])
			if len(f) >= 3 && strings.Contains(f[len(f)-1], ".xo:") && !isGlue(f[1]) {
				loc = f[len(f)-1]
			}
		}
		if id >= 0 && loc != "" {
			out[id] = loc
		}
	}
	return out
}

// treeSnaps lists the blocked threads below root, in creation order.
func treeSnaps(root *cnode) []waitSnap {
	locs := lazyLocs()
	seen := map[*thread]bool{}
	var out []waitSnap
	var walk func(n *cnode)
	walk = func(n *cnode) {
		if n.tk != nil && n.tk.th != nil && !seen[n.tk.th] {
			seen[n.tk.th] = true
			if sn, ok := n.tk.th.snap(locs); ok {
				out = append(out, sn)
			}
		}
		for _, c := range n.kids() {
			walk(c)
		}
	}
	for _, r := range allRoots(root) {
		walk(r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].th.id < out[j].th.id })
	return out
}

// deadlockFault is fault XO-F001 (core 11.4): the wait cycle when there is
// one, else every blocked thread, each with what it waits on and where.
func deadlockFault(snaps []waitSnap, none string) *FaultErr {
	byTh := map[*thread]int{}
	for i, s := range snaps {
		byTh[s.th] = i
	}
	descs := make([]string, len(snaps))
	edges := make([][]int, len(snaps))
	for i, s := range snaps {
		d, holders := s.describe()
		// every task is blocked, so nobody can complete a channel operation
		switch s.kind {
		case waitRecv:
			d += " that no task can send on"
		case waitSend:
			d += " that no task can receive from"
		}
		descs[i] = s.th.label() + " " + d
		for _, h := range holders {
			if j, ok := byTh[h]; ok {
				edges[i] = append(edges[i], j)
			}
		}
	}
	cycle := findCycle(edges)
	var b strings.Builder
	msg := "deadlock: " + none
	loc := ""
	if len(cycle) > 0 {
		msg = fmt.Sprintf("deadlock: a wait cycle of %d tasks", len(cycle))
		if len(cycle) == 1 {
			msg = "deadlock: a task waits for itself"
		}
		b.WriteString("wait cycle:")
		for _, i := range cycle {
			b.WriteString("\n" + descs[i])
		}
		loc = snaps[cycle[0]].loc
		in := map[int]bool{}
		for _, i := range cycle {
			in[i] = true
		}
		first := true
		for i := range snaps {
			if in[i] {
				continue
			}
			if first {
				b.WriteString("\nalso blocked:")
				first = false
			}
			b.WriteString("\n" + descs[i])
		}
	} else {
		b.WriteString("blocked:")
		for i := range snaps {
			b.WriteString("\n" + descs[i])
		}
		if len(snaps) > 0 {
			loc = snaps[0].loc
		}
	}
	return &FaultErr{Msg: msg, Code: "XO-F001", Loc: loc, Detail: b.String()}
}

// findCycle returns the first cycle of the graph in index order, or nil.
func findCycle(edges [][]int) []int {
	const (
		white = iota
		grey
		black
	)
	color := make([]int, len(edges))
	var stack []int
	var found []int
	var dfs func(v int) bool
	dfs = func(v int) bool {
		color[v] = grey
		stack = append(stack, v)
		for _, u := range edges[v] {
			if color[u] == grey {
				for k, x := range stack {
					if x == u {
						found = append([]int(nil), stack[k:]...)
						return true
					}
				}
			}
			if color[u] == white && dfs(u) {
				return true
			}
		}
		stack = stack[:len(stack)-1]
		color[v] = black
		return false
	}
	for v := range edges {
		if color[v] == white && dfs(v) {
			return found
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// xo dump (core 11.4)

// Node kinds of the cancellation tree, for `xo dump`.
const (
	nodePlain uint8 = iota
	nodeRoot
	nodeBody
	nodeTasks
	nodeTask
	nodeServer
	nodeRequest
)

// nodeSeq orders the nodes of the tree by creation.
var nodeSeq atomic.Uint64

// extraRoots are roots of the tree outside main's (the request tasks of
// a real HTTP server), shown by `xo dump` and the deadlock monitor.
var extraRoots sync.Map

// allRoots is root followed by the extra roots in creation order.
func allRoots(root *cnode) []*cnode {
	out := []*cnode{root}
	var extra []*cnode
	extraRoots.Range(func(k, _ any) bool {
		extra = append(extra, k.(*cnode))
		return true
	})
	sort.Slice(extra, func(i, j int) bool { return extra[i].seq < extra[j].seq })
	return append(out, extra...)
}

// kids returns the children of n in creation order.
func (n *cnode) kids() []*cnode {
	n.mu.Lock()
	ks := make([]*cnode, 0, len(n.children))
	for k := range n.children {
		ks = append(ks, k)
	}
	n.mu.Unlock()
	sort.Slice(ks, func(i, j int) bool { return ks[i].seq < ks[j].seq })
	return ks
}

// dumpText is the task tree below root (`xo dump`, SIGQUIT): each scope,
// its tasks, and what each task is blocked on, with locations. The root
// node is shown through the body of main's (or the test's) root scope.
func dumpText(root *cnode) string {
	locs := lazyLocs()
	tasks := 0
	var lines []string
	var walk func(n *cnode, depth int)
	walk = func(n *cnode, depth int) {
		kids := n.kids()
		line := ""
		switch n.kind {
		case nodeTasks, nodePlain, nodeRoot:
			for _, k := range kids {
				walk(k, depth)
			}
			return
		case nodeServer:
			line = "requests of a server"
		case nodeBody:
			if p := n.parent; p != nil && p.kind == nodeRoot && p.tk != nil {
				line = p.tk.th.label()
				tasks++
			} else {
				line = "scope"
				if loc := xoLocationPCs(n.site[:]); loc != "" {
					line += " at " + loc
				}
			}
		default:
			if n.tk == nil || n.tk.th == nil {
				break
			}
			line = n.tk.th.label()
			tasks++
		}
		if n.tk != nil {
			line += ": " + nodeState(n, kids, locs)
		}
		if n.canceled.Load() {
			line += " (canceled)"
		}
		lines = append(lines, strings.Repeat("  ", depth)+line)
		for _, k := range kids {
			walk(k, depth+1)
		}
	}
	for _, r := range allRoots(root) {
		walk(r, 0)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "xo dump: pid %d, %d tasks\n", os.Getpid(), tasks)
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	b.WriteString("end of xo dump\n")
	return b.String()
}

// nodeState is what the task of n does: blocked (with what on and where),
// in a scope it opened, waiting for the tasks of its ended scope body,
// running, or ready (deterministic scheduler).
func nodeState(n *cnode, kids []*cnode, locs map[int64]string) string {
	th := n.tk.th
	if sn, ok := th.snap(locs); ok {
		if sn.tk.node == n {
			d, _ := sn.describe()
			if s := th.sch; s != nil {
				s.mu.Lock()
				if th.st == stReady {
					d += " (woken, waiting for its turn)"
				}
				s.mu.Unlock()
			}
			return d
		}
		if sc, ok := sn.obj.(*Scope); ok && sc.bodyNode == n {
			return "waiting for its tasks"
		}
	}
	for _, k := range kids {
		if k.kind == nodeBody && k.tk != nil && k.tk.th == th {
			return "in scope"
		}
	}
	th.mu.Lock()
	done := th.done
	th.mu.Unlock()
	if done {
		return "finished"
	}
	if s := th.sch; s != nil {
		s.mu.Lock()
		st := th.st
		s.mu.Unlock()
		if st == stReady {
			return "ready"
		}
	}
	return "running"
}

// DumpPath is where a program writes its dump on SIGQUIT for `xo dump
// <pid>`: $XO_DUMP_DIR or the system temporary directory.
func DumpPath(pid int) string {
	dir := os.Getenv("XO_DUMP_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, fmt.Sprintf("xo-dump-%d.txt", pid))
}

// writeDump prints the task tree to standard error and to DumpPath.
func writeDump(root *cnode) {
	text := dumpText(root)
	stderrWrite(text)
	path := DumpPath(os.Getpid())
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err == nil {
		os.Rename(tmp, path)
	}
}

// ---------------------------------------------------------------------------
// Wait descriptions

func (c *Chan[T]) waitDesc() (string, []*thread) {
	loc := xoLocationPCs(c.made[:])
	if loc == "" {
		return "on a channel", nil
	}
	return "on a channel made at " + loc, nil
}

func (c *Chan[T]) isExt() bool { return c.ext }

func (l *lock) waitDesc() (string, []*thread) {
	l.mu.Lock()
	owner, site := l.owner, l.site
	l.mu.Unlock()
	if !l.mutex {
		return "on a semaphore", nil
	}
	s := "on a mutex"
	if loc := xoLocationPCs(l.made[:]); loc != "" {
		s += " made at " + loc
	}
	if owner == nil {
		return s, nil
	}
	s += ", held by " + owner.th.shortLabel()
	if loc := site.loc(); loc != "" {
		s += " since " + loc
	}
	return s, []*thread{owner.th}
}

func (h *Handle[T, E]) waitDesc() (string, []*thread) {
	if h.th == nil {
		return "", nil
	}
	return "for " + h.th.shortLabel(), []*thread{h.th}
}

func (s *Scope) waitDesc() (string, []*thread) {
	s.mu.Lock()
	var ths []*thread
	var names []string
	for _, t := range s.tasks {
		if t.th != nil && !t.done.isSet() {
			ths = append(ths, t.th)
			names = append(names, t.th.shortLabel())
		}
	}
	s.mu.Unlock()
	if len(names) == 0 {
		return "", nil
	}
	return "for " + strings.Join(names, ", "), ths
}

func (o *onceState[T, E]) waitDesc() (string, []*thread) {
	o.mu.Lock()
	r := o.runner
	o.mu.Unlock()
	if r == nil {
		return "", nil
	}
	return "run by " + r.shortLabel(), []*thread{r}
}

func (s *Select) waitDesc() (string, []*thread) {
	var locs []string
	for _, c := range s.cases {
		if t, ok := c.obj.(waitTarget); ok {
			d, _ := t.waitDesc()
			locs = append(locs, strings.TrimPrefix(d, "on "))
		}
		if c.isAfter {
			locs = append(locs, "an after arm")
		}
	}
	if len(locs) == 0 {
		return "", nil
	}
	return "over " + strings.Join(locs, ", "), nil
}
