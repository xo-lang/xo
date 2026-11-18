# std/amqp changes

## 2026-11-18: fix: a null message section is skipped, as go-amqp

- Kind: fix
- Commit: this push
- Found by: task #152 (xw-1's amqp fuzz against go-amqp v1.4.0, seed 43 on host 3: 1 of 4601 cases differ, `00 53 71 40`)
- Go reference: go-amqp v1.4.0 message.go Message.Unmarshal, internal/encoding/decode.go Unmarshal (returns at a null)
- Test: std/amqp_test.xo "a null section leaves the message as it was, as go-amqp (TestAmqpFuzz seed 43, task #152)" (an error before)
- Shipped in: not yet released

decode_message refused a section whose value is null. A null delivery-annotations, message-annotations,
application-properties, footer or amqp-value section now leaves the message as it was, and a null data or
sequence section is an empty one, as go-amqp; a null header or properties section is still an error.

## 2026-11-17: fix: a null application-properties key is an error, as go-amqp

- Kind: fix (behavior change)
- Commit: this commit
- Found by: TestAmqpFuzz seed 9 (task #152, fuzz evidence on darwin/arm64)
- Go reference: github.com/Azure/go-amqp v1.7.0 readString ("type code 0x40 is not a recognized string type")
- Test: std/amqp_test.xo "an application-properties key is a str or sym, never null, as go-amqp"; TestAmqpFuzz seeds 9 and 10
- Timing: none (one comparison per key)
- Shipped in: not yet released

decode_message read a null (0x40) application-properties key as the key "" and kept the message; it is now refused with "application-properties key is null". The fuzz oracle (scripts/stdcorpus/amqporacle) also counts a header ttl past what Go's time.Duration holds in milliseconds as one go-amqp cannot represent (go-amqp wraps it; std/amqp keeps the wire value).

## 2026-11-16: fix: compound values and booleans decode as go-amqp reads them (task #115)

- Kind: fix (behavior change)
- Commit: this commit
- Found by: TestAmqpFuzz (internal/stdtest/amqpfuzz_test.go), a differential of decode_message against github.com/Azure/go-amqp v1.7.0's Message.UnmarshalBinary on go-amqp's encodings of seeded messages, most mutated (scripts/stdcorpus/amqporacle fuzz mode, scripts/amqpdiff/xo)
- Go reference: go-amqp internal/encoding readListHeader, readMapHeader, unmarshalMapItems, readArrayHeader and the typed arrays, readBool
- Test: std/amqp_test.xo "lists and maps read their count of elements, booleans any nonzero byte, as go-amqp" (fails before); TestAmqpFuzz seeds 2 to 8 (about 18,500 cases each, equal on amd64 and llvm)
- Shipped in: not yet released

- A list, map or array is its count of elements, read after its count and ending where they end;
  the size is checked against the bytes left, not against the elements (it had to match them
  exactly). An array's elements are read from the bytes left, past its size; a map's size may be
  below its count field; a map's count is checked against the bytes left.
- An odd map count reads its last key's value after the count (go-amqp's `i += 2` loop); it was
  "odd map count".
- A boolean (0x56, alone or in an array) is true for any byte but 0; it was "invalid boolean" past 1.

decode_message's doc lists the departures the fuzz models: std/amqp refuses a str or sym that is
not UTF-8, a second header or properties section, body sections of two kinds or a second value
section, and a ulong section descriptor whose code is past a byte (go-amqp reads its last byte),
all of which go-amqp accepts; it decodes char, the decimals, described values in any place, list,
map and binary map keys, and header or properties fields in any integer encoding, which go-amqp
cannot (it panics on a binary annotations key).

Found with it, not fixed here: on the native backends `Float32.from_bits` quiets a signaling NaN
(7f800043 reads back as 7fc00043; std/native/prelude.xo narrows a double, and `float32_to_bits`
sets the quiet bit), so a float32 sNaN does not round trip through std/amqp there; the Go backend
keeps the bits. Reported to the lead; the fuzz compares those values quieted.

## 2026-11-17: feature: SASL EXTERNAL and XOAUTH2

- Kind: feature
- Commit: this commit
- Found by: task #184 (stdcov feature gaps: go-amqp's SASLTypeExternal and SASLTypeXOAUTH2)
- Go reference: github.com/Azure/go-amqp v1.7.0 sasl.go
- Test: std/amqp_test.xo's "SASL EXTERNAL ...", "SASL XOAUTH2 ...", "a refused XOAUTH2 token ..." and "XOAUTH2 refuses ..." tests

