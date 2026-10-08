# Benchmark times on the development host drift between runs

Status: Observed (2026-10-08, bench/perf sortint)

Symptom: the same Go binary measured 149 ms on one run of
`bench/perf/run.sh` and 135 ms twenty minutes later; an Xo binary measured
159 ms in back to back runs and 141 ms when interleaved with the Go binary.
A change looked like a 5 percent regression that was not there.

Cause: the host (Apple M4 Pro laptop) changes clock speed with load and
temperature; a binary run many times in a row heats the cores. Best of N
does not remove a drift that lasts minutes.

Fix: compare before and after in the same session and close in time, run
the two binaries interleaved (`for k in 1 2 3 4 5 6; do a w; b w; done`),
and treat differences under about 3 percent as noise. Report the Go time of
the same run next to every Xo time, as `run.sh` does.

Update 2026-10-08 (arm64 backend): other agents building and testing on
the same host raised the load average to 40 to 85 for minutes at a
time; Go's fib went from 228 ms to 608 ms within one run. Check
`uptime` before and after a measurement and discard runs with a load
average above a few; `bench/perf/llvm/native.sh` interleaves the three
binaries so a slow period hits all of them.

Update 2026-10-08 (decision 0058): with load averages of 20 to 60 from
other agents, wall time medians of the same binary moved by 25%. macOS
`/usr/bin/time -l` prints the process's own `cycles elapsed` and
`instructions retired`; the minimum of 11 or more interleaved runs of
cycles was stable to about 1% at that load, and an unchanged
instruction count shows a difference is layout, not work
(`gotchas/native-rc-fast-path-costs.md`).
