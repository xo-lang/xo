# Loop carried Float pairs (decision 0323 stage 3): nbody (2026-11-04)

Xo Lang 2, the lead's item: nbody was the one row of the 0.38.1 release
comparison slower than Go (1.071, bench/results/release-vs-go-v0.38.1.md).

## Cause

Xo ran 0.867 of Go's instructions and 1.036 of its wall (Mac, 15 rounds),
so the loop waited, not worked. Not fused multiply-add: a variant with
`x.fma(y, z)` wherever Go fuses (Go fuses on arm64, Xo only when asked,
decision 0385) ran 1.033 of Go. Not the pairing: with slp_f64 off nbody
ran 1.066 of the paired build. Every j iteration rebuilt body i's
(vx, vy) pair from two scalars (zip1) and split it again (two dups):
decision 0323's stage 3, never built.

## Stage 3 as first built: worse

The pair carried across the j loop as one wide register (no zip1 or dup
in the loop), split after it, built before it: 0.971 of the
instructions but slower on every one of 16 code placements
(XO_ARM64_PAD=0..15): Mac 1.013 to 1.047, host 1 1.019 to 1.033 of the
base. nbody's j loop runs 0 to 4 times an entry; a 40 body variant (long
j loops) ran 0.928 to 0.936 of the base at pads 0 to 3 (Mac), so the cost
was at the loop's entry and exit.

## With the entry and exit as one q load and one q store

The preheader loads the pair with `ldr q` where it loaded the two words
next to each other; the exit stores it with `str q` where it stored them
next to each other (slp_carry_load, slp_carry_store).

Mac, pads 0 to 3, 7 rounds: 5 bodies 1.007, 0.979, 0.983, 0.979 of the
base; 40 bodies 0.922 to 0.950.

Host 1 (linux/arm64), 16 pads, 7 rounds each, the same xo with and
without slp_carry (XO_PASSES=-slp_carry): better at every pad, 0.965 to
0.993 of the base (median about 0.987). Against Go's nbody (Go 1.26.1),
pads 0, 5, 10, 15: Xo with slp_carry 0.983, 0.987, 0.988, 1.012 of Go's
wall; without it 1.017 to 1.028.

## The pushed build

The final build (rounds of one carry each, edge splits for exits
entered from outside the loop, the q load across cold fault blocks)
against its base on host 1, ab-host.sh, bench/perf/llvm/xo/nbody:
0.986 of the base's wall (0.98 to 0.992, better, stopped after 6 of 10
rounds), peak RSS the same. Correctness: progen's alias (200 programs)
and collections (100) differentials on linux/arm64, every output the
Go backend's; test/codegen (arm64 and LLVM); the arm64 goldens.


## The pass's own cost

The first pushed candidate made xo's self build slower on the
linux/arm64 offload host 2 (ab-host.sh self: wall 1.037, 1.034 to
1.045, worse): slp_carry_one built four maps over every instruction of
every loop. A 150 Hz XO_PROF diff of the self build put
arm64.G.slp_carry_one at +8.8 s cumulative and map.put[Map[Int, Int]]
at +2.6 s. It now skips a loop with no `zip1 $vd0.2d` (the only pack
it carries) before building them. Same host, 5 rounds: wall 1.001
(0.98 to 1.012), CPU 1.005 (0.983 to 1.015), peak RSS 1.0, all within
noise. The generated code is unchanged (test/codegen 32/32, the slp
goldens).
