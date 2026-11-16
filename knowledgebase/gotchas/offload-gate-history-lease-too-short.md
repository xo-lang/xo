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

Rerun with `OFFGATE_HIST=0` (the formula's lease) or an `OFF_MAX_MIN` above
the formula; never below it (gotcha of the offload budget). A gate chain of
several cold pushes in a row sets `OFFGATE_HIST=0`.
