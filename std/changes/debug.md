# std/debug changes

## 2026-11-06: feature: /debug/sched reports heap_chunks and heap_cache_bytes

- Kind: feature
- Commit: this push
- Found by: decision 0520 (a load run of stability/notes, 2026-11-06: Xo's RSS rose about 90 KiB per extra busy connection, and the soak needs to see whether that is the size classes' chunks or the large block cache while the program runs)
- Go reference: runtime/metrics /memory/classes/heap/* (Go reports its heap by use; these are Xo's two footprints)
- Test: cmd/xo TestDebugSchedHeapFootprint (24 MiB of small blocks kept live carve at least 4 more chunks; a freed 2 MiB block is in the cache)
- Timing: none (two atomic loads per GET)
- Shipped in: not yet released

`GET /sched` of `debug.router` on the native backends adds `heap_chunks N`,
the 4 MiB chunks the heap's size classes were carved from so far (never
returned), and `heap_cache_bytes N`, the bytes of freed large blocks kept
mapped for reuse. Neither walks another thread's free lists.

## 2026-11-05: feature: /debug/sched reports timers_pending

- Kind: feature
- Commit: this push
- Found by: decision 0520 (the stability program's soak needs the live timer count while the program runs)
- Go reference: none exported (Go's runtime keeps per-P timer heaps; runtime/metrics has no timer count)
- Test: cmd/xo TestDebugSchedTimersPending (10 tasks asleep: timers_pending rises by at least 10, falls back once they are canceled)
- Timing: none (one load of the timer heap's length per GET; no new counting)
- Shipped in: not yet released

`GET /sched` of `debug.router` on the native backends adds a line
`timers_pending N`: the timers in the event loop's heap now (sleeping
tasks, descriptor deadlines, select and scope timeouts), read without
the event loop's lock.

## 2026-11-05: feature: /debug/sched reports fds_open

- Kind: feature
- Commit: this push
- Found by: decision 0520 (the stability program's soak needs the open descriptor count from inside the program, on macOS as well as Linux)
- Go reference: none in Go's runtime (Go programs read /proc/self/fd themselves)
- Test: cmd/xo TestDebugSchedFdsOpen (20 more files held open: fds_open rises by at least 20)
- Timing: none (read on request: one directory listing per GET)
- Shipped in: not yet released

`GET /sched` of `debug.router` on the native backends adds a line
`fds_open N`: the open file descriptors of the process, read from
/proc/self/fd on Linux and /dev/fd on macOS (the descriptor that lists
the directory is not counted). The wasm backend and Windows omit it.

## 2026-11-05: feature: /debug/sched reports heap_live

- Kind: feature
- Commit: this push
- Found by: decision 0520 (the stability program's soak needs the live allocation count while the program runs; XO_LEAKCHECK reports it only at exit)
- Go reference: runtime.MemStats Mallocs - Frees (runtime.ReadMemStats)
- Test: cmd/xo TestDebugSchedHeapLive (100,000 blocks held: heap_live rises by at least that and falls back after)
- Timing: none (read on request; the counting it reads is the existing XO_LEAKCHECK/XO_RTSTATS switch)
- Shipped in: not yet released

`GET /sched` of `debug.router` on the native backends adds a line
`heap_live N`: the heap blocks allocated and not yet freed, summed over
every thread's heap, when the heap counts its allocations from the start
(`XO_LEAKCHECK`, `XO_HEAPSTATS` or `XO_RTSTATS` set). A soak samples it
to tell a leak from a resident set that only grows by fragmentation.

## 2026-11-04: fix: mem_trim gives back freed blocks on Linux and of every thread

- Kind: fix
- Commit: this push
- Found by: decision 0505's probe against Go's debug.FreeOSMemory on linux/arm64 (160 MB freed in blocks of 16 KiB to 16 MiB)
- Go reference: runtime/debug.FreeOSMemory (a GC, then the scavenger returns every free page, madvise MADV_DONTNEED on Linux)
- Test: cmd/xo TestMemTrimOtherThreads (linux: 162 MB resident before the trim, 8 after; 148 to 148 before the fix); golden mem_trim_mid (every backend)
- Timing: --ab self and bench/perf/alloc in decision 0505's "As built"
- Shipped in: not yet released

`debug.mem_trim()` on Linux now advises the page aligned free size class
blocks of 16 KiB and over (MADV_DONTNEED) and unmaps the cached large
blocks of 1 MiB and over, where before it did nothing on Linux. On every
OS it now reaches each worker thread's heap record (a moved trim epoch:
idle workers are woken and waited for up to about 20 ms; a busy one
trims at its next switch) and the central lists' magazines, and before a
program's first spawn it trims the heap the allocator uses rather than a
new, empty one. The probe's resident set after the trim, linux/arm64:
16 KiB blocks 324 to 4 MB, 64 KiB 332 to 13, 1 MiB 17 to 1 (Go 6 to 8).

## 2026-11-02: fix: mem_trim does nothing on wasm

- Kind: fix
- Commit: (this commit)
- Found by: Xo Lang 2 (the alloc_stress golden trapped on wasm)
- Go reference: none (runtime/debug.FreeOSMemory has no wasm specific path)
- Test: internal/llvm/testdata/golden/alloc_stress (wasm, through TestSelfhostWasm)

`debug.mem_trim` called from a task while other tasks ran made a wasm
module trap (`null function or function signature mismatch` in the
scheduler) or hang: it walked the native heap's thread record, which the
wasm runtime's own allocator never builds, and its stores landed in the
scheduler's memory. It now returns at once on wasm, as on the Go backend.

## 2026-11-02: perf: mem_trim also unmaps the cached large blocks of 2 MiB and over

- Kind: perf
- Commit: (this commit)
- Found by: Xo Lang 2 (ADR 0448 case (c), accepted by the owner 2026-11-02)
- Go reference: runtime/debug.FreeOSMemory (returns freed memory on request)
- Test: internal/llvm/testdata/golden/mem_trim_large (blocks after a trim, and while another task allocates)

`debug.mem_trim` now also unmaps the cached large blocks of 2 MiB and
over, on every target. A kept mapping stayed in the RSS (on darwin as
reusable pages, on Linux with no advice at all). The next such block maps
anew, so a program that trims in a loop pays for the mappings; xo trims at
three phase ends. Smaller cached blocks stay, for the next reuse.

## 2026-10-31: fix: a heap snapshot's own work is not sampled as the program's

- Kind: fix
- Commit: (this commit)
- Found by: Xo Lang 2 (ADR 0448 stage 4: an XO_PROF_HEAP_AT snapshot's DWARF read was about 50 MB of a warm build's exit profile)
- Go reference: runtime/pprof (Go's profile writer does not sample its own symbolization as the program's allocation)
- Test: internal/llvm TestProfHeap (snapshots still written and read); the self build's snapshot no longer lists dwline

A heap snapshot (XO_PROF_HEAP_AT) symbolizes its samples, which reads the
executable's DWARF, and those allocations were sampled into the profile
as if the program had made them. The sampler is paused while a snapshot
is written: each thread still draws its next sample point, so the
sampling stays unbiased afterwards, but nothing is recorded.

## 2026-10-31: fix: the heap profile counts a grown big block at its size

- Kind: fix
- Commit: (this commit)
- Found by: Xo Lang 2 (ADR 0448 stage 3: the self build's in use read 2.2 GB while the heap held about 7 GB mapped and RSS agreed with the heap)
- Go reference: runtime/pprof (Go samples each allocation; a slice's growth is a new allocation there, so it is sampled as one)
- Test: internal/llvm TestProfHeapGrown

XO_PROF=heap kept a big block's first size when it was resized, moved
or in place, and never counted the bytes it grew by: a list or text
grown to megabytes showed its first 16 KiB, or nothing when that first
size was not sampled. A resized block that was sampled now has its new
size and address; the growth of one that was not counts as allocated
bytes, so it is sampled as an allocation of that size is.
## 2026-10-30: feature: mem_trim gives the thread's free heap pages back

- Kind: feature
- Commit: (this commit)
- Decision: 0448 stage 1
- Go reference: runtime/debug.FreeOSMemory (Go's runs a collection and returns every free span; Xo's returns the calling thread's free blocks of 16 KiB and over, each once, with no collection to run)
- Test: cmd/xo TestHeapStatsFreeLists (the report's shape), the coll measurement in ADR 0448 (Stage 1 as built)

`debug.mem_trim()` gives the system the pages of the calling thread's free
heap blocks of 16 KiB and over (its free lists and spare magazines) on
darwin; the pages come back on the blocks' next use. On Linux it does
nothing yet (the page faults on reuse cost a warm build 3 to 4.7% of its
wall; ADR 0448 Stage 1 as built). xo calls it after its
front_end_freed, emit and link phases. With it the heap's large block cache gives back a cached
mapping's pages while it waits (`XO_HEAP_CACHE_ADVISE=0` keeps them
resident) and the 16 KiB class's full magazines are scavenged like the
larger classes'. Nothing on the Go backend.

## 2026-10-30: fix: a CPU profile that filled its buffer says so on its exit line

- Kind: fix
- Commit: (this commit)
- Found by: Xo Lang 1 (since f79ea3f9d, about 3 s of CPU at 10 kHz fills the profile's 32,768 samples)
- Go reference: none (Go's profile buffer drops and counts as "lost" too, in the profile only)
- Test: internal/llvm TestProfDroppedLine

`xo profile: 32768 samples (N more dropped: the profile holds 32768;
lower XO_PROF_HZ for a run this long) in FILE`: a run whose samples
overflow the profile used to print only the count it kept, so a profile
of the first seconds of a run read as the whole run.

## 2026-10-30: fix: XO_PROF's sample count matches CPU time on macOS and Linux

- Kind: fix
- Commit: (this commit)
- Found by: Xo Lang 2 (a metricsum profile with 26% of its samples; gotchas/xo-prof-macos-undercounts-samples.md)
- Go reference: runtime/pprof (Go 1.18's per-thread timers on Linux; Go on macOS keeps setitimer and undercounts too)
- Test: internal/llvm TestProfCountsCPU (4 spinning tasks at 1 kHz: samples within 0.9 to 1.1 of CPU seconds times the rate)

macOS drops most ITIMER_PROF ticks (a C control got 0.16 of its samples
at 1 kHz), so a CPU profile's counts were not time. On macOS a sampler
thread now reads every thread's CPU time each period and samples a
thread once per whole period it used, reading its registers with
thread_suspend and thread_get_state: 4 tasks at 1 kHz 0.970 of the
expected samples (was 0.14), one task 0.987 (was 0.29). On Linux each
SIGPROF sample counts the thread's CPU time since its previous sample
(CLOCK_THREAD_CPUTIME_ID), so a tick the process-wide timer merged is
not lost: on the linux/arm64 offload host, 4 tasks at 1 kHz 0.999 of
the expected samples (was 0.683), one task 0.998 (was 0.997). At
10 kHz a run of a few seconds fills the profile's 32,768 sample buffer
(one task 0.913 of 3.6 s of CPU before it filled).

## 2026-10-30: fix: mem_total reads physical memory on Windows

- Kind: fix
- Commit: (this commit)
- Found by: xo-lang-4's per OS host probe measurements (xo sized its emit jobs from a 16 GB guess on a 32 CPU Windows box)
- Go reference: none (Go's runtime does not expose physical memory)
- Test: a windows/amd64 box run printing `debug.mem_total()`, and the Windows sample

`debug.mem_total()` was 0 on native windows/amd64; it now returns the machine's physical memory from GlobalMemoryStatusEx's ullTotalPhys. So xo on Windows gets decision 0349's default memory budget (three quarters of physical memory, at least 16 GiB) when XO_MAX_MEM is unset, as on macOS and Linux, and sizes its LLVM and emit jobs from the real memory.

## 2026-10-27: feature: a crashed process can be held for a debugger (XO_CRASH_SLEEP)

- Kind: feature
- Commit: 5afb1e04a
- Found by: crash investigation (a SIGSEGV or SIGBUS process was gone before vmmap or lldb could read it)
- Go reference: none (Go's GOTRACEBACK=crash dumps core; macOS has no core dump by default)
- Test: TestCrashReport

`XO_CRASH_SLEEP=N` holds a process that took SIGSEGV or SIGBUS for N seconds after the crash report's first phase, printing its pid, so `vmmap -v PID` and `lldb -p PID` can read the live process. Off by default.

## 2026-10-23: feature: the heap in use at a labeled point (XO_PROF_HEAP_AT)

- Kind: feature
- Commit: d449dc24c
- Found by: decision 0352 step D (what a build keeps alive between phases)
- Test: a build of the scale suite's fns-4096 program with XO_PROF=heap XO_PROF_HEAP_AT=inline (compiler/xo labels its phases), converted by xo prof

With the heap sampler on, `XO_PROF_HEAP_AT=TEXT` writes the samples in use
when the first `mem_label` text ending with TEXT is set, to
`XO_PROF_HEAP_OUT`: the memory a phase leaves alive, which the profile
written at exit (everything freed by then) does not show. xo's own build
labels its phases, so `XO_PROF=heap XO_PROF_HEAP_AT=inline xo build ...`
shows what is alive once inlining is done (on fns-4096: 741 MB, two
thirds of it the parser's syntax trees).

## 2026-10-21: perf: a read profile gives its ring back

- Kind: perf
- Commit: 99c1c8a64
- Found by: decision 0325 follow-up
- Test: internal/llvm/testdata/golden/debug_routes (two profiles in a row; arm64, LLVM)

After `GET /profile` reads a profile, the native profiler returns the
ring's used pages to the system (madvise DONTNEED on Linux, FREE on
Darwin) instead of holding up to 17 MB resident until the next run.

## 2026-10-21: feature: std/debug, live profiles and scheduler counters

- Kind: feature
- Commit: 0c17dba55
- Found by: decision 0325 (the owner's language poll)
- Go reference: net/http/pprof (`/debug/pprof/profile?seconds=`, `/heap`)
- Test: internal/llvm/testdata/golden/debug_routes (auth refused, counters, heap, a one second profile, a second request during it refused, a profile again; arm64, LLVM, Go parity)

A router a server mounts to profile itself while it runs: `GET
/profile?seconds=N&hz=H` (CPU, native: the XORAW container `xo prof`
converts; Go: pprof), `GET /heap`, `GET /sched`, each behind an `auth`
function, one profile at a time. The native profiler can now be started
and stopped at run time (llrt/prof.xo profile_begin, profile_stop,
profile_collect: the ring stays mapped, signal handlers in flight are
counted so a stopped profile has no late sample).
