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
