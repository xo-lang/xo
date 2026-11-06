# std/amqp interop against a real broker

Last verified: 2026-11-05 (Artemis 2.44.0)

`std/native/amqp_test.xo` ends with an opt-in test, "interop: send, receive and settle against a real
broker". It runs only with `xo test --real` and `XO_AMQP_INTEROP_ADDR` set, and passes without doing
anything otherwise, so no gate or tier runs it. It is not the stage 4 real-namespace test (decision 0521):
that one stays owner-only.

## Run it (Mac, Docker Desktop)

Own container, removed by its ID, Docker started and stopped in one chain (feedback: never open Docker by hand):

    ~/xo-lead/agents/docker-up.sh && {
      CID=$(docker run -d -p 127.0.0.1:56720:5672 -e ARTEMIS_USER=artemis -e ARTEMIS_PASSWORD=artemis apache/activemq-artemis:latest-alpine)
      until nc -z 127.0.0.1 56720; do sleep 1; done; sleep 5
      XO_AMQP_INTEROP_ADDR=127.0.0.1:56720 xo test --real --count=1 --std-module=native/amqp --run interop
      docker rm -f "$CID"; }; ~/xo-lead/agents/docker-up.sh stop

`XO_AMQP_INTEROP_USER` and `XO_AMQP_INTEROP_PASSWORD` default to artemis / artemis.

## What was checked (2026-11-05)

Image `apache/activemq-artemis:latest-alpine`, digest `sha256:1bce124d2324faeb1253e2db4bb68d64decf46412ecfd8d9ef46c08b1b4af5b4`
(linux/arm64), AMQP on port 5672. Passed: SASL PLAIN and Open (idle timeout and `timeout_ms` on), Begin, a sender
(target capability `queue`) sending 3 messages with properties and application properties, all `Accepted`; a receiver
(source capability `queue`, credit 3) getting them in order; accept, release (redelivered) and accept; closes.

Findings: Artemis creates a multicast address for an attach without the `queue` capability, so messages sent before the
receiver exists are dropped and a receive times out; a client must set `target_capabilities` / `source_capabilities` to
`["queue"]` for queues (go-amqp users do the same).

## Not covered yet

- CBS put-token: Artemis has no `$cbs` node. Microsoft's Service Bus emulator (`mcr.microsoft.com/azure-messaging/servicebus-emulator`)
  does, but needs a SQL Server container (linux/amd64 only), so it runs on the amd64 offload host, not the Mac.
- A second broker (Qpid, RabbitMQ 4). The second test also sends a message and settles it `modified` with delivery-failed: Artemis redelivers it with `header.delivery_count` 1.

Second test (same day): a 200 000 byte delivery (several transfer frames at the default 64 KiB frame size) sent,
received and compared byte for byte; a small message sent and rejected; `keepalive` running over a 1.5 s idle period;
closes. Finding: Artemis 2.44.0 closes the connection with `amqp:internal-error` AMQ119008 ("error cancelling message
AMQPLargeMessage") when a large message is rejected, so the test rejects a small one.

## Service Bus emulator attempt (2026-11-06, owner approved ACCEPT_EULA=Y in the session)

`~/xo-lead/notes/sbemu/run.sh up|down` starts `mcr.microsoft.com/azure-sql-edge` and
`mcr.microsoft.com/azure-messaging/servicebus-emulator` on a private network. On the Mac (Docker Desktop, arm64) the SQL
Edge container's sqlservr crashed (a core dump in its log, DB recovery then exit), the emulator's health check then
reported "Name or service not known" for the SQL host and halted, so the AMQP port never opened. Not tried yet: an
amd64 host with Docker (the emulator's supported target), or a SQL Server 2022 image in place of SQL Edge. The test
(`interop: Service Bus emulator, CBS put-token...`, `XO_AMQP_SB_ADDR`) is written and skips without the variable.

Re-run 2026-11-05 after the reader task (keepalive owns the transport read): the three interop tests pass unchanged against Artemis (`apache/activemq-artemis:latest-alpine`, image sha256:1bce124d2324).
