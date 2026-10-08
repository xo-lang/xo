# 0004: Effects in signatures, authority via capability values

Status: Proposed, pending benchmark (2026-10-07). See ../../bench/README.md

## Context
Agent written code and dependencies need an auditable blast radius. Record and
replay needs every side effect to pass through one interception point.

## Decision
Builtin effect labels (`net fs clock rand env proc stdio ffi`, with sub effects)
appear in `uses` clauses, required on `pub` functions and inferred otherwise.
Authority comes only from capability values rooted at `main(os: Os)`.

## Consequences
- Supply chain reach is visible in signatures.
- Tests get fake capabilities by default.
- Replay (P2) intercepts capability calls.
- Cost: capabilities must be threaded through code. Mitigated by storing them
  in structs (e.g. a `Store` holding a `Net`).

## Alternatives
- Effects only (no capability values): no real authority boundary.
- Capabilities only (no `uses`): effects hidden inside closures and struct fields.

## Evidence
- Paper test 2026-10-07 ([results](../../bench/results/paper-2026-10-07.md)): 0 violations, medium friction in 2 of 5 tasks (no effect polymorphism, subsumption unspecified).
- Paper test 2026-10-07 spec 0.2 ([results](../../bench/results/paper-2026-10-07-v0.2.md)): 0.2 run: effect plumbing heavy in generic code; purity unclear around Mutex.
- Paper test 2026-10-07 spec 0.3 ([results](../../bench/results/paper-2026-10-07-v0.3.md)): 0.3 run: capability threading verbose (03); closure creation effects unclear (05).
- Harder tasks 2026-10-07: effects leak into generic type names (`Cache[..., net.dial]`) and repeat in handler signatures (06, 07). ([results](../../bench/results/planted-and-harder-2026-10-07.md))
