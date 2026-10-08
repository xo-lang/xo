# A loop without scheduling points starves the deterministic scheduler

Status: Observed (2026-10-08, running the `cancellation` golden under
`xo run --seed=1`)

Symptom: a program that passes normally hangs under `xo run --seed=N`
(or `XO_SEED`). `xo dump <pid>` shows one task `running` and the others
`blocked ... (woken, waiting for its turn)`. SIGTERM does not stop it:
the first signal only requests a stop (core 7.5), and the second cancels
tasks that never reach a cancellation point.

Cause: the deterministic scheduler (core 7.4, std/rt/sched.go) runs one
task at a time and switches only at scheduling points (channel
operations, await, Mutex.with, sleep, effect calls, spawn, Atomic
operations). The golden polled `while !task.is_canceled() { ... }` with
nothing else in the loop: the timer of `task.with_timeout` woke the
waiting task, but the polling task never gave up the turn, so nobody ran
the cancellation. A test world shows the same loop as a hang too: its
virtual clock only moves when every task is blocked.

Fix: `task.is_canceled()` and `task.stop_requested()` are scheduling
points (2026-10-08). Any other loop that spins without one still starves
the others: put a channel operation, `clock.sleep`, or an effect call in
it. To stop such a process that this session started, send a signal to
its own PID only, and `kill -9 <PID>` if two SIGTERMs do not end it.
