# amd64's baseline is SSE2: an instruction past it needs a CPUID check

Status: Observed (2026-10-27, xo-lang-4; caught before it was pushed)

**Symptom.** The SSSE3 ChaCha20 kernel (compiler/ssa x86chacha.xo:
pshufb, and SSE4.1's pinsrd in the fused ChaCha20-Poly1305 kernel) ran
on every amd64 CPU, with no check. On an x86-64 CPU without SSSE3 (the
first Athlon 64s and Pentium 4s) it would fault with SIGILL. No test could
see it: every machine that runs the tests has SSSE3.

**Cause.** x86-64's guaranteed instruction set is SSE2 (Go's GOAMD64=v1;
Xo states no higher level). AES-NI, PCLMULQDQ, SHA-NI and FMA3 kernels
already ran only under their cpu.hw() bits, but a "plain SIMD" lowering
looked baseline when it was not.

**Fix.** cpu.HW_SSSE3 (CPUID.1 ECX bits 9 and 19: SSSE3 and SSE4.1),
checked in chacha.xo before cpu.chacha_stream4, with
cpu.chacha_stream4_soft otherwise. The rule:

- Any lowering that uses an instruction past SSE2 needs a cpu.hw() bit,
  checked by every std caller (or by the callers of a helper).
  internal/repotest TestX86SIMDGated lists these intrinsics with their
  bits (x86Needs), and fails on a call from a function that neither
  checks the bit nor is a listed helper (x86GateHelpers, with the
  callers' check). Add a new intrinsic to x86Needs when it is lowered.
- Run the fallback, not only compile it: build from a copy of std whose
  cpu.hw() masks the bit off (`detect()` result `& ~(HW_SSSE3)`), run
  the goldens and the twin against Go on a CPU that has the instruction.
  On 2026-10-27 that was the chacha_blocks4 and crypto_prims goldens and
  the chacha twin's checksums on an i7-9750H with HW_SSSE3 off.
