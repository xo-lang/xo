# `xo test` writes regressions_test.xo into the module it tests

Status: Observed (2026-10-08, bench/xo/v0.4/03-retry-client; hit again
the same day by the arena experiment's with and without `--arenas` runs)

Symptom: running `xo test` on a benchmark solution leaves a new
`regressions_test.xo` in the solution's directory, an edit to a directory
that must not change.

Cause: core 9.2: a shrunk contract fuzz failure is written as a regression
test into the module (`cmd/xo/regress.go`). 03-retry-client's `backoff`
overflows a Duration under fuzzing (`base_delay: -1ms`, `attempt = 63`),
in every mode.

Fix: run `xo test` on a scratch copy of solutions that must stay
untouched (`cp -R bench/xo/v0.4 "$TMP"/`), or delete the file afterwards.
