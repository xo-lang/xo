# Profile-guided layout, stage 2: the hot blocks aligned, across placements (2026-10-29)

Decision 0403. Apple M4 Pro, xo at main 0c8407b5b plus stages 1, 1b and 2
(xo-lang-6's worktree xl42), bench/perf/llvm/xo interp, collatz and strings,
each with a layout file from a 10 kHz CPU profile of its own run (`xo prof
--layout`) against `--layout=off`, at the 16 placements XO_ARM64_PAD=0..15,
7 rounds interleaved, the median of `/usr/bin/time -l`'s cycles per
placement. The goal (the lead, 2026-10-29): the spread (max over min of the
per-placement medians) shrinking toward the noise with the median no worse.

## The control: a jump table block aligned without a profile

interp at the 16 placements, a throwaway xo that puts `.p2align 6` before
every block ending in a jump table's `br` (compiler/arm64 emit.xo, no profile)
against the same xo without it (both at main 185ea6d4f, before stage 1), 7
rounds interleaved, Mcycles per placement (medians):

| xo | pads 0 to 15 | min | median | max | spread |
|---|---|---|---|---|---|
| control | 159 174 162 171 158 161 172 161 168 157 168 177 168 159 171 155 | 155.4 | 164.7 | 176.6 | 1.136 |
| table blocks at 64 | 181 167 167 168 168 178 162 165 172 167 174 174 170 181 170 172 | 162.4 | 170.0 | 181.2 | 1.116 |

Aligning the dispatch blocks does not remove the spread (1.136 to 1.116) and
costs 3 percent at the median (164.7 to 170.0): the `br`'s low address bits
are not what the placement changes. interp's cycles are 155 to 181 M now
against 204 to 281 M in the 2026-10-28 sweep (threaded dispatch, 0397): the
spread fell from 38 to 14 percent with the code, not with alignment.

## With the layout file

Stage 2 as built: the layout file from a 10 kHz profile of each program's own
run (interp: 2 functions, 13 lines; collatz: 3 and 4; strings: 11 and 13),
the hot blocks aligned (interp 7 blocks, collatz 0: its loop head's line is
not among the sampled ones, strings 2), against `--layout=off`, Mcycles per
placement (medians of 7):

| program | side | min | median | max | spread |
|---|---|---|---|---|---|
| interp | off | 159.2 | 164.1 | 170.4 | 1.070 |
| interp | layout | 161.5 | 170.6 | 177.4 | 1.098 |
| collatz | off | 1790.3 | 1799.6 | 1806.5 | 1.009 |
| collatz | layout | 1791.4 | 1801.1 | 1812.0 | 1.012 |
| strings | off | 154.1 | 158.9 | 163.2 | 1.060 |
| strings | layout | 158.6 | 162.1 | 168.2 | 1.061 |

No spread shrinks; interp's median is 4 percent worse and strings' 2.

A second throwaway, interp's `run` at a 4 KB boundary (every address inside
the hot loop the same at every placement), against the control: control min
154.8, median 165.0, max 170.1 (spread 1.099); aligned 153.1, 165.9, 173.9
(1.135). Whole-loop address invariance does not remove it either.

## Reading

## The noise floor: one binary as 16 placements

The same statistic over one binary (each program's `--layout=off` build at
pad 0) run in 16 slots x 7 rounds, a quiet window (load 6):

| program | min | median | max | "spread" |
|---|---|---|---|---|
| interp | 155.3 | 164.0 | 176.9 | 1.139 |
| collatz | 1810.6 | 1816.3 | 1825.1 | 1.008 |
| strings | 155.8 | 157.9 | 159.5 | 1.023 |

interp's one-binary spread (1.139) is the sweeps' spread (1.070 to 1.136):
nothing of interp's remaining variation is placement; it is the run-to-run
noise of a 50 ms run. collatz's floor (1.008) is its sweep (1.009): no
placement effect either. strings has one: 1.060 in the sweep against 1.023
for one binary, about 4 percent, which neither the hot blocks' alignment
(1.061) nor anything else here touched.

## Reading

The 38 percent placement spread of interp (2026-10-28) is gone: threaded
dispatch (decision 0397) took it from 204 to 281 Mcycles down to 155 to 177
with no placement structure left (even and odd pads were distinct before;
the per-pad medians are noise now). So the measurement goal, the spread
shrinking to the noise, is met for interp and collatz by the code change,
not by layout, and stage 2's alignment has nothing to remove there and
costs 2 to 4 percent at the median where it applies; it stays off and
unpushed. strings keeps a 4 percent placement component that alignment
does not reach; the cold split's sweep (below) is the next lever on it.
A method note: a 50 ms run's cycle count varies 14 percent between runs
on this machine even in a quiet window, so a placement claim on such a
program needs the one-binary control beside it, or a longer run.

## The cold split (stage 2b)

The same sweep with three sides: off, the layout (hot blocks aligned), and
the layout with the split (a sampled function's blocks without a sampled
line moved after the sampled ones; XO_LAYOUT_SPLIT=0 is the middle side).
New 10 kHz profiles: interp 2 functions, 5 lines; collatz 2 and 4; strings
11 and 15.

| program | side | min | median | max | spread |
|---|---|---|---|---|---|
| interp | off | 156.0 | 164.6 | 175.5 | 1.125 |
| interp | layout | 158.5 | 165.9 | 172.6 | 1.089 |
| interp | layout, split | 186.3 | 188.3 | 191.9 | 1.030 |
| collatz | off | 1791.7 | 1799.7 | 1814.2 | 1.013 |
| collatz | layout | 1787.7 | 1803.4 | 1814.4 | 1.015 |
| collatz | layout, split | 1785.8 | 1788.0 | 1789.7 | 1.002 |
| strings | off | 152.1 | 156.5 | 160.9 | 1.058 |
| strings | layout | 153.4 | 159.1 | 162.9 | 1.062 |
| strings | layout, split | 171.5 | 177.6 | 179.8 | 1.048 |

The split costs interp 14 percent and strings 13 percent at the median
while flattening their spreads, and gains collatz 0.7 percent with a spread
of 1.002 against a floor of 1.008. The cause of the cost: a 50 ms run's
profile has 500 samples over 5 to 15 lines, so blocks on the hot path that
caught no sample count as cold and leave the run, and the hot path pays a
branch at each; "no sample" is not "cold" at that density. A split that
keeps a block hot when it lies between sampled blocks in the CFG (or a
profile of a run long enough that every hot line is sampled) is the version
to try; this one stays off and unpushed with the alignment.
