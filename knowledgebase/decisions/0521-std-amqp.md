# 0521: std/amqp, a port of github.com/Azure/go-amqp (AMQP 1.0)

Status: Accepted (owner, 2026-11-05), with one condition on stage 4: the real namespace smoke test runs only on the owner's machine, after the owner confirms a test plan through the lead, and never in CI or by an agent. (Proposed by Xo Lang 3, 2026-11-05, for the Azure SDK.)

## Context

The Azure SDK for Xo (xolang/azure, decision 0006-xo-gaps, accepted by the
owner 2026-11-05) needs AMQP 1.0 for Service Bus and Event Hubs: a TLS
connection (port 5671), SASL (PLAIN, ANONYMOUS, EXTERNAL), sessions, sender
and receiver links, flow control, settlement modes, dispositions, the AMQP
type system for message bodies and properties, and claims based security
(CBS: a `$cbs` link carrying SAS or Entra tokens). Xo has no AMQP (checked
2026-11-05: no `std/amqp`, no spec section). Go's library is
`github.com/Azure/go-amqp` (MIT, Copyright 2017 Kale Blankenship and
Microsoft; v1.7.0 is the one Azure's `azservicebus` and `azeventhubs` use).

Size of the Go library (v1.7.0, non-test lines): `internal/encoding` 4,514
(the type system: described types, performatives, message sections),
`internal/frames` 1,699 (frame headers and performatives), `conn.go` 1,208,
`receiver.go` 1,031, `session.go` 854, `sender.go` 631, `message.go` 515,
`link.go` 416, `sasl.go` 259, and about 1,100 of queue, bitmap, buffer and
fakes; 165 test functions. About 11,000 lines of Go, a port of perhaps
8,000 lines of Xo.

What the port rests on in Xo: `std/tls` (`tls.client` over `os.net.dial`,
with `set_deadline`), tasks and channels in place of goroutines (a
connection's mux loop is a background task, `Chan` the hand off), `Duration`
and the clock as arguments, and the effect system (`uses net.dial`).
Everything else (UUID, Time, Map, List, Bytes builders) is in the prelude or
std.

## Decision

Add `std/amqp`, a port of go-amqp's behavior, in four stages, each with a
corpus or test against the Go library, `std/changes/amqp.md`, `spec/std.md`
and a timing row against Go where it applies:

1. **The type system and frames** (`std/native/llrt/amqpenc.xo` and frame
   codecs): every AMQP 1.0 primitive, described types, lists, maps, arrays,
   the performatives (open, begin, attach, flow, transfer, disposition,
   detach, end, close), the SASL frames, and the message sections
   (header, properties, application properties, body data, sequence and
   value, footer, delivery and message annotations). Decode and encode are
   compared with go-amqp's `internal/encoding` and `internal/frames` on its
   own test vectors plus a generated corpus (the oracle is a Go program
   in `scripts/stdcorpus/amqporacle`, its own module as yamloracle is).
2. **Connection, SASL and sessions**: `amqp.dial(net, addr, opts)` returns a
   `Conn` after the SASL exchange and the open frames; sessions with
   incoming and outgoing windows and handle tables; idle timeout and
   heartbeats; errors as go-amqp's (`ConnError`, `SessionError`,
   `LinkError`, `Error` conditions). Tested against go-amqp's own tests that
   use its fake connection (ported to a Xo fake net) and an in process
   Xo peer.
3. **Links**: `Sender.send`, `Receiver.receive`, credit and drain,
   settlement modes (first, second, settled, unsettled), `accept`,
   `reject`, `release`, `modify`, dispositions, resume of unsettled
   deliveries, message size limits, link filters and properties (the
   Service Bus and Event Hubs ones are options, not special cases).
4. **CBS and the Azure shapes**: `amqp.cbs_put_token(session, audience,
   token, token_type)` as a helper on a `$cbs` sender and receiver (the
   request and response shape Azure uses), plus a smoke test against a real
   namespace run by the owner (a gated, opt in test: it needs a
   connection string, so CI never runs it).

### API shape (proposal, for the owner to confirm)

Xo's conventions, not go-amqp's: results are `T ! AmqpErr` rather than
`(T, error)`, an `enum AmqpErr` with the three go-amqp error families,
options are structs with defaults (`ConnOptions`, `SessionOptions`,
`SenderOptions`, `ReceiverOptions`), time limits are `Duration` arguments
instead of `context.Context` (a task's cancellation stops a blocked call,
as everywhere in std), and `Message` is a struct whose `body` is an enum of
data, sequence and value. TLS is `tls.client` composed by `dial` when the
address scheme is `amqps`, with `ConnOptions.tls_roots`, `.tls_verify` and
`.tls_cert`.

```xo
struct Message { header: Header?, properties: Properties?, app_properties: Map[Str, Value],
                 annotations: Map[Value, Value], body: Body, footer: Map[Value, Value], ... }
fn dial(net: Net, addr: Str, opts: ConnOptions = ConnOptions{}) -> Conn ! AmqpErr uses net.dial
fn (c: Conn) new_session(opts: SessionOptions = SessionOptions{}) -> Session ! AmqpErr
fn (s: Session) new_sender(target: Str, opts: SenderOptions = SenderOptions{}) -> Sender ! AmqpErr
fn (s: Session) new_receiver(source: Str, opts: ReceiverOptions = ReceiverOptions{}) -> Receiver ! AmqpErr
fn (x: Sender) send(m: Message, timeout: Duration = 0s) -> () ! AmqpErr
fn (r: Receiver) receive(timeout: Duration = 0s) -> Message ! AmqpErr
fn (r: Receiver) accept(m: Message) -> () ! AmqpErr
```

`Value` is the AMQP dynamic type (null, boolean, the integer widths, floats,
decimal, char, timestamp, uuid, binary, string, symbol, list, map, array,
described), as `json.Value` is JSON's.

### Backends

The port is Xo, compiled to Go for the Go backend through `std/go_compiled.txt`
(decision 0312, as `std/yaml` and `std/jwt`), so one implementation serves
every backend and no Go code is vendored. The Go backend's reference for
tests is go-amqp itself (the oracle), not a runtime dependency.

## Consequences

- Service Bus and Event Hubs modules of the Azure SDK can start at stage 2
  (connection and sessions) with a fake broker, and ship at stage 3.
- About 8,000 lines of new std and a second corpus oracle module; the
  effect is bounded to `std/amqp`, with no compiler or runtime change. A
  stage may find a runtime gap (a blocking read with a deadline on a
  `TlsConn`, task cancellation inside a mux loop); that is reported as a
  separate item, as with the Node type check of std/yaml.
- Each stage lands on its own, so a partial port is usable: stage 2 alone
  speaks SASL and opens sessions.

## Alternatives

- **`use go` bindings to go-amqp**: works only on the Go backend, not on the
  native backends the SDK targets (the Azure ADR's own rejected option).
- **A smaller AMQP 1.0 client written from the specification**: less code,
  but it would not share go-amqp's behavior in the places Azure depends on
  (credit and drain, link recovery, the exact error conditions), and it has
  no oracle to test against.
- **Vendoring go-amqp's Go source for the Go backend and writing Xo
  separately for native**: two implementations to keep in step; rejected
  for the same reason std/yaml is one Xo implementation.

## Open questions for the owner

1. Is the API shape above (Xo style, `Duration` timeouts, `Value` and `Body`
   enums) acceptable, or should `std/amqp` mirror go-amqp's names more
   closely (`NewSession`, `Receiver.ReceiveMessage`)?
2. Order: stage 1 and 2 first and the Azure Service Bus work in parallel, or
   all four stages before any SDK work?
3. Settled by the owner: the real namespace smoke test needs a connection
   string, runs only on the owner's machine after a confirmed plan, never in CI.

## As built (2026-11-05, std owner)

- Stage 2 is split: 2a (frames and performatives, `encode_frame`/`decode_frame`, 0708e6e29), 2b-i (the
  synchronous connection: `dial`, SASL PLAIN and ANONYMOUS, Open, `begin`, `end_session`, `close`, driven
  by one task) and 2b-ii (a reader task and a session mux so several links share a connection).
  Messages (header, properties, annotations, body, footer) land as 2c, before links.
- Names: the connection type is `amqp.Connection` (not `Conn`, which is the prelude's TCP `Conn`).
  Fields of the wire structs that hold an unsigned value with a default are `Int` and range checked on
  encode, because a `UInt32` field with a literal default needs an Int to UInt32 conversion that the
  native backends lack in a struct literal (XO1201; reported to Lang 1).
- A connection that outlives its dial call and runs a reader task needs a `task.Group`; 2b-ii takes it as a
  parameter (`dial(net, tasks, addr, opts)`), as the structured concurrency rule (spec 7.1) requires.
- Stage 2b-ii as built (2026-11-05): no separate reader task. A read loop that holds the connection lock
  while it waits cannot be shared with a heartbeat task, so the deadlines live in the read loop itself
  (`Conn.set_read_deadline`, decision 0430): `timeout_ms`, idle detection and empty-frame heartbeats are
  done by whichever call is waiting, and `Connection.keepalive(task.Group)` starts the same loop in a task
  (5 ms polls) for idle periods. `dial` keeps its signature; the clock comes in `ConnOptions.clock`, and every
  call is `uses (net.dial, clock)`. A truly concurrent reader (waiting without the lock) needs a lock free
  transport read and is not built. The timeout tests run with `xo test --real` (the test clock is virtual).
- Stages 3 and 4 as built (2026-11-05): links and CBS follow the synchronous model of 2b-i. `Sender` and `Receiver`
  are handles over the connection (all state lives in `ConnState`, one Mutex), and every blocking call pumps the shared
  read loop, so a background reader task is not needed for correctness, only for concurrent use of the connection from
  several tasks (2b-ii). The API differs from the proposal where Xo requires it: `Connection.new_sender(session, ...)`
  instead of `Session.new_sender`, `send(m, settled)` returning the peer's outcome, and no timeout arguments yet.
- Reader task as built (2026-11-05, lead's request to close the Service Bus concurrency gap): `keepalive(g)`
  now starts a reader task (go-amqp's connMux in shape) that owns the transport read: it reads without the
  lock, applies each frame under it (`apply_frame`, split from `pump`), wakes waiters, sends heartbeats at
  half the peer's idle timeout and fails the connection (`broken`) when the peer is silent for
  `idle_timeout_ms`. Blocked calls are a loop of idempotent steps run under the lock (`await_cond`); a step
  that is not done registers a one slot channel in `ConnState.waiters` and the call waits on it outside the
  lock with its own deadline (`select ... after`), so a waiting `receive` does not hold up a `send`.
  Without `keepalive` the same steps pump inline as before. A send takes its session's send turn
  (`SessState.sending`) for the transfer ids and frame order. `Begin` and `End` frames are queued in
  `pending` while the reader runs and `begin`/`end_session` take theirs; raw `recv` is refused then.
  `begin` picks the channel and reserves it in `ConnState.channels` in one critical section (released on
  failure), so concurrent begins get distinct channels (test: 8 concurrent begins). Test: "two receivers and a sender on
  one connection do not block each other" (100 sends and 100 echoed receives with a third receiver waiting:
  about 6 ms on loopback; with one lock the sends would wait for the receiver that waits).
- Test broker as built (2026-11-06, lead: no emulator for now, the owner decided to test Service Bus against an
  in-process fake): `std/amqptest` (spec/std.md 64.6) is a Service Bus shaped AMQP broker for tests: CBS,
  peek-lock queues with lock tokens and expiry, settle modes, dispositions mapped to complete, abandon, defer and
  dead letter, `$management` (renew-lock, peek, schedule, cancel, receive-by-sequence-number, update-disposition,
  session lock and state), topics with subscriptions, session queues, and injected faults and forced detaches or
  closes. It is a std module so a client library in another repository can use it; the emulator run script stays on
  branch sbemu. Not modelled: SQL and correlation filters, rules, auto-forwarding, duplicate detection, message
  size limits, per entity authorization beyond the token audience suffix.

