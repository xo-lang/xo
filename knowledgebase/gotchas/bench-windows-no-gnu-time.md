# xo bench --ab cannot run on Windows under Git Bash

Status: Observed (2026-10-30)

**Symptom.** `xo bench --ab` on the owner's windows/amd64 box stops at round
0 with `A: /usr/bin/time: executable file not found`.

**Cause.** xo bench times each round with GNU time (`/usr/bin/time -f`) for
wall, CPU and peak memory. Git Bash ships no `/usr/bin/time`, and Windows has
no equivalent on PATH.

**Fix.** xo bench no longer runs GNU time anywhere (2026-10-30): cpu_ms
and max_rss_kb come from each run's `ProcOutput.usage` (decision 0426's
addendum, a child's own usage from wait4 or GetProcessTimes and
PeakWorkingSetSize), and only macOS keeps `/usr/bin/time -l`, for
instructions and cycles. An earlier fix read cpu_ms as
`Proc.children_usage()`'s growth on Windows and left max_rss_kb out:
children_usage keeps the largest peak of every child waited so far, so
a per-child peak cannot be differenced from it.
