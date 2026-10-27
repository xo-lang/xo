# std/big changes

## 2026-10-27: perf: multiplication and division by machine word rows

- Kind: perf
- Commit: this commit
- Found by: bench/perf/big, the std Go twin (mul 3.00x Go)
- Go reference: math/big arith.go addMulVVW, mulAddVWW, subVV; nat.go basicMul, divBasic (go1.26.1)
- Test: internal/stdtest big corpus; internal/llvm big and rsa goldens; bench/perf/big twincheck

The schoolbook product adds each row with bigmod.mac_row (the RSA code's
row, unrolled by 4, in place of a loop over list elements), and long
division subtracts qhat times the divisor as a mac_row product and one
borrow chain. Quiet --ab, 10 rounds, the parent's std as A: mul 0.435
(1.29x Go), quorem wall within noise (instructions 0.848; 1.95x Go).

## 2026-10-27: perf: exp with an odd modulus uses Montgomery multiplication

- Kind: perf
- Commit: this commit
- Found by: bench/perf/big, the std Go twin (exp 12.75x Go: a full division after every product)
- Go reference: math/big nat.expNNMontgomery (go1.26.1)
- Test: internal/stdtest big corpus; bench/perf/big twincheck

`x.exp(y, Some(m))` with m odd and above one word runs in
native/llrt/bigmod's Montgomery domain (its 4 bit window, the RSA code's
exp), as Go does for an odd modulus of more than one word. Quiet --ab, 10
rounds: a 1024 bit exp wall 0.072 against the parent (instructions
0.196), 0.92x Go.

## 2026-10-21: perf: divide and conquer text natively

- Kind: perf
- Commit: abbb97c3
- Found by: decision 0311 stage 4 (text of 3^300000 took 942 ms against Go's 5)
- Go reference: math/big natconv.go itoa, convertWords, divisors, leafSize 8 (go1.26.1)
- Test: internal/stdtest/testdata/corpus/big; internal/llvm/testdata/golden/big_basic

Natively, `text` of a number of more than 8 words splits it by a table of
powers of the base near its square root (recursive division and Karatsuba
squaring), converts the halves, and joins the digits, as Go's itoa does;
smaller numbers keep word division. On 3^300000 (143,137 digits, paired runs of 3 on a busy machine), `text` went from 178 to 191 ms down to 53 to 56 ms (3.4 times); `parse` and `quo_rem` are unchanged.

## 2026-10-21: perf: recursive division natively

- Kind: perf
- Commit: f601176a
- Found by: decision 0311 stage 3
- Go reference: math/big natdiv.go divRecursive, divRecursiveThreshold 40 (go1.26.1)
- Test: internal/stdtest/testdata/corpus/big; internal/llvm/testdata/golden/big_basic

Natively, `quo_rem` (and so `div` and `mod`) with a divisor of 40 words or
more and a quotient of 40 words or more uses Burnikel-Ziegler recursive
division; smaller operands keep Knuth D. On a 3^300000-sized
quotient, quo_rem went from about 270 ms to 35 ms (best two of three paired
runs; the third after run overlapped other load at 161 ms). `text` and `parse`
divide by a word and are unchanged.

## 2026-10-21: perf: parse and text of large numbers natively

- Kind: perf
- Commit: f676659d
- Found by: measurement against the Go backend (3^300000, 143,137 digits: text 942 ms against Go's 5, parse 3,654 ms against 12)
- Go reference: math/big arith.go reciprocalWord and divWW, natconv.go scan's digits per word (go1.26.1)
- Test: internal/stdtest/testdata/corpus/big; internal/llvm/testdata/golden/big_basic

Natively, dividing by a word uses Go's reciprocal (two multiplications
instead of a 128-by-64 division), `text` divides one copy in place, and
`parse` takes 18 digits per multiply-add, in place. On 3^300000 (paired
runs on a busy machine) parse is 18 to 40 times faster and text 3 to 6
times. quo_rem's change, the reciprocal in its quotient estimate, was
within the noise. Go's divide-and-conquer conversion is still to come,
so the Go backend stays faster.

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
