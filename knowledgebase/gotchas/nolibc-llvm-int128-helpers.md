# The LLVM emitter's linux code calls libgcc's 128-bit division

Status: Observed (2026-11-16)

## Symptom

An `xo build --backend=llvm` program on linux fails to link:
`ld.lld: error: undefined symbol: __udivti3`, referenced from ordinary
loops (internal/arm64/testdata/run/latch, a sieve, main.xo:35 and 41).
xo reports only `xo: /opt/llvm/bin/clang failed`; TestDevGoldensLLVM/latch
failed on the linux/arm64 gate hosts while the same build passed on the Mac.
The 0.52.0 seed failed the same way, so it was never a regression: no
tier ran TestDevGoldensLLVM on linux.

## Cause

LLVM widens some loop trip count computations to 128 bits and calls
compiler-rt's or libgcc's `__udivti3` for the division. A linux program
links with no C library and no libgcc (decision 0206), so the helper
was missing. darwin links libSystem's compiler-rt, which has it.

## Fix

std/native/llrt/libcint.xo divides two-word unsigned 128-bit values
(shift and subtract, one 64-bit division when both fit in 63 bits), and
cabi_libc.xo exports it as `__udivti3` and `__umodti3` in no-libc builds
(an unsigned __int128 is two Int words in and out, on arm64 and amd64).
Tests: `xo test --std-module=native/llrt/libcint`, and
TestDevGoldensLLVM/latch on linux. If a link names another libgcc helper
(`__divti3`, `__modti3`, `__multi3`), add it beside these.
