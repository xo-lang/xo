# 0192: LLVM value ranges, told facts, and the arm64 inlining model

Status: Accepted (2026-10-12), implemented in `internal/llvm` and its
Xo twin `selfhost/llvm` (byte identical IR, decision 0166):
`ranges.go` / `ranges.xo` (value ranges, facts, `llvm.assume`),
`inline.go` / `ranges.xo` (the inlinable set), `fromssa.go` /
`func.xo`, `call.xo`, `gen.xo` (unchecked arithmetic, call site
`alwaysinline`, the declarations). Extends decision 0184 (the arm64
cost model and range analysis) to the LLVM backend; answers the LLVM
half of decision 0180 item 7(b). Tests: golden `range_facts` (Go, LLVM,
arm64), `TestRangeFacts` (the IR: checks removed and kept, assumes,
`#6` calls), TestSelfhostLLVM (`XO_SELFHOST_DIRS` added). Evidence:
`bench/results/runtime-pilot-2026-10-12.md`, section "LLVM value ranges
and inlining (decision 0192)".

## Context

Decision 0180 item 7(b) asks for `bench/perf/unicode` within 5 percent
of the C path (`XO_UNICODE=c`) in instructions on every workload on both
native backends before more of the C runtime is ported. Decision 0184
met it on arm64. On LLVM the lower compare (`eqlower`) stayed at 1.126x
the C path (11.47 billion instructions against 12.92 billion).

