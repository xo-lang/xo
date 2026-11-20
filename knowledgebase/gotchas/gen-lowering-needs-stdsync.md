# A Go backend lowering change makes std/rt's generated Go stale

Status: Observed (2026-11-20, task #429)

## Symptom

A compiler/gen change (a `match` on Str or Int literals lowered to a Go
`switch`, 9aa99ba59) passed its own gen test and golden, then failed its
offload gate at the first step: "stdgen: FAIL: std/rt's generated Go is
stale: run go run ./scripts/stdsync and commit". The gate stopped there
(fail fast), so selfbuild and seedbuild never ran.

## Cause

std/rt/std_gen_*.go (mime, multipart, netip, regexp, uniclass, url, and
more) is std's Xo code compiled by the Go backend and checked in. Any
change to what compiler/gen emits for code std uses changes those files;
stdgen -check compares them with a fresh generation.

## Fix

After any compiler/gen change, before the gate, rebuild xo and run
`XO_CLI=WT/bin/xo go run ./scripts/stdsync` under heavy.sh, then commit the
std/rt/std_gen_*.go diff with the change (here 6 files, 2301 lines in and
2627 out). Read a hunk of the diff: it should be the new lowering only.
