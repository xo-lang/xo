# 0430: connection deadlines, WebSocket control frame handlers, the task's deadline

Status: Proposed (Xo lang 5, 2026-10-30). The lead agreed the API shape the same day. It stays Proposed until Xo Lang 3, which owns the HTTP and TLS internals, agrees on the design. Code after the seed cut from main on 2026-10-30.

## Context

The backend gap survey of 2026-10-30 ranked gap 5, deadlines, as the first item to do. Three services hit it: queueworker (a line protocol over Conn and TlsConn), chathub (a WebSocket heartbeat) and grpcsvc (forwarding the caller's remaining time as `grpc-timeout`).

What exists is one model:

- Decision 0007: deadlines, cancellation, request ids and spans flow through the scope tree.
- Decision 0015: `task.with_timeout(clock, d, f)` runs f in a child scope whose deadline cancels the scope. Every blocking call in it ends with its `Canceled` variant, and the caller sees `Err(TimedOut)`. Cancellation is sticky. Natively the scope holds a timer (llrt/tasks.xo `S_DEADLINE`), and nothing can read it.
- Decision 0071: Conn reads, writes and accepts are cancellation points.
- Per operation limits as Durations: http Server.with_read_timeout, with_read_header_timeout and with_idle_timeout, Request.with_timeout and with_idle_timeout (0404), Net.dial_timeout (0406).

The gap is that a scope deadline cancels, and a canceled task stays canceled, so the connection cannot be used after it. "No frame in 30 s: send a ping and keep the connection" cannot be written. Neither can a heartbeat that pushes the read deadline forward on each pong, or a client that forwards the time left.

## Decision

Two layers of the one model:

1. **The scope deadline stays the only thing that cancels, and it becomes readable** (Go's `ctx.Deadline`):

   ```
   pub fn deadline(clock: Clock) -> Time? uses clock
       // in std/task: the nearest deadline of the current scope and the scopes around it
       // (with_timeout; with_budget's cpu ceiling is not one), as a Time on clock;
       // None when no enclosing scope has one
   ```

2. **A connection deadline never cancels.** It fails one operation with `NetErr.Timeout` and leaves the connection usable (Go's `net.Conn` SetDeadline, SetReadDeadline and SetWriteDeadline). These go on Conn, ServerConn, TlsConn and WsConn:

   ```
   fn (c: Conn) set_deadline(clock: Clock, t: Time?) uses clock        // read and write
   fn (c: Conn) set_read_deadline(clock: Clock, t: Time?) uses clock
   fn (c: Conn) set_write_deadline(clock: Clock, t: Time?) uses clock
   ```

   The semantics are Go's:
   - the deadline is absolute, and `None` clears it;
   - a time already past fails the waiting operation and the following ones at once;
   - it holds for every later operation until it is changed;
   - an operation past it is `Timeout` (`i/o timeout`), and a write may have written part of its data;
   - a later deadline makes the connection usable again.

   TlsConn passes the deadline to its connection. A timed out WsConn read is `WsErr.Net(Timeout)` and, as in gorilla, it sticks (std 35: a read error sticks), since the frame state is lost.

   An operation waits until the earlier of its connection's deadline and its scope's. If the connection's comes first, the result is `Timeout`. If the scope's comes first, the result is `Canceled`, as today.

3. **WebSocket control frame handlers** (gorilla's SetPongHandler and SetPingHandler):

   ```
   fn (c: WsConn) on_pong(f: fn(Bytes) -> ())    // runs inside read() for each pong; default: nothing
   fn (c: WsConn) on_ping(f: fn(Bytes) -> ())    // replaces the default answer (a pong with the same data)
   ```

   ```
   c.set_read_deadline(clock, Some(clock.now() + 60s))
   c.on_pong(fn(_) { c.set_read_deadline(clock, Some(clock.now() + 60s)) })
   ```

**The clock argument** (lead, 2026-10-30): Xo passes the Clock explicitly everywhere else (capabilities, FakeClock in test worlds, `with_timeout(clock, d, f)`). A deadline read against a hidden `time.Now`, as Go does, would be the odd one out. It would also escape a test world's virtual time.

Native: each connection's poll descriptor gets a read and a write deadline word, and `pd_wait` (which already takes an absolute deadline) waits until the earliest of them, the request's own limits and the scope's. A connection handed over by a 101 starts with no deadline: net/http leaves the server's deadlines on a hijacked connection for the caller to clear, and std/rt's upgrade clears them. HTTP/2 streams are not connections and get none.

## Consequences

- Spec: std 3.2 (Conn, ServerConn), std/tls (TlsConn), std 35 (WsConn), std task (deadline).
- Goldens on every backend with Go parity:
  - a read past its deadline is Timeout, and the next read after a new deadline succeeds;
  - a past deadline wakes a waiting read;
  - a write deadline against a peer that does not read;
  - the TlsConn and WsConn forms;
  - on_pong pushes the read deadline forward;
  - task.deadline in nested with_timeout scopes, where the nearer one wins.
- Timing row: a read loop with a deadline set, against Go's SetReadDeadline.
- std/changes entries for net, tls, websocket and task.
- Survey gaps 1 and 2 (HTTP/2 trailers and full duplex, mTLS) follow, as one design with Xo Lang 3.

## Options for the owner

- **Stream gains the deadline setters (BREAKING).** The setters are on the concrete types only. Adding them to the `Stream` interface would let a function given either kind of connection set them, but it breaks every user type that implements Stream. It can be added later without undoing anything here.

## Alternatives

- Duration-only limits (`set_read_timeout(d)`, an idle limit per read). Rejected as the only form: they cannot express "this whole exchange by T". They are not Go's semantics, and they are not what a forwarded deadline is. The existing Duration limits stay.
- Using `with_timeout` for I/O limits (today's workaround). Rejected: it cancels, so the connection is lost.
- Reading the deadline as a Duration (`task.remaining(clock)`). Rejected: it is `deadline(clock)` minus `clock.now()`, and the Time form is Go's.
