# std/llrt/testdata/tasks.c: "too many jobs" in the optimized run only

Status: Observed (2026-10-10, adding the decision 0118 tests).

Symptom: TestRuntimeTasks passes its ThreadSanitizer runs, then the
optimized run exits 2 printing `too many jobs`.

Cause: every `spawn` in tasks.c records a job in a fixed table of
`1 << 16` entries, and the optimized run is built with `-DN=20000`, ten
times the default `N`. A test that spawns `16 * N` tasks fits at N=2000
and overflows at N=20000 (fanout alone takes 2N entries).

Fix: a test that spawns many identical tasks makes one job with `mkjob`
and spawns it with `xo_task_spawn` directly (test_prune does), or keeps
its task count independent of `N`.
