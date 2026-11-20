# Inlined fault messages made a hot loop spill

Status: Observed 2026-11-19 (task #450).

## Symptom

std/brotli's update_nodes on (private commit) (BytesBuilder `.at` inlined in
its j loop) ran 6,934,579,946 instructions for enc11 against main's
6,083,508,765 (callgrind, linux/arm64). Its j loop reloaded almost every
value from the stack each iteration, and the function's asm doubled
(6,394 lines against 3,083).

## Cause

Two things, both from the bounds messages the inlined `.at` builds on
its fault path (calls to rs_with_cap and rs_concat):

1. The message blocks were laid out and allocated as hot code, so their
   calls made every value live across them take a callee-saved register
   or a spill.
2. loop_split (decision 0724) skips a function over SPLIT_MAX_INSTS
   (2000) instructions, and counted the cold message blocks: the
   function was over the limit by its fault paths alone and the loop was
   never split. A split loop is also limited to MAX_SPLIT_EDGES (8)
   exits, which the 17 fault exits alone passed.

## Fix

- The fault_cold pass (compiler/arm64/isel.xo, `fault_only`) marks
  blocks only on the way to a fault cold: laid out after the epilogue,
  weighted as cold by the allocator.
- loop_split counts only hot instructions against SPLIT_MAX_INSTS, and
  exits to fault paths against their own limit (MAX_SPLIT_FAULT_EDGES).
- Tests: test/codegen/loops `probes`, test/codegen/loopsplit
  `work_rare_big`.

Results: enc11 6,934,579,946 to 6,146,953,463 on (private commit) (-11.36%,
cgab l450br3 and l450br5).

## Avoid

A size or count limit on a pass should count the code the pass works on
(hot blocks), not code another pass has moved out of the way.
