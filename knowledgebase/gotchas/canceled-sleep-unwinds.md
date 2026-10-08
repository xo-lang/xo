# Polling `task.is_canceled()` around `clock.sleep` never sees cancellation

Status: Observed (2026-10-07, writing the `cancellation` golden test)

Symptom: `while !task.is_canceled() { os.clock.sleep(1ms) }` followed by
cleanup: the cleanup never ran.

Cause: `clock.sleep` cannot fail, so in a canceled task it unwinds the task
(core 7.2); the loop condition is never evaluated again. Only operations
that return a Result report cancellation as a value (`FsErr.Canceled`).

Fix: put cleanup in `defer` (shielded, core 3.5), or poll without a
blocking call. `Mutex.with` is a cancellation point only while it waits for
the lock (core 7.2 "Mutex.with while waiting"), so an uncontended `with` in
a canceled task still runs.
