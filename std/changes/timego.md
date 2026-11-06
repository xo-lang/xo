# std/timego changes

## 2026-11-06: behavior: std/timego is Go's layout engine, not a translation to core layouts (decision 0537)

- Kind: behavior
- Commit: this commit
- Found by: xo-lead's review of the first version: a Go-compat module that rejects `-07:00`, `Z07:00`, `.999`, `_2`, `3`, `pm`, `06`, `002` and `Z0700` fails at porting Go code
- Go reference: time/format.go nextStdChunk, appendFormat and parse (go1.26.1); the differential oracle internal/stdtest/timeref
- Test: testdata/corpus/timego (44 layouts, every element of Go's layout language and the time/format.go constants, x 24 instants x offsets: Format and Parse of Go's own output and of mutated texts all match Go; no allowance for core being stricter), std/native/timego_test.xo, TestTimegoFuzz seeds 2 to 7 pass

`layout` can no longer fail and `LayoutErr`, `to_core` and `timego.Err` are gone: every string is a layout, and the module scans,
formats and parses it with Go's own rules. `parse` returns `time.ParseErr`. Changes from the first version: `15` and the other numeric
fields accept one digit where Go does; the weekday text is not checked against the date; a fraction after the seconds is read when
the layout has none; offsets take Go's ranges (hour 24, minute and second 60); `MST` takes Go's zone name heuristic (`UTC`, 3 to 5
capitals, `GMT+3`, `+03`); the layout constants (`RFC3339`, `RFC1123`, `ANSIC`, `Kitchen`, ...) are in the module; and `RFC3339` and
`RFC3339_NANO` texts take Go's fast path. Remaining differences (spec/std.md 67): a `Time` spans 1677 to 2262, so a year outside it
or no year is a `ParseErr`; a civil time in a daylight saving gap or overlap is the earlier one; a zone name `z` does not have at that
instant is a fixed zone with a zero offset; `ParseErr` wording is core's.

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
