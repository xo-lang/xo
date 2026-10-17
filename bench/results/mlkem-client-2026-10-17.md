# ML-KEM's cost in the TLS client, arm64 and LLVM (2026-10-17, Xo Lang 2)

Question (the lead's queue): ML-KEM was about 216.7M of the x25519 hybrid
handshake's client instructions (bench/perf/tls, N=200; xo-fa's
keccak-spills-2026-10-17.md), mostly NTT, inverse NTT and sampling. Where
does it go on the arm64 backend against LLVM's output, and what in std
removes it?

## Method

- Handshake: `WORKLOADS="x25519 x25519only" bench/perf/tls/measure.sh`,
  N=200, client instructions retired (best of 5), on the Mac (arm64).
  ML-KEM's part is x25519 (X25519MLKEM768 and X25519 offered) minus
  x25519only (no ML-KEM share). The benchmark's Go server
  (bench/perf/tls/serve) prefers X25519, so the client builds the hybrid
  share and never decapsulates: the measured part is key generation and
  the share.
- Encapsulation and decapsulation: C2SP CCTV's accumulated ML-KEM-768
  test through `internal.crypto_op("mlkem.accumulated", "200")` (200
  times: a key, an encapsulation, two decapsulations, one of them the
  implicit rejection), instructions retired. The digest is unchanged
  (2d1724bc...).
- Per part: a scratch op (not committed) timing expand, sample_ntt,
  sample_cbd, the NTTs and ntt_mul_acc per call, both backends.

## Where it went (before)

Per call, instructions (arm64 / LLVM): expand (ML-KEM-768 key generation
from a seed) 604,657 / 536,552; sample_ntt 27,342 / 23,757 (nine per
key); sample_cbd 17,180 / 14,041 (six); NTT 22,134 / 21,235 (six); inverse
NTT 24,200 / 22,546; ntt_mul_acc 9,202 / 8,296 (nine); a Keccak
permutation about 5,300 / 4,700. The two backends were within 15% of each
other everywhere, so the cost was in the algorithm and the std code, not
in arm64 code generation.

And the handshake's part was more than ML-KEM: the client computed its
X25519 public key twice (once inside the hybrid share, once for the plain
X25519 share beside it, the same key).

## Changes (std/native/llrt mlkem.xo, sha3.xo, tls13.xo)

1. The plain X25519 share is the hybrid share's own ECDH public key
   (mlkem.hybrid_ecdh_share): one scalar multiplication, not two.
2. Lazy reduction in the NTT, the inverse NTT and ntt_mul_acc: a Barrett
   step for x < 2^32 (constant floor(2^32/q) = 1290167, result in [0, 2q))
   on products, sums left unreduced between layers (bounds in the
   comments: below 15q forward, below 128q inverse), the final reduction
   folded into the last layer, the inverse's 128^-1 scaling folded into
   its last layer; byte offsets and raw reads of ZETAS and GAMMAS.
3. H(ek) out of the key expansion (nine permutations): the decapsulation,
   its only user, computes it.
4. sample_ntt reads each SHAKE128 block as the sponge's lanes
   (sha3.next_block, no Bytes copy) and takes 16 candidates per three
   words with a branchless store (the count advances past accepted
   candidates only; the list has 16 spare slots, dropped at the end).
5. sample_cbd: pair sums of 64 bits at a time, 16 coefficients a word.
6. Lists of zeros (polynomials, sample_ntt's, the sponge's 25 lanes)
   copy a constant instead of pushing.
7. sha3.write absorbs whole lanes when aligned (short inputs were a byte
   at a time).
8. The TLS client keeps the hybrid key's expanded ML-KEM key (Client.kem,
   mlkem.hybrid_key_for) for its share and its decapsulation, as Go keeps
   a DecapsulationKey768: when the server picks the hybrid, the seed is
   expanded once instead of twice. The benchmark's server picks X25519,
   so this does not show in the handshake numbers below.

## Results

Handshake, ML-KEM's part (x25519 minus x25519only, 200 handshakes):

| backend | main 11d3eb85 | this change | change |
|---|---|---|---|
| arm64 | 217.1M | 83.3M | -62% |
| LLVM | 180.7M | 73.8M | -59% |

(x25519: arm64 1,694,244,633 to 1,562,938,986; LLVM 1,381,919,512 to
1,276,270,672. x25519only unchanged within noise: arm64 1,477,138,075 and
1,479,603,325, LLVM 1,201,253,002 and 1,202,497,453.)

Accumulated test (200 iterations, encapsulation and decapsulation
included):

| backend | main | this change | change |
|---|---|---|---|
| arm64 | 859,055,504 | 621,719,061 | -28% |
| LLVM | 755,833,221 | 533,515,053 | -29% |

Per call after (arm64 / LLVM): expand 377,949 / 325,099; sample_ntt
18,061 / 15,754; sample_cbd 10,672 / 7,573; NTT 15,976 / 14,448; inverse
NTT 16,160 / 15,333; ntt_mul_acc (with its zero list) 5,526 / 4,951.

## What is left

- Keccak: about 35% of an expansion (34 permutations: 27 for A, 6 for the
  CBD samples, 1 for G) and 32% of the accumulated test. The arm64 loop
  is 178 instructions a round, 44 of them stack traffic (25 lanes and the
  temporaries against about 26 registers); hand-scheduled scalar Keccak
  is about 125. That is register allocation: sent to Xo lang 4 (arm64
  isel), with xo-fa's ideas (x30 in a call-free loop, spilling lanes read
  once a round). LLVM's permutation is 4,700, so it has room too.
- The remaining SHAKE overhead per call (sponge setup, finish, reads of
  ciphertext-sized inputs) and ntt_mul_acc's per-call reductions (an
  unreduced sum over k products, reduced once, would save about 2%).
- encode_d and decode_d push a byte at a time (about 5% of the
  accumulated test).