The C path and the Xo path both go through clang with full LTO (the C
runtime is compiled as bitcode and linked with the program's IR), so the
gap was not a missing cross language inlining. Comparing the hot loops'
IR and assembly showed two causes:

- **Checks LLVM cannot prove.** The binary searches of
  `std/native/llrt/unicode.xo` (`to_case`, `in_ranges`) keep the
  overflow checks of `hi - lo`, `lo + (hi - lo) / 2`, `m * 12`, `m + 1`
  and the bounds checks of the table reads, and the UTF-8 decoder keeps
  those of `i + k`, `i + 1`, `j + 1`. LLVM's value tracking does not
  iterate over loop phis (it never learns that `lo` and `hi` stay in
  [0, 328]), knows nothing of the relation `lo < hi` that bounds a
  midpoint, and does not know that a Str is at most 2^48 bytes. With
  every check of the module removed (an experiment), lower_u went from
  5.08 to 4.04 billion instructions and eqlower from 12.92 to 10.83.
- **Calls LLVM does not inline.** `decode` and `to_case` (180 and 241
  SSA instructions with their inlined helpers) are called from two
  loops each; LLVM's inliner (a threshold of about 225 at -O2) kept
  them as calls in `case_append`, the loop of every case mapping. The C
  path's `n_case_into` has decode, case mapping, and encode inlined.

## Decision

1. **Value ranges** (`ranges.go`), the arm64 analysis of decision 0184
   (`internal/arm64/ranges.go`) for the LLVM backend: an interval per
   Int value, iterated in reverse postorder to a fixed point with
   widening of loop parameters; midpoints `a + (b - a) / 2` (or `>> 1`)
   computed where `a < b` holds are in `[a.lo, b.hi - 1]`; a wrapping
   operation (`ssa.Wrap`) knows nothing.
2. **Facts where an operation is computed.** The operands of each
   operation are narrowed by the facts that dominate it: the comparison
   of the branch whose edge is the only way into a dominating block
   (Int, not UInt64); through the join of `a && b` or `a || b`, the
   comparison of the one predecessor that can take the edge and that
   predecessor's facts; and an index a dominating `b[i]` or `xs[i]` took
   (a passed bounds check: `0 <= i < 2^48` for Bytes and Str, decision
   0184's bound, `i < 2^63 - 1` for lists). Facts form a persistent list
   per block (its own, then its immediate dominator's); narrowing looks
   at the nearest 32.
3. **Unchecked arithmetic.** A checked Int add, subtract, or multiply
   whose narrowed operand intervals prove it cannot overflow is lowered
   as `add nsw` (`sub`, `mul`) without the check.
4. **Facts told to LLVM.** The interval of every midpoint is emitted as
   `llvm.assume` (`icmp ule` of the value minus the lower bound, or one
   signed compare for a half open interval) right after it: after LLVM
   inlines a table read (`u24`) into the search, its own passes (value
   tracking, correlated value propagation) drop the bounds checks the
   told interval proves. Nothing else is told: assumes on loop
   parameters gained nothing here and made the sieve 1.6 times slower
   (Alternatives).
5. **Inlining.** A call of a function the arm64 cost model of decision
   0184 finds inlinable (cost at most 40 instructions and 12 blocks, or
   at most 2 call sites with sites times cost at most 500 and at most 80
   blocks; never main, never a function with landing pads of its own,
   never a cycle) carries the call site attribute `alwaysinline`
   (attribute group `#6`). The same constants as arm64: one model for
   both native backends.

## Consequences

- `bench/perf/unicode` on LLVM is within 5 percent of the C path on
  every workload (decision 0180 item 7(b)), below it in fact: lower
  0.663x, upper 0.624x, lower_u 0.819x, upper_u 0.815x, eqlower 0.948x
  (1.126x before), classify 0.850x, runecase 0.967x (1.023x before),
  quote 0.725x. With decision 0184's arm64 result (eqlower 1.015x and
  classify 1.023x the worst), **7(b) is met on both native backends**.
- Bench programs (`bench/perf/llvm/xo`): no change beyond noise but
  sort, 4.6 percent fewer instructions; binaries the same size.
- More functions are inlined on LLVM (the arm64 set, which includes
  functions LLVM would not inline: few call sites, larger), so code of
  programs with such functions grows as on arm64 (decision 0184: xo1's
  assembly 8.5 percent); not measured on a large program here.
- Every function is analysed: a fixed point over its Int values with
  at most 32 facts looked at per operand, as arm64 already does; the
  Xo twin does the same, so the self hosted emitter's IR stays byte
  identical (TestSelfhostLLVM: 297 runs, 10,498 functions identical).
- The facts rely on decision 0184's bound of a Str or Bytes length
  (2^48) and on unchecked SSA arithmetic other than `ssa.Wrap` not
  overflowing (gotchas/llvm-nsw-hash-overflow.md).
- Next steps, ranked: (1) a copy budget of 600 for both backends
  (`prelude.rune_case` becomes inlinable: LLVM runecase 188 million
  instead of 247 million instructions; arm64 to be measured) with the
  bench programs and xo1's self compile as the check; (2) drop the
  bounds checks the facts prove in the LLVM lowering itself
  (`bytes.index` and `list.index` under `i < len` facts, decision
  0184's relational rule), for code where the table is not a constant
  LLVM can see; (3) port xo_std.c's number and duration text next, per
  decision 0180 item 7, now that (b) holds.

## Alternatives

- **Only the facts, LLVM's inliner as it is** (an intermediate version
  with loop parameter assumes, scratch runs): lower_u 4.54, eqlower
  11.89 billion instructions (0.92x and 1.04x of the C path), runecase
  222 million: eqlower within the bar by 1.4 points only; decode and
  to_case stay calls.
- **Telling LLVM the interval of every loop parameter**: nothing gained
  on the unicode workloads, and `bench/perf/llvm/xo` sieve went from
  2.09 to 3.37 billion instructions (LLVM unrolled its inner loop with a
  trip count computed in 128 bits by `__udivti3`;
  gotchas/llvm-loop-phi-ranges.md).
- **Only the inlining** (`alwaysinline` on decode and to_case, no
  facts): lower_u 4.63 and eqlower 12.00 billion (0.93x, 1.046x), and
  runecase worse (264 million).
- **`inlinehint` on the few call site functions**: LLVM inlines the
  Rune case chain into the loop (runecase 0.75x) but still not decode
  and to_case into `case_append` (lower_u unchanged).
- **A copy budget of 600** instead of 500 for the few call sites rule:
  `prelude.rune_case` (264 instructions, 2 call sites) becomes
  inlinable, runecase 188 million (0.75x) instead of 247 million, the
  rest unchanged. Not taken here: the constant is decision 0184's for
  both backends; changing it is a joint measurement (arm64's runecase
  is the same 247 million).
- **Proving the bounds checks in the backend** (decision 0184's
  relational `x < len` facts): the table reads are in `u24`, a separate
  function until LLVM inlines it, so the facts must reach LLVM anyway;
  the assumes do that.
- **Changing unicode.xo**: out of scope; the backend change helps every
  program.
