# A cancel file also cancelled the next run with the same name

Status: Observed (2026-11-19, xw-1; fixed in heavyd by task #440)

## Symptom

I filed `~/xo-lead/locks/cancel/xw1-425mon-(private commit)` to move an
offload-gate lease off host 2, stopped the chain that owned it, and
reran the same worktree on host 4 at once. The rerun got the same lease
name (offload-gate names a lease after the worktree and its HEAD), and
heavyd cancelled it 2 s after it started: `CANCEL lease
xw1-425mon-(private commit) ... on pass4 stopped after 2s`, rc 130.

## Cause

A lease takes its cancel file on the next 2 s tick. Stopping the chain
ended the host 2 lease first (`end lease ... rc -1`), so the file was
never taken and sat in `locks/cancel` until the next run with that NAME
or PID found it. The directory held 11 such leftovers that day.

## Fix

Since task #440 a run takes only a cancel file last touched after the
run began (`runBegan` in scripts/lead/heavyd/client.go). An older one
was for an earlier run: the run removes it, prints `heavyd: removed
PATH: older than this run`, and goes on. Touching the file again (the
queued line's `touch`) renews its time, so a cancel filed for the
current run still works. offload.sh's no-lane wait (task #333) does the
same: a file there before its wait began is removed with that line.
Tests: `TestLeaseCancelFiledBeforeItBegan`,
scripts/lead/offload_nolane_cancel_test.sh.

With an older heavyd, check `ls ~/xo-lead/locks/cancel/` for the name
before rerunning a job you cancelled.
