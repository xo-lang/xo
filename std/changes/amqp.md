# std/amqp changes

## 2026-11-10: refactor: width bounds as the type constants (decision 0587 point 4)

## 2026-11-10: refactor: integer bounds as `Int.max` and `Int.min` (decision 0587 point 4)

- Kind: refactor
- Commit: this commit
- Found by: decision 0587 point 4 (std's literal bounds become the type constants)
- Go reference: none (no behavior change; Go's math.MaxInt32, MaxUint16, MaxUint32)
- Test: xo test --std-module=amqp and its corpus where it has one

In std/amqp, the range checks of the uint and ushort fields are against `Int(UInt32.max)` and `Int(UInt16.max)`. No behavior change.

- Go reference: none (no behavior change; Go's math.MaxInt64 and MinInt64)
- Test: the std module tests and goldens of the gate; the self build

`9223372036854775807` and `-9223372036854775807 - 1` in std/amqp's code are
`Int.max` and `Int.min`; masks, Float literals, comments and texts keep their
digits. No behavior change.

## 2026-11-06: feature: SenderOptions and ReceiverOptions take the Attach's desired capabilities

- Kind: feature
- Commit: this push
- Found by: aws-19's Event Hubs v2 slice 2 (geo-replication asks for `com.microsoft:georeplication` on the link)
- Go reference: go-amqp SenderOptions.DesiredCapabilities, ReceiverOptions.DesiredCapabilities; azeventhubs v2.0.2 internal/constants.go CapabilityGeoDRReplication ("com.microsoft:georeplication")
- Test: std/native/amqp_test.xo "a link's desired capabilities go in its Attach, apart from its source's and target's"
- Timing: none
- Shipped in: not yet released

`desired_capabilities` on `SenderOptions` and `ReceiverOptions` is sent as the Attach frame's desired-capabilities, the link's own, apart from the source's and target's `capabilities` the options already mapped. The Attach type and its encoding already carried the field; the link constructors now pass it through. The default, empty, sends the field as before (absent).

## 2026-11-06: fix: an unsettled send no longer waits forever when its outcome arrives at once

- Kind: fix
- Commit: this push
- Found by: llvm TestGoldenTasks/amqptest_broker hanging in about 1 run of 4 on linux/arm64, and once in the linux/amd64 hourly run
- Go reference: none (go-amqp records the delivery before it writes the transfer)
- Test: std/native/amqptest_test.xo "every unsettled send gets its outcome with the reader task running" (a guard: it passed on the old code too, 100 runs with --real); the proof is the golden under load, 16 native copies at a time pinned to 4 CPUs on linux/arm64: 3 hangs in the first 48 runs before, 0 in 640 after
- Timing: none
- Shipped in: not yet released

With the reader task running (`keepalive`), `Sender.send` recorded its delivery as unsettled in a lock section after the one that wrote the last transfer frame. A disposition the reader applied in between matched no delivery and was dropped, and `send` waited for it forever (3 hangs in the first 48 runs of the golden, 16 at a time pinned to 4 CPUs). The delivery now counts as sent in the same lock section as its last frame's write.

## 2026-11-05: feature: message_format on send, manual credit and drain, settle mode Second waits, remote_attach

- Kind: feature
- Commit: this push
- Found by: aws-19's azservicebus (Go v1.11.0) port over std/amqp and std/amqptest
- Go reference: github.com/Azure/go-amqp v1.7.0 (Receiver.IssueCredit, drain in ReceiveMessages, AcceptMessage in settle mode Second, Sender.Send with MessageFormat)
- Test: std/native/amqptest_test.xo (batch format, manual credit and drain, settle mode Second with a lost lock, typed remote attach) and the amqptest_broker golden on every backend
- Timing: none
- Shipped in: not yet released

`Sender.send(m, settled, message_format)`; `ReceiverOptions.credit: 0`, `Receiver.issue_credit(n)` and `Receiver.drain(timeout_ms)`; with `ReceiverSettleMode.Second` `accept`, `reject`, `release` and `modify` wait for the peer's settlement and report a refusal as `Peer(error)`; `modify(..., annotations)`; `Sender.remote_attach()` and `Receiver.remote_attach()`. A timed `receive` without the reader task no longer deadlocks in the test world (see prelude changes: a FakeNet read is woken by its deadline).

## 2026-11-05: feature: std/amqp stage 1, the AMQP 1.0 type system and encoding

- Kind: feature (port)
- Commit: this push
- Found by: the Azure SDK's Service Bus and Event Hubs needs (decision 0521, accepted by the owner 2026-11-05)
- Go reference: github.com/Azure/go-amqp v1.7.0 internal/encoding (MIT, Copyright 2017 Kale Blankenship and Microsoft): types.go, encode.go, decode.go
- Test: the amqp corpus (scripts/stdcorpus/amqporacle: go-amqp's encoder and decoder on typed cases, on native, LLVM and the Go backend) and std/native/amqp_test.xo
- Timing: Mac arm64, decode plus encode of the corpus values (147 cases x 2000, Xo on Values, go-amqp on Message.UnmarshalBinary and MarshalBinary into `any`): Go 963 and 985 ms, Xo 294 and 352 ms (0.31 to 0.36 of Go; Go builds interface values and goes through the message wrapper, so this is the cost of the whole path, not of the codec alone)
- Shipped in: not yet released

`amqp.encode(Value)` and `amqp.decode(Bytes)` over a `Value` enum of every AMQP 1.0 type (spec/std.md 64). Stages 2 to 4 (connection and sessions, links, CBS) follow.

## 2026-11-05: feature: std/amqp stage 2a, frames and performatives

- Kind: feature (port)
- Commit: this push
- Found by: the Azure SDK's Service Bus and Event Hubs needs (decision 0521)
- Go reference: github.com/Azure/go-amqp v1.7.0 internal/frames and internal/encoding composites (MIT, Copyright 2017 Kale Blankenship and Microsoft)
- Test: std/native/amqp_test.xo and the amqp corpus FRAMES table (the frames go-amqp's client wrote over a fake peer, on native, LLVM and the Go backend)
- Timing: none for this stage (it adds no hot path; stage 1's row stands)
- Shipped in: not yet released

`amqp.encode_frame` and `amqp.decode_frame` over `Frame` and `FrameBody` (spec/std.md 64.1): every AMQP and SASL performative as a typed struct, byte identical to go-amqp's encoder for the frames its client sends. Stage 2b (connection, SASL and sessions) follows.

## 2026-11-05: feature: std/amqp stage 2b-i and 2c, the connection and messages

- Kind: feature (port)
- Commit: this push
- Found by: the Azure SDK's Service Bus and Event Hubs needs (decision 0521)
- Go reference: github.com/Azure/go-amqp v1.7.0 conn.go (the open, SASL and header exchange) and message.go (MIT, Copyright 2017 Kale Blankenship and Microsoft)
- Test: std/native/amqp_test.xo (an in-process peer over a loopback listener: SASL PLAIN accepted and refused, open, begin, end, close, a peer End with an error) and the amqp corpus MESSAGES table (go-amqp's encoder on 22 messages, on native, LLVM and the Go backend)
- Timing: none for this stage (stage 1's row stands)
- Shipped in: not yet released

`amqp.dial`, `Connection` (`send`, `recv`, `begin`, `end_session`, `close`, `abort`) and `amqp.encode_message` / `decode_message` over `Message` (spec/std.md 64.2, 64.3). The connection is driven by one task; a background reader and the session mux are stage 2b-ii.

## 2026-11-05: feature: std/amqp stages 3 and 4, links and CBS

- Kind: feature (port)
- Commit: this push
- Found by: the Azure SDK's Service Bus and Event Hubs needs (decision 0521)
- Go reference: github.com/Azure/go-amqp v1.7.0 sender.go, receiver.go and session.go (MIT, Copyright 2017 Kale Blankenship and Microsoft); the CBS put-token shape of the Azure SDK for Go
- Test: std/native/amqp_test.xo (an in-process peer: a sender whose 1500 byte message is split to the peer's 512 byte frames and settled by the peer, a receiver assembling a two frame delivery and accepting it, CBS put-token with a 202 and with a 401)
- Timing: none for this stage (no hot path; stage 1's row stands)
- Shipped in: not yet released

`Connection.new_sender` and `new_receiver`, `Sender.send` (credit, session window, fragmentation, outcome), `Receiver.receive`, `accept`, `reject`, `release`, `modify`, `close`, and `amqp.cbs_put_token` (spec/std.md 64.4, 64.5). Driven by one task; the real namespace smoke test is run by the owner only (decision 0521).

## 2026-11-05: feature: std/amqp stage 2b-ii, timeouts and heartbeats

- Kind: feature (port)
- Commit: this push
- Found by: the Azure SDK's Service Bus and Event Hubs needs (decision 0521)
- Go reference: github.com/Azure/go-amqp v1.7.0 conn.go (idle timeout, keepalive), MIT
- Test: std/native/amqp_test.xo (three `uses (net, clock)` tests, run with `xo test --real`)
- Shipped in: not yet released

`ConnOptions.clock` and `timeout_ms`, `Connection.keepalive(group)`: i/o timeouts, empty-frame heartbeats at half the peer's idle timeout, and a failure when the peer is silent for `idle_timeout_ms`. All `amqp` calls that block now declare `uses (net.dial, clock)` (spec/std.md 64.2).

An opt-in interop test (`xo test --real` with `XO_AMQP_INTEROP_ADDR`) passes against `apache/activemq-artemis:latest-alpine` (knowledgebase/infrastructure/amqp-interop.md).

## 2026-11-05: behavior: amqp.Value.Float holds a Float32

- Kind: behavior
- Commit: this push
- Found by: the std survey follow-up (decision 0521)
- Test: the amqp corpus (float cases on native, LLVM and the Go backend) and std/native/amqp_test.xo
- Shipped in: not yet released

`Value.Float(v: Float32)` replaces `Value.Float(bits: UInt32)`; the wire bits still round trip exactly, NaN payloads included (spec/std.md 64).

## 2026-11-06: feature: amqp connect timeout and Receiver.receive(timeout_ms)

- Kind: feature
- Commit: this push
- Found by: aws-19's Service Bus needs (lead, 2026-11-05)
- Test: the interop test against Artemis (an empty queue gives `i/o timeout` at the deadline, then the link still works)
- Shipped in: not yet released

`dial` bounds the connect by `ConnOptions.timeout_ms`; `Receiver.receive(timeout_ms: Int = 0)` waits at most that long for a delivery. Together with 2b-ii this covers connect, CBS and attach waits, receive and idle heartbeat.

## 2026-11-06: feature: amqp reader task, concurrent calls on one connection

- Kind: feature
- Commit: this push
- Found by: aws-19's Service Bus needs (lead, 2026-11-05: concurrent receivers, a sender while a receive waits, lock renewal while processing)
- Go reference: github.com/Azure/go-amqp v1.7.0 conn.go (connMux), MIT
- Test: "two receivers and a sender on one connection do not block each other" (the whole module `--real`, the Artemis interop tests, the corpora on native, LLVM and the Go backend)
- Shipped in: not yet released

`Connection.keepalive(g)` now starts a reader task that reads without the connection lock; calls that block (`receive`, `send`, link and session setup and close, `close`) wait on their own channel with their own deadline instead of holding the lock, so a waiting receive no longer stops a send or another receiver. Without `keepalive` nothing changes. Raw `recv` is refused while the reader runs (spec/std.md 64).

Concurrent `begin` calls reserve their channel in the same critical section that picks it, so they get distinct channels (test: 8 concurrent begins).

## 2026-11-06: feature: cbs_put_token takes the expiration property as it is

- Kind: feature
- Commit: this commit
- Found by: Xo Azure 1 (a byte for byte comparison with Go's clients)
- Go reference: azeventhubs and azservicebus send the put-token `expiration` as a string of Unix seconds
- Test: native/amqptest "put-token takes expiration as a string of Unix seconds, as Go's clients send it"

`cbs_put_token(..., expiration: Value? = None)` sends the given value as `expiration` and wins over `expiry_ms`. The test
broker (std/amqptest) reads a string of Unix seconds as well as a Timestamp.

## 2026-11-06: feature: ConnOptions.hostname is the TLS server name of an amqps dial

- Kind: feature
- Commit: this commit
- Found by: Xo Azure 1 (a TLS proxy such as an Azure custom endpoint)
- Go reference: go-amqp ConnOptions.HostName
- Test: golden amqp_tls_hostname (hostname good.test connects past the handshake, wrong.test fails the certificate check)

`amqp.dial` of an `amqps://` address sent and verified the TLS server name of the address's host. With
`ConnOptions.hostname` set it now uses that name for both, as it already did for the Open's hostname, so a client
dialing a proxy's address can verify the namespace's name.
