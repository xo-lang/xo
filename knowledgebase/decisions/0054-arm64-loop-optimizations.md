# 0054: Native arm64 loop optimizations: pointer induction variables, value ranges, rotation, latch layout

Status: Accepted (2026-10-08); extended by 0060. Implemented in `internal/arm64/lsr.go` and
`internal/arm64/ranges.go` (hooks in `isel.go`, `opt.go`, `emit.go`).
Evidence: `bench/results/arm64-perf-2026-10-08.md` (section "Loops,
2026-10-08").

## Context
After decision 0051 the native arm64 backend (decision 0049) ran sort at
1.09x of the LLVM backend and retired 28 percent more instructions
(1756M against 1375M). The quicksort scan loops recomputed `xs + 24 * i`
(two instructions) and checked `i + 1` for overflow on every iteration;
the generator loop's seed chain was a checked `mul`, a checked `adds`,
and four instructions of signed `% 2^31`; and the scan loops had two
taken branches per iteration. sieve (1.07x in quiet runs) spent two taken
branches per iteration of its outer loop. All changes stay inside
`internal/arm64`; `internal/ssa` is unchanged.

## Decision
- **Pointer induction variables** (`planLSR`, `lsrMoves`): a loop header
  Int parameter that every back edge passes as `i + c` (c constant), used
  as the index of `list.index`/`list.set` in a block that runs on every
  iteration, of a list whose buffer cannot move in the loop (defined
  outside it, or a header parameter every back edge passes back through
  `list.set` and in loop parameters only), gets a second header
  parameter `p = xs + i * size`: entry edges compute it, back edges add
  `c * size`. Only for element sizes that are not a power of two (a power
  of two is one `add ..., lsl #k` already). The element address memo of
  the header starts with p, so every block the header dominates
  addresses `[p, #24 + 8k]`.
- **Index facts**: a passed bounds check bounds its index to
  `0 .. MaxInt64 - 1`; an induction variable that only grows is at least
  its smallest initial value (only shrinks: at most the largest);
  `i + c` gets i's bounds shifted and is addressed from i's pointer
  (`offsetBase`). These remove overflow checks of `i + 1`, `i - 1`.
- **Value ranges** (`ranges.go`): an interval per Int SSA value, iterated
  over the blocks to a fixed point; a growing loop parameter is widened
  after two rounds to the next threshold (the bounds of the function's
  `%` and `&` constants, then the Int limits). Checked operations are
  clipped to Int (a result outside faults). `bounds` intersects the
  interval with the memo's branch facts. Uses: a multiplication that
  cannot overflow is unchecked, `x % 2^k` and `x / 2^k` of a non negative
  x are `and` and `asr`, and safeAdd sees the intervals.
- **Rotation at a bounds check** (`rotateJump`): a jump back to a block
  that starts with a check (`cmp` with a fault exit) emits the check and
  branches past it (`b.lo L_c`, then `b L` to fault on the original).
- **Load pairs** (`pairLoads`): two adjacent `ldr` of words 8 bytes apart
  through one base into integer registers become one `ldp` (a text
  peephole per function). Not for `d` registers.
- **Latch layout** (`placeLatches`): when a loop block branches either to
  the latch (directly or through small jump blocks) or into a path that
  contains an inner loop, the latch is laid out right after the branch,
  unless the branch is the test of the inner loop itself.

## Consequences
- sort: 1.09x to parity with LLVM in cycles (890 to 864M against 862M;
  instructions 1756M to 1428M). sieve: 1.07x to 1.03x (452 to 410M cycles
  against 398M). Other programs unchanged within noise.
- Bounds checks are all kept: proving `0 <= i < len` from the loop and
  dropping the check was implemented and measured (commit 328ff04) and
  removed: sort gained 0.4 percent, nbody lost 1.4 percent at every
  alignment of its hot function.
- The range analysis is per function and ignores branches; facts from
  branches still come from the memos.
- Golden assembly gains `_c` labels after the first check of a block.

## Alternatives
- **Loop invariant element addresses** (an index the loop does not
  change addressed from a pointer computed before the loop): nbody 0.9
  percent slower, sort unchanged; removed with the bounds check removal.
- **Post-indexed loads** (`ldr x, [p], #8`): not applicable to the bench
  loops (the pointer is used after the load, by the swap); not built.
- **Register offset addressing** for `List[Bool]` (`strb wzr, [b, j]`
  with `b = xs + 24` hoisted): sieve by hand 1 percent slower.
- **Shrink wrapping and tail calls** (quicksort by hand): 1 percent each
  on sort, not additive with rotation and pairs; not built.
- **Changing `internal/ssa`** (a range analysis or strength reduction
  for every backend): out of scope here; the LLVM backend gets both from
  clang.
