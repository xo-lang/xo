# 0194: Division by a constant as a multiply high on arm64

Status: Accepted (2026-10-12). Implemented in internal/arm64 (isel.go
`divConst`, `sdivQuot`, `sdivMagic`, `negConst`; opt.go `memo.quo`) and
its mirror selfhost/arm64 (isel.xo `div_const`, `sdiv_quot`, util.xo
`sdiv_magic`, state.xo `neg_const`, `Memo.quo`), with std/native/llrt
text.xo's digit loop. Tests: `TestSdivMagic`, golden `const_div`.
Evidence: `bench/results/const-div-2026-10-12.md`.

## Context

Decision 0193 left Duration text (bench/perf/text `dur_fmt`) at 1.16x
of the C path on arm64 and 1.12x on LLVM in instructions retired,
against decision 0180's bar of 1.05x, and ranked three next steps: (a)
the arm64 backend lowering a division by a constant to a multiply high,
(b) shift checks the ranges prove, (c) fewer digit loop trips. The arm64
backend lowered only positive powers of two specially (shifts with a
sign fixup); every other `x / c` and `x % c` was `sdiv` (and `msub`),
while LLVM (and Go, and clang for the C path) use Granlund and
Montgomery's multiply high. Xo's semantics constrain the lowering: a
division by a constant cannot fault except by the constant 0 (a
diagnostic or a divide by zero fault, unchanged) and `MinInt / -1`
(the overflow fault, unchanged); `MinInt % -1` is 0.

## Decision

1. **Signed division and remainder by an Int constant d** other than 0,
   1, -1, the minimum, and the positive powers of two (which keep their
   own lowering) are lowered in both arm64 emitters, with identical
   output:
   - the quotient by |d|: for a power of two, the rounding add and a
     shift; else `smulh` by the magic M (`sdivMagic`, Hacker's Delight
     10-1: the least p >= 64 with (M d - 2^p) anc < 2^p, anc the
     largest Int with anc mod d = d - 1; computed in Ints below 2^63 so
     the Xo mirror computes the same), `add` of x when M is negative as
     an Int, `asr` by p - 64, and `add q, q, q, lsr #63` unless the
     value ranges prove x not negative;
   - `x / d` is that quotient, negated for d < 0; `x % d` (whose sign is
     x's) is `msub` of the quotient and |d| from x;
   - the quotient is memoized by (dividend register, |d|) per block and
     inherited by dominated blocks, so `x % c` and `x / c` multiply once
     (a division that may fault ends its SSA block, so the two are
     usually in successive blocks).
   0, 1, -1, and the minimum keep the `sdiv` path and its checks; so a
   constant -1 divisor still faults on the minimum.
2. **A negated Int constant is a constant** in the arm64 emitters
   (`negConst`): the SSA has `-7` as `OpNeg` of 7, so `OpNeg` of a
   constant other than the minimum becomes the constant (no overflow
   check), and a negative literal divisor takes item 1.
3. **Unsigned division** has no lowering to do: the native backends
   lower sized integer arithmetic (UInt8 to UInt64) to C calls (decision
   0193 item 1 on crypto), so only Int (`ssa.KInt`) division
   reaches `sdiv`.
4. **text.xo's digit loop** (`digits_word`) masks the dividend (`v &
   0x7FFFFFFFFFFFFFFF`) and the word before its shift (`w &
   0x7FFFFFFFFFFFFF`), which change nothing for its inputs but let the
   ranges prove the quotient's sign fixup and the shift check away
   (decision 0193's step (b), done in the source because the ranges do
   not follow loop counts).
5. **Duration text stays on its C path** (`XO_LLRT_C=text`): it is
   1.171x of the C on arm64 and 1.087x on LLVM (results file, "Final").

## Consequences

- Every Xo division by a constant on arm64 is shorter in latency and
  longer in instructions: `sdiv` is one instruction, the multiply high
  two to four plus a four instruction 64 bit constant where it is not
  hoisted from a loop. bench/perf/text: time_fmt 8639 to 6976 us and
  dur_fmt 5440 to 5155 us (best of 5, noisy), but dur_fmt 149,470,312
  to 152,995,555 instructions; with (b) and without (a) it would be
  145,820,545 (1.130x). The bar of decision 0180 counts instructions,
  so this decision moves arm64 away from it on division heavy code;
  that trade is accepted (LLVM, Go, and clang make it too) and recorded
  in gotchas/arm64-magic-division-retires-more.md.
- LLVM: no backend change; text.xo's masks take dur_fmt from 1.117x to
  1.087x and time_fmt from 0.184x to 0.173x.
- The other bench/perf/text workloads keep the bar (time_parse 1.037x
  on arm64, trim 1.008x and 1.012x).
- Assembly changes for every program with a constant divisor or a
  negative literal; both emitters agree (TestSelfhostArm64).
- Next, ranked by likelihood of reaching the bar: (1) materialize wide
  constants with a logical immediate (`orr` of a repeating pattern such
  as 0x6666666666666666, then `movk` of the low chunk: 0x6666666666666667
  for 10 and 0x8888888888888889 for 60 in two instructions instead of
  four), in internal/arm64 `movImm` and selfhost/arm64 `mov_imm`, then
  remeasure; (2) ranges of call results (`ndigits` returns 1 to 8), so
  the checked variable shifts of `cat` and `frac_words` (about ten
  instructions each, several per Duration) go away; (3) the two digits
  a trip loop (best on LLVM, 1.063x) once (1) makes its constants
  cheap; (4) fold negative literals in the SSA builders (both, with
  their goldens).

## Alternatives

- **Keep `sdiv`** (the instruction count is lower): rejected for the
  latency, and because every other compiler of the C path makes the
  same choice; the bar's metric does not see it.
- **Use the multiply high only in loops** (where the constant is
  hoisted): it still retires more instructions than `sdiv` there, so it
  would not reach the bar either, and it would make the lowering depend
  on loop shape.
- **Hacker's Delight's condition with 2^63 for anc** (simpler): gave a
  longer form for some divisors (3 needed the add and a shift); the
  exact anc is cheap to compute.
- **Step (c) as fixed steps of 8, 4, 2, 1 trailing zeros**, **(b) with a
  masked variable shift per digit**: measured worse (results file).
