# LLVM on linux/amd64: a fault crashes at the fiber's stack top, and the dump names the runtime

Status: Observed and fixed 2026-11-12 (Xo Lang 2).

## Symptom

On the linux/amd64 offload host, five cmd/xo tests failed on every main
back to at least 0.46.0's base (not a regression, never passing):
TestBuildRunLLVM, TestHolesNative, TestFieldEnsuresResult and
TestTracebackIsolatedNoLeak exited 139 (`fatal error: SIGSEGV ... at
address 0x7...fe000`, a page boundary) where a fault should exit 70, and
TestDump/native printed `blocked in sleep at poll.xo:1187` where the
program's `main.xo:9` was wanted. The same programs built with
`--backend=native` were right. On linux/amd64 the tests' "native" backend
is LLVM (`xotool.NativeBackend()`: arm64 only on arm64 hosts), which hid
that every failure was an LLVM build.

## Cause

Two separate bugs:

1. The crash: compiler/llvm emits `xo_fiber_entry` as a naked function.
   On linux/amd64 clang gives it an FDE with the default rule, the return
   address at CFA-8. A fault's unwind (std/native/llrt/unwind.xo raise)
   walks with no stack bounds (`step(s, cur, 0, Int.max)`) to the end of
   the stack, so it stepped past the fiber's first frame and read the
   word just above the fiber's stack: SIGSEGV with the fault address
   equal to the stack's top. linux/arm64 already marked that frame's return
   address undefined (`.cfi_escape 0x07, 30` plus the `uwtable`
   attribute, so clang opens a CFI region); amd64 did not. The native
   amd64 emitter gives `xo_fiber_entry` no CFI at all, so its walk stops
   at a failed lookup.
2. The dump: LLVM's line tables name runtime files
   `std/native/llrt/poll.xo` with no directory; the native backends' name
   them `./std/native/llrt/poll.xo` (directory `.`). dump.xo's test for
   std's files was `contains("/std/")`, so in an LLVM build the
   runtime's park frame counted as the program's. On arm64 the
   fiber walk reaches the same test, but arm64 hosts build "native" with
   the arm64 backend, so no Mac or arm64 run saw it.

## Fix

compiler/llvm/sched.xo switch_asm: `.cfi_escape 0x07, 16`
(DW_CFA_undefined of rip) at the start of `xo_fiber_entry` on linux and
freebsd amd64, with `uwtable` (sched_switch); sched_test.xo checks the
directive per target. dump.xo std_file: `starts_with("std/") ||
contains("/std/")`, as compiler/debug tasks.xo already tested.

## How to avoid

- A test that fails only on one host: check which backend the test's
  helper picks there before reading the failure as a backend-neutral one.
- A new naked or hand-written function that can be a stack's outermost
  frame needs its return address marked undefined on every unwinder that
  walks without bounds.
- Tell std's files from the program's by one rule (both `std/` forms);
  compiler/debug tasks.xo, compiler/ssa/stdfacts.xo and dump.xo each had
  their own.
