# 0439: replay an unchanged test run's results

Status: Accepted (owner, 2026-10-30)

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
XO_TEST_CACHE=off. The replayed report is the stored one with
` (cached)` after the `ok:` or `FAIL:` summary line (not with `--json`),
so tools that parse the summary still see "N passed, M failed".

As built (compiler/xo/testresults.xo, 2026-10-30): the report is
recorded through wholebuild.xo's Tee (the run cache's), which keeps
neither `--phase-times` lines nor the cache and front end statistics,
and stored as `<entry of the result key>.result`; the result key is the
binary's run key, the run cache entry's recorded inputs (its `.m`: every
file, listing, probe and variable the build read, with their hashes;
after a fresh build, only an entry that build wrote) and the run-only
flags, and is empty (nothing stored or
replayed) with `--real`, `--std-module`, `--count=1`, XO_TEST_CACHE=off,
`--profile`, `--trace` or `--only`. A build whose program links no `use
go` or C binding writes `<run entry>.pure`, and only a binary with that
marker has its runs stored. Test: cmd/xo TestXoTestResultCache (an
unchanged run replays; --count=1, XO_TEST_CACHE=off, another seed, a
failing run and a `--real` run whose input file changed run); TestXoTestResultCacheEdits: an edit to the test body, a function it
calls, an embedded file or a std function (a copy of std) runs again).

## Consequences

- Found by the linux/arm64 gate (2026-10-30, before the push): the first
  result key was the run key plus the flags, and the run key leaves the
  sources out (an edit keeps it; the entry's `.m` decides a hit), so a
  rebuild after an edit replayed the earlier "ok" (cmd/xo
  TestNoGCRunCache/arm64: an edit that should fail printed `ok ...
  (cached)`). The key now hashes the entry's `.m`, and a rebuild whose
  program links a binding removes the earlier build's `.pure` marker.
  A test cache must never replay a result across an edit; the edit
  tests above guard it.

- An unchanged rerun costs the run cache's lookup and the printing. On
  linux/arm64 (the offload host, run alone, `xo bench --ab` n=20 of `xo
  test` on an unchanged 40-test package, A XO_TEST_CACHE=off, B the
  result cache, the key hashing the entry's `.m`): wall 0.121 (70 to
  8.4 ms, 95% interval 0.115 to 0.124), cpu 0 (570 ms of test processes
  to none), peak RSS 1.111 (66 to 73 MB: xo itself, which now reads the
  entry's `.m` for the key; the tests' own processes no longer start).
  The first key, without the `.m`, measured 0.118 and 1.074.
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
