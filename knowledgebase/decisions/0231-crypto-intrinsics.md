# 0231: Crypto intrinsics: add with carry, AES rounds, carry-less multiply

Status: Accepted (owner, 2026-10-15); part 1 (carries) implemented
2026-10-15; parts 2 (carry-less multiply) and 3 (AES rounds) to follow.

## Context

bench/results/tls-perf-2026-10-15.md: after the field, table, and
formula work (steps 1 to 4) what is left of the Xo TLS's cost is bound
by instructions Xo cannot name. Every carry of the Montgomery products
of nistec.xo and bigmod.xo was bits.Add64's formula (five instructions
where arm64 has one flag); bulk AES-128-GCM was 139M instructions a MiB
on LLVM against Mbed TLS's 14M, nearly all of it the bitsliced rounds
and the software GHASH multiply. The owner approved three intrinsics
(2026-10-15, relayed by the lead).

## Decision

1. The intrinsics are internal to std (decision 0074): functions of
   `std/native/llrt/cpu.xo`, which only std's native code imports; no
   user-visible API, no spec change.
2. Each is an ordinary Xo function with a portable, constant time body
   (no branch, no table, no comparison). The SSA builder
   (selfhost/ssa/call.xo `cpu_call`) replaces a call of one with
   `rt.<name>`, which the arm64 and LLVM emitters lower inline; a wasm
   module, and every xo older than this decision, compile the body.
   So a seed (0.11.0) builds xo from these sources with no new seed
   and no chain hop (gotchas/seed-older-than-intrinsics.md): checked by
   building selfhost/xo with the 0.11.0 seed, whose output then emits
   the instructions (test/codegen/intrinsics).
3. Part 1, carries: `cpu.carry(x, y)` (the carry out of x + y as
   UInt64s) and `cpu.borrow(x, y)` (the borrow out of x - y), 0 or 1.
   arm64: `cmn` (`cmp`) and `cset cs` (`cc`); LLVM:
   `llvm.uadd.with.overflow.i64` (`usub`), which the arm64 and x86-64
   backends select as adds/adcs (add/adc) chains. Constant time: the
   flag is computed by one data-independent instruction and moved by
   cset, never branched on; the callers that turn a carry into a mask
   still pass it through ct.mask_bit's barrier (decision 0222).
4. Part 2, carry-less multiply (planned): `cpu.clmul_lo(a, b)` and
   `clmul_hi`, PMULL/PMULL2 on arm64, PCLMULQDQ on x86-64 behind a
   CPUID check, the body BearSSL's constant time multiply; for GHASH.
5. Part 3, AES rounds (planned): an encryption round over a 16 byte
   state in memory (AESE+AESMC on arm64, AESENC on x86-64 behind a
   CPUID check for AES-NI); aesgcm.xo keeps the bitsliced rounds for
   wasm, the Go backend's absence, and CPUs without the feature.
   Hardware AES is constant time by design.

## Consequences

- Part 1 (LLVM, M instructions a handshake, step 4 to now): X25519
  with a P-256 chain 14.7 to 10.9, the P-256 group 18.6 to 13.4,
  P-384 33.6 to 23.7, RSA-2048 12.3 to 9.4, Ed25519 11.8 to 9.3
  (results file, step 5). The goldens are byte identical.
- A new function of cpu.xo needs its emitter lowering on both native
  backends and its name in cpu_call; until then it runs its body.
- The fixed point holds: s1 (seed built) runs the bodies, s2 and s3 the
  instructions, and both s2 and s3 compile cpu.xo the same way.

## Alternatives

- Bodiless `rt.*` declarations (as rt.u64_mul_hi): an older xo compiles
  the call as an external `xo_<name>`, which nothing defines, so the
  seed's build of xo fails to link; a C shim would work but the zero-C
  work is deleting C.
- A seed bump (0.12.0) first: possible, but not needed with portable
  bodies, and seeds need the owner's approval each time.
- Leaving the formulas to LLVM's pattern matching: with them the
  X25519 handshake took 15.9M instructions where an unsigned compare
  took 9.9M (the results file's proposal), and the arm64 emitter
  matches no such pattern. An unsigned compare in Xo is not constant
  time on every compiler.
