# An offload prune that cannot reach its target walks every cache before every job

Status: Observed (host 2, 2026-11-16)

## Symptom

Every job on host 2 spent 10 to 20 minutes or more in offload-prune.sh
before it started, and a cleanup job could be stopped while still inside
the prune. /work stayed under the 30 GB target the whole time.

## Cause

`before` mode runs all its passes whenever /work is under OFF_FREE_TARGET.
On host 2 most of the used space was outside what the prune may remove
(52 GB of gate bundles under 6 hours idle left by a retry storm,
gotchas/offload-gate-refused-try-leaks-bundle.md), so no run could reach
the target and each job repeated the full walk of every cache, the job
directories and /work/in, freeing nothing. Separately, `lru` counted a
file toward its goal before removing it, so a file it could not remove
(another user's, in a read-only directory) counted as freed and ended
the pass early.

## Fix

A walk that leaves /work short and frees under OFF_PRUNE_DRY_MB (1 GB)
writes `~/.cache/xo/.prune-dry` with the free MB; for OFF_PRUNE_DRY_MIN
(60) minutes later jobs skip the walk unless free space fell by
OFF_PRUNE_DRY_MB since. `lru` adds a file's size only after the file is
gone. Tests: scripts/lead/offload-prune_test.sh cases 5 and 6 (3 FAIL
before the fix, 36 of 36 after). When the prune reports "skipped" and
the disk stays short, find what fills /work by hand: the prune cannot
reach it.
