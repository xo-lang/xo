# A Go test's os.Chdir leaks into every later test

Status: Observed (2026-11-17, task #226)

Symptom: scripts/stdproven TestStressProgramsSkipRSSUnderCheck passed
when run alone (`-run`, as its push gated it) and failed in the hourly
`go test -short` of main at 97036298d with "no stress programs: <nil>".

Cause: TestSourceHashStopsAtRuntime moved to the repository root with
`os.Chdir("../..")` and never moved back, so every test after it ran from
the root. The new test globbed `../../std/evidence/*/stress/main.xo`,
which matches from the package directory only. Other tests had grown
"root or package directory" fallbacks around the leak.

Fix: one `TestMain` moves to the root before `m.Run`, and tests use
root-relative paths; a test that must move elsewhere uses `t.Chdir`,
which moves back when the test ends. Check a package whose tests change
directory with the whole package and `-shuffle=on`, not `-run` alone.
