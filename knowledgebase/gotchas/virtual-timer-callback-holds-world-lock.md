# Virtual clock timer callbacks run with the world locked

Status: Observed (2026-10-07, adding `FakeProc.signal_after`; approach
changed 2026-10-08 for two phase shutdown)

`afterFunc` and `World.timerFunc` on a test world call their callback with
`World.mu` held. Calling `cnode.cancel()`, `Chan.M_close`, or
`event.fire` from there deadlocks, because waking blocked tasks takes the
same lock (`World.wake`, `World.wakeCanceled`).

The first fix was a locked variant (`cnode.cancelLocked`). It does not
scale: two phase shutdown (core 7.5) closes a channel, fires an event, and
starts or stops other timers from a timer, and a channel lock taken under
the world lock inverts the order used by senders (channel lock, then
world lock), a deadlock the moment a program sends on or closes the
channel at the same instant.

Fix (`stopState.after` in std/rt/stop.go): the callback only counts one
more running task (`w.running++`, legal because it holds the lock) and
starts a goroutine that does the work with the ordinary, unlocked APIs and
ends with `w.taskEnded()`. The virtual clock cannot advance while the
goroutine runs, so the effect still happens at the timer's instant. Use
this pattern for any timer that does more than wake one task.
