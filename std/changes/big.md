# std/big changes

## 2026-10-21: perf: Karatsuba multiplication natively

- Kind: perf
- Commit: e2bed9a0
- Found by: decision 0311 stage 2
- Go reference: math/big nat.go karatsuba, karatsubaThreshold 40 (go1.26.1)
- Test: internal/stdtest/testdata/corpus/big (now with 2,600 to 8,000 bit operands)

Natively, multiplying numbers of 40 words or more uses Karatsuba: 3^300000,
and its product with that plus one, from 0.31 s to 0.06 s on arm64 (median
of 5, the same output). Squaring in `exp` and every `mul` use it.

## 2026-10-21: port: std/big, Go's math/big Int

- Kind: port
- Commit: 432d314e
- Found by: survey (gap 10: public RSA and ECDSA keys, big integers; decision 0311)
- Go reference: math/big int.go, nat.go, natdiv.go, natconv.go (scan), prime.go (ProbablyPrime, probablyPrimeLucas); math/bits Div64 (go1.26.1)
- Test: internal/stdtest/testdata/corpus/big; internal/llvm/testdata/golden/big_basic

New module: `big.BigInt`, an immutable integer of any size. It has
arithmetic with Go's truncated and Euclidean division, exp, gcd,
mod_inverse, sqrt, shifts and bitwise operations on two's complement,
probably_prime, and Go's SetString and Text in bases 2 to 62. Its order
and equality are the numbers', so it sorts and keys maps.
