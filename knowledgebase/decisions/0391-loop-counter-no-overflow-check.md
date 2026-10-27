# 0391: A loop counter that only counts up by 1 needs no overflow check

Status: Accepted (owner, 2026-10-27, relayed by the lead; Xo Lang 3); implemented and measured, no wall gain (pmap 1.003, collatz 0.995), not landed; the branch is kept (local branch loop-counter-0391 and a patch in the lead's notes, xo-lang-3/loop-counter-0391.patch). A rule that changes what the spec allows needs a gain; do not retry it without a loop where the check is on the critical path.

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

## Measured (2026-10-27)

Built and checked: counted-loop codegen patterns (arm64 and amd64: the positive case drops `adds` and `b.vs`;
a variable step, a counter that also decreases and a variable start keep them; they fail on the parent's xo),
`xo test compiler/arm64` 55 of 55, `TestGoldenRun`, fixed point (s2 and s3 identical), the Docker amd64 sample
48 of 48. `xo bench --ab`, 20 rounds, A the parent's xo and std, B this tree's, the same program built by each:

| program | wall B/A | 95% interval | cpu |
|---|---|---|---|
| bench/perf/conc pmap (`steps` inlined into a task) | 1.003 | 0.992 to 1.015 | 590 ms both |
| serial Collatz over 3,000,000 starts | 0.995 | 0.990 to 1.000 | 470 to 460 ms |

The loop in pmap's output lost `adds` and `b.vs` (`add x12, x12, #1`), two instructions an iteration, and no
cycles: the check is a perfectly predicted branch off the loop's critical path (the chain through `n`). The ceiling
I gave for it (4% of pmap's cycles, from the checks) was wrong; replacing the `3 * n` check with a wrapping
multiply was no faster either (1.016 worse, 478.8 to 486.5 ms). pmap's gap to Go is not these checks.

## Consequences

- spec/core.md's overflow section states the rule, so the fault's absence there is
  specified, not an accident.
- The LLVM and Go backends use their own analyses and would not change (not measured).
- Code size and speed: measured in the commit that lands it (pmap, counting loops).

## Alternatives

- Keep the check: the cost is a few percent on counting loops.
- Fuse the check into the loop condition: needs a bound the counter does not have.
- A general induction-variable range proof: larger, and the counter rule covers the loops
  that dominate the benchmarks.
