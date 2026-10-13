# Runtime port, step 3: Int and Float text, Float.parse, hex and base64 (2026-10-12)

Decision 0193 step 3. Machine: the shared 12 core Apple M series Mac
(darwin arm64), about ten agents running; every run through the heavy
lock, one at a time. Figures are instructions retired (`/usr/bin/time
-l`, the minimum of 5 runs of each workload, `bench/perf/text/measure.sh`;
the minimum of 3 for the scratch programs below). Native builds used the
Go driver (`XO_SELF_DRIVER=0`); the self hosted emitters produce the same
code (TestSelfhostSSA, TestSelfhostArm64, TestSelfhostLLVM pass on the
tree, below).

## What was measured

`bench/perf/text` gained five workloads (1,000 inputs each):
`int_fmt` (200,000 `"${x}"` of Ints of 1 to 19 digits, both signs),
`float_fmt` (200,000 `"${f}"` of Floats from subnormal to 1e308, short
and 17 digit texts), `float_parse` (200,000 Float.parse of shortest
texts of such Floats, decimal texts with exponents, and `+Inf`),
`base64` (100,000 to_base64 and from_base64 of about 17 bytes), `hex`
(100,000 to_hex and from_hex of the same). "C" is the same compiler with
`XO_LLRT_C=num`: the SSA builder then emits the C runtime's
`str.concat_int` and friends exactly as before the port, and num_c.xo
calls the C Float.parse, hex, and base64. "base" is the compiler at
1ba9946 (before this work); its figures are within 0.1 percent of the C
path's for the five workloads, except base64 on LLVM (412,176,525, 3.4
percent more; raw log).

The setup differs between the paths: it formats about 3,000 Floats, which
the C does with an snprintf loop (`none`: 21,947,201 against 45,741,449
on arm64). So "ratio" (the whole process) favors the Xo path, and the
bar is read on "less setup" (each path's `none` subtracted). The other
twelve workloads use only text.xo and strings.xo on both paths; their
"less setup" ratios are 0.998 to 1.003, which checks the subtraction.

Checksums are identical on both backends, both paths, and equal the Go
backend's (int_fmt 792882063, float_fmt 508981408, float_parse
557081635, base64 199658004, hex 725143115).

## Final (the committed code)

| Workload | arm64 Xo | arm64 C | ratio | less setup | LLVM Xo | LLVM C | ratio | less setup |
|---|---|---|---|---|---|---|---|---|
| `none` | 21,947,201 | 45,741,449 | 0.480 |  | 19,337,490 | 43,350,581 | 0.446 |  |
| `int_fmt` | 72,214,923 | 108,556,842 | 0.665 | 0.800 | 62,235,559 | 101,759,098 | 0.612 | 0.734 |
| `float_fmt` | 140,359,754 | 6,136,464,209 | 0.023 | 0.019 | 105,961,525 | 6,130,214,683 | 0.017 | 0.014 |
| `float_parse` | 139,457,230 | 462,366,092 | 0.302 | 0.282 | 131,338,548 | 452,876,219 | 0.290 | 0.273 |
| `base64` | 162,438,761 | 410,339,560 | 0.396 | 0.385 | 129,456,065 | 398,770,895 | 0.325 | 0.310 |
| `hex` | 101,325,359 | 124,843,123 | 0.812 | 1.003 | 91,533,527 | 119,995,595 | 0.763 | 0.942 |

The bar (1.05) holds on both backends for every workload. `hex` is
from_hex in Xo with to_hex in C (below).

## How it got there

- **Int text** (`int_fmt`, less setup): first version (eight digits a
  word by lane multiplies, `ndigits8` and `digits8` as calls) 1.251x on
  arm64, 1.055x on LLVM; with the common case (-10^7 < v < 10^8) written
  out in one function and the logical immediates below, 1.139x and
  1.088x; then `rs_push_word` and `rs_push_words` make a new string
  directly when appending to the literal `""` (what `"${x}"` lowers to)
  instead of going through `rs_append`: 0.80x and 0.73x. The C path's
  `str.of_int` did the same with one call, so the difference was that.
