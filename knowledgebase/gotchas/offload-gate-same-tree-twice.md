# Two offload-gates of one worktree share their bundle

Status: Observed (Xo Lang 1, 2026-11-08)

## Symptom

Two `offload-gate.sh` runs started at once for the same worktree (one
with `short`, one with `llvm`, to split a gate that hit its lease): the
second failed at once with "bundle did not fetch" and ran nothing.

## Cause

offload-gate names everything after the worktree and its HEAD
(`name=$(basename WT)-$(short HEAD)`): the bundle it puts on the host
(`/work/in/gate-NAME.bundle`) and the run's work directory. Both runs had
the same name, so the first run's cleanup removed the bundle the second
was about to fetch (or one overwrote it while the other read it).

## Fix

Run one offload-gate per worktree at a time: list every step in one run
(the lease length comes from the steps since 1963a5eec), or start the
second when the first prints GATE-DONE. To gate two step sets in
parallel, use a second worktree at the same commit (its basename, and so
its bundle name, differs).
