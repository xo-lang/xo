# llvm's assembler checks ARM64 .seh directives against the instructions

Observed 2026-11-08 (Xo Lang 2, decision 0569 stage 1), assembling the
native backend's windows/arm64 assembly with `clang -target
aarch64-windows-msvc -c`.

## Symptom

Two errors that x86_64's .seh directives never gave:

- `Incorrect size for FUNC prologue: 56 bytes of instructions in range,
  but .seh directives corresponding to 44 bytes` (also with 0 bytes);
- `fatal error: error in backend: Failed to evaluate function length in
  SEH unwind info`, for the whole file, naming no function.

## Cause

On ARM64 every unwind code stands for one instruction (the unwinder
counts them to know how much of a prologue ran), and llvm checks that
the instructions between `.seh_proc` and `.seh_endprologue`, and those
of each epilogue, match the directives one for one:

- a probe loop (`sub`, `str`, `subs`, `b.ne`) with one `.seh_stackalloc`
  after it is four instructions for one code;
- a fragment opened with codes but no instructions before
  `.seh_endprologue` is the 0 byte case.

The fatal error came from a `.p2align` inside a function (a jump table's
alignment): llvm cannot compute the function's length across an
alignment fragment. A three line test reproduces it.

## Fix

compiler/arm64 winarm.xo gives every prologue and epilogue instruction
exactly one directive (`.seh_nop` for a probe), probes a loop's pages
through x17 below sp and then moves sp once with one `sub` and its
`.seh_stackalloc`, writes a fault stub fragment's two prologue
instructions once at its start (never run), and leaves the jump table's
`.p2align 2` out on windows/arm64 (code is 4 byte aligned anyway).
