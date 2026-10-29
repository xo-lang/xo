# Profile-guided layout, stage 2b: the dominator cold split on longer runs (2026-10-29)

Decision 0403. Apple M4 Pro, xo at main 012a77e4d's parent plus the split
(xo-lang-6's worktree xl70; the diff is in the lead's notes,
xo-lang-6/0403-stage2b-split.diff), bench/perf/llvm/xo collatz (as is),
strings at 100 rounds and interp at 60M steps (copies with the one constant
raised, so a run is 370 to 740 ms and a 10 kHz profile has hundreds of
samples). The split's criterion, after stage 2's version cost interp 14
percent: a block is hot when it has a sampled line or dominates or
post-dominates a sampled block (a loop's header for a sampled body, the only
successor of a sampled block, the merge after a sampled branch); the others
go after the hot ones, the entry first. No alignment (held in stage 2, no
evidence anywhere).

Each program: its own profile (`XO_PROF=cpu XO_PROF_HZ=10000` on a
`--release --profile-support` build, `xo prof --layout`), then quiet
`xo bench --ab`, 15 rounds, each pair interleaved; "off" is `--layout=off`,
"lay" the layout file, 0 and 8 the placement (XO_ARM64_PAD). B/A with the
95 percent interval; the one-binary pair (off0 against a copy of itself) is
the noise floor.

| program | pair | what it shows | wall | instructions | cycles |
|---|---|---|---|---|---|
| collatz | off0 vs off0 copy | the floor | 0.998 (0.99 to 1.009) | 1.0 | 0.995 (0.99 to 1.004) |
| collatz | off0 vs off8 | placement alone | 0.995 (0.991 to 1.005) | 1.0 | 0.996 (0.993 to 1.0) |
| collatz | off0 vs lay0 | the split at placement 0 | 0.995 (0.988 to 1.004) | 1.0 | 0.994 (0.989 to 1.001) |
| collatz | off8 vs lay8 | the split at placement 8 | 1.004 (0.993 to 1.009) | 1.0 | 1.006 (1.0 to 1.01) |
| strings | off0 vs off0 copy | the floor | 0.987 (0.977 to 1.004) | 1.0 | 0.991 (0.983 to 1.009) |
| strings | off0 vs off8 | placement alone | 1.027 (1.0 to 1.032) | 1.0 | 1.021 (1.001 to 1.03) |
| strings | off0 vs lay0 | the split at placement 0 | 1.006 (0.997 to 1.023) | 1.0 | 1.003 (0.998 to 1.021) |
| strings | off8 vs lay8 | the split at placement 8 | 0.979 (0.958 to 0.991) | 1.0 | 0.982 (0.966 to 0.992) |
| interp | off0 vs off0 copy | the floor | 0.995 (0.979 to 1.016) | 1.0 | 1.0 (0.98 to 1.015) |
| interp | off0 vs off8 | placement alone | 0.988 (0.984 to 1.017) | 1.0 | 0.989 (0.982 to 1.009) |
| interp | off0 vs lay0 | the split at placement 0 | 0.997 (0.965 to 1.016) | 1.0 | 1.004 (0.965 to 1.016) |
| interp | off8 vs lay8 | the split at placement 8 | 1.004 (0.974 to 1.021) | 1.0 | 1.003 (0.974 to 1.021) |

Profiles: collatz 3 functions, 3 lines (main 255 of 257 samples); strings
19 functions, 19 lines (obj.rs_push_word 62, alloc.alloc 32, obj.reserve_slow
26, num.append_int 23 of 230); interp 3 functions, 3 lines (run 223 of 225).

What the split moved (blocks of the hottest functions in the assembly, off
against lay): collatz main 61 and 61; strings obj.rs_push_word 12 and 12,
alloc.alloc 32 and 32; interp run 275 and 274. `__text` of the program: collatz
129156 both at placement 0 (136412 at 8); strings 129576 off, 129560 lay
(136824 and 136800 at 8); interp 133592 both (140936 at 8). xo's own `__text`
69857876 (xo carries no layout file, so its code is the parent's; the split
enters only a build with a profile).

## Reading

The dominator split changes nothing measurable: with a 10 kHz profile every
block of a hot loop is sampled or dominates or post-dominates a sampled one,
so collatz and interp keep every block in place (one block of interp's run
moves) and strings moves a few blocks of cold error paths (16 bytes of text).
Every split pair is inside its program's floor: collatz and interp within
noise at both placements, strings' only "better" (off8 vs lay8, 0.979) sits
beside the placement pair's "worse" (off0 vs off8, 1.027) and the floor's 0.987,
so it is placement, not the split. Placement itself is the only effect that
clears the floor here (strings 1.027 at pad 8), and the split does not flatten
it. Held, as stage 2 was: the code stays out of main (the diff in the lead's
notes). What a block level layout would need to show anything: a program
whose hot functions have real cold paths (error handling, rare branches) in
the middle of hot code, measured by the same pairs; the three here have none.
