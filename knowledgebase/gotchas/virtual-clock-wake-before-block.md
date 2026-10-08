# The virtual clock can jump while a woken task has not run yet

Status: Observed (2026-10-07, design of std/rt/world.go)

The test clock auto advances when every task is blocked (std.md 8.2). It
counts running tasks: blocking decrements, waking increments.

Trap 1: if the woken task increments the count itself when it resumes,
there is a window where every task looks blocked, and the clock fires the
next timer (for example a `with_timeout` deadline) before the woken task
runs. Fix: the waker (a channel send, a released mutex, a finished task, a
timer, a cancellation) counts the woken task as running before it
signals it. That requires the runtime to know who waits on what, so
channels, mutexes, semaphores, and `select` keep explicit wait queues
instead of using Go channel operations.

Trap 2: a waker can fire before the task has registered as blocked (it
registered as a waiter, then is about to block). The world records a
pending wakeup and the next block of that task does not decrement. Unblock
clears it, and a task that leaves a wait early removes itself from the
waiter list, so no stale wakeup survives.

When every task is blocked and no timer is pending, the test can never
finish: the runner reports a deadlock instead of hanging.
