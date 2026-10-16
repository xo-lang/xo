# 0238: A sampling profiler in the native runtime

Status: Accepted (owner, 2026-10-16). Not implemented yet.

## Context

Profiling a native Xo program today takes tools from outside: callgrind
under valgrind in a linux/arm64 Docker container (instruction counts by
function), or macOS `sample` (stacks by address and symbol). Neither
shows `.xo` lines, the Mac has no deterministic counts, and each
diagnosis is a container round trip (the 2026-10-16 conc spawn and
pingpong diagnosis, bench/results/zero-c-perf-sweep-2026-10-15.md). The
runtime's own counters (XO_RTSTATS, XO_HEAPSTATS, XO_LEAKCHECK) report
totals, not where the time or the allocations go. Agents profile
often, so a profile they can read as text, from one command on any
host, shortens the loop the most.

The runtime already walks native stacks: fault unwinding has a frame
cache (d8219048), and both backends emit unwind tables.

## Decision

1. A CPU profile in the native runtime, chosen by `XO_PROF=cpu` (and
   `xo run --profile=cpu FILE`, `xo test --profile=cpu`): a per thread
   CPU timer (setitimer ITIMER_PROF on Linux and macOS, a sampling
   thread with SuspendThread and GetThreadContext on Windows) at
   100 Hz by default (`XO_PROF_HZ`). The handler records the
   interrupted pc and walks the frames with the fault unwinder's
   tables into a lock free per thread ring, with no allocation and no
   lock in the handler. Samples a full ring would lose are counted and
   reported.
2. At exit (or `rt.prof_flush()`), the runtime writes the profile in
   pprof's protobuf format (gzip), so `go tool pprof`, flame graph tools
   and speedscope read it unchanged, to `XO_PROF_OUT` (default
   `xo-cpu.pprof` in the working directory). It also writes a text
   summary (top 30 by flat and by cumulative, as `go tool pprof -top`
   prints) next to it, for agents that read text.
3. Symbols come from the binary's symbol table at write time. Source
   lines come with decision 0239's line tables; until then the profile
   names functions only.
4. A heap profile, `XO_PROF=heap`: allocation sites sampled every
   512 KiB allocated on average (`XO_PROF_HEAP_RATE`), as Go does,
   through the allocation counting hook that XO_HEAPSTATS already uses,
   written in the same pprof format (alloc_space, alloc_objects,
   inuse_space, inuse_objects). `XO_PROF=cpu,heap` takes both.
5. Off costs nothing: the timer is never armed and the allocation hook
   stays off (as with dd5adca2's allocation counting).
6. Every native backend (arm64, LLVM) and target (darwin, linux, windows)
   gets it. The Go backend keeps runtime/pprof.

## Consequences

- One command profiles on the Mac, no Docker and no valgrind; Docker
  callgrind stays for exact instruction counts.
- pprof output joins Go's tool chain and every viewer that reads pprof.
- The signal handler shares the fault unwinder, so an unwinder bug
  shows up in profiles too (tests: a profile of a known hot loop names
  that loop as the top function on every backend and target).
- A program that already uses SIGPROF is not supported while profiling.

## Alternatives

- Keep using callgrind and `sample`: no source lines, no Mac counts, a
  container per diagnosis.
- perf on Linux: Linux only, needs perf_event access in containers, and
  still no `.xo` lines without decision 0239.
- Instrumenting function entry and exit: exact counts, but it changes
  what is measured (inlining, small functions) and costs when on.
- A custom profile format: pprof is already read by everything.
