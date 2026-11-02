# Part borrow: parked at -0.76% of xo's code

Date: 2026-11-02. Parked by the Lead's bar (under 1% of size after every
quadratic case is closed). Decision 0149 stands, and no ADR changes it.

## The idea

Borrow inference makes a struct parameter owned if any path consumes any
leaf, so `fn (r: Reader) conn() { r.h }` makes every caller retain every
word of `r`. Decision 0149 kept that rule for non-enums because its first
all-types version was 25x slower: a `List` or `Map` at a count above one is
copied on update. The attempt: keep a struct or tuple parameter borrowed
when only some of its leaves are consumed, unless a consumed leaf needs a
count of one to update in place.

## Results

xo's native `__text`, one Mac build each. The early rows are against
33,896,264 on an older main; the last is same source, each xo at its own
commit, on 469c7f82d.

| Rule | `__text` | Change | Worst cases |
|---|---|---|---|
| any Str leaf may stay borrowed | 31,232,896 | -7.9% | strgrow 1.1 ms to 1,200 ms |
| returned Str leaves only | 33,745,976 | -0.44% | not swept |
| Str unless a call consumes it | 33,103,668 | -2.3% | strwrap, stredge about 800x |
| flow rule (trace to an append) | 33,057,312 | -2.5% | through a call result 1.3 ms to 1,141 ms |
| plus call results | 33,081,928 | -2.4% | getter in a borrowed body 1.4 to 1,055 ms; local List read 2.8 to 979 ms |
| plus list reads and returns, per function | 33,792,684 | -0.31% | flat |
| per parameter | 33,776,196 | -0.35% | flat |
| only parameters that reach the return | 33,666,464 | -0.68% | flat |
| per Str leaf | 33,669,652 | -0.67% | flat |
| final, reads that keep their element stop the trace | 33,735,316 against 33,992,236 | -0.76% | flat |

Each saving above about 1% came from a real quadratic case. The rule that
closes them all costs 480 lines of `compiler/ssa/rc.xo`.

An intermediate -2.29% for the final rule was false: `xo` built from that
code was itself miscompiled by the thread_cset bug (469c7f82d,
gotchas/branch-compare-past-parallel-move.md), so its trace stopped after
one pass and dropped the fixes. It is how that bug was found.

Speed of the final rule, linux/arm64 `--ab`, 3 rounds, worst case rows only:
strgrow 0.999, strget 1.010, strwrap 1.020, stredge 0.998, strthru 0.995,
strmid 1.012, strlist 0.986 (0.933 to 1.156), listget 0.985, mapput 0.992,
mapleaf 0.993. The run was stopped when the size missed the bar, so self,
alloc, sort, json and HTTP were not measured on this rule. On the earlier
rule 1db6eb2f5 (which had holes) self was CPU 0.994, max RSS 0.989, on a
loaded host: no claim.

## What was learned

- A borrowed parameter's caller holds the leaf across the whole call, so
  anything in the body that updates the leaf in place, or passes it to code
  that does, now copies. "Reaches an append" has to follow call results,
  list reads and returns across callers, not only the body.
- Summary benches do not see this: the first rule's -7.9% looked clean until
  a named row went 1,000x. Each later rule passed its rows until a new row
  was written, so the rows are kept as a test.
- Known quadratic with or without the rule, so not a row: a List built and
  read inside the same loop turn, then appended to (about 1,000 ms on base).

## Kept

- `bench/perf/rcpart`: every row, `rcpart check`, internal/arm64
  `TestRcpartFlat`. Run them for any change to borrow inference.
  Without the rule 20 rows, 0 slow; under the flow rule 1db6eb2f5, which had
  the holes, 10 slow, each 867 to 1,231 ms (threshold 300 ms).
- The rule, unmerged: local branch `lang1-partborrow2`.

## Next paths, ranked

1. Cost of a retain at the call site rather than the convention: retained
   leaves are what the rule tried to avoid, and the retain thunk and
   `rc_thunk_loops` shrank each one to a call. Lever C of the rc work,
   coalescing adjacent retains of one struct's fields into one call, is
   untried: count those runs with `xo dev size --by-cause ./compiler/xo`.
2. Per-return-part tracking (which part of a struct result the caller
   appends to) is the largest remaining over-approximation; try it only with
   a new estimate that it can pass 1%.
