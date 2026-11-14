# A stdproven batch past OFF_MAX_MIN loses every result, finished ones too

Status: Observed (2026-11-14, Xo Lang 3)

## Symptom

`stdproven-offload.sh WT NAME -- batch fuzz ... + stress ... + race ...`
on host 2 ended with `OFFLOAD-RC=124` and `offload.sh: the job ran past
OFF_MAX_MIN=180 minutes and was stopped`. Three fuzz steps had passed
(`stdproven batch: rc=0`), yet no evidence file reached the worktree.

## Cause

The batch copies its evidence back only when the whole job ends; a stop
at OFF_MAX_MIN (default 180) ends it first. One fuzz step can take 70
minutes (url), stress 30 and race about 20, so ten steps pass 180.

## Fix

Size each batch under the limit from those times, or set OFF_MAX_MIN
for it (the non-exclusive lane passes it through to offload.sh). To
recover finished steps of a stopped batch, fetch them from the host:
`OFFLOAD_ENV=offload-arm64b.env ~/xo-lead/agents/offload.sh --get
/work/sp<NAME>/std/evidence/MOD/KIND.md LOCAL` (the run directory is
named after the batch; check its commit and source lines against the
module's current hash before committing).
