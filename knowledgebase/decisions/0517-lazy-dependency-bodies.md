# 0517: A build checks and builds a dependency's function bodies as it reaches them

Status: Proposed (Xo Lang 1, 2026-11-05; the semantic change is the
owner's to accept). Stub: the measurement and the full text follow.

## Context

A cold `xo build` of a program using the AWS SDK's s3 and dynamodb
(linux/arm64, main 72b4b94c8) peaks at 1,104 MB against `go build`'s
625 MB. The front end parses, checks and builds every function body of
every module it loads: 3,668 bodies, of which dead code removal keeps
898. Decision 0247 already checks the runtime's bodies only when the SSA
builder reaches them.

## Decision (proposed)

Extend 0247 to non-root modules (dependencies, other workspace members):
their function bodies parsed lazily (decision 0351's lazy parse), checked
when the SSA builder first reaches them, and only functions reachable
from the root modules built. Prototype behind XO_DEP_LAZY=1.

To cover in the full text: signatures, capabilities and effects still
checked eagerly; the cache keys; `xo check`, `xo test` and `xo vet` of
the dependency itself stay complete; whether `xo get` or `xo mod tidy`
gives a published module version a full check.

## Consequences

To be filled in from the prototype's measurement.

## Alternatives

To be filled in.
