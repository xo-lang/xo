# 0068: Structured concurrency on the native backends: a thread pool runtime and explicit unwinding

Status: Accepted (2026-10-08); point 1 (a blocked task holds a thread) superseded by 0069, which also narrows point 7; the scheduler of point 1 and the counting of point 7 refined by 0077 (2026-10-09); Mutex waking refined by 0082 (2026-10-09). Implemented in `std/llrt/xo_rt.c`
("Tasks"), `internal/ssa/build_conc.go` (lowering), `internal/ssa`
(`KHandle`, `OpFuncAddr`, `Func.Entry`), `internal/llvm/fromssa.go` and
`internal/arm64/isel.go` (`rtCall`, handle counts). Tests:
`internal/llvm/testdata/golden/conc_*` (Go, LLVM, and arm64 must agree),
`internal/llvm/threads_test.go` (TestRuntimeTasks, TestGoldenTasks,
TestDeadlock), `std/llrt/testdata/tasks.c`.

## Context
Core 7 (scopes, spawn, await, cancellation, channels, select) and core
5.4 (Mutex, Atomic) ran only on the Go backend, where every task is a
goroutine, cancellation unwinds with a panic, and memory is garbage
collected. The native backends (decisions 0046, 0049) reference count
their memory and have no unwinding, and decision 0058 prepared shared
counts but left every crossing to this decision. The native HTTP server
needs tasks first.

## Decision
1. **Runtime: a pool of worker threads (1:1 while blocked, M:N
   otherwise).** Tasks are heap objects run to completion on worker
   threads: one worker per CPU (`XO_THREADS=n` overrides), started on
   the first spawn. Each worker has a deque (owner pushes and pops at the
   bottom, others steal from the top; spawns from other threads go to an
   inject queue). Tasks are not stackful. Instead:
   - `t.await` on a task nobody started runs it on the awaiting thread,
     and a scope's join runs the scope's tasks nobody started before it
     waits (the scope waits by helping).
   - A worker whose task blocks (a channel, a lock, await, a join)
     counts as blocked; when work is queued and fewer than one worker
     per CPU can run, another worker starts. So a blocked task holds a
     thread, but queued tasks never wait for blocked ones (the managed
     blocking of Java's ForkJoinPool). Workers beyond one per CPU exit
     when they find no work; their heaps (decision 0058) go to the next
     thread that starts.
   - Idle workers spin briefly (at most two) before they sleep; runtime
     mutexes use the first fit policy on Darwin
     (`gotchas/darwin-mutex-fairshare.md`); the deques, the cancellation
     tree, and the deadlock counters use spin locks.
2. **Cancellation tree.** Every task and every scope body is a node
   (a scope also has a node for its tasks, replaced by `s.cancel()`), as
   in std/rt. Canceling a node marks its subtree (sticky, core 7.2) and
   signals the park of each context blocked at a node in it. A context
   (the main program, or a running task) waits only on its own park; an
   object (channel, mutex, task, scope) keeps its waiters' parks and a
   waker claims a waiter before it hands it a value (Go's protocol in
   std/rt/chan.go).
3. **Unwinding is explicit in the SSA.** A cancellation point (send,
   recv, `for x in ch`, select, await, waiting for a lock) in a canceled
   context sets the context's unwinding flag and returns a placeholder
   (an immortal empty list or handle). After it, after each call of a
   user function that may unwind, and after each call of a function
   value, the builder emits `rt.unwinding` and a branch to the unwind
   path, which closes the scopes being left (mode 2) and returns zero
   values of the results (built so that releasing them is valid). The
   reference counting pass then releases everything live on that path:
   unwinding leaks nothing. After building, checks after calls of
   functions that cannot unwind (a fixpoint over the call graph; a
   function value call always may) are removed. A program without tasks
   is built without any check (the builder tries without them first).
   A scope whose child failed turns its body's unwinding into the
   child's error, returned as by `?` (std/rt's `Close`).
4. **Scope exits.** Every way out of a scope body closes it: the end of
   the body (mode 0: background tasks canceled), `return`, `?`, `break`,
   `continue` (mode 1: every task canceled), and unwinding (mode 2). A
   scope with a spawn that can fail with an error nobody awaits checks
   the close's result and returns that task's error, converted to the
   function's error type (spawn sites are numbered; the failed task
   reports its site).
