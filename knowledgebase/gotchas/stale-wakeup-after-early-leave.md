# A late wake up after a task left its wait breaks the scheduler

Status: Anticipated (2026-10-08, designing the deterministic scheduler)

Symptom (would be): under `xo run --seed` a task's next blocking call
returns at once, the task keeps the turn while its Go level wait never
completes, and the program hangs.

Cause: a waker that is not a task (a real timer goroutine, the signal
handler) can call `wakeTask` after the task already left the wait for
another reason. The scheduler records a wake up of a running thread as
pending (it registered as a waiter and is about to block), so a late one
makes the next, unrelated block a no op.

Fix (std/rt): every wait ends only after its possible wakers either
cannot fire any more or have finished: `waiter.claim` (channels, locks,
select, real `clock.sleep`), the `claimed` flag of `awaitDeadline` (the
task claims it when it leaves first, else waits for the timer's close),
and `event.fire` waking its waiters under the event lock. `unblock` then
clears any pending wake up.
