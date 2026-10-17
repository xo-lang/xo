# Joining SSA blocks changed which diamonds the arm64 emitter if-converts

Status: Observed (2026-10-17, ssa.merge_straight)

Symptom: a pass that only joined straight-line SSA blocks after inlining
(ssa.merge_straight) made bench/perf/xo sortpt 1.025x the instructions on
linux/arm64, and text/* 1.006 to 1.018x on macOS, while the code it was
for (bigmod's adc loops) got faster.

Causes, two:

1. A joined block a value lives across a call in: the register allocator
   gave that value a callee saved register, so obj.rs_new_n, a leaf on
   its fast path, got a frame (its slow path's call joined to the shared
   tail). merge_straight joins only blocks that make no call.
2. A compare's three-way result (-1, 0, 1) tested after a join: the join
   of `if c != 0 { c } else { 0 }` became a one-instruction side block,
   so select_branch (compiler/arm64/isel.xo) if-converted it into a csel
   and a compare after the join, where before each side's constant went
   straight to its branch. select_branch now leaves a diamond a branch
   when the join only tests the selected values (join_tests: constants
   and up to three operations on the parameters, then the branch).
3. A `||` (or `&&`) made into a Bool, true on one path, then `if b`:
   the joined shape let select_branch turn `if b { x + 1 }` into a csel
   on the Bool (a mov, a cset, a compare, a csel) where each test went
   straight to its side (bench/perf/text float_fmt 1.004x). select_branch
   now keeps a branch on a parameter of its own block that a predecessor
   passes a constant (cond_from_const).

How to avoid: a pass that changes block shape (joins, splits, forwarding
blocks) changes the emitter's heuristics that look at a block's size or
its neighbours (select_branch's side blocks and join, the reload memo,
regalloc's per-block call crossings). Measure bench/perf/xo on Linux
(callgrind) as well as the macOS sweep, which does not include it.
