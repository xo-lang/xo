# clang spent over 10 minutes on one module in the AArch64 machine combiner

Status: Observed (2026-11-07)

## Symptom

internal/llvm TestGoldenSplit/pass_const_edges/none (split 3, no LTO, -O1)
ran into XO_BUILD_LIMIT on the Mac during tier 59 (1a1ed8eda). The hang report
showed xo's main fiber waiting on clang's stdout. Meanwhile one clang -cc1
(prog.0.ll) ran at full CPU at 1.2 to 2.7 GB for over 10 minutes.

The same module from tier 55's commit (edf1b6e3b) was just as slow: x.main's IR
is identical at both commits after renumbering. So this is not an IR
regression within that range. Tier 55 most likely reused a cached object
(internal/llvm's clang cache keys on the IR), or finished under the limit.

## Cause

pass_const_edges' main is 9,673 basic blocks, with 4,750 conditional
branches, in 44,589 lines of IR. A `sample` of clang put 100% of the time in
AArch64 MachineCombiner::runOnMachineFunction. The pass's time grows faster
than linearly with the size of such a function.

Size in lines is not what matters. xo's own build_native has 95,297 lines in
4,749 blocks, and its largest function by block count has 5,944
(arm64 G.inline_rt). xo's LLVM build compiles both in minutes.

## Fix

A module that holds a function of more than 8,000 basic blocks
(split.xo GIANT_FN_BLOCKS) is compiled with
`-mllvm -aarch64-enable-mcr=false` on arm64. With LTO, the link gets the
option too: `-Wl,-mllvm,...` on darwin and `-Wl,-plugin-opt=...` on Linux.

This applies both to a split build's modules (split_build, per module) and
to a single module build. Programs with no such function compile with the same IR and the
same clang arguments as before.

Result: that module compiled in 85 s, and the split golden ran in 61 s.
Checked by logging clang's arguments: of pass_const_edges' three modules, one
compiles with the option (at -O1 and -O2); bench/perf/json and cbor get
none, and xo's own IR has no function over 5,944 blocks.

Watch for amd64 and other machine passes: a giant generated function
(an SDK module's) on x86 may need the same treatment.
