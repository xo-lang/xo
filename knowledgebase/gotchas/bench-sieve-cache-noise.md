# sieve's cycles swing by 4 percent between runs of the same binary

Status: Observed (2026-10-08, bench/perf/llvm/xo/sieve, Apple M4 Pro,
other agents compiling)

Symptom: the same sieve binary measured 385 to 420M "cycles elapsed"
within one interleaved round set (interquartile range of the per round
ratio to LLVM about 5 percent), while nbody's spread was under 0.2
percent at the same time. Medians of 9 to 15 runs disagreed on a 1 to 2
percent change from one set to the next, in both directions.

Cause: sieve walks a 10 MB `List[Bool]` (five times per run, freshly
allocated each time), which lives in the L2 cache the performance cores
share; other processes' builds on the same cluster evict it, and the
kernel's page faults for the new buffer land in the count too. nbody
works in a few cache lines and only measures the core.

Fix: for a change to sieve, run 41 to 101 interleaved rounds in random
order while the load average is under 8 (wait for it), and compare the
median of the per round ratio against the same round's baseline, not
medians of separate sets. The cache resident variant (n = 1e6) is no
quieter. To see a front end effect directly, use the per phase
bottleneck counters (`xctrace` "CPU Counters",
infrastructure/toolchain.md): sieve's phase past sqrt(n) (flag tests
only) is easy to pick out (processing near 0, discarded above 25
percent).
