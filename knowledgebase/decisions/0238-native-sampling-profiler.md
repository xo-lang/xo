# 0238: A sampling profiler in the native runtime

Status: Accepted (owner, 2026-10-16). CPU profile implemented on darwin and linux, arm64 and LLVM, 2026-10-16 (xo-4); the heap profile too, 2026-10-16 (xo-4); both on windows/amd64 (LLVM), 2026-10-16 (xo-4), checked under Wine with Box64 only; windows/arm64 not yet. Since decision 0245 (2026-10-17) the program writes a raw file and `xo prof` writes the pprof files and summaries.

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

## Implementation (CPU profile, 2026-10-16)

std/native/llrt/prof.xo, unwind.xo prof_walk, cabi.xo on_prof and
prof_finish, entry.xo (start and the end of main), process.xo exit;
selfhost/xo `--profile=cpu` for run and test.

- The timer is setitimer(ITIMER_PROF), process wide on both Linux and
  macOS (macOS has no per thread CPU timer; Linux's timer_create per
  thread is a later refinement): the kernel sends SIGPROF to a thread
  that is using CPU.
- The handler (SA_SIGINFO, SA_RESTART) seeds the unwinder's register
  set from the signal's ucontext (arm64 and x86_64, Linux and macOS
  layouts) and walks with lookup_frame and step in a record of the
  profiler's own: a pool of 64 records mapped at start, one claimed per
  thread on its first sample (thread local word 15), each prepared with
  the program's .eh_frame_hdr or image so the handler never looks them
  up. A step whose stack pointer does not move up within 64 MiB, or a pc
  outside the program's text, ends the walk (a pc in a C library
  function first continues once through the link register or the
  return address at the stack pointer). One lock free ring of 32,768
  slots of 64 frames (17 MB, mapped only when profiling) takes the
  samples, a slot claimed with one atomic add; the full ring's samples
  and those of threads past the pool are counted as lost.
- At the end the timer stops, SIGPROF is ignored, and the samples are
  symbolized from the binary's own symbol table (macOS LC_SYMTAB through
  __LINKEDIT, in memory; Linux .symtab, else .dynsym, of
  /proc/self/exe), Xo names demangled (`xo_f_`, the LLVM backend's
  `x.`, exports' `xo_x_`); a pc outside the program is named by dladdr
  (macOS) or its shared object (Linux). pprof: a location per distinct
  pc, its function the call's (pc - 1), every sample its own; gzip with
  stored deflate blocks (no compressor in the runtime). XO_PROF_OUT's
  `%p` is the process id (xo test --profile writes one per process).
- Off: Linux, 242 instructions a process (the XO_PROF lookup, and
  start and finish finding nothing armed; callgrind of hello). macOS:
  the runtime's binary is 53 KB (arm64) to 66 KB (LLVM) larger, and
  loading it costs hello 1.6% to 2.5% of its 14M instructions (21
  interleaved runs); a long run is within noise (0.05%).
  bench/results/prof-cpu-2026-10-16.md.
- Test: internal/llvm TestProfCPU (a recursive fib is the top function by
  flat samples on each native backend, go tool pprof reads the file,
  nothing is written without XO_PROF); linux/arm64 checked in Docker.

## Implementation (heap profile, 2026-10-16)

std/native/llrt/heapprof.xo (the sampler, static word 66), obj.xo
(halloc, hfree, hresize), heapos.xo (two words more per heap record,
the hook copied into a thread's new record), prof.xo (start_heap,
finish_heap, the writer), unwind.xo prof_trace, cabi.xo heap_prof;
selfhost/xo `--profile=heap` and `cpu,heap` for run and test.

- Through the counting hook item 4 named (since 2026-10-16, the second
  design): starting the profile turns counting on in every heap record
  (heapos.xo profile_on; a thread's new record copies it), and
  alloc.xo's counting branch (alloc, alloc_fast, resize's move) compares
  the record's ABYTES with its SAMPLE_AT (the largest Int while not
  profiling). alloc.xo makes no call but rt.heap_*, so a due sample keeps
  the block in the record's PSAMPLE and tail calls rt.heap_grow with
  SAMPLE added to the size (cabi.xo heap_grow hands it to
  heapprof.sampled); alloc_fast, which generated code inlines, stays
  without a call: it gives the block back to its class's list and
  returns 0, and the caller's slow path (alloc) takes it and samples it.
  A sampled block of a class gets minus its class in its header, so its
  free takes the big block's way to rt.heap_big_release (cabi.xo: a
  header from -1 to -72 is forgotten, its class put back, and freed the
  ordinary way); resize's big block test is `hdr < -128`, so a sampled
  block moves. Big blocks are sampled, forgotten, and followed in
  cabi.xo heap_big, heap_big_release, heap_big_resize. The heap keeps
  its paths: a single threaded program keeps its record and fast paths.
  (The first design, 02792d0c and before, switched the program to the
  threaded heap and hooked obj.xo's threaded paths: an off cost of one
  bit test per allocation and free, and an on cost of up to 2x for a
  single threaded program.)
