# Fewer instructions in a hot native loop can be slower

Status: Observed (2026-10-08, internal/arm64, bench nbody on Apple M4 Pro;
cause narrowed 2026-10-08, see below)

Symptom: dropping two never taken bounds checks (`cmp`, `b.hs`) from
nbody's `advance` inner loop retired 6 to 8 percent fewer instructions
and ran 1.0 to 1.4 percent slower (690 against 683M cycles, interleaved,
7 to 9 runs). Computing an invariant element address before the loop
instead of in it: 0.9 percent slower. Moving `advance` by 4 bytes (a
change in another function) moved it by 0.7 percent either way.

Cause: the speed of this loop depends on the size and placement of its
54 instructions, not on what the removed ones did. Measured by hand on
`prog-darwin.s` (2026-10-08, 7 runs each, spread under 0.2 percent):

- the checks removed and k `nop`s put at the top of the loop body, for
  k = 0 to 16: 690, 686, 688, 686, **683**, 688, 684, 687, 693, 689, 694,
  690, 689, 689, 690, 689, 686M cycles. k = 4 (the original size) matches
  the original (683M); no other size does; there is no period.
- the checks kept and 4 `nop`s added at the top: 692M; 2 and 8: 687M.
  So the original shape is itself a lucky point, not a floor.
- both checks hoisted together to the top of the body (same count):
  684M. Four `nop`s in the middle of the body: 687M; at the latch: 689M;
  after the back edge (not executed): 693M.
- the whole function shifted by 0 to 60 bytes: 683 to 692M with the
  checks, 690 to 698M without. The inner loop header aligned to 16, 32,
  or 64 bytes: 689M with the checks, 693M without.

The loop is bound by `fsqrt` and `fdiv` (about 13 cycles per
iteration) and a chain through memory (`bs[i].vx` stored and reloaded
every iteration); its throughput moves by about 1.5 percent with how the
front end groups its instructions, which the removal changes. It is a
layout effect, not a cost of the code shape: no instruction count makes
the removal faster than the original, and some make it equal.

Fix (workaround): measure every code shape change on every bench
program, in cycles (`/usr/bin/time -l`, "cycles elapsed") as well as
wall time; keep a change only if no program is slower. To tell a layout
artifact from a real cost, sweep: `nop`s at the loop top (changes the
body), and `.p2align 6` plus 0 to 15 `nop`s before the function label
(moves it), in the `.s`, linked by hand. A change whose best point over
the sweep is no better than the original's is not worth a layout risk.
The bounds check removal stays off for this reason (decisions 0054,
0060). Not tried: making the loop's speed independent of placement
(LLVM's version of the loop is 9 percent slower and placed differently;
nothing to copy).

Update 2026-10-08 (decision 0075): placement made deliberate does not
rescue the removal. Swept over 16 shifts of every function
(`XO_ARM64_LAYOUT=pad=K`, K = 0 to 15) at no alignment, functions at 64
bytes, and loop tops at 64 bytes, nbody with the removal is 1.4, 1.5,
and 1.0 percent slower by median, and without alignment or with loop
tops aligned every shift with the removal is slower than every shift
without it. The cost belongs to the loop's shape (the relative offsets of
its instructions), not to its absolute address. The pad sweep is now the
way to judge a code shape change: compare medians over the 16 shifts.
