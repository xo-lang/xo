# `xo test` on a golden program writes into the repository

Status: Observed (2026-10-10)

Symptom: after a survey ran `xo test` over every
`internal/gen/testdata/golden/test_*` program (to look at the generated
Go), `TestGolden/test_contracts` and `test_loop_bound` failed: four
tests where want.txt has two, the extra ones named
`regression: ... (seed 1)`.

Cause: contract fuzzing (core 9.2) writes each shrunk failure as a test
in `regressions_test.xo` next to the source, and those two programs fail
contract fuzzing on purpose. `git add internal/gen` then committed the
new files (removed in 8549692).

Fix: build golden programs through the Go API (`build.Build`, as the
tests and `TestGeneratedGoVets` do), or copy the program to a temporary
directory before running `xo test` on it. Check `git status` for new
files under testdata before committing a directory.
