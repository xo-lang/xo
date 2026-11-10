# 0591: std/amqptest's broker methods that read the clock declare `uses clock`

Status: Accepted (lead, 2026-11-10). Drafted by Lang 3.

## Context

Merging std/amqptest with its implementation (decision 0588 stage 2) puts
each declaration's effect clause on its body for the first time. Five
`Broker` methods were declared pure but their bodies read the broker's
clock (`now_ms`, for lock expiry, scheduled times and event hub enqueue
times): `event_hub`, `put`, `ready`, `locked` and `scheduled`. While the
body lived in std/native/amqptest.xo, nothing compared its effects with
the declaration, so the checker never saw the difference; merged, it is
XO0701.

## Decision

The five methods declare `uses clock`, as `serve` already declared
`uses (net.listen, clock)`. The declared effect is what the code does.

## Consequences

- A caller of these methods needs the `clock` capability in its effects:
  a test function or a closure passed where a pure `fn` is expected
  changes. amqptest is a test helper, so the break is small; the Azure
  SDK's azservicebus and azeventhubs tests use it.
- std/amqptest merges (std/amqptest.xo holds its bodies).
- A test that wants a deterministic clock passes one to `amqptest.new`;
  a separate addition if a test needs more.

## Alternatives

- Keep the methods pure by reading a time cached at another call: changes
  behavior (lock expiry would lag) to keep a declaration that was wrong.
- Leave amqptest unmerged: keeps the wrong declaration and blocks 0588.
