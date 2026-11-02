# std/task changes

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
