# The xo test run cache reused a plain binary for --seed

Status: Observed (2026-10-23, decision 0350 stage 2)

## Symptom

A native `xo test --seed=1` passed, while the same command in a gate
failed; the passing run had followed a plain `xo test` of the same
directory.

## Cause

compiler/xo testcache.xo run_key leaves the run-only flags out of the
key (`--run`, `--seed`, `--cases`, ...). Since stage 2, `--seed` and
`--explore` build the deterministic flavor of the test binary, so the
seeded run took the cached thread pool binary.

## Fix

run_key adds `det <bool>` (seed or explore given). A flag that changes
the binary must never be in run_only_flag; TestTestExploreNative runs a
plain run before its seeded ones. When a result differs between two
runs of the same command, check the run cache (XO_BIN_CACHE=off) before
calling it flaky.
