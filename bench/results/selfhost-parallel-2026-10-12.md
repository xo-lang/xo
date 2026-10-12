# internal/selfhost on many cores (2026-10-12)

Question: the full tier of internal/selfhost took more than 3 hours and
used about 4 of the 32 cores of a c8g.8xlarge once the other packages had
finished. Can it scale with cores while the merge tier stays cheap on the
24 GB development Mac?

Change (internal/selfhost/jobs_test.go, commits e8dc50c to 73feab0):

- one pool of `XO_SELFHOST_JOBS` slots (default min(GOMAXPROCS, 4)) for
  every native build, every Xo process on a batch of runs, and every Go
  side item. A second pool, `XO_SELFHOST_HEAVY_JOBS`, also covers the
  builds and the mergeHeavy runs. Its default is one slot per 8 GiB of
  memory, or per 16 GiB in the full tier;
- each mergeHeavy run gets its own process, largest first. The light
  batches shrink so that every slot has work;
- the backend subtests run at once. The Go side results are computed
  while the native builds and the Xo processes run;
- the full tier starts every native build at TestMain;
- `XO_LLVM_JOBS` scales with the pool;
- the SSA, arm64, and LLVM comparisons run per run in the pools;
- closure renaming (canonClosures) no longer uses the one regular
  expression that alternated every closure name;
- `-short` no longer samples the mergeHeavy runs.

Every timing below is wall clock. The Mac runs shared the machine with
other agents (noisy). The EC2 runs had the instance to themselves.

## Merge tier, development Mac (12 cores, 24 GB)

Test binary run directly, `-test.v`, GOMAXPROCS=4. The binary cache was
warm. The process tree's RSS was sampled every 2 s.

| commit | XO_SELFHOST_JOBS | wall | peak tree RSS | TestSelfhostSSA/go | TestSelfhostSSAMutations | TestSelfhostGen/go | TestSelfhostArm64/go | TestSelfhostLLVM/go |
|---|---|---|---|---|---|---|---|---|
| e8dc50c | default (4, 3 heavy) | 281 s | 4,718 MB | 105.5 s | 40.5 s | 18.7 s | 15.5 s | 11.3 s |
| e8dc50c | 1 | 476 s | 2,711 MB | 196.3 s | 67.3 s | 44.6 s | 45.1 s | 30.9 s |
| 73feab0 | default (4, 3 heavy) | 233 s | 4,951 MB | 53.1 s | 18.6 s | 26.2 s | 13.2 s | 8.3 s |
| 73feab0 | 1 | 468 s | 3,620 MB | 156.8 s | 43.1 s | 64.0 s | 50.6 s | 28.5 s |

Both 73feab0 runs failed only TestSelfhostWasm, on the golden
map_remove_bytes. That golden came from main (b4e9f1c), not from this
change. The 1,329 s merge tier of 2026-10-12 (toolchain.md) predates
mergeHeavy, so it is not a baseline for this change.

`go test -short ./internal/selfhost`: 175 to 208 s before. selfhost/llvm
was in the every-25th sample: one Xo SSA builder process took 57.8 s and
4,738 MB, and one Xo LLVM emitter process took 28.7 s and 5,853 MB. It
is 31 to 52 s after (gotchas/selfhost-heavy-runs-memory.md).

## Full tier, EC2 (scripts/remote-full.sh, internal/selfhost only, -v)

| run | commit | instance | XO_SELFHOST_JOBS | result |
|---|---|---|---|---|
| 1 | f13caa6 | c8g.8xlarge spot (32 vCPU, 63 GB) | 32 | stopped after 18 min: load average 4.0, every LLVM build on the 4 clang slots of XO_LLVM_JOBS=4 |
| 2 | 415144f | c8g.8xlarge spot | 32 (7 heavy) | stopped after 33 min: seven Xo arm64 emitter processes at 5.0 to 13.2 GB each next to a 10 GB test process; 59 of 63 GB in use, SSH timed out, the process on selfhost/xo was killed |
| 3 | dfd702a | r8g.8xlarge spot (32 vCPU, 247 GB) | 32 | spot reclaimed after about 6 minutes |
| 4 | 46b4d8c | r8g.8xlarge on demand | 32 (15 heavy) | PASS, 5,233 s; peak memory in use 165,560 MB |
| 5 | 41d01eb | r8g.8xlarge on demand | 32 (15 heavy) | PASS, 5,106 s; peak memory in use 193,038 MB |
| 6 | 73feab0 | r8g.8xlarge on demand | 32 (15 heavy) | PASS, 3,490 s; peak memory in use 157,127 MB |

