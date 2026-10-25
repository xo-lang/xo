# std/time changes

## 2026-10-25: perf: a duration's text is read with unchecked byte reads (Duration.parse, a decoded JSON Duration)

- Kind: perf
- Commit: this commit
- Found by: the asm diff of text.parse_duration against LLVM on bench/perf/text dur_parse (a bounds check at each `b[i]` under `i < n`, n at most the length by space_start's contract, which the native range pass cannot relate)
- Go reference: time/format.go ParseDuration (go1.26.1), unchanged semantics
- Test: duration_parse_edges, parse_edge_scalars, text_table, std_error_display and codex_gaps on arm64; stdfuzz -area=time,strconv seeds 1, 7, 11 at 1500 lines per backend, no divergence

The reads of b[0], b[len - 1], the sign, the lone `0`, the number start, the
`.` and unit_of's bytes use raw.byte_at on the bytes' address, as the digit
loops already did: each is under `i < n` with n at most the length. A text
with a space or a non ASCII end byte still trims through strings.space_end and
space_start, whose results stay within [lo, hi]. bench/perf/text dur_parse,
instructions on arm64: 3.449 G to 3.276 G (0.950; LLVM 3.154 G, so 1.039 of
LLVM's, was 1.093), the checksum unchanged.

## 2026-10-25: perf: an RFC 3339 time's fraction and digits are read in single passes (Time.parse_rfc3339, a decoded JSON Time)

- Kind: perf
- Commit: this commit
- Found by: the sampler on bench/perf/text time_parse (XO_PROF=cpu at 1 kHz: text.parse_time_at 24 ms of 286 samples on arm64 and 18 ms of 210 on LLVM, the hottest line text.xo:601, the nine iteration fraction loop with its `i + 1 + k < j` test)
- Go reference: time/format_rfc3339.go parseRFC3339 (go1.26.1), whose fraction it parses and truncates to nanoseconds the same way
- Test: text_test on arm64 and LLVM; TestStdCorpora time and strconv on arm64 and LLVM (13 fraction length rows added at the guard: lengths 0 to 12, all nines at 9 and 10); stdfuzz -area=time (200 lines per backend, no divergence); the wasm goldens sample; the fixed point (bootstrap ok)

A full nine digit fraction now reads its digits with no test a digit, at the
form num2 and num4 take; a shorter one pads with zeros, and digits past the
ninth are dropped, not rounded, as before. The five separators are one xor
test instead of five branches, and the year's four digits are one 32 bit load
and one nibble test (high nibble 3, low nibble at most 9) instead of two num2
calls, four loads. bench/perf/text time_parse against main's std (one xo, A
the parent's std): instructions 0.955 on arm64 (0.948 to 0.964) and 0.976 on
LLVM (0.974 to 0.979), wall within noise on both. A single test a fraction
byte (`d | (9 - d)` sign bit) measured better on arm64 and worse on LLVM than
Go's two compares, so the compares stay.

## 2026-10-24: perf: an RFC 3339 time is parsed with one span check (Time.parse_rfc3339, a decoded JSON Time)

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 1 (the native JSON Time decode at 1.13 times LLVM's instructions: text.parse_time_at was 1,282 instructions to LLVM's 621, with 31 bounds checks and 29 overflow checks)
- Go reference: time/format_rfc3339.go parseRFC3339 (go1.26.1), which reads its bytes unchecked after one length test
- Test: TestStdCorpora time, json, jsondecode, jsonencode (Go's RFC 3339 parse and error rows) on arm64 and LLVM; goldens time_parse_digits, parse_edge_formats, parse_edge_scalars, time_fixed_offsets, json_text, json_value, json_decode_fields, native_json, text_table (arm64, LLVM with Go parity)

The parser checked every byte it read (b[o + k], fourteen of them in the digit
pairs) and every sum and product. It now checks its span once and reads the
bytes through the data address, and the arithmetic its own range checks bound
(digits, a year of 0 to 9999, the fields, the zone, the seconds) wraps
instead of checking. A span outside the Bytes is None (it faulted; no caller
passes one). Quiet `xo bench --ab` (one xo, A the parent's std): a list of 1000
Times decoded 20,000 times, wall 0.746 (0.735 to 0.752), instructions 0.783;
bench/perf/jsontime wall 0.948 (0.936 to 0.956), instructions 0.879. The
LLVM build gains too: native is at 0.984 of its instructions and 1.038 of its
wall time on the first.

## 2026-10-24: behavior: Time.parse_rfc3339 and a Time decoded from JSON refuse a time outside Time's range

- Kind: behavior
- Commit: this commit
- Found by: the parity sweep of the Go backend's lenient parses (owner decision, 2026-10-24; spec gap queued 2026-10-23)
- Go reference: time.Parse then UnixNano (which wraps outside 1677-09-21T00:12:43.145224192Z to 2262-04-11T23:47:16.854775807Z)
- Test: golden time_parse_digits (13 texts at and one past either end of the range, and far outside it: years 0001, 1500, 2300 and 9999)

`Time.parse_rfc3339` returned `Some` of a wrapped nanosecond count for a time
outside the 64-bit range of `Time` (2300-01-01 came back as a time in the
1700s), and std/json decoded it the same way. Both now answer `None` and the
decode error "expected RFC 3339 time string", as for any text that is not a
time; the range's two ends, to the nanosecond, are accepted. The native
backends (std/native/llrt text.xo parse_time) and the Go backend (std/rt
time.go inTimeRange) agree.

## 2026-10-24: perf: Duration.parse's digit loops on the native backends

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 1, a dur_parse loop at 1.24x LLVM's wall time with 1.06x its instructions (quiet --ab, 30 rounds)
- Go reference: time.ParseDuration (unchanged results)
- Test: golden duration_parse_edges (51 texts at the overflow edges: the largest and smallest durations and one past each, long runs of leading zeros, huge fractions, Unicode white space at either end; the same on arm64, LLVM and the Go backend), parse_edge_scalars, parse_edge_formats

std/native/llrt text.xo parse_duration read each digit's byte three times
and checked `nv * 10`, `MIN_INT + c` and `t - c` for overflow, which its
guards already rule out (nv at least -922337203685477580, c 0 to 9, t at
least MIN_INT + c); LLVM's ranges dropped those checks and the native
backends did not. Each byte is read once and the guarded operations wrap.
The white space trim (strings.space_end and space_start, two calls the
native inliner does not take) runs only when the text's first or last
byte is white space or not ASCII. Quiet --ab, 30 rounds, against main: a
loop of 2,000,000 parses wall 0.885 (0.865 to 0.902), instructions 0.874;
bench/perf/text dur_parse wall about 0.93 net of its setup; against LLVM
the loop's wall went from 1.238 to 1.105.

## 2026-10-24: fix: Time.parse_rfc3339 on the native backends refuses what RFC 3339 forbids

- Kind: fix
- Commit: this commit
- Found by: the main check (TestTimeDiff) after the Go backend's parse became strict (9af520838)
- Go reference: time.Parse(time.RFC3339Nano, ...), whose lenient fallback accepts what the RFC forbids
- Test: golden time_parse_digits (Time.parse_rfc3339 beside the JSON decode, six new cases), TestTimeDiff

`Time.parse_rfc3339` on the native backends was written as the union of Go's
parsers (std/native/prelude.xo time_parse_rfc3339): it took a one digit hour
("1994-08-29T2:05:34Z"), a comma fraction, and an offset up to +24:60, while
the JSON decoder of the same type refused them (text.xo parse_time). Both
backends and both entry points now read the one shape: two and four digit
fields, upper case T and Z, a '.' fraction of one or more digits, a Z or
+-HH:MM offset with hours to 23 and minutes to 59.

## 2026-10-24: fix: the Go backend refuses an RFC 3339 time the spec does not allow

- Kind: fix
- Commit: this commit
- Found by: Xo Lang 1's time_parse_digits golden (ad8786870): the Go backend took a one digit hour
- Go reference: time.Parse(time.RFC3339Nano, ...) (its lenient fallback accepts what RFC 3339 forbids)
- Test: golden time_parse_digits (arm64, LLVM, Go parity), eleven new cases

`Time.parse_rfc3339` and a `Time` decoded from JSON on the Go backend (std/rt
time.go parseRFC3339) called time.Parse, which after its strict RFC 3339
parser refuses falls back to a lenient layout parse: it accepted
"2026-10-24T0:02:03Z" (a one digit hour, minute, second, month or day), a
lower case t or z, and a comma fraction. The native backends refuse those
(text.xo parse_time: fixed two and four digit fields, T, a '.' fraction, Z or
+-HH:MM). The Go backend now checks the same shape first, then lets time.Parse
check the ranges.

## 2026-10-23: perf: the monotonic clock on macOS is CLOCK_UPTIME_RAW, as Go's nanotime

- Kind: perf
- Commit: this commit
- Found by: macOS `sample` of bench/perf/conc rwmutex (clock reads were most of the running samples), Xo Lang 3
- Go reference: runtime nanotime on darwin (mach_absolute_time)
- Test: xo bench --ab (bench/results/mono-clock-2026-10-23.md)

On macOS `mono_ns` (the scheduler's stamps, timers, budgets, and the monotonic
reading of `Time`) read CLOCK_MONOTONIC, which goes through gettimeofday and
costs several times a CLOCK_UPTIME_RAW read (one commpage read). It now reads
CLOCK_UPTIME_RAW, as Go does. Behavior change: the clock stops while the Mac
sleeps, so a duration measured across a system sleep no longer includes it
(spec/std.md makes no promise either way; Linux and Windows are unchanged).

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
