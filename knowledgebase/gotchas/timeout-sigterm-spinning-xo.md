# `timeout N prog` never ends a spinning Xo program

Status: Observed (2026-10-21; again 2026-11-02, below)

## Symptom

A gate's check that a program hangs (`XO_PREEMPT=0 timeout 10 ./prog`,
expected exit 124) ran for over two hours in a Docker container: the
program at 99% CPU, `timeout` still waiting.

## Cause

`timeout` sends SIGTERM, and Xo's runtime turns SIGTERM into a request
for two-phase shutdown (std/native/llrt stop.xo) that tasks see at their
next poll or runtime call. A task spinning in a loop with preemption off
(XO_PREEMPT=0) or in runtime code that does not poll never sees it, so
the process keeps running. `timeout` waits for the child without a
second signal unless told to.

## Fix

Use `timeout -s KILL N` (exit 137) for a check that expects a hang, or
`timeout -k 5 N` to follow the TERM with a KILL. Any harness that stops
an Xo program with SIGTERM needs the same fallback.

## Again, 2026-11-02: seven programs ran 10 hours on an offload host

scripts/coll-diff.sh ran each generated program as `timeout 10 prog`
(plain). Seven Go backend programs of overnight Coll runs ignored the
TERM, kept 105% CPU each for about 10 h after their job and lease had
ended (their directories deleted, the `timeout` still waiting), and
took the host's load to 40+, slowing every job on it. They were stopped
by PID. Fixed in 2bbe6f734: coll-diff.sh runs `timeout -k 5`.

An audit of scripts/ and scripts/lead for plain `timeout` on a generated
or test program found four more, now `timeout -k 5 N` (windows-batch.sh,
progen-amd64-run.sh, golden-run.sh's $limit, ci/windows-pdb.sh's cdb);
the ci/windows-*.sh runners and lead/offload.sh already used `-k`. A new
script that bounds a program with `timeout` gives it `-k` from the start.
