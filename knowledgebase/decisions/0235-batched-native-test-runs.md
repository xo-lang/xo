# 0235: Batched native test runs

Status: Accepted (owner approved the test speed work, relayed by the
lead, 2026-10-15). Implemented 2026-10-15 (xo-4b). Amends decision
0083's "one process per test" and decision 0120's "each case in a
process of its own": both remain as the fallback and behind
XO_TEST_BATCH=0.

## Context

Native `xo test` (`--no-gc`, decisions 0083 and 0120) ran every test
block in a process of its own, and every case of a property test in a
process of its own (100 by default), each through `sh -c 'cd DIR &&
exec env ... BIN'`. A test binary's run is cheap (about 2 ms), but the
three execs per run are not: selfhost/lexer's `xo test --no-gc` started
614 processes (6 property tests of 100 cases, 14 tests), and they were
most of its wall clock. Plain test blocks matter less (examples/
notes-service: 16 processes, about 0.4 s of a 4 s warm run, the rest
the build). Measurements: bench/results/xo-test-batched-2026-10-15.md.

## Decision

The harness (selfhost/xo test.xo) runs the tests that need no real
capability, and each case of the property tests among them, in batches:

- **The plan.** XO_TEST_BATCH lists the runs, separated by commas:
  `index/seed` for a test, `index/seed/first-last` for cases first to
  last (from 0) of a property test, whose case seeds the binary computes
  as the harness does (std/rt's seed*1000003 + case). XO_TEST is still
  set (to the first run's index), so the binary starts as a test binary.
  One process runs the runs of one module directory.
- **The binary.** A test binary's main loops over the native prelude's
  llrt_test_next (selfhost/ssa testmode.xo; twrun.xo next). Without a
  batch it gives XO_TEST's test once (no XO_TEST is still the "no test
  with this index" failure); in a batch each run is bracketed by
  `XO-TEST-BEGIN` and `XO-TEST-END` lines on the real standard output,
  and the test world is reset between runs to what a new process starts
  with: the kept FakeNet handlers, failures, requests and MemFs released
  (as at exit), the fake environment, arguments, captured stdout and
  stderr, log records, FakeProc outputs, exit code, kill record, and
  standard input emptied, FakeClock's sleeps, queued signals, ports
  given and run_main state cleared, the virtual clock back at its
  start, the stop state fresh (stop.xo reset_test), log.configure and
  FakeStdio.set_tty undone, and Rand and a property case's generator
  seeded as XO_TEST_SEED and XO_PROP_SEED seed them.
- **Isolation.** A run that began and has no XO-TEST-END (a failed
  `expect`, an error returned, a fault, a deadlock, an exit) runs again
  alone, one process, exactly as before, and its outcome and message
  are that run's; the next process starts after it. So a failure's
  report is the one process mode's by construction, and no run follows
  a failure in the same process. When the world cannot be reset (an in
  memory listener, a timer, or a cancellation of the test's context
  outlives the test) the binary ends the batch after XO-TEST-END and a
  new process takes the rest. A process that fails after its last run
  ended, or runs none, sends its runs to one process each.
- **Shrinking over a pipe** (2026-10-15). Each replay depends on the
  last, so shrinking cannot be planned as a batch: the harness starts
  the test binary with XO_TEST_SERVE=1 and writes one run per line on
  its standard input (`index/seed/r=c1,c2,.../b=c1,c2,...`, the
  XO_PROP_REPLAY and XO_PROP_BEST it would set), reading the run's
  lines up to XO-TEST-END. A case whose choices are not smaller than
  the best skips its body in process (its inputs are reported before
  the body runs, so the test binary's main goes back to the loop) and
  the harness reads it as the status 3 of a process of its own. A
  failing replay ends the binary: if it was not the binary's first run
  it runs again alone, and the next replay starts a new binary.
- **What stays one process per run.** Tests with `uses` under
  `--real` (their real effects and real standard streams), and
  everything under XO_TEST_BATCH=0.
- **Time limit** (2026-10-15). Every run has `xo test --timeout=D` of
  real time (the Go backend's std/rt -timeout: default 60s, now also 0
  for none, and `xo test` forwards it). The harness reads each process's
  streams as they come and a watchdog kills the process (the child it
  started, by its handle) when a run takes longer: from the start of a
  one process run, from each XO-TEST-BEGIN line of a batch, from each
  replay a serving binary reads. The run fails with std/rt's message
  `the test did not finish within D of real time` and the log records it
  wrote, without running again; the batch goes on in a new process.
  Native test binaries have no task dump (std/rt's SIGQUIT dump is the
  Go backend's), and the Go backend prints none on a time out either,
  so the report is the same on every backend.
- **Parallel chunks** (2026-10-15). The runs are cut into chunks of
  whole tests (a property test's cases stay in one chunk, so its first
  failing case is the one reported, as in one process), each in one
  module directory and of about runs / jobs runs, and the chunks run in
  parallel processes: XO_TEST_JOBS, else the physical cores. The
  outcomes are merged before anything prints, so the report is the
  sequential one; the run cache's key leaves XO_TEST_JOBS out. The Go
  tests give the xo they run XO_TEST_JOBS=2 (internal/xotool
  TestJobsEnv) so test processes do not multiply with the other tests'.
- **Leaks.** XO_LEAKCHECK still reports at exit only; a batch's report
  covers all its runs, so TestNoGCTestLeaks runs every passing test of
  its packages in one batched process too and requires 0 live.

## Consequences

- The harness output (the report, `-v`, `--json`, XO-TEST-FAIL,
  XO-TEST-LOG) is unchanged: the gate ran both modes over selfhost's xo
  unit tests, cmd/xo testdata/nogc (same, prop, native, real with and
  without --real), examples/notes-service, every internal/gen
  testing golden (test_*), and bench/xo's tests, on arm64 and LLVM,
  and compared output and exit status byte for byte.
- A test that only passes because an earlier test left state behind
  would pass in a batch and fail alone; the reset covers every piece of
  test world state the runtime keeps (the list above). New process
  global test state must be added to the reset (twrun.xo between, or
  the prelude's llrt_test_next for modules above twrun.xo).
- A failing run costs one extra process (it runs twice).
- Every process starts the test binary directly (os.proc's cmd with
  `dir` and `set_env`, PWD as `cd` set it), no longer through `sh -c
  'cd DIR && exec env ... BIN'`: about 7 ms less per run (2026-10-15).
- Not done: recovering in process from a failed `expect` (unwinding to
  the loop instead of exiting). Failures are rare, a fresh process is
  the strongest isolation, and the unwinding would have to cross tasks
  and closures where `expect` fails today by exiting.

## Alternatives

- In process recovery on a clean unwind, falling back to one process
  per test afterwards: more code in the generated test bodies for a
  path that only failing tests take.
- A long lived test binary driven over a pipe for every run, not only
  shrinking: the batch needs no round trip per run, so the pipe is kept
  for the runs that depend on the previous outcome.
- Fewer property cases by default: changes what the tests check.
