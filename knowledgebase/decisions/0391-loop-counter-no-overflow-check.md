# 0391: A loop counter that only counts up by 1 needs no overflow check

Status: Accepted (owner, 2026-10-27, relayed by the lead; Xo Lang 3). Implementation in progress.

## Context

Spec core.md (the overflow section) says an Int `+ - *` that overflows faults. The native
backend keeps that check on every add it cannot bound, so a counting loop such as pmap's
`steps` (`c += 1` in a Collatz loop) pays an `adds` and a branch each iteration. Go has
no check there; the 0.32.0 comparison has pmap at 1.076 of Go's wall time, about 4 of
those points from the checks (2026-10-26 cycle counts).

## Decision

The compiler may drop the overflow check on `c + 1` when c is a loop-carried counter of
the plain Int type: every value that reaches c's block parameter is a constant of at most
2^32 in magnitude, the counter itself, or the counter plus 1. Such a value is at most
2^32 plus the number of increments the program has executed, and 2^62 increments are
unreachable (a billion a second takes 146 years), so the fault could never be observed:
the program's behavior does not change. Exactly that pattern: a counter incremented by a
variable, one that also decreases (`c -= 1`, `c + -1`), one that starts at a variable,
one that takes a call's result, and every sized integer type keep their check.

The range analysis (compiler/arm64/ranges.xo) gives such a parameter the interval
[start, 2^62] instead of widening it to the top of Int, and the existing `safe_add` then
removes the check on the `+ 1`. Other facts derived from that interval hold under the same
unreachability assumption.

## Consequences

- spec/core.md's overflow section states the rule, so the fault's absence there is
  specified, not an accident.
- LLVM and Go backends need nothing (LLVM already proves counters; the Go backend does not
  use the arm64 analysis).
- Code size and speed: measured in the commit that lands it (pmap, counting loops).

## Alternatives

- Keep the check: the cost is a few percent on counting loops.
- Fuse the check into the loop condition: needs a bound the counter does not have.
- A general induction-variable range proof: larger, and the counter rule covers the loops
  that dominate the benchmarks.
