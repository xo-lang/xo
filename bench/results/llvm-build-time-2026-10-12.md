# LLVM build time of selfhost/ssa, 2026-10-11 and 2026-10-12

Decision 0168. Apple M4 Pro, 12 cores, 24 GB, Apple clang 21.0.0
(clang-2100.1.1.101), macOS SDK of Xcode. Every timing ran under the
machine-wide heavy lock (`scratchpad/agents/heavy.sh`), which had two
slots during these runs: another agent's heavy job ran beside each
measurement, so wall times carry that noise (about 10 percent between
rounds of the same binary below). Peak memory is `/usr/bin/time -l`'s
maximum resident set size.

## The program

`xo build --backend=llvm selfhost/ssa` at commit 2cdbb29 (main of
2026-10-11) with a clang wrapper that kept the IR and failed the link:

- IR: 380 MB, 7,928,966 lines, 2,569 functions (1,655 program
  functions and `xo_entry`, 914 helpers `xo.dup.*`/`xo.drop.*`/
  `xo.eq.*`/... of 14,023 lines in all), 8,720 constants. The largest
  function, `Builder.runner`, is 127,552 lines (1.6 percent); the next
  are `Builder.std_call` 121,140 and `Builder.call_as` 96,256.
- Everything before clang (check, SSA, RC, IR generation, writing the
  files, one runtime object compile): 37.4 s wall, 3.4 GB peak.

## Single module (today's build and variants)

The captured `prog.ll` compiled by `clang <opt> [lto] -c prog.ll`, then
linked with the runtime (`scratchpad/llvmsplit/single.sh`).

| configuration | compile | link | total | peak memory | binary |
|---|---|---|---|---|---|
| -O2, full LTO (today) | 372 s | 2,023 s | 2,394 s | 6.8 GB | 51 MB |
| -O2, no LTO | 1,589 s | 0.2 s | 1,589 s | 9.0 GB | 64 MB |
| -O1, no LTO | 1,576 s | 0.3 s | 1,576 s | 9.8 GB | |
| -O0, no LTO | 207 s | 0.3 s | 207 s | 4.4 GB | 280 MB |

With full LTO the compile step only runs the pre-link pipeline; the
link does the optimization and code generation of the whole program on
one core (1,927 s user of 2,023 s). -O1 costs as much as -O2 on this
IR. -O1 with full LTO and -O2 with ThinLTO on one module were not
measured this time (stopped on the lead's request to finish); an
informal run on 2026-10-11 (not recorded elsewhere) put one module
with ThinLTO at 2,742 s, slower than full LTO, as one module gives
ThinLTO nothing to run in parallel.

## Split into modules (decision 0168)

`llvm.Split` on the same IR (0.5 s), the modules compiled by parallel
clangs (`xargs -P jobs`), then linked (`scratchpad/llvmsplit/split.sh`).

| modules, LTO, jobs | split | compile (wall) | per clang | link | total |
|---|---|---|---|---|---|
| 6, none, 6 | 0.5 s | 429 s | 330 to 427 s, 2.5 to 4.0 GB | 0.2 s | 430 s |
| 12, none, 4 | 0.5 s | 512 s | 125 to 198 s, 1.4 to 2.3 GB | 0.2 s | 512 s |
| 12, thin, 4 | 0.5 s | 76 s | 21 to 29 s, 1.0 to 1.4 GB | 478 s (3,670 s CPU, 7.7 GB) | 556 s |

- 6 modules with 6 jobs is the fastest, but six clangs at up to 4 GB
  each hold about 20 GB of the machine's 24 GB; 12 modules with 4 jobs
  stay near 8 GB.
- The 12 no LTO compiles total about 1,860 s of clang time against
  1,589 s for one module without LTO: the copies of constants and
  helpers and the parallel contention cost about 17 percent.
- ThinLTO's link runs its backends on every core (3,670 s of CPU in
  478 s): fast on an idle machine, but it takes the whole machine.

## Speed of the output

The Xo SSA builder's own benchmark (`ssa --std=std --bench=10 <dir>`,
the builder alone, 10 builds), each binary in turn, three rounds,
median ms (`scratchpad/llvmsplit/bench.sh`):

| binary | selfhost/check | selfhost/parser | selfhost/ssa |
|---|---|---|---|
| one module, full LTO (today) | 1,367 | 431 | 2,597 |
| 12 modules, no LTO | 1,397 (+2.2%) | 432 (+0.2%) | 2,683 (+3.3%) |
| 12 modules, ThinLTO | 1,313 (-4.0%) | 407 (-5.6%) | 2,551 (-1.8%) |
| 6 modules, no LTO | 1,392 (+1.8%) | 442 (+2.6%) | 2,700 (+4.0%) |

Rounds of one binary vary by up to 10 percent (full LTO on
selfhost/check: 1,308 to 1,449), so ThinLTO and full LTO are equal
within the noise, and no LTO is 2 to 4 percent slower. All four
binaries built the same function counts (10,270, 3,770, 16,550).

## Decision taken

- Real builds keep one module with full LTO (unset `XO_LLVM_SPLIT`).
- Test processes of internal/selfhost: 12 modules, 4 jobs, no LTO
  (`build.EnableTestLLVMSplit`): 512 s instead of 2,394 s for
  selfhost/ssa, 4.7 times faster, with bounded memory and CPU.
- `XO_LLVM_SPLIT=n` with the default `XO_LLVM_LTO=thin` is the choice
  for a fast build at full output speed on an otherwise idle machine.

## Correctness

`TestGoldenSplit` (internal/llvm): every golden program (123) built in
3 modules, alternating no LTO and ThinLTO (the -short goldens both),
matches want.txt and frees every allocation; passed 2026-10-11.
`TestSplitShape`, `TestSplitLinks`: the module shape and a hand written
module split 1 to 4 ways, compiled, linked, and run. The self hosted
comparisons with the split LLVM binary (TestSelfhostSSA with
`XO_SELFHOST_BACKEND=llvm`) were not run, on the lead's request.

## Next experiments

1. **The full tier with the split build**: time `XO_SELFHOST_TIER=full
   go test ./internal/selfhost` against the 1 h cold run, and check the
   LLVM comparisons pass with the split binaries.
2. **-O0 or -O1 on the split for tests**: one module at -O0 compiles in
   207 s; 12 modules at -O0 should take well under a minute, but the
   selfhost tests run the binary on hundreds of inputs: measure the
   binary's `--bench=10` at -O0 (`split.sh 12 -O0 none 4` then
   `bench.sh`) and the tier's total.
3. **Per module object cache**: constants are numbered across the
   program (`.rs.N`), so one edit renames constants in every module.
   Name constants by content, cache objects by module hash
   (`<cache>/xo/llobj`), and rebuild only the modules that changed.
4. **ThinLTO by default for large real builds**: benchmark the bench/
   programs with `XO_LLVM_SPLIT=12` against today's build, and bound
   the ThinLTO link threads (`-Wl,-mllvm,-threads=N` if ld64 accepts
   it) before making it the default above some IR size.
5. **The remaining single module rows**: `single.sh -O1 full` and
   `single.sh -O2 thin`.
