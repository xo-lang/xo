# std/websocket changes

## 2026-10-30: feature: deadlines, on_ping, on_pong, pong

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 std survey (bench/survey-2026-10-30, gap 5,
  deadlines), decision 0430
- Go reference: gorilla/websocket v1.5 Conn.SetReadDeadline,
  SetWriteDeadline, SetPingHandler, SetPongHandler, and its writeFatal
- Test: internal/llvm/testdata/golden/websocket_deadline (native and Go
  backends)

`WsConn` has `set_deadline`, `set_read_deadline` and
`set_write_deadline(clock, t)`: a read or write past the deadline fails
with `Net(Timeout)`, and the error sticks for that direction. `on_ping`
and `on_pong` run a function for each ping or pong that `read` reads
(`on_ping` in place of the automatic pong, sent with the new `pong`), so a
keepalive can move the read deadline. A write error now sticks too, as in
gorilla: a frame may have gone out in part.

## 2026-10-30: fix: a stop waits for an upgraded connection on the Go backend

- Kind: fix
- Commit: this commit
- Found by: the backend gap survey of 2026-10-30 (bench/survey-2026-10-30/chathub: std 4.4 did not say whether an upgraded connection is in flight), then reading both backends
- Go reference: net/http Server.Shutdown (does not track hijacked connections); gorilla/websocket's examples close with 1001 on shutdown
- Test: internal/llvm/testdata/golden/websocket_stop (arm64, LLVM, Go parity): it printed `handler done: false` on the Go backend before the fix

A connection handed over by a 101 (`upgrade`, a proxied upgrade) is a
request in flight until its function returns (decision 0429). The native
backends already waited for it on a stop (hserve's srv_wait waits for
every connection that is not idle). The Go backend's `serve` stopped with
net/http's Shutdown, which forgets a hijacked connection, so `serve`
returned while the handler still ran. std/rt now counts the hand-over
functions and waits for them after Shutdown, within the same grace
period, then cancels them as the native backends do. Natively a
handed-over connection's reads (the WebSocket's and a proxied
upgrade's) no longer mark it idle, so a stop no longer closes one
blocked in `read()`: the handler decides, as on the Go backend (the
spec shows a background task that closes with 1001 at a stop). Timing: no hot path
changed (one WaitGroup Add and Done per upgrade); none measured.

## 2026-10-27: perf: masking runs a word at a time

- Kind: perf
- Commit: this commit
- Found by: bench/perf/websocket big at 1.55x gorilla, 2.8x its instructions (about 12 more a payload byte: the byte at a time mask and unmask loops)
- Go reference: github.com/gorilla/websocket mask.go maskBytes (a word at a time once aligned) (v1.5.3)
- Test: internal/llvm/testdata/golden/websocket and websocket_tls (arm64, LLVM); twincheck bench/perf/websocket; a differential against gorilla in both directions (an Xo client of gorilla's echo server, a gorilla client of an Xo echo server; binary and text messages of 0 to 200,003 bytes, 2, 4 and 10 byte length headers, text read in pieces at any key offset), 44 messages each, equal on arm64 and LLVM

On the native backends a client masked its payload, and either side
unmasked one, a byte at a time through a BytesBuilder; both now XOR
eight bytes at a time with the key repeated (rotated to where a piece
starts) and write the frame, header, key and payload, into one buffer.
Quiet --ab, 15 rounds, the parent's std as A: bench/perf/websocket big
wall 0.507 (0.503 to 0.509), instructions 0.223.

## 2026-10-21: fix: invalid UTF-8 in a text message fails at its fragment

- Kind: fix
- Commit: 1a6117d1b (per fragment), 5edde9467 (within one frame)
- Found by: the Autobahn run (cases 6.4.1 to 6.4.4 were NON-STRICT)
- Go reference: github.com/gorilla/websocket Conn.NextReader (the utf8 check of each fragment, gorilla's utf8 reader)
- Test: internal/llvm/testdata/golden/websocket (utf8 fail fast, utf8 split ok, utf8 split bad, utf8 unfinished); Autobahn 6.4.x

On the native backends, a text message was checked for UTF-8 only when
it was complete, so a bad byte in an early fragment was answered with
close 1007 at the end of the message. Each fragment is now checked as
it arrives (a character split across fragments is carried to the next
one), so the connection fails at the bad fragment. A text frame is also
checked in the pieces of it that arrive, so a bad sequence in the middle
of one frame fails before the rest of the frame is read.

## 2026-10-21: fix: a read after a close keeps the close error

- Kind: fix
- Commit: b5d9c4c66
- Found by: a flaky golden under load (TestGoldenRun/websocket, Xo lang 4's gate)
- Go reference: github.com/gorilla/websocket Conn.NextReader (readErr sticks)
- Test: internal/llvm/testdata/golden/websocket (bye_late: the close frame in a later segment than the message; failed 3 of 3 before, and 12 of 450 runs of the old golden under CPU load)

On the native backends, a read after the connection's close frame read
the connection again (an `unexpected EOF` close 1006, or a reset)
instead of giving the same close error, when the close frame had
arrived after the message before it; a failed send also forgot that a
close frame was sent.

## 2026-10-21: feature: wss:// in websocket.dial

- Kind: feature
- Commit: 9515923f
- Found by: decision 0313 stage 2
- Go reference: github.com/gorilla/websocket v1.5 Dialer (TLSClientConfig, the wss scheme's port 443)
- Test: internal/llvm/testdata/golden/websocket_tls (echo over TLS with the test CA as roots, the system roots refused, roots that are not PEM; native identical to the Go backend)

`websocket.dial(net, "wss://host/path", roots: pem)` connects over TLS
through std/tls's client and verifies the host against `roots`, or the
system's roots when none are given.

## 2026-10-21: feature: std/websocket

- Kind: feature
- Commit: 933b1b96
- Found by: owner's request (2026-10-21), decision 0313
- Go reference: github.com/gorilla/websocket v1.5 (conn.go, server.go, client.go)
- Test: internal/llvm/testdata/golden/websocket (a server and a client: echo, ping, close codes, subprotocol, a message over max_message, refused handshakes, 13 raw frames that break the protocol; native identical to the Go backend)

`websocket.upgrade(req, fn(c) { ... })` in a handler accepts a WebSocket
and `websocket.dial(net, "ws://...")` opens one; `read`, `write`,
`ping`, and `close` follow gorilla/websocket, its handshake checks, frame
rules, close codes, and error texts.
