# 0565: a Vec128 value type for vector intrinsics

Status: Accepted (owner, 2026-11-08, on the lead's recommendation). Proposed (Xo Lang 2, 2026-11-08; the owner decides: it adds a
type class to the SSA and to every native register allocator)

## Context

Decision 0231's crypto intrinsics pass 128-bit values across the call in
one of three ways, and each costs something:

1. As two Ints. `cpu.clmul_lo(x, y)` and `clmul_hi` move both operands
   from general registers into vector registers (arm64 `fmov` to v30 and
   v31), multiply, and move the half back (`fmov` or `umov`). GHASH's
   Karatsuba step (aesgcm.xo `Gcm.mul_hw` and the bulk loop beside it)
   takes six products per block, so twelve moves in and six out around
   six PMULLs, and the reduction then runs in general registers.
2. Through memory. `cpu.aes_round(st, rk)` and `aes_last` take
   addresses: the state is stored, loaded into a vector register, run
   through AESE and AESMC, and stored again for the next round to load.
   A load wider than the stores before it is not served by store
   forwarding: aesgcm.xo's key schedule did SubWord with `aes_last` on a
   two word list (two 8 byte stores, then one 16 byte load), ten times
   in a chain for AES-128, and `sub_word_hw` was 14.4% of
   bench/perf/aesgcm key on the Mac (decision 0231's addendum of
   2026-11-07). That one site was fixed by a new scalar intrinsic,
   `cpu.aes_sub_word` (key 0.717 of main, 7 rounds on linux/arm64 host
   1), not by removing the pattern.
3. As a fused kernel. `cpu.aes_enc10/12/14`, `aes_ctr10/12/14`,
   `aes_gcm8_enc10` and the rest keep the state and the round keys in
   vector registers for a whole block or eight, but each is a hand
   written sequence per backend (arm64 native, x86-64 native, LLVM
   inline asm) and per key size, so each new shape is about nine
   sequences and their encoder rows.

The pattern recurs whenever a std algorithm wants a vector instruction
that is not yet in a kernel: the 2026-11-07 survey
(bench/results/xo-vs-go-survey-2026-11-07.md) still has aesgcm/open at
1.290 of Go, aesgcm/key at 1.274 (before aes_sub_word), aesgcm/seal at
1.144 and aesgcm/open1 at 1.107. Decision 0532 orders the SIMD work as
intrinsics for search shapes, idiom recognition, then a loop vectorizer;
none of those give std code a way to keep a 128-bit value in a register
between two intrinsic calls.

## Decision (proposed)

1. A value type `cpu.V128` in std/native/llrt/cpu.xo, internal to std as
   0231's intrinsics are (decision 0074): no user visible API and no
   spec change in this decision. Its body is a struct of two Ints (lo,
   hi), so wasm, the Go backend and every older xo compile it as
   ordinary code, and no seed bump is needed (the compiler's own code
   does not use it; gotchas/seed-older-than-intrinsics.md).
2. Operations, each an intrinsic with a portable body like 0231's:
   `v128(lo, hi)`, `.lo()`, `.hi()`, `v128_load(addr)`, `v128_store(addr,
   v)`, `xor`, `and`, `aes_round_v(st, rk)`, `aes_last_v(st, rk, rk2)`,
   `clmul_v(a, b, sel)` (PMULL or PMULL2, PCLMULQDQ's immediate), and
   the byte and lane moves GHASH's reduction needs (`ext`, `shl_lanes`,
   `shr_lanes`). The list grows by the same rule as 0231: a body, a
   `cpu_prims` golden row against a Go reference, a codegen pattern.
3. The native backends give V128 SSA values the vector register file:
   arm64 the q view of the registers floats use (v8 to v15 are callee
   saved only in their low 64 bits, so a V128 live across a call is
   spilled whole), x86-64 xmm; spill slots are 16 bytes and 16 byte
   aligned. LLVM maps it to `<2 x i64>` and the operations to its
   intrinsics (`llvm.aarch64.crypto.aese`, `llvm.x86.aesni.aesenc`,
   `llvm.aarch64.neon.pmull64`, `llvm.x86.pclmulqdq`), so clang
   allocates it. A V128 is passed and returned in a vector register
   inside std; it never crosses a C or exported boundary.
4. The fused kernels stay: they are already near Go and they hold round
   keys in registers across a loop, which a register allocator will not
   always match. V128 is for the next shapes and for the single block
   and setup paths around the kernels.

## Prototype plan (each step measured before the next)

0. Baseline: `xo bench --ab` of bench/perf/aesgcm (open1, open, seal,
   key) on linux/arm64 host 1, and callgrind's instruction count of
   `Gcm.mul_hw`'s loop, so the moves of point 1 above are counted, not
   assumed.
1. arm64 native: the SSA type class, the q register class in the
   allocator, the spill slots, and five operations (`v128`, `.lo`,
   `.hi`, `clmul_v`, `xor`). Rewrite `Gcm.mul_hw` and the bulk GHASH
   loop on V128 with the reduction still scalar. Proving: cpu_prims and
   the crypto goldens on arm64, LLVM and Go; `--ab` of aesgcm/open1
   (the path with the most single block GHASH). Keep it only if open1
   improves outside noise.
2. The reduction in vector registers (Go's arm64 gcmReduce shape:
   PMULL by the reduction constant, `ext`), then the key schedule on
   `aes_last_v` with the state in a register (it would retire
   `aes_sub_word`'s splat if it is as fast).
3. LLVM: `<2 x i64>` and the intrinsics; compare against step 1's
   native numbers.
4. x86-64 native: xmm allocation and the same operations; the amd64
   sample on the linux/amd64 offload host and `--ab` there.
5. Only then a decision on a user visible vector type (the owner's
   SIMD request of 2026-11-06), with this one's measurements.

## Consequences

- The allocators of both native backends learn a 16 byte class; the
  arm64 float class is the start (same registers, wider spills).
- New crypto shapes can be written in Xo once rather than as nine
  sequences, with the bodies as the reference.
- A V128 the backend fails to keep in a register costs a 16 byte store
  and load, the cost it replaces, so a miss is a performance regression
  rather than a correctness one; each step's `--ab` guards it.

## Alternatives

- More fused kernels (the status quo): fastest per site, but about nine
  hand sequences per shape and nothing for the setup paths.
- Point fixes like `aes_sub_word`: cheap each time, but the stall comes
  back with each new algorithm.
- Decision 0532's loop vectorizer: it vectorizes arithmetic loops and
  has no reason to pick AES or carry-less multiply instructions.
- A user visible vector type now: the larger promise, before the
  internal one has shown its numbers; deferred to step 5.
