# 0352: the front end's data is freed once the SSA exists

Status: Proposed (lead, 2026-10-23)

## Context

A native build keeps every phase's data alive to its end: the checker
(every module's AST, names, types and flow tables, and the parse cache
written at the end) and the SSA module before inlining stay alive
through emit, assembly and link. The scale suite's fns knob measures
about 115 KB of xo memory per small function; xo's self build peaks at
about 10 GB (knowledgebase/infrastructure/scale-suite.md; design note by
Xo Lang 1, 2026-10-23).

## Decision

To be filled in as steps A to C land:

- A: the parse cache is written right after check, not at the build's end.
- B: after the last lowering, the build passes a small BuildFacts value
  (module paths and flags, the link's inputs, diagnostics) instead of the
  checker, and ssa.Builder lets go of the checker.
- C: once the inlined module verifies, the module before inlining is
  dropped (single target OS builds).

## Consequences

Measured per step (fns-16k and compiler/xo peaks, identity, fixed point).

## Alternatives

Keep everything alive (today); free per function instead of per phase
(a larger change to the passes).
