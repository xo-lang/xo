# Closing with unread request bytes can reset the reply (Linux)

Status: Observed (2026-10-30)

## Symptom

golden http_server_limits prints "big header: error" instead of
"431 Request Header Fields Too Large" on the linux/arm64 host (the Mac
passes): the client sends a 100 KB header, the server answers 431 at its
limit and closes, and the client's read fails with a reset.

## Cause

close(2) on a socket whose receive buffer holds unread bytes sends a RST
on Linux; the peer's kernel then drops the reply it had received but the
program had not read. macOS delivers the reply first. The server stopped
reading at the limit, so the rest of the request was unread.

## Fix

After an early error reply (431, 413), shut down the sending side and read
and discard until the peer closes or a bound passes, then close:
hconn.linger_close (500 ms, 1 MiB), net/http's closeWriteAndWait. Any new
early-reply-and-close path in the server does the same.

## 2026-11-04: macOS too, with another client; and the cap

The WebSocket server's close 1009 (a message over max_message) hit the
same reset on the Mac: Node's WebSocket client got 1006 on 47 of 600
oversized messages (bench/results/websocket-1009-linger-2026-11-04.md),
so "macOS delivers the reply first" depends on the client. Xo's own
client never showed it (it reads only after its write ends), so a test
that reproduces this needs a client that reads while it writes. The
linger's read cap matters too: linger_close's 1 MiB default reset an
8 MiB message partway; the WebSocket path passes the message's remaining
length (linger_close's `max`).

