# Fiber stacks kept to the peak (2026-10-19, Xo lang 4)

Xo Lang 3 found the native scheduler's fiber stacks churning under a
burst of connections: each worker kept 8 stacks and the pool 64 more
(std/native/llrt/pool.xo FCACHE, FIBER_CACHE), so a burst of 2,000
connections mapped and then unmapped about 1,840 stacks, with server
threads waiting on munmap's lock (D state). The pool now keeps up to
4,096 stacks (FIBER_CACHE) and, once it has been idle 3 s
(FIBER_TRIM_US: a worker going idle with more than FIBER_KEEP = 64
cached sleeps that long first, and when nothing woke it and no fiber
was taken or returned meanwhile, fiber_trim unmaps the rest), goes back
to 64, as Go's scavenger returns memory after a while.

linux/arm64 in Docker on the Mac, bench/perf/http/xo, `loadgen -burst
2000 -reps 150` after 2 warm up bursts (Xo Lang 3's harness; minor
faults and CPU ticks from /proc/PID/stat, RSS from /proc/PID/status),
stock against the change, alternating, two each:

| Server | minor faults (150 bursts) | CPU ticks | VmHWM | VmRSS at end | VmRSS after 6 s idle |
|---|---|---|---|---|---|
| stock | 117,601 | 2,230 | 55.4 MB | 49.2 MB | 49.2 MB |
| kept to peak | 3,496 | 1,413 | 57.4 MB | 57.4 MB | 49.9 MB |
| stock | 120,662 | 1,594 | 54.4 MB | 48.8 MB | 48.8 MB |
| kept to peak | 2,750 | 1,892 | 56.5 MB | 56.5 MB | 49.0 MB |

Faults 97% fewer; the peak RSS 2 MB higher (the stacks kept), back to
the stock size after the idle trim. CPU ticks on the loaded Mac vary
more than the change (Xo Lang 3 measured 5% to 7% less on a quieter
run); the bursts' slow rounds are Docker on a loaded Mac and hit Go's
server too (gotcha docker-burst-stalls-on-loaded-mac).
