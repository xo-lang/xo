# std/amqptest changes

## 2026-11-10: behavior: broker methods that read the clock declare `uses clock` (decision 0591)

- Kind: behavior (breaking)
- Commit: this commit
- Found by: decision 0588 stage 2 (merging the module checked its bodies against their declarations)
- Go reference: none
- Test: std/amqptest_test.xo (`xo test --std-module=amqptest` on native and llvm); golden amqptest_broker

`Broker.event_hub`, `put`, `ready`, `locked` and `scheduled` were declared pure but read the broker's clock; they now
declare `uses clock`. std/amqptest.xo also holds its bodies now (std/native/amqptest.xo removed, decision 0588).

## 2026-11-06: fix: a frame sent after disconnect_all or detach_links returns is never answered on what they ended

- Kind: fix
- Commit: this push
- Found by: llvm TestGoldenTasks/amqptest_broker failing in 1.25 s in Lang 4's gate (750a25b71); under load 3 times in 8 rounds, "begin ok" for "begin failed"
- Go reference: none (a test double)
- Test: the golden under load, 8 native and 8 TSan copies at once pinned to 4 CPUs on linux/arm64: 3 failures in the first 8 rounds (128 runs) before, 0 in 60 rounds (960 runs) after
- Timing: none
- Shipped in: not yet released

`disconnect_all` and `detach_links` record the request and wake each connection's loop, which selected between the peer's next frame and the wake; a Begin already read could be answered before the close was applied. Each connection now applies the pending detaches and closes before it handles a frame (knowledgebase/gotchas/test-control-races-peer-frame.md).

## 2026-11-06: feature: an event hub's READ reports georeplication_factor; the broker logs each attach's desired capabilities

- Kind: feature
- Commit: this push
- Found by: aws-19's Event Hubs v2 port (azeventhubs v2.0.2 newEventHubProperties fails without georeplication_factor; slice 2 checks com.microsoft:georeplication on each link)
- Go reference: github.com/Azure/azure-sdk-for-go/sdk/messaging/azeventhubs v2.0.2 (mgmt.go newEventHubProperties, internal/constants.go CapabilityGeoDRReplication)
- Test: std/native/amqptest_test.xo "an event hub answers the hub and partition properties on the namespace management link, and the broker logs each attach"
- Timing: none
- Shipped in: not yet released

The `com.microsoft:eventhub` READ reply carries `georeplication_factor`, a Long, 1 by default; `Broker.event_hub_geo_factor(hub, n)` sets it (above 1: geo-replication enabled). `Broker.attaches()` lists every link a client attached: its address (a sender's target, a receiver's source), the client's role and the Attach's desired capabilities.

## 2026-11-05: fix: lock tokens in Service Bus's byte order and in the delivery annotations

- Kind: fix
- Commit: this push
- Found by: aws-19's port of azservicebus (Go's lockTokenFromMessageTag swaps the tag back; it reads x-opt-lock-token from the delivery annotations only)
- Go reference: github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus (internal/amqpwrap, message.go)
- Test: std/native/amqptest_test.xo "send, receive and complete through the broker, with CBS" (tag swapped equals the annotation token)
- Timing: none
- Shipped in: not yet released

A delivery's tag is the lock token in .NET GUID byte order (the first three groups little endian), as on real Service Bus, and `x-opt-lock-token` is also in the delivery annotations (it stays in the message annotations). Tokens now carry a non-palindromic prefix so the order shows. A client that used the tag as the token must read the annotation or swap the tag.

## 2026-11-06: feature: std/amqptest, an in-process Service Bus broker for tests

- Kind: feature
- Commit: this push
- Found by: aws-19's Service Bus client needs (lead, 2026-11-05: no emulator, test against an in-process fake)
- Test: std/native/amqptest_test.xo (8 tests, `xo test --real`: CBS, send/receive/complete, abandon and dead letter, renew-lock, peek, schedule, cancel, lock expiry, receive and delete, topics, deferral with receive-by-sequence-number and update-disposition, sessions and session state, injected faults, forced detach and close) and the golden amqptest_broker (native, LLVM, Go parity)
- Shipped in: not yet released

`amqptest.new(clock)`, `Broker.serve(group, listener)`, queues, topics, subscriptions, and inspection and fault methods (spec/std.md 64.6). `std/amqp` also changes: `ReceiverOptions.sender_settle_mode`, and `new_sender` and `new_receiver` report a refused attach (an answering Attach without target or source, then a Detach with an error) as the `Peer` error at once.

