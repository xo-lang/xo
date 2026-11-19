# A clock read per task costs 20% of a small task on linux/amd64

Status: Observed (2026-11-19, task #419)

## Symptom

bench/perf/task map (1,000 items of about 1 us each, 8 at a time) took
145 ms on the linux/amd64 host against Go's 82 ms, while the same program
beat Go on the Mac. A CPU profile showed `sysio.mono_ns` near the top.

## Cause

The native runtime timed every run of a spawned task (pool.xo site_clock
and site_note, decision 0359) to learn whether its spawn site is small.
`cpu.ticks()` is `cntvct` on arm64 (a few cycles) but a vDSO
clock_gettime on amd64, about 140 ns each on the WSL2 host, two per run.

## Fix

Time a run from a site already known to be small only every 8th time
(the worker's `W_SITE_N` count); `XO_RTSTATS=1` shows `site_timed`.
TestOneThreadSpawnJoin (internal/llvm) bounds the count. Measure
per-task bookkeeping on linux/amd64 too, not only on arm64.
