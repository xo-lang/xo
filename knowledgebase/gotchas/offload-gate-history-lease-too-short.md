# offload-gate's history lease stopped a cold gate at 10 minutes

Observed 2026-11-16.

## Symptom

A `selfbuild seedbuild stdfast` gate on host 2 (l2-perf88) ended with rc 124
and "offload.sh: the job ran past OFF_MAX_MIN=10 minutes and was stopped",
right after its precheck; nothing had failed.

## Cause

offload-gate.sh takes the lease from gate-history.tsv when there are at
least 5 passing runs of the same steps and xo state: the p90 times 1.3, at
least 10 minutes ("lease 10 minutes from 21 passing runs of steps
seedbuild,selfbuild,stdfast (cold xo): p90 391 s"). It replaces the formula
(49 minutes here, which adds 20 for building xo on the host). With 3 slots
busy on the host this cold run needed longer than the history's p90.

## Fix

Since task #121 (2026-11-16) a cold host's history lease is the p90 times
1.3 plus the build allowance (`OFFGATE_XOBUILD_MIN`, 20), and never under
the formula; a warm host keeps the history's lease
(scripts/lead/offload_gate_test.sh, the cold and warm cases). Before that
fix: rerun with `OFFGATE_HIST=0` (the formula's lease) or an `OFF_MAX_MIN` above
the formula; never below it (gotcha of the offload budget). A gate chain of
several cold pushes in a row sets `OFFGATE_HIST=0`.

## A warm gate on a slower host (observed 2026-11-20)

xo-4c's warm `seedbuild selfbuild stdfast stdobj` gate t239b-(private commit)
got "lease 10 minutes from 12 passing runs ... p90 362 s times 1.3 (the
formula gave 32)" and was cut at 10 minutes on host 3 (rc 124) with no step
past precheck. The history pooled host 2's and host 3's rows (host 3 is a
container on the Mac, at load 50 then), and a cut run left no row, so the
history never learned. Since task #532 the lease is taken from the gate
host's own rows, a cut (rc 124) records a row, and a cut of the same steps
and xo state on that host in the last `OFFGATE_HIST_CUT_DAYS` (7) days
keeps the formula's lease (offload_gate_test.sh, the per-host cases).
