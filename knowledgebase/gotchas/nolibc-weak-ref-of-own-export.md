# XO_NOLIBC: a weak platform call of a C name the program itself exports

Status: Observed (2026-10-15, xo-fa, cross linking XO_NOLIBC programs for
linux/arm64 and linux/amd64 on the Mac for the backend entry stubs).

## Symptom

An XO_NOLIBC=1 build fails before linking:

- LLVM backend: `prog.ll: error: invalid redefinition of function
  '__errno_location'`;
- arm64 backend, assembled by LLVM's assembler: `error:
  __errno_location changed binding to STB_GLOBAL`.

And, before that, a linux/amd64 LLVM cross build from the arm64 Mac
compiled the runtime with `-mno-outline-atomics` and linked the arm64
outline atomics assembly (`cbnz w15, 1b` in an x86_64 compile).

## Cause

std/native/llrt calls `__errno_location` as a weak platform call (`cw`,
decision 0232), which declares it `extern_weak` (LLVM) or `.weak`
(arm64); in an XO_NOLIBC build cabi_libc.xo also exports that name, so
the module declares and defines it (LLVM rejects both in one module;
GNU as may let `.globl` win, LLVM's assembler does not). The atomics:
compiler/xo main.xo chose the nolibc flags by the host's architecture
for the LLVM backend, not the target's.

## Fix

The emitters skip the weak declaration of a name the module exports
(compiler/llvm gen.xo generate drops the declare; compiler/arm64
calls.xo weak_ref returns early), and the driver takes the target's
architecture for the nolibc flags and atomics (link_inputs' tgoarch).
