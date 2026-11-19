# std/task changes

## 2026-11-19: perf: map_concurrent runs limit tasks, not a task per item

- Kind: perf
- Commit: this push
- Found by: perf row bench/perf/task map (task #419: 1.78 of Go's goroutine per item with a channel semaphore on the linux/amd64 host)
- Go reference: none (errgroup.SetLimit spawns a goroutine per item, as map_concurrent did)
- Test: internal/llvm TestMapConcurrentSpawns (10 calls of 1,000 items, limit 8: 10,000 spawns before, at most 200 after; order, errors and the limit checked); goldens map_until, task_native, conc_mutex_snapshot and big_struct on arm64 and llvm

map_concurrent spawns min(limit, n) tasks, and each takes the next item
from a shared counter until none is left, instead of a task per item
behind a channel of permits. Results keep input order and each item's
error; at most limit items run at once. A task per item cost a spawn, a
join and the permit channel's send and recv, about 2.5 times the work of
a 1 us item. `xo bench --ab` bench/perf/task map: wall 0.305 (0.303 to
0.311) on linux/amd64 (139 to 42 ms; Go 82 ms) and 0.337 (0.253 to
0.462) on host 1 (linux/arm64). The tasks are long enough now to run on
other threads, where the per item tasks ran inline on the spawner.

## 2026-11-19: perf: a done Once.get is one load, with no lock

- Kind: perf
- Commit: this push
- Found by: perf row bench/perf/task once (task #419: 11.7 of Go's sync.OnceValues on the linux/amd64 host after the read lock change)
- Go reference: sync.OnceValues (a done flag read atomically, then the stored values)
- Test: golden once_published (counted values read by 8 tasks while the first get runs, a caller's copy changed, a failed Once never published); internal/arm64 TestOnceGetFast ceiling 0.9G instructions (0.44G now, 2.97G before)

A done Once keeps its value in a list of one (the box) and publishes the
box's address in an Atomic (Once.done) after the first get stores it; a
later get loads the address and reads the value through the new
uncounted rt.obj_peek, taking no lock (native and wasm targets; the Go
backend keeps the read lock). bench/perf/task once on linux/amd64:
30.3 to 2.35 ms (Go 2.58); `xo bench --ab` on linux/arm64 B/A 0.176
(0.163 to 0.182). A Once's Debug text gains `done: <redacted>` (the
field is a Secret, so no address shows).

## 2026-11-19: perf: a done Once.get copies out only its value

- Kind: perf
- Commit: this push
- Found by: perf row bench/perf/task once (task #419: 12.7 of Go's sync.OnceValues on the linux/amd64 host)
- Go reference: none (sync.OnceValues reads a done flag and returns the stored values)
- Test: internal/arm64 TestOnceGetFast; goldens task_native and test_once on every backend

A get of a done Once returns its value from under the read lock instead
of a copy of the whole state, whose Result and Isolated cases were
counted and matched on every get (a quarter of a get's profile on the
Mac). A failed Once now takes the write lock on each get, as a first get
does. 20,000,000 gets on the Mac: 0.19 to 0.14 s.

## 2026-11-17: perf: Semaphore.with takes a free permit without the channel lock

- Kind: perf
- Commit: this push
- Found by: perf row bench/perf/task sem (task #88: 2.51 of Go's buffered channel)
- Go reference: none (Go's sync primitives use the same write then read handoff)
- Test: goldens task_native and task_std on every backend; std/evidence/task/stress (a semaphore of 3 never lets a fourth in)

A free permit is now taken with one compare and swap and given back with
one atomic add; only a task that finds no free permit waits on the
channel, and a release that sees a waiter moves a permit to it. Host 1
`xo bench --ab` bench/perf/task sem: wall 97.7 to 13.6 ms (0.139, 95% CI
0.139 to 0.142). A semaphore with a free permit is no longer a
cancellation point on entry; a waiting one still is.

## 2026-11-17: fix: isolated faults in many tasks no longer report a data race under --race

- Kind: fix
- Commit: this push
- Found by: xw-1's host 3 race round (#152, task #225): TSan race at llrt/fault.xo reached through tasks.spawn
- Go reference: none (runtime)
- Test: internal/llvm/testdata/golden/race_fault_msg_tasks in TestRaceRuntimeInlinedUnchecked (fails before: TSan data race)
- Shipped in: not yet released

A fault message is built in its worker thread's buffer, which several
fibers on that worker use in turn; fiber switches carry no TSan
synchronization, so the writes looked racy. They are now ignored by
TSan as unwind.xo's register and scratch copies already were. Answers
were right before; only `--race` runs reported a false race.
## 2026-11-17: fix: a caught fault with a location no longer leaks

- Kind: fix
- Commit: this push
- Found by: std/sse stress (task #199): max RSS grew through a 30 minute run of Str slices faulting inside isolate
- Go reference: none
- Test: golden fault_isolate_leak (100,000 caught faults of each kind grow max RSS under 2 MiB; split and fault grew 6.2 MB before)

`task.isolate` catching a fault raised with a source location (a Str slice
splitting a UTF-8 character, `fault(msg)`, contract failures) leaked about
60 bytes per fault: std/native/llrt/fault.xo copied the location to the heap
for the raise, and fault.set copies it again into the context, so the first
copy was never freed. The location now goes into the per-thread message
buffer after the message, and only a location that does not fit there is
copied to the heap.
## 2026-11-17: perf: a done Once is read under the read lock (#103)

- Kind: perf
- Commit: this push
- Found by: #103 lever 7 (bench/perf/task once 2.62 times Go's sync.Once; the profile: write lock and value write back on every get)
- Go reference: sync.Once (Do's atomic done check)
- Test: internal/arm64 TestOnceGetFast (instruction count of 10,000,000 gets); bench/perf/task once
- Shipped in: not yet released

Once.get reads a done result with Mutex.with_read, which copies it out
and writes nothing back; only the first callers take the write lock,
where one runs f and the rest wait. 10,000,000 gets on the Mac: 6.71G to
2.97G instructions (20,000,000: 0.44 to 0.13 s of user time).
Host 1 (linux/arm64, `xo bench --ab`, 3 rounds, 2026-11-17): bench/perf/task
once 37.81 to 16.87 ms (B/A 0.446, 95% interval 0.443 to 0.449).
Xo Lang 2 had measured an earlier Once change about 11% slower on host 2
(a pass/fail host); the host 1 --ab, the timing host (decision 0340), is
the authority.

## 2026-11-11: refactor: std/task's Xo parts are one file on every backend (decisions 0588, 0597's amendment)

- Kind: refactor
- Commit: this push
- Found by: owner decision 2026-11-11 (rt.fault_raise and rt.task_deadline_left portable for std/task alone)
- Go reference: none
- Test: goldens task_native, task_deadline, task_budget, task_budget_fault, map_until, conc_mutex_snapshot, big_struct (arm64, llvm, Go parity, wasm)
- Shipped in: not yet released

std/native/task.xo moved into std/task.xo. Semaphore, Once, map_concurrent,
map_concurrent_until, deadline and budget are now the same Xo on the Go
backend, which compiled them from hand-written Go before (std/rt sync.go,
once.go, budget.go). isolate, with_timeout, with_budget, is_canceled,
stop_requested, stopping and Group's spawns stay lowered by the compiler.
A semaphore or map_concurrent argument fault still names the caller's line.

## 2026-11-11: docs: the header says what the module is for

- Kind: docs
- Commit: this commit
- Found by: pkg.xolang.org and xolang.dev showed `xo doc std`'s summary of std/task as a note about the file (the Xo Lead, 2026-11-11)
- Go reference: none
- Test: cmd/xo TestDocStdSummaries
- Shipped in: not yet released

The header comment of std/task.xo opens with a sentence that says what the module is for, which `xo doc std` lists as its summary. No declaration changed.

## 2026-11-10: refactor: integer bounds as `Int.max` and `Int.min` (decision 0587 point 4)

- Kind: refactor
- Commit: this commit
- Found by: decision 0587 point 4 (std's literal bounds become the type constants)
- Go reference: none (no behavior change; Go's math.MaxInt64 and MinInt64)
- Test: the std module tests and goldens of the gate; the self build

`9223372036854775807` and `-9223372036854775807 - 1` in std/task's code are
`Int.max` and `Int.min`; masks, Float literals, comments and texts keep their
digits. No behavior change.

## 2026-11-03: fix: Windows preemption no longer gives a task a wrong integer result under load

- Kind: fix
- Commit: this commit
- Found by: the 2026-11-02 Windows stress (async_preempt_regs under load), xo-lang-1
- Go reference: runtime/os_windows.go preemptM and runtime/preempt_amd64.s asyncPreempt (the thread saves its own registers)
- Test: async_preempt_regs, 16 rounds of 32 copies beside CPU burners on windows/amd64, the host to itself (main: 6 of 512 wrong sums; now 0 of 512, and 0 of 1,024 over two runs; async_preempt_alloc 0 of 512)

Present since v0.25.0 (native Windows preemption, decision 0318). A task
preempted on Windows was resumed from the registers the monitor had read
from its suspended thread, and in about 1 in 100 runs of a busy machine
one of its integer values came back wrong. The monitor now only sends the
thread to a routine that saves the thread's own registers on the task's
stack, parks the task, and restores them when it runs again, as Go does
(decision 0318's 2026-11-03 addendum).

## 2026-11-02: fix: Windows preemption no longer loses a preempted task's registers under load

- Kind: fix
- Commit: this commit
- Found by: the 2026-11-02 Windows stress (async_preempt_regs under load), xo-lang-1
- Go reference: runtime/os_windows.go preemptM (Go reads the thread's context into a buffer of the monitor's own)
- Test: async_preempt_regs and async_preempt_alloc, 4 rounds of 32 copies beside CPU burners on windows/amd64 (main: async_preempt_regs 26 of 128 failed, 13 hangs, 12 C0000005 and 1 wrong sum, async_preempt_alloc 2 of 128 hung; with the fix, 8 rounds: no hang and no fault in 512 runs, async_preempt_alloc 256 of 256; 8 of 256 async_preempt_regs runs still print a wrong integer sum, a separate bug still open)

Present since v0.25.0 (native Windows preemption, decision 0318). The monitor read a suspended thread's registers straight
into the fiber's save area. Under load the thread it had just sent to the park
routine could still be waiting to run when the next kick came, and that read
overwrote the registers the park routine was about to restore. The task then
looped in the park routine (a hang), faulted, or continued with wrong integer
registers. The registers are now read into the scratch half and copied into
the save area only when the thread is redirected.

## 2026-11-01: perf: runtime locks spin longer before a futex sleep on Linux

- Kind: perf
- Commit: this commit
- Found by: decision 0324's futex trace, xo-lang-4
- Go reference: none (Go's lock_futex spins differently, with its own active and passive rounds)
- Test: bench/results/lock-spin-2026-11-01.md (10 round HTTP bench, `--ab`, oversubscribed)

`lock_slow` spins 1600 rounds in place of 100 on Linux. Other systems are not
measured and keep 100.

## 2026-11-01: fix: an idle main thread no longer locks a finished inline task's park

- Kind: fix
- Commit: this commit
- Found by: tier 35, TestGoldenTasks/conc_reader_yield (ThreadSanitizer, linux/arm64 LLVM); cause by xo-lang-4
- Go reference: none
- Test: internal/arm64 TestHeapGuardMainInlinePark, TestGoldenTasks/conc_reader_yield

Present since main's body moved to a fiber (decision 0286, 2026-10-20). When main's fiber ran a spawned task inline and that task parked, main's idle thread could lock the task's park after the task had ended and its context had been freed or reused, so it could write into another object, crash or hang. Normal builds have not been seen to do that; the race detector reported it under load, and with the runtime's records against guard pages (XO_HEAP_GUARD=1) 366 of 500 loaded runs on linux/arm64 crashed there, and 0 of 500 with the fix. Main's thread now holds a claim on the park that main's fiber waits out before it goes on.

## 2026-10-30: feature: task.deadline

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 std survey (bench/survey-2026-10-30, gap 5,
  deadlines), decision 0430
- Go reference: context.Context.Deadline
- Test: internal/llvm/testdata/golden/task_deadline (native, Go and wasm
  backends)
- Timing: every spawn copies one more word (the deadline); `xo bench
  --ab` of bench/perf/conc/xo/spawn, 442b9712e against 89ed2225d on the
  linux/arm64 offload host, 10 rounds, 0 loaded: wall 28.78 against 27.42
  ms, B/A 0.953 (0.839 to 1.034), within noise

`task.deadline(clock)` gives the nearest `with_timeout` deadline the
current task works under, as an instant on clock, or None, as Go's
`ctx.Deadline()`. A task spawned inside a `with_timeout` keeps the
deadline in force where it was spawned, even after that body returns, so
a handler can pass the time left to a database or a peer.
