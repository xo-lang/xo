# Str starts_with, ends_with and find with short needles: no gap, 2026-10-24

The question after `bytes-affix-2026-10-24.md` was whether the Str forms
go the same way, since `scan.starts_with` and `scan.ends_with` call
`scan.eq_at`, whose ladder falls to a byte loop below four bytes, and
`str_trim_prefix` uses `scan.eq_text` instead. Measured first, and the
answer is that they are already at or under Go.

A = Xo, B = Go, 20,000,000 iterations, same checksum line, one revision
(origin/main at 4a238c8df). B/A is Go over Xo, so under 1.0 means Go ran
fewer instructions.

| operation, needles of 1 to 3 bytes | Xo | Go | B/A |
|---|---|---|---|
| `starts_with` | 1,258,877,993 | 1,262,141,759 | 1.003 |
| `ends_with` | 1,283,465,252 | 1,455,892,080 | 1.134 |
| `find` (Contains) | 2,801,303,285 | 2,838,387,286 | 1.013 |
| all three in one loop | 4,166,609,515 | 4,361,630,037 | 1.047 |

## The measurement that lied first

A first cut left the mode dispatch inside the loop, `if which == "sw"`
and `if which == "ew"` on a Str from argv. It reported `starts_with` at
1.48 of Go and `ends_with` at 1.94, which would have sent someone into
`eq_at`. Its profile showed why: `obj.str_eq` 25% flat and `obj.eq_short`
18% flat, 43% of the run in the dispatch's string compares. Hoisting the
dispatch to an Int before the loop (`fn mode(a: List[Str]) -> Int`)
removed it; the operations themselves were at parity all along.

Isolated afterwards, Str equality is not the problem either: `s ==
"abcabcabc"` per iteration is Xo 426,772,599 against Go 466,589,503
(B/A 1.093, Xo 8.5% under Go), with the profile entirely in `obj.str_eq`,
which already has the length test, the pointer test, the 16-byte-or-less
word compare and `rt.mem_cmp`.

## What it means

No change to std. The affix result that started this was about
`scan.eq_at` reached through `Bytes.trim_prefix` and `trim_suffix`, where
the affixes of that benchmark are 1 to 3 bytes and the compare ran
per call against a 200,000 iteration loop's own cost; here the same
compare runs inside a 20,000,000 iteration loop whose dispatch, indexing
and modulo are shared with Go's, so the compare is a small part of the
row and the small-n path does not show.

The piece of these three that is still bigger than Go's in kind, not in
instructions, is the search: `scan.find_bytes` was 39% of the `find`
profile and `eq16_1` 14%, and Go's `bytealg` has a short-haystack search
we have not ported. `strfind-2026-10-24.md` records that porting its
brute force made the cut row worse, so that needs a profile-led look at
`find_bytes` rather than a port.
