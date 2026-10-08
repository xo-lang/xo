# nilaway traces name the work directory, so base and variant must share it

Status: Observed (2026-10-08, `bench/harness/score.py planted`)

Symptom: rescoring the Go planted bugs gave plain + strict 23/30 instead of
the published 13/30: nilaway was a "new finding" on every variant of tasks
01, 03, and 04, including the logic bugs.

Cause: a strict tool counts only for findings the unmodified base does not
have, and findings are compared as text with the work directory's absolute
path removed. nilaway's trace lines also print the module directory's base
name relative (`m/main_test.go:47:29`), which that removal does not catch.
The scorer built the base in `<tmp>/base` and each variant in `<tmp>/m`, so
the same nilaway finding read differently. The original
`run_detection.py` reused one directory, which hid the problem.

Fix: build the base and every variant in a directory with the same base
name (`<tmp>/<task>`). Any detection that diffs tool output between two
trees must keep the tree's path, including its last element, identical.
