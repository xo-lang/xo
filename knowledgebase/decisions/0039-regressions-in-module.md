# 0039: Regression tests are written into the module

Status: Accepted (2026-10-08)

## Context
Core 9.2 said shrunk failures go to `testdata/regressions/<test>.xo`. With
decision 0006 (a directory is a module), that would be a separate module that
cannot call the private function under test.

## Decision
`xo test` writes a shrunk contract fuzz failure to `regressions_test.xo` in the
module, as `test "regression: <function> (seed N)"`, only after a copy of the
module type checks with it. Property test failures are reported, not written.

## Consequences
The task 03 overflow became a 5 line regression test that fails
deterministically. Values are rendered from `Debug` output, so types without a
literal form (for example `Time`) cannot be written; `xo test` reports why.
