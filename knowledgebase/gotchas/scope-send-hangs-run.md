# A blocked send inside a scope hangs `xo run` instead of failing

Status: Observed (2026-10-08, writing the map_value_semantics golden).
The hang is gone since 2026-10-08: the deadlock monitor of real runs
faults with XO-F001 (see the end).

Symptom: a program whose spawned task sends on an unbuffered channel that
is only received after the `scope` block never exits under `xo run` (or a
built binary); nothing is printed after the last line before the scope
ends. `xo test` would report the deadlock (its virtual clock sees every
task blocked), but `xo run` uses real capabilities and has no such check,
and Go's own "all goroutines are asleep" detector does not fire because
the runtime keeps other goroutines (signal handling) alive.

Cause: the scope waits for its tasks; the task waits for a receiver that
runs after the scope. A plain deadlock in the program, not a runtime bug.

Fix: give the channel a buffer (`Chan[T].new(cap: 1)`) or receive inside
the scope. When a golden program hangs, list its processes and stop only
the binary that this session started (`kill <PID>`), never by name.

Update (2026-10-08): real runs now have a deadlock monitor (core 11.4,
std/rt/sched.go). When every task is blocked on another task (channel,
mutex, await, scope end) for 200 ms, the program prints
`fault: XO-F001: deadlock: every task is blocked` with each task, what it
waits on, and where, writes a crash bundle, and exits with status 70.
Golden `deadlock_real` is this exact program. Waits on timers, signals,
`task.stopping()`, or I/O do not count, so a program sleeping or serving
is never reported.
