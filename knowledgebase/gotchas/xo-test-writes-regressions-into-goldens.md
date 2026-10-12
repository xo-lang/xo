# `xo test` on a golden directory writes regressions_test.xo into it

Status: Observed (2026-10-12, decision 0178 stage 1)

## Symptom

After `scripts/selfhost-gogoldens.py` ran with an xo1 that writes
regression tests, `git status` showed untracked
`internal/gen/testdata/golden/test_contracts/regressions_test.xo` and
`test_loop_bound/regressions_test.xo`.

## Cause

`xo test` (and now `xo1 test`) turns every shrunk contract fuzz failure
into a test in the module's regressions_test.xo (core 9.2). The goldens
`test_contracts` and `test_loop_bound` fail contracts on purpose, so any
`xo test` of them writes the file into the testdata directory.

## Fix

Scripts that run `xo test` or `xo1 test` on a golden remove a
regressions_test.xo they created (selfhost-gogoldens.py does). Do not
commit such files unless they are meant to be part of the golden.
