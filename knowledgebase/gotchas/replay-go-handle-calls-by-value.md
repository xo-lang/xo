# Answering a Go handle method from the trace skips its side effects

Status: Anticipated (2026-10-09)

Symptom (expected): a replay of a program that calls methods on a Go
handle diverges or hangs although each method's result is a plain value.
`sync.WaitGroup.wait` returns unit; answered from the trace, main would
run ahead of the Go callbacks it should wait for, and the scheduler
stream would no longer match.

Cause: the handle's Go object lives in the replay (the call that made it
ran again), but a method answered by value never touches it, so later
Go behavior (other goroutines, later methods) sees a different state.

Fix: decision 0093 stage 1 records by value only calls without a handle
anywhere (receiver, arguments, results) and without callbacks; the rest
run again and are reported (`internal/gobind` `replayMode`). Do not widen
the by value rule to methods of handles without a test that replays a
program whose Go state matters (`TestReplayGoCallback`).