`Sasl.External(response)` sends response as the EXTERNAL initial response
(the identity comes from the TLS client certificate). `Sasl.Xoauth2(user,
bearer)` sends `user=U\x01auth=Bearer B\x01\x01`; a server that refuses
the token sends its error as a challenge, the client answers with an empty
response, and the Auth error reads as go-amqp's ("SASL XOAUTH2 auth failed
with code 0x1: ..."), as does a second challenge, an empty or unprintable
bearer and a user holding `\x01`. go-amqp's frame size override has no
counterpart: SASL frames are not held to the peer's frame size here.

## 2026-11-15: perf: `parse_addr` finds its separators with `find` and `rfind`

- Kind: perf
- Commit: this commit
- Found by: `xo vet --cost --std-module=amqp` (the stdfast push check, 2026-11-15): three XO0913 notes
- Go reference: none (amqp091-go parses the URI with net/url)
- Test: std/amqp_test.xo's parse_addr cases

The address parser scanned for `/`, `]` and `:` a byte at a time with a
one-byte slice each step; it now calls `find("/")`, `rfind("]")` and
`rfind(":")`. The results are the same.

## 2026-11-15: feature: `AmqpErr` is an open enum (decision 0649)

- Kind: feature
- Commit: this commit
- Found by: decision 0649 (a new variant of a std error enum broke two SDKs, 0644's `ClientErr.Tls`)
- Go reference: none (Go's errors are values compared with errors.Is; Rust's `#[non_exhaustive]`)
- Test: compiler/check's open enum test; the open_enum golden; xo check over the programs that use the module

`AmqpErr` is declared `open`: a `match` on it outside std needs a `_` (or
binding) arm, warning XO0306 in 0.52 and an error after, so that std can
add a variant in a minor release without breaking a build. Matches that
already have `_` are unchanged.

## 2026-11-14: security: compound counts are bounded as go-amqp's are

- Kind: security
- Commit: this push
- Found by: Xo Lang 3's sweep of std for allocations sized from a decoded header (after the std/jpeg review's F1)
- Go reference: go-amqp v1.7.0 internal/encoding readArrayHeader, readListHeader and readMap8Header/readMap32Header: a count above maxCompoundCount (65536) or above the body's bytes after the count field is refused, an array's too
- Before: get_value refused a count above the body's bytes for a list or map but not for an array (0xe0, 0xf0), and had no count cap, so an array of a zero width constructor (null, true, false, the zero forms) decoded as many values as its count asked: 10 bytes asking for 10 million took 43 ms and 244 MB, and the 32-bit count's largest about 100 GB; Mac, native
- Fix: MAX_COMPOUND_COUNT (65536) refuses a larger count first, and the body check applies to every compound
- Test: std/amqp_test.xo "compound counts are bounded as go-amqp's are" (go-amqp's four count tests' frames, the 32-bit count's largest, 65536 one byte elements decoding and 65537 refused); xo test --std-module=amqp
- Affected: every backend, 0.41.0 to 0.51.1
- Shipped in: not yet released

## 2026-11-11: refactor: std/amqp is one file (decisions 0588, 0597)

- Kind: refactor
- Commit: this push
- Found by: decision 0597 (std/native/amqp.xo imports nothing native)
- Go reference: none (no behavior change)
- Test: TestStdCorpora amqp on go, arm64, llvm; xo test --real --std-module=amqp (28 passed) on arm64 and llvm; goldens amqptest_broker and amqp_tls_hostname (arm64, llvm, Go parity)
- Shipped in: not yet released

std/native/amqp.xo moved into std/amqp.xo, its tests into std/amqp_test.xo.
In its own module the match arms over its own enums name their variants
instead of `_` (XO0305), and the test peers declare `uses net.listen`.

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

## 2026-11-16: fix: a receiver's prefetch stays within its credit

- Kind: fix
- Commit: this commit
- Found by: azure-1 (std/evidence/amqptest/stress, task #115: consumers under load got lock-lost on deliveries a minute old)
- Go reference: go-amqp v1.7.0 receiver.go (credit issued is the maximum less the messages held)
- Test: native/amqptest "a receiver's prefetch stays within its credit"

A receiver with `ReceiverOptions.credit` topped its credit up to the whole target whenever it fell to half, without
counting the deliveries it had received and not yet handed out, so the inbox grew by about the target on each receive.
Under load the inbox held hundreds of deliveries, and a broker's message locks (Service Bus's 60 s) ran out before
`receive` reached them. The credit now tops up to the target less the deliveries held.
