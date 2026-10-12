# A wasm fault flag cannot tell "unwinding" from "a fault is pending"

Status: Observed (2026-10-12), selfhost/wasm stage 2 (decision 0179).

**Symptom.** The golden `fault_tasks` printed
`fault: XO-F004: Mutex.with_read called re-entrantly in one task` after
a fault inside `m.with(fn(var xs) { ... fault(...) })` was caught by
`task.isolate`: the Mutex stayed locked. `fault_defer_chain` lost the
deferred lines that run while a fault unwinds.

**Cause.** The backend has no exception handling: a fault sets a flag
and every call is followed by a test of it. Stage 1 tested `$faulting`,
which stays set from the raise until `task.isolate` (or the end of
main) takes the fault. But on the native backends a landing pad stops
the unwinding: its cleanup code runs normally, and a function whose
unwind exit has no `rt.fault_continue` (a packed `with` closure hands
back the value it was given; a `Map.update` closure) returns to its
caller like any call, with the fault still pending. The caller then
unlocks the Mutex and only its `rt.unwinding` check sends the fault on.
Testing `$faulting` after every call returned at once instead, skipping
the unlock and every call in deferred cleanup.

**Fix.** A second flag, `$prop`: set when a fault is raised
(`$fault_set`) and by `rt.fault_continue` (from `$faulting`), cleared
when control reaches a landing block (`fault_exit` with a pad). The
test after a call reads `$prop`; `rt.unwinding` and the report at the
end of main read `$faulting`. The task scheduler saves both per task.
