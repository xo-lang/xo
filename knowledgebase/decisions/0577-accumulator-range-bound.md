# 0577: Induction bounds for loop accumulators (checked adds without a fault)

Status: Proposed (Xo Lang 4, 2026-11-08; the lead asked for it, design
first, prototype behind a flag). Extends decision 0054's value ranges.

## Context

Every Int `+` is checked (spec core 4.4: overflow faults). The native
backends test it after each add (arm64 `adds` + `b.vs`, x86 `add` +
`jo`); LLVM gets `llvm.sadd.with.overflow`. isel's add_sub drops the
test only when g.safe_add proves it from the operands' intervals, and
only for a constant operand (compiler/arm64/opt.xo safe_add,
ranges.xo).

The intervals come from compiler/arm64/ranges.xo (decision 0054): one
interval per value, found by iterating to a fixed point, with a loop
parameter that keeps growing widened to the next threshold, in practice
MAX_INT. So a loop accumulator is never bounded, and every add into it
keeps its test, even where no run can overflow:

    fn byte_sum(b: Bytes) -> Int {
      var s = 0
      for x in b { s += Int(x) }   // s <= 255 * len(b) < 2^40
      s
    }

The test costs a fused branch per add. On adler32's 8-byte loop (std
checksum, 2026-11-08) 16 of the iteration's 18 branches are these
tests, and on the linux/amd64 offload host (AMD Zen 5) the loop runs
at about 9.4 cycles an iteration, close to its branch throughput bound
(the accumulator tie investigation, gotchas/amd64-accumulator-live-out.md).
A byte sum loop is add, test, increment, compare, branch per byte; the
test is a fifth of it.

## Decision

Add an induction bound to ranges.xo: an upper (and lower) bound for a
loop header parameter that only ever grows by a bounded step per trip,
in a loop whose trips are bounded. Then let safe_add prove a checked add
from both operands' intervals (not only a constant one), so the adds
inside such a loop lose their tests.

### The rule

A header parameter s of loop L (header h, its back edges the edges into
h from blocks h dominates) has an induction bound when:

1. Counted trips. L's header has a parameter i whose value on every back
   edge is `i + d` for one constant d > 0, whose entry values are in
   [c, ...], and h ends in a branch that leaves L unless `i < n` (or
   `n > i`), with n's interval [.., N]. Then each trip that reaches a
   back edge started with i < N, so i only takes c, c+d, ..., and the
   back edges are taken at most T = max(0, ceil((N - c) / d)) times
   (saturating). A Bytes or Str length's interval is [0, 2^32 - 1]
   (MAX_STR_LEN, the length word's 32 bits), so `for x in b` gives
   T <= 2^32 - 1.
2. Bounded step. On every back edge, s's value is s itself or a chain of
   adds `s + x1 + ... + xk` (each add with s's chain as one operand),
   where each xj has an interval [loj, hij] from the first fixed point.
   The step's interval is [Σ min(loj, 0) over the chain, Σ hij] taken
   over every back edge (a back edge passing s unchanged steps by 0).
   `Int(x)` of a UInt8 is [0, 255]; adler32's 8-byte step is
   8 x [0, 255].
3. Entry. s's entry values (the edges into h from outside L) are in
   [lo0, hi0].

Then every value s takes is in
[lo0 + T * steplo, hi0 + T * stephi], each product and sum saturating
(MIN_INT / MAX_INT on overflow, which then proves nothing). The adds of
the chain get their intervals from s's by the usual rules, so an add
whose result interval fits in Int needs no test.

### Why it is sound

- The bound only uses intervals that hold for every run: the first fixed
  point's intervals of the xj, n and the entry values are sound over
  approximations (decision 0054). The bound is computed from them after
  the first fixed point, then the fixed point is run again with each
  bounded parameter's interval capped by its bound (a cap is an
  intersection with a fact true of every value s takes, so the second
  fixed point stays sound; the xj intervals of the first run were
  computed with s uncapped, so they over approximate the second run's).
- Trips: s changes only on a back edge, and every back edge carries
  i + d, so the k-th time s is updated i equals at least c + (k - 1) d
  and the branch at h let the trip run only because i < n <= N. Hence
  at most T updates, each by at most stephi.
- The checked add being proven is one of the chain's adds, executed
  once per trip on the way to a back edge or to an exit; its operand is
  a value s took (bounded) plus a partial sum of the step (bounded), so
  its result is in [lo0 + T * steplo, hi0 + T * stephi] as well. An exit
  path that adds after the last back edge adds one more step at most:
  the rule caps with T + 1 trips for the adds, T for s itself.
- Width: with s starting at 0, a byte step (stephi = 255) and
  T < 2^32, s < 255 * 2^32 < 2^40. Int is 64 bits, so overflow needs
  more than (2^63 - 1) / 255 ≈ 3.6 x 10^16 trips, 2^23 times what any
  Bytes length allows; a step of 8 bytes (2040) covers 4.5 x 10^15
  trips. The rule does not assume this: it checks hi0 + (T + 1) *
  stephi <= MAX_INT with saturating arithmetic and proves nothing
  otherwise (a nested loop's outer sum, `t += byte_sum(b)` over 20
  million calls, saturates and keeps its test).
- Faults are unchanged where a test remains. A removed test is one that
  cannot fire, so no program's output changes (the fault stubs and
  their messages for those adds disappear from the binary).

### Passes it touches

- compiler/arm64/ranges.xo: compute_ranges gains the induction step
  (find loops from the dominator tree it already takes, match i, n, the
  step chains, compute the caps, rerun the fixed point with the caps in
  range_set). arm64 and amd64 share it: amd64 is arm64's MIR retargeted
  (decision 0254), so both lose the same tests.
- compiler/arm64/opt.xo safe_add and isel.xo add_sub: prove a checked
  add from both operands' bounds (lo(a) + lo(b) >= MIN_INT and
  hi(a) + hi(b) <= MAX_INT), not only a constant b. Sub symmetric.
