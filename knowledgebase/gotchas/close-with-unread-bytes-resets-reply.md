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
