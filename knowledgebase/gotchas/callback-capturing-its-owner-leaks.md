# A callback stored in the value it captures is never freed

Status: Observed (2026-10-30)

## Symptom

Golden websocket_deadline passed its output on every backend, but arm64's
TestGoldenRun failed with `37 allocations still live at exit (of 638)`.

## Cause

`c.on_pong(fn(_) { c.set_read_deadline(...) })` stores a closure in the
WsConn's hooks, and the closure captures `c`. Native memory is reference
counted (no cycle collector), so the WsConn holds the closure and the
closure holds the WsConn: neither count reaches zero. gorilla's handlers
capture their conn the same way, so the pattern is the expected use, not
a mistake to warn about.

## Fix

The owner drops what it stores when nothing can call it again:
std/native/websocket.xo drops the hooks when a read fails for good (the
error sticks, so no read runs a hook again), at `close`, and when
upgrade's `f` returns. Any std API that stores a user callback in the
object the callback naturally captures needs such a release point; the
arm64 golden leak check finds a missing one.
