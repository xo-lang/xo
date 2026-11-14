# A stopped offload-gate run deletes the files of a rerun with the same name

Status: Observed (2026-11-13)

## Symptom

A rerun of `offload-gate.sh WT seedbuild` failed twice on linux/arm64 host 1
for the 0.51.0 respin: first "the bundle did not fetch"
(`/work/in/gate-rel-0510r-d039e43ba.bundle` missing), then, after xobuild,
every step reported "not a git repository" and `go.mod file not found`,
because the checkout `/work/gaterel-0510r-d039e43ba` was gone.

## Cause

The gate's name is the worktree's basename plus the commit
(`rel-0510r-d039e43ba`), and the bundle and the host checkout are named by it.
Stopping the local offload-gate.sh (kill of its PID) after its heavyd lease had
started leaves the job on the host running; when it ends it removes its bundle
and checkout, which by then belong to the rerun of the same name.

## Fix

Do not rerun under the same name while a stopped run's host job may still be
alive. Gate from a second worktree at the same commit (`git worktree add
--detach ~/xo-wt/OTHER COMMIT`), whose basename gives a new gate name, or wait
until the old job's log ends. Stop a queued run with the cancel file its lease
prints (`~/xo-lead/locks/cancel/ID`) instead of killing it once it may have
started. See also feedback on orphaned leases (a stopped chain keeps its host
lease).
