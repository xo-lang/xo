package rt

import (
	"fmt"
	"runtime/metrics"
	"sync/atomic"
)

// Resource budgets (std.md 7, decision 0173): `task.with_budget(clock, b,
// f)` runs f in a child scope whose tasks share a budget of running time,
// allocation, and steps. Generated code polls it (Task.Tick) at the start
// of every loop iteration and at the entry of every function that calls
// other code, so pure computation is bounded too; the poll costs a nil
// check when no budget is active. Every budgetQuantum steps the thread
// charges what it used to the budget (and to the budgets enclosing it)
// and checks the ceilings; when one is passed the scope is canceled and
// the task unwinds at that poll, like at a cancellation point.

// budgetQuantum is how many steps a thread runs between charges.
const budgetQuantum = 1024

// budget is the state of one with_budget call, shared by its tasks.
type budget struct {
	parent *budget
	cpu    int64 // running time ceiling in nanoseconds, 0: none
	alloc  int64 // allocation ceiling in bytes, 0: none
	steps  int64 // step ceiling, 0: none

	usedCPU   atomic.Int64
	usedSteps atomic.Int64
	alloc0    uint64 // heap bytes allocated by the process at the start
	// over is the resource passed first: 1 cpu, 2 alloc, 3 steps
	over atomic.Int32
	node *cnode // canceled when a ceiling is passed
}

var budgetResources = [...]string{1: "cpu", 2: "alloc", 3: "steps"}

// Task_budget is `task.budget(cpu, alloc, steps)`.
func Task_budget(tk *Task, cpu Duration, alloc, steps int64) Task_Budget {
	return Task_Budget{F_cpu: cpu, F_alloc: alloc, F_steps: steps}
}

