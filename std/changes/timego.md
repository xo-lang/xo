# std/timego changes

## 2026-11-16: fix: parse reads a signed two digit year as Go does

- Kind: fix
- Commit: this commit
- Found by: task #116, timego fuzz seed 20 (layout "06-1-2", input "-3-4-6")
- Go reference: time.Parse stdYear (the two bytes are read with atoi, which takes a sign)
- Test: std/timego_test.xo "parse matches Go" ("-3" is 1997, "+3" is 2003; it failed before on arm64)
- Shipped in: not yet released

A two digit year (`06`) of a sign and a digit failed as "expected 2
digit year"; Go reads the two bytes with atoi, so "-3" is year -3 + 2000
(1997) and "+3" is 2003. parse now does the same.

## 2026-11-16: fix: parse reads a signed fixed width fraction as Go does

- Kind: fix
- Commit: this commit
- Found by: task #116, timego fuzz seed 10 (layout "15:04:05.000000000", input "11:41:44.-567712250")
- Go reference: time.parseNanoseconds (the digits are read with atoi, which takes a sign)
- Test: std/timego_test.xo "parse matches Go" (the seed 10 case and a plus sign; it failed before on arm64)
- Shipped in: not yet released

A fixed width fractional second (`.000`, `.000000000`) whose digits
started with a sign failed as "cannot parse"; Go reads the digits with
atoi, so a minus sign gives "fractional second out of range" and a plus
sign parses (`.+56771225` is 56771225 nanoseconds). parse now does the
same.

## 2026-11-16: fix: parse takes an offset in the text over a zone name

- Kind: fix
- Commit: this commit
- Found by: task #116, timego fuzz seed 8 (layout "2006-01-02T15:04:05Z07:00 MST", input "2017-02-01T16:26:03-09:30 ZZZ")
- Go reference: time.parse (zoneOffset is applied before zoneName)
- Test: std/timego_test.xo "parse matches Go" (the seed 8 case; it failed before on arm64)
- Shipped in: not yet released

A text with both a numeric offset and a zone name that the location does
not know used the zone name's rule (a fixed zone with an unknown offset,
so UTC) and ignored the offset: "16:26:03-09:30 ZZZ" parsed as 16:26:03
UTC, 9 h 30 min early. Go applies the offset when the text has one and
reads the name only for the zone's label; parse now does the same.

## 2026-11-14: perf: format writes one and two byte separators in one store; parse borrows its layout, input and zone (native)