## 2026-11-06: feature: amqptest batches, drain and the locked-until property

- Kind: feature
- Commit: this push
- Found by: aws-19's session and batch paths
- Test: std/native/amqptest_test.xo (raw frame tests: a null session filter and the attach reply, a batch and a drain, a scheduled message appearing on the broker's clock)
- Shipped in: not yet released

Transfers with message format 0x80013700 are unpacked into separate messages, a drain flow is answered, and a session receiver's attach reply carries `com.microsoft:locked-until-utc`.


## 2026-11-06: fix: a dead letter is settled with Rejected and no error

- Kind: fix
- Commit: this commit
- Found by: Xo Azure 1 (azservicebus dead_letter_message under ReceiverSettleMode.Second)
- Go reference: go-amqp v1.7.0 receiver.go: the service answers a dead letter with a rejected response that carries no error
- Test: native/amqptest "settle mode Second: a dead letter is settled without an error, so reject succeeds"

The broker settled a client's `reject` with the client's own `Rejected(error)`, so a receiver in settle mode
Second reported the successful dead letter as `Peer(com.microsoft:dead-letter)`. It now answers `Rejected(error: None)`;
`Rejected(error)` stays for refusals such as a lost lock.

## 2026-11-06: feature: a Modified outcome's annotations are applied to the message

- Kind: feature
- Commit: this commit
- Found by: Xo Azure 1 (azservicebus abandon and defer properties to modify)
- Go reference: Service Bus applies the properties to modify to the message's application properties
- Test: native/amqptest "a Modified outcome's annotations land in the message's application properties"

`Receiver.modify(..., annotations:)` now lands in the stored message's `app_properties` (a later key replaces an
earlier one), visible through `Broker.ready` and the next delivery.

## 2026-11-06: feature: get-message-sessions on a session queue's $management

- Kind: feature
- Commit: this commit
- Found by: Xo Azure 1 (azservicebus v1.11.0 internal/mgmt.go GetMessageSessions)
- Go reference: azservicebus GetMessageSessions request and reply shape
- Test: native/amqptest "get-message-sessions lists the sessions with messages, pages by skip and top, and answers 204 when none"

`com.microsoft:get-message-sessions` answers `sessions-ids` (an Array of Str, name order) and `skip`, for the sessions with
messages (last-updated-time at the year 10000) or whose state was set after that time; `skip` and `top` page the list and
no sessions is 204. `FakeSession` gains `updated_ms`, set by set-session-state.

## 2026-11-06: feature: event hubs (Broker.event_hub, Broker.events)

- Kind: feature
- Commit: this commit
- Found by: Xo Azure 1 (azeventhubs v1.4.0 port)
- Go reference: azeventhubs v1.4.0 partition_client.go, mgmt.go, event_data.go (what the client sends and expects)
- Test: native/amqptest "an event hub ..." (three tests), golden amqptest_broker

`Broker.event_hub(name, partitions, consumer_groups)` declares a hub; sends to `hub` or `hub/Partitions/id` store events with
x-opt-sequence-number, x-opt-offset and x-opt-enqueued-time; receivers at `hub/ConsumerGroups/cg/Partitions/id` read from a
selector filter without removing anything; `com.microsoft:epoch` ownership with `amqp:link:stolen`; the namespace
`$management` link answers READ for the hub and a partition; `Broker.events` lists a partition. spec/std.md 64.6.

## 2026-11-06: feature: put-token expiration as a string of Unix seconds

- Kind: feature
- Commit: this commit
- Found by: Xo Azure 1
- Go reference: azeventhubs and azservicebus send the put-token `expiration` as a string of Unix seconds
- Test: native/amqptest "put-token takes expiration as a string of Unix seconds, as Go's clients send it"

The broker reads the put-token `expiration` as a Timestamp or as a string of Unix seconds.

## 2026-11-06: fix: event hub links accept a claim on their own path

- Kind: fix
- Commit: this commit
- Found by: Xo Azure 1 (azeventhubs NegotiateClaim puts the token on the link's path)
- Go reference: azeventhubs links negotiate a claim for `hub/Partitions/id` or `hub/ConsumerGroups/cg/Partitions/id`
- Test: native/amqptest "an event hub link under require_token accepts a claim on the hub or on its own path"

Under `require_token(true)` an event hub link is authorized by a claim whose audience ends with the hub's name or with the
link's own path.
