# Does 0283 stage 2 pay off for the test suites? (item C, 2026-11-05 to 06)

Host: linux/arm64 offload host 1, main cd05891b8, warm shared caches.
Raw logs: ~/xo-lead/notes/xo-lang-4/itemc/fixed-cd05891b8.log,
suites-cd05891b8.log.

## Fixed cost of one warm build (uncontended, 5 rounds)

| program | build wall | run wall |
|---|---|---|
| hello | 0.22 to 0.25 s | 0.22 to 0.24 s |
| golden json_decode_fields | 0.22 to 0.31 s | 0.33 to 0.38 s |

Every timed build set XO_CACHE_STATS=1, which keeps the whole-build
run cache (decision 0360) out (compiler/xo/testcache.xo BUILD_DEBUG_ENV),
so these are compiles through the back-end caches, not stored results.
Process start (`xo version`): 0.00 s. Phases of a warm hello build over
5 ms: check 44 ms (prefetch 24, dirs 20), fe.build 30, ssa 42, cln 19,
insert_rc 22, inline 24, fn_keys 9, emit 12, hollow 9, link 8. Every
cache layer hits (parse 110/110, fe 1592/1597, functions 682/682,
objects 17/17); about 24 MB of cache reads per build.

What stage 2 (a resident compiler holding std's checked and lowered
state) can remove: the check, fe, ssa, cln, insert_rc and inline
phases' reading and decoding of std's entries, about 0.19 of the 0.22 s.
emit, hollow and link (about 30 ms) stay.

## Builds per suite

| suite | xo builds | build CPU sum | mean per build | suite wall |
|---|---|---|---|---|
| internal/arm64 TestGoldenRun* | 648 | 1341.7 s | 2.07 s | 146 s |
| internal/llvm TestGolden | 596 | 6115.7 s | 10.26 s | 533 s |
| internal/wasm | 1 | 28.6 s | 28.6 s | 2126 s |
| cmd/xo (full, amd64 host) | 458 compiling (213 build, 156 test, 89 run) + 572 check | about 87 s (builds) + 25 s (checks) | n/a (counted only) | at most about 5 s of 377 s (28 CPUs) |
| internal/stdtest TestStdModules (amd64 host) | 353 `xo test` | 2229.0 s | 6.32 s | 1172 s |

The suite means are 10x the uncontended 0.22 s: builds run in parallel
and contend for CPU, and the goldens are larger than hello. The fixed
part per build stays about 0.19 s of CPU.

## Ceiling

Stage 2 removes at most about 0.19 s of CPU per build (the warm fixed
part above); it cannot touch clang, assembling, linking or running.

| suite | builds | ceiling (CPU) | share of build CPU | wall ceiling (CPU / parallel jobs) |
|---|---|---|---|---|
| arm64 goldens | 648 | 123 s | 9% | about 13 s of 146 s |
| llvm goldens | 596 | 113 s | 2% | about 10 s of 533 s |
| std modules | 353 | 67 s | 3% | about 35 s of 1172 s (amd64 host) |
| wasm | 1 | 0 | 0% | 0 of 2126 s |
| cmd/xo (full, amd64 host) | 458 compiling + 572 check | about 112 s | n/a (counted only) | at most about 5 s of 377 s |
| go test -short (repo but runs/) | 507 compiling (366 build, 107 test, 34 run) + 571 check | about 96 s (builds) + 25 s (std check share of the checks) | n/a (counted only) | at most about 10 s of 121 s (12 CPUs) |

Per 8-hourly tier (short, arm64, llvm, cmd, gen, fp): the goldens alone
give about 23 s of wall, under 3% of the tier. Per 2-hourly main check
(-short and the fixed point): -short makes 1903 xo calls in 121 s of
wall (507 of them compile a program); at 0.19 s each and about 44 ms of
std check per `xo check`, stage 2 could save about 120 s of CPU, at most
about 10 s of wall at the host's 12-way parallelism. The fixed point is
three xo self builds, which stage 2 does not change.

## Cheaper fixes

(a) Building many goldens in one xo process (one loaded std): not
available (`xo build A B` is one program); it would need a multi-program
mode in xo and in the test drivers, for the same ceiling as stage 2.

(b) Making std's cache reads cheap (a pack read once, decoded lazily):
a warm hello reads about 24 MB, but the reads are only about 30 ms of
the 0.22 s; key computation (about 25 ms), replay and apply (about 65 ms)
and the check (44 ms) are the rest. A pack that carries std's decoded
state and precomputed keys reaches most of stage 2's 0.19 s without a
resident process, and also speeds every developer build. The owner
approved it on 2026-11-06 (decision 0529, xo-lang-4).