- Kind: perf
- Commit: this commit
- Found by: the std/time speed work of decision 0616 (std/evidence/time/perf.md at da2ba49a9: timego/format 1.237 and timego/parse 1.210 of Go; XO_PROF: rs_append's copy 9% of format; drop List[Int] 11% of parse)
- Go reference: time.appendFormat (a layout's separators appended as bytes); time.Parse takes its strings by reference
- Test: TestStdCorpora time, timefuzz and timego on go, arm64 and llvm; std/timego tests on arm64 and llvm; the rc stage of `xo dev dump ssa` shows Layout.parse's three parameters borrowed
- Timing: --ab on linux/arm64 host 1, 9 rounds: bench/perf/timego format 0.755 for the separators (41.21 to 31.1 ms, n=6); bench/perf/timego parse 0.806 for the borrowed parameters (10.27 to 8.28 ms, n=9); put_int and put_zone writing in place through a var parameter 0.993 (within noise, n=9), kept for its shape
- Shipped in: not yet released

Layout.format wrote each literal through BytesBuilder.write, a general copy; the one and two byte separators of the common layouts (`-`, `:`, `T`, `, `) are now one store. Layout.parse's error paths moved its input and the layout's text into the ParseErr they return, so the compiler passed the layout, the input and the zone as owned values and released them on every call; the error values now copy their texts (only when an error is made), and a fixed zone's abbreviation is a view of its label instead of the label moved out, so all three are borrowed.

## 2026-11-14: perf: Layout.format counts the days once, and writes a fractional second's digits in place (native)

- Kind: perf
- Commit: this commit
- Found by: the std/time speed work of decision 0616 (XO_PROF of bench/perf/timego format: time.civil, floor_div and days_from_civil 42%; std/evidence/time/perf.md: timego/format 1.328 of Go)
- Go reference: time.Time.appendFormat (absDate once; the weekday only for a layout naming it; appendNano writes the digits into the buffer)
- Test: std/native/time_test.xo (floor_div against its definition for every sign, exact multiples and the Int range's ends); TestStdCorpora time, timefuzz and timego on go, arm64 and llvm (the timego corpus's .000, ,000 and run of nines layouts against Go's output)
- Timing: --ab on linux/arm64 host 1, 9 rounds: bench/perf/timego format 0.951 (41.21 to 39.21 ms, n=6) for floor_div and the weekday, then 0.953 (41.57 to 39.6 ms, n=9) for the digits in place; bench/perf/time format 0.995 and civil 0.985 (within noise)
- Shipped in: not yet released

floor_div took a division and a remainder; with a positive divisor (every caller's) it now takes one division and a multiply. Layout.format computed the weekday from the date (another days count) for every layout; it now does so only for a layout with Mon or Monday. A fractional second built a nine digit Bytes of its own per call; its digits are now cut and written into the output.

## 2026-11-13: perf: RFC 3339 parse reads digit pairs, and format writes small numbers in one store

- Kind: perf
- Commit: this commit
- Found by: the std/time speed work of decision 0616 (bench/perf/timego against Go: parse 1.74, format 1.23 on linux/arm64 host 2)
- Go reference: time.parseRFC3339 (fixed positions read two digits at a time), appendInt
- Test: std/timego_test.xo; TestStdCorpora timego on arm64 and llvm
- Timing: bench/perf/timego --ab on linux/arm64 host 1, 9 rounds: parse 0.819 (16.9 to 13.9 ms, n=6); put_int format 0.912 (46.1 to 42.1 ms, n=6)
- Shipped in: not yet released

The RFC 3339 fast path read each number a digit at a time through a bounds checked helper per byte; after its length check it now reads two digits per call. `put_int` writes a number of one or two digits, and a four digit year, as one little endian store. Output unchanged.

## 2026-11-10: refactor: one file with its bodies (decision 0588 stage 2)

- Kind: refactor
- Commit: this commit
- Found by: decision 0588 stage 2 (pure modules merged with their implementation)
- Go reference: none (no behavior change)
- Test: std/timego_test.xo (`xo test --std-module=timego` on native and llvm); internal/stdtest corpus timego on go, arm64 and llvm

std/timego.xo now holds the bodies of std/native/timego.xo (removed), and the Go
backend compiles them from it as its own package. The API, texts and errors
are unchanged: every item the declaration file had is `pub`, with the same
parameter names.

## 2026-11-09: feature: timego.go_text(e) gives Go's time.Parse error text

- Kind: feature
- Commit: this commit
- Found by: Xo Azure 1 (azidentity prints Go's text for a bad time in a developer tool's output; a fixed width rebuild stood in)
- Go reference: time/format.go parse and ParseError.Error (go1.26.1), time.quote; the oracle internal/stdtest/timeref now prints Go's error text
- Test: testdata/corpus/timego (every failing parse case compares `go_text` with Go's text, go, arm64 and llvm)
- Shipped in: not yet released

`timego.go_text(e)` returns `parsing time "X" as "LAYOUT": cannot parse "REST" as "ELEM"`, `parsing time "X": extra text: "REST"` or
`parsing time "X": month out of range` for a failed timego parse, as Go prints them. `time.ParseErr` gains four defaulted fields
(`go_kind`, `go_elem`, `go_rest`, `go_msg`) the scanner fills; an error from anywhere else gives its display text.

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

Timing (host 2, n=5, bench/results/std-twin-timego-2026-11-06.md, d3fcfb4f9): format 1.15x Go, parse 1.94x (3.33x before the allocation free format and the direct instant).
