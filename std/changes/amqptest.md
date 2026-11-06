# std/amqptest changes

## 2026-11-06: feature: std/amqptest, an in-process Service Bus broker for tests

- Kind: feature
- Commit: this push
- Found by: aws-19's Service Bus client needs (lead, 2026-11-05: no emulator, test against an in-process fake)
- Test: std/native/amqptest_test.xo (8 tests, `xo test --real`: CBS, send/receive/complete, abandon and dead letter, renew-lock, peek, schedule, cancel, lock expiry, receive and delete, topics, deferral with receive-by-sequence-number and update-disposition, sessions and session state, injected faults, forced detach and close) and the golden amqptest_broker (native, LLVM, Go parity)
- Shipped in: not yet released

`amqptest.new(clock)`, `Broker.serve(group, listener)`, queues, topics, subscriptions, and inspection and fault methods (spec/std.md 64.6). `std/amqp` also changes: `ReceiverOptions.sender_settle_mode`, and `new_sender` and `new_receiver` report a refused attach (an answering Attach without target or source, then a Detach with an error) as the `Peer` error at once.
