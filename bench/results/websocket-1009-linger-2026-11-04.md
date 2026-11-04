# WebSocket: close 1009 lost to a reset, and the linger that keeps it (2026-11-04)

Xo Lang 2, the lead's item from the owner's v0.36.0 evaluation: a message
over max_message gets close 1009, then the server closes with the rest of
the message unread, and the client sometimes sees 1006 (a reset) instead.
The owner's tests/10-tls-ws counted 1006 on 8 of 60 (2 MiB) and 34 of 60
(8 MiB) with 0.36.0, 48 and 38 on the Go backend, 44 and 38 with
gorilla/websocket, 0 and 19 with coder/websocket (which waits for the
client's close).

## Method

A server program (std/websocket upgrade, a handler that reads until an
error, max_message 1 MiB) built by one xo with origin/main's std and with
the fix; the owner's client, Node 22's built-in WebSocket, opens one
connection at a time, sends one text message of 2 or 8 MiB, and counts the
close codes. The Mac (darwin/arm64), 300 connections per size and side.

| std | 2 MiB: 1006 | 2 MiB: 1009 | 8 MiB: 1006 | 8 MiB: 1009 |
|---|---|---|---|---|
| origin/main (6cc783baa's std) | 18 | 282 | 29 | 271 |
| with the linger | 0 | 300 | 0 | 300 |

Xo's own client never saw a 1006 with either std (480 connections on the
Mac and 480 on the linux/amd64 host, ws and wss): it writes the whole
message before it reads, and its read found the 1009 already buffered, so
a hammer with Xo's client cannot show the bug.

## Fix

std/native/websocket.xo: after a server sends close 1009 it lingers
(hconn.linger_close: stop sending, read and drop the peer's input until it
closes or 500 ms pass) for up to the rest of the message's declared length
plus 64 KiB (64 MiB at most; the 413 and 431 replies' 1 MiB cap would
reset an 8 MiB message partway).
