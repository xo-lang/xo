# xo bench --ab cannot run on Windows under Git Bash

Status: Observed (2026-10-30)

**Symptom.** `xo bench --ab` on the owner's windows/amd64 box stops at round
0 with `A: /usr/bin/time: executable file not found`.

**Cause.** xo bench times each round with GNU time (`/usr/bin/time -f`) for
wall, CPU and peak memory. Git Bash ships no `/usr/bin/time`, and Windows has
no equivalent on PATH.

**Fix.** xo bench runs the child alone on Windows and measures wall_ms
only; under the table it prints that cpu_ms and max_rss_kb are not
measured there (the rows are absent, not zero). They arrive with decision
0426's `Proc.children_usage()` (GetProcessTimes and the peak working set
of each waited child), which bench will read around each round. Until
then a Windows CPU or memory comparison has no tool; time wall only.
