# std/big changes

## 2026-10-21: port: std/big, Go's math/big Int

- Kind: port
- Commit: (filled in after the push)
- Found by: survey (gap 10: public RSA and ECDSA keys, big integers; decision 0311)
- Go reference: math/big int.go, nat.go, natdiv.go, natconv.go (scan), prime.go (ProbablyPrime, probablyPrimeLucas); math/bits Div64 (go1.26.1)
- Test: internal/stdtest/testdata/corpus/big; internal/llvm/testdata/golden/big_basic

New module: `big.BigInt`, an immutable integer of any size. It has
arithmetic with Go's truncated and Euclidean division, exp, gcd,
mod_inverse, sqrt, shifts and bitwise operations on two's complement,
probably_prime, and Go's SetString and Text in bases 2 to 62. Its order
and equality are the numbers', so it sorts and keys maps.
