# std/task changes

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
