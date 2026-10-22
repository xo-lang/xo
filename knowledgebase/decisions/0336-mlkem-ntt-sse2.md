# 0336: ML-KEM's NTT loops in SSE2 on x86-64

Status: Accepted (Xo Lang 4, 2026-10-22, the lead's NTT item: design
note ntt-vector-design.md option A, decision 0329's pattern)

## Context

TLS 1.3's X25519MLKEM768 key share runs ML-KEM's number theoretic
transform, its inverse and the NTT domain products in every handshake.
On linux/amd64 the native backend ran them as scalar loops (17
instructions a butterfly), while LLVM's loop vectorizer did two 64 bit
lanes with SSE2: tls-newconn per request (Docker callgrind, native at
7a9a083d9, LLVM at a32988ede) inverse_ntt 72,952 native against 58,608
LLVM, ntt 57,177 against 47,220, ntt_mul_acc 75,756 against 63,564. On arm64 LLVM does
not vectorize them (no vector instruction in the three; its edge was
scalar, closed by d42108db5's addressing).

Coefficients are 8 byte Ints below 2^16 in the lazy layers and every
product fits 32 bits (std/native/llrt mlkem.xo's bounds), so pmuludq
(32 by 32 to 64 bit, SSE2, in every x86-64 CPU) computes two at once.

## Decision

- cpu.mlkem_ntt(p, zp), cpu.mlkem_inverse_ntt(p, zp) and
  cpu.mlkem_mul_acc(h, f, g, gp) in std/native/llrt cpu.xo, with
  portable bodies (the loops moved from mlkem.xo, which calls them), as
  decision 0231's intrinsics.
- compiler/ssa cpu_call maps them to rt.* on amd64 only; arm64 (both
  backends) and wasm run the portable bodies.
- compiler/ssa x86mlkem.xo holds the instruction lines both x86-64
  emitters use (as x86sha.xo for SHA-NI): two coefficients an xmm
  register, one in each 64 bit lane; Barrett (bred) as pmuludq, psrlq
  32, pmuludq q, psubq; the full reduction (bfull) adds the sign by
  psrad 31, pand q (each lane's value fits 32 bits). The forward NTT
  runs the len 2 layer like the others and reduces every coefficient
  after; the products split two pairs into lanes by punpcklqdq and
  punpckhqdq and join them again for the stores.
- Native (compiler/arm64 calls.xo): one instruction with its text,
  clobbering rax, rcx, rdx, r8 to r10 and xmm0 to xmm5 (caller saved on
  System V and Windows), with xmm14, xmm15 and r11 the emitter's scratch
  registers. LLVM (compiler/llvm call.xo): the same lines as inline asm,
  those registers clobbered.

## Consequences

- tls-newconn linux/amd64 per request (Docker callgrind, 154 requests):
  native 3,202,802 to 3,115,327 (ntt 57,177 to 31,473, inverse_ntt
  72,952 to 38,420, ntt_mul_acc 75,756 to 48,180); LLVM 3,017,038 to
  2,963,024 (base a32988ede: ntt 47,220 to 31,473, inverse_ntt 58,608 to
  38,420, ntt_mul_acc 63,564 to 48,216): both backends below LLVM's own
  vectorized loops.
- The text is hand written: a change to mlkem.xo's arithmetic must
  change cpu.xo's bodies and x86mlkem.xo's lines together. The goldens
  (crypto_mlkem's known answers, the TLS 1.3 goldens with ML-KEM) check
  both through amd64 Docker runs; the arm64 gates run the portable
  bodies only.
- An older xo (a seed) runs the portable bodies: no new seed.

## Alternatives

- An integer lane SLP or loop vectorizer pass in compiler/arm64's MIR
  for x86 (extending 0323's Float pairing): helps any similar loop, much
  larger; still a candidate.
- SSE4.1 pmulld with four 32 bit lanes behind cpuid: needs the
  coefficients narrowed to 32 bits in registers and a feature test;
  measure after this.
- NEON on arm64 (four 32 bit lanes): native arm64 now matches LLVM's
  scalar loops; next candidate there.

## Addendum (Xo Lang 4, 2026-10-22): NEON on arm64

The Alternatives' NEON item, done: compiler/ssa cpu_call maps the three
intrinsics on arm64 too (both backends); wasm keeps the portable bodies.

- compiler/ssa a64mlkem.xo holds the lines both arm64 emitters use: four
  coefficients a vector register (32 bit lanes; the 8 byte Ints narrowed
  by uzp1 on load, widened by ushll/ushll2 on store); Barrett as umull,
  umull2, uzp2 (the high halves), mul and sub; the full reduction adds
  the sign (sshr 31, and q). The layers len 128 to 4 run four
  butterflies a step; len 2 pairs each group's halves by zip1/zip2 .2d.
- Native (compiler/arm64 calls.xo): one instruction with its text,
  clobbering x9 to x15 and v16 to v31 (caller saved; a callee keeps only
  v8 to v15's low halves). LLVM (compiler/llvm call.xo): the same lines
  as inline asm with those clobbers; the amd64 arm is now guarded by
  the target.
- compiler/macho encodes the new forms (dup .4s from w, umull/umull2,
  uzp2, mul/sub .4s, sshr .4s, and .16b, zip1/zip2 .4s), tested in
  macho_test.xo.
- Measured (the crypto_mlkem golden, Mac, instructions retired, least
  of 3, base 185699ae0): native 793,576,441 to 634,053,655 (20% fewer),
  LLVM -O2 849,332,618 to 710,645,372 (16% fewer), output identical. The
  arm64 gates now run the vector lines (crypto_mlkem's known answers,
  the TLS 1.3 goldens with ML-KEM, cpu_prims), so the Consequences' "the
  arm64 gates run the portable bodies only" holds for wasm only.
- `xo bench --ab` (decision 0340), crypto_mlkem built by ac992d4e5's xo
  (A) and 177602f83's (B), 10 interleaved rounds each, none loaded
  (2026-10-22): native arm64 wall 41.5 to 34.6 ms, B/A 0.833 (95%
  interval 0.816 to 0.849), better; LLVM -O2 wall 40.4 to 34.7 ms, B/A
  0.86 (0.824 to 0.881), better. CPU time is at its 10 ms resolution
  (30 and 30 ms) in runs this short, so wall time decides.