- A sample: an exponential gap of mean XO_PROF_HEAP_RATE (Go's
  MemProfileRate, 512 KiB; 1 samples every allocation; xorshift and a
  5 term atanh logarithm, no allocation), the frames by the fault
  unwinder's tables in an unwind record of the profiler's own per
  thread (unwind.prof_trace: a raise may hold the thread's own
  scratch), the address and size, in a table of 65,536 slots mapped at
  start, and the address in an open addressed set (four entries per
  slot, tombstones reused) so the free marks the slot freed. Nothing in
  the hook allocates or locks.
- The writer leaves the allocator's frames off each stack (obj, alloc,
  heapos, cabi, the sampler, and the bare C ABI exports `heap_prof`
  and `obj_*`), scales each sample by 1 / (1 - e^(-size/rate)) as Go
  does, and writes alloc_objects, alloc_space, inuse_objects,
  inuse_space (period: the rate in bytes) to XO_PROF_HEAP_OUT (else
  XO_PROF_OUT without cpu, else xo-heap.pprof), and a text summary of
  the top 30 by inuse_space and by alloc_space.
- inuse is what is still allocated when the profile is written: at
  main's end Xo has dropped main's values (0 in use but leaks); at
  os.proc.exit the values of other tasks still held. A value whose last
  use is before os.proc.exit is dropped before the call too.
- Cost, bench/perf/alloc on the Mac (instructions, minimum of 7 runs,
  bench/results/prof-heap-2026-10-16.md). Off: the counting branch is
  not taken, so the heap is main's before either design within noise
  (small 1.000, mixed 1.004, maps 1.000, strings 1.006, tasks 1.008;
  conc within noise). On at the default rate: small 1.33x, strings and
  maps 1.27x, mixed 1.22x, tasks 1.13x (the first design: 2.05x, 1.58x,
  1.46x, 1.14x), grow 2.08x (a 65M instruction run: the profile's fixed
  start and writing, about 30M, dominate).
- Test: internal/llvm TestProfHeap (rate 1: the 2000 strings a task
  holds at os.proc.exit are in use and the top function by inuse_space
  is str_repeat, the churned lists are churn's; the default rate
  estimates the 40 MB allocated within 2x; cpu,heap writes both; go
  tool pprof reads it), arm64 and LLVM.

## Windows (implemented 2026-10-16, windows/amd64)

Designed as Go's profileloop (parked 2026-10-16, built the same day once
the owner made room for the cross toolchain on the Mac):

- A sampler thread (CreateThread, prof.xo win_sampler) ticks every
  1/XO_PROF_HZ s on a waitable timer (CreateWaitableTimerExW with
  CREATE_WAITABLE_TIMER_HIGH_RESOLUTION, a relative due time each tick;
  an ordinary timer where that flag is refused). Each tick it lists the
  process's threads (CreateToolhelp32Snapshot TH32CS_SNAPTHREAD,
  Thread32First/Next), opens each once (SUSPEND_RESUME | GET_CONTEXT |
  QUERY_INFORMATION, a table of 256 with the last CPU reading), and
  samples a thread whose CPU moved since the last tick
  (QueryThreadCycleTime; where that fails, as under Wine, GetThreadTimes'
  kernel plus user time, which moves in clock ticks): SuspendThread,
  GetThreadContext (CONTEXT_CONTROL | CONTEXT_INTEGER), the walk into the
  ring, ResumeThread. A thread whose GetThreadTimes shows it exited is
  reopened (its id reused).
