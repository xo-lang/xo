# `xo test` on a golden directory writes regressions_test.xo into it

Status: Observed (2026-10-09)

Symptom: after running `xo test` by hand on
`internal/gen/testdata/golden/test_contracts`, TestGolden failed with two
more failing tests, regressions of the contract fuzz cases.

Cause: a failing contract fuzz case is saved as a regression test in
`regressions_test.xo` next to the module (core 9.1), also when the
directory is a golden.

Fix: delete the generated file (`git status` shows it untracked); to try
a golden by hand, copy it to a scratch directory first.
