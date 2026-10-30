# 0439: replay an unchanged test run's results

Status: Proposed (xo-lang-4, 2026-10-30; queued by the lead after 0436 and the --std-module item; the owner accepts it with its measurements)

## Context

`xo test` keeps its test binary in the run cache (decision 0360's test
entries: the binary and its test list, keyed on the build's inputs), but
runs every test again on each call. Go replays a passed package's output
when the test binary, its flags, and the files and environment variables
the tests read are unchanged (cmd/go's test cache; `-count=1` bypasses it).

Measured (bench/results/test-vs-go-2026-10-30.md): an unchanged rerun of
url's 66 tests takes about 130 ms, of which about 115 ms is running the
tests, against Go's replay at about 50 ms; agents rerun unchanged
packages constantly (a gate after an unrelated edit, a second look at a
summary).

Xo's tests are better placed than Go's to replay safely: a test block's
`os` is a TestOs (MemFs, a virtual clock from 2026-01-01, a seeded Rand,
FakeEnv, FakeNet, FakeProc; core 9.2), so a test without `uses` cannot read
a file, the environment, the clock or the network of the machine. Its
outcome is a function of the binary, the seed and the run flags.

## Decision

After a run in which no test failed, `xo test` stores its report (the
standard output and standard error it printed, and its exit status)
beside the run cache entry, and a later run with the same key prints the
report again with ` (cached)` after the summary line, running nothing.

The key: the run cache key of the test binary (every build input, every
XO_ variable but XO_TEST_JOBS and XO_TEST_ONLY), plus the run-only flags
the binary key leaves out (`--run`, `--seed`, `--cases`, `--timeout`,
`-v`, `--json`, `--explore`), plus the backend and the host.

Never stored (each run runs):

- a run with `--real`, or in which any test declared `uses` and ran (a
  `uses` test skipped without `--real` stays fake);
- a binary that links a `use go` or C binding (its functions reach the
  machine outside the fakes);
- `--std-module` runs of std/native/llrt and any std module whose tests
  call llrt directly (they make system calls); stage 1 leaves every
  `--std-module` run out, and a later stage can admit the modules that
  only use the fakes;
- a run that failed, timed out, or was killed (a timeout depends on the
  machine's speed);
- a run with a debug or profile variable (decision 0437's
  BUILD_DEBUG_ENV) or `--watch`.

Bypass: `--count=1` (Go's spelling, so agents' habits work) or
XO_TEST_CACHE=off. The replayed report is byte for byte the stored one,
so tools that parse the summary still see "N passed, M failed".

## Consequences

- An unchanged rerun costs the cache lookup and printing: expected under
  20 ms against about 130 ms for url (to be measured with `xo bench --ab`
  before the owner accepts this).
- A test that is nondeterministic despite the fakes (a data race on the
  real scheduler, a dependence on memory addresses) passes once and is
  then replayed; Go has the same exposure. `--count=1` and `--explore`
  run it again.
- The prune of the run cache (decision 0360) removes reports with their
  binaries.
- `xo test --json` replays its JSON lines; the summary's ` (cached)` is
  text output only.

## Alternatives

- Replay per test instead of per run: finer (an edit that changes one
  test reruns only it), but the run cache key is per binary, and a test's
  outcome can depend on code shared with every other test; deferred.
- Track real-world reads at run time (a "touched the machine" bit set by
  the runtime's real capabilities) instead of the static rules above:
  more runs replay (a `uses` test that did not use its capability), but
  it needs runtime hooks on every capability; the static rules come first.
- Do nothing: agents keep paying the full run on every unchanged rerun.
