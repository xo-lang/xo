# 0349: A memory budget for xo (XO_MAX_MEM)

Status: Accepted (owner, 2026-10-22).

## Context

On 2026-10-22 a quadratic bug in the debug information writer took one
`xo build` to 37 GB. The Mac has 24 GB: it swapped, every other session's
gates stalled, and nothing named the function being compiled. xo is a
native Xo program, so it grows until the operating system stops it, and
the operating system stops it late (macOS swaps first; Linux's OOM killer
may pick another process). Xo Lang 1 is adding a size guard to the debug
information writer itself; this decision is the general backstop: any
phase of xo that runs away stops early, with a message that says where.
heavy.sh's HEAVY_MAX_GB (12 by default) stops a job's process group from
outside, but only for jobs run under it, and it can only say that the
process grew.

## Decision

1. **What is measured.** The bytes the native heap has mapped from the
   operating system and not given back: std/native/llrt heapos.xo `map`
   and `unmap` (alloc.xo's 4 MiB chunks, large blocks and their reuse
   cache, and the C runtime records of rt.heap_large, which map through
   it). One atomic add per mapping, which is 4 MiB or larger for the Xo
   heap, so an allocation pays nothing; the check is a compare in `map`.
   Fiber stacks (pool.xo, at most one mapping per live fiber) and
   memory a C library maps itself are not counted. Mapped bytes bound the
   resident set from above for this heap, which keeps freed blocks on its
   lists rather than giving every page back, so the budget is a little
   stricter than RSS, never looser.
2. **The budget.** `XO_MAX_MEM` sets it for any native program: bytes,
   or a number with `K`, `M`, `G` (powers of 1024); `0` or `off` turns
   it off. Unset, a program has none, except xo itself: xo sets its own
   default at start, half the machine's physical memory (12 GB on a
   24 GB Mac, the same as heavy.sh's HEAVY_MAX_GB default), when
   XO_MAX_MEM is not set. Builds measured on 2026-10-22 peak far below
   that: a self build of xo about 2 to 3 GB per process, an LLVM build
   of xo several GB.
3. **The stop.** When a mapping would take the total past the budget,
   the runtime writes to standard error, without allocating:
   ```
   fatal error: memory budget exceeded: 12290 MB mapped, more than XO_MAX_MEM (12288 MB)
     while: building, past phase insert_rc
   ```
   and exits with status 70, as an out of memory does (heapos.xo oom).
   The second line is the program's memory label (below), or absent.
4. **The label.** `debug.mem_label(text)` (std/debug, no capability:
   it only labels a report) records a short text (copied, at most 255
   bytes) that the stop prints after `while:`; the empty text clears it.
   xo sets it at start (`checking (the first phase)`) and at the end of
   each phase its `--phases` report times (`building, past phase NAME`:
   check, ssa, the passes, emit, assemble, link and the rest), so the
   report names the phase in progress. Per function labels are left out:
   the code generation phases run functions on several threads at once,
   where one label would name whichever function set it last; a
   function that grows its own output past reason is the per phase
   guards' job (Xo Lang 1's debug information size guard).
   `debug.mem_budget(bytes)` sets the budget from the program (xo's
   default), 0 turning it off; `debug.mem_mapped()` and
   `debug.mem_total()` read the count and the physical memory.
5. **Platforms.** The physical memory xo's default divides: macOS
   sysctl `hw.memsize`, Linux `sysinfo(2)` (totalram times mem_unit).
   Windows (`GlobalMemoryStatusEx`) is not read yet, so xo sets no
   default there; `XO_MAX_MEM` works on every native target. The Go backend's programs are Go programs: `XO_MAX_MEM` is
   not read there (Go's own GOMEMLIMIT is a soft limit, not a stop).

## Consequences

- A runaway xo stops at half of memory with the phase and the function
  in the message, instead of swapping the machine; the report is what a
  bug report needs.
- A build that genuinely needs more (a very large program on a small
  machine) fails where it used to swap; `XO_MAX_MEM=off` (or a larger
  value) lets it run.
- Any native program can use `XO_MAX_MEM` as a hard ceiling, as a
  container memory limit would, but with a message.
- heavy.sh's HEAVY_MAX_GB stays the outer guard for every job (Go test
  processes, clang, the linkers), which XO_MAX_MEM does not see.

## Test

cmd/xo TestMemBudget: `XO_MAX_MEM=16M xo build` of a program larger
than 16 MB of compiler memory fails with status 70 and the two lines,
the second naming a phase; a native program that allocates in a loop
under `XO_MAX_MEM=32M` stops the same way with its own label, and runs to
the end with `XO_MAX_MEM=off`.

## Alternatives

- **RSS sampled by a thread.** Rejected: a sampling thread costs a
  wakeup per interval and still reacts late; the heap knows each mapping
  as it happens.
- **Counting every allocation.** Rejected: an atomic add on the
  allocation fast path, for no better answer than per mapping.
- **setrlimit(RLIMIT_AS / RLIMIT_DATA).** Rejected: macOS does not
  enforce them for mmap, address space is not memory (fiber stacks and
  guard pages reserve far more than they use), and the failure is a
  failed mapping with no message.
- **Only heavy.sh's limit.** Not enough: it covers only jobs run under
  it, and kills without saying which function grew.

## Addendum (2026-10-22, evening): what is counted, and no default yet

The first measure, every mapping's size, overstated the heap: a growing
large block maps twice its size in reserve, and the default of half of
physical memory (12 GB here) stopped every self build of xo
(gotchas/mapped-bytes-overstate-memory.md). 75296384b removed the
default. The budget now counts chunks and records as mapped and large
blocks by the bytes handed out (heapos.xo large_room, large_resize,
release); `debug.mem_peak()` gives the high-water mark and `xo build
--phase-times` prints the heap and its peak after each phase. A cold self
build of xo peaks at 9,909 MB counted against 9,141 MiB of maximum
resident set (bench/results/memory-budget-measure-2026-10-22.md). xo sets
no default until the LLVM build of xo (the full tier) and a gate are
measured the same way and the owner approves the number; `XO_MAX_MEM`
works as described above.

