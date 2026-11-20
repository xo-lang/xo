# 0724: arm64 loop splitting from a first allocation

Status: Accepted (xo-lang-1, 2026-11-19, task #426)

## Context

The arm64 register allocator is greedy: each value gets one location for
its whole life, with no live range splitting. A value defined before a
hot inner loop and used after it, in a function whose calls force many
values into callee saved registers or slots, often lands in a slot for
the whole function, so the loop reloads it on every iteration (brotli's
`rb_match` and `create_backward_references` showed `ldr x, [sp, #N]`
before the loop compare). A naive split of every inner loop (rename
every value live through it) cost 0.24% more instructions on brotli
enc11: it renamed values that were fine in a register and added moves on
the loop edges.

## Decision

A default on pass `loop_split` (compiler/arm64/loopsplit.xo; off with
`XO_PASSES=-loop_split`) splits in two phases:

1. In a function with a non tail call, at most `SPLIT_MAX_INSTS` (2000)
   instructions in hot blocks (cold blocks, such as fault messages under
   fault_cold, do not count; 2026-11-19, task #450) and an innermost loop with no call, switch, entry or
   return, it allocates once as usual and keeps the locations. A loop
   with such a block counts as the blocks its header reaches without
   one, when they hold a cycle back to the header and no other loop's
   header (2026-11-19, task #438): a rare call, and an inner loop only
   reached past it, leave the common path split.
2. For up to 16 such loops with at most 8 edges in or out (not
   counting exits to fault paths, of which at most 48; task #450) and at least
   11 general registers live in it, if a value the loop names had no
   register in the first allocation, every value named in the loop and
   live across its edges (not wide, not live into a landing pad of a
   faulting point in the loop, and either named in the loop or in a
   register in the first allocation) gets a new name inside the loop;
   edge blocks copy between the names with a parallel move flagged
   `IF_SPLIT`, which the allocator takes as a hint to give both names
   the same register.
3. If any loop was split, the function is allocated again; otherwise
   the first allocation is kept as is.

Liveness is computed once per function and reused across its loops
(innermost loops are disjoint, so a split changes only its own loop and
edges).

## Consequences

- bench/perf/brotli enc11 0.19% fewer instructions (callgrind,
  linux/arm64), enc6 unchanged (+0.007%); `xo bench --ab` on host 1
  within noise (1.000, 0.991 to 1.004; a version without the size cap
  measured cpu 0.965 against an older base). Outputs are identical.
- 67 of brotli's 1955 functions change. Building compiler/xo takes the
  same time (emit phase within noise with the size cap; without it the
  second allocation of the largest functions made emit 50% to 100%
  longer).
- test/codegen/loopsplit checks that the loop no longer reloads its
  bound from the stack; it fails with `-loop_split`.
- Splitting the call free part of a loop (task #438, 2026-11-19):
  bench/perf/brotli enc11 6,499,614,129 to 6,209,902,866 instructions
  (4.46% fewer, callgrind, linux/arm64); update_nodes' j loop, which
  calls rb_match on about 1 in 30 iterations and runs an inner loop
  after it, had 14 stack loads and stores per iteration and has 1.
  Outputs are identical. test/codegen/loopsplit work_rare fails
  without it. Renaming the read only values a pad reads as well (the
  outside name living on through the loop for the pad) measured 0.4%
  worse than leaving them and was dropped.
- The second allocation is kept only when every instruction still finds
  registers to reload its spilled operands into (split_scratch_ok,
  2026-11-19): split names held across a loop left a madd of three
  spilled names in xo.mlog.vals no free register, and compiler/xo did
  not build; such a function keeps its first allocation.
- Landing pads still read their live ins in one location, so values
  live into a pad are never renamed; the call blocks of a loop stay
  outside the split part, the loop's other blocks are split.

## Alternatives

- Split every inner loop: more moves than reloads saved (+0.24%).
- Real live range splitting in the allocator (second chance binpacking
  or splitting at spill time): the right long term fix, much larger.
- Raise the size cap: the bigger functions' second allocation dominated
  the build's longest function.
