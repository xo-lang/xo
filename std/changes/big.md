# std/big changes

## 2026-10-28: perf: bit_len and trailing_zeros read the key; make trims without a copy

- Kind: perf
- Commit: this commit
- Found by: bench/perf/big mul at 1.29x Go; its CPU profile (parts 15% and make 16%: the key unpacked into a list of limbs and packed back at every operation; bit_len unpacked a whole copy to read the top limb)
- Go reference: math/big nat.go bitLen and trailingZeroBits (read the limbs in place) (go1.26.1)
- Test: internal/stdtest/testdata/corpus/big (go, arm64, LLVM); twincheck's checksums agree on none, mul, quorem, text, parse, exp

bit_len and trailing_zeros read the limb count and the limb they need
straight from the key (the count, then the limbs most significant first,
each flipped by its sign's mask), where each copied every limb out first;
make finds the top nonzero limb in place, where norm sliced (copied) the
list when its top word was zero, as a product's often is. Output is
unchanged. Quiet --ab, 15 rounds, the parent's std as A: mul wall 0.906
(0.892 to 0.917), instructions 0.927; quorem instructions 0.939; Xo / Go
for mul about 1.17 (estimate: 1.291 x 0.906).

## 2026-10-27: perf: long division's row runs its two carry chains one after the other

- Kind: perf
- Commit: this commit
- Found by: bench/perf/big quorem at about 1.30x Go after the buffer change; the arm64 code of bigmod.msub_row (inlined into nat_divmod_basic): the multiply chain and the subtract chain interleaved word by word, so the borrow went through a register at every word (cmp, adcs, cset): about 11 instructions a word
- Go reference: math/big arith_arm64.s (mulAddVWW and subVV: one carry chain per loop) (go1.26.1)
- Test: internal/stdtest/testdata/corpus/big (go, arm64, LLVM); internal/llvm and internal/arm64 big_basic; a differential of 3,000 quo_rem calls over 1 to 70 word dividends and 2 to 65 word divisors (top words near all ones and near zero for the add-back case) equal on the Go backend (math/big), arm64 and llvm; twincheck's checksums agree on mul, quorem, text, exp

Each 4-word round of msub_row now forms the four products as one carry
chain (each high half added into the next word's low half, the last high
half plus the carry as the next carry word) and then subtracts them as
another, so each chain stays in the flags: per round 4 loads of pairs, 8
multiplies, 5 adds, 4 complements, one compare, 4 adds with carry, one
carry set and 2 stores of pairs, about 8 instructions a word. Output is
unchanged. Quiet --ab, 15 rounds, the parent's std as A: quorem wall
0.779 (0.770 to 0.792), instructions 0.889, cycles 0.752; Xo / Go about
1.01 (estimate: 1.451 x 0.898 x 0.779), from 1.30.

## 2026-10-27: perf: long division shifts into its buffers and trims in place

- Kind: perf
- Commit: this commit
- Found by: bench/perf/big quorem at 1.45x Go (re-measured on main d088ecee3); its CPU profile (msub_row 58%, and about 20% in the temporaries around it: two nat_shl copies, the copy of u into un, norm's slices, un[0..n], nat_shr)
- Go reference: math/big natdiv.go divBasic and divLarge (shlVU into the divisor and u buffers, the remainder shifted back with shrVU, norm in place) (go1.26.1)
- Test: internal/stdtest/testdata/corpus/big (go, arm64, LLVM); internal/llvm and internal/arm64 big_basic; twincheck's checksums agree on all six bench/perf/big workloads

nat_divmod_basic shifted the divisor and the dividend into new lists,
copied the dividend into its working buffer, and built the remainder by
slicing that buffer (a List slice copies), trimming it and shifting it
into another list; the quotient was trimmed by a slice too. The divisor
and the dividend are now shifted straight into their buffers, the
remainder is written once from the working buffer, and both results are
trimmed by popping their zero top words. Output is unchanged. Quiet --ab,
15 rounds, the parent's std as A: quorem wall 0.898 (0.883 to 0.907),
instructions 0.811; Xo / Go about 1.30 (estimate: 1.451 x 0.898), from
1.451.

## 2026-10-27: perf: decimal text divides by a constant 10

- Kind: perf
- Commit: this commit
- Found by: bench/perf/big text at 1.49x Go; its CPU profile (convert_words 29%: a divide and a remainder by the base in a variable for every digit)
- Go reference: math/big natconv.go convertWords (the base 10 loop "hard-coded for speed") (go1.26.1)
- Test: internal/stdtest/testdata/corpus/big (go, arm64, LLVM); internal/llvm and internal/arm64 big_basic; twincheck bench/perf/big

A word's decimal digits are taken by a division by the constant 10,
which the compiler turns into a multiply, and a subtraction, as Go's
convertWords does; other bases keep the division by the base. Output is
unchanged. Quiet --ab, 15 rounds, the parent's std as A: text wall 0.907 (0.895 to 0.934), instructions
0.972 (the divisions removed were the slow instructions, not the many).

## 2026-10-27: perf: long division subtracts qhat times the divisor in one pass

- Kind: perf
- Commit: this commit
- Found by: bench/perf/big quorem at 1.95x Go; its CPU profile (nat_divmod_basic 84%, a serial carry chain)
- Go reference: math/big arith.go subVV and mulAddVWW as used by nat.go divBasic (go1.26.1)
- Test: internal/stdtest big corpus (go, arm64, llvm); internal/llvm and internal/arm64 big_basic, big_struct and tls_rsa goldens; TestStdAPICovered

nat_divmod_basic cleared a row, filled it with qhat times the divisor
(mac_row) and then ran a second borrow chain to subtract it; it now calls
bigmod.msub_row, which subtracts src times k from dst in one pass and
returns the borrow word: unrolled by 4, the product chain as mac_row's
and the subtraction an add of the product's complement with a not-borrow
carry. Quiet --ab, 15 rounds: the fused row against the parent's std,
quorem wall 0.89 (0.885 to 0.897), instructions 0.699; the unrolled
chains against the fused row, wall 0.857 (0.84 to 0.873), instructions
0.895.

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
