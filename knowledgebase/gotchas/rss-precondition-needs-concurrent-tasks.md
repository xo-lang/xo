# A resident set precondition assumed tasks ran at once

Status: Observed and fixed (2026-11-05, Xo Lang 2).

## Symptom

cmd/xo TestMemTrimOtherThreads (decision 0505's addendum, Linux only) failed
now and then inside a full `go test ./cmd/xo` run on the linux/arm64 offload
host, and passed alone: 0 of 132 runs alone, 6 of 112 beside a full cmd/xo
run. Every failure was the precondition, "at least 60 MB resident before the
trim" (31 to 54 MB); the trim itself left at most 18 MB (bound 40) in every run.

## Cause

The program spawned 8 tasks that each allocated 1280 blocks of 12,000 bytes
and freed them at the end. The 60 MB floor assumed the 8 tasks held their
blocks at the same time. On a loaded machine fewer threads ran them, partly one
after another, and a later task reused the blocks an earlier one had freed, so
the peak resident set stayed low.

## Fix

The tasks hold their blocks until all 8 have allocated: each sends on a ready
channel and waits on a go channel, and a coordinator task sends the go once it
has 8 readies. Channels park the task, not its thread, so this works with any
number of threads (checked with XO_THREADS=1, 2 and 8).

## Avoid

A test that measures memory (or time) of work spread over tasks makes the
overlap it needs explicit, with a barrier, instead of relying on the
scheduler to run the tasks at once.
