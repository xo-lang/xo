# 0120: Property tests, testing.gen, and `xo test --real` on the native backends

Status: Implemented (2026-10-10): property tests, `testing.gen`, and
`--real` (section at the end). Amends 0083 (native test binaries:
"not available yet"). Code: `std/llrt/xo_testing.c` (`xo_gen_*`,
`xo_prop_report`), `internal/ssa/build_gen.go`,
`internal/ssa/build_testmode.go` (`propInputs`, `testingGen`),
`internal/build/nogc.go` (`TestInfo.Prop`), `cmd/xo/nativetest.go`
(`runNativeProperty`, `shrinkNative`). Tests: TestNoGCTests and
TestNoGCTestLeaks (cmd/xo, `testdata/nogc/prop`: the native report equals
the Go backend's byte for byte on both native backends).

## Context

Native test binaries (0083) reported property tests (`test "x" for v:
T`) and `testing.gen[T](seed)` as XO1201 and skipped tests that declare
`uses` even with `--real`. On the Go backend std/rt generates inputs from
a GenRand (math/rand/v2's PCG, every choice recorded), runs up to
`--cases` cases (100) each in a fresh test world, and shrinks a failing
case by replaying edited choice lists (shortlex smaller, at most 2000
tries); the report names the case and the shrunk input as `name =
Debug` lines. A native test runs in a process of its own because a fault
ends the process.

## Decision

1. **The same choices.** The C runtime's generator is std/rt's: Go's PCG
   seeded `(s, s ^ 0x2545f4914f6cdd1d)`, `Uint64N` as Go computes it
   (mask for powers of two, Lemire with rejection), every draw recorded,
   replay with `v % n`. internal/ssa builds `xo.gen[T]` per checker type
   in the order of std/rt's `Gen` and the generated `XoGenAny` methods
   (depth limit, sizes, zigzag integers and edges, the alphabet,
   structs retrying an invariant up to 1000 times, enums falling back to
   unit variants when deep). Scalars come from the runtime, compound
   values from the generated code (`Map` and `Set` through the native map
   put, Box fields and Secret as lowered elsewhere). A seed therefore
   gives the Go backend's values, and the native report is the Go
   backend's.
2. **One process per case.** The test binary's main makes the case's
   inputs (`XO_PROP_SEED`: std/rt's `seed * 1000003 + case`, or
   `XO_PROP_REPLAY`: choices to replay), writes their choices and Debug
   text for the harness (`XO-PROP-CHOICES`, `XO-PROP-INPUT`), and calls
   the test. `cmd/xo` runs the cases and, on a failure, std/rt's
   shrinking loop; the binary skips a try whose choices are not smaller
   than `XO_PROP_BEST` (exit 3), as std/rt does before running it.
3. **`testing.gen[T](seed)`** makes a generator, the value, and frees it.
4. A sized integer key (`Set[Int8]`) no longer fails the native build
   (its hash mixed Int8 and Int; found by the tests).

## Consequences

- `xo test --no-gc` runs property tests with the Go backend's cases,
  shrunk inputs, and messages; fault reports keep the native difference
  (no frames). A property test costs a process per case and per shrink
  try: the test module of TestNoGCTests (ten property tests, seven of
  them failing and shrunk) takes about 3 s natively against 0.3 s on the
  Go backend.
- Contract fuzzing (core 9.1) stays skipped natively; it can reuse the
  generators (`small` choices are in the runtime) and the harness loop.
- Generated values of 5 and more element tuples fail on the Go backend
  (std/rt has XoGenAny for 2 to 4) and work natively.

## Alternatives

- **All cases in one process** (resetting the test world between
  cases): faster, but a fault ends the process, so the harness would
  restart from the faulting case anyway, and every fake would need a
  reset; one process per case keeps cases independent as std/rt's fresh
  worlds are.
- **fork per case**: the runtime starts worker threads, which fork does
  not copy.
- **Different values from the Go backend's** (any good generator): the
  reports and pinned seeds would differ between backends.

## `xo test --real` (2026-10-10)

Decision: a native test binary built with `--real` contains the tests
that declare `uses` (ssa.Options.RealTests, build.Options.RealTests);
the harness runs each with `XO_TEST_REAL=<its effects>`, and the test
world makes those effects real and keeps the fakes for the others, as
std/rt's realCaps does:

- C switches (`tw_is_fake`): standard streams and standard input
  (`stdio`), the environment and arguments (`env`), listen and dial
  (`net`; the http client reaches real servers, `rttest.test_fake`),
  `Proc.run` (`proc`), and the clock (`clock`).
- Fs: a TestOs's fs is a MemFs; with `--real` every Fs or MemFs method
  that has a real Fs implementation calls a generated wrapper
  (`xo.testfs[...]`) that picks the real one at run time when the test
  declares `fs`, `fs.read`, or `fs.write`, as a read only or write only
  view for one of the two (`rt.test_fs_real`); MemFs's control methods
  stay fake.
- What a test with real `stdio` prints reaches `xo test`'s output as
  on the Go backend (the harness forwards the lines that are not its
  own protocol).

Consequence and known difference: natively, a test that declares
`clock`, `net`, or `proc` runs on the real clock (there is one timer
heap per process, and a socket or child process wait is not a virtual
wait), while the Go backend keeps its virtual clock unless the test
declares `clock`. A `uses net` test that sleeps on `os.clock` therefore
sleeps for real natively. `os.proc.exit` ends the test as without
`--real` (it never exits the test process). Test:
`cmd/xo/testdata/nogc/real` (nine tests: each effect, a fake kept next
to a real one, real output) in TestNoGCTests, with and without `--real`,
the same report as the Go backend on LLVM and arm64.
