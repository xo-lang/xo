# 0075: Native arm64 code placement: no alignment by default, a layout knob for measurement

Status: Accepted (2026-10-08). Extends decisions 0049, 0054, 0060. The "bounds check removal stays off" part superseded by 0184 (2026-10-12).
Implemented in `internal/arm64/align.go` (`Layout`, `ParseLayout`,
`alignLoops`, `innerLoops`), `funcHeader` in `internal/arm64/arm64.go`,
and `$XO_ARM64_LAYOUT` read in `internal/build/arm64.go`. Evidence:
`bench/results/arm64-perf-2026-10-08.md` (section "Code placement").

## Context
Decision 0060 left nbody's slowdown from bounds check removal as a layout
effect: its inner loop's speed moved by 1.5 percent with size and
placement, and the original shape was a lucky point. The question was
whether deliberate placement (functions and loop tops aligned to 16, 32,
or 64 bytes, padding placed where it does not run) makes the backend's
speed independent of luck, so that changes like the bounds check removal
can be judged on their merit.

## Decision
- **No alignment by default.** Functions stay at 4 bytes and loop tops are
  not aligned; the generated assembly is byte for byte the same as before.
  On Apple M4 no setting (function alignment 16, 32, 64; innermost or all
  loop tops aligned to 16, 32, 64, the padding jumped over or run
  through) made every bench program at least as fast. Loop tops aligned
  to 64 cut the placement spread of collatz (2.2 to 0.7 percent of
  cycles over 16 shifts of the code) and nbody (1.1 to 0.2 percent), but
  at a worse point: collatz 1 percent, nbody 0.8 percent, sort 1 percent
  slower than the default placement's median.
- **A layout knob for measurement.** `XO_ARM64_LAYOUT` (comma separated
  `func=N`, `loop=N` as log2 alignments, `fall=1`, `off=K`, `outer=1`,
  `pad=K`) sets `arm64.Options.Layout` for a build. `pad=K` puts K nops
  before every function label (never run) to shift the code, the test
  for placement sensitivity: a code shape change is judged by its median
  over pad 0 to 15, not at the one placement it happens to land on.
- **Bounds check removal stays off** (0054, 0060 unchanged): re-enabled
  from commit 328ff04 and swept over 16 shifts at each of no alignment,
  functions at 64, and loop tops at 64, nbody's median over the shifts
  was 1.4, 1.5, and 1.0 percent slower (with no alignment and with loop
  tops at 64, its fastest shift was slower than the original's slowest),
  so the cost is the loop's own shape, not where it lands. sort: 0.7
  percent slower, 0.3 and 0.2 percent faster.

## Consequences
- Update 2026-10-28: the pad knob is in the Xo emitter as `XO_ARM64_PAD=K`
  (compiler/arm64 gen.xo func_header; 0158 had ported no layout
  instrument). The alignment settings (`func=`, `loop=`, `fall=`, `off=`)
  were not ported: none was a win.
- No change to generated code or speed. `TestLayoutRun` builds the bench
  programs and seven run programs with two layouts and requires the same
  output and zero live allocations, so the knob stays usable.
- Code shape experiments on the native backend should run the pad sweep
  (`XO_ARM64_LAYOUT=pad=K`, K = 0 to 15) for programs whose spread is
  above the noise of identical binaries (collatz, nbody; sort is at its
  noise floor; fib, sieve, interp, strings are dominated by run noise on
  a shared host).
- nbody's default placement is currently one of its slow ones (690M
  cycles at pad 0, 684M at most other shifts); that is luck, not a
  regression of this change.

## Alternatives
- **Loop tops at 64 with an offset** (`off=K`): collatz is best at K = 11
  (1908M, the default's level), but nbody is 688 to 694M at every K
  (default sweep median 684.5M) and sort 870 to 890M (even K near 871M);
  a constant picked for one loop is tuning, not a rule.
- **Functions aligned to 64 only**: nbody 0.8 percent faster at pad 0, sort
  0.4 to 1.3 percent slower. Not a win on every program.
- **Padding run through instead of jumped over** (`fall=1`, measured at
  pad 0 only): nbody 688.5M against the default's 684.5M sweep median,
  collatz 1.7 percent slower than the default.
- Not tried: choosing each function's shift from hardware counters (a
  model of which front end structure the shifts cross), the next
  experiment in the results file.
