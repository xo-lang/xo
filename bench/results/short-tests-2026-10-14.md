# `go test -short` speed, 2026-10-14

The repository's `-short` run (every package but `runs/`, the per commit
and merge gate run) and `./cmd/xo -short`, before (main 82379089) and
after this work, on the Mac (12 cores, 24 GB), `-count=1`, warm caches
(bin/xo built, XO_BIN_CACHE, the Go build cache).

Caveat: about ten agents shared the Mac; the load average was 11 to 61
during the runs (each line names it). Every comparison is a pair: the
base tree (an archive of 82379089 in the scratchpad with its own bin/xo
of the same key) and the work tree, run one after the other, alternating,
at the default parallelism (`-p` and `-parallel` = GOMAXPROCS = 12), the
setting of the lead's merge gate since 2026-10-14. The base tree is not a
git checkout, so its TestArchiveTree skips (1.3 s idle) and both sides
ran with XO_SEED_COMMIT set: the base is, if anything, favored.

## Result

| run | before (median) | after (median) | speedup |
|---|---|---|---|
| repository `-short`, wall | 35.0 s | 15.7 s | 2.2x |
| `./cmd/xo -short`, wall | 24.6 s | 5.7 s | 4.3x |
| repository `-short`, CPU (user + sys, one pair) | 158.5 s | 121.0 s | 0.76x the CPU |

Repository pairs (wall, load average at start and end):

| pair | before | after |
|---|---|---|
| 1 | 35.0 s (15.4, 23.2) | 19.3 s (23.2, 30.5) |
| 2 | 35.4 s (30.5, 29.9) | 15.7 s (29.9, 35.0) |
| 3 | 39.2 s (35.0, 35.6) | 17.0 s (35.6, 40.1) |
| 4 | 31.2 s (11.1, 13.0) | 14.7 s (13.0, 18.6) |
| 5 | 30.0 s (18.6, 20.8) | 15.2 s (20.8, 28.1) |

`./cmd/xo` pairs: before 22.3, 24.5, 24.7, 32.3 s; after 5.5, 5.9, 4.3,
6.2 s (load 20 to 31). CPU pair: before real 43.7 s, user 110.1 s, sys
48.4 s; after real 15.0 s, user 78.4 s, sys 42.6 s (`/usr/bin/time -p`,
load 14.7 to 20.9).

The same tests pass on both sides (pair 4: 215 top level tests pass and
93 skip before; after, the same 215 pass plus TestCCCache, which is new,
and TestArchiveTree, which skipped on the non-git base tree; none fails).

### The ten slowest packages, inside the repository run

Median package elapsed over the five pairs (seconds):

| package | before | after |
|---|---|---|
| cmd/xo | 34.2 | 8.2 |
| internal/llvm | 31.2 | 15.2 |
| internal/gen | 26.4 | 14.3 |
| internal/benchharness | 12.3 | 13.1 |
| std/rt | 8.7 | 12.6 |
| internal/xotool | 7.3 | 13.5 |
| internal/arm64 | 5.0 | 13.4 |
| internal/devgoldens | 2.0 | 3.8 |
| third_party/mbedtls | 0.8 | 1.6 |
| internal/gotool | 0.5 | 0.6 |

After the change every package finishes within the same 13 to 15 s
window: the run is CPU bound (the Mac was shared), so packages that did
not change (arm64, devgoldens) now overlap the busy part of the run and
show longer elapsed times. Alone (one package, default parallelism,
load 9 to 28): cmd/xo 24 to 26 s before, 4.6 to 5.7 s after;
internal/llvm 20.4 s before, 8.0 s after; internal/gen 18.9 s before,
9 s after; internal/benchharness's TestHarness 12.2 s before (in the
first baseline repository run, load 14 to 22), 4.3 s alone after; std/rt 6.0 s
before, 4.0 s after; internal/xotool's TestSeedPackHello 4.9 s before,
2.6 s after; internal/arm64 (unchanged) 3.1 to 3.9 s.

### At the agents' preamble setting