- The walk (unwind.xo win_walk) reads only the image and the stack:
  the function's RUNTIME_FUNCTION by a binary search of the image's
  .pdata (not RtlLookupFunctionEntry, which takes the loader's lock that
  a suspended thread may hold), RtlVirtualUnwind for the frame; a leaf
  without an entry, or a first frame outside the program, returns to the
  word at Rsp. Each return address must follow a call in the program's
  text and the stack pointer must rise, or the walk ends. Nothing
  allocates while a thread is suspended (no list literal either).
- The heap profile walks its own thread the same way from
  RtlCaptureContext (unwind.xo prof_trace).
- Symbols: Windows links pass `-Wl,/debug:symtab` (selfhost/xo link.xo),
  so lld-link keeps a COFF symbol table in the file (not mapped, no
  PDB); prof.xo coff_symbols reads it from the program's file
  (GetModuleFileNameW): functions by type 0x20, or external and static
  symbols in a code section. No line numbers (no DWARF in the PE).
- The fault traceback (XO_TRACEBACK, decision 0239) collects its frames
  with the same walk (unwind.xo trace, which returned none on Windows)
  and names them from these symbols: functions only, no lines or
  inlined frames (user_fault prints `main`, where macOS prints the
  inlined withdraw and both lines).
- Files are created with CREATE_NEW (an existing file or link is not
  written); Windows has no mode bits, so the file inherits its
  directory's ACL.
- Constants and layouts (THREADENTRY32, CONTEXT, the PE and COFF headers,
  the access rights): std/llrt/testdata/win_consts.c, TestWinConsts.

Checked 2026-10-16 under Wine 10.0 with Box64 0.3.4 in Docker
(knowledgebase/infrastructure/toolchain.md "Windows programs under
Wine"): testdata/prof/fib names fib at 100% flat; testdata/prof/heap's
heap profile names raw.str_repeat under main.fn1 through the task's
fiber. Under Box64, 25 to 40% of the CPU stacks stop after fib: Box64
reports Rip at a call instruction while Rsp has already taken the
call's push, so the caller read is fib's saved n (gotcha
box64-thread-context-skewed.md). Native Windows is not checked (CI is
off). windows/arm64 prints that the profile is not supported (its
CONTEXT and RUNTIME_FUNCTION differ).

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

## Addendum: release builds and the files written (owner, 2026-10-16; implemented 2026-10-16, xo-4)

Relayed by the lead. Development builds (`xo run`, `xo test`, a plain
`xo build`) keep the profiler; `xo build --release` leaves it out unless
`--profile-support` is given. When the profiler writes a file it creates
a new one (O_EXCL), refuses a symbolic link, and uses mode 0600. The
same rules apply to the execution trace's recorder (decision 0240:
`--trace-support`). Decision 0245 (sampling in the binary, symbolizing
with `xo prof`) is a separate proposal, still waiting on the owner.

Implementation:

- `prof.supported()` (std/native/llrt/prof.xo) is `true`, and the SSA
  builders fold a call of it to the build's answer (selfhost/ssa expr.xo
  prof_call, in target_test as decision 0246's test world switches):
  false when the driver builds with `--release` and neither
  `--profile-support` nor `--profile` (selfhost/xo main.xo lower,
  ssa.build_program_prof). entry.xo's start and finish, cabi.xo's
  SIGPROF handler and prof_finish, and the heap hooks in cabi.xo
  (heap_grow's sample, heap_big, heap_big_release, heap_big_resize) test
  it, so neither prof.xo's profiler nor heapprof.xo is built or linked
  in such a release build (the fault traceback's symbolizer, prof.xo
  trace_text, stays: XO_TRACEBACK is not the profiler). With XO_PROF set
  such a binary prints one line: built without the profiler, add
  `--profile-support`.
- The profile and its summary open with O_WRONLY | O_CREAT | O_EXCL |
  O_NOFOLLOW | O_CLOEXEC, mode 0600 (O_NOFOLLOW is 0100000 on Linux
  arm64, 0400000 on x86_64): an existing file or symbolic link is not
  written, and the run says so. A second `xo run --profile=cpu` in the
  same directory needs the first xo-cpu.pprof moved away.
- Test: internal/llvm TestProfGating (no prof.start in a --release
  binary and its note; --profile-support samples; mode 0600; a second
  run and a symbolic link are refused).
