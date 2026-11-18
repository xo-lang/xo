# 0716: sizing padded functions without their fault blocks

Status: Accepted (Xo Lang 2, 2026-11-18; task #271). Amends decision
0256 (which functions with landing pads the arm64 inliner takes).

## Context

Decision 0256 lets the arm64 and amd64 inliner copy a callee's landing
pads, but sizes a padded callee by all of its blocks, and inlines one
with more than one call site only when it is tiny. A small function that
can fault carries cleanup blocks that release what it holds and end in
`rt.fault_continue`: they never run unless something faults, yet they
counted toward INLINE_MAX_INSTRS. std/brotli's decoder reads every symbol
through `peek_bits` and `drop_bits` (a `var d: Dec` and a checked
refill), so both stayed out of line at every site: text decode was 1.72
s on the Mac for 5000 decodes, about 1.2 of Go.

## Decision

1. fn_sum (compiler/arm64/inlsum.xo) leaves the fault blocks out of a
   function's size: a block ending in `rt.fault_continue`, the blocks
   only it reaches, and blocks whose every successor is one of them.
2. A padded function whose remaining size fits INLINE_MAX_INSTRS and
   INLINE_MAX_BLOCKS is inlined at up to INLINE_PADDED_SITES (8) call
   sites, when it is not hard, not bad, does not update in place, and
   has no `coldrel`.
3. `coldrel` (a FnSum flag, inline sum v5): a fault block releases a
   field or payload (at any depth) of an entry parameter that is not
   `var`. Inlined where the argument is borrowed, such a release moves
   out of a value the caller does not hold (verification failed in
   format.Printer.use_decl inlining effect_name: "moves out of v253.2,
   which is not held"). `var` parameters are owned by the callee, so
   they are exempt.
4. The growth is bounded by what it takes: a padded callee whose hot
   part (its blocks other than the fault blocks) has at most
   INLINE_MAX_INSTRS (40) SSA instructions and INLINE_MAX_BLOCKS (12)
   blocks, called
   from at most 8 sites in the program; a callee with more sites stays
   out of line everywhere. The fault blocks are copied with it (they are
   the landing pads 0256 copies), so each site grows by the whole
   callee; the site cap is what keeps that in check (without it xo's
   own `__text` grew 6.3%).
5. Only the arm64 and amd64 native backends use this inliner; LLVM does
   its own inlining and is unchanged.

## Consequences

- bench/perf/brotli text decode, Mac: 1.72 to 1.51 s (0.875); mixed
  0.795 to 0.688 s (0.864). Host 1 `xo bench --ab` text: wall 0.909
  (0.884 to 0.909, 3 of 3 rounds better). Callgrind (linux/arm64 in Docker on
  the Mac) text: 1,777,580,720 instructions to 1,530,383,075 (0.861),
  the same output.
- Code size, `__text` on darwin/arm64 (native; LLVM column is the
  brotli source change only):

  | Program | native base | native new | change | LLVM change |
  |---|---|---|---|---|
  | xo | 37,923,500 | 38,982,692 | +2.8% | |
  | bench/perf/brotli | 877,740 | 911,524 | +3.85% | +0.06% |
  | bench/perf/metrics | 550,984 | 556,184 | +0.94% | +0.06% |
  | bench/perf/json | 487,668 | 488,812 | +0.23% | +0.06% |
- xo builds and verifies itself under XO_INLINE_STRICT.
- test/codegen/faultexit pins it: `peek` and `drop_n`, small padded
  functions with `var` parameters and a fault path, have no calls left
  in `sym` (fails before this change).
- std/brotli's peek_bits and drop_bits keep their slow paths
  (fill_bits, drop_bits_more) out of line, so the inlined part is the
  fast path only.

## Alternatives

- No site cap: +6.3% `__text` for xo, no further brotli gain.
- Rejecting every callee whose fault blocks release a parameter: kept
  drop_bits out (its `var d` releases); the `var` exemption is what
  makes the brotli case work.
- Marking the brotli helpers by hand (an inline attribute): Xo has none,
  and the same shape (a small reader with a checked refill) recurs in
  other decoders.
