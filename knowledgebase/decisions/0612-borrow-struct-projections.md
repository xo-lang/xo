# 0612: A struct parameter whose consumed leaves are some of its fields stays borrowed

Status: Accepted (owner, 2026-11-12; proposed by Xo Lang 1, 2026-11-11)

## Context

Borrow inference (decision 0046, rc.xo borrow_scan) makes a parameter
owned once any path consumes any of its leaves; decisions 0149 and 0489
keep enums and enum only structs borrowed unless every path consumes
them. A by-value method that returns one field of its receiver, such as
the arm64 backend's `fn (g: G) lbl_local() -> Str { ... g.lp_str }` or
`fn (g: G) memo_at(i) -> Memo { ... g.cm }`, therefore takes its
receiver owned, and a caller that keeps the receiver (any `var g`
method) counts up every heap field of it for the call while the callee
drops them all after. G has dozens of heap fields: on the aiplatform
test build these copies were most of `inc thunk_x9` (5.6% of CPU) and of
the List drops under `lbl_local` (1.5%) and `memo_at` (1%).

## Decision

A struct or tuple parameter (not enum like) of at least 8 heap leaves
(PART_BORROW_MIN, compiler/ssa/rc.xo), whose consumed leaves are some
but not all of its heap leaf paths, stays borrowed. borrow_scan collects
the consumed paths (consumed_path) and part_consumed decides; the paths
that consume a leaf count up that leaf where they consume it, through
the retains decisions 0149 and 0489 already insert.

The threshold is measured, not derived. Without it (c76f23c51) the rule
borrowed every partly consumed struct, and bench/perf/rcpart's rows, two
leaf builders a caller moves into a callee that appends to a leaf and
stores it back, went quadratic: a borrowed leaf has a count above one at
the append, so the append copies (decision 0149's trap). `rcpart check`
went from 39 ms to 16.4 s (17 of 20 rows slow) and TestRcpartFlat
failed. A struct a caller moves through calls is small; a large one (a
compiler's state, G) is kept by its callers, so the field it gives away
is shared either way and owned only adds the copy of every other leaf.
At 8 leaves every rcpart row is flat again.

## Measurements

All against 0b6f7faa6, linux/arm64, `--ab` medians with 95% intervals.

| run | metric | without threshold (c76f23c51) | at 8 leaves (68fbc3c4b) |
|---|---|---|---|
| aiplatform test build (9 rounds) | wall | 0.945 (0.936 to 0.954) | 0.937 (0.915 to 0.972) |
| | cpu | 0.882 (0.876 to 0.890) | 0.856 (0.835 to 0.883) |
| | rss | noise | 0.993, noise |
| stage2 self build (7 rounds) | wall | 0.939 (0.937 to 0.941) | 0.944 (0.942 to 0.949) |
| | cpu | 0.840 | 0.844 (0.842 to 0.849) |
| | rss | 0.95 | 0.953 (0.945 to 0.965) |
| rcpart check | | 417x worse (39 ms to 16.4 s) | 1.001 (0.999 to 1.006), flat |
| json parse, template render | | noise, 0.997 | json 0.982 and template 1.003, both noise |

Notes: ~/xo-lead/notes/lang1/bproj-aip.txt, bproj-rt2.txt (without),
bproj8-aip.txt, bproj8-rt.txt (at 8). The aiplatform run at 8 leaves ran
on host 3, the others on host 1.

## Consequences

- A `var` caller of a by-value method that returns one field of a large
  receiver no longer counts up and drops every heap field of the
  receiver per call; the arm64 backend's lbl_local and memo_at were the
  measured cases.
- Structs under 8 heap leaves keep the old rule (owned once any leaf is
  consumed), so the in-place update paths rcpart guards are unchanged.
- test/codegen/bproj (a 10 heap field struct; three consecutive inc
  thunk calls in `step` fail the pattern) and TestRcpartFlat guard both
  sides.

## Alternatives

- No threshold, restrict instead by whether a consumed leaf is updated
  in place later in the caller: needs caller knowledge in a callee's
  convention (the convention is per callee), so it was not taken.
- Per-leaf ownership in the calling convention (pass the given away
  field owned, the rest borrowed): a convention change across every
  backend; the threshold gets the measured gain without it.
