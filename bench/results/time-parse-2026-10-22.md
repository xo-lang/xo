# time.Layout.parse against Go (2026-10-22)

Program: bench/perf/timeformat `layoutparse` (300,000 parses of an ISO layout
with an offset and 300,000 of "{weekday:short}, {day} {month:short} {year}
{hour}:{minute}:{second} {zone}"), native arm64 on the development Mac. Go
twin: `time.Parse` with "2006-01-02T15:04:05Z07:00" and "Mon, 02 Jan 2006
15:04:05 MST" over the same texts, including the same Sprintf per iteration.

| | instructions retired |
|---|---|
| Go time.Parse | 1,290 M |
| Xo before | 3,605 M (2.8x) |
| Xo after | 1,622 M (1.26x) |

`xo bench --ab` (7 rounds, before against after): wall 0.409 (0.392 to 0.441),
cpu 0.389, instructions 0.450. Profile before: `parse_items` 80% cumulative,
with `time.pad` and `num.append_int` 14% (the error-text date built on every
call), `rs_push_word` 11% (the month and weekday name lists), `str_eq` 8% (the
field names as strings). After: the item loop itself and `read_digits`.
