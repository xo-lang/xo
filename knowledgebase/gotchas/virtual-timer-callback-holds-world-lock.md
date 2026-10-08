# Virtual clock timer callbacks run with the world locked

Status: Observed (2026-10-07, adding `FakeProc.signal_after`)

`afterFunc` on a test world calls its callback with `World.mu` held. Calling
`cnode.cancel()` from there deadlocks, because cancellation wakes blocked
tasks through `World.wakeCanceled`, which takes the same lock. Use
`cnode.cancelLocked()` (or only wake a task, as `task.with_timeout` does) when
the callback's `locked` argument is true.