- **arm64 logical immediates**: `and`, `orr`, `eor` with a constant took
  only a single run of ones; repeating patterns (0x7F007F007F007F,
  0x0F000F000F000F, 0x3030303030303030) were four instruction moves.
  Both emitters now take every positive logical immediate (isel.go
  `onesRun`, selfhost/arm64 `ones_run`).
- **Float text** (`float_fmt`): Dragonbox (Go 1.26's ftoadbox.go) on the
  new word intrinsics; 0.023x of the C on arm64 before `rt.bytes_word`
  (162,511,229), 0.019x after (the 128 bit powers of ten read as two
  words instead of sixteen bytes).
- **Float.parse** (`float_parse`): a scratch program parsing 200,000
  texts per class, arm64 instructions, Xo against C: short decimals
  86,282,175 against 451,516,899; shortest 17 digit texts 157,849,955
  against 531,878,709; subnormals 54,810,989,247 against 585,989,260
  through Go's big decimal (gotchas/subnormal-parse-big-decimal-cost.md),
  then 194,796,208 with subnormals rounded in Eisel-Lemire. The whole
  workload went from 1,318,872,176 (2.85x of the C) to 139,457,230.
- **hex**: with to_hex also in Xo (16 bytes a call, two digits per 16
  bit lane) the workload was 1.516x on arm64 (gather helper calls), then
  1.093x with word loads and an inlinable digit routine (LLVM 0.971x).
  A scratch program of 100,000 to_hex of 17 bytes: Xo 61,934,145 against
  C 53,477,421 on arm64; from_hex 77,524,717 against 77,702,526. So
  to_hex stays C and from_hex is Xo. Batching from_hex into one
  `rs_push_words` call per 64 digits was worse (1.140x) and was taken
  out.
- **base64**: 0.563x on arm64 at first (the C decodes twice, into a
  malloc'd buffer, and checks first), 0.385x with table decoding and
  word loads.

## Correctness

- TestNumDiff (internal/llvm/num_test.go): 336,386 input lines (Float
  texts from random bits of every class in 'g', 'e' with 25 digits, and
  'x' forms; 19 digit texts near 1e-330; exact halfway points between
  neighboring Floats with 800 digits and just above and below; powers of
  ten from 1e-350 to 1e330; edge and malformed texts, hex floats,
  underscores; random Ints; random bytes and damaged base64) through a
  native program on arm64 and LLVM, Xo and C paths: every line equals
  Go's strconv, encoding/hex, encoding/base64 (the C path skips texts
  with underscores, hex floats, or over 4096 bytes, which its Float.parse
  never took). A one off run with `XO_NUMDIFF_N=300000` (about 3.2
  million lines) also passed on the Xo path.
- internal/llvm TestGolden, internal/arm64 TestGoldenRun, TestGoldenAsm,
  TestInlineVerifies, internal/selfhost TestSelfhostSSA,
  TestSelfhostArm64, TestSelfhostLLVM, TestSelfhostWasm: pass on the tree
  at d902255f (to_hex was then Xo; the later change moves it back to
  the C call it was).
- `go test -short` over the repo passes.

## Raw (measure.sh: workload, backend, instructions, best us, checksum)

```
# xo path
none arm64 21947201 16 -1
none llvm 19337490 15 -1
parse_int arm64 45038988 782 559416257
parse_int llvm 43353692 821 559416257
dur_fmt arm64 133065638 3982 575464498
dur_fmt llvm 108296427 3170 575464498
dur_parse arm64 177247684 6398 896627521
dur_parse llvm 145007734 4855 896627521
time_fmt arm64 169136839 6022 826361646
time_fmt llvm 125928066 5124 826361646
time_parse arm64 170119519 5389 10031675
time_parse llvm 120136211 3786 10031675
quote arm64 146240069 4931 879119452
quote llvm 136123369 4685 879119452
split arm64 100980265 4246 133827773
split llvm 85983664 3671 133827773
lines arm64 149050658 6324 514739973
lines llvm 140052623 6088 514739973
replace arm64 83979629 2746 708681165
replace llvm 74462232 2461 708681165
trim arm64 162318053 6230 208194108
trim llvm 153653144 5498 208194108
rune_count arm64 77875115 2477 4824600
rune_count llvm 74842252 2436 4824600
int_fmt arm64 72214923 1745 792882063
int_fmt llvm 62235559 1594 792882063
float_fmt arm64 140359754 4830 508981408
float_fmt llvm 105961525 4151 508981408
float_parse arm64 139457230 6410 557081635
float_parse llvm 131338548 4898 557081635
base64 arm64 162438761 5280 199658004
base64 llvm 129456065 3152 199658004
hex arm64 101325359 2529 725143115
hex llvm 91533527 2329 725143115
# c path
none arm64 45741449 16 -1
none llvm 43350581 14 -1
parse_int arm64 68817243 799 559416257
parse_int llvm 67352105 818 559416257
dur_fmt arm64 156805586 3909 575464498
dur_fmt llvm 132325120 2995 575464498
dur_parse arm64 201018848 6776 896627521
dur_parse llvm 169012424 4477 896627521
time_fmt arm64 192889083 5937 826361646
time_fmt llvm 149922858 5112 826361646
time_parse arm64 193918936 5297 10031675
time_parse llvm 144228364 3971 10031675
quote arm64 170021520 4858 879119452
quote llvm 160035270 4643 879119452
split arm64 124760903 4042 133827773
split llvm 109996710 3659 133827773
lines arm64 172864590 6299 514739973
lines llvm 164063138 6160 514739973
replace arm64 107711016 2633 708681165
replace llvm 98588858 2547 708681165
trim arm64 186079061 6136 208194108
trim llvm 177661371 5716 208194108
rune_count arm64 101529265 2472 4824600
rune_count llvm 98882321 2516 4824600
int_fmt arm64 108556842 2951 792882063
int_fmt llvm 101759098 2726 792882063
float_fmt arm64 6136464209 395622 508981408
float_fmt llvm 6130214683 393694 508981408
float_parse arm64 462366092 17796 557081635
float_parse llvm 452876219 17355 557081635
base64 arm64 410339560 17870 199658004
base64 llvm 398770895 17214 199658004
hex arm64 124843123 2837 725143115
hex llvm 119995595 2715 725143115
# base
none arm64 45757239 16 -1
none llvm 43403762 14 -1
parse_int arm64 68886905 784 559416257
parse_int llvm 67418171 833 559416257
dur_fmt arm64 177628032 5221 575464498
dur_fmt llvm 153269131 4329 575464498
dur_parse arm64 201129966 6419 896627521
dur_parse llvm 169110340 4642 896627521
time_fmt arm64 217091477 6898 826361646
time_fmt llvm 174289585 5977 826361646
time_parse arm64 193938614 5274 10031675
time_parse llvm 145459361 3898 10031675
quote arm64 169759802 4907 879119452
quote llvm 159870503 4644 879119452
split arm64 124773345 4168 133827773
split llvm 110037426 3539 133827773
lines arm64 172891915 6311 514739973
lines llvm 164129117 6073 514739973
replace arm64 107855363 2669 708681165
replace llvm 98668603 2494 708681165
trim arm64 186143712 6124 208194108
trim llvm 177727590 5497 208194108
rune_count arm64 101737243 2479 4824600
rune_count llvm 98923749 2461 4824600
int_fmt arm64 108630768 2922 792882063
int_fmt llvm 101892082 2726 792882063
float_fmt arm64 6136595702 397468 508981408
float_fmt llvm 6130211977 394428 508981408
float_parse arm64 462427929 17732 557081635
float_parse llvm 452915843 17378 557081635
base64 arm64 410393141 17592 199658004
base64 llvm 412176525 17838 199658004
hex arm64 124954599 2840 725143115
hex llvm 120056132 2725 725143115
```
