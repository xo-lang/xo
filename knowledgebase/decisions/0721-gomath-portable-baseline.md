# 0721: Go's portable math is the perf baseline where package math uses FMA assembly

Status: Accepted (owner, 2026-11-19, owner question #173 option 1; task #406)

## Context

Decision 0461 makes a std module Proven on performance when every row of
its bench twins runs at or under 1.05 of Go. Decision 0287 has Xo's math
follow Go's portable algorithms (exp.go, erf.go, pow.go, ...) with every
operation rounded and no fused multiply-add, so a float result is the
same on every backend and architecture.

Go's package math does not run that code for Exp: exp_amd64.go and
exp_arm64.s are assembly that use FMA (useFMA on amd64), and Erf, Erfc
and Pow call Exp. On arm64 Go's compiler also fuses `x*y+z` in the pure
Go functions. On the linux/amd64 host (task #406, 2026-11-19, ms per
10 runs, best of 7) Xo's LLVM build was at or under Go's portable copy
(std/rt/gomath_gen.go, the code Xo matches) for these four, and 1.07 to
1.27 over package math:

| work | Xo LLVM | Go package math | Go portable |
|---|---|---|---|
| erf | 479 | 375 | 503 |
| erfc | 638 | 676 | 666 |
| exp | 165 | 151 | 220 |
| pow | 821 | 705 | 876 |

Closing the gap would take an Xo exp written with explicit fma, which
changes Xo's results (0287's identical floats would hold, the values
move), or giving up 0287's no-fusion rule. Neither is wanted.

With the twin below (linux/amd64 host, 2026-11-19, stdproven perf, ms):
erf 46.3 Xo, 47.7 portable, 36.2 package math; erfc 54.3, 55.9, 40.6;
exp 15.9, 21.6, 14.7; pow 90.6, 79.4, 69.3 (bench/perf/gomath/results.md).

## Decision

For erf, erfc, exp and pow, the performance baseline in bench/perf/gomath
is Go's portable code: the twin runs the copy from std/rt/gomath_gen.go
(Go 1.26's math without assembly, each product wrapped in `float64()` so
it does not fuse), with package math's Sqrt and Trunc, which are exact
operations on every machine. Package math's time is kept beside each of
these rows: a twin.txt case `- - W -- W pkg` runs package math's W, and
`stdproven perf` prints its Go time in a "Go package math ms" column of
the row W instead of judging it (perfRef).

This applies only to functions where Go's package math runs assembly
whose results Xo cannot produce under 0287. Every other gomath row
(gamma, lgamma, remainder, acosh, ...) is still judged against package
math. Decision 0287 is unchanged.

## Consequences

- The rows measure the algorithm Xo implements, not FMA assembly. The
  reference column still shows how far Xo is from what a Go user gets.
- A function joins this list only by an amendment here, with its Go
  assembly named and a timing table like the one above.
- If Go's package math drops the assembly, or 0287 is revisited to allow
  fma, the twin goes back to package math for that row.

## Alternatives

- Judge against package math: the four rows stay over 1.05 for as long
  as 0287 holds, so prelude is never Proven on performance.
- An fma exp in Xo (Go's exp_arm64.s): matches Go arm64 bit for bit but
  moves Xo's results on every backend and target; rejected (lead,
  2026-11-19).
- Drop the four rows from the perf set: hides what a Go user would see.

## Amendment 2026-11-19: random's zipf (owner, xw ask #197, xw decision #198)

random.Zipf.draw is Go's Zipf.Uint64 (math/rand/v2 zipf.go) line for
line; its time is in math.Exp and math.Log (XO_PROF on the linux/amd64
host: exp 52%, log 18% of 30M draws). On amd64 Go's package math runs
assembly for both (exp_amd64.s with VFMADD213SD under useFMA, and
log_amd64.s; haveArchExp and haveArchLog are true in exp_asm.go and
log_asm.go), whose results Xo cannot produce under 0287.

bench/perf/random's zipf row therefore runs Go's Zipf with Go's portable
Exp and Log (gmExp, gmLog: the copy bench/perf/gomath/twin.go.txt holds
from std/rt/gomath_gen.go), with package math's rand.Zipf beside it as
`- - zipf -- zipf pkg`. linux/amd64 host, stdproven perf -rounds 9, 2026-11-19 (on main
1cc8f1e2b; std/evidence/random/perf.md):

| work | Go portable ms | Xo ms | Xo / Go | Go package math ms |
|---|---|---|---|---|
| zipf | 36.4 | 33.3 | 0.913 | 26.9 |

This is the first row outside bench/perf/gomath on this list: a
module whose Go twin spends its time in package math's assembly joins
it the same way, by an amendment naming the assembly with a timing table.
