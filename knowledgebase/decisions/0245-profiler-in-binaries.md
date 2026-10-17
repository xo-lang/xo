# 0245: Where the native profiler lives in a binary

Status: Accepted (owner, 2026-10-17): option 3 with the fault traceback's symbolizer kept in binaries (lead, 2026-10-17); implemented 2026-10-17 (xo-4).

## Context

Decision 0238 put a sampling profiler in the native runtime, chosen by
`XO_PROF=cpu|heap` on any binary, and 0239 gave it DWARF line tables
with inlined frames. entry.xo calls `prof.start()` in every program, so
every native binary links the whole profiler: 158 KiB of hello's 593
KiB on darwin/arm64 (27 percent), the same 158 KiB in the network
programs (bench/results/binary-size-native-2026-10-16.md).

What the 158 KiB is (json_server, darwin/arm64, bytes of code by
function, 2026-10-16):

- the sampler, which has to be in the process: `prof.start`,
  `on_signal`, `start_heap`, and heapprof's hooks (sample, hook,
  next_gap, remember, forget): about 10 KiB;
- the output side, which runs once at exit: the pprof writer and its
  gzip (write_profile, pprof, pprof_tail, gzip), the text summaries
  (heap_text, trace_text, top_text, finish_heap, finish_cpu), reading
  the binary's own symbols (macho_symbols, elf_symbols, demangle), and
  all of dwline.xo (74 KiB: the .debug_line and .debug_info reader and
  the dSYM loader): about 148 KiB.

XO_PROF works on any binary today, including a release build on a
server, and agents rely on it (one command, text out).

## Options

1. **Status quo.** 158 KiB in every binary. Simplest; profiling needs
   no rebuild.
2. **Opt in at build time.** `xo build --profile` (and `xo run
   --profile=cpu`, `xo test --profile`, which already exist) link the
   profiler; other builds link a `prof.start` that, when XO_PROF is
   set, prints `xo: built without the profiler; rebuild with --profile`
   and runs normally. Saves the whole 158 KiB. Loses profiling a binary
   that is already deployed, and a profile needs a rebuild that may
   change what is measured (a different binary).
3. **Split: sample in the binary, symbolize in `xo`.** The binary keeps
   the sampler (about 10 KiB) and writes raw samples at exit: the
   program counters, stacks, heap records, and the binary's LC_UUID or
   build ID, in a small fixed format (`PROG.xoprof`). `xo prof
   PROG.xoprof [--binary PROG]` (the toolchain, which already reads
   Mach-O and ELF objects in selfhost/macho) reads the binary's symbols
   and DWARF and writes the pprof file and the text summary that
   XO_PROF writes today. Saves about 148 KiB per binary and keeps
   XO_PROF on every binary; a profile needs the binary (or its dSYM)
   and an `xo` to read it, as `go tool pprof` needs the binary for a
   stripped Go program. `xo run --profile` and `xo test --profile` run
   `xo prof` themselves, so the one command workflow agents use does
   not change.
4. **Option 3 plus 2**: no profiler at all in `--release` builds
   unless `--profile`. Saves the last 10 KiB; not worth a second mode.

## Recommendation

Option 3: it removes 94 percent of the profiler's bytes and keeps
decision 0238's property that any binary can be profiled. The work: a
raw sample writer in prof.xo (replacing finish_cpu, finish_heap and the
writers), `xo prof` in selfhost/xo built on selfhost/macho's readers
and a port of dwline.xo's parser (it is Xo already, so it moves rather
than being rewritten), and the run and test drivers calling it. Tests:
TestProfCPU, TestProfHeap, and the inlined frame tests run through
`xo run --profile` and keep their expectations.

