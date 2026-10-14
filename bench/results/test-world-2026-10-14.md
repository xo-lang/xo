# The test world, part 1, in Xo (zero-C audit B14), 2026-10-14

Work item B14 of `bench/results/zero-c-audit-refresh-2026-10-13.md`:
the part of the native test world (decision 0083) that needs no timers
or wait lists moved from std/llrt/xo_testing.c, xo_stdin.c, and
xo_proc.c to `std/native/llrt/testworld.xo`, with the C ABI exports the
C still calls in `std/native/llrt/cabi_tw.xo`.

## What moved

- The switches of `xo test --real` (the effects made real), XO_TEST,
  and the property test variables (XO_PROP_REPLAY, XO_PROP_SEED,
  XO_PROP_BEST): the C's tw_init reads the six variables with getenv
  and hands them to `xo_x_tw_setenv`; `xo_x_tw_init` parses them and
  returns the bits the C keeps in `tw.real` (the C still reads `tw.on`,
  `tw.real`, `tw.virt`; xo_rt.c is unchanged).
- Rand's seeded PCG (rand.xo calls testworld.seeded and rand_u64; the
  C's xo_tw_seeded and xo_tw_rand_u64 and rt.xo's declarations are
  gone), the generators of property tests and testing.gen (gen_new,
  gen_prop, gen_draw, gen_intn, the depth, the record of choices, the
  replay), the report's XO_PROP_BEST and the choices parser.
- Captured standard output and error, FakeEnv (set, get, vars, args),
  FakeProc.exit_code, log records (testing.logs), FakeNet's handlers,
  failures, and requests, the MemFs store, FakeStdio.set_input and the
  fake reads, FakeProc.on_run and the fake Proc.run, Cmd.output and run,
  and pipelines (in the native prelude), the end of test cleanup.
- State: rt.static_word words 32 to 39 (whether the test world runs,
  the real bits, the lock, Rand's state, the MemFs lock and state, the
  state record) and a record from rt.heap_large whose vectors hold
  malloc'd copies of the text, as the C kept it. FakeNet's and MemFs's
  values (rt.Any) are kept by address with one count; the C releases
  them at the end of the test (gotchas/test-world-drop-from-xo.md).
- Calls: the SSA builder's test calls (test_index, test_fail,
  test_output, test_env_set, test_set_args, test_exit_code,
  test_fs_real, test_eprint, tw_set_input, proc_on_run, gen_new,
  gen_free, gen_prop, gen_intn, gen_enter, gen_leave, gen_deep) go to
  the native prelude's llrt_* (selfhost/ssa testmode.xo tw_rt);
  std/native testing.xo, log.xo, http.xo, time.xo, report.xo, gen.xo,
  and rand.xo call testworld directly. cabi_tw.xo's exports are built
  in test binaries only (selfhost/ssa build.xo declare_exports); the C
  has weak definitions for other programs.
- What stays C: part 2 (the virtual clock, the in memory listeners,
  testing.run_main, FakeProc.signal_after; B15), tw_exit's unwinding
  (it records the code through `xo_x_tw_set_exit`), the real standard
  output write and `_exit` of xo_test_write and xo_test_quit (B03's
  streams and exit), xo_test_now, FakeProc.kill's record, and the fake
  child of a spawned command (it reads the entry testworld.xo keeps).

std/llrt: 916 lines deleted, 87 added (xo_testing.c 722 and 56,
xo_proc.c 135 and 16, xo_stdin.c 59 and 15; `git diff --numstat
a58401a7`). Xo: testworld.xo 1,214 lines, cabi_tw.xo 59.

## Instructions (bench/perf/testworld)

`bench/perf/testworld/measure.sh`: the test binary of
testworld_test.xo per backend, each test run 5 times, the fewest
instructions. Base: main at a58401a7 (the C test world); new: the B14
branch at 4282b239 (the same counts as 1806b0d8 within 800 instructions
but rand). Measured on linux/arm64 (EC2 r8g, `perf stat -e
instructions:u`) because the Mac's disk was full during the work (the
lead paused Mac builds); the runs on a warm queue machine, one at a time.

| Test | arm64 base | arm64 new | ratio | LLVM base | LLVM new | ratio |
|---|---|---|---|---|---|---|
| stdio (200,000 println) | 84,276,862 | 99,281,617 | 1.178 | 82,285,538 | 77,488,644 | 0.942 |
| env (2,000 set, 100,000 get) | 4,840,667,856 | 3,041,999,366 | 0.628 | 4,912,940,495 | 2,913,690,160 | 0.593 |
| rand (300,000 rand.int) | 43,551,850 | 55,580,995 | 1.276 | 21,351,915 | 21,956,516 | 1.028 |
| log (20,000 records) | 151,378,265 | 155,063,682 | 1.024 | 122,906,512 | 123,627,638 | 1.006 |
| gen (20,000 List[Str]) | 54,474,675 | 57,914,418 | 1.063 | 47,712,386 | 44,398,524 | 0.931 |
| memfs (3,000 writes and reads) | 51,926,285 | 51,912,227 | 1.000 | 34,535,333 | 34,663,864 | 1.004 |

The "none" test (the setup) is 0.2 million on both. The first version
(commit 9481320d) was 1.216 and 1.300 on arm64 for stdio and rand and
1.428 and 1.320 for gen; the hot paths then lost their calls and checked
arithmetic and the vectors start at 64 entries (1806b0d8).

