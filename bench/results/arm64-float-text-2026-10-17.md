# arm64 float workloads against LLVM and Rust (2026-10-17, Xo lang 3)

The lead asked for arm64 floating point performance, nbody first (2.56x
Rust's instructions on the Mac in rust-baseline-2026-10-16.md). Mac
M-series, `/usr/bin/time -l` cycles; Linux instruction counts and
branch mispredicts from valgrind 3.24 cachegrind (`--branch-sim=yes`)
in scripts/docker-linux.sh's arm64 image.

## nbody: the gap to Rust is the benchmark's constant input

| Build | cycles | instructions |
|---|---|---|
| Rust, as in the baseline | 456M | 1,409M |
| Rust, `std::hint::black_box` on the bodies and the list | 695M | 2,410M |
| Xo arm64 (main c9758ac0) | 690M | 3,609M |
| Xo LLVM | 843M | 2,700M |

Rust's LTO inlines `bodies()` into `main`, sees five bodies, unrolls
the ten pairs, keeps them in registers, and pairs the work into two lane
SIMD (`fsqrt.2d`, `fdiv.2d`). With the list opaque it runs as fast as
Xo's arm64 build. The loop is latency bound: hoisting `bs[i]`'s loads
and keeping its velocity in registers by hand cut 7 percent of the
instructions and no cycles (gotchas/arm64-fewer-instructions-slower.md).
Fused multiply add is out: LLVM does not contract either, and fusing
changes results between backends.

## Float text: what was slow

The profiler (XO_PROF=cpu, XO_PROF_HZ) and cachegrind per line put the
arm64 build's extra cycles in num.xo's Dragonbox formatter and float
parser, and in branch mispredicts more than in instruction count (json
floats: 3.47M simulated mispredicts against LLVM's 2.49M; instructions
within 11 percent):

- `x % C == 0` by a constant as a quotient (smulh, msub) instead of a
  multiply by the inverse, a rotate, and a compare;
- `if v < 10 { 1 } else { 2 }` (ndigits8) and `if (j & 1) != 0 { p = p
  * x }` (exact_pow10) as unpredictable branches instead of csel, csinc,
  fcsel;
- `c >= 48 && c <= 57` as two compares and branches, and as a Bool
  value (jsontext.digit) through a branch;
- `mant * 10 + digit` with wrapping operations as mul and add (madd).

## Changes (selfhost/arm64)

- select_branch (if-conversion): a branch whose two sides reach one
  block, directly or through a block of constants or of up to four
  instructions that cannot fault (spec_ok: Float add, subtract,
  multiply, negate; logic; unchecked or wrapping Int arithmetic; Int to
  Float), selects the differing Int, Bool, or Float parameters with
  csel, csinc (constants one apart), xzr (zero), or fcsel. Not when the
  condition comes from memory (from_memory) or the join only branches
  on the selected Bool.
- range_branch, range_and: `c >= lo && c <= hi` (as a branch) is a
  subtract and one unsigned compare; logic_value: `a && b` and `a || b`
  as a Bool value without branches (the range form, or cset of each
  and and/orr), not when a test divides.
- fused_add, fused_sub: a wrapping add or subtract of a wrapping
  multiply is madd or msub, also through an inlined helper's result.

Blocks the conversions leave unreachable are not selected (G.spec,
gotchas/arm64-skipped-block-memo.md).

Tried and dropped: `x % C == 0` as a multiply by the inverse, a rotate,
and a compare (Hacker's Delight 10-17, as LLVM). It takes three wide
constants per test where the quotient form shares one magic number
(std time's is_leap tests 100 and 400 with one), so it cost instructions
everywhere (float_fmt +3 percent) for at most 1 percent of cycles, and
nothing once the selects were in.

## What the first version got wrong (gotchas/arm64-if-conversion-cost.md)

The sweep (bench/perf/sweep.py, instructions) found 45 of 114 workloads
slower; in cycles text zone_offset was 1.17x, json enc_strings 1.09x,
zone_civil 1.04x:

- speculating the compare of `a && b` ran it every time for a Bool that
  was then branched on anyway (enc_strings);
- `y % 4 == 0 && (y % 100 != 0 || y % 400 == 0)` evaluated every test
  (is_leap);
- a binary search's `if sec < w[m] { hi = m } else { lo = m + 1 }` as
  csel made each load wait for the one before (zone_offset).

## Results

Cycles, median of 15 interleaved runs on the Mac, the workload less its
input setup (`none`), against main c9758ac0 (the machine's load average
13 to 15; repeated runs move by up to 3 percent):

| Workload | main | new | ratio |
|---|---|---|---|
| json floats | 70.81M | 68.33M | 0.965 |
| json enc_floats | 21.20M | 20.81M | 0.982 |
| text float_parse | 25.85M | 25.52M | 0.987 |
| text float_fmt | 20.16M | 20.09M | 0.997 |
| text lines | 28.40M | 26.72M | 0.941 |
| fault owned | 88.66M | 87.00M | 0.981 |
| fault defer | 136.72M | 137.08M | 1.003 |
| portable checked | 79.74M | 79.76M | 1.000 |

The sweep (instructions retired, 3 rounds): 9 workloads faster (json
ints 0.981, floats 0.988, float_parse 0.976, tls chacha 0.968, utf8
0.988), 10 slower by instructions (portable checked 1.035, the fault
programs 1.008, float_fmt 1.012, lines 1.011; httpparse h1 1.03 to
1.05 inside a spread of 1.01 to 1.04), none of which is slower in
cycles where measured above; httpparse in two more rounds of its own
script: h1_get and h1_chunked mixed, h1_url 1.02. The LLVM build is
still ahead on float text (0.71 to 0.82 of main's arm64 cycles): its
whole program inlining puts append_float into the caller's loop and
keeps the constants in registers.
