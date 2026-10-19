# 0271: Whole-program compilation with a parallel module front end

Status: Proposed (xo-fa, 2026-10-19; owner-approved direction, lead
2026-10-19). Stub claiming the number; the record is filled in from the
cold-build phase profile and the design.

## Context

To be written: a cold build of xo profiled by phase and by cores used
(serial and parallel time per phase), the ceiling that sets, and the
front end caches of decision 0220 (addendum stage 2).

## Decision

To be written: the compilation model stays whole-program optimization
(cross-module inlining, borrow inference, monomorphization, dead code
elimination) plus fine-grained caches plus type checking and SSA
building of independent modules in parallel, in dependency order; not
separate compilation in the style of Go. Which checker and SSA state is
per module and which is global, and how determinism is kept (binaries
identical to a serial build, identity tests, the fixed point, verify
mode).

## Consequences

To be written.

## Alternatives

To be written.
