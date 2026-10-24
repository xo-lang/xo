# to_lower and to_upper fold as they copy: 2026-10-24

Follow-up on `case-fold-2026-10-24.md`, which made `obj.fold` change
eight bytes at a time: after it, the appended ASCII runs were still
copied (`move_at`) and then walked again to be folded, so every byte of
such a run was read and written twice.

Program: `bench/perf/unicode` and its Go twin `bench/perf/unicode/go`.
Instructions retired (`/usr/bin/time -l`, best of 5), arm64 native, with
`XO_INLINE_STRICT=1` on both sides so a module that fails verification
cannot silently build without inlining. Base is `origin/main`'s std
(`/tmp/xo-base-run`, its program built by the pre-change `bin/xo`); fused
is the worktree's `std/native/llrt/obj.xo`, where `rs_append_bytes` folds
and copies in one pass (`fold_copy`). Every checksum is identical on
arm64, LLVM, wasm and Go, which is the cross check that the four programs
do the same work.

| workload | base | fused | fused / base | Go |
|---|---|---|---|---|
| lower (ASCII, 0.4 MB x 200) | 392.8 M | 378.5 M | 0.963 | 2,724.6 M |
| upper (ASCII, 0.4 MB x 200) | 402.5 M | 387.2 M | 0.962 | 3,340.3 M |
| lower_az (letters only, 104 KB x 200) | 107.3 M | 104.6 M | 0.975 | 660.0 M |
| upper_az (letters only, 104 KB x 200) | 107.4 M | 104.5 M | 0.973 | 662.4 M |
| lower_u (mixed, 260 KB x 200) | 4,596.8 M | 4,554.8 M | 0.991 | 7,763.1 M |
| upper_u (mixed, 260 KB x 200) | 4,568.9 M | 4,515.0 M | 0.988 | 7,748.9 M |
| eqlower (case compare) | 11,596.8 M | 11,522.3 M | 0.994 | 21,490.3 M |
| controls (classify, runecase, classes, latin1, title, utf8len, quote, indexfunc, trimfunc, fieldsfunc, mapfunc) | | | within 0.1% | |

Wall time, best of 5 on a loaded machine, so it is weaker evidence than
the counts: lower 16.74 to 16.56 ms, upper 16.75 to 16.06 ms, lower_az
3.53 to 3.34 ms, upper_az 3.56 to 3.31 ms, lower_u 1.078 to 1.081 s
(noise).

What it says:

- The fused pass removes one read and one write of every copied byte. An
  ASCII text is a single span, so the whole call is the copy and the
  rows gain 3 to 4 percent; a mixed text is a span per ASCII gap plus a
  decode per non-ASCII rune, and its instructions are decoding and case
  lookup, not copying, so it gains 1 percent.
- `lower_az` and `upper_az` are new: a text of letters only, so every
  byte of every eight byte word folds and no range test finds a gap.
- The remaining cost of the ASCII rows is now divided between the fold
  loop and the destination allocation: each iteration allocates a 400 KB
  string, and first touch page faults are as large as the folding.
- Hoisting `between8`'s two range constants out of the loop (they were
  recomputed per word) measured no change (378.26 M before the hoist,
  378.45 M after), so the compiler already hoisted them. The hoist stays
  for clarity.
- The mixed rows are where the absolute cost is: 4.5 G instructions, 0.59
  of Go. Decoding a rune (`raw.decode`) and `to_case` per rune are the
  next thing to look at, not the copy.
