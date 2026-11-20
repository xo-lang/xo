# Files a gate step keeps in its logs dir are gone after an offload gate

Status: Observed 2026-11-20 (task #476; #471's diagnostic, af8ba5d89)

## Symptom

A test writes the evidence of a rare failure (both binaries of a warm and
cold build, XO_WARMSEQ_KEEP) under the gate's `$logs`, the failure happens
on an offload host, and nothing is there to fetch: only the failed step's
log comes back (/work/offgate-keep/NAME-STEP.log.gz).

## Cause

offload-gate-remote.sh runs scripts/gate.sh in a fresh worktree `$W` with
logs in `$W.logs`, and removes both when the gate ends (its cleanup trap).
It keeps only the logs of failed steps, gzipped. So af8ba5d89's
`XO_WARMSEQ_KEEP=$logs/warmseq-keep` never kept a pair on a host.

## Fix

Keep evidence in `$gate_keep` (scripts/gate.sh), which is GATE_KEEP when
set, else `$logs`. offload-gate-remote.sh sets GATE_KEEP to
/work/offgate-keep/NAME. When files are there it prints
`OFFGATE-KEEP DIR (kept binaries: ...)`, and it prunes them after 3 days.
Fetch them with offload.sh --get. A new step that keeps files writes
under `$gate_keep`, never `$logs`.
