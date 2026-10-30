# std/debug changes

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
