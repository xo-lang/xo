# std/websocket changes

## 2026-10-21: fix: invalid UTF-8 in a text message fails at its fragment

- Kind: fix
- Commit: (filled in after the push)
- Found by: the Autobahn run (cases 6.4.1 to 6.4.4 were NON-STRICT)
- Go reference: github.com/gorilla/websocket Conn.NextReader (the utf8 check of each fragment, gorilla's utf8 reader)
- Test: internal/llvm/testdata/golden/websocket (utf8 fail fast, utf8 split ok, utf8 split bad, utf8 unfinished); Autobahn 6.4.x

On the native backends, a text message was checked for UTF-8 only when
it was complete, so a bad byte in an early fragment was answered with
close 1007 at the end of the message. Each fragment is now checked as
it arrives (a character split across fragments is carried to the next
one), so the connection fails at the bad fragment.

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
