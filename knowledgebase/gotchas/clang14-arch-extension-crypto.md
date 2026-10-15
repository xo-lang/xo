# LLVM 14's assembler: `.arch_extension crypto` does not enable aes or sha2

Status: Observed (2026-10-15, xo-fa, the arm64 goldens in a Debian
bookworm container: Debian clang 14.0.6).

## Symptom

Every arm64-backend build of a program that reaches std/native/llrt
cpu.xo's crypto instructions (the TLS, HTTP, and crypto goldens: 27 of
178) fails to assemble on Linux:

    prog-linux.0.s:17049:2: error: instruction requires: aes
            aese v30.16b, v31.16b

The Mac's clang and GNU as accept the same file.

## Cause

The arm64 emitter declares `.arch_extension crypto` (decision 0231: the
code runs the instructions only when the CPU has them). For ARMv8.0,
LLVM 14 maps `crypto` to nothing it checks for these instructions; it
wants the names `aes` and `sha2` (pmull is under aes).

## Fix

The emitter writes all three (selfhost/arm64 gen.xo):
`.arch_extension crypto`, `.arch_extension aes`, `.arch_extension sha2`.
Clang 14, clang 19, Apple clang 17, and GNU as 2.40 assemble that.
