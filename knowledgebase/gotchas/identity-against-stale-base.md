# An identity check against the pre-rebase base

Status: Observed (2026-10-19, Xo Lang 3).

## Symptom

A push's overlap gate (pushq re-gates after rebasing onto a moved main)
printed `identity amd64: compared 402, differing 402` for a change that
did not touch code generation (19e374ec, the inline cache key), and the
push went through because the gate script echoed the line and passed.

## Cause

The script built its base xo from the worktree's original base commit,
fixed when the script was written; after pushq's rebase the change sat
on a newer main, which had gained e551121a (mir reuse_loads, an amd64
code change). Every amd64 output differed because of main's own commit.
Against the true parent, 19e374ec was identical on amd64 and arm64
(402 compared, 0 differing each).

## Fix

Build the identity base from the change's parent as it is when the
check runs (`git rev-parse HEAD~1` after the rebase, or the merge-base
with origin/main), and make any nonzero "differing" fail the gate:
scripts/arm64-identity.sh exits 1 on a difference; a wrapper must keep
that exit status rather than print the line.