(c) Separate compilation in the harness (decision 0388): the test
drivers set XO_SEPCOMP_BG=0 (internal/xotool init), so no build made
std's two variants and every native golden compiled std with the
program. Prebuilding them once per test process (`xo dev stdcache
--conc=0` and `--conc=1`) lets each golden link std's store. Measured
2026-11-06 on host 1 (ac2664c08), internal/arm64 TestGoldenRun, 648
builds, a fresh private back-end cache per run, binary and whole-build
caches off; logs ~/xo-lead/notes/xo-lang-4/sepcomp-*.log:

| runs | std | wall s | user CPU s | mean build s | max load |
|---|---|---:|---:|---:|---:|
| 2 | whole (as today) | 131 to 177 | 1360 to 1369 | 2.27 to 2.99 | 24 to 53 |
| 2 | variants prebuilt | 50 to 80 | 335 to 362 | 0.54 to 1.22 | 23 to 55 |
| 3, 32 busy loops | whole | 310 to 320 | 1254 to 1284 | 5.3 to 5.5 | 56 to 58 |
| 3, 32 busy loops | variants prebuilt | 96 to 104 | 328 to 337 | 1.37 to 1.40 | 50 to 52 |
| 13 more, 0 or 16 busy loops | variants prebuilt | 41 to 80 | 318 to 341 | 0.48 to 1.04 | 11 to 36 |

Building both variants takes about 20 s wall and 140 s CPU on an idle
host (28 to 37 s wall loaded), once per xo. About 75% less CPU and 55
to 70% less wall for the suite. 17 of the 18 separate runs passed;
the one failure (the second run of the first pair, at load 55, beside
other jobs) did not name its golden and did not recur in 16 runs at
loads 11 to 57. A whole against separate comparison of all 596 goldens
(host 2, outputs and exit status, each difference rerun 3 times)
found no behavior difference: 590 identical, log_state's timestamps,
and 5 fault traces without std's prelude.llrt_* wrapper frames
(gotchas/sepcomp-trace-drops-std-wrapper-frame.md, decision 0224's
tail calls; TestTracebackSeparate pins them).

## Recommendation

Do not build 0283 stage 2 for the test suites: its ceiling is about
13 s of the arm64 goldens' 146 s, 10 s of the llvm goldens' 533 s and
35 s of std's 1172 s, under 3% of an 8-hourly tier, and it adds a
resident process to every test driver. Do (c) for the suites: the
harness prebuilds std's variants (internal/xotool, 2026-11-06), with
whole-program coverage kept by scripts/docker-amd64-sample.txt's
goldens, which always build whole, and by the 8-hourly full tier with
XO_TEST_SEPCOMP=0. Then (b), the cache pack with lazy decode (decision
0529), narrowed to the builds that stay whole program (xo's own build,
--release, cc links, the whole sample, a first build before the
variants exist). The suites' time is elsewhere: clang in the llvm goldens
(10.3 s a build against 2.1 s for arm64) and running programs in wasm
(2126 s for 1 build); those are the next levers if suite time matters.

The cmd/xo count (linux/amd64 host, cd05891b8, the exec wrapper; log
~/xo-lead/notes/xo-lang-4/itemc/cmdxo2-amd64-cd05891b8.log) took 377 s
with 2061 xo calls; five tests failed there, none from the counting:
TestCheckSupportPerMember and TestHeapStatsWaste (failing on main that
day), and TestGobindNewerGo, TestHolesNative and
TestTracebackIsolatedNoLeak (amd64 only at that revision; amd64 is best
effort and they are recorded, not chased).

The -short count (host 1, cd05891b8; log
~/xo-lead/notes/xo-lang-4/itemc/short2-cd05891b8.log) had two failing
tests unrelated to the counting: TestCheckSupportPerMember (failing on
main that day, routed to Xo Lang 1) and TestSeedPackHello (0.03 s).

Note (2026-11-06): the first `go test -short` count run hung (no CPU for 15 min,
offload stopped it) after 1906 xo calls, with an empty log. Likely cause:
the wrapper runs xo as a child of bash, so a test that signals or waits on
the process it started (xo serve, xo lsp, a stopped run) talks to the
wrapper, not xo. The rerun's wrapper logs the subcommand and then execs
xo (counts only; the per-build fixed cost comes from the timed runs),
and it finished in 121 s.
