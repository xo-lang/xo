# Bytes.trim_prefix and trim_suffix: eq_at's small compares, 2026-10-24

Follow-up to `bytes-split-2026-10-23.md`, where the affix row
(`trim_prefix` and `trim_suffix`) was the one row where Xo ran more
instructions than Go: 1.80 gross and 1.96 net of the loop.

Program: `bench/perf/bytessplit`, the affix workload (200,000 iterations,
a text and an affix from tables), against its Go twin. All instruction
counts are `xo bench --ab --rounds=9 --keep-loaded`.

## Where it stood today

| | Xo, M | Go, M | Xo / Go | net Xo, M | net Go, M | net Xo / Go |
|---|---|---|---|---|---|---|
| the loop alone (index row) | 29.58 | 23.39 | 1.26 | | | |
| main before tw380 | 54.23 | 38.67 | 1.40 | 24.65 | 15.28 | 1.62 |
| main with tw380 (2aa576ce2) | 49.15 | 38.21 | 1.29 | 19.57 | 14.82 | 1.32 |
| main with tw380 and `scan.eq_bytes` | 44.04 | 38.25 | 1.15 | 14.46 | 14.86 | 0.97 |

The 1.80 of the October file had already come down to 1.40 on today's
main before either change: the loop itself fell from 1.71 to 1.26.

## The profile

`XO_PROF=cpu XO_PROF_HZ=500` on a 300,000,000 iteration run of the same
two calls, clean `origin/main` tree, 442 samples in 2.9 s:

| flat | frame |
|---|---|
| 41.9% | `scan.eq_at`, line 178 (its byte-by-byte loop for n below 4) |
| 30.1% | `main` (the bench loop, its `% 6` and `% 7`) |
| 19.5% | `drop Str` (the view's retain and drop; Lang 1's area, not touched) |
| 4.3% | `prelude.bytes_trim_prefix` (28.7% cumulative) |
| 3.9% | `prelude.bytes_trim_suffix` (21.5% cumulative) |

The bench's affixes are 1 to 3 bytes, so every compare took `eq_at`'s
tail loop and its 4-to-7 and 8-to-16 word paths never ran. That was 42%
of the row against the view drop's 19.5%.

## What was done

`scan.eq_bytes`, `eq_text`'s form for Bytes: the first and last 4 bytes
of a 4-to-n compare, the first and last 2 bytes of a 2-to-3 byte compare,
one byte for one, all on `raw.bytes_addr` and `rt.mem_load` with no Bytes
values and no bounds checks (the caller has tested `p.len() <= b.len()`).
`Bytes.trim_prefix` and `trim_suffix` call it where they called
`scan.eq_at`, which stays as it is for every other caller.

tw380 (`2aa576ce2`) had already taken the same row from 1.40 to 1.29 by
inlining compares under 8 bytes. `eq_bytes` takes it to 1.15 gross and
0.97 net, which is the part left over: at 10.4% fewer instructions than
main with tw380 (B/A 0.896, 95% interval 0.896 to 0.897, 9 rounds), well
outside noise.

## What is left

The view's retain and drop is now 19.5% of the row (Lang 1 owns it), and
the bench loop's constant modulo is the index row's 1.26: a standalone
loop of `sum += t[i % 6]` measures 27.3 instructions per iteration
against Go's 19.4 (1,366,311,111 against 970,377,139 for 50,000,000
iterations), which is Lang 1's divmod work, not std's.
