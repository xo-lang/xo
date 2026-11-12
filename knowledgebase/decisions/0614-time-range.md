# 0614: Times outside 1677 to 2262 (years 1 to 9999 in formats)

Status: Proposed (Xo Lang 3, 2026-11-11, at the lead's request; for the owner)

## Context

`Time` is the nanoseconds since 1970 in one Int, so it covers 1677-09-21 to
2262-04-11 (spec/std.md 3.3; decision 0364 made a parse outside the range
fail instead of wrapping). Go's `time.Time` covers years 1 to 9999 and more,
and formats Xo programs read carry those years: BigQuery TIMESTAMP is
0001-01-01 to 9999-12-31, ASN.1 GeneralizedTime and UTCTime, YAML
timestamps and mail dates can name any of them (the lead's report: each hits
"out of range" today). The common case is a sentinel: `0001-01-01T00:00:00Z`
(Go's zero Time, written by many Go services) and `9999-12-31T23:59:59Z`
("never expires").

`Date { year: Int, month: Int, day: Int }` and `Civil` (spec/std.md 13.2)
already hold any year; only the parse, the format and the codecs stop at
`Time`'s range.

## Options

(a) **Go's representation.** `Time` becomes two words: seconds since
0001-01-01 and nanoseconds (Go also keeps a monotonic reading and a
location; Xo's `Time` is an instant, so only the two). Range: years 1 to
9999 and far beyond.
- Cost, measured (bench/results/timerep-2026-11-11.md, proxy, host 1
  --ab): `t + d` 2.90x, `a - b` 1.61x, sort 1.41x and 1.43x memory, `now` at
  most 1.13x (likely about 1.0 natively), format within noise.
- Cost, not measured: `Atomic[Time]` stops being one word (16 byte CAS or a
  lock); every struct, list, map key and channel slot holding a Time grows by
  8 bytes; `Time - Time` needs a saturation rule (Go saturates at Duration's
  range, 292 years), so `a - b` stops being exact.
- Work: every backend (native layouts, LLVM, wasm, the Go backend's
  `type Time int64`), the records and cache formats, derive Json/Cbor/Proto
  and the wire forms that write a Time as one integer; a seed bump.

(b) **Keep `Time`; make `Civil` the wide type for I/O.** `Time` stays one
word for arithmetic and clocks. Text and wire formats that may hold any year
get a `Civil` form:
- `time.parse_civil(layout, s) -> Civil ! ParseErr` and
  `Civil.format(layout)`, years 1 to 9999 (RFC 3339 first);
- `Civil.unix() -> Int` (seconds; fits an Int for years 1 to 9999) and
  `Civil.nanos_of_second()`, so wide values compare and sort without `Time`;
- the codecs that meet such years (std/json and derive Json for a `Civil`
  field, std/yaml, std/asn1 GeneralizedTime, std/mail dates, the BigQuery
  and SQL TIMESTAMP columns) decode to `Civil` when the field is `Civil`;
  a `Time` field keeps decision 0364's refusal.
- Cost: none on the hot paths (rows above unchanged); a program that wants
  year 1 says so in its field type. Go code that keeps `time.Time` zero values
  maps to `Civil` or `Time?` (`None` for Go's zero) at the boundary.
- Work: std only (time, json, yaml, asn1, mail, sql/bigquery), no compiler
  or format change.

(c) **Status quo.** Out of range is an error (0364). Cost: none; programs
reading Go's zero Time or a 9999 sentinel fail, and each one writes its
own string handling.

## Recommendation

(b). The measured cost of (a) lands on the operations every server runs per
request (`now`, `t + d`, `a - b`, comparisons), and the unmeasured costs
(atomics, every stored Time doubling) are larger, all to serve dates that
occur in data as sentinels and archives, not in clocks. (b) gives those
formats every year Go reads, with no cost to programs that never see one.
Its gap: arithmetic on a year 1 instant goes through `Civil.unix()` seconds
(or `Date`), not `Time`; if a real workload needs instant arithmetic past
2262, (a) can be reopened with that workload as its benchmark.

## Consequences (if (b) is accepted)

- spec/std.md 13.2 and 13.3 gain the Civil parse, format, unix and the
  codec rules; decision 0364 is unchanged for `Time`.
- One golden per codec at both ends (0001-01-01T00:00:00Z and
  9999-12-31T23:59:59.999999999Z) against Go's output.
- `Time?` with `None` for Go's zero Time is documented as the mapping for
  Go interop (`use go`, gobind).

## Alternatives

- A 128-bit nanosecond count: the same two words as (a), plus 128-bit
  arithmetic on every operation.
- Microseconds in one word (range ±292,000 years): loses Go's nanosecond
  precision and changes every Duration conversion.
- A separate `WideTime` instant type with arithmetic: (b) plus a second
  instant type to learn and convert; `Civil` already exists and holds the
  year.
