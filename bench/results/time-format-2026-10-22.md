# time Layout.format against Go's time.Format, 2026-10-22

Program: bench/perf/timeformat-cache/main.xo mode `format_layout` (300,000
ISO formats in UTC and 300,000 long formats in America/New_York, lazy const
Layouts), native arm64, Apple M4 Pro. Go: the same loop with `time.Format`,
983 M instructions (bench/results/time-format-cache-variants-2026-10-22.md).

| | instructions | vs Go |
|---|---|---|
| Xo before (origin/main b428ce0ae) | 1,707 M | 1.74 |
| Xo after | 1,467 M | 1.49 |

`xo bench --ab --rounds=9` (B / A, 95% interval): wall 0.798 (0.789 to 0.815),
instructions 0.859 (0.859 to 0.859), max RSS within noise. Output is the same
(md5 of the 14,700,000 bytes).

Changes: `tz.lookup` (offset and name position in one walk, instead of
`offset` then `abbr`, which both ran `tzset` for a date past the last
transition); `{zone}` builds its string only when printed; `put_num` writes
width 2 and 4 numbers without the division loop; `BytesBuilder.with_capacity(40)`.

Tried and not kept: a table of two digit pairs written with `write` of a
slice: 1,663 M to 1,776 M (worse than two `write_byte` calls).

Still above Go: a byte at a time through `write_byte` (obj_rs_push_byte and
reserve, about 20% of the profile) and `tzset`'s per call rule arithmetic
(year_yday, rule_time; Go does the same work).
