# A raw test client that drops the bytes read past a handshake hangs on Linux

Status: Observed (2026-11-17, task #235)

## Symptom

std/websocket's test "a compressed frame has RSV1 and inflates; the RFC
7692 Hello frame reads" passed on the Mac and failed on linux/arm64
(offload gate, stdfast) with `XO-F001: deadlock: every task is blocked
and no timer is pending`. It hung the same way on the Mac once in a
later run.

## Cause

The test's raw client called `raw_open(...).0`, keeping the connection
and dropping the second value: the bytes read past the 101's
`\r\n\r\n`. The server writes its first frame right after the 101, so
when both arrive in one read (most of the time on Linux loopback) the
frame was thrown away and the next `raw_fill` waited for bytes that had
already come.

## Fix

Keep what the handshake read past its head and start the frame read
from it: `let (c, held) = raw_open(...)`, `raw_fill(c, held, n)`. Any
hand written HTTP or WebSocket client in a test has the same trap: the
read that ends the head may carry the body or the first frame. A test
whose raw client only writes after the handshake may drop it.
