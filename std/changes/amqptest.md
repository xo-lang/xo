# std/amqptest changes

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
