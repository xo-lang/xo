# A test that sets a missing XO_CLI makes every later xotool test find no xo

Status: Observed, fixed (2026-11-06, linux/arm64 host 2, xo-lang-4)

## Symptom

`go test ./internal/xotool` failed TestTierSepcompWarm in 0.00 s with
`XO_CLI=/tmp/TestBuildResultNeverNil.../no-such-xo: stat ...: no such
file or directory`, a path from another test. TestBuildTempCleaned had
been skipping with "no xo" on every run, which a passing package hides.

## Cause

xotool.Path resolves the xo once per process (pathOnce) and keeps the
error too. TestBuildResultNeverNil sets XO_CLI to a missing file with
t.Setenv; t.Setenv restores the variable afterwards, but not Path's
cached result, so every test after it in the package saw the missing xo:
a test that skips on "no xo" skips silently, one that calls Command
fails.

## Fix

TestBuildResultNeverNil resets Path's state before it runs and again in
t.Cleanup (build_test.go resetPath). A test that changes XO_CLI (or
anything else Path reads) must do the same. A test that needs xo should
fail, not skip, when the package expects one, or its skip goes unseen.
