# The 2-hourly -short check is red on a loaded Mac, a different package each run

Status: Observed (2026-11-16 to 2026-11-17, xw-1, task #193)

- Symptom: every 2-hourly main check from 2026-11-16 09:35 to 2026-11-17
  00:02 failed its `go test -short` step. Each time cmd/xo passed its 20
  minute package limit (`panic: test timed out after 20m0s`), and a
  different other set failed beside it: TestDevGoldensGo and
  TestSelfhostWasm (09:35), internal/gen and TestWarmBuildSequence
  (14:18), internal/arm64 at 600 s (19:35), internal/testtmp
  TestRunTimed (00:02). Each passed alone. mainbisect had nothing to
  bisect ("no failed test": a timeout is no one test), and the red alert
  said nothing about the code.
- Cause: two things at once. The Mac ran fuzz, stress and gates at a load
  of 20 to 60 while the step ran every package at -p=2 -parallel=4, so
  timing tests and the slowest package ran past their limits. And cmd/xo
  itself got slower on main (TestDevEdges about 60 s at 415dd4f41, about
  133 s at 823d6a665 on the linux/amd64 host; bisect in
  notes/xw-1/t193/), so the load pushed it past 20 minutes where it once
  had room.
- Fix: hourly.sh reruns each failed package alone once
  (scripts/lead/short-rerun.sh, tested by short-rerun_test.sh): a package
  that passes alone is a `note: -short PKG: passes alone in DUR` line in
  hourly.log, not a failure; one that fails alone stays red, and the
  summary and mainbisect read the rerun's output, so they name and bisect
  a failure that repeats. The full run's log is kept as
  notes/lead/hourly-short-HHMM-full.log. A note that repeats on one
  package means it is slow enough to fail alone soon: find why (the
  cmd/xo slowdown above).
