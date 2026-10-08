# 0051: Native arm64 `match` dispatch: test runs, jump tables for large matches

Status: Accepted (2026-10-08). Implemented in `internal/arm64/jumptab.go`.
Evidence: `bench/results/arm64-perf-2026-10-08.md`.

## Context
The SSA IR (decision 0045) lowers a `match` on an enum tag or an Int to a
chain of blocks, each comparing the same register with one constant.
The native arm64 backend (decision 0049) emitted the chain in reverse
postorder, which puts each arm's code between two tests: every test that
fails is a taken branch. On the interp benchmark (a ten arm `match` in a
stack machine loop) the native code ran at 1.23x of the LLVM backend.
Decision 0049 listed jump tables for `match` as the first gap to close.

Measured on an Apple M4 Pro (interp, and a synthetic `match` of 3 to 16
arms over random tags and over a repeated tag list):

| interp, median ms | |
|---|---|
| chain in reverse postorder (before) | 80.7 |
| chain laid out as a run of tests | 54.9 |
| jump table (`adr`, `ldrsw`, `add`, `br`) | 44.5 |
| branch table (`br` into a run of `b`) | 59.4 |
| LLVM backend | 65.1 |

On random tags the jump table lost to the test run at every size tried
(4 arms: 342 against 262 ms; 8: 384 against 328; 16: 403 against 374),
and on a repeated 4096 entry tag list it lost by up to 3x (the
conditional predictor learns the sequence, the indirect one does not).
The table's branch resolves after a dependent load, so a mispredict costs
more. The interpreter's dispatch is the opposite case: a repeating
sequence the indirect predictor learns and the chain spreads over too
many conditional branches.

## Decision
- Every compare chain (blocks holding nothing but `cmp r, #k` with
  `b.eq`/`b.ne` on the same register) is laid out as a contiguous run of
  tests: a branch is taken only to the arm that matches.
- A chain that tests at least 8 distinct values (`jumpTableMin`), with a
  table of at most 3 entries per value (`jumpTableSlack`) and values in
  0 to 4095, becomes an `iSwitch`: bounds check against the default,
  `adr`/`ldrsw`/`add`/`br` through a table of 32 bit offsets placed after
  the function's fault stubs.
- `XO_ARM64_JT_MIN` overrides the threshold (0 disables tables) for
  measurement; it is not a supported setting.

## Consequences
- interp: 1.23x to 0.70x of the LLVM backend (LLVM itself lowers this
  `match` to a binary search tree, not a table).
- Matches of 2 to 7 arms keep chains (now contiguous), which are as fast
  as before or faster and never slower on unpredictable tags.
- A large `match` over unpredictable data pays 7 to 15 percent against a
  test run; accepted, since large matches are mostly dispatch loops.
- Linux uses the same text (local labels, `.long` differences within
  `.text`).

## Alternatives
- **Tables from 4 arms (LLVM's default)**: lost 25 to 30 percent on
  random tags at 4 to 6 arms in our measurements.
- **Tables in `__TEXT,__const`** (as LLVM places them): no difference in
  time (444 ms both on the 10 arm random test).
- **Branch tables** (no load): better on random tags at 16 arms, worse on
  interp (59 against 45 ms).
- **Binary search trees** (LLVM's choice for this `match`): 61 ms on
  interp by hand, between the run of tests and the table.
- **Profile guided choice**: no profiles exist yet; revisit with them.
