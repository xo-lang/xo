# std/amqptest changes

## 2026-11-17: fix: a client's sender gets link credit again as it spends it

- Kind: fix
- Commit: this push
- Found by: the amqp stress program (std/evidence/amqp/stress) stopped as idle on hosts 2 and 3 (task #152); on the Mac every task waited in send at its 100,000th message
- Go reference: none (a broker's flow control, AMQP 1.0 section 2.6.7: the receiver issues credit)
- Test: std/amqptest_test.xo "a sender gets more link credit as it spends it" (credit 6, 20 sends; fails before); the stress program past 100,000 sends a task
- Timing: none (one flow frame per half of the credit)
- Shipped in: not yet released

The broker granted a client's sender 100,000 link credit at attach and never again, so a sender stopped for good at its 100,000th message. It now grants the credit again each time half is spent. `BrokerState.link_credit` (default 100,000) sets the grant.

## 2026-11-16: fix: a SQL filter cut short reads "unexpected end of the expression" (task #148)

- Kind: fix (error text)
- Commit: this push
- Found by: the amqptest_rules golden written for TestStdAPICovered (task #148)
- Go reference: none (Service Bus's SQL filter grammar; the text is std/amqptest's own)
- Test: std/amqptest_test.xo's SQL filter table and add_rule test (fail before: "unexpected the end"); golden amqptest_rules
- Shipped in: not yet released

The parser's two "unexpected" errors wrote the end of the input as "unexpected the end"; they now
read "unexpected end of the expression" (an unexpected token is still "unexpected" and its quoted
text). "expected ... at the end" was already right.

## 2026-11-16: perf: queue operations update the message list in place

- Kind: perf
- Commit: this push
- Found by: the std/amqptest Go twin (bench/perf/amqptest, ADR 0655 rule 7)
- Go reference: none (the twin is a Go port of this broker on go-amqp v1.7.0's codec, bench/perf/amqptest/go/goamqp/twinbroker)
- Test: std/amqptest_test.xo (`xo test --real --std-module=amqptest`: 30 passed); bench/perf/amqptest twincheck (none, pipe, pubsub agree)
- Timing: Mac, 2,000 messages, Xo over the Go twin: pubsub 2.80 to 0.91, pipe 0.80 to 0.83, none 0.76 to 0.71 (host 1 row to follow in bench/results)

enqueue, take, the lock expiry and complete copied the queue out of the broker's state and back, so each
update of one message copied the whole message list it shared, and complete rebuilt the list with a filter.
They now update the queue through its place and remove a completed message with remove_at. A broker holding
N messages sent N before any receive went from quadratic copying to the scan alone.
A settled take removes the message it found with remove_at directly (once an arm64 miscompile of a tuple
sret result, fixed in 258bd9d26, no longer crashed it) instead of searching for it again.

## 2026-11-15: feature: subscription rules (SQL and correlation filters, SQL actions, the rule management operations)

- Kind: feature
- Commit: this push
- Found by: the stdaudit features line (decision 0655): amqptest.behavior.rules
- Go reference: none (Service Bus's "Subscription Rule SQL Filter Syntax", "SQL rule action syntax", "Topic filters and actions" and the AMQP request-response operations add-rule, remove-rule and enumerate-rules)
- Test: std/amqptest_test.xo: 51 SQL filter cases (operators, three valued AND, OR and NOT, IN, LIKE with ESCAPE, EXISTS, IS NULL, delimited and quoted names, property(), every system property, type mixes and parse errors), correlation filters, four subscriptions with SQL, correlation and false rules fed by a topic, rule actions (SET, REMOVE, sys.Label, RuleName, a copy per rule), and the three management operations over AMQP with their status codes and encodings
- Timing: none

A subscription took every message of its topic. It now has rules, starting
with `$Default`, which decide what it takes, as Service Bus's do; a test
using no rules sees no change.

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

## 2026-11-16: fix: lock expiry, dead letter reasons and stale deliveries behave as Service Bus

- Kind: fix
- Commit: this commit
- Found by: azure-1 (TestAmqptestFuzz, task #115: go-amqp v1.7.0 against a model of the queue semantics)
- Go reference: azservicebus v1.11.0 messageSettler.go (dead letter with `com.microsoft:dead-letter` and the reason in the error's info) and message.go (DeadLetterReason and DeadLetterErrorDescription read from the application properties)
- Test: native/amqptest "a lock that runs out past the delivery limit dead letters", "a lock that runs out redelivers to a waiting receiver, and the old delivery is lock lost", "closing a receiver after its lock ran out keeps the delivery counted", "a dead letter's reason as azservicebus sends and reads it"

A lock that runs out is a delivery attempt: past the maximum delivery count the message is dead lettered as
MaxDeliveryCountExceeded (it was delivered again), and a broker task wakes the connections when a lock runs out or a
scheduled message comes due, so a waiting receiver gets it (it got nothing until another event). A delivery remembers its
lock token: settling one whose message came back under a new lock is `com.microsoft:message-lock-lost` (it was accepted),
and closing its link gives back only a lock it still holds. Closing a link or losing a connection after a lock ran out
keeps that delivery counted (the give-back took it back). A rejection with `com.microsoft:dead-letter` takes the reason,
description and properties to modify from the error's info, as azservicebus sends them (the condition was the reason),
and a dead lettered message carries DeadLetterReason and DeadLetterErrorDescription in its application properties, where
azservicebus reads them (they were only a delivery annotation). `ready` and `locked` apply the locks that ran out first.

## 2026-11-17: perf: releasing, abandoning, deferring and dead lettering a message no longer copies its queue

- Kind: perf
- Commit: this commit
- Found by: xo-lang-3 (std/evidence/amqptest/stress heap profile, decision 122: 65% of 885 GB allocated in 10 min at put_msg)
- Go reference: none (the broker is Xo's own; Go's twin in bench/perf/amqptest updates the message in place)
- Test: native/amqptest "settling a message does not copy the queue's message list"

unlock, apply_modified, defer_msg, dead_letter and release_conn took a copy of the queue, which shares its message
list, so the first change copied the whole list: O(queue length) per settle. They now change the message through
`s.queues[qi]` in place, as enqueue, complete and expire_locks have since 2026-11-16, and dead_letter takes the
message out with remove_at in place of a filter over the list.
