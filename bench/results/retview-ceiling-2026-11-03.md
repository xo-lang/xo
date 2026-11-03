# Decision 0465 step 2's ceiling (2026-11-03, Xo Lang 1)

What returning a Str view uncounted (decision 0465 step 2) is worth at
most: bench/perf/retview's `field`, a function too big to inline (527
instructions after insert_rc) whose every return is a slice of its
borrowed Str parameter or a constant, called 3,000,000 times.

- `use`: the result's length only (step 2: no count at all).
- `keep`: every result stored in a List (step 2: the caller retains it).

Step 2 is not on main; it was measured from its branch (lang1-0465s2,
f40e1704d plus this program), against 219d17223 plus this program alone.

## linux/arm64 (first offload host), xo bench --ab

| workload | A median | B median | B/A | 95% interval | rounds |
|---|---|---|---|---|---|
| use | 12.42 ms | 8.726 ms | 0.702 | 0.688 to 0.718 | 6 |
| keep | 20.22 ms | 15.26 ms | 0.755 | 0.737 to 0.952 | 15 |

## Instructions retired (Mac, /usr/bin/time -l, XO_PASSES=-ret_views against on)

| workload | off | on | change |
|---|---|---|---|
| use | 329,344,402 | 292,861,007 | -11.1% |
| keep | 413,305,360 | 407,235,866 | -1.5% |

## What qualifies (XO_INLINE_REPORT, "returns a view")

On the same branch a function qualifies when its returns are bare slices of
the parameter and constants. These leave a function out: returning the
parameter itself (`line`), a loop that slices it, `bytes()`, and a call of
std's `trim` or `trim_chars` (prelude str_trim, str_trim_chars: two
instruction wrappers that are not marked). In xo's own build 17 functions
qualify, 16 of which the inliner took before step 2, so step 2 as built
made xo's self build 0.8% slower (1.008, 1.005 to 1.010) and the
trimchars rows did not move (trim 0.994, trim_end 0.952, both within
noise).
