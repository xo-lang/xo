# 0313: std/websocket

Status: Accepted (owner, 2026-10-21, within decision 0276: std follows
Go's ecosystem where Go's standard library has nothing; Xo Lang 3; the
design is Xo Lang Lib 1's draft, ~/xo-lead/notes/lib1/websocket.md)

## Context

Go's standard library has no WebSocket; Xo programs need one for live
updates, chat, dashboards, log streams, and clients of WebSocket APIs.
Of the Go packages, gorilla/websocket is the most used and passes the
Autobahn conformance suite; golang.org/x/net/websocket is frozen (no
close codes, no ping handling).

## Decision

A module `std/websocket` with gorilla/websocket's behavior (RFC 6455),
both sides:

```xo
struct Message { kind: Kind, data: Bytes }
enum Kind { Text, Binary }
enum WsErr {
  Closed(code: Int, reason: Str)   // the peer's close frame
  Protocol(msg: Str)               // a bad frame: close 1002 sent (gorilla's texts)
  TooBig(limit: Int)               // a message over max_message: close 1009
  Net(err: NetErr)
  Handshake(status: Int, msg: Str) // the client's upgrade was refused
}
struct WsConn {}
fn (c: WsConn) read() -> Message ! WsErr
fn (c: WsConn) write(m: Message) -> () ! WsErr
fn (c: WsConn) write_text(s: Str) -> () ! WsErr
fn (c: WsConn) ping(data: Bytes = b"") -> () ! WsErr
fn (c: WsConn) close(code: Int = 1000, reason: Str = "") -> () ! WsErr
fn (c: WsConn) subprotocol() -> Str

fn upgrade[fx: Effects](r: http.Request, f: fn(WsConn) -> () uses fx, protocols: List[Str] = [], origins: List[Str] = [], max_message: Int = 1048576) -> http.Response uses fx
fn dial(net: Net, url: Str, headers: Map[Str, Str] = {}, protocols: List[Str] = [], timeout: Duration = 30s, max_message: Int = 1048576) -> WsConn ! WsErr uses net.dial
```

- **Server.** `upgrade` checks the request as gorilla's Upgrader (GET,
  `Connection: upgrade`, `Upgrade: websocket`, version 13, a 16 byte
  base64 key; the origin against `Host` unless `origins` allows it) and
  answers a failed check with gorilla's status and text. Otherwise it
  returns a response with status 101 whose streamed body is the hand-off:
  the server sends nothing itself, takes the connection out of its
  request loop (net/http's Hijacker; the native server's connection with
  the input it buffered), writes gorilla's 101, runs `f` with the
  connection, and closes it when `f` returns. HTTP/1.1 only (an HTTP/2
  request has no `Connection: upgrade`, so it fails the check).
- **Frames.** RFC 6455 as gorilla applies it: client frames masked,
  server frames not; control frames at most 125 bytes, unfragmented;
  reserved bits and unknown opcodes are protocol errors; text messages
  valid UTF-8 (1007); a message over `max_message` is close 1009; `read`
  answers a ping with a pong, skips pongs, and answers a close with a
  close and `Closed(code, reason)`. A write is one frame, under a lock,
  so one task may read while another writes.
- **Client.** `dial` takes `ws://` (and, in stage 2, `wss://` through
  std/tls), sends gorilla's handshake (a random key, the protocols,
  the headers), and checks the 101 and `Sec-WebSocket-Accept`.
- **Backends.** Go: std/rt/websocket.go by hand (no third-party module
  in std/rt), over the hijacked net.Conn and the program's Conn. Native:
  std/native/websocket.xo over the server's connection and Conn.

Stages: (1) the module, ws:// both sides, both backends, a golden of a
server and a client in one program; (2) wss:// in dial; (3) the
Autobahn suite in Docker (sections 1 to 7 and 9) with its report under
bench/results, and interop with a Go gorilla program.

## Consequences

- A response with status 101 and a streamed body is a connection
  hand-off on both backends (std.md 4.3): its function owns the raw
  connection. Only std/websocket makes one.
- The native hand-off: native std modules cannot name std/http's
  internals, so `upgrade` returns a 101 whose body is
  `hserve.HANDOFF` and the stream registry's id of a
  `fn(rthttp.HConn) -> ()`; std/native/http.xo respond takes it and runs
  it with the connection (http1.take_rest gives the input already read).
  The Go backend's 101 stream gets net/http's ResponseWriter to hijack.
- Stage 1 landed 2026-10-21 (golden websocket: native identical to the
  Go backend); stage 2 (wss:// in dial, `roots`) the same day (golden
  websocket_tls).
- Not now: permessage-deflate (waits for streaming flate), a message
  written in pieces (`NextWriter`), WebSockets over HTTP/2 (RFC 8441).

## Alternatives

- x/net/websocket's API: frozen, no close codes.
- coder/websocket's context-first API: Xo's cancellation is structured
  already.
- A Router method (`r.websocket("/ws", f)`): `upgrade` inside a handler
  lets it check auth and answer with an error first, as gorilla's.
