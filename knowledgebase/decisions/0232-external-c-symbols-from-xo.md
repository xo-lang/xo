# 0232: External C symbols from Xo on every native OS

Status: Accepted (owner, 2026-10-15). Implementation by xo-fa on the
zero-c branch; until it lands the C it deletes stays, labelled "GLUE:
needs ADR 0232" in the zero-C audit
(bench/results/zero-c-audit-refresh-2026-10-13.md).

## Context

The zero-C push (decision 0206 and its audit) moves the C runtime into
std/native. Xo code calls a platform's C library only through the
declaration files std/native/sys_darwin.xo (libSystem) and
std/native/sys_windows.xo (kernel32, ws2_32, ntdll): a bodiless function
there is a C ABI call of the symbol of its name. Those calls are bound to
their OS: selfhost/ssa/call.xo platform_call faults "the platform call of
<os> on another OS", and the LLVM and arm64 emitters trap. On Linux Xo
makes raw system calls and has no way to call a C symbol.

That leaves C in xo_rt.c that exists only to call such symbols
(2026-10-15, after the scheduler block's batch S1, zc-b19):

- the sanitizers' fiber API around every fiber switch (xo_sched_hook:
  __tsan_create_fiber, __tsan_destroy_fiber, __tsan_switch_to_fiber,
  __tsan_get_current_fiber, __sanitizer_start_switch_fiber,
  __sanitizer_finish_switch_fiber) and the ThreadSanitizer edges the Xo
  code needs (rt.sched_rt 22 and 23: __tsan_release, __tsan_acquire);
- the C library calls the Linux C library build must make itself:
  pthread_create (glibc's thread setup: rt.sched_rt 8, pool.xo
  thread_start's Linux arm) and the fiber stacks' mmap and munmap
  (rt.sched_rt 11 and 12: ThreadSanitizer must see the mappings, which
  it learns only through its mmap interceptor).

The sanitizer symbols exist only in sanitizer builds, where the C
compiles the calls under #ifdef XO_TSAN or XO_ASAN; a plain reference
from Xo would fail every other link. Xo has no compile time switch for a
sanitizer build.

## Decision

1. A declaration module std/native/sys_c.xo, imported only by runtime
   modules (decision 0074), whose bodiless functions are C ABI calls of
   the symbol of their name on every native target (glibc's or musl's
   libc on Linux, libSystem on macOS, the C runtime on Windows; a fault on
   wasm). The SSA builder lowers them as platform_call does, without the
   OS check (aux rt.pc.c.<name>); the LLVM and arm64 emitters emit the
   call. XO_NOLIBC builds resolve the libc names to the Xo exports of
   cabi_libc.xo (decision 0230), as the C does today.
2. A weak form: a declaration marked weak (a `weak_` name prefix, as
   `va_` marks variadic parameters, or an attribute if the language gets
   one) is a weak undefined reference: extern_weak in LLVM IR, a weak
   reference in the arm64 emitter's Mach-O (N_WEAK_REF) and ELF
   (STB_WEAK) output, and on PE/COFF a weak external with a null default.
   rt.sym_present(f) (an intrinsic folding to the address compared with
   0) tells whether a weak symbol was linked.
3. A constant target.sanitizer (none, thread, address), set by the
   driver for a sanitizer build (the C's XO_TSAN and XO_ASAN switches), so
   `match target.sanitizer` folds the sanitizer calls away in every other
   build, as `match target.os` folds OS arms.

## Consequences

- Deletes (estimate): xo_sched_hook and its ASAN and TSAN macros and
  declarations, tsan_worker and sched_hooks, rt.sched_rt ops 4 and 5's
  fiber create and destroy, 8, 11, 12, 22 and 23, and fiber_map_failed's
  report: about 150 to 200 lines of xo_rt.c, and the import cycle
  workarounds pool.xo has for them (a new leaf module over sys_c.xo that
  pool.xo imports).
- Compiler work: selfhost/ssa (call lowering, the constant), selfhost/llvm
  and selfhost/arm64 (the calls, the weak references in both object
  formats), the driver (the sanitizer constant): an estimated 300 to 500
  lines, with tests (codegen patterns, a golden that calls a libc
  function on each OS, a sanitizer build's TSan run through heavy.sh).
  It touches what the self build compiles, so it needs the fixed point.
- Seeds: std/native is compiled into xo itself, so the first use of
  sys_c.xo in a module the compiler's runtime imports makes older seeds
  fail (gotchas/seed-older-than-intrinsics.md). Either the uses stay out
  of the compiler's own build until a seed with the feature exists, or
  the change bumps the minor version and cuts seeds for every platform
  (toolchain.md "Cutting a seed"), which the owner approves.
- It opens every C library function to the runtime on Linux too, which
  the zero-C direction mostly avoids (raw system calls); the module's
  doc comment should list why each declaration is there.

## Alternatives

- Keep the C (the current state): small, sanitizer and Linux glibc only,
  and the owner's rule is Mac first with OS tuning at the end.
- Raw system calls only: mmap without the interceptor leaves
  ThreadSanitizer's shadow wrong for fiber stacks, and clone instead of
  pthread_create breaks glibc's thread state (TLS, errno, cancellation),
  so neither replaces the C in C library builds.
- Per OS declaration files (a sys_linux.xo beside sys_darwin.xo):
  solves the libc calls but not the sanitizer symbols, which need the
  weak form and the constant anyway.
