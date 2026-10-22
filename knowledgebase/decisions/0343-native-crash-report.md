# 0343: Native crash reports for hardware faults

Status: Accepted (owner, 2026-10-22: keep the signal's exit status, so
core files and WER dumps keep working; one xo-windows CI run approved for
stage 2). Proposed the same day by Xo Lang 4; the owner approved building
the tool after 0.25.1's windows/amd64 xo.exe crashed on every build and
left only a minidump.

## Context

A Xo fault (core 8: overflow, bounds, contracts, `fault`) is reported by
the runtime: one line, and with `XO_TRACEBACK` the frames with their
files and lines (decisions 0239, 0268). A **hardware fault** is not: an
access violation, an illegal instruction, a trap, a bus error. Safe Xo
code should never raise one, so when it happens it is a miscompile, a
runtime bug, a linker bug, or C code behind `use c`, which are exactly
the cases where a report matters most.

What a native program (arm64, amd64 and LLVM backends, all linking
std/native/llrt) does today, read from entry.xo `signals()` on
2026-10-22:

- Linux and macOS install handlers only for SIGINT, SIGTERM, SIGQUIT
  (`xo dump`) and SIGPIPE (ignored). SIGSEGV, SIGBUS, SIGILL, SIGFPE and
  SIGTRAP take the default action: the process dies with status
  128+signal and the shell prints "Segmentation fault" or "Trace/breakpoint
  trap" with no address and no frame.
- Windows installs one vectored handler, on_overflow, for
  EXCEPTION_STACK_OVERFLOW only. Any other exception ends the process
  silently with the NTSTATUS as exit code (0xC0000005 for an access
  violation).
- The generated code's own traps also land there: arm64 `brk #1` after a
  call that never returns and in the fiber entry (SIGTRAP), x86 `ud2` in
  the same places (SIGILL, EXCEPTION_ILLEGAL_INSTRUCTION).

The cost was measured on 0.25.1: the Windows crash took a CI run with
cdb attached, a minidump, a watcher script with hand found offsets, and
lldb reading xo.exe's DWARF against the dump's load base, before the
cause (COFF relocation count overflow,
gotchas/coff-relocation-count-overflow.md) was found statically. A report
naming `write_dumps (main.xo:1552)` and the faulting address would have
been the first line of the CI log.

The runtime already has every piece the report needs:

- unwind.xo `prof_walk`: the frames of a thread from a signal's ucontext,
  async signal safe (no allocation, no lock, no C library call), with
  stop conditions for a wild stack (decision 0238, the sampling
  profiler). `prof_init` and `prof_text` find the tables and the text
  bounds ahead of time.
- Windows: the profiler's sampler walks a suspended thread's CONTEXT with
  the image's .pdata and RtlVirtualUnwind (unwind.xo `win_image`).
