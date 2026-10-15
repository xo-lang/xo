# 0232: External C symbols from Xo on every native OS

Status: Accepted (owner, 2026-10-15). Items 1 and 2 implemented
2026-10-15 (xo-fa, zero-c); item 3 replaced, see Implementation. The C
it deletes stays until the port lands, labelled "GLUE: needs ADR 0232"
in the zero-C audit (bench/results/zero-c-audit-refresh-2026-10-13.md).

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
3. A sanitizer build is known at run time: rt.sym_present of the
   sanitizer's weak symbols (amended 2026-10-15 in place of a constant
   target.sanitizer: the 0.11.0 seed's SSA builder builds `target` only as
   a struct of exactly two fields, so a third field would stop it building
   the runtime, and xo emits the program before clang decides on
   -fsanitize). The gate on every fiber switch stays the worker's hooks
   word, set once at worker start, so a build without a sanitizer never
   checks a symbol on a switch.

## Implementation (2026-10-15)

- std/native/sys_c.xo: the sanitizers' fiber API and edges (all
  `weak_`), and getpid for the module's own check. selfhost/ssa lowers a
  call as `rt.pc.c.<sym>` (`rt.pc.cw.<sym>` for `weak_`), and
  rt.sym_present("sym") (a string literal) as `rt.pc.cp.<sym>`; a wasm
  module faults on a call and gets false from sym_present.
- arm64: the call as a platform call on every OS; a weak reference adds
  `.weak_reference _sym` (Mach-O) or `.weak sym` (ELF) to the function's
  text; sym_present loads the address through the GOT (adrp @GOTPAGE and
  ldr @GOTPAGEOFF, `:got:` and `:got_lo12:` on ELF) and compares it
  with 0. selfhost/macho assembles both GOT forms, marks a
  `.weak_reference` symbol N_WEAK_REF, and its linker binds an import
  every reference to which is weak as a weak import (dyld leaves it 0).
  Windows: sym_present is false (no weak references yet; UNTESTED).
- LLVM: `declare extern_weak`, and sym_present an `icmp ne ptr @sym,
  null`.
- macOS ld rejects a weak reference to a symbol no image defines, so
  the driver (selfhost/xo link.xo weak_undefined_args) and the Go tests'
  links (internal/xotool WeakUndefinedFlags) add `-Wl,-U,_sym` for each
  weak reference of the program's .s or .ll
  (gotchas/macos-weak-reference-needs-U.md).
- Item 3 (target.sanitizer) is not done: the 0.11.0 seed's SSA builder
  builds `target` only as a struct of exactly two fields, so a third
  field in std/prelude.xo would stop the seed from building every
  runtime module that matches on target.os. A sanitizer build is also
  chosen when clang compiles the C runtime (`-fsanitize`), after xo has
  emitted the program, so xo cannot know it then. The runtime asks at
  run time instead: rt.sym_present of the sanitizer's symbols (one GOT
  load and compare), which is true exactly in a sanitizer build
  (internal/llvm TestSysCTSan). A constant can come with a seed that
  knows a three field Target, if a measurement ever wants the fold.
- The port (2026-10-15, after Xo Lang 3 agreed for pool.xo and sched.xo):
  std/native/llrt/sanhook.xo, a leaf over sys_c.xo, has the switch
  hooks (cabi.xo's sched_hook export, which rt.sched_hook now calls on
  both native backends), the workers' and fibers' ThreadSanitizer
  fibers, the release and acquire edges (pool.xo, tasks.xo, poll.xo),
  the Linux C library's pthread_create, and the fiber stacks' libc mmap
  under ThreadSanitizer. In a normal macOS build the sanitizer symbols
  are weak undefined imports looked up at load (nm -m: "weak external
  ... dynamically looked up"), bound to 0.
- The arm64 emitter keeps the arms of a `match` on target.os that
  another OS takes (gotchas/dead-target-arm-still-links.md), so a sys_c
  function that is not in every C library (glibc's __errno_location) is
  declared weak.
- Seeds: a sys_c call compiled by an xo older than this decision is an
  rt call of `xo_<name>`, which nothing defines. The 0.11.0 seed builds
  xo from these sources because nothing xo's own build reaches calls
  sys_c yet (the `sysc` op is unreachable there). The port of the
  scheduler's hooks will reach it, so it needs C forwarders of those
  names for the seed until a seed has this decision (the pattern of
  gotchas/seed-older-than-intrinsics.md).
- Tests: the sys_c golden (getpid called and linked, every backend),
  internal/llvm TestSysCTSan (a ThreadSanitizer build sees its symbols),
  test/codegen/intrinsics (the call, the weak directive, the GOT load,
  the null compare), selfhost/macho's encoder tests.

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
