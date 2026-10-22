# 0338: LLVM unrolls loops with Float arithmetic, not every loop

Status: Accepted (the lead under the owner's overnight delegation,
2026-10-22, on the build time measurements below). Implementation: Xo
lang 4. Supersedes decision 0333's module wide `-unroll-threshold=300`.

## Context

Decision 0333 passed `-mllvm -unroll-threshold=300` (-O3's value, -O2's
is 150) to every -O2 LLVM build, for nbody (2,308 to 1,160 M
instructions). Measured afterwards on xo's own LLVM build (single module,
full LTO, bench/results/llvm-o2-unroll-2026-10-21.md, 3793e2ab9): user
CPU 1,924 to 2,170 s (+12.8%), __text +1.1%, for a program with no loop
that gains from it. The threshold applies to every loop of the module;
clang has no per function threshold, and a split build compiles modules
whose loops are just as mixed (xo's own build is one module, so a per
module choice would give it 300 everywhere: it has Float loops).

xo's IR has 8,801 natural loops; 17 have Float arithmetic in their body.
nbody's gain is in loops of that kind (the 5 body loops, main.xo:33, 47,
50, 63, 64, 80).

A prototype (an IR rewrite in a clang wrapper, 2026-10-22) marked the
loops with Float arithmetic `!llvm.loop` `llvm.loop.unroll.full` at the
default threshold 150 (instructions, least of 3, the Mac):

| build | nbody | dot/axpy (trip counts unknown) |
| --- | ---: | ---: |
| -O2 (threshold 150) | 2,318 M | 122.5 M |
| -O2, 0333 (threshold 300) | 1,160 M | 125.1 M |
| -O2, the mark | 1,159.8 M | 123.4 M |

`llvm.loop.unroll.enable` instead gave nbody 1,635 M. A marked loop's
size limit is `-pragma-unroll-threshold` (16,384 by default); at 300,
600 and 1,200 nbody measured the same (1,160 M), its text +1.4 KB over
the unmarked build. `unroll.full` does nothing to a loop whose trip
count is unknown (dot/axpy: no regression).

A marked loop's trip count limit is `-pragma-unroll-full-max-iterations`
(1,000,000 by default): the first implementation also marked dot/axpy's
outer loop (20,000 iterations, `t += dot(a, b)`), and clang's compile of
that small program ran over 13 minutes (killed); with the limit at 64 it
took 4.5 s. A 32 iteration loop of 48 Float statements (8
accumulators): with the limits 300 and 32, 2,506 to 1,165 M
instructions and the text 2.4 KB smaller when the body depends on a run
time value; 1,326 to 15 M and the text +62.6 KB when every value is a
constant (the unrolled loop folds).

xo's own build, the prototype against the old -O2 (one paired run each,
bulk slot; bench/results/llvm-o2-unroll-2026-10-21.md): user CPU 2,075
s against 2,336 s, __text 2,395,012 bytes both (0333's: 2,420,796).

## Decision

The LLVM backend marks the back edge of every natural loop with Float
arithmetic (an Add, Sub, Mul or Div of a Float in the loop's blocks,
compiler/ssa cost.xo `float_loop_latches`) with a distinct `!llvm.loop`
node carrying `llvm.loop.unroll.full` (compiler/llvm func.xo
`loop_md`). Every LLVM build above -O0 passes `-mllvm
-pragma-unroll-threshold=300` and `-mllvm
-pragma-unroll-full-max-iterations=32` to clang's compile and to an LTO
link (the
same spellings as 0333: `-Wl,-mllvm,...` to darwin's ld64,
`-Wl,-plugin-opt=...` on Linux, the compile flag alone without LTO;
compiler/xo split.xo `unroll_flags`), so a marked loop unrolls fully
when its trip count is known and at most 32 and its unrolled size is
under 300. Other
loops keep the level's own threshold: 0333's `-unroll-threshold=300` is
gone.

## Consequences

- nbody keeps 0333's speed (1,159.7 M); Int only programs (xo itself,
  the TLS server) build as at -O2 before 0333: xo's text identical in
  size, its build CPU within the machine's noise. collatz, fib, interp,
  sieve, sort and strings within 0.1% of 0333.
- -O1 builds now fully unroll marked loops too (tests build at -O1; the
  output is the same, the IR differs by the metadata). -O3's own
  threshold is already 300.
- The IR of a function with a Float loop changes (the metadata), so its
  clang cache entries change once.
- A loop with Float arithmetic only in a cold path (a fault message
  formatting a Float) is marked too; full unrolling still needs a known
  trip count under the size limit.

## Alternatives

- Keep 0333 (+12.8% CPU on xo's own build, for loops that do not gain).
- 300 only for modules with Float loops: xo's own build is one module
  with Float loops, so nothing changes for it.
- `llvm.loop.unroll.enable`: nbody 1,635 M, half the gain.
- `llvm.loop.unroll.count`: needs a count per loop the emitter does not
  know before inlining.
