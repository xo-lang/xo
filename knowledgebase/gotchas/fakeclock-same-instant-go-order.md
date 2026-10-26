# Timers due at the same instant woke their tasks in reverse on the Go backend

Status: Observed and fixed 2026-10-26 (std/rt/world.go fireNextLocked)

Symptom: std.md 8.2 says timers due at the same instant fire in the order
they were scheduled, and on arm64 and LLVM they did. On the Go backend two
tasks that slept to the same deadline appended their results in the reverse
of the order they had called `sleep`: three tasks sleeping 20s, 10s, 10s
then `advance(10s)` woke `[c,b]` where b had scheduled first (the corpus
test in internal/stdtest/testdata/corpus/fakeclock now checks the order).

Cause: the Go world fired every timer of the instant in one call
(`fireNextLocked` popped the heap while `at <= earliest`, calling each
timer's `fire`, which wakes its thread). Both woken threads were then
ready at once, and the deterministic scheduler picks a ready thread by its
seeded PRNG (`pickLocked`, std/rt/sched.go), so the second timer's task
could run first. The fire order itself was right; the run order was not
the fire order. arm64 was right because its run queue is a FIFO ring: the
woken fibers were queued in fire order.

Fix: `fireNextLocked` fires one timer, the earliest, per call. The callers
already loop (`schedIdleLocked`, `changedLocked`, and Advance's goroutine
path), so the woken task runs and blocks again before the next same-instant
timer fires, and the scheduled order holds whatever the seed picks.

Trap for a test of this: which task reaches `sleep` first is not fixed on
the Go backend (a spawn need not run until the task blocks, and the
scheduler picks at random), so a test that spawns two tasks and expects
`b` then `c` from spawn order is testing the scheduler's seed, not the
clock. Schedule the second timer from a later point of the clock instead
(`sleep(5)` in the main task, then a task that sleeps 5 to the same
deadline), so the scheduled order is fixed whatever order the tasks start
in.
