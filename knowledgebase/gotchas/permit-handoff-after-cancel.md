# A finishing task hands its permit to a canceled spawn loop

Status: Observed (2026-10-08, `task.map_concurrent_until`, decision 0043)

Symptom: with `limit` 1 and a deadline that cancels the running item,
`map_concurrent_until` sometimes started one more item after the deadline
(a counter in `f` read 3, expected 2), depending on which task the test
world woke first.

Cause: canceling the scope unwinds the running item; its deferred
`sem.release()` hands the permit to the first waiter, which can be the
spawning loop blocked in `sem.acquire`. The hand off claims the waiter
before the loop's own cancellation does, so `acquire` reports success and
the loop spawns the next item (`Spawn` does not check cancellation) before
it reaches a cancellation point.

Fix: `mapConcurrent` (std/rt/sync.go) checks `body.Canceled()` after
`acquire` returns and, if set, releases the permit and unwinds. Any loop
that acquires a lock and then starts work must recheck cancellation after
a successful acquire.
