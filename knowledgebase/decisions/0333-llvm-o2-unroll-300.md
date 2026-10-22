# 0333: LLVM -O2 builds unroll at threshold 300

Status: Superseded by 0338 (2026-10-22: the loops with Float arithmetic are marked for full unrolling instead of a module wide threshold). Was: Accepted (owner, 2026-10-21). Implementation: Xo lang 4.

## Context

The LLVM backend builds at -O2 by default (decision 0220; `--release`,
seeds and releases too). nbody runs twice as fast at -O3, but the owner
kept -O2: at -O3 strings slowed 13% (since fixed, d3d212e7a) and the
TLS program's text grew 6.9%. The follow-up measurement
(bench/results/llvm-o2-unroll-2026-10-21.md, 163bab1e8) found that
nbody's gain is one LLVM threshold: the loop unroll threshold, 150 at
-O2 and 300 at -O3. With `-mllvm -unroll-threshold=300` at -O2, on the
Mac (instructions, least of 3, output identical):

| workload | -O2 | -O2 unroll 300 |
| --- | ---: | ---: |
| nbody | 2,308 M | 1,160 M (-49.7%) |
| collatz, fib, interp, sieve, sort, strings | | within 0.3% |
| TLS program text | 1,578,368 | 1,603,372 (+1.6%) |

nbody's gain is a step between thresholds 200 and 250 (its inner loop
unrolls or not). Thresholds 400 to 600 slow sieve 2.1 to 2.5% and grow
the TLS text 2.6 to 3.4%.

## Decision

An LLVM build at -O2 passes `-mllvm -unroll-threshold=300` to clang's
compile, and to an LTO link (which optimizes again): `-Wl,-mllvm,...`
to darwin's ld64 and `-Wl,-plugin-opt=...` to a Linux linker, because
neither Apple's clang nor Debian's forwards `-mllvm` to the linker's
LTO (checked with `clang -###`, 2026-10-21). A build without LTO
(`XO_LLVM_LTO=none`) gets the compile flag only: GNU ld refuses the
plugin option without LTO. -O0, -O1 and -O3 are unchanged. The flags are
link flags of the build (compiler/xo split.xo `unroll_flags`), so the
binary cache keys on them and a split build compiles each module with
them. There is no knob: `XO_LLVM_OPT=O3` remains the way to get every
-O3 setting.

## Consequences

- nbody at -O3's speed without -O3's text growth; the TLS program's
  text +1.6%.
- Every -O2 LLVM binary changes, xo itself (seeds and releases are -O2)
  included; the -O2 LLVM goldens sample (internal/llvm o2Goldens) runs
  with it. Tests at -O1 and -O0 are unchanged.
- Measured with the change (default -O2, single module, the gate's xo,
  2026-10-21): nbody 1,165 M, strings 945 M, sieve 1,998 M, sort
  1,316 M, collatz 3,918 M, fib 3,823 M, interp 1,581 M, outputs
  identical. `XO_LLVM_LTO=none`: nbody 1,200 M (the compile flag alone).
- A split build (`XO_LLVM_SPLIT=4`, ThinLTO) gets no nbody gain with or
  without the flag (2,117 M against 2,115 M), and none at -O3 either
  (2,116 M): its loss is the split's, not this decision's; open
  follow-up.
- 250 would give nbody 7% more but sits next to the step at 200, where a
  slightly larger loop body loses the whole gain; 300 is -O3's own value
  and leaves margin.

## Alternatives

- -O3 by default: TLS text +6.9% (rejected by the owner earlier the same
  day).
- Inline thresholds (500, 1,000): nothing for nbody, text +22% to +58%.
- A higher unroll threshold (400 to 2,400): sieve slower, text +2.6% and
  up.
- An `XO_LLVM_UNROLL` knob: not needed for one tested value; the clang
  wrapper (`XO_CLANG`) still allows experiments.
