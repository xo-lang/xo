# std/task changes

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
