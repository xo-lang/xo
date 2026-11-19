# A job run inside heavyd's lease records no offload event, so xw sends no end

Status: Observed (2026-11-19, xw-1, task #420)

## Symptom

Lang 2 ran two `ab-host.sh` --ab runs (l2bbpo2, l2bbpc2) in one
background Bash call. They ended about 11:47 with exit 0, but no message
reached the session: the Claude Code task notification was lost (a
context compaction fell near the end), and xw sent nothing either, so xw
showed Lang 2 idle for 30 minutes with nothing queued. xw's events table
held no offload event for either run.

## Cause

Two gaps, each enough on its own:

- With no `OFFLOAD_ENV`, ab-host.sh runs itself again inside heavyd's
  arm64 lease, and inside a lease (`HEAVY_LANE` set) offload.sh records
  no `offload.start` or `offload.end` (it skips them so its own run
  inside its lease is not counted twice). So the run left no event.
- xw's notifier and hook inbox sent only `job.end`, `gate.end` and
  `push.end`; an `offload.end`, when there was one, was never sent.

## Fix

- ab-host.sh records `offload.start` and, from its EXIT trap,
  `offload.end` with its rc, named `ab-host-NAME`, from the entry run
  (lease or not), and runs offload.sh with `XW_OFFLOAD=0` so the run is
  not recorded twice (scripts/lead/ab_host_events_test.sh).
- xw sends an `offload.end` whose run took 2 minutes or more
  (`OFFLOAD_END_MIN_S`; shorter ones are mostly foreground probes) as
  `offload NAME: pass` (workstream test/work_test.sh, #420).

A script that wraps its own work in `heavyd lease` and calls offload.sh
inside it must record its own events the same way.
