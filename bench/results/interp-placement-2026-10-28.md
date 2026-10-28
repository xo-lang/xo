# interp: a 38 percent placement spread, and threaded dispatch (2026-10-28)

Apple M4 Pro, xo at 3114a7f7e plus the thread_switch prototype
(compiler/arm64 threadsw.xo), bench/perf/llvm/xo/interp (a stack
machine running a 16 op program 3M steps). One quiet window (load 4),
11 rounds, every binary once per round in a fixed order; the median of
`/usr/bin/time -l`'s cycles and instructions. Notes:
~/xo-lead/notes/xo-lang-6/inliner-interp.md (xo-lang-6).

## The question

Lang 1's 0388 stage 3 oracle built interp with std's bodies kept out
of program code (XO_ORACLE_OPAQUE=inline) and saw 74 to 63 ms, three
times: inlining std into interp looked 15 percent too eager.

## What the inliner does: nothing

`xo dev dump asm --ssa=inline --fn=run` is byte for byte the same with
and without the oracle (1323 lines), and run's final code is the same
modulo placement under every oracle kind. Only main changed (171 to 76
instructions: println and the interpolation no longer inlined), which
moved run from 0x100000ca0 to 0x100000c1c.

| binary | cycles | instructions | wall |
|---|---|---|---|
| A, normal | 276.8M | 1458.7M | 76 ms |
| B, XO_ORACLE_OPAQUE=inline | 208.6M | 1458.4M | 59 ms |

Same instructions, 25 percent fewer cycles: placement.

## The same code at 16 placements (XO_ARM64_PAD, decision 0075)

run's body is identical at every pad (only adrp page numbers differ).

| pad | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| cycles M | 281.4 | 233.1 | 207.1 | 238.0 | 210.5 | 238.2 | 225.7 | 238.7 | 218.1 | 237.2 | 221.9 | 237.3 | 204.5 | 233.3 | 212.1 | 230.9 |
| wall ms | 77 | 65 | 59 | 67 | 61 | 67 | 64 | 67 | 62 | 67 | 63 | 67 | 58 | 65 | 60 | 65 |

204.5M to 281.4M, a 38 percent spread of one instruction stream; the
default placement (pad 0) is the worst of the sixteen. Even pads run
204 to 226M, odd pads 231 to 238M. run's loop dispatches the 16 op
cycle through one jump table `br` (decision 0051), whose indirect
predictor is indexed by the branch's address and the history, so where
the `br` lands decides how well it learns the period 16 sequence.

## Threaded dispatch (thread_switch, pad 0)

The latch (pc += 1), the header (steps += 1, the bounds check, the
`ldp` of tag and payload, the Push test) and the dispatch block are
copied into each of the 9 arms that jump back, so each arm ends in its
own table `br`; the copies are placed after their arm. run: 491 to 957
lines of code, 1 to 10 `br`; the output is unchanged.

| binary | cycles | instructions | wall |
|---|---|---|---|
| off (XO_PASSES=-thread_switch) | 284.8M | 1458.5M | 78 ms |
| on | 161.7M | 1338.3M | 48 ms |

43 percent fewer cycles at this placement, and 8 percent fewer
instructions (the copied latch saves the arm's jump to it, and the
allocator stopped reloading `prog` from the frame each iteration).

Over 14 bench programs (bench/perf/llvm's seven, json, strsplit,
bytessearch, httpparse, unicode, text, portable) the pass changes the
code of two: interp, and text's time.format_items (the layout item
dispatch of time formatting, 1 to 14 `br`, 1533 to 1885 lines). Every
other program is byte-identical, so there is nothing to lose there.

## The threaded code at 16 placements

Same method, same window kind (load 4.6), the pass on at every pad and
off at pad 0.

| pad | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| cycles M | 162.0 | 160.4 | 163.1 | 162.4 | 166.4 | 168.5 | 161.3 | 158.4 | 164.2 | 167.0 | 164.2 | 163.2 | 162.5 | 162.1 | 169.1 | 164.5 |
| wall ms | 50 | 51 | 51 | 50 | 51 | 52 | 50 | 49 | 50 | 50 | 50 | 51 | 50 | 50 | 53 | 52 |

158.4M to 169.1M: a 6.8 percent spread (38 before), every placement
under the unthreaded code's best (204.5M); 1338.5M instructions at
every pad; off at pad 0 in the same window 283.4M. The pass is on by
default (passes.xo thread_switch; XO_PASSES=-thread_switch turns it
off).

## text (time.format_items threaded)

`xo bench --ab --rounds=11` of bench/perf/text, the pass off (A) and
on (B), load 4.14 in a quiet window: wall 4.72 vs 4.65 ms (B/A 0.985,
0.938 to 1.029), cycles 12.08M vs 11.96M (0.99, 0.963 to 1.029),
instructions 45.98M vs 45.96M (1.0): within noise. The threaded
function is not the program's hot path; it does not lose.
