# 0231: Crypto intrinsics: add with carry, AES rounds, carry-less multiply

Status: Proposed (2026-10-15)

Stub: number claimed for xo-fa's TLS performance work (the owner approved
the three intrinsics of bench/results/tls-perf-2026-10-15.md on zero-c):
rt functions for 64 bit add and subtract with carry, an AES round, and a
64 bit carry-less multiply, internal to std (decision 0074), each with a
pure Xo body the seed compiles and the native emitters replace with the
instructions (arm64 ADDS/ADCS, AESE/AESMC, PMULL; LLVM's intrinsics),
with CPU feature checks and the constant time fallbacks; filled in by
that change.
