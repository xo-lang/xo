# A long wasm run crashes Node's single threaded collector; a spinning task runs forever

Status: Observed (2026-10-12), Node 22.22.1, selfhost/wasm stage 2.

**Symptom.** The goldens `conc_unwind` and `conc_rwlock_handoff`, run
under `selfhost/wasm/run.mjs`, printed their first lines and then
exited with status 133 and a V8 fatal error on stderr:
`Check failed: concurrent_marking_handle_ implies
!concurrent_marking_handle_->IsValid()` (in
`IncrementalMarkingJob::Task::RunInternal`), every run. Later the self
hosted checker built as wasm printed its whole, correct output and then
died the same way.

**Cause.** Two things. (1) The goldens waited for their tasks in a loop
that only polls an Atomic (`while started.get() < 8 { }`); the wasm
backend runs tasks on one thread under a cooperative scheduler, so the
loop never let the other tasks run and spun forever. (2) Any run that
lasts seconds lets an incremental marking task of the main isolate
(which waits for the worker) run, and with `--single-threaded-gc`
(gotchas/node-wasm-concurrent-gc-crash.md) that task fails the check
above: a V8 bug, not the module's.

**Fix.** (1) Atomic operations and `task.is_canceled()` are scheduling
points in the wasm runtime (rt3.xo `$yield_point`: every 64th lets the
other tasks run), as in the Go backend's deterministic scheduler
(gotchas/busy-loop-starves-deterministic-scheduler.md); a loop with no
operation of the runtime in it still spins forever under the wasm
backend (on a native backend the other tasks run on other threads).
(2) run.mjs sets `--no-incremental-marking` with `--single-threaded-gc`;
the checker then exits 0. Retest without it when Node is upgraded.
