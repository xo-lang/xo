# 0430: connection deadlines, WebSocket control frame handlers, the task's deadline

Status: Accepted (Xo lang 5, 2026-10-30; the lead agreed the API shape, and Xo Lang 3, which owns the HTTP and TLS internals, agreed the design the same day with the three implementation points under Consequences). Code after the seed cut from main on 2026-10-30.

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
   fn (c: WsConn) on_pong[fx: Effects](f: fn(Bytes) -> () uses fx) uses fx   // runs inside read() for each pong; default: nothing
   fn (c: WsConn) on_ping[fx: Effects](f: fn(Bytes) -> () uses fx) uses fx   // replaces the default answer (a pong with the same data)
   fn (c: WsConn) pong(data: Bytes = b"") -> () ! WsErr                       // the answer an on_ping sends
   ```

   (Amended at implementation, stage C: the hooks take an effect variable,
   since the keepalive below calls set_read_deadline, which uses clock;
   registering counts as performing them, as Router.handle. `pong` was
   added so an on_ping can still answer.)

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

Implementation points (Xo Lang 3, 2026-10-30):

1. **TLS keeps its own limit apart.** tlssrv has one `when`, which hconn
   sets before each read and zeroes after. The user's deadlines are new
   words in the TLS record, `user_rd` and `user_wr`, and a record read
   waits until `min(when, user_rd)`. TLS writes do not wait on any
   deadline today, so a write deadline needs a new wait. The client side
   (cabi_tlsclient) gets the same words.
2. **The clock.** For the real clock, a deadline becomes monotonic at set
   time (`mono_now + (t - clock.now())`) and pd_wait takes it. Under a
   FakeClock or a test world it fires through the fake timer, never
   pd_wait, or virtual time would never wake the read.
3. **A timeout leaves the connection whole.**
   - Handshakes honor the deadline (Go's tls.Conn does), through the
     existing handshake waits (CC_TIMEOUT), as `Timeout`. A failed
     handshake stays failed, as in Go.
   - A read timeout is temporary, as Go's is. tlssrv keeps the bytes of
     a partly read record across the timeout, so the connection is
     usable after a new deadline even mid-record.
   - http1's own limits (read_ns, head_ns, idle_ns) stay separate. hconn
     waits until the earlier of its limit and the user's, and keeps the
     outcomes apart: its own limit is the existing path (408, close), and
     the user's is `Timeout` to the handler.
   - The user's words live on the connection, and the server clears them
     when a request ends (net/http resets them per request).

Implementation notes (stage C, xo-lang-5, 2026-10-30):

- task.deadline: natively a context word C_TASK_DL (the timers' ns, 0
  none). with_timeout's scope saves it (S_OLD_DL), lowers it to
  min(old, now + d), and restores it at its close in every mode; spawn
  copies the spawner's into T_DEADLINE and run_claimed into the new
  task's context, so a task keeps the deadline in force where it was
  spawned, also after that body returned. T_SIZE 256 to 264 and S_SIZE
  464 to 472 cross one size class each: spawn-heavy --ab on linux/arm64
  decides, else the deadline shares T_RESULT until the task runs. Wasm:
  rt3.xo's $tdl global, saved per task. Go backend: Task.dl {clock, at}.
- WsConn: a client's deadlines are its Conn's or TlsConn's (live, as
  Go's). A server's read and write deadlines are words the hand-over
  wire passes to hconn.recv_until and hconn.send_until (written by
  xo-lang-5, reviewed by Xo Lang 3) at each wait, so one set while a
  wait is parked applies from the next wait. Waking the parked wait
  (pd_wake) is left until a program needs it: on_pong runs inside
  read(), so a keepalive's new deadline applies at the next wait
  anyway. A timeout, and any write error, sticks for that direction, as
  gorilla's: a frame may have been read or sent in part.
- TlsConn: a write that fails (a timeout included) fails every later
  write, as Go's tls.Conn (halfConn.setErrorLocked); reads stay
  temporary. Point 3's "usable after a new deadline" holds for reads
  only.
- on_ping and on_pong hooks usually capture their WsConn, which holds
  them: they are dropped when a read fails for good, at close, and when
  upgrade's f returns, or the pair would never be freed.

## Options for the owner

- **Stream gains the deadline setters (BREAKING).** The setters are on the concrete types only. Adding them to the `Stream` interface would let a function given either kind of connection set them, but it breaks every user type that implements Stream. It can be added later without undoing anything here.

## Alternatives

- Duration-only limits (`set_read_timeout(d)`, an idle limit per read). Rejected as the only form: they cannot express "this whole exchange by T". They are not Go's semantics, and they are not what a forwarded deadline is. The existing Duration limits stay.
- Using `with_timeout` for I/O limits (today's workaround). Rejected: it cancels, so the connection is lost.
- Reading the deadline as a Duration (`task.remaining(clock)`). Rejected: it is `deadline(clock)` minus `clock.now()`, and the Time form is Go's.
