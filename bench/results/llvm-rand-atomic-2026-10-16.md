# LLVM testworld rand and atomic get against pre-zero-C main (2026-10-16)

The perf resweep (zero-c-perf-sweep-2026-10-15.md, main 40c1d729 against
pre-zero-C main dd33dd13) left two LLVM rows above the 5% bar with no
profile: testworld rand 1.081 and atomic get 1.058. Both are register
allocation in a function the runtime's Xo code now shares, not the cost
of the operations themselves.

Method: instructions retired (`/usr/bin/time -l`), best of 5, on the
development Mac, LLVM at -O2 (XO_LLVM_OPT=O2 for the test binaries, as
the resweep's rerun), bench/perf/testworld's measure.sh and
bench/perf/atomic built by each xo: base is dd33dd13's xo, main is
c5b39e8d's.

## What the extra instructions were

- **atomic get** (2M iterations, 1.98M more instructions: one per
  iteration). The loop is inlined into `xo_entry` with every other
  workload. Base kept the Atomic's address in x22; main spills it and
  reloads it each iteration (`ldr x9, [sp, #0x28]` before the `ldar`).
  Main's `xo_entry` is 2,349 instructions against base's 1,314: LTO now
  inlines the runtime the C used to keep opaque (the pool lock's slow
  path, the futex wake, scope and spawn paths), and the hot loop lost its
  register.
- **testworld rand** (300,000 iterations, about 10 more instructions
  each). The Xo of the rand path is identical on both sides (testworld.xo
  was already Xo at dd33dd13), but every `test.N` function has one call
  site and LLVM inlined all seven tests into the test binary's main.
  There the PCG step's four 64 bit constants were rebuilt in the loop
  each iteration (`mov` and three `movk` each) where base kept them in
  x19, x23, and x24.

## Fix (selfhost/llvm func.xo, gen.xo)

1. The runtime's slow paths are `noinline cold` (attribute #10, by
   name: pool.lock_slow, pool.futex_wake, budget.tick_slow,
   mutex.held_add_slow, obj.reserve_slow, obj.push_slow,
   obj.rl_free_n_slow, stdio.out_slow).
2. A test block's function (`test.N`) is `noinline` (#11): each test
   is compiled on its own, not inside a main shared with every other
   test.

| row | base | main | fixed | fixed / base |
|---|---|---|---|---|
| atomic none | 13.86M | 13.36M | 13.54M | 0.977 |
| atomic get | 27.85M | 29.34M | 27.74M | 0.996 |
| testworld none (0) | 14.08M | 13.85M | 13.61M | 0.967 |
| testworld env (2) | 2744.3M | 2744.8M | 2689.8M | 0.980 |
| testworld rand (3) | 36.42M | 39.14M | 37.44M | 1.028 |

The fixed column is the committed change's build (testworld env from
the sweep below, the same build).

Steps measured on the way:
the slow paths alone fixed atomic get (27.39M) and left rand (39.16M);
dropping `alwaysinline` from string concatenation in blocks that raise a
fault (rand.next_int's message) changed nothing (39.25M) and was
reverted; the test functions out of line brought rand to 37.43M. Without
pool.futex_wake in the list rand read 38.04M (1.044), so it stays.

## The rest of bench/perf on LLVM

main against the fix, 14 programs' measure.sh (atomic ctx digest fault
fsproc gen glob httpparse json portable report text unicode testworld),
best of 5 each: every workload within 0.95 to 1.04. The largest
increase, ctx spawn 1.036 (71.5M to 74.1M), is run to run noise: main
alone read 71.5M and 73.7M in two runs, and the second comparison was
73.7M against 73.5M. The arm64 backend does not use these attributes.
