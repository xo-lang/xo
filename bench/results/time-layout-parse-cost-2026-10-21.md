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

Parsing the layout is 62% of `format` (measured). The two figures below
are ESTIMATES, not measurements (6,166 M minus 3,830 M plus about 200
instructions per hit; measuring both variants is a follow-up). With a cache hit costing about 200
instructions (one Mutex-held Map lookup) `format` would take about 2,500 M
instructions, 2.5 times fewer (Go's `time.Format` takes 953 M for the same
calls, so 6.5 times Go becomes 2.6 times); a lazy `const` parsed once at first
use drops the lookup as well (about 2,400 M). `parse` pays the same layout
parse, so it gains in proportion (its remaining cost is the input scan).

The figures are before this day's byte-level recognizer too: see
bench/results/regexp-time-csv-vs-go-2026-10-21.md, steps 1 and 6, for the
other time changes.

## Reproduce

Program: bench/perf/timeformat/main.xo (modes `none`, `format`, `parse`,
`civil`, `duration`; the `format` mode is the 600,000 calls above). Commit
measured: 80d74035b (main when the figures were taken); the program is
committed in the commit that adds this section.

```
cd bench/perf/timeformat
xo build --backend=arm64 -o /tmp/timefmt.bin .
for m in none format; do /usr/bin/time -l /tmp/timefmt.bin $m 2>&1 | grep "instructions retired"; done
```

The figure is the `format` run minus the `none` run, median of 5. Go's 953 M
comes from a Go twin of the same loop (time.Format with the two layouts'
Go equivalents).

The layout-only figure (3,830 M) came from a scratch edit not committed: add
`export fn layout_count(layout: Str) -> Int { match layout_items(layout) { Ok(v) => v.len()  Err(_) => 0 } }`
to std/native/time.xo (with a declaration in std/time.xo), and run the same
loop calling `time.layout_count` on the two layouts 300,000 times each. Redo it
that way to confirm; the original program was not kept.
