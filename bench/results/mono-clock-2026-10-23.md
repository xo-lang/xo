# macOS monotonic clock: CLOCK_UPTIME_RAW (2026-10-23)

`sysio.mono_ns` on macOS read `clock_gettime_nsec_np(CLOCK_MONOTONIC)`,
which goes `_mach_boottime_usec` -> `gettimeofday` -> `mach_absolute_time`.
macOS `sample` of bench/perf/conc `rwmutex` (400,000 accesses a task, 2 s)
showed the running threads mostly in that chain: `mach_absolute_time`
2,828 samples against about 600 in all of the runtime's own functions.
Callers: `pool.find_work` (the grace spin, pool.xo), `pool.run_fiber`
(W_RUN_AT), `pool.push` (W_NEXT_AT), `grace_begin`, `mutex.wake`.

Change: `darwin.CLOCK_UPTIME_RAW` (8, as tracerec.xo already used) in
`mono_ns` and `clock_read`. Like Go's nanotime it stops while the Mac
sleeps.

`xo bench --ab --rounds=10`, base (A) against the change (B), bench/perf/conc,
load 3.7 to 3.9, 0 loaded rounds (agents/ckab-cmd.log):

| row | wall | instructions | cpu_ms |
|---|---|---|---|
| rwmutex | 0.971 (0.96 to 1.011) within noise | 0.805 (0.788 to 0.821) better | 1.0 within noise |
| pingpong | 0.916 (0.885 to 0.987) better | 0.884 (0.884 to 0.885) better | 1.0 too coarse |
| mutex | 0.968 (0.899 to 1.242) within noise | 1.002 within noise | 1.0 too coarse |

The clock was not the main cost of rwmutex: 247,713 of 256,000 accesses
still park (a queued writer blocks every arriving reader; Go's twin blocks
about 2,700 times, bench in agents/gop-cmd.log), so wall stays 3.9x Go's.