- compiler/llvm: unchanged at first; a later stage passes the same fact
  as `add nsw` instead of sadd.with.overflow (the LLVM emitter reads the
  same SSA, not arm64's ranges; it would need the analysis moved to
  compiler/ssa or recomputed there).
- Prototype flag: `XO_ACC_RANGE=1` (classified in
  TestCodegenInputsClassified as a codegen input), off by default until
  measured; the default flip is its own push with the touched packages'
  full suites (owner, 2026-10-22).

### Stages

1. Counted loops with a constant step and a constant-or-interval bound
   (byte_sum, adler32's s1, crc and checksum byte loops), behind the
   flag. Measure: --ab on the amd64 host (Zen 5) and the Intel laptop
   for bench/perf/bytesum and checksum adler32, fnv64a, crc32, and
   instruction counts (callgrind) on arm64.
2. Relational trip bounds: adler32's inner loop runs at most 2^20 bytes
   per chunk (`end = min(i + 2^20, n)`), which an interval of end alone
   does not show (end <= 2^32 - 1); s2 (`s2 += s1`) needs T <= 2^20 to
   fit (s1 < 2^40 would overflow s2 over 2^32 trips). A bound
   `end - start <= K` from the min would let s2 lose its 8 tests too.
3. LLVM: `add nsw` from the same facts.

## Consequences

- Fewer instructions and branches in accumulating loops on arm64 and
  amd64; the expected gain is largest on cores where these loops are
  branch bound (Zen 5's adler32) or front end bound.
- Fewer fault stubs (size down slightly).
- ranges.xo does a second fixed point per function that has a bounded
  parameter (functions without loops pay nothing; with loops, one more
  pass over the blocks per round). Compile time to be measured on the
  self build (--phase-times).
- A fault message for an overflow that can no longer happen is gone;
  no program's observable behavior changes.

## Alternatives

- Widening thresholds from Bytes lengths (add 255 * 2^32 to ups):
  widening would then stop there for any parameter, including ones
  stepping by more than 255, which is unsound without the step check;
  rejected.
- Loop versioning (a fast loop without tests when `len(b) < 2^55 / 255`
  is checked once at entry, the checked loop otherwise): handles
  unknown steps but doubles the loop's code; the interval rule covers
  the common case without code growth. Kept for later if stage 2 is not
  enough.
- Unrolling with partial sums: changes where an overflow faults (the
  fault message's operands), and needs the same proof to be exact;
  later, on top of this.
