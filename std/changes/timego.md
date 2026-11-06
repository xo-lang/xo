# std/timego changes

## 2026-11-06: feature: std/timego, Go time layouts on core time (decision 0537)

- Kind: feature
- Commit: this commit
- Found by: aws-19's time feedback (5f3c3e5), owner decision 2026-11-06 (separate module, core time API unchanged per 0087)
- Go reference: time.Format and time.Parse (go1.26.1), the differential oracle (internal/stdtest/timeref)
- Test: testdata/corpus/timego (17 layouts x 24 instants x offsets: 408 Format cases and the parses of Go's own output, all matching Go), std/native/timego_test.xo, TestTimegoFuzz (XO_TIMEGOFUZZ_SEED; seeds 2 to 5 pass on arm64)

New module for code ported from Go. `timego.to_core` translates a Go layout (`2006-01-02T15:04:05Z07:00`) to a core layout,
`timego.format` and `timego.parse` run it through core time. An element core time has no field for (`.999`, `_2`, `3`, `4`,
`5`, `06`, `-07:00` (Go prints +00:00 for a zero offset, core prints Z), `002`, `-07`, `Z0700`, `pm`, seconds in offsets) is a `LayoutErr`, never a literal. Parsing is strict (core's rule: a weekday in the text must match the date, which Go ignores; `15` takes two digits, Go accepts one), and a zone
abbreviation without an offset resolves in the given zone instead of Go's made up zone.

Timing (host 2, n=5, bench/results/std-twin-timego-2026-11-06.md): translating on every call is 15.0x (format) and 25.5x (parse) of Go, so
`timego.layout` translates once into a `time.Layout`: format 1.25x, parse 3.33x (Go's RFC 3339 fast path).