Before this change the full tier took more than 3 hours on c8g.8xlarge
(the lead's run of 2026-10-12, not timed per test) and 3,474 s on the
Mac with fewer passes (2026-10-11, toolchain.md).

Per test, run 6 (the parent test's time is mostly the wait for its
native builds; the subtests' times follow it):

| test | parent | subtests (go, arm64, llvm) |
|---|---|---|
| TestSelfhostArm64 | 1,166 s (the llvm build of selfhost/arm64) | 943 s, 908 s, 1,286 s |
| TestSelfhostSSA | under 4 s | 437 s, 325 s, 335 s |
| TestSelfhostLLVM | under 4 s | 261 s, 200 s, 226 s |
| TestSelfhostGen | under 4 s | 58 s, 55 s, 50 s |
| TestSelfhostArm64Mutations | 79 s | |
| TestSelfhostLLVMMutations | 52 s | |
| TestSelfhostSSAMutations | 27 s | |
| TestSelfhostCheckerModuleMutations | 26 s | |
| TestSelfhostCheckerMutations | 24 s | |

Native builds in run 6, all started at TestMain and running at once:
selfhost/arm64 took 29 s (go), 720 s (arm64), and 1,166 s (llvm).
selfhost/ssa took 17 s, 579 s, and 948 s. selfhost/llvm took 28 s,
648 s, and 816 s. selfhost/gen took 24 s, 591 s, and 620 s.
selfhost/check took 18 s, 99 s, and 239 s.

The longest single items in run 6 bound the run:

- the Go arm64 emitter (in process) on selfhost/xo: 926.8 s;
- the Go SSA builder with its passes on selfhost/xo: 260.1 s;
- the Go LLVM emitter on selfhost/xo: 139.6 s;
- comparing the arm64 output of selfhost/xo: 59.6 s per backend;
- comparing the SSA output of selfhost/xo: 27.0 s per backend.

Run 5 had the same pools without the renamer and the parallel
comparisons. There, TestSelfhostSSA's subtests took 1,781 to 1,851 s,
while the Go side took 258.1 s and the Xo processes about 285 s. The
comparison of selfhost/xo was the critical path. In the merge tier the
renamer cut that comparison from 50.8 s to 1.4 s.

Memory per unit, measured on the Mac (darwin ru_maxrss is per process):

- a light batch: 40 to 300 MB;
- the Xo checker on selfhost/xo: 0.9 GB;
- the Xo generator on selfhost/xo: 1.0 GB;
- the Xo SSA builder on huge_frame, build stage only: 1.9 GB;
- the Xo SSA builder with stages on selfhost/llvm: 4.7 GB;
- the Xo LLVM emitter on selfhost/llvm: 5.9 GB.

Full tier on EC2:

- an Xo arm64 emitter process on a self hosted component: 5.0 to 14.9 GB
  (from ps on the instance);
- one clang on a split part: about 1.1 GB;
- the test process: about 10 to 35 GB.

On linux a child's ru_maxrss includes the memory it shared with the test
process before exec, so jobs_test.go prints n/a there.

## Next experiments, by expected gain

1. The arm64 path of selfhost/xo is now the critical path: its llvm
   build, then about 930 s for each emitter, in sequence. Profile the Go
   emitter: `go test -run 'TestSelfhostArm64$' -cpuprofile` with
   XO_SELFHOST_DIRS=selfhost/xo and XO_SELFHOST_BACKEND=go. Profile the
   Xo emitter: run `xo run selfhost/arm64 -- --each selfhost/xo` under a
   sampling profiler.
2. Start the llvm build of selfhost/arm64 first and give it more clang
   slots: XO_LLVM_JOBS is process wide, and 1,166 s of the critical path
   is that build sharing clang with the other 21.
3. Make the Go side per backend stage parallel. The Go SSA builder's
   passes on selfhost/xo take 260 s in one goroutine.
