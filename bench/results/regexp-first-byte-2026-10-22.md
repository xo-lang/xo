# std/regexp: first byte filter, 2026-10-22

Program: bench `rxb` (three patterns on a 127 KB text; modes match, find_all,
replace, captures), arm64, native. Go: the same patterns with Go's regexp.
Instructions retired (`/usr/bin/time -l`), then `xo bench --ab --rounds=7`
(before against after).

| mode | Xo before | Xo after | Go | after / Go |
|---|---|---|---|---|
| match | 7,160 M | 5,659 M | 3,034 M | 1.87 |
| find_all | 5,777 M | 5,166 M | 3,903 M | 1.32 |
| replace | 2,958 M | 2,984 M | 1,508 M | 1.98 |
| captures | 442 M | 252 M | 207 M | 1.22 |

`--ab` (B / A, 95% interval): match wall 0.757 (0.746 to 0.772), instructions
0.790; find_all wall 0.911 (0.909 to 0.932), instructions 0.894; captures wall
0.636 (0.581 to 0.685), instructions 0.570; replace wall 1.055 (0.913 to 1.189,
within noise), instructions 1.009 (worse by 0.9%). Output hashes are the same
before and after in every mode.

Changes (std/native/regexp.xo): a first byte bitmap in Prog (`first_bytes`)
used by the backtracker and the Pike loop (`next_start`); the backtracker's
job stack is one list of `pos << 12 | pc << 1 | arg`.

Still above Go: match 1.9x (the backtracker's visited-bit check and list
bounds checks dominate the profile: should_visit 36%, push 18%), replace 2x
(the Pike path with a pattern whose first set is nearly every byte).

## Tried and not kept (same day, `xo bench --ab --rounds=7` against the pushed build)

- Pike loop without the per-position empty width context for a program with
  no `^ $ \b` instruction: instructions 0.99x on replace and find_all 0.985x,
  wall 1.01x (worse or flat): the profile's 30% on empty_context was sample
  noise at 10 samples; a longer run (157 samples) puts `add` at 51% and the
  step loop at 19%.
- One packed Int per instruction (op, out, arg) in `add`, the step loop: 0.99x
  instructions on replace, wall within noise.
- The backtracker over a window of 64 positions (then 4x) instead of the Pike
  search for a long text, falling back to Pike on overflow: replace
  instructions 0.938x but wall 1.12x (worse), and the window fields in the
  backtracker's hot path made the short text modes worse (match instructions
  1.146x, captures 1.092x).

The Pike cost is in `add` (thread copies, the sparse set); the next candidate
is Go's one-pass matcher for anchored patterns, which `replace` here is not.
