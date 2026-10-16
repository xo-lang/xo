# RSA: bigmod's Montgomery arithmetic on adc chains (2026-10-16)

The lead's RSA item after the TLS handshake steps: rsa.xo's signatures
and verifications are bigmod.xo's Montgomery products. The bench's
"RSA-2048" TLS client handshake spends about 3% in them (the client
verifies one RSA signature; the rest is the CA's P-256 check and
X25519), so this is measured where RSA dominates: signing (the
server's RSA keys) and verification on their own.

## Method

bench/perf/rsa (new): PKCS #1 v1.5 with SHA-256 through
internal.crypto_op on fixed test keys of 2048 and 4096 bits made by
Go's crypto/rsa (the signatures must equal Go's, the verifications
pass). `N=50 bench/perf/rsa/measure.sh XO OUT arm64 llvm`: instructions
retired, the best of 5, less the process alone, divided by N, on the
development Mac through the heavy lock.

## Change (std/native/llrt/bigmod.xo)

- A product is now the double width product then Montgomery's
  reduction (separated operand scanning), where it was CIOS with a
  multiply and accumulate (mac) per word that summed full word carries
  with cpu.carry.
- One pass per row, mac_row: dst[j] += src[j] k plus the carry word,
  the low half added with cpu.adc and the next carry word the high half
  plus the two carries (hi + k1 + k2 cannot overflow), unrolled by 4
  with wrapping offsets (a checked `8 * j` and `o + 8` cost an overflow
  branch each in the hottest loop).
- A dedicated square: the cross products f_i f_j (i < j) once, doubled
  with one chain, plus the squares f_i f_i (n (n + 1) / 2 products for n
  n); mul_at takes it when both operands are the same number (the
  exponentiation's 4 squarings per window, modulus setup's R^2).
- The reduction: per word, q = T_i (-1/m) and a mac_row of q m, its
  carry word added at word i + n with a 0 or 1 carry passed on.
- The scratch grows to 2 n + 2 words.

Constant time as before: no branch or index on a value; the
exponentiation's fixed 4 bit windows and masked table reads are
unchanged, and verification's square and multiply follows the public
exponent only.

An intermediate design (four passes per row: the low halves of f g_i,
of q m, then the high halves, each a separate adc chain) was slower on
LLVM (sign 2048 25.9M to 32.0M): twice the loads and stores of the
accumulator and the checked offsets; not kept.

Both designs were checked before use in a Python model of the exact
loops (word positions, carries, and the bounds the code relies on:
hi + k1 + k2 below 2^64, a reduced word 0, the result below 2m) against
big integer arithmetic, limb counts 1 to 9, 16, 17, 32, 33.

## Results (instructions per operation)

| Operation | Backend | Before | After | Change |
|---|---|---|---|---|
| sign, 2048 | arm64 | 50.59M | 32.96M | -35% |
| sign, 2048 | llvm | 25.92M | 17.52M | -32% |
| verify, 2048 | arm64 | 2.15M | 1.14M | -47% |
| verify, 2048 | llvm | 1.13M | 0.63M | -44% |
| sign, 4096 | arm64 | 360.9M | 194.8M | -46% |
| sign, 4096 | llvm | 185.4M | 104.1M | -44% |
| verify, 4096 | arm64 | 8.53M | 3.97M | -53% |
| verify, 4096 | llvm | 4.48M | 2.22M | -50% |

(arm64 with the adc pairs one template each, main bfb2c4f6: the chains
come back once the emitter serves longer templates.)

## Checks

TestRSADiff (internal/llvm, new): keys of 1024, 2048, 2056 (a limb count
not a multiple of 4), 3072, and 4096 bits with SHA-256, SHA-384, and
SHA-512: PKCS #1 v1.5 signatures equal to crypto/rsa's, Go's PKCS #1 v1.5
and PSS signatures verified and a changed hash refused, PSS signatures
crypto/rsa verifies, on arm64 and LLVM. The crypto and TLS goldens
(tls_rsa, tls_x509, tls_verify, http_tls_keys and the rest) and the TLS
and X.509 diff tests pass on both backends (http_tls on arm64 waited
at _dyld_start until the owner's macOS fix that morning, then passed);
the self rebuild fixed point holds.
