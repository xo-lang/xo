# The golden harnesses and the selfhost recording before `xo dev` (2026-10-13)

Decision 0209 phase 1: what the three Python golden harnesses and the
internal/selfhost recording cost before any change, and where the time
goes, as the baseline for `xo dev goldens` (the design is in
knowledgebase/decisions/0209-xo-dev-harness.md, "Design (2026-10-13)").

## Setup

- Commit 78a06532 (main before decision 0215's Go compiler deletion).
- Mac: the shared 12 core Apple silicon Mac (24 GB), every run under the
  heavy lock (one heavy run at a time, other agents' light work going
  on; load average 1.85 at the start). One run each: compare CPU sums
  and instruction counts more than seconds.
- xo1: built by `selfdriver.Binary()` from this worktree's sources,
  cache key 36a0f09c54a6cb61 (420.37 s wall, 685.47 s user, 11.05 GB
  max RSS on the Mac).
- The Go `xo` for the references of internal/arm64/testdata/run: built
  from the same commit (`XO_GO_XO`, `XO_SELF_DRIVER=0`).
- Totals: each script under `/usr/bin/time -l` with its default 4
  workers (`XO_GOLD_DIR` in the scratchpad). macOS `time -l` counts the
  children's CPU time in user and sys; max RSS is the largest single
  process; the instruction count is the Python process's own and is not
  reported.
- Breakdown: a scratch copy of the scripts' logic (same programs, same
  4 workers, not committed) that times each step of each program and
  wraps each build in `/usr/bin/time -l` with xo1's `--phase-times`.

## The three harnesses (Mac)

| harness | programs | wall | CPU (user + sys) | max RSS |
|---|---|---|---|---|
| selfhost-goldens.py, `--backend=arm64` | 179 (153 want.txt, 26 against the Go backend) | 49.12 s | 136.84 s | 1,443 MB |
| selfhost-goldens.py, `XO1_BACKEND=llvm` | 179 | 65.24 s | 207.01 s | 1,409 MB |
| selfhost-gogoldens.py (`--backend=go`) | 136 | 19.85 s | 96.40 s | 152 MB |
| elf-goldens.py, build half only | 179 | 27.83 s | 68.28 s | 1,440 MB |

All 179 arm64, 179 LLVM, and 136 Go backend programs pass. The ELF run
is a build only figure and fails every program: Docker is not running
on the Mac (the script never starts it), and before that every
linux/arm64 `--ld=macho` build stops in the assembler on `svc #0`
(knowledgebase/gotchas/macho-assembler-lacks-svc.md), so its 27.83 s
covers xo1's check through emit, not the assembly, link, or run.

## Where the time goes (Mac, sums over the programs)

| step | arm64 | LLVM | Go backend | ELF (to emit) |
|---|---|---|---|---|
| harness wall (4 workers) | 45.2 s | 62.2 s | 14.4 s | 27.7 s |
| builds, wall summed | 109.6 s | 181.9 s | 35.7 s | 69.0 s |
| builds, CPU | 112.3 s | 185.2 s | 67.1 s | 66.0 s |
| of it: check (std load and check) | 20.7 s | 20.9 s | n/a | 20.8 s |
| of it: SSA, passes, emit | 28.1 s | 24.3 s | n/a | 28.3 s |
| of it: cc (assemble and link; clang for LLVM) | 56.6 s | 132.8 s | n/a | n/a |
| running the programs | 22.1 s | 21.1 s | 21.3 s | n/a |
| Go `xo` references (26 programs: build, run) | 5.8 s, 3.4 s | 4.6 s, 3.4 s | n/a | n/a |
| comparing | 0.1 ms | 0.1 ms | 1.4 ms | n/a |
| build instructions | 9.57e11 | 7.69e11 | 5.01e10 | 8.12e11 |

(SSA, passes, emit: the phases ssa, add_landings, cleanup, insert_rc,
inline, emit, write.)

