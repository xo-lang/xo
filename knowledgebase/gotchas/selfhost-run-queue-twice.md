# compiler/ssa run_queue run twice built every body twice

Status: Observed, fixed (2026-10-13, decision 0207)

Symptom: after making the SSA builders declare the C ABI exports after
the program's code (build, then declare_exports, then build again), the
self hosted emitters faulted in prune_unwind (`index 1 out of range for
List of length 1` in drop_check, compiler/ssa passes.xo) on every
program, while the Go builder was fine.

Cause: compiler/ssa's run_queue walked `b.queue` from index 0 and never
emptied it, so a second call rebuilt every queued body; the duplicate
landing checks then pointed at blocks whose terminators had changed.
The Go builder pops its queue as it goes.

Fix: run_queue empties the queue when it is done (build.xo). A pass
that may run twice must consume its work list, as the Go side does.
