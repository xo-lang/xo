# go test -run that matches nothing passes

Status: Observed (2026-10-21 onward; Lang 2, Lang 4; fixed by decision 0437)

Symptom: `go test ./internal/arm64 -run TestGolden` (the LLVM package's name, not arm64's `TestGoldenRun`), or any misspelt test name, printed `testing: warning: no tests to run` and `ok`, so a gate that ran no test read as green.

Cause: the testing package treats a -run pattern that matches nothing as success.

Fix: the packages with goldens (internal/arm64, internal/llvm, internal/gen, cmd/xo) call `testtmp.RequireRunMatch(m)` in TestMain after `flag.Parse`: a -run whose first element matches no test or example exits 1 with `-run=X matches no test in this package`. `^$`, -bench, -fuzz and -list runs are left alone. Golden test names: arm64 `TestGoldenRun`, llvm `TestGolden` and `TestGoldenGoParity`.

Observed 2026-11-04 (Xo Lang 2): the guard applied a `-skip A/B` as a skip
of all of A, so `-run TestGoldenTasks -skip TestGoldenTasks/debug_routes`
exited 1 with "matches no test" and ran nothing. A `-skip` with a `/`
skips subtests only, so the guard now ignores it (runmatch.go; the test
covers both shapes).
