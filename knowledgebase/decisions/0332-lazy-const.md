# 0332: lazy `const` (a value computed once per process on first use)

Status: Accepted in direction (owner, 2026-10-21, after the language poll);
the design below is a stub. Xo Publish writes the design (syntax, the
once-initialization in the runtime, faults during initialization, effects
allowed in the initializer); Xo Lang Lib 1 measures the gain first by
caching a parsed time layout by hand in one hot path.

## Context

Std re-derives fixed data on every call: parsed time layouts, compiled
regexp patterns, lookup tables built from text. `const` takes only values
the compiler can evaluate, and Xo has no global mutable state, so the
ports either recompute per call or rewrite the parse as byte-level code
(the time format work of 2026-10-21). Two of six agents named it in the
2026-10-21 language poll.

## Decision

To be written: an immutable value computed at most once per process, on
first use, shared by every task, with initialization safe under
concurrency and preemption, on every backend (native, LLVM, wasm, Go).

## Consequences

To be written with the design.

## Alternatives

To be written with the design (compile-time evaluation of more `const`
expressions; an explicit `Once[T]` std type).
