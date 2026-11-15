# A running lane lease ran 2h25m past its -max=90, and a cancel file could not stop it

Status: Fixed 2026-11-14 (aws-43, for the lead; Lang 2 found it)

## Symptom

Lang 2's offload gate `mon0640-(private commit)` (a memory lease, pass2, so all
three of host 2's slots) started at 17:00:28 with `-max=90`. Its log on
the Mac showed the header line and nothing more. It held the lane until
Lang 2 stopped its own lease PID at 19:26 (heavyd.log: `end lease
mon0640-(private commit) in pass2 rc -1 after 2h25m45s`). Touching
`locks/cancel/PID` did nothing: the queued line said "to drop it before
it starts", and that was all it did.

## Cause

- **Nothing on the Mac bounded a running lease.** `-max` only went to
  heavyd for scheduling (backfill, `long`). The only limit was the host's
  `timeout -k 60 90m` around the job in offload.sh's wrapper. For this
  run it did not end the job: host 2's stopped.log has no "left after the
  job ended (exit 124)" entry for run 1794693628-57412-28819, only the
  stop from the Mac at 00:26:28Z, which needed KILL for two processes
  (55787, 31582) that outlived TERM. So a job the host's timeout does not
  end holds the lane for as long as the Mac side waits, which was
  forever.
- **The cancel file was read only while waiting.**
- **A header-only log was normal.** offload-gate-remote.sh wrote all of
  gate.sh's output to `gate.out` and printed it at the end, so every
  offloaded gate showed only its header until it finished. Nobody could
  tell a slow gate from a stuck one.
- Found on the way: when offload-gate.sh moved a gate (heavyd's exit 78,
  or 77), it removed the bundle on the host it left with an `offload.sh -`
  job. That job waits for one of that host's slots, so a gate moving off
  a busy host 2 sat 26 minutes on an `rm` (aws-43's and mbfatal's gates,
  19:10 to 19:37; they were the two "nolane offload-gate.sh" entries on
  pass2).

## Fix

- `heavyd lease` stops a running lease past `-max` plus
  `HEAVYD_MAX_GRACE` (10 minutes: the host's limit counts from its slot,
  after the bundle and the copy). It stops the job's process group, sends
  the exit so the slots go, exits 124, and writes an `OVER lease ...` line
  to heavyd.log and to notes/lead/bottleneck.log.
- A cancel file (`locks/cancel/PID` or `/NAME`) also stops a running
  lease: exit 130, with a `CANCEL lease ...` line. The queued line now
  says "to drop it, or to stop it once it runs". heavy.sh's line says
  that a started run is not stopped.
- `HEAVYD_PROGRESS=FILE`: while a lease runs, no line written to FILE for
  `HEAVYD_STALL_MIN` (30) minutes logs one `STALLED lease ...` line to
  heavyd.log and bottleneck.log, and marks the lease `STALLED: no line
  since HH:MM` in `heavy.sh --status`. A new line clears the mark.
  offload-gate.sh passes its log, with 45 minutes when the llvm step runs
  (it ends after up to 32 minutes on host 2).
- offload-gate-remote.sh prints `offload-gate: step STEP ended after N s`
  as each step ends (it reads gate.sh's times.tsv every 15 s). The Mac
  log now moves once per step.
- `offload.sh --rm /work/in/FILE` removes a file over a plain ssh, with no
  slot. offload-gate.sh's move cleanup uses it.
- Tests: heavyd TestLeaseRunningLimits (OVER, CANCEL, slots freed, both
  logs), TestLeaseStalled (STALLED in --status, cleared by a new line, one
  bottleneck.log line); offload_test.sh's `--rm` cases.

## Still open

Why the host's `timeout` did not end the job at 90 minutes. The two PIDs
that needed KILL suggest a process that ignored TERM, or one stuck in
uninterruptible I/O. /work was at 11 GB free at the start (pruned to 17).
The Mac-side bound makes this stop the lane either way. Finding the cause
needs the processes' state while it happens: read `/proc/PID/stat` and
`wchan` of the job's group on the host the next time a lease shows
STALLED.