`GOMAXPROCS=4 go test -short -p 2 -parallel 2` (one pair, load 11 to 29):
repository 61.8 s before, 44.5 s after; `./cmd/xo` 23.8 s before, 13.4 s
after. The default parallelism after the change (15.7 s median) is about
3 times faster than the preamble setting; the -short run holds only eight
packages that do real work.

## Where the time went, and what changed

- **cmd/xo** (25 s serial, slowest test 5 s): all 63 tests ran one after
  another because they used `t.Setenv` and `t.Chdir`, which forbid
  `t.Parallel`. The xo the tests drive is a process, so a test's
  environment and working directory now live in cmd/xo/env_test.go
  (`setenv(t, k, v)`, `chdir(t, dir)`, `command(t, ...)`, keyed by test
  name, subtests inherit) and every test is parallel;
  modtest.SetupWith takes the setter. TestSupplyChainAttacks runs its 17
  attacks as parallel subtests (6 s in turn).
- **internal/llvm** (20 s): the runtime tests compiled the same
  sanitized C drivers every run (http_diff.c with AddressSanitizer about
  5 s, three times). `xotool.CompileC` now caches linked binaries under
  XO_CC_CACHE (internal/xotool/testcache.go; the key: clang and its
  version, target, sysroot, flags, every source, every header `clang -MM
  -MG` lists, the TLS sources when linked; TestCCCache checks that a
  header change rebuilds); on by EnableTestCCache in the package's
  TestMain. The native signal tests, TestTimeDiff, and TestRealtimeNative
  (with its own build directory) became parallel.
- **internal/gen** (19 s): XO_GOBIND_DIR was a fresh directory per run,
  so every `use go` golden was bound again and missed XO_BIN_CACHE (5 to
  7 s each under load); it is now kept per xo binary and go version
  (xotool.TestGobindDir). TestArithChecks checks 25 programs per `xo
  check` in parallel (one check per program in turn took 4 s; 0.2 s).
  The compile property programs live in content keyed directories
  (xotool.TestProgramDir: a build of a fresh t.TempDir() never reuses a
  binary because the build information records the directory), so their
  CPU fell from 21 to 12.7 s on a warm run. TestGolden, the signal
  tests, TestHoles, and TestReduceDiffFake run in parallel.
- **internal/benchharness** (12 s): the Python self test ran its twelve
  methods in one process; each now runs in its own `python3 -I
  test_harness.py -k NAME` subtest, in parallel (the two adapter tests
  take 4 s each).
- **internal/xotool** (6 s): scripts/seed-pack.sh takes
  XO_SEED_GZIP_LEVEL (default 6, as before); TestSeedPackHello packs at 1
  (4.9 s to 2.6 s). TestArchiveTree archives a small three file
  repository with -short; the whole checkout (78 MB) stays in the full
  tier (1.0 s idle, 7.6 s under load).
- **std/rt** (6 s): the list and integer model tests (pure Go, race
  clean) run in parallel.

## Left

- Cold caches: the first run after a change to selfhost/ or the C
  runtime (a new xo, new C sources) misses XO_BIN_CACHE and XO_CC_CACHE.
  After merging origin/main into this branch (9493ac0f) the first
  repository run took 49 s (load average 10 to 24; internal/llvm 47.9 s,
  TestCryptoDiff 21.9 s), the next one 19 s (load 20 to 30). The merge
  gate often runs cold, so its times sit between the two.

- The run is now CPU bound: the remaining CPU is mostly real builds and
  runs. The largest share left is programs built from fresh temporary
  directories (cmd/xo's module worlds, the harness's tasks), which never
  hit XO_BIN_CACHE because the build information records the program's
  directory; xotool.TestProgramDir is the pattern to move them to.
- TestGolden's 137 programs cost 14 CPU seconds, 8.8 of them system time
  (one 87 MB xo process per build, binaries copied out of the cache).
- The golden arena_holes failed once on the base tree at load average 51
  (an Io error at main.xo:313); not seen on the work tree in these runs.
