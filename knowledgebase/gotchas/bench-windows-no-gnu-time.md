# xo bench --ab cannot run on Windows under Git Bash

Status: Observed (2026-10-30)

**Symptom.** `xo bench --ab` on the owner's windows/amd64 box stops at round
0 with `A: /usr/bin/time: executable file not found`.

**Cause.** xo bench times each round with GNU time (`/usr/bin/time -f`) for
wall, CPU and peak memory. Git Bash ships no `/usr/bin/time`, and Windows has
no equivalent on PATH.

**Fix.** xo bench runs the child alone on Windows. Its cpu_ms is what
`Proc.children_usage()` (decision 0426: GetProcessTimes of each waited
child) grew by across the round, read before and after it; bench runs
one child at a time, so the difference is that child's (2026-10-30).
max_rss_kb is still not measured there, and the note under the table
says so (the row is absent, not zero): `children_usage` keeps the
largest peak working set of every child waited so far, so in `--ab`
rounds the smaller side would read the larger side's peak. A per-child
figure needs the child's own usage (a 0426 addendum, the owner's call).
