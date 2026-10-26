# hoist_consts and rare branches' constants (negative result)

Date: 2026-10-25. Xo Lang 1. Machine: the development Mac (arm64).
Instructions retired, bench/perf/sweep.py, 2 rounds, base main's xo
(b7faeedb3), new the branch; the branch is kept as
`refs/recover/xo4-hoist-every` (7a3117988), not on main.

## Question

Inline B (1729fd39e, decision 0184 addendum) made Str `cut` (DS-2's
bench/perf/text cut rows) 1.015 times its parent's instructions on
arm64, while cut_prefix and cut_suffix went to 0.85. Why, and can
hoist_consts be fixed for it?

## Cause

B shrank scan.eq_at to 57 instructions, under INLINE_LOOP_INSTRS (60),
so it is inlined at loop sites. In find_bytes' candidate loop that
removed the loop's last call. hoist_consts moves only the dearest
constants out of a loop that calls on its hot path, and every constant
out of one that does not. So six one-instruction constants of rare
branches (eq_at's length ladder, find_short's masks) were made in
the preheader on every find_bytes call, a loop that runs about once a
call for `cut`. `XO_INLINE_DENY=scan.eq_at` on B gives the parent's
count exactly (194.34M against 194.33M).

## Rules tried in compiler/arm64/opt.xo hoist_consts

| rule | text/cut | slower rows (of 189) | worst |
|---|---|---|---|
| 1. a one-instruction constant moves only from a block every iteration runs | 0.951 | 8 | stdio/lines 1.050, stdio/small 1.037, text/dur_parse 1.029 |
| 2. as 1, paths through cold blocks not counted | 0.951 | 6 of 98 (mover dirs) | stdio/lines 1.047 (the flush path is not cold) |
| 3. as 2, but only when a loop has more than 4 such constants | 0.951 | 3 | unicode/title 1.016, unicode/classify 1.014, ctx/none 1.008 |

Rule 3's faster rows: text/cut 0.951, strsplit bytes_split 0.944,
split_after 0.967, split_multi 0.979, split_absent 0.981, text/replace
0.974, unicode/runecase 0.981, json/invalid 0.984, text/base64 0.988,
tls/chacha 0.989, json bytes/small 0.995, strsplit/split 0.995.

## Why it stops here

The rows a rule loses and the rows it wins look the same to a static
rule: unicode's is_space test has more than 4 one-instruction
constants (0x2000, 0x200a, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000,
0x1680, 0x10000) in a branch that non-ASCII text takes every rune, a
hot loop; find_bytes has 6 in branches a loop that runs once a call
seldom takes. Telling them apart needs trip counts or branch weights.

## Next steps, ranked

1. Encode 0x2000, 0x3000, 0x10000 (and other `imm12 << 12` values) as
   `cmp x, #imm, lsl #12` in isel, so is_space's constants need no
   register at all; then rerun rule 3, whose classify and title rows
   come from exactly those constants.
2. A static trip-count estimate per loop (a loop whose header is
   reached from a find-style scan that exits on its first match, or
   whose latch is only on a mismatch path), used by hoist_consts.
3. Lower INLINE_LOOP_INSTRS for a callee whose loop site sits on a
   branch after the loop's exit test (eq_at in find_bytes), or keep
   eq_at a call there with a site rule, then measure cut again.

## Rerun on the shifted immediates (2026-10-26)

Next step 1 landed as 28fe5d24c (`cmp x, #k, lsl #12` for 0x2000,
0x3000, 0x10000; 163 workloads, 0 slower, 5 faster; its TestGoldenAsm goldens c79a62256). is_space then
compared offsets from 0x2000 (`refs/recover/lang1-isspace`, 3fc907734; alone 0 slower, 0 faster of 74 rows, so not landed), so none of its
constants needs a register. Rule 3 rebased on both
(`refs/recover/xo4-hoist-rule3b`), 2 rounds, base the two changes:

| rule | text/cut | slower rows (of 163) | worst |
|---|---|---|---|
| 3 | 0.950 | 4 | unicode/title 1.017, stdio/err 1.015, stdio/mixed 1.012, ctx/spawn 1.006 |
| 3b: constants an inner loop's pass lifted into its preheader do not stay | 0.950 | 3 | unicode/title 1.017, json/parse 1.008, sysio/write 1.023 (spread 1.008; 1.017 in an unrelated sweep the same hour) |

classify went from 1.011 to 1.000 (is_space). title's cost is
unicode.case_map, inlined through title_sum into main: its found path
(the delta's lookup, `kind * 3`, 16777216 and 8388608 masks) is outside
the binary search's natural loop and on the rune loop's non-ASCII
branch, so rule 3 keeps its constants there, run on every non-ASCII
rune, where the base hoists them out of the whole loop. That branch is
the common one for title's input and the rare one for find_bytes'
candidate loop; the static rule still cannot tell them apart.

Ranked next steps (replacing the list above):

1. A static trip-count estimate per loop (find_bytes' candidate loop
   exits on its first match; a loop whose only latch is on a mismatch
   path runs about once a call), used by hoist_consts: keep a rare
   branch's constants only in a loop estimated to run about once.
2. eq_at kept a call at find_bytes' candidate site (an inliner site
   rule), then text/cut measured again without any hoist change.
3. Profile-guided weights (XO_PROF samples per block) for hoist_consts;
   the largest change, needs a profile format for blocks.