With `-s` (darwin and Linux) the binary's local symbols are gone; `xo
prof` then names only the exported functions unless it is given the
unstripped binary or the dSYM, which is the same as today's in process
symbolization of a stripped binary.

## Consequences (option 3)

- hello about 445 KiB instead of 593 (stripped, darwin/arm64), every
  native binary about 148 KiB smaller.
- Profiles still come from `XO_PROF` on any binary; reading one takes
  `xo prof` (or `xo run --profile`, which does it).
- The heap profile's text at exit (XO_PROF=heap printing a summary to
  standard error) becomes `xo prof`'s output; programs that relied on
  the in process text need the extra step.

## Addendum: the measured split, and the traceback (2026-10-17)

The fault traceback (XO_TRACEBACK, decision 0239; `xo test` sets it)
symbolizes in the process with the same symbol reader and dwline.xo, so
those stay in every binary: a crash on a deployed server prints
`FUNCTION (FILE:LINE)`. Measured on hello (darwin/arm64, 2026-10-17):
618 KiB as a development build, 516 KiB as a release build, which since
decision 0238's addendum leaves the profiler out but keeps the traceback
symbolizer. So the profiler's output side (the pprof writer and its
gzip, the text summaries, the heap profile's writer) is about 102 KiB,
and that is what moves to `xo prof`; the ADR's earlier 148 KiB counted
dwline.xo, which the traceback keeps.

The raw file is decision 0240's container: the 64 byte `XORAW1` header
(kind 2 for a profile), then typed sections.

Considered and rejected: the traceback raw as well (program counters
and the build ID, `xo run` and `xo test` symbolizing them, a deployed
binary needing `xo prof`): about 50 KiB more per binary, but it would
change 0239's behavior, a crash in production printing addresses
instead of lines.

## Implementation (2026-10-17, xo-4)

- The binary (std/native/llrt/prof.xo finish) stops both profilers and
  writes one raw file at XO_PROF_OUT (default `xo-prof.xoraw`, %p the
  process id; O_EXCL, no symbolic link, mode 0600): the XORAW1 header
  (kind 2, version 1; the start time, the lost samples, the rate, the
  target), then a CPU section (rate, period, duration, lost, each
  sample's pcs) and a heap section (rate, lost, untracked, each sample's
  pcs, size and freed flag), each with its locations named: the pc, its
  frames innermost first (a function's index, its line, its file: the
  callees inlined there from .debug_info, then the function) and the
  names. The program names them itself with the symbolizer the traceback
  keeps (table_of, resolve), so `xo prof` needs neither the binary nor
  its dSYM, and a profile from a deployed server is read anywhere. One
  line on stderr names the file; XO_PROF_QUIET=1 drops it.
  XO_PROF_HEAP_OUT is gone (one file holds both).
- `xo prof FILE [--cpu-out=PATH] [--heap-out=PATH]` (selfhost/xo
  prof.xo) writes what the program wrote before, byte for byte for the
  same samples (xo-heap.txt of testdata/prof/heap at rate 1 is
  identical to main's): pprof (gzip, stored blocks) and the .txt
  summary for each, the heap profile's allocator frames left off and
  its values scaled there. It creates each file with Fs.create_new
  (O_EXCL, the owner's, 2026-10-17: nothing at the path, not even a
  symbolic link) and sets mode 0600 while it is empty; until then it
  wrote a temporary file and renamed it into place after checking the
  path, which left a window between the check and the rename.
- `xo run --profile` writes the raw file beside the binary and converts
  it into the working directory; `xo test --profile` gives each process
  `.xo-prof-<pid>.xoraw` and converts each to xo-cpu-<pid>.pprof and
  xo-heap-<pid>.pprof; neither leaves the raw file.
- Measured (darwin/arm64, arm64 backend, development builds, main's xo
  against this): hello 633,314 to 565,106 bytes (66.6 KiB less),
  json_text 741,838 to 673,693 (66.5 KiB), http_server 2,556,704 to
  2,505,135 (50.4 KiB: it links some of the moved helpers for itself).
  A release build (no profiler, decision 0238's addendum) is 528,098
  for hello: the 37 KiB between are the sampler, the heap hooks, the
  0240 trace recorder, and the location naming (table_of, resolve, the
  two sections: about 17 KiB).
- Tests: internal/llvm TestProfCPU, TestProfHeap, TestProfGating (the raw
  file's rules; xo prof's mode 0600 and its refusal to overwrite) run the
  program and then `xo prof`; TestProfDrivers runs `xo run --profile` and
  `xo test --profile` (testdata/prof/tested).
