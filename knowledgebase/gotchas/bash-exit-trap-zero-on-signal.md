# A stopped bash script's EXIT trap sees status 0

Status: Observed 2026-11-20 (task #538).

## Symptom

xw recorded task #531 "pushed ((private commit))" and done, and pushq's
`pushq-run/l531.result` said `rc=0`, but (private commit) was not on
origin/main. Lang 1 had stopped its push chain while pushq waited on its
fast checks.

## Cause

A non-interactive bash killed by SIGTERM (or SIGINT, SIGHUP) still runs
its `trap ... EXIT`, and `$?` in the trap is the status of the last
command that finished, not the signal's 143. pushq's `trap 'pq_end $?'
EXIT` read 0 as "pushed" and recorded the push on the task. A
`PUSHQ_CHECK_ONLY=1` run, which exits 0 before any push, did the same.

## Fix

Never take `$?` in an EXIT trap as proof the work finished. pushq.sh's
pq_end records a push only when this run's log has a `pushed SHA` line
(written by the lane after `git push`) and `git merge-base --is-ancestor
SHA origin/main` holds; otherwise it records rc 1 with "no push
recorded". `scripts/lead/pushq_record_test.sh` covers a stop before git
push, a real SIGTERM, a SHA not on main, and a landed push.