5. **Values cross as lists of one, marked shared.** The runtime's
   interface keeps scalars and pointers only (decision 0046): a value of
   any type crosses as a `List[T]` of one element, which is one pointer.
   Each crossing marks it shared first (`OpShare`, decision 0058): a
   spawned closure (its captures), a task's result, a sent value, the
   value of a Mutex. Task, Scope, task.Group, Chan, Mutex, Atomic, and a
   select are handles (`KHandle`): runtime objects `{count, free, ...}`
   created shared (count -1), retained and released like lists. A
   container the runtime frees with values in it (a task's result, a
   channel's buffer, a mutex's value, a select's unsent values) releases
   them through an entry function of the element type (`droplist`).
   Entry functions (`Func.Entry`, parameters owned, the C calling
   convention, address taken with `OpFuncAddr`) also run each task: they
   call the closure and record its result, or that it unwound.
6. **Mutex.with** takes the value out under the lock; a closure with a
   `var` parameter is lowered to return (result, new value), stored back
   only when it succeeds; an error, unwinding, or a read only closure
   leaves the old value (the closure works on a copy, as on the Go
   backend). Re-entrant use faults with XO-F004 and both sites (a held
   set per context).
7. **Deadlock (XO-F001).** The runtime counts live contexts and blocked
   ones; when every live context is blocked and no task is queued, it
   faults. The native backends have no timers, I/O, or signals that
   could wake a blocked context, so this is exact. The report is the
   first line of the Go backend's without its location and the list of
   blocked tasks.
8. **The deterministic scheduler stays a Go backend feature** (core
   7.4: `xo test`, `--seed`, `--explore`, `XO_SEED`). Native builds run
   the real scheduler; a program whose output depends on scheduling may
   print differently from run to run.

## Consequences
- Every `conc_*` golden prints the same on the Go, LLVM, and arm64
  backends with no allocation live at exit, also under ThreadSanitizer
  with one worker, two, and one per CPU, with share probes on four
  copies at once, and over hundreds of repeated runs.
- Not lowered yet: `after` in `select`, `task.with_timeout`,
  `map_concurrent(_until)`, `isolate`, `Semaphore`, `Once`,
  `stop_requested`, `stopping`, `os.tasks`, `defer` (and so shielded
  defers), clock, I/O, and network effects. Each is a build error that
  names it. (`defer` and shielded defers: decision 0070, 2026-10-08.)
- A fault in a task exits the process from that task's thread at once;
  the Go backend first cancels and waits for the scope's other tasks.
  Without `defer` the difference shows only in output other tasks would
  have printed meanwhile.
- Spawn heavy code (bench note in the commit, indicative): a program of
  100,000 tiny tasks ran in 0.06 s with one worker and 0.33 to 0.48 s
  with twelve, against 0.15 to 0.24 s on the Go backend; contention on
  the runtime's locks and wake ups dominate with many workers. A
  channel pipeline of 100,000 values ran as fast as the Go backend.
- Each send and receive allocates its list of one. `Mutex.with` that
  changes a list or string in the value copies it, because the old value
  stays in the mutex until the closure succeeds (the Go backend copies
  too); moving the value out when the closure can neither fail nor
  unwind would update it in place.
- A blocked task holds a thread (64 MiB of reserved stack): thousands of
  tasks blocked at once mean thousands of threads (at most 4096 workers).

## Alternatives
- **Stackful green threads** (Go, Lean's tasks on fibers): a blocked task
  would not hold a thread, but each needs its own stack, context switch
  code per architecture, and guard pages; deferred until thread counts
  matter (the HTTP server with many idle connections may need it, or an
  event loop for I/O).
- **Unwinding with setjmp/longjmp or C++ style exceptions**: no checks on
  the normal path, but the values live in the skipped frames would leak
  (no landing pads to release them) and scopes would not close.
- **A hidden error result on every function that may unwind** (Swift's
  errors): what the checks do, without changing every signature; the
  zero values make the unwind path an ordinary early return for the
  reference counting pass.
- **One global task queue**: simpler, but every spawn and pop contends
  on one lock.
- **Copying values into the runtime by layout** (an element size and
  callbacks per call): every runtime function would need the type's
  layout; lists of one already carry it.
