# 0521: std/amqp, a port of github.com/Azure/go-amqp (AMQP 1.0)

Status: Proposed (Xo Lang 3, 2026-11-05, at the lead's request for the Azure SDK; the owner decides)

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
3. The opt in real namespace smoke test needs a connection string from the
   owner; it would live outside CI.