- std load and check: the check phase takes 109 ms for arith (no
  imports) and a median of 113 ms per program (max 220 to 226 ms):
  about 96 percent of it is std, the same work 179 times, 20.7 s of
  CPU per backend. Checking std once per run would save about 20 s of
  CPU per backend run (18 percent of the arm64 builds' CPU, 11 percent
  of LLVM's).
- The single largest step is the C toolchain: cc is 56.6 s of the
  arm64 builds' 109.6 s and clang 132.8 s of LLVM's 181.9 s.
  `--ld=macho` (no cc) removes it for the arm64 backend on darwin; for
  LLVM it is the compile of the IR and stays.
- One program sets the floor: internal/arm64/testdata/run/huge_frame
  (one 5,000 line function) builds in 15.80 s on arm64 and 15.54 s on
  LLVM (insert_rc 9.57 s, ssa 3.83 s), a third of the arm64 harness's
  45.2 s wall; every other build is under 1.8 s on arm64 and 4.9 s on
  LLVM. Starting it first (the largest first, as runXoBatches does)
  keeps it off the end of the run.
- Running and comparing are cheap: the programs run 21 to 22 s summed
  (http_client 1.13 s the longest), the comparison is under 2 ms in all.
- The Go backend's builds use the Go toolchain's own parallelism (67.1 s
  CPU in 35.7 s summed wall); its runs (21.3 s) weigh more than in the
  native harnesses.
- Python itself is not the cost: the time is xo1, cc, and the programs.
  What the rewrite gains is the std check once per process, starting
  the largest first, and skipping unchanged work, not a faster loop.

## The selfhost recording's serial floor (EC2)

One on demand r8g.8xlarge (32 vCPUs, 247 GiB), commit 78a06532,
`XO_SELF_DRIVER=0`, through the lead's queue (request harness-floor):

- A: the recording as the lead ran it (`XO_SELFHOST_GOLDENS=record`,
  the 16 TestSelfhost tests named in the task, default pool: 4 jobs, 4
  heavy) with `-v`: 2,020 s wall (go test 2,013.063 s), 10,078.34 s
  user, 283.02 s sys, 26.95 GB max RSS. The earlier runs without `-v`
  took 1,885 to 2,110 s (the task's figures).
- B: a probe test written on the instance only (never committed): each
  Go side kind on each mergeHeavy run alone, one at a time, with the
  process's peak RSS (VmHWM, reset before each item); 4,081 s for the
  heavy runs alone.

### Per test (A, in the order go test ran them; the tests run one after another)

| test | time | Go side items | longest item |
|---|---|---|---|
| TestSelfhostArm64 | 1,009.05 s | 748 on 4 jobs, 1,009.0 s | 653.9 s (selfhost/xo) |
| TestSelfhostSSA | 470.78 s | 831, 470.7 s | 296.3 s (selfhost/xo) |
| TestSelfhostLLVM | 253.26 s | 831, 253.2 s | 170.3 s (selfhost/xo) |
| TestSelfhostArm64Mutations | 134.04 s | 300, a serial loop | not logged |
| TestSelfhostSSAMutations | 36.33 s | 425, 33.5 s | 3.6 s (s0001) |
| TestSelfhostLLVMMutations | 29.85 s | 400, 27.1 s | 12.2 s (l0133) |
| TestSelfhostGen | 19.60 s | 1,579, 19.5 s | 5.9 s (selfhost/xo) |
| TestSelfhostCheckerMutations | 17.55 s | a serial loop | not logged |
| TestSelfhostCheckerModuleMutations | 16.12 s | a serial loop | not logged |
| TestSelfhostChecker | 5.89 s | 748, 5.2 s | 2.8 s (selfhost/xo) |
| TestSelfhostGenMutations | 5.25 s | a serial loop | not logged |
| TestSelfhostFormat | 4.36 s | a serial loop | not logged |
| TestSelfhostDefhash | 4.21 s | a serial loop | not logged |
| TestSelfhostCheckerHatches | 0.17 s | | |
| TestSelfhostDifferential, ParserDifferential | 0.00 s | (recorded under the go backend's subtests) | |

### Ranked items (B, each alone)

| rank | item | time | peak RSS |
|---|---|---|---|
| 1 | Go arm64 emitter, selfhost/xo | 615.0 s (653.9 s in A) | 17,221 MB |
| 2 | Go arm64 emitter, selfhost/arm64 | 294.9 s | 7,726 MB |
| 3 | Go SSA builder (every pass), selfhost/xo | 283.4 s (296.3 s in A) | 6,907 MB |
| 4 | Go arm64 emitter, selfhost/wasm | 265.7 s | 8,195 MB |
| 5 | Go arm64 emitter, selfhost/llvm | 260.3 s | 7,146 MB |
| 6 | Go arm64 emitter, selfhost/ssa | 252.3 s | 6,831 MB |
| 7 | Go arm64 emitter, selfhost/gen | 235.0 s | 5,983 MB |
| 8 | Go LLVM emitter, selfhost/xo | 149.6 s (170.3 s in A) | 11,944 MB |
| 9 | Go SSA builder, selfhost/arm64 | 138.1 s | 4,428 MB |
| 10 | Go arm64 emitter, selfhost/cli/modules | 131.4 s | 12,831 MB |
| 11 | Go arm64 emitter, selfhost/cli/modules/xomod | 130.6 s | 12,794 MB |
| 12 | Go SSA builder, selfhost/wasm | 120.0 s | 3,682 MB |
| 13 | Go SSA builder, selfhost/llvm | 116.7 s | 3,790 MB |
| 14 | Go SSA builder, selfhost/ssa | 109.7 s | 3,586 MB |
| 15 | Go SSA builder, selfhost/gen | 78.9 s | 2,796 MB |

Every other heavy item is under 80 s; the Go checker's longest is 2.4 s
and the Go generator's 5.5 s (selfhost/xo). huge_frame: 13.0 s arm64
emitter, 11.6 s LLVM, 3.6 s SSA.

Duplicates: selfhost/cli/modules and selfhost/cli/modules/xomod
allocate exactly the same (119,003 MB in the arm64 emitter, 39,512 MB
in the SSA builder), as do selfhost/cli/bind and cli/bind/xobind (1,109
MB, 1,015 MB): each pair compiles the same program, so the second of
each is about 130 s of repeated work in the arm64 emitter alone.

### Why more cores barely help

The tests run one after another, and each waits for its longest item.
The three selfhost/xo items alone are 653.9 + 296.3 + 170.3 = 1,120.5 s
of the 2,020 s, plus about 182 s of serial loops (Arm64Mutations
134.04 s, CheckerMutations 17.55 s, CheckerModuleMutations 16.12 s,
GenMutations 5.25 s, Format 4.36 s, Defhash 4.21 s). That is about
1,300 s that no pool size removes, which is where the 32 job run landed
(1,413 s).

### How each could be split or skipped

1. selfhost/xo in the arm64 emitter (653.9 s): per unit parallelism,
   by function. The comparison already cuts the dump into one piece per
   function; emitting the functions on several workers (the module's
   SSA built once, functions emitted in parallel and joined in order)
   makes the item's floor its slowest function. Input hash skip: the
   unit's key is its whole module plus every repository package it uses
   (selfhost/xo uses nearly all), so it is skipped only when none of
   selfhost/ changed.
2. Running the tests one after another (the sum of 1, 3, 8): one pool
   across the top level tests (t.Parallel on the top level tests, or a
   single scheduler for every unit of every kind), so the floor is the
   longest single item, 653.9 s, instead of the sum, about 1,120 s.
3. The selfhost/xo SSA (296.3 s) and LLVM (170.3 s) items, and ranks 2
   and 4 to 7: the same per function split as 1.
4. The serial loops (about 182 s): run their items through parallel()
   as the differentials do; Arm64Mutations alone is 134.04 s for 300
   small programs.
5. The duplicate units (cli/modules/xomod, cli/bind/xobind, about 130 s
   and 1.5 s in the arm64 emitter): skip a unit whose input hash
   (resolved program files) equals one already done in the run.

After decision 0215 (on main at 5e21dc31) the Go side is gone: replay
runs the Xo side against these hashes and recording runs the Xo side.
The items above are the Go implementation's; the Xo side's heaviest
items are the same inputs (selfhost/xo first), so the same three levers
apply. The merge tier after the deletion is measured below.

## The merge tier after the Go compiler's deletion (EC2)

The warm r8g.4xlarge (16 vCPUs), commit 5e21dc31 (decision 0215 merged,
before decision 0217), `go test -count=1 -v ./internal/selfhost`
(merge tier, replay, default pool: 4 jobs, 4 heavy), request
harness-merge: 2,252 s wall (go test 2,250.878 s), 4,033.52 s user,
277.82 s sys, 17.65 GB max RSS.

- The toolchain first: the seed chain built the test's xo in 462 s
  (`xotool: hop 4f792c55... built in 460 s`), once per machine cache.
- TestSelfhostArm64 1,626.68 s, almost all of it its first line
  `selfhost/arm64 with go: built in 1626.7 s` (the component build,
  waiting behind the toolchain and the other heavy builds); its replay
  of 365 runs took 47.8 s (longest batch 15.0 s) and the comparison 2.3 s.
- The rest: SSAMutations 105.20 s, Wasm 98.45 s, Arm64Mutations 15.30 s,
  CheckerXoTests 13.80 s, CheckerMutations 13.37 s, Defhash 13.22 s,
  CheckerModuleMutations 12.45 s, Gen 7.74 s, LLVM 6.91 s,
  LLVMMutations 6.06 s, Format 5.30 s, MachoXoTests 3.51 s,
  ParserXoTests 3.49 s, GenMutations 2.51 s, CheckerHatches 1.97 s,
  XoTests 1.17 s; the others under 0.1 s (their work runs in the
  background of the tests before them).

## Re-ranked after decision 0217 (verify by behavior)

Decision 0217 (owner, 2026-10-13) skips the back end and mutation dump
replays by default (TestSelfhostSSA, SSAMutations, Arm64,
Arm64Mutations, LLVM, LLVMMutations, Gen, GenMutations, Defhash,
CheckerMutations, CheckerModuleMutations, CheckerHatches;
`XO_SELFHOST_DUMPS=all` runs them) and needs no re-recording for an
intended change. That removes every item of the recording's ranked list
above from the default runs: the 2,020 s recording, and in the merge
tier above TestSelfhostArm64 (1,626.68 s with its component build) and
the other skipped tests (about 190 s).

What remains per change, ranked:

1. Building xo1 when selfhost/ or a std module it uses changed: 420.37
   s wall on the shared Mac for this worktree's xo1 (Setup), 163.83 s
   for xo1 compiling selfhost/xo (bench/results/xo1-speed-2026-10-13.md:22); and
   the self rebuild fixed point for compiler and runtime changes (two
   more such builds). The single largest step; the arm64 emitter on
   selfhost/xo is its core (the same input as the recording's 653.9 s
   Go item, in Xo). Lever: per function parallel emission (lever 3).
2. The seed chain's xo for the tests: 462 s once per machine cache, 0
   after (cached by its inputs).
3. The program output goldens through xo1: 49.12 s arm64, 65.24 s LLVM,
   19.85 s Go backend on the Mac (above); huge_frame's 15.80 s build is
   the single longest program. Levers: std checked once per worker
   (about 20 s of CPU per backend), largest first, input hash skip for
   unchanged programs on a rerun.
4. TestSelfhostWasm, 98.45 s on r8g.4xlarge.
5. The front end replays and Xo unit tests kept on by decision 0217:
   CheckerXoTests 13.80 s, Format 5.30 s, MachoXoTests 3.51 s,
   ParserXoTests 3.49 s, XoTests 1.17 s, the checker, lexer, and
   parser differentials under a second each (about 27 s together, plus
   the component builds: check 7.5 s, gen 7.7 s, llvm 6.9 s, ssa 8.8 s
   on the go backend).

Expected time per compiler change after decision 0217, from these
figures: the xo1 rebuild (164 to 420 s), the goldens on three backends
(about 134 s on the Mac with today's scripts), the remaining selfhost
tests (about 130 s), then the fixed point; about 7 to 12 minutes plus
the fixed point, against the 3 to 5 minutes decision 0217 estimates
without the xo1 rebuild. The levers of decision 0209 apply to items 1
and 3; item 1 is where per function parallelism pays most.

## Phase 2: `xo dev` beside the Python scripts (Mac, 2026-10-13)

The same xo for both (bin/xo of this branch at 2d327cb0, built by
scripts/xobuild in 348 s), the shared Mac under the heavy lock (load
average 3.1 at the start), one run each, 23:47 to 23:55 (after the disk
full of 22:55 to 23:10; the first comparison at 22:49 finished at
22:55:30 with the same verdicts). `/usr/bin/time -l` counts the children
in user and sys.

### Agreement

Both report every program passing, the same set: arm64 179 of 179, LLVM
179 of 179, Go backend 136 of 136. `xo dev errorcheck test/errorcheck`:
15 of 15; `xo dev codegen test/codegen`: 4 builds, 11 patterns, all
pass. A wrong expectation fails as it should (a scratch `// ERROR
XO0601` on an unused binding reports the missing XO0601 and the
unexpected XO0110; a `#4` shift pattern reports the mismatch), and a
failing build inside a worker fails that program only.

### Wall time and CPU (Python = the scripts, 4 workers)

| harness | Python | xo dev -j 4 | xo dev -j 8 | xo dev -j 12 |
|---|---|---|---|---|
| arm64, one `xo build` per program (--no-each) | 49.74 s (122.43 s CPU) | 35.37 s (123.18 s CPU) | 17.10 s (119.96 s CPU) | |
| arm64, build-each workers (static shares) | | 36.00 s (108.44 s CPU) | 18.05 s (115.46 s CPU) | 19.43 s |
| LLVM, --no-each | 61.84 s (188.32 s CPU) | 51.49 s | 27.46 s | |
| LLVM, build-each workers (static shares) | | 65.18 s | 33.35 s | |
| Go backend | 16.79 s | 14.72 s | 8.77 s | |
| arm64, pass cache warm (-j 8) | | | 0.21 s | |

(CPU = user + sys.)

### Lever 3: in-process workers, largest first

At 4 workers, like the scripts, xo dev takes 35.37 s against Python's
49.74 s on arm64 and 51.49 s against 61.84 s on LLVM, with the same CPU:
the difference is the order (huge_frame, 15.8 s, starts first instead
of last in path order). At 8 workers: 17.10 s arm64 (2.9 times
faster than Python), 27.46 s LLVM (2.25 times), 8.77 s Go (1.9 times).
12 workers is slower than 8 on the shared Mac (19.43 s).

### Lever 2: passes skipped by input hash

With every program passing and nothing changed, the run is 0.21 s (179
cached) against 17.27 s with --force. The key is checked: a copy of the
xo binary with one byte appended reruns every program (0 cached), the
same binary skips them; hashing the 83 MB binary and std costs 0.2 s.

### Lever 1: std parsed once per worker

Where the per program check goes (126 ms native on arith): parsing the
68 std/native files is 67 ms of it (`xo dev dump ast --bench=10`: 671
ms for 10), lexing 16 ms; without the native modules the check is 4 ms.
The checker now keeps the parses of std files (check.Checker
parse_cache) for the next checker of the process:

- `check --native --bench=20` of arith: 2,449, 2,380, and 2,406 ms
  without the cache, 1,595, 1,593, and 1,606 ms with it (-34 percent);
- `check --native --each` of all 179 goldens: 23.35 s and 245.4 billion
  instructions without, 14.64 s and 154.3 billion with (-37 percent of
  the instructions), dumps byte identical (5,838,680 bytes).

In the harness the native builds go to `xo dev build-each` workers that
carry the cache from build to build. With the programs dealt into
static shares it saved CPU (arm64 -j 4: 123.18 to 108.44 s) but not
wall time, and LLVM got slower (27.46 to 33.35 s at -j 8): fixed shares
cannot balance builds whose times vary 100 times. The workers now take
the next program from a queue (`build-each --stdin`); figures below.

With the queue (bin/xo at 8ea018be, 2026-10-14 00:41 to 00:45, load
average 1.5 at the start), one `xo build` per program against
build-each workers, no cache:

| backend, workers | --no-each: wall, CPU | build-each queue: wall, CPU |
|---|---|---|
| arm64, -j 4 | 33.27 s, 115.55 s | 31.00 s, 107.27 s |
| arm64, -j 8 | 16.93 s, 118.82 s | 17.26 s, 112.79 s |
| LLVM, -j 4 | 51.52 s, 190.31 s | 49.44 s, 182.07 s |
| LLVM, -j 8 | 28.75 s, 210.95 s | 26.62 s, 198.71 s |

The std parse once per worker saves 4 to 7 percent of the CPU (about 6
s of arm64's 119 s, 12 s of LLVM's 211 s at -j 8), in line with the
check figures above: of the 20.7 s of check per backend run, the parse
of std/native is about half, and the other half (checking the native
modules' bodies again for each program) is not shared yet. Wall time
moves within the run to run noise at -j 8 on arm64 and gains 2 s on
LLVM. Build-each is the default for the native backends; `--no-each`
keeps one `xo build` per program.

## Per change total after decisions 0209 and 0217 (Mac)

Measured 2026-10-14 01:26 to 01:29 on the shared Mac under the heavy
lock (load average 1.7 at the start), branch at 17fd54d1 (origin/main
merged, so every replay of decision 0217 is skipped by default):

| step | wall | CPU (user + sys) |
|---|---|---|
| rebuild bin/xo after a selfhost change (scripts/xobuild) | 332 to 354 s (four rebuilds on 2026-10-13 and 14: 327.84, 332.07, 348, 354 s) | 311.56 to 313.96 s user |
| `go test ./internal/selfhost` (merge tier) | 85.30 s | 160.97 s |
| `go test ./internal/devgoldens` (arm64, LLVM, Go goldens at -j 8, error check, codegen; no cache) | 61.04 s | 431.44 s |

(The devgoldens run of this table found one failure, gc_config on the Go
backend, which was the wrapper's $PWD; fixed in 46529f21,
knowledgebase/gotchas/go-test-pwd-misleads-xo.md; the backend's 136
programs pass from a shell and from the fixed wrapper.)

Total for a compiler change: about 354 + 85 + 61 = 500 s, 8.3 minutes,
plus the fixed point; the low end of the phase 1 estimate of 7 to 12
minutes. Without the xo rebuild (a change outside selfhost/ and the std
modules xo uses) the tests take 146 s, 2.4 minutes, under decision
0217's 3 to 5 minutes. A rerun with nothing changed in the goldens takes
0.21 s per backend from the pass cache. The xo rebuild is now 71
percent of the total, so it is where the next minutes are: the
fast-builds work (decision 0209 lever 3 applied to the compiler itself:
per function parallel emission).

## Phase 2 on linux/arm64 (EC2)

A warm r8g.8xlarge (32 vCPUs), commit a84c8182 (build-each with static
shares), request harness-linux2, 2026-10-14; bin/xo built from the
0.9.0-5e21dc31 seed in 431 s. GNU `time` (children included in user and
sys). One run each.

| harness | Python (4 workers) | xo dev |
|---|---|---|
| arm64 | 61.56 s, 172.43 s CPU | -j 4: 32.98 s (--no-each), 31.45 s (build-each, 109.27 s CPU against 128.72 s); -j 16: 26.30 s; -j 32: 25.95 s |
| LLVM | 124.02 s, 427.04 s CPU | -j 16: 44.08 s, 401.47 s CPU |
| Go backend | 30.69 s, 389.11 s CPU (the first Go builds of the instance: a cold Go build cache) | -j 16: 6.37 s, 41.94 s CPU |

Agreement: every program passes both ways (arm64 179, LLVM 179, Go
136), the same sets as on the Mac. On this evidence the three Python
scripts were deleted (64fa162c).

- More workers stop helping at 16 on arm64 (26.30 s at 16, 25.95 s at
  32): the longest program, huge_frame (about 16 s to build), is the
  floor, the same shape as the selfhost recording's.
- The ELF goldens, never run before on Linux (elf-goldens.py needs
  Docker): with a linux/arm64 runtime pack (`xo rtpack
  --target=linux/arm64`, 5.89 s) and `--flags='--target=linux/arm64
  --ld=macho'`, 166 of 179 pass, run directly on the host. The 13
  failures are selfhost/macho gaps for ELF, among them `unsupported
  directive .data` (sys_files_clock, unix_kill); the full list is in the
  final Linux run below.
- TestDevGoldensGo failed gc_config there too, from the wrapper's $PWD
  (fixed in 46529f21, gotchas/go-test-pwd-misleads-xo.md); `xo dev
  goldens --backend=go` itself passed 136 of 136.

## Final linux/arm64 run (EC2, 64fa162c)

Request harness-linux3 (warm r8g.8xlarge, bin/xo from the seed in 495
s), after the Python scripts were deleted:

- `go test ./internal/devgoldens`: TestDevGoldensArm64 179 of 179
  (30.11 s), TestDevGoldensLLVM 179 of 179 (33.36 s), TestDevGoldensGo
  136 of 136 (25.05 s, the $PWD fix), TestDevErrorcheck 15 of 15,
  TestDevCodegen 11 patterns; TestDevGoldensELF 166 of 179.
- The 13 ELF failures have one cause: selfhost/macho's ELF path did not
  take the `.data` directive the linux emitter writes (every network and
  clock program). Fixed after this run (gotchas/macho-assembler-lacks-svc.md,
  second gap), with an assembler test; the ELF rerun is pending a
  linux/arm64 host.
- `--cmd=run` (`xo run` in the program's directory): 178 of 179 on Linux
  and on the Mac (21.73 s at -j 8): tz_table's run prints the checker's
  note `zone "Not/AZone" is not in the zone database` on standard error
  before the program's output, which want.txt (made from a built binary)
  does not hold. `xo run` passing check notes through to the program's
  stderr is the open question; the build then run mode is the default.