Within decision 0180's 5 percent: every LLVM row, and on arm64 env,
log, memfs. Not yet on arm64: stdio (1.178), rand (1.276), gen (1.063).
The profiles (`perf record`, same binaries) show why on Linux:
`__aarch64_swp8_rel` (the outline atomic of the lock's release, which
the arm64 emitter calls on Linux, decision 0200) is 21 percent of the
stdio samples and 26 percent of rand's, and `ints.u64_rem` is no longer
inlined into rand.next_int (29 percent of rand's samples) now that
next_u64 inlines testworld's lock (inlining the remainder of a draw,
1f2e0644, took rand from 1.280 to 1.276; the limit's remainder stays a
call). The Mac's arm64 code has the atomics
inline (LSE), so the Mac numbers, the bar's platform, are still to be
taken.

## The Mac (the bar's platform)

The same measure.sh on the Mac (arm64, /usr/bin/time -l instructions
retired), 2026-10-14 after the disk was cleared: base a58401a7 against
the branch at 56971145. Ratio whole, and net of "none" (the setup:
17.7 and 17.5 million on arm64, 14.2 and 14.4 million on LLVM).

| Test | arm64 base | arm64 new | ratio | net | LLVM base | LLVM new | ratio | net |
|---|---|---|---|---|---|---|---|---|
| stdio | 121,358,825 | 122,238,825 | 1.007 | 1.010 | 110,458,818 | 105,422,156 | 0.954 | 0.946 |
| env | 4,616,402,570 | 2,919,759,409 | 0.632 | 0.631 | 4,592,400,242 | 2,695,791,355 | 0.587 | 0.586 |
| rand | 63,162,981 | 69,438,195 | 1.099 | 1.142 | 39,823,875 | 36,734,037 | 0.922 | 0.871 |
| log | 1,748,410,724 | 701,646,197 | 0.401 | 0.395 | 1,724,919,025 | 668,550,284 | 0.388 | 0.382 |
| gen | 78,133,707 | 85,518,224 | 1.095 | 1.126 | 67,298,270 | 68,319,059 | 1.015 | 1.015 |
| memfs | 69,183,491 | 69,164,279 | 1.000 | 1.004 | 53,124,567 | 53,068,586 | 0.999 | 0.993 |

Within decision 0180's 5 percent on the Mac: every LLVM row, and on
arm64 stdio, env, log, memfs. Not met on arm64: rand 1.142 and gen
1.126 (net). Both are short hot functions that call others (the lock's
slow paths, pcg, u64n, the record's growth); the arm64 backend's
prologues and epilogues on their fast paths are the likely cost (shrink
wrapping, found by B13, is being added to selfhost/arm64), so B14's
arm64 rows are rechecked after it.

## After main's 2026-10-14 merges, the rand and gen fixes (the bar met)

Rechecked 2026-10-14 on the Mac: base origin/main 616b25a6 (shrink
wrapping in, B03's stdio.xo and process.xo, which B14 now calls
directly), against the branch merged with it plus three changes:

- `rand.next_int` computes 2^64 - 1 mod span without the unsigned
  division for every span up to 2^62 (`max_rem`: 2 m + 1, less span,
  where m = (2^63 - 1) mod span): `ints.u64_rem(-1, span)` went through
  `u64_div` on every draw.
- `testworld.pcg` takes the carry without a branch.
- testing.gen's generators skip the record of choices (`G_RECORDS`):
  only a property test's generator is reported (prop_report), so
  testing.gen no longer allocates and frees a 512 byte record per value;
  the generator's words are zeroed with stores, not two memset calls.
- The compiler folds a constant Int expression (`const MIN_INT: Int =
  -9223372036854775807 - 1`) to one constant (selfhost/ssa show.xo
  const_value through the checker's const_int, which now returns None on
  overflow instead of faulting): every `MIN_INT` use in std was a
  `movz`, `movk`, and `sub`.

Same columns as above (none: 17.7 and 17.6 million arm64, 14.3 and
14.3 million LLVM):

| Test | arm64 base | arm64 new | ratio | net | LLVM base | LLVM new | ratio | net |
|---|---|---|---|---|---|---|---|---|
| stdio | 124,478,608 | 123,771,216 | 0.994 | 0.994 | 108,683,673 | 103,649,492 | 0.954 | 0.946 |
| env | 4,616,460,543 | 2,916,325,808 | 0.632 | 0.630 | 4,591,586,779 | 2,745,936,161 | 0.598 | 0.597 |
| rand | 63,127,455 | 63,499,794 | 1.006 | 1.010 | 39,797,583 | 36,533,922 | 0.918 | 0.870 |
| log | 1,737,577,150 | 698,892,366 | 0.402 | 0.396 | 1,712,022,038 | 667,631,425 | 0.390 | 0.385 |
| gen | 77,914,419 | 71,994,745 | 0.924 | 0.903 | 67,243,721 | 56,640,108 | 0.842 | 0.799 |
| memfs | 68,402,150 | 68,620,011 | 1.003 | 1.006 | 53,138,513 | 53,118,369 | 1.000 | 0.998 |

Within decision 0180's 5 percent on every row of both backends: rand
1.006 (net 1.010) and gen 0.924 on arm64 were 1.099 and 1.095. A scratch
split of gen (200,000 testing.gen[Int] values) showed the cost was the
generator, not the draws: 1,097 instructions per value on main, 1,232
on the branch, 674 with the record skipped.