// heapAllocs reads the bytes the process has allocated on the heap so
// far. Go keeps no count per goroutine, so a budget's allocation is the
// process wide count since the budget started: an upper bound of what
// its tasks allocated (other tasks running at the same time count too).
func heapAllocs() uint64 {
	s := []metrics.Sample{{Name: "/gc/heap/allocs:bytes"}}
	metrics.Read(s)
	if s[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return s[0].Value.Uint64()
}

// Tick is the budget poll of generated code: a step of the current
// thread under a budget, nothing without one.
func (tk *Task) Tick() {
	if tk.bud != nil {
		tk.budgetTick()
	}
}

func (tk *Task) budgetTick() {
	th := tk.th
	if th == nil {
		return
	}
	th.budLeft--
	if th.budLeft > 0 {
		return
	}
	tk.budgetCharge()
	if b := tk.bud.exceeded(); b != nil {
		if tk.shield == 0 {
			panic(cancelUnwind{})
		}
		// deferred code runs shielded and cannot be unwound; one that
		// keeps working past twice the ceiling faults
		if r, used, limit := b.twice(); r != "" {
			FaultCode("XO-F008", xoLocation(2), "a budgeted task kept running in shielded code: %s %s, past twice its ceiling of %s", r, used, limit)
		}
		return
	}
	if tk.Canceled() {
		// under a budget a pure loop is a cancellation point too
		panic(cancelUnwind{})
	}
}

// budgetCharge charges the thread's steps and running time since the
// last charge to every budget of tk, and refills the step count.
func (tk *Task) budgetCharge() {
	th := tk.th
	if th == nil || tk.bud == nil {
		return
	}
	steps := th.budQ - th.budLeft
	now := monoNow()
	run := now - th.budRun
	th.budRun = now
	var allocs uint64
	q := int64(budgetQuantum)
	for b := tk.bud; b != nil; b = b.parent {
		usedS := b.usedSteps.Add(steps)
		usedC := b.usedCPU.Add(run)
		switch {
		case b.cpu > 0 && usedC > b.cpu:
			b.pass(1)
		case b.steps > 0 && usedS > b.steps:
			b.pass(3)
		case b.alloc > 0:
			if allocs == 0 {
				allocs = heapAllocs()
			}
			if int64(allocs-b.alloc0) > b.alloc {
				b.pass(2)
			}
		}
		if b.steps > 0 {
			if left := b.steps - usedS; left < q {
				q = max(left, 1)
			}
		}
	}
	th.budQ, th.budLeft = q, q
}

// budgetBlock and budgetUnblock bracket a blocking wait: the time a task
// waits is not running time.
func (tk *Task) budgetBlock() {
	if tk.bud != nil && tk.th != nil {
		th := tk.th
		steps := th.budQ - th.budLeft
		run := monoNow() - th.budRun
		for b := tk.bud; b != nil; b = b.parent {
			b.usedSteps.Add(steps)
			b.usedCPU.Add(run)
		}
		th.budQ = th.budLeft
	}
}

func (tk *Task) budgetUnblock() {
	if tk.bud != nil && tk.th != nil {
		tk.th.budRun = monoNow()
	}
}

// pass records that resource r passed its ceiling and cancels the
// budget's scope.
func (b *budget) pass(r int32) {
	if b.over.CompareAndSwap(0, r) {
		b.node.cancel()
	}
}

// exceeded is the innermost budget of the chain that passed a ceiling.
func (b *budget) exceeded() *budget {
	for ; b != nil; b = b.parent {
		if b.over.Load() != 0 {
			return b
		}
	}
	return nil
}

// twice reports the resource of an exceeded budget that is now past
// twice its ceiling, with the use and the ceiling as text.
func (b *budget) twice() (string, string, string) {
	switch b.over.Load() {
	case 1:
		if u := b.usedCPU.Load(); u > 2*b.cpu {
			return "cpu", Duration(u).String(), Duration(b.cpu).String()
		}
	case 3:
		if u := b.usedSteps.Load(); u > 2*b.steps {
			return "steps", fmt.Sprint(u), fmt.Sprint(b.steps)
		}
	case 2:
		if u := int64(heapAllocs() - b.alloc0); u > 2*b.alloc {
			return "alloc", fmt.Sprintf("%d bytes", u), fmt.Sprintf("%d bytes", b.alloc)
		}
	}
	return "", "", ""
}

// withBudget starts budget b on the task child (the first task of the
// budgeted scope, not yet running).
func (b *budget) start(child *Task) {
	child.bud = b
	if child.th != nil {
		child.th.budRun = monoNow()
		child.th.budQ, child.th.budLeft = 1, 1 // charge at the first poll
	}
}

// Task_with_budget is `task.with_budget(clock, b, f)` (std.md 7): runs f
// in a child scope under the budget; when a ceiling is passed the scope
// is canceled, its tasks unwind at their next poll, and the result is
// `Err(Exceeded(resource))`. A budget with no ceiling runs f unbudgeted
// (still in a child scope).
func Task_with_budget[T, E any](tk *Task, clock Clock, bb Task_Budget, f func(*Task) Result[T, E]) Result[T, Task_Budgeted[E]] {
	tk.CheckCancel()
	if bb.F_cpu < 0 || bb.F_alloc < 0 || bb.F_steps < 0 {
		Faultf(xoLocation(2), "task.with_budget: a negative ceiling (cpu %s, alloc %d, steps %d)", bb.F_cpu, bb.F_alloc, bb.F_steps)
	}
	// what the calling thread ran so far is the caller's
	tk.budgetCharge()
	s := NewScope(tk)
	b := &budget{parent: tk.bud, cpu: int64(bb.F_cpu), alloc: bb.F_alloc, steps: bb.F_steps, node: s.tasksNode}
	if b.alloc > 0 {
		b.alloc0 = heapAllocs()
	}
	h := spawnWith(tk, s, f, false, true, b)
	if !h.done.isSet() && !h.done.wait(tk, true, waitAwait, h) {
		// the caller was canceled
		s.shutdown()
		tk.Unwind()
	}
	s.shutdown()
	if tk.th != nil && tk.bud != nil {
		tk.th.budRun = monoNow()
	}
	if h.fault != nil {
		panic(h.fault)
	}
	if r := b.over.Load(); r != 0 {
		tk.CheckCancel()
		return Err[T](Task_Budgeted[E]{Tag: 1, P: Task_Budgeted_Exceeded[E]{F_resource: budgetResources[r]}})
	}
	if h.canceled {
		tk.CheckCancel()
		// canceled by an enclosing budget
		if ob := tk.bud.exceeded(); ob != nil {
			panic(cancelUnwind{})
		}
		return Err[T](Task_Budgeted[E]{Tag: 1, P: Task_Budgeted_Exceeded[E]{F_resource: "canceled"}})
	}
	if h.res.Ok {
		return Ok[T, Task_Budgeted[E]](h.res.V)
	}
	return ErrTr[T](Task_Budgeted[E]{Tag: 2, P: Task_Budgeted_Failed[E]{F0: h.res.E}}, h.res.Tr)
}
