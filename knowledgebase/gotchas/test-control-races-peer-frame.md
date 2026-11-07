# A fake server's test control races the peer's next frame

Status: Observed (2026-11-06)

## Symptom

llvm TestGoldenTasks/amqptest_broker failed fast (1.25 s, no hang) on
linux/arm64 in Lang 4's gate, at 750a25b71, after the hang fix of
0cb551516. Under load (8 native and 8 TSan copies at once pinned to 4
CPUs) it failed 3 times in the first 8 rounds: exit 0, every line as
want.txt except `begin ok` where it wants `begin failed`.

## Cause

The golden calls `b.disconnect_all()` and then `c.begin()`. std/amqptest's
`disconnect_all` (and `detach_links`) only records the request with a
generation number and wakes each connection's loop. That loop (`run_conn`)
selects between the peer's next frame and the wake. When the client's
Begin is already read when the wake comes, the select can take the frame
first, and the broker answers the Begin before it applies the close the
test asked for, although `disconnect_all` had returned before the Begin
was sent.

## Fix

A fake server applies the test's pending controls (forced detaches,
closes) before it handles each frame, not only when woken: any frame the
peer sent after the control call returned then meets the state the test
asked for. In std/native/amqptest.xo's `run_conn`, `control` runs first
in the frame branch (one read of the broker state under its lock a
frame). The same applies to any test double with
an asynchronous "do this to every connection" call: either apply it on
the next event of each connection, or make the call wait until every
connection has applied it.
