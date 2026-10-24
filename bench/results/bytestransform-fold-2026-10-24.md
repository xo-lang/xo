# Bytes.replace, replace_n, equal_fold and the shared Str paths: 2026-10-24

Follow-up on `bench/results/bytestransform-2026-10-23.md`, where `replace`
was 1.23 and `equal_fold` 1.26 times Go's instructions net of the loop.
XO_PROF=cpu on 20,000,000 iterations (1000 Hz, ~330 samples for replace and
~50 for fold) put the cost in two places:

- fold: `simple_fold` was 25% flat. Its 38 odd runes span 0x6b to 0xa64b, so
  the match compiled to a compare chain 38 tests long and every miss walked
  all of them (compiler/arm64/jumptab.xo makes a jump table only for a chain
  that is dense: at most JUMP_TABLE_SLACK times as many entries as tests).
  `equal_fold_bytes`' ASCII loop also computed two lowercase forms per byte
  where Go's tests byte equality first and returns at the first difference.
- replace: `cabi.obj_rs_append_bytes` was 22.7% flat and 58.9% cumulative,
  `obj.reserve` 4.9% (the capacity branch Go's pre-sized Replace does not
  have) and `move_at`/`move_small` 15.3% (the copies, which Go makes too):
  two intrinsic calls per match, that is the cost, not the loop.

## What changed (std/native only, so Str and Bytes share it)

- `unicode.equal_fold_bytes` and `unicode.eq_lower`: Go's ASCII loop shape,
  byte equality first, then the one-letter two-cases test, returning at the
  first difference.
- `unicode.simple_fold`: the table split into five buckets (r < 0x200, a bare
  -1 for 0x200 to 0x3a2, then 0x3a3 to 0x3ff, 0x400 to 0x4ff and the rest),
  so a miss costs two to five compares instead of walking the chain. The 38
  entries are unchanged.
- `strings.replace` and `strings.replace_bytes`: count first and size the
  buffer exactly as Go's ReplaceAll does (`sl + cnt * (new - old)` when new is
  longer), instead of growing; `replace_build` skips the append call when the
  segment or `new` is empty.
- `scan.count_bytes`: bytes.Count's one-byte fast path (count_byte), which
  `replace_n` was already paying for.

## xo bench --ab base vs new, 20 rounds, instructions

| workload | base | new | new / base |
|---|---|---|---|
| replace | 349,962,103 | 347,419,284 | 0.993 |
| replace_n | 165,237,908 | 158,296,896 | 0.958 |
| fold | 75,253,339 | 67,605,372 | 0.898 |
| lower | 272,888,742 | 272,919,283 | 1.000 |
| upper | 302,680,163 | 302,519,280 | 0.999 |
| repeat | 72,113,698 | 72,046,554 | 0.999 |

Every wall_ms ratio was within noise. lower, upper and repeat are untouched
paths, so their 1.0 is the control.

## Xo against the Go twin, same programs, 20 rounds, instructions

| workload | Xo, M | Go, M | Xo / Go | Xo net of index | Go net of index | net Xo / Go |
|---|---|---|---|---|---|---|
| index (the loop alone) | 41.36 | 23.30 | 1.78 | | | |
| replace | 346.96 | 274.80 | 1.26 | 305.60 | 251.50 | 1.22 |
| replace_n | 157.80 | 148.71 | 1.06 | 116.44 | 125.41 | 0.93 |
| lower | 272.70 | 394.00 | 0.69 | 231.34 | 370.69 | 0.62 |
| upper | 302.31 | 406.54 | 0.74 | 260.95 | 383.24 | 0.68 |
| fold | 67.38 | 50.00 | 1.35 | 26.03 | 26.70 | 0.98 |
| repeat | 71.83 | 79.39 | 0.90 | 30.47 | 56.09 | 0.54 |

`equal_fold` went from 1.26 to 0.98 net of the loop and `replace_n` from 1.00
to 0.93. Wall_ms now favours Xo on replace (0.79) and fold (0.78) and is
unchanged on the rest.

## What is left, and where

`replace` is still 1.22 net. The cost is two intrinsic calls per match
(`rs_append_bytes` for the segment, `rs_append_bytes` for new), each through
`cabi.obj_rs_append_bytes` (18.4% flat, 55.3% cumulative in the new profile),
plus `obj.reserve`'s capacity branch (6.7%); the copies themselves
(`move_at`, `move_small`, `platform_memmove`, 21%) are what Go pays too, and
Go's `copy` into a pre-sized slice needs neither a call boundary nor a
reserve. This is a construct cost, not a std/native one: it wants a builder
append that does not box through the C ABI per call (one intrinsic appending
a segment and a text, or a reserve-once/write-at-offset/set-length pair, or
`obj_rs_append_bytes`' fast path inlined into its caller).
