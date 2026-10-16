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
- **What stays one process per run.** Tests with `uses` under
  `--real` (their real effects and real standard streams), shrinking
  (each replay depends on the last), and everything under
  XO_TEST_BATCH=0.
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
- Not done: recovering in process from a failed `expect` (unwinding to
  the loop instead of exiting). Failures are rare, a fresh process is
  the strongest isolation, and the unwinding would have to cross tasks
  and closures where `expect` fails today by exiting.

## Alternatives

- In process recovery on a clean unwind, falling back to one process
  per test afterwards: more code in the generated test bodies for a
  path that only failing tests take.
- A long lived test binary driven over a pipe (requests on stdin):
  would batch shrinking too, but needs an interactive process API in
  the harness; possible later.
- Fewer property cases by default: changes what the tests check.
