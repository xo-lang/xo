# 0638: public memory checks, XO_THREADS, and a runtime collector in std/metrics

Status: Proposed (Xo Lang 2, 2026-11-14; owner request through the lead)

## Context

Three things the runtime already has are internal: documented only in
knowledgebase/infrastructure/toolchain.md, absent from the spec, from
`xo help`, and from xo-at-a-glance.md and agent-tools.md. The owner asked
(2026-11-14) to expose them, Go's equivalents as the model:

1. **Memory checkers.** XO_FREE_CHECK (use after free, decision 0484),
   XO_HEAP_GUARD (heap overrun) and XO_LEAKCHECK (blocks live at exit).
   Go needs none of the three (a garbage collector; bounds checks always
   on); its nearest are `-race`, `-asan` and `-msan` on `go build`, and
   GODEBUG settings at run time.
2. **XO_THREADS**, Go's GOMAXPROCS, and whether its default should follow
   a container's CPU quota (cgroup v2 `cpu.max`), as Go does since 1.25.
3. **Runtime metrics.** The runtime keeps counters (XO_RTSTATS's 52,
   XO_HEAPSTATS, std/debug's `mem_*`) that a service cannot export; Go has
   runtime/metrics, and its Prometheus and OpenTelemetry clients export it.

What exists today (read on main 2527165b4):

| Setting | Read by | When | Backends | On a find |
|---|---|---|---|---|
| XO_FREE_CHECK=1 | the program (llrt entry.xo:257) | start; checked as a freed block leaves a 65,536 block ring per thread, and at exit | native (arm64, amd64); LLVM builds the same llrt (not yet run under it); ignored by a sanitizer or --race build, the Go backend and wasm | a load through a freed block: SIGSEGV at 0xdeadbeefdeadbeef with a trace; a write: `fault: XO_FREE_CHECK: a freed block was written after its free ...`, exit 70 |
| XO_HEAP_GUARD=1 | xo (compiler/xo main.xo:2918), a build cache key | build | native; wasm drops it; the Go backend ignores it | a large block's overrun faults on its guard page; a List slot write checks its index |
| XO_LEAKCHECK=1 | the program (entry.xo:263, 511) | start (keeps allocation counts) and exit | native, LLVM, wasm (run.mjs); the Go backend ignores it | `xo leakcheck: N live of M allocations` on stderr; the exit status is unchanged (the golden harness fails a nonzero count) |
| XO_LOCKCHECK=1 | the program (poolsetup.xo:118) | pool setup | native, LLVM | `xo: lock order: ...` and similar, exit 70 |
| XO_RC_CHECK=1 | xo (main.xo:3008) | build | arm64, amd64, LLVM | `xo: internal: a var List parameter proved unique ...`, exit 70 |
| XO_THREADS=N | the program (poolsetup.xo:80-92) | pool setup | native, LLVM | none (1 to 256; default cpu_count()) |

cpu_count() (poolsetup.xo:254-302): Linux counts the sched_getaffinity
mask (else /sys/devices/system/cpu/online); darwin hw.activecpu; Windows
GetActiveProcessorCount over all groups, no affinity; FreeBSD and WASI 1.
No cgroup file is read (gotchas/worker-count-ignored-cpu-affinity.md).

`--race` is a build flag (TSan instrumentation and runtime, no variable);
`--profile=cpu|heap` is a flag that reaches the program as XO_PROF,
XO_PROF_OUT and XO_PROF_QUIET (main.xo:553, test.xo:992). There is no
`--check` on build, run or test (`--check` exists on `xo fmt` and `xo
publish` only).

### Measured: XO_THREADS in a CPU-limited container (2026-11-14)

A static linux/arm64 program built by 0.51.1's xo: 32 tasks, each 60
million multiply-adds; Docker on the Mac (a 12 CPU Linux VM, cgroup v2),
5 runs each; threads from /proc/self/status (the workers plus the
runtime's own). The program and script: ~/xo-lead/notes/xo-lang-2/thr.

| Container | Threads | Wall time (5 runs) |
|---|---|---|
| no limit | 16 | 248 to 341 ms |
| `--cpus=2` (cpu.max 200000 100000) | 16: the quota is not read | 989 to 1146 ms |
| `--cpus=2`, XO_THREADS=2 | 6 | 1096 to 1104 ms |
| `--cpuset-cpus=0,1` (affinity) | 6: affinity is read | 1121 to 1136 ms |

Go 1.27.4 in the same containers: GOMAXPROCS 12 with no limit, 2 under
`--cpus=2`, 3 under `--cpus=2.5` (the quota rounded up).

So Xo runs 12 workers on 2 CPUs' worth of quota. For pure throughput that
costs nothing here (about 5% faster than 2 workers: more threads use the
quota sooner in each period). Go's reason to follow the quota is latency:
threads that spend a period's quota early are throttled for the rest of
the period (up to 100 ms at the default period), and a request arriving
then waits. This run does not measure that; step 2 does before the
default changes.

## Decision (proposed)

### 1. `--check=free,guard,leak` on xo run, test and build

- One flag, a comma list, on `xo run`, `xo test` and `xo build`: `free`
  (XO_FREE_CHECK), `guard` (XO_HEAP_GUARD), `leak` (XO_LEAKCHECK);
  `--check=all` is the three. An unknown name is a usage error (exit 2).
- **The XO_* variables stay, documented, as the mechanism under the flag
  (owner, 2026-11-14).** A built binary run outside xo (deployed, in a
  container) needs them, as Go keeps GOMAXPROCS and GODEBUG beside
  runtime.GOMAXPROCS. The flag is the front door; the spec and `xo help`
  say so.
  - free and leak are read by the program at start: `xo run` and `xo
    test` set the variables for the child; `xo build --check=free,leak`
    embeds them as the binary's defaults (a variable set at run time
    still wins, so `XO_FREE_CHECK=0` turns an embedded check off).
  - guard is a build-time choice (the allocator's layout): `--check=guard`
    sets it for the build, and it stays part of the build cache key.
- Backends: native (arm64, amd64) for all three. LLVM: leak today; free
  and guard once the LLVM goldens pass under them (step 1 runs them). The
  Go backend: free, guard and leak are refused with a usage error naming
  the backend ("--check=free: native backends only; the Go backend has a
  garbage collector"), where today the variables are silently ignored.
  wasm: leak works, free and guard are refused. With `--race` or a
  sanitizer build, free is refused (no page blocks); guard and leak work.
- Exit status: a free or guard find is a fault (exit 70, the fault text
  and trace). Leak becomes a failure under the flag: `xo run --check=leak`
  and `xo test --check=leak` exit 1 when blocks are live at exit (the
  report line is unchanged). The variable alone keeps today's report-only
  behaviour, which the golden harnesses read.
- Cost (measured in step 1 with `xo bench --ab` on host 1, and stated in
  the spec): free fills each freed block and holds up to 65,536 per
  thread (0484 measured no number); guard maps a guard page per large
  block (TLS goldens slowed past 30 s under load,
  gotchas/heapguard-http-goldens-flake-under-load.md); leak's counting is
  about 6 instructions an allocation (alloc small 0.979, text split
  0.986; alloc.xo:73-83).
- **XO_LOCKCHECK and XO_RC_CHECK stay internal.** They check the
  scheduler's lock order and the compiler's uniqueness proof (decision
  0322): a find is a bug in the runtime or the compiler, not in the
  user's program, so a user flag would only produce reports the user
  cannot act on. They stay in toolchain.md for the toolchain's own tests.

### 2. XO_THREADS documented; the default follows the CPU quota after a measurement

- XO_THREADS goes into the spec (a runtime section beside XO_TRACEBACK
  and XO_PROF) as Go's GOMAXPROCS: the number of worker threads running
  Xo tasks, 1 to 256; the default is the CPUs the process may use.
- Proposed default (step 2, only with its measurement): on Linux the
  default also reads cgroup v2 `cpu.max` (the process's cgroup from
  /proc/self/cgroup, then its parents, as Go does) and takes
  min(affinity count, ceil(quota / period)), at least 1; cgroup v1
  `cpu.cfs_quota_us` and `cpu.cfs_period_us` as Go reads them. Read once
  at pool setup (Go 1.25 also rechecks it periodically; Xo's pool does not
  resize, so that is a later decision if ever). XO_THREADS set wins.
- No call to set the worker count at run time (Go's
  runtime.GOMAXPROCS(n)): Xo's pool is sized once.
- Windows: count the process affinity mask too (today every group), so a
  job object's CPU limit is honoured. FreeBSD: cpuset_getaffinity in place
  of the fixed 1.

### 3. Runtime metrics in std/metrics

- `metrics.runtime(r: Registry)` registers collectors (std/metrics
  `gauge_func`, `counter_func` and `collector`, read at scrape time) over
  what the runtime already keeps, named after Go's runtime/metrics and the
  Prometheus Go client where one maps:
  - always kept, so no cost until scraped: `xo_heap_mapped_bytes`
    (debug.mem_mapped), `xo_heap_peak_bytes` (mem_peak),
    `xo_memory_total_bytes` (mem_total), `xo_tasks_live` (rtstats
    fiber_live, always counted), `xo_threads_workers` (the pool's
    workers), `xo_heap_chunks`, `xo_heap_cache_bytes`, `xo_fds_open`,
    `xo_timers_pending` (std/debug's /sched values on native).
  - only when XO_RTSTATS is on (each count one relaxed atomic add):
    `xo_sched_spawns_total`, `xo_sched_steals_total`,
    `xo_sched_wakes_total`, `xo_sched_parks_total`, `xo_lock_slow_total`,
    and the net and poll counters; registered either way, with no sample
    when off, so a dashboard does not read a 0 as a fact.
  - heap counts (`xo_heap_allocs_total`, `xo_heap_frees_total`,
    `xo_heap_live_objects`) only when allocation counting is on
    (XO_LEAKCHECK, XO_HEAPSTATS, XO_RTSTATS or a memory budget); turning
    it on for metrics alone costs the 2 to 3% above, so it is opt in
    (`metrics.runtime(r, heap_counts: true)`).
  - the Go backend: the same names where Go's runtime/metrics has the
    value (`/memory/classes/total:bytes`, `/sched/goroutines:goroutines`,
    `/sched/gomaxprocs:threads`, `/gc/cycles/total:gc-cycles`), the rest
    with no sample.
- Cost of reading: each value is a load of a word the runtime keeps (the
  rtstats counters summed over threads on read); step 3 measures a scrape
  of every runtime metric (target under 10 us) and states it.
- std/otlp exports them unchanged: `encode_metrics` already maps
  collectors (a gauge to Gauge, a counter to a monotonic cumulative Sum).
- No runtime histograms (Go's `/sched/latencies:seconds`, GC pauses) in
  this decision: the runtime keeps no distributions today.

### Steps

Each ships alone, with the spec, CHANGELOG, std/changes where std changes,
and knowledgebase/infrastructure/agent-tools.md and xo-at-a-glance.md in
the same push, then a note to the lead to broadcast it (owner,
2026-11-14).

1. `--check=free,guard,leak`: flag parsing (compiler/xo opts.xo), the
   variables for run and test, embedding for build, the refusals per
   backend, leak's exit 1; goldens that plant each bug and see it found
   on arm64, amd64 and LLVM, and the refusal texts; `--ab` cost rows.
2. XO_THREADS in the spec; the cgroup reading behind a measurement (an
   HTTP server's p50 and p99 latency under `--cpus=2` on host 2, the
   default's 12 workers against the quota's 2), so the default changes
   only with a number; Windows affinity and the FreeBSD cpuset.
3. `metrics.runtime`: the collectors and names, the Go backend mapping, a
   scrape cost row, an otlp golden.

## Consequences

- Users get the memory checks that only the toolchain's own tests used,
  through a flag in `xo help` and spec text, and a deployed binary keeps
  the variables.
- Refusing the checks on the Go backend and wasm turns today's silent
  no-op into an error for the flag only; a script setting XO_FREE_CHECK on
  every backend works as before.
- After step 2, a container's quota bounds the workers, as in Go; a
  program that wants more sets XO_THREADS.
- Metric names become an API: renaming one later is a breaking change.

## Alternatives

- **Separate flags** (`--free-check`, `--heap-guard`, `--leak-check`), as
  Go's `-race`, `-asan` and `-msan` are separate: one list scales to later
  checks without new flags. Rejected for the list.
- **Only document the variables, no flag**: the least work, but users and
  agents find flags in `xo help`, not variables in a spec section.
- **One GODEBUG-style variable** (`XO_DEBUG=freecheck=1,leakcheck=1`):
  Go's shape for run-time settings; Xo already has one variable per
  setting and many scripts use them. Not now.
- **Follow the quota without measuring**: Go's choice, but the throughput
  run above shows no loss from ignoring it, so the change waits for the
  latency number.
- **Runtime metrics in a separate std/runtime module** (Go has
  runtime/metrics apart from its clients): Xo's std/metrics is the one
  registry std/otlp and the Prometheus handler read, so the collectors
  live there.
