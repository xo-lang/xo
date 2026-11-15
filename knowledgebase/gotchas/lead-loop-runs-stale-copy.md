# A long-running lead loop keeps running the copy it started with

Observed 2026-11-15 (lead and Xo Lang 2). Status: Observed.

**Symptom.** irman.sh's deferral cap ((private commit)) was installed, yet the
loop hourly.sh had started from cron (PID 20128) kept logging "pass2 has
leases waiting: later" with no count and never queued.

**Cause.** install.sh replaces each tool with an atomic `mv`, so a
running bash keeps reading the old file's inode: the loop ran the old
copy until something stopped it. Stopping it needed a kill of a process
no session had started, which the safety rules leave to the owner. And
each `irman.sh --once` wrote its own PID over the loop's PID file, so
hourly.sh saw the loop as dead and could start a second one.

**Fix.** irman.sh compares its file's hash with the one it started with
between jobs and execs the installed copy when they differ (same PID,
after `bash -n`); `touch ~/xo-lead/notes/irman/stop` makes it cancel its
job and exit, and hourly.sh starts none while the file exists; `--once`
leaves the PID file alone. Any new long-running lead loop does the same:
reload on a changed file, stop through a file, never through a kill.
