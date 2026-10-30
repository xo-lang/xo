# A reset offload job leaves the priv lock and the sent ref behind

Status: Observed 2026-10-30

## Symptom

Every linux/amd64 (WSL) offload job on the Windows box ran for 30 minutes
and more with no output (load 0 on the host), and after that runs failed
with "the bundle did not send all necessary objects" / "the bundle did not
fetch" (OFFLOAD-RC=2).

## Cause

Two leftovers of a job whose ssh connection was reset (timeouts and
resets around a WSL VM restart):

- `~/xo-ci/priv.lockdir`, the directory lock around the shared priv
  repository's fetch and worktree change (scripts/lead/offload-amd64.sh),
  stayed: the job died without running its EXIT trap, and every later job
  waited on the lock forever.
- amd64-run.sh advanced `refs/offload-amd64/sent` for any exit code but 2
  and 4, so the reset (255) marked a commit as sent that the host never
  fetched; later bundles held only the commits after it.
- The fetch cut off by the reset left `refs/amd64/offload-amd64-97958`
  in the host's priv repository pointing at a missing object, so even a
  full bundle failed with "did not send all necessary objects".

## Fix

The lock now records its holder (`$$` and the kernel boot ID) in
`priv.lockdir/holder`, and a waiter removes a lock whose holder is no
longer running or belongs to an earlier boot. amd64-run.sh advances the
sent ref only after a run that got past the fetch (rc 0 or 1), and
offload-amd64.sh drops refs/amd64/* (fetch targets only) and leftover
`tmp_*` packs before each fetch. By hand:
remove the lock directory when no git runs on the host, and
`git update-ref -d refs/offload-amd64/sent` (or `refs/offload-win/sent`)
so the next bundle is full.
