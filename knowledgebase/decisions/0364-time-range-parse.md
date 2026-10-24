# 0364: Parsing a Time outside its range fails instead of wrapping

Status: Accepted (owner, 2026-10-24; implemented by Xo Lang 3)

## Context

`Time` is the nanoseconds since 1970 in an `Int`, so it covers 1677-09-21T00:12:43.145224192Z
to 2262-04-11T23:47:16.854775807Z (spec/std.md 3.3, civil time). `Time.parse_rfc3339` and the
decode of a `Time` from JSON read any year 0000 to 9999 and returned the nanosecond count
wrapped modulo 2^64: `2300-01-01T00:00:00Z` came back as a time in the 1700s, on the native
backends (std/native/llrt text.xo `parse_time`) and, as Go's `UnixNano` does, on the Go
backend. The spec was silent; the parity sweep of 2026-10-24 found it, and the owner decided
it must not wrap.

## Decision

A time outside the range of `Time` is not a `Time`:

- `Time.parse_rfc3339(s)` returns `None`.
- Decoding a `Time` from JSON is a decode error, with the text of any other bad time
  ("expected RFC 3339 time string").
- Both ends of the range, to the nanosecond, parse.

Construction paths that already fault on overflow (`Time.from_unix`) are unchanged. The owner's
word "faults" is read as these refusals: a parse of untrusted text answers with `None` or an
error like every other malformed time, and does not abort the program.

## Consequences

- A program that stored a far-future time (year 2300 and later, or 1600 and earlier) in JSON
  now gets a decode error where it got a wrong time. No known program does.
- Native and Go backends agree (golden `time_parse_digits`, 13 texts at and past either end).
- `Civil.instant` already refuses a time outside the range (`CivilErr.Invalid`); the parse now
  matches it.

## Alternatives

- Abort on an out of range text: a parse of external input must not end the program.
- Widen `Time` (a 128-bit count): a representation change across every backend and the wire
  format, for dates nothing in the language's target use needs.
- Keep wrapping and document it: the result is a silently wrong time.
