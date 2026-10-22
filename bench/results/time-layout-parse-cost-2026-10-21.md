# What parsing a time layout costs per call (2026-10-21), for decision 0332

Measured to size lazy `const` / a parsed-layout cache: std/time's `format`
and `parse` parse their layout string (`{year}-{month}-...`) on every call.

## Method

Native arm64 on the development Mac (Apple M4 Pro), xo from main at the
parent of this commit, instructions retired (`/usr/bin/time -l`), the setup
(process start, zone load) removed by a `none` run. Two layouts, each used
300,000 times:

- `{year}-{month}-{day}T{hour}:{minute}:{second}{offset}` (UTC)
- `{weekday:short}, {day} {month:short} {year} {hour}:{minute}:{second} {zone}`
  (America/New_York)

The layout-only figure comes from a scratch export that runs
`layout_items(layout)` and returns the item count (not committed), called 300,000
times per layout.

## Result

| work | instructions (M) | per call |
|---|---|---|
| format, both layouts, 600,000 calls | 6,166 | 10,277 |
| layout parse alone, the same 600,000 layouts | 3,830 | 6,383 |
| the rest of format (digits, zone, builder) | 2,336 | 3,894 |

Parsing the layout is 62% of `format`. With a cache hit costing about 200
instructions (one Mutex-held Map lookup) `format` would take about 2,500 M
instructions, 2.5 times fewer (Go's `time.Format` takes 953 M for the same
calls, so 6.5 times Go becomes 2.6 times); a lazy `const` parsed once at first
use drops the lookup as well (about 2,400 M). `parse` pays the same layout
parse, so it gains in proportion (its remaining cost is the input scan).

The figures are before this day's byte-level recognizer too: see
bench/results/regexp-time-csv-vs-go-2026-10-21.md, steps 1 and 6, for the
other time changes.