- dwline.xo: function, file and line from the program's own DWARF, on
  Linux (/proc/self/exe), macOS (the dSYM with a matching LC_UUID) and
  Windows (the PE's .debug_ sections, COFF long names); prof.xo names
  functions from the symbol table.

## Decision

Native programs install a crash handler at start that reports a
hardware fault on standard error and then lets the operating system's
default action happen, so exit status, core files, Windows Error
Reporting and attached debuggers see the same crash they see today.

### What is caught

- Unix (Linux, macOS): SIGSEGV, SIGBUS, SIGILL, SIGFPE, SIGTRAP, with
  SA_SIGINFO, SA_ONSTACK and SA_RESETHAND. Each OS thread the runtime
  creates (main, pool workers, the poller, the profiler's threads,
  threads from sanhook.xo `thread_start`) gets an alternate signal stack
  (sigaltstack, 64 KiB plus a guard page, from mmap) so a fault on a
  guard page (an overflowed fiber or thread stack) is still reported.
- Windows: SetUnhandledExceptionFilter, for every exception code but
  EXCEPTION_STACK_OVERFLOW (on_overflow keeps it). An unhandled filter,
  not a vectored handler: a vectored handler runs first and would report
  exceptions that C code behind `use c` catches with its own SEH.
- Not installed under `--race` and `--asan`: the sanitizer runtimes
  install their own SEGV handlers and report with their own stacks.
- `XO_CRASH_REPORT=0` installs nothing (for C code that wants the
  signals, or to compare with the old behavior).

### What is reported

```
fatal error: SIGSEGV: segmentation violation (SEGV_MAPERR) at address 0x10
pc 0x100003a1c (prog+0x3a1c) in xo_f_main_2efill (main.xo:12)
thread 3 (worker 2)

registers:
  x0  0x0000000000000010  x1  0x0000000000000003  ...
  fp  0x000000016fdfe9f0  lr  0x0000000100003b40  sp  0x000000016fdfe9d0

traceback:
  main.fill (main.xo:12)
  main.main (main.xo:30)
```

- The first line: the signal or exception name, its sub-code (si_code;
  for an access violation, read, write or execute), and the faulting
  data address where the system gives one (si_addr,
  ExceptionInformation[1]).
- The pc as an absolute address and as module plus offset (the
  executable's name, or the shared library's for a pc outside the
  program's text), so a report without symbols can be resolved later.
- The registers of the faulting context, by architecture.
- `traceback:` in the format of decision 0239 (`  FUNCTION (FILE:LINE)`,
  the callees inlined at a pc first), with `  module+0xOFFSET` for a
  frame no table names. With `XO_TRACEBACK=locals` each frame gets its
  variables, as for a Xo fault.

### How it is written: two phases

1. **Async signal safe.** In the handler, on the faulting thread: claim
   the process's one crash record with an atomic exchange (a second
   thread that faults while a report is written prints one line and
   waits for the first to end the process), walk the frames with
   `prof_walk` (Windows: the CONTEXT from EXCEPTION_POINTERS through
   .pdata), and write the first line, the pc, the thread and the
   registers with write(2) or WriteFile from a fixed buffer. Every
   frame's pc is kept. After this phase the report has everything needed
   to resolve it offline.
2. **Symbolization.** The frames are named with prof.xo's symbol tables
   and dwline.xo, which read files and allocate. A crashing process may
   hold the heap's lock or have a corrupt heap, so this phase is
   guarded: a fault inside it reaches the same handler, which sees the
   phase in the crash record, prints `(symbolization faulted)` and goes
   to the default action; a deadlock is ended by a watchdog (Unix:
   alarm(XO_CRASH_TIMEOUT, 5 s by default), whose handler restores the
   original signal's default action and raises it; Windows: a thread
   started at phase 2 that waits that long). Either way phase 1's lines
   are already on standard error.

### How it ends

- Unix: the handler returns. SA_RESETHAND restored the default action,
  so the faulting instruction runs again and the kernel ends the process
  with the same signal (status 128+signal, a core file where enabled).
  A signal sent by another process (si_code SI_USER or SI_QUEUE) does not
  repeat on return, so the handler raises it itself.
- Windows: the filter returns EXCEPTION_CONTINUE_SEARCH; Windows Error
  Reporting, LocalDumps and an attached cdb see the exception as before,
  and the exit code stays the NTSTATUS.

## Plan

1. Unix (darwin and linux, arm64 and amd64): the handler, alternate
   stacks for the runtime's threads, both phases, the watchdog. Tests
   (darwin/arm64, linux/arm64 in Docker, amd64 in the Docker sample): a
   program whose `use c` function writes through a null pointer, one
   that reaches `brk`/`ud2`, and a stack overflow by C recursion; each
   checks the first line, the faulting Xo frame's file and line, that
   the exit status is the signal's, and that `XO_CRASH_REPORT=0` prints
   nothing.
2. Windows (amd64): the unhandled filter, phase 1 from the CONTEXT,
   phase 2 with the PE's DWARF. Verified by one approved CI windows run
   (Wine runs only single-threaded smoke tests); the test asserts the
   same lines.
3. Offline resolution: `xo dev symbolize BINARY < report` resolves the
   `module+0xOFFSET` frames of a phase-1-only report (a stripped release
   binary, or symbolization that faulted) with compiler/debug's readers.
4. The spec (section 8, next to the fault report) and toolchain.md
   describe the report, `XO_CRASH_REPORT` and `XO_CRASH_TIMEOUT`.

The PDB/CodeView writer for the PE linker (owner approved the same day)
comes after stage 2: it serves debuggers and WER dumps, while this
report serves the log.

## Consequences

- A hardware fault in a native program prints where it happened, in CI
  logs and users' terminals, with no debugger and no dump.
- Exit status, core files, WER and debuggers behave as before.
- Every runtime thread maps a 64 KiB alternate stack (address space,
  touched only on a fault).
- The handler code is in every native binary (estimated under 8 KiB of
  text; measured in stage 1 against hello's size).
- Phase 2 can still hang or fault in a badly corrupted process; the
  guard limits that to a missing traceback, never a missing first line
  or a changed exit status.
- Native builds still write no crash bundle (spec 8); a bundle for
  hardware faults would be a separate decision.

## Alternatives

- **Leave it to the system** (core files, macOS .ips reports, WER
  dumps): off by default in containers and CI, needs a debugger and the
  matching binary to read, and names no Xo function. This is what 0.25.1
  needed a day of CI runs for.
- **A vectored exception handler on Windows**: sees first chance
  exceptions, including ones C code handles; it would report crashes
  that are not crashes. The unhandled filter sees only what would end
  the process.
- **Symbolize only offline** (phase 1 plus `xo dev symbolize`): safe in
  every state, but every report would need the binary and a second step;
  phase 2 gives the names in place and phase 1 still covers its failure.
- **Exit with status 70 like a Xo fault**: would hide the signal from
  shells, core dumps and WER, and mix bugs below the language with
  faults the language defines.
