# Go's select picks at random among ready cases

Status: Anticipated (2026-10-08, designing the deterministic scheduler)

Symptom (would be): the same seed gives different results from run to
run, but only when a wake up and a cancellation land at the same instant
(a sleep whose timer fired in a task that was canceled, an event that
fired while its waiter was canceled).

Cause: the runtime waits with Go `select` on the wake channel and the
task's cancellation channel. When both are ready, Go chooses uniformly at
random, so the outcome (return normally or unwind) is not a function of
the seed.

Fix (std/rt): decide from state, not from the select. `Task.wait` and
`World.Sleep` first try the wake channel without blocking; `event.wait`
rechecks `fired` under the event lock after a cancellation;
`awaitDeadline` reads `tk.Canceled()` after the select. Under the
scheduler the state is fixed by the time the thread gets its turn, so
these checks are deterministic.
