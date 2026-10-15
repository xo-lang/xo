# 0231: Crypto intrinsics: add with carry, AES rounds, carry-less multiply

Status: Accepted (owner, 2026-10-15), implemented 2026-10-15 (part 1,
carries; parts 2 and 3, carry-less multiply and AES rounds).

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
4. Part 2, carry-less multiply: `cpu.clmul_lo(a, b)` and `clmul_hi`,
   the halves of the 128 bit product: PMULL on arm64 (fmov to v30 and
   v31, pmull, fmov or umov back), PCLMULQDQ on x86-64 (LLVM inline
   asm; a pure asm, so LLVM shares one instruction between the two
   halves). The body is BearSSL's constant time multiply (the high half
   through bit reversals). aesgcm.xo's GHASH takes the three Karatsuba
   products whole with them (Gcm.mul_hw), then the same reduction.
5. Part 3, AES rounds: `cpu.aes_round(st, rk)` (arm64's AESE then
   AESMC: MixColumns(ShiftRows(SubBytes(st xor rk))) on 16 bytes in
   memory) and `cpu.aes_last(st, rk, rk2)` (AESE then the last key).
   x86-64 maps them to pxor then AESENC with a zero key, and AESENCLAST
   with rk2. The body works on eight bytes a word: the S-box as the
   field inverse (x^254, eleven SWAR products) and the affine map, no
   table. aesgcm.xo's Cipher keeps the round keys in FIPS 197's order
   beside the bitsliced ones and encrypt4 takes the rounds (hw4) when
   the CPU has them; decryption (TLS 1.2 CBC only) stays bitsliced.
   Hardware AES is constant time by design.
6. Neither part 2 nor part 3 is in every CPU (ARMv8 without the crypto
   extension, x86-64 without AES-NI or PCLMULQDQ), and a lowered call
   is the instruction whatever the CPU: callers use them only when
   `cpu.hw()` has the bit. hw() looks once a process (rt.static_word
   30): macOS arm64 always; Linux arm64 AT_HWCAP's AES and PMULL bits
   (linux.auxval); x86-64 CPUID leaf 1 (`cpu.cpuid1_ecx`, an LLVM
   inline asm); Windows arm64 none for now; and none where the module
   is not lowered (`cpu.lowered()`, 0 in its body: wasm, and code an
   older xo compiled). The software paths stay for those.
7. Assembly: the instructions need the crypto extension in an ARMv8
   assembler, so the arm64 emitter's Linux file header says
   `.arch_extension crypto` (selfhost/macho ignores it) and the LLVM
   inline asm starts with it; macOS's default CPU has it.
8. Tests: the `cpu_prims` golden compares every op (and its body, the
   `_soft` ops, which no emitter replaces) with a plain Go reference
   (std/rt cryptoCPU) on every backend; test/codegen/intrinsics checks
   that nistec's products, aesgcm's rounds, and GHASH contain the
   instructions; selfhost/macho's encoder tests have each new
   encoding as clang assembles it. The crypto goldens use std/internal,
   which the wasm backend cannot build yet (the scheduler), so the
   `_soft` ops are what tests the bodies.

## Consequences

- Part 1 (LLVM, M instructions a handshake, step 4 to now): X25519
  with a P-256 chain 14.7 to 10.9, the P-256 group 18.6 to 13.4,
  P-384 33.6 to 23.7, RSA-2048 12.3 to 9.4, Ed25519 11.8 to 9.3
  (results file, step 5). The goldens are byte identical.
- Parts 2 and 3 (bulk, M instructions a MiB, LLVM then arm64):
  AES-128-GCM 138.6 to 27.1 and 205.0 to 38.0, AES-256-GCM 171.6 to
  30.2 and 253.6 to 41.6 (Mbed TLS 14.0 and 15.0).
- A new function of cpu.xo needs its emitter lowering on both native
  backends and its name in cpu_call; until then it runs its body.
- UNTESTED: Linux (AT_HWCAP, the `.arch_extension` header, GNU as),
  x86-64 (CPUID, PCLMULQDQ, AESENC), and Windows.
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
