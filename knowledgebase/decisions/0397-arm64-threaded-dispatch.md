# 0397: Threaded dispatch: a jump table loop's dispatch copied into each arm

Status: Accepted (2026-10-28, lead; xo-lang-6). Extends decisions 0051
(jump tables for `match`) and 0075 (code placement). Implemented in
compiler/arm64 threadsw.xo (`thread_switch`, an optional pass after
`tail_dup` in isel.xo's pipeline, on both native backends). Evidence:
bench/results/interp-placement-2026-10-28.md.

## Context

A `match` of 8 or more arms is a jump table (decision 0051): a bounds
test, `adr`, `ldrsw`, `add`, `br`. An interpreter's loop runs its whole
program through that one `br`: interp's 16 op cycle is a period 16
sequence of targets the indirect predictor must learn from one branch
address and the history. How well it does depends on the bits of that
address: the same instruction stream, shifted with `XO_ARM64_PAD` over
16 placements, ran 204.5M to 281.4M cycles (a 38 percent spread; the
default placement was the worst), with identical instruction counts.
That spread was first read as a 15 percent inliner loss (the 0388
stage 3 oracle moved main, and run with it); the hot code was byte for
byte the same (gotchas/arm64-indirect-dispatch-placement.md).

Decision 0075 found that aligning loop tops picks one fixed point and
lost 1 percent elsewhere, so alignment is not the fix. Interpreters
fix this by threading: each handler ends with its own dispatch, so
each indirect branch sees only that op's successors.

## Decision

- `thread_switch` (threadsw.xo), after `tail_dup`: for each loop whose
  header reaches a `K_SWITCH` within four copyable blocks (plain
  instructions and parallel moves, a checked one included: each copy
  gets its own fault stub, the landing pad shared through the stub's
  call site; no call, poll or frame access), with one latch chain into
  the header (the blocks each the only body predecessor of the next,
  up to four) and 2 to 16 arms jumping to it, the chain and the path
  (at most 24 instructions together) are copied into each arm, placed
  right after it; the arm's jump goes to its copy, the copy's switch to
  the shared arms. The original header stays for the loop's entry; the
  latch nothing reaches any more is dropped (`thread`).
- The pass is on by default on arm64 and amd64 (`XO_PASSES=
  -thread_switch` turns it off); it is in passes.xo's order, so `xo
  dev bisect-pass` can name it.
- A code shape change on a dispatch loop is judged by the pad sweep
  (decision 0075's rule, the knob `XO_ARM64_PAD` since 3114a7f7e), not
  by one placement.

## Consequences

- interp: 284.8M to 161.7M cycles at the default placement (78 to 48
  ms), 8 percent fewer instructions (the copied latch saves the arm's
  jump, and the allocator stopped reloading `prog` from the frame);
  over 16 placements 158.4M to 169.1M (a 6.8 percent spread), every
  one under the unthreaded best.
- Code size: a threaded loop grows by (arms times the copied
  instructions plus a jump table per copy): interp's run 491 to 957
  lines. Over 14 bench programs the pass changes two functions: run,
  and std time.format_items (the layout item dispatch; its row in the
  results file).
- The same function has several jump tables; each copy's table is
  named by its block (jumptab.xo emit_switch), so the emitters need no
  change. The pass runs before `x86_retarget`, so amd64 threads the
  same loops (checked by the Docker amd64 sample).

## Alternatives

- Aligning the `br` or the loop top (decision 0075): a fixed point that
  is good for one loop and bad for another; the spread stays.
- A compare chain instead of the table (0051's test run): 54.9 ms
  against the table's 44.5 on interp (2026-10-08); worse again.
- Copying only the latch (`tail_dup`, 4 arms at most): the one `br`
  stays, so the spread stays.
- A per-arm "next" prediction hint: no such instruction on arm64.
