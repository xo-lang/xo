# 0339: GHASH four blocks at a time in NEON (cpu.ghash4)

Status: Accepted (Xo lang 4, 2026-10-22, on Xo Lang 3's request,
lead-approved: ghash64 was 25.9% of a bulk TLS 1.3 server's
instructions after the AES-GCM fusion, 93f258200). Implementation: Xo
lang 4.

## Context

std/native/llrt/aesgcm.xo's ghash64 hashes 64 bytes (four blocks) with
one reduction: twelve 64 by 64 bit carry-less products (cpu.clmul_lo and
cpu.clmul_hi, one pmull each on arm64, the halves moved between general
and vector registers) against H^4 to H^1 (Karatsuba), then a reduction
in general registers. About 141 instructions a 64 byte chunk. Decision
0329's pattern (cpu.* functions with a portable body, lowered on the
native targets) fits: the 128 bit products and their sums stay in
vector registers.

## Decision

`cpu.ghash4(y, hp, d)` in std/native/llrt/cpu.xo: GHASH of the 64 bytes
at d into the state at y (two words, y1 then y0, ghash64's), with the H
powers at hp in Gcm.hk's layout (p0, p1, p2 a block, p2 = p0 ^ p1). The
state is in memory, not a returned pair: an intrinsic returns no tuple.
ghash4_soft is ghash64's body. compiler/ssa cpu_call maps it to
rt.ghash4 on arm64 only; compiler/ssa a64ghash.xo's lines are the
native emitter's (one instruction, its V registers clobbered) and
LLVM's (inline asm) lowering:

- a block loaded and rev64'd is X = (x1, x0); H = (p0, p1) and Hs = (p1,
  p0) by ext; pmull and pmull2 of X by Hs give x1 p1 and x0 p0, of X by H
  the cross products, whose sum is Karatsuba's middle term without its
  adds (so p2 is not read and must be p0 ^ p1);
- blocks 1 to 3 first, then block 0 xored with the state, so the chain
  from one call's state to the next is short;
- the 256 bit product shifted left by one, reduced in two steps of a
  pmull by 0xc200000000000000 (w << 63 ^ w << 62 ^ w << 57 in the low
  word, w >> 1 ^ w >> 2 ^ w >> 7 in the high one), the first started
  before the shift.

amd64 runs the portable body (its clmul_lo and clmul_hi are pclmulqdq)
until it gets a lowering of its own. compiler/macho's assembler encodes
rev64, ext, shl and ushr on .2d, and ins between any two d lanes
(checked against the system assembler).

## Consequences

A dependent chain of 2,000,000 calls (each call's state the next's, as
in GCM), the Mac (instructions, cycles):

| build | ghash4 (NEON) | ghash4_soft |
| --- | ---: | ---: |
| native arm64 | 187 M, 78.3 M | 323 M, 99.8 M |
| LLVM arm64 | 178 M, 81 to 83 M | 226 M, 83 M |

Native: 42% fewer instructions, 22% fewer cycles; LLVM: 21% fewer
instructions, cycles even (its scalar body was already latency bound
the same way). The first version (Karatsuba with p2, the reduction as
ghash64's shifts in vector registers, block 0 first) retired 27% fewer
instructions but took 33% more cycles: a serial chain of about 40 vector
operations of 2 cycles each; the reduction by pmull and the order of
the blocks fixed it.

The golden cpu_prims compares ghash4 with ghash4_soft (and the Go
backend's reference) on 14 inputs. Xo Lang 3 switches ghash_hw and
ctr_at_a's fused loop to it.

## Alternatives

- A twisted H (H x mod the polynomial, as OpenSSL's ghash-v8) would drop
  the shift by one from the chain; it changes Gcm.hk for every backend.
  Not now.
- ghash64 as it is: 141 instructions a chunk.
- Returning (y1, y0): no intrinsic returns a pair today; the state's two
  words in memory cost a store and a load a call.
