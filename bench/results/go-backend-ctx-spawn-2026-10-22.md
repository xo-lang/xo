# Go backend: ctx spawn and await (2026-10-22)

Item 8 of bench/results/go-backend-gap-2026-10-22.md: bench/perf/ctx's
spawn and await ran 5x the native backend (instructions retired; wall 3.7
to 4.7x).

## Where the time goes

`XO_CPUPROFILE` of spawn with 60,000 rounds (600,000 tasks): 85% of the
samples are the Go scheduler, not Xo code: `findRunnable` and `stealWork`
spinning on idle Ps (62%), `wakep`, `startm` and `pthread_cond_signal`
waking a thread for each `go` statement (17%), parking (48%). Xo's own
bookkeeping (`spawnWith`: the child task, its node, the events, the
mutex) is 11%, of which `runtime.Callers`, which records the spawn site
of each task and each scope, is 5%.

The scheduler share is the Go runtime's cost of starting a goroutine
while Ps are idle: with GOMAXPROCS=1 the same program ran at 3.8x native
instructions and 1.55x native wall (spawn, `--ab`, 5 rounds; 12, 4 and 2
Ps: 5.6x, 5.5x, 4.8x instructions). It is not removable from std/rt
without running tasks on the spawner's goroutine, which changes what a
task is; nothing was changed for it.

## Change

The spawn site of a task and of a scope body keeps 2 frames
(`runtime.Callers(2, site[:2])`) instead of 4: the sites are read by the
deadlock report and `xo dump` for their innermost Xo frame.

`xo bench --ab` (the Go backend, same xo, before against after, a gate
slot on the development Mac; spawn and await, 11 rounds):

| Program | Workload | instructions | wall | cpu |
|---|---|---|---|---|
| bench/perf/ctx (2,000 rounds) | spawn | 0.875 | 0.931 (better) | too coarse |
| | await | 0.883 | 1.021 (within noise) | too coarse |
| 60,000 rounds | spawn | 0.852 | 0.897 (within noise) | 0.803 (better) |
| | await | 0.821 | 0.835 (within noise) | 0.914 (within noise) |

Wall time is noisy here (spread 10 to 48%: thread wakeups). The Go
backend stays near 4x native on these two rows; the remaining gap is the
Go scheduler's wakeups, which the native runtime's fibers do not pay.
