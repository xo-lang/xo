# A wasm task that spins forever crashes Node instead of hanging

Status: Observed (2026-10-12), Node 22.22.1, selfhost/wasm stage 2.

**Symptom.** The goldens `conc_unwind` and `conc_rwlock_handoff`, run
under `selfhost/wasm/run.mjs`, printed their first lines and then
exited with status 133 and a V8 fatal error on stderr:
`Check failed: concurrent_marking_handle_ implies
!concurrent_marking_handle_->IsValid()` (in
`IncrementalMarkingJob::Task::RunInternal`), every run.

**Cause.** The program waited for its tasks in a loop that only polls
an Atomic (`while started.get() < 8 { }`). The wasm backend runs tasks
on one thread under a cooperative scheduler, so the loop never let the
other tasks run and spun forever; while the worker spins, V8's
incremental marking task on the main isolate fails that check with
`--single-threaded-gc` (gotchas/node-wasm-concurrent-gc-crash.md). The
crash is a symptom of the endless loop, not a separate bug.

**Fix.** Atomic operations and `task.is_canceled()` are scheduling
points in the wasm runtime (rt3.xo `$yield_point`: every 64th one
lets the other tasks run), as in the Go backend's deterministic
scheduler (gotchas/busy-loop-starves-deterministic-scheduler.md). A
loop with no operation of the runtime in it still spins forever under
the wasm backend; on a native backend the other tasks run on other
threads. When a wasm module dies with that V8 check, look for a task
that never waits.
