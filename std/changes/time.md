# std/time changes

## 2026-10-22: fix: Earlier and Later in a daylight saving gap were swapped

- Kind: fix
- Commit: 86e417086
- Found by: a field report (02:30 on 2026-03-08 in New York gave Earlier 03:30 EDT and Later 01:30 EST, so Earlier was after Later)
- Go reference: none (Go's time.Date picks one instant and documents no choice); Temporal's `disambiguation: "earlier"` and `"later"`
- Test: internal/gen/testdata/golden/time_civil ("order": gaps and overlaps in New York, Berlin, Sydney and Lord Howe's 30 minute change, Earlier never after Later, and the distance between the two), internal/llvm/testdata/golden/tz_table (arm64, LLVM, and the Go backend agree)

`instant` with `Ambiguity.Earlier` in a spring forward gap used the offset
before the change, which gives the later instant, and `Later` the offset after
it. The overlap case was right. Both now give the earlier and the later
instant: 02:30 on 2026-03-08 in New York is 01:30 EST with `Earlier` and 03:30
EDT with `Later`; in Lord Howe's gap (02:00 to 02:30) 02:15 is 01:45 and 02:45.
spec/std.md 13.2 said the offset before and after the change (Python's `fold`),
which is the reverse of the names; it now says the earlier and the later
instant. A program that relied on the old gap result swaps the two arguments.
The native `civil_instant` and the Go backend's `civilInstant` changed alike.

## 2026-10-22: perf: format looks the zone up once

- Kind: perf
- Commit: fdc3297ca
- Found by: the std Go ranking (Layout.format 1.7x Go's time.Format), XO_PROF=cpu
- Go reference: time.Format
- Test: std module tests native/time (arm64, llvm); goldens time_format, time_zones

`Layout.format` and `format` called the zone twice per call that printed a zone
name (the offset, then the abbreviation, each walking the footer rule with
`tzset`). A new `tz.lookup` returns the offset and the name's position in one
walk and `{zone}` builds the string only when printed. Number fields of width
2 and 4 write their digits without the division loop, and the output buffer
starts with room for a typical result. Same output. 300,000 ISO in UTC and
300,000 long layouts in America/New_York: 1,707 M to 1,467 M instructions
(native arm64; `xo bench --ab` wall 0.798, instructions 0.859); Go's
`time.Format` on the same loop: 983 M, so 1.74x to 1.49x Go.
bench/results/time-format-2026-10-22.md.

## 2026-10-22: perf: parse no longer builds strings it does not use

- Kind: perf
- Commit: 6235fe89d
- Found by: the std Go ranking (Layout.parse 3.4x Go's time.Parse), XO_PROF=cpu
- Go reference: time.Parse
- Test: std module tests native/time (arm64); goldens time_format, time_zones

`parse` and `Layout.parse` built the error-text date (`when`, six `pad` calls)
on every successful call, named each number field with a string it then
matched on, built a name list for every month and weekday name, and sliced the
text for each literal. The date text is built only in the error paths, number
fields carry an integer, short month and weekday names compare three bytes
against one packed string, and literals compare bytes in place. Same results
and error texts. 300,000 ISO and 300,000 long layout parses: 3,605 M to 1,622 M
instructions (native arm64; `xo bench --ab` wall 0.409, instructions 0.450);
the same program against Go's `time.Parse`: 1,290 M, so 2.8x to 1.26x Go.
bench/results/time-parse-2026-10-22.md.

## 2026-10-22: guidance: std formats with a lazy const Layout, not the layout text

- Kind: perf
- Commit: 8ddf1bbe0
- Found by: bench/results/time-format-cache-variants-2026-10-22.md (ADR 0332's two estimates, measured)
- Go reference: none
- Test: none (std's own callers: native/http.xo HTTP_DATE_LAYOUT, native/mail.xo DATE_LAYOUT)

`time.format(t, z, text)` and `time.parse` parse the layout text on every call
(5,344 M instructions for 600,000 formats). A Mutex held Map from text to
Layout costs 1,995 M (`xo bench --ab` 0.373x instructions) and a lazy
`const L: time.Layout = time.layout(text)` 1,710 M (a further 0.856x
instructions, 0.822x wall); Go's time.Format is 983 M. The const is the
shipped choice: std callers with a fixed layout (the HTTP date) use one.

## 2026-10-22: perf: format writes digits into one byte buffer

- Kind: perf
- Commit: da2bdc4c9
- Found by: the mail Date header measurement (a Layout formatter was 13% slower than a hand written one), Xo Lead
- Go reference: time.Time.appendFormat (appendInt, appendNano)
- Test: std module tests native/time ("every format field and argument form"; 2,625 layout, zone and time combinations agreed with the old text)

`Layout.format` and `time.format` build the text in a byte buffer with the
digits written straight in (no `"${n}"` strings, no padding by
concatenation, no name lists built per call), and each item carries its
argument as a number instead of comparing it. Same text. 600,000 formats
of a Layout: 2,390 M to 1,637 M instructions (-31.5%, native arm64; Go's
955 M, so 2.5x to 1.7x Go); the Str layout form 6,232 M to 5,626 M (it
parses the layout on every call). bench/results/time-format-bytes-2026-10-22.md.

## 2026-10-22: feature: time.Layout, a layout parsed once

- Kind: feature
- Commit: 3d8c505f0
- Found by: bench/results/time-layout-parse-cost-2026-10-21.md
- Test: std module tests native/time ("a Layout formats and parses as its text")

`time.layout(text)` returns a `Layout` (faults on a bad layout; a constant
layout is checked at compile time), `Layout.format(t, z)` and
`Layout.parse(s, z)` give the results of `format` and `parse` with that text,
and `RFC3339_LAYOUT`, `RFC1123_LAYOUT`, `DATE_LAYOUT` and `TIME_LAYOUT` are the
named layouts. Held in a `const` (decision 0332) the layout is parsed once per
process on first use. std/http keeps its literal layout until a seed has
decision 0332 (the compiler's own build imports it).
The Str functions are unchanged. 600,000 formats: 6,202 M to 2,375 M
instructions (native arm64; Go's time.Format 955 M, so 6.5x to 2.5x Go);
600,000 parses: 3,291 M to 1,586 M (Go 471 M, 7.0x to 3.4x). The Go backend
gains little (5,666 M to 5,522 M format). `RFC1123_LAYOUT` prints the zone as
UTC (the `{zone}` field), unlike Go's abbreviation; HTTP dates keep a literal
GMT layout.

## 2026-10-21: perf: layout fields recognized in bytes

- Kind: perf
- Commit: (filled in after the push)
- Found by: bench/results/regexp-time-csv-vs-go-2026-10-21.md
- Test: std module tests native/time on arm64 and LLVM

A layout field and its argument are recognized by comparing the layout's bytes
(no Str per field name), and `format` builds its text in one StrBuilder.
300,000 x 2 formats 7,390 M to 6,206 M instructions, 300,000 parses 3,768 M to
3,300 M. Results and error texts are the same.

## 2026-10-21: perf: layout items carry an integer field code

- Kind: perf
- Commit: (filled in after the push)
- Found by: XO_PROF=cpu (bench/results/regexp-time-csv-vs-go-2026-10-21.md)
- Test: std module tests native/time on arm64 and LLVM

`format` and `parse` switch on an integer code per layout item instead of
comparing field names. 300,000 x 2 formats 9,612 M to 7,390 M instructions,
300,000 parses 4,824 M to 3,768 M. Results and error texts are the same.

## 2026-10-21: perf: layout fields validated without a list

- Kind: perf
- Commit: (filled in after the push)
- Found by: XO_PROF=cpu (bench/results/regexp-time-csv-vs-go-2026-10-21.md)
- Test: std module tests native/time on arm64 and LLVM

A layout field is checked without building its argument list; the list is
built only for the error text. 300,000 formats: 9,612 M to 8,725 M
instructions; parses 4,824 M to 4,382 M. Results and error texts are the same.

## 2026-10-21: fix: the sign of an offset under a minute

- Kind: fix
- Commit: dd2e401a
- Found by: differential test against Go
- Go reference: time.Time.Format's zone offset (format.go, go1.26.1)
- Test: std/native/time_test.xo (1,200 random instants in fixed zones, both layouts covering every field, against Go's Format); internal/llvm/testdata/golden/time_fixed_offsets (arm64, LLVM, and the Go backend agree)

`{offset}` and `{offset:compact}` of a zone whose offset is negative but
under a minute (-59s) showed `-00:00`; Go takes the sign of the offset in
whole minutes and shows `+00:00`. Native and the Go backend's formatter
both had it. Everything else matched Go on all 1,200 instants.
