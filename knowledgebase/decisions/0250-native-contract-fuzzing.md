# 0250: Native contract fuzzing

Status: Accepted (lead, 2026-10-16); implemented the same day (xo-fa)

## Context

Contract fuzzing (core 9.2) runs every function with contracts on
generated inputs that satisfy its `requires`, shrinks a failure, and
writes a regression test. Only the Go backend does it (selfhost/gen
fuzz.xo emits a test function per target; std/rt drives generation,
shrinking, and regress.go's records). Since decision 0248 made native the
default backend, `xo test` skips it unless `--backend=go`, and prints how
many contracts it did not fuzz.

The native backends already have every other piece: property tests
(`test "..." for (x: T)`) lowered with generated inputs (selfhost/ssa
testmode.xo prop_inputs, `xo.gen[T]`), cases run and shrunk by the
driver (selfhost/xo test.xo run_property), and contract checks
(`requires`, `ensures`, loop bounds) compiled into debug builds, faulting
on a violation.

## Decision

Native contract fuzzing is a generated property test per target, checked
with the program:

1. `xo test` on a native backend checks the program, then picks the
   targets as the Go backend does (gen.contract_fuzz_targets: a function
   with a contract, no type parameters, generatable or capability
   parameters).
2. For each root module with targets it writes, in the checker's overlay
   (never on disk), a file of property tests, one per target:
   `test "contract fuzz: F" for (p1: T1, ...) { if !(REQUIRES) { return }
   F(p1, ...) }` with the parameter types and `requires` conditions as
   written, the function file's `use` lines, `var` parameters as `var`
   locals, capability parameters from the test's `os`. The checker lists
   overlay files of a module's directory (load_dir) like files on disk.
3. It checks again with the overlay; a generated test that does not check
   (a type not expressible in the test's file) is dropped and counted as
   not fuzzed, with its reason under -v.
4. The tests lower and run as property tests: generated inputs, cases,
   shrinking, the reports. A failure is reported under the test's name
   "contract fuzz: F" with the shrunk inputs, as on the Go backend.
5. The driver turns a shrunk failure into the Go runtime's
   RegressRecord (call template, result kind, parameters from the
   report's `name = value` lines) and write_regressions appends the
   regression test, so both backends write the same file.
6. The "N contracts not fuzzed" line remains only for targets dropped in
   step 3.

## Consequences

- As built (selfhost/xo fuzzsrc.xo, test.xo, main.xo check_program_ov,
  selfhost/check load.xo): the generated file of `<dir>/<f>.xo` is
  `<dir>/<f>.xo_contract_fuzz_test.xo`, in the overlay only. Its
  diagnostics are never printed or kept; an unused name there (the
  copied `use` lines) is ignored; any other error drops the test (at most
  three checks), and the target is counted as not fuzzed with the reason
  under `-v`. A target taking `os: Os` is not generated (a test cannot
  pass one), as on the Go backend's regressions.
- The tests are named `contract fuzz: MODULE.F`, the Go backend's names;
  `--run` matches them like any test; they are listed where the Go backend
  lists them (after their function, before the file's next test block)
  and report at the function's location, without the generated file's
  traceback frames.
- Parity: the same targets (gen.contract_fuzz_targets), 100 cases, case
  seeds `seed * 1000003 + i`, shrinking, and the regression record and
  file; the native and Go generators draw differently, so the first
  failing case can differ (case 7 against 2 on halver.half) while the
  shrunk input and the regression test agree. cmd/xo
  TestRegressionCrossBackend records a regression on each backend and
  replays it on the other, and checks the two files are the same.
- Cost: the program is checked twice in a native `xo test` with targets.
  A module of 20 contract functions (Int, Str, List[Int] parameters, `use
  std/json`), three runs each with the caches off: the check phase 192 to
  274 ms before (20 targets skipped) and 401 to 443 ms after (20 fuzzed);
  the whole `xo test` 0.73 to 0.94 s before, 0.98 to 1.02 s after,
  including 2000 cases.
- Decision 0248's "not fuzzed" note now applies only to dropped targets.

## Alternatives

- A Job per target in the SSA builder that builds the precondition and
  the call from the checked AST of the function: no second check, but a
  second lowering path for calls, `var` parameters, receivers and
  capabilities, which the generated source gets from the checker for free.
- Run contract fuzzing with the Go backend from a native `xo test`: needs
  the Go toolchain, which decision 0248 removed from the default path.
