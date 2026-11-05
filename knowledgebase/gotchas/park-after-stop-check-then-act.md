# A connection that checks "stopping" before a slow step can park after the stop

Status: Observed (2026-11-05)

## Symptom

A native HTTP server never returns from `serve` after SIGTERM (or
`os.proc.request_stop()`) when a kept-alive HTTP/1.1 connection answered a
request in the 20 ms before the stop. A task dump (`kill -QUIT`, or
`xo debug -p PID -c tasks`) shows only the serving scope, blocked in
`serve`, and no connection tasks. The stability soak hung on 5 of 8
two-minute runs. A one-request repro hung on 10 of 10.

## Cause

`hconn.park_ready` read `stopping` first. It then lingered up to
LINGER_NS (20 ms) waiting for the next request, and only then set the
connection's idle mark. That ordering breaks the mark-then-read rule
that the stop relies on (hconn.xo, stopping_at), so a stop inside the
linger:

1. found the connection not idle, so `srv_stop` left it open;
2. woke the resumer, which saw stopping with nothing parked and ended;
3. the connection then parked, with no resumer to run it and no close.

The server's connection count never reached 0, so `srv_wait` (which has
no time limit after a stop request) waited until the grace period
canceled it, or for ever without one.

## Fix

`hserve.hc_park` checks `stopping` again under the server's resume lock,
after it has decided to park and before it links the connection into the
parked list. The resumer's own end check holds the same lock, so one of
two things happens:

- the resumer sees the parked connection and waits for it; or
- the park sees the stop, refuses, and the connection reads on, finds
  stopping, and closes.

Golden: internal/llvm/testdata/golden/http_stop_keepalive. Its grace
period of 2 s bounds a regression, which shows as missing output, not
a hang.

## The general rule

A flag that two parties each check before acting needs one of two
guards. Either each side writes its own mark and then reads the
other's, with nothing slow in between. Or both checks sit under one
lock. Any wait (a linger, a peek, a handshake) between "read the flag"
and "publish my state" reopens the window. Re-check after the wait,
under the lock the other side uses.
