# 0250: Native contract fuzzing

Status: Proposed (claimed 2026-10-16 by xo-fa; the lead's priority item
after decision 0248)

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

## Decision (proposed)

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

To be filled in with the implementation: the cost of the second check,
the targets dropped on the goldens, and tests.

## Alternatives

- A Job per target in the SSA builder that builds the precondition and
  the call from the checked AST of the function: no second check, but a
  second lowering path for calls, `var` parameters, receivers and
  capabilities, which the generated source gets from the checker for free.
- Run contract fuzzing with the Go backend from a native `xo test`: needs
  the Go toolchain, which decision 0248 removed from the default path.
