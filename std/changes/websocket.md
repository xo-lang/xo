# std/websocket changes

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
