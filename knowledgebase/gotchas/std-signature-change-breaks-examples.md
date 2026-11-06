# A std type change breaks examples that only the full tier checks

Status: Observed (2026-11-06, Xo Lang 2; full tier 55 at edf1b6e3b)

## Symptom

After f04a8b4ca gave std/sql's `Db` an effect parameter (decision 0534),
examples/notes-service stopped checking (XO0701 "the expected function
type allows only `net.dial`, but performs `Db.fx`" in its `auth`), and
cmd/xo TestNoGCNotesServer, TestNoGCTestLeaks/notes-service,
TestNoGCTests/notes-service and internal/gen TestExampleArenaDiff and
TestExampleArenaServeDiff failed. The push's gate had checked
stability/notes, the program it migrated, and passed; `-short` and the
two hourly checks do not build examples/, so only the full tier saw it.

## Cause

A std signature change that can turn user code into an error (a new type
or effect parameter, a narrowed type, a removed method) reaches every
program written against std, and the repository keeps several outside
std's own tests: examples/*, stability/*, bench/perf/*, and the copies of
examples/notes-service that cmd/xo and internal/gen test.

## Fix

When a std change can break callers, the gate runs `xo check` on every
program directory under examples/, stability/ and bench/perf/ with the
new xo (`for d in examples/*/ stability/*/ bench/perf/*/; do XO_STD=$PWD/std bin/xo check "$d" || echo "FAIL $d"; done`)
and migrates the ones that fail in the same push, with the
std/changes/MODULE.md migration line naming them. For the 0534 case:
auth, routes and run take `sql.Db[()]` (968655596).
