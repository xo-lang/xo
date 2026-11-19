# A WSL offload job outlives its ssh, and a full disk ran it as success

Status: Observed 2026-11-19 (amd64 host; fixed in scripts/lead/offload.sh, task #404).

## Symptom

- The amd64 host's WSL disk filled (956 of 1007 GB, then again within
  30 minutes of a cleanup). An orphaned `xo2` from a stopped job had
  written a deleted file for 2.5 hours at about 260 MB/s; `timeout 20`'s
  TERM did not stop it.
- On the full disk every amd64 job printed `cat: write error: No space
  left on device` and ended with `OFFLOAD-RC=0`: nothing had run.

## Cause

- When the Mac side dies with no trap (a TaskStop of the caller's group,
  `kill -9`), nothing on the host stops the job. Its parent is WSL's
  `/init` relay, which stays, so the job's parent PID never changes, and
  the relay goes on reading the job's output for up to about 2 minutes
  after the ssh is gone (tested: writes 2 s apart failed after 12 s,
  writes 30 s apart after about 2 minutes).
- The job file wrote its body with `cat > FILE <<EOF`; on a full disk the
  write failed, the empty file ran, and bash reported success.
- The stop (`JOB --stop`) first copied its script to /tmp, which fails
  on a full disk.

## Fix

- The job file's watcher writes a heartbeat every 30 s (offload.sh's
  filter drops it); a failed write, or a job past OFF_MAX_MIN plus 5
  minutes, stops the job's session and process tree (TERM, KILL after
  10 s). The stop ignores HUP and TERM: the job shell's exit hung up the
  watcher's group and cut the stop before its KILL.
- A body that cannot be written fails the job with `OFFLOAD-RC=98`.
- The stop is piped to `bash -s` (no file).
- Each job's prune flags `OFFLOAD-ORPHAN` (a live job whose job shell is
  gone) and `OFFLOAD-OVERDUE` (10 minutes past its OFF_MAX_MIN), logged
  in notes/offload-stalls.log.
- Test: scripts/lead/offload_wsl_job_test.sh (TestOffloadWslJob).

To check a fix on the real host, kill the local offload.sh with `-9` by
the PID your own script captured (`$!`), then probe with another job.
