# Shrink wrapping and tail calls in selfhost/arm64 (decision 0224), 2026-10-14

B13 (string-list-objects-2026-10-14.md, stage 2) and B14
(test-world-2026-10-14.md) missed decision 0180's bar on arm64 only;
B13 traced it to the arm64 emitter saving callee saved registers on
every path. Decision 0224 adds shrink wrapping (the frame made only on
the paths that need it) and tail calls (a call whose results are
returned as they are is a branch after the epilogue) to selfhost/arm64
(shrink.xo). This file holds the measurements.

Method: the Mac (12 cores, other agents running, under the heavy lock),
`/usr/bin/time -l` instructions retired, minimum of 5 runs per workload.
Base: main at a58401a7, its xo. New: commit d458703d (the change on
a58401a7).

## Generated code (fib, internal/arm64 TestGoldenAsm)

`fib` returns n below 2 with no frame (`cmp x0, #2; b.ge; ret`); the
frame record and x19, x20 are saved after the branch. A function whose
only call returns its result (`prelude.llrt_concat_int`, the drop at the
end of `main`) is a branch: `b _xo_f_num_2eappend__int`. The goldens
internal/arm64/testdata/{fib,sieve}.{darwin,linux}.s were rewritten
deliberately with `TestGoldenAsm -update` for this change.

## The self build of xo (compiler instructions)

As fast-builds-2026-10-13.md measures the run time of the compiler's
own code: A is xo built from this tree by the base xo (its code has no
shrink wrapping or tail calls), B is A building this tree (its code has
both). Each builds selfhost/xo twice, `XO_EMIT_JOBS=8`, a fresh
`XO_BUILD_DIR`, `XO_BIN_CACHE=off`.

| | run 1 | run 2 |
|---|---|---|
| A (instructions) | 1,274,333,824,814 | 1,274,791,117,115 |
| B (instructions) | 1,255,213,299,694 | 1,254,991,751,918 |
| B / A | 0.985 | 0.984 |

The outputs of A and B are byte identical (51,947,936 bytes): the fixed
point s2 = s3 on the Mac.

## bench/perf on arm64 (base xo against the new xo, same std)

New over base: alloc 0.963 (small) to 1.003 (mixed); nested 0.991
(walk) to 1.003 (mapput); text 0.979 (lines) to 1.001 (debug); json
0.988 (ints) to 0.999; xo's 18 workloads 0.998 to 1.004 (iface). No
workload regresses by more than 0.4 percent; alloc small (0.963), text
split and lines (0.980, 0.979), and json ints (0.988) gain from the
runtime's Xo heap (alloc.alloc's calls of rt.heap_large and
rt.heap_grow are tail calls, so it has no frame).

## B13's stage 2 on top of the change (std/native/llrt/obj.xo)

B13's method: one xo, two std trees through `XO_STD`, A at 5733ae10 (the
C list objects) and B at 3da29c8b (the Xo list objects). The xo is
3da29c8b's (no change) or 3da29c8b with this change cherry-picked
(scratch worktree; the branches are not merged). B / A:

| Workload | B13's xo | with shrink wrapping |
|---|---|---|
| alloc small | 1.242 | 1.201 |
| alloc strings | 1.043 | 1.022 |
| alloc maps | 1.046 | 1.025 |
| nested walk | 1.049 | 1.036 |
| nested shared | 1.004 | 1.004 |
| text split | 1.086 | 1.058 |
| text lines | 1.059 | 1.032 |
| text int_fmt | 1.042 | 1.023 |

(B13's own figures for version 2 were linux/arm64 `perf stat`: 1.266,
1.115, 1.098 for alloc small, nested walk, text split.) obj_rl_free's
path with no drop now has no frame and ends in a branch to alloc.free;
the loop over the drops makes the frame before it and undoes it after.
Still over 1.05: alloc small, text split. What remains is the levels
(B13's next experiment 3): `xo_rl_new` in C calls the export, which
calls obj.rl_new, which keeps the capacity across its call of
alloc.alloc in x19 and so has a frame; clang inlines the C's levels.

## B14 on top of the change (bench/perf/testworld, the Mac)

`measure.sh` of the B14 branch (56971145): base main a58401a7, B14 the
branch; with the change, both compilers have it (main + d458703d, and
the B14 branch with this change cherry-picked). New / base:

| Test | without | with shrink wrapping |
|---|---|---|
| stdio | 1.008 | 1.008 |
| env | 0.632 | 0.632 |
| rand | 1.099 | 1.100 |
| log | 0.401 | 0.406 |
| gen | 1.093 | 1.094 |
| memfs | 0.998 | 0.999 |

These are the first Mac figures for B14: stdio meets the bar on the Mac
(1.008; its linux/arm64 1.178 is the outline atomics, decision 0200).
rand and gen are unchanged: their hot functions call on every path and
have landing pads (rand.next_int calls ints.u64_rem and testworld.pcg,
neither inlined), so there is no frameless path to wrap.

## Next experiments (ranked)

1. B14 rand and gen: inline testworld.pcg and the remainder in
   rand.next_int (selfhost/arm64/inline.xo's limits), and fold the
   constant switch chain the asm shows in next_int (`mov x9, #1; cmp x9,
   #2` sequences: a constant compared after a K_PMOVE).
2. B13 alloc small: give generated code `xo_x_obj_rl_new` directly (the
   C level gone) and let alloc.alloc inline into rl_new, or keep
   rl_new's capacity in a caller saved register by recomputing it after
   the call (it is stored from the argument).
3. Shrink wrapping with landing pads (excluded now): a pad's block needs
   the frame; its region could include the pads' blocks.

## Verification (linux/arm64, EC2 queue, commit a7f05222 after merging main)

Warm r8g (i-EXAMPLE06), seed 0.10.0-23ce8ce2: `xo dev goldens
--force` 182 of 182 on arm64 and 182 of 182 on LLVM, internal/arm64
TestGoldenAsm. Warm r8g.8xlarge (i-EXAMPLE01): go vet and
`go test -short` clean, scripts/bootstrap-xo.sh: s2 and s3 assembly
identical (297,336,303 bytes) and binaries identical (53,485,152 bytes).
On the Mac: the 29 fault, unwind, contract, and task goldens on arm64
(`--only`, darwin's compact unwind) pass.
